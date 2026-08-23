package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/meltforce/vimmary/internal/storage"
)

// topicReuseLimit caps how many existing tags the summarize prompt carries.
// The list is ordered by usage, so the cut drops the singleton tail — exactly
// the tags not worth steering new summaries toward.
const topicReuseLimit = 40

// consolidateFloor is the tag count below which consolidation does nothing:
// a hand-countable set has nothing worth an LLM call.
const consolidateFloor = 10

// existingTopics returns the reuse vocabulary for the summarize prompt, nil
// on any failure — a summary must never fail because a hint query did.
func (s *Service) existingTopics(ctx context.Context, userID int) []string {
	counts, err := s.db.ListTopicCounts(ctx, userID)
	if err != nil {
		s.log.Warn("failed to load topic vocabulary for prompt", "user_id", userID, "error", err)
		return nil
	}
	if len(counts) > topicReuseLimit {
		counts = counts[:topicReuseLimit]
	}
	topics := make([]string, len(counts))
	for i, c := range counts {
		topics[i] = c.Topic
	}
	return topics
}

// TopicConsolidation reports what a consolidation run did.
type TopicConsolidation struct {
	Before        int               `json:"before"`
	After         int               `json:"after"`
	Merged        int               `json:"merged"`
	UpdatedVideos int               `json:"updated_videos"`
	Mapping       map[string]string `json:"mapping"`
}

// ConsolidateTopics asks the configured LLM to merge near-duplicate topic
// tags across the whole library and applies the mapping to every row. It is
// one model call plus one UPDATE per affected row, and it only ever renames
// tags — summaries, key points and embeddings stay untouched.
func (s *Service) ConsolidateTopics(ctx context.Context, userID int) (*TopicConsolidation, error) {
	counts, err := s.db.ListTopicCounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	if len(counts) < consolidateFloor {
		return &TopicConsolidation{Before: len(counts), After: len(counts), Mapping: map[string]string{}}, nil
	}

	summarizer, providerName, err := s.getSummarizer(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get summarizer: %w", err)
	}

	answer, err := summarizer.Complete(ctx, buildConsolidationPrompt(counts), 8192)
	if err != nil {
		return nil, fmt.Errorf("consolidation call (%s): %w", providerName, err)
	}

	mapping, err := parseTopicMapping(answer)
	if err != nil {
		return nil, err
	}
	mapping = sanitizeTopicMapping(mapping, counts)

	result := &TopicConsolidation{Before: len(counts), Merged: len(mapping), Mapping: mapping}
	if len(mapping) == 0 {
		result.After = len(counts)
		return result, nil
	}

	rows, err := s.db.ListVideoTopics(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list video topics: %w", err)
	}
	for _, row := range rows {
		next := applyTopicMapping(row.Topics, mapping)
		if next == nil {
			continue
		}
		if err := s.db.UpdateVideoTopics(ctx, row.ID, next); err != nil {
			return result, fmt.Errorf("update topics for %s: %w", row.ID, err)
		}
		result.UpdatedVideos++
	}

	after, err := s.db.ListTopicCounts(ctx, userID)
	if err != nil {
		s.log.Warn("failed to recount topics after consolidation", "error", err)
		result.After = len(counts) - len(mapping)
	} else {
		result.After = len(after)
	}

	s.log.Info("topics consolidated", "user_id", userID,
		"before", result.Before, "after", result.After, "updated_videos", result.UpdatedVideos)
	return result, nil
}

func buildConsolidationPrompt(counts []storage.TopicCount) string {
	var b strings.Builder
	b.WriteString(`These are the topic tags of a video library, one per line as "tag — usage count".
Many are near-duplicates: singular/plural pairs, spelling or hyphenation variants,
translations of the same concept, and narrow one-off tags that an existing broader
tag already covers.

Produce a merge mapping. Rules:
- Map a tag onto another EXISTING tag from the list; prefer the more-used variant
  as the target, and keep its exact spelling.
- Merge only tags that genuinely mean the same thing or where the narrow tag adds
  nothing over the broader one. When in doubt, do not merge.
- Do not invent new tags and do not map a tag onto itself.

Return ONLY valid JSON of this shape, listing only tags that change:
{"mapping": {"old tag": "target tag"}}

Tags:
`)
	for _, c := range counts {
		fmt.Fprintf(&b, "%s — %d\n", c.Topic, c.Count)
	}
	return b.String()
}

// parseTopicMapping extracts the {"mapping": …} object from the model answer,
// tolerating markdown fences around it.
func parseTopicMapping(text string) (map[string]string, error) {
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("consolidation answer carries no JSON object")
	}
	var parsed struct {
		Mapping map[string]string `json:"mapping"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &parsed); err != nil {
		return nil, fmt.Errorf("parse consolidation mapping: %w", err)
	}
	if parsed.Mapping == nil {
		return map[string]string{}, nil
	}
	return parsed.Mapping, nil
}

// sanitizeTopicMapping drops what the model must not do anyway: identity
// entries, empty sides, targets that are not existing tags, and chains — a
// chain (a→b, b→c) is flattened to its final target, with a bound so a cycle
// cannot loop forever.
func sanitizeTopicMapping(mapping map[string]string, counts []storage.TopicCount) map[string]string {
	existing := make(map[string]bool, len(counts))
	for _, c := range counts {
		existing[c.Topic] = true
	}

	clean := make(map[string]string, len(mapping))
	for old, target := range mapping {
		old = strings.TrimSpace(old)
		target = strings.TrimSpace(target)
		if old == "" || target == "" || old == target || !existing[old] || !existing[target] {
			continue
		}
		clean[old] = target
	}

	// Flatten chains to their terminal target; an entry whose walk revisits a
	// node sits on a cycle — the model contradicted itself there, so every
	// entry on that cycle is dropped rather than merged in arbitrary order.
	resolved := make(map[string]string, len(clean))
	for old := range clean {
		target := clean[old]
		visited := map[string]bool{old: true}
		acyclic := true
		for {
			if visited[target] {
				acyclic = false
				break
			}
			visited[target] = true
			next, isMapped := clean[target]
			if !isMapped {
				break
			}
			target = next
		}
		if acyclic {
			resolved[old] = target
		}
	}
	return resolved
}

// applyTopicMapping returns the rewritten tag list, or nil when nothing
// changes. Order is kept; a merge that produces a duplicate keeps the first.
func applyTopicMapping(topics []string, mapping map[string]string) []string {
	changed := false
	seen := make(map[string]bool, len(topics))
	next := make([]string, 0, len(topics))
	for _, t := range topics {
		if target, ok := mapping[t]; ok {
			t = target
			changed = true
		}
		if seen[t] {
			changed = true
			continue
		}
		seen[t] = true
		next = append(next, t)
	}
	if !changed {
		return nil
	}
	return next
}
