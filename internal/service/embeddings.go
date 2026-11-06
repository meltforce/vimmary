package service

import (
	"context"
	"time"
)

// reembedInterval and reembedBatch pace the re-embed. A model change leaves the
// whole library stale at once; 50 rows every two minutes works 484 rows (the
// library on 2026-11-06) off in about 20 minutes without a burst against the
// gateway.
const (
	reembedInterval = 2 * time.Minute
	reembedBatch    = 50
)

// StartEmbeddingBackfill runs the re-embed loop until ctx is cancelled. It
// starts after pollStartDelay for the same reason the pollers do: the embedder
// may dial over the tsnet node, and nothing touches the network before the
// listener is up.
func (s *Service) StartEmbeddingBackfill(ctx context.Context) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollStartDelay):
		}

		ticker := time.NewTicker(reembedInterval)
		defer ticker.Stop()
		for {
			s.reembedStale(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// reembedStale embeds one batch of rows whose embedding is missing or was
// produced by a model other than the configured embedder's.
//
// Two kinds of row land here. After a change of embedding model, every row:
// search filters on the query's model, so a row keeps its old vector but drops
// out of semantic search until it is redone. And rows whose embedding call
// failed at summary time — summarizeAndStore saves the summary without one
// rather than failing it — which before this loop stayed out of semantic
// search for good.
//
// The text is the one summarizeAndStore embeds, title and summary, so a
// re-embedded row is indistinguishable from a freshly summarized one.
func (s *Service) reembedStale(ctx context.Context) {
	model := s.embedder.Model()
	rows, err := s.db.ListStaleEmbeddings(ctx, model, reembedBatch)
	if err != nil {
		s.log.Warn("re-embed: list stale rows failed", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	done := 0
	for _, r := range rows {
		if ctx.Err() != nil {
			return
		}
		embedding, err := s.embedder.Embed(ctx, r.Title+"\n\n"+r.Summary)
		if err != nil {
			// One failure usually means the embedder is unreachable, and the
			// rest of the batch would fail the same way. The next cycle retries.
			s.log.Warn("re-embed: embedding failed, stopping this cycle",
				"video_id", r.ID, "model", model, "done", done, "error", err)
			break
		}
		if err := s.db.UpdateVideoEmbedding(ctx, r.ID, embedding, model); err != nil {
			s.log.Warn("re-embed: store failed", "video_id", r.ID, "error", err)
			continue
		}
		done++
	}
	s.log.Info("re-embed: batch finished", "model", model, "embedded", done, "batch", len(rows))
}
