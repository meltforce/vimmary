package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/meltforce/vimmary/internal/cast2md"
	"github.com/meltforce/vimmary/internal/storage"
)

// pollStartDelay keeps the first poll out of the startup path, so migrations,
// the database pool and the HTTP listener are all up before cast2md is called.
const pollStartDelay = 30 * time.Second

// staleAfterMinutes is how long a podcast row may sit in pending or processing
// before the poller re-queues it. It has to exceed the longest normal
// processing time, which is dominated by the LLM call.
const staleAfterMinutes = 15

// staleRequeueLimit caps one requeue pass, so a large backlog is worked off
// over several ticks instead of filling the queue in one go.
const staleRequeueLimit = 50

// StartPodcastPoller runs the cast2md poll loop until ctx is cancelled. It is a
// no-op when cast2md is not configured.
func (s *Service) StartPodcastPoller(ctx context.Context) {
	if s.cast2md == nil {
		s.log.Info("podcast poller not started", "reason", "cast2md not configured")
		return
	}

	interval := time.Duration(s.cast2mdCfg.PollIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 15 * time.Minute
	}

	s.log.Info("podcast poller starting",
		"base_url", s.cast2md.BaseURL(), "interval", interval, "max_per_poll", s.maxPerPoll())

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollStartDelay):
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			s.pollOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) maxPerPoll() int {
	if s.cast2mdCfg.MaxPerPoll > 0 {
		return s.cast2mdCfg.MaxPerPoll
	}
	return 25
}

// pollOnce runs one cycle: re-queue whatever got stuck, then advance every
// enabled subscription.
func (s *Service) pollOnce(ctx context.Context) {
	s.requeueStalePodcasts(ctx)

	subs, err := s.db.ListEnabledSubscriptions(ctx)
	if err != nil {
		s.log.Error("failed to list podcast subscriptions", "error", err)
		return
	}

	for _, sub := range subs {
		if ctx.Err() != nil {
			return
		}
		if err := s.pollSubscription(ctx, sub); err != nil {
			s.log.Warn("podcast poll failed",
				"feed_id", sub.FeedID, "user_id", sub.UserID, "error", err)
			if dbErr := s.db.SetSubscriptionError(ctx, sub.ID, err.Error()); dbErr != nil {
				s.log.Error("failed to record poll error", "feed_id", sub.FeedID, "error", dbErr)
			}
		}
	}
}

// requeueStalePodcasts puts rows back on the queue that have been pending or
// processing for too long. This covers a restart in the middle of processing, a
// queue overflow and a crashed job with one mechanism.
func (s *Service) requeueStalePodcasts(ctx context.Context) {
	stale, err := s.db.ListStalePodcasts(ctx, staleAfterMinutes, staleRequeueLimit)
	if err != nil {
		s.log.Error("failed to list stale podcast rows", "error", err)
		return
	}
	for _, v := range stale {
		episodeID, err := strconv.Atoi(v.ExternalID)
		if err != nil {
			s.log.Warn("stale podcast row has a non-numeric episode ID",
				"video_id", v.ID, "external_id", v.ExternalID)
			continue
		}
		s.log.Info("re-queuing stale podcast row",
			"video_id", v.ID, "episode_id", episodeID, "status", v.Status)
		s.ProcessEpisodeAsync(v.UserID, episodeID, v.DetailLevel)
	}
}

// queueEpisode writes the row and enqueues the work, reporting whether it got
// that far. The row is written before the caller moves the watermark past the
// episode: if the process dies in between, the row exists and
// requeueStalePodcasts finds it, so the in-memory queue is not a place work
// can be lost.
func (s *Service) queueEpisode(ctx context.Context, sub storage.PodcastSubscription, episodeID int) bool {
	if _, err := s.EnsureEpisodeRow(ctx, sub.UserID, episodeID, sub.DetailLevel); err != nil {
		s.log.Warn("failed to create podcast row, skipping episode",
			"episode_id", episodeID, "feed_id", sub.FeedID, "error", err)
		return false
	}
	s.ProcessEpisodeAsync(sub.UserID, episodeID, sub.DetailLevel)
	return true
}

// pollSubscription advances one subscription by its watermark.
func (s *Service) pollSubscription(ctx context.Context, sub storage.PodcastSubscription) error {
	feedID, err := strconv.Atoi(sub.FeedID)
	if err != nil {
		return fmt.Errorf("subscription has a non-numeric feed ID %q", sub.FeedID)
	}

	// First poll after the subscription was switched on. It reads the feed's
	// newest completed episodes, adopts the newest timestamp as the watermark,
	// and summarizes as many of them as initial_backfill asks for. The
	// watermark therefore comes from cast2md's clock, which removes both the
	// timezone and the clock-skew failure mode, and because it is the newest of
	// the batch, none of these episodes is fetched again on the next tick.
	//
	// initial_backfill = 0 restores plain "from now on": watermark only.
	if !sub.Initialized {
		limit := sub.InitialBackfill
		if limit < 1 {
			limit = 1
		}
		eps, err := s.cast2md.ListCompleted(ctx, cast2md.ListCompletedOptions{
			FeedID: feedID,
			Order:  cast2md.OrderUpdatedDesc,
			Limit:  limit,
		})
		if err != nil {
			return fmt.Errorf("initial poll: %w", err)
		}

		watermark := ""
		if len(eps) > 0 {
			watermark = eps[0].UpdatedAt
		}

		queued := 0
		if sub.InitialBackfill > 0 {
			// Oldest first, so the list fills in reading order.
			for i := len(eps) - 1; i >= 0; i-- {
				if s.queueEpisode(ctx, sub, eps[i].ID) {
					queued++
				}
			}
		}

		if err := s.db.SetSubscriptionWatermark(ctx, sub.ID, watermark, true); err != nil {
			return err
		}
		s.log.Info("podcast subscription initialized",
			"feed_id", sub.FeedID, "user_id", sub.UserID,
			"watermark", watermark, "backfilled", queued)
		return nil
	}

	eps, err := s.cast2md.ListCompleted(ctx, cast2md.ListCompletedOptions{
		Since:  sub.Watermark,
		FeedID: feedID,
		Order:  cast2md.OrderUpdatedAsc,
		Limit:  s.maxPerPoll(),
	})
	if err != nil {
		return fmt.Errorf("poll: %w", err)
	}

	watermark := sub.Watermark
	queued := 0
	for _, ep := range eps {
		// The watermark advances either way. A single broken episode must not
		// block every later one in the feed.
		if s.queueEpisode(ctx, sub, ep.ID) {
			queued++
		}
		watermark = ep.UpdatedAt
	}

	if err := s.db.SetSubscriptionWatermark(ctx, sub.ID, watermark, true); err != nil {
		return err
	}
	if queued > 0 {
		s.log.Info("podcast episodes queued",
			"feed_id", sub.FeedID, "user_id", sub.UserID, "count", queued, "watermark", watermark)
	}
	return nil
}

// BackfillResult reports what a backfill request did.
type BackfillResult struct {
	Queued  int `json:"queued"`
	Skipped int `json:"skipped"`
}

// backfillAllLimit caps "summarize everything" so one click cannot queue an
// unbounded number of LLM calls. The largest feed measured on 2026-08-06 had
// 367 completed episodes, so this is a backstop rather than a working limit.
const backfillAllLimit = 2000

// SummarizeAllCompleted summarizes every episode of a feed that cast2md has a
// transcript for. It is BackfillFeed without the small limit; the watermark
// stays where it is, so a running subscription is unaffected.
func (s *Service) SummarizeAllCompleted(ctx context.Context, userID int, feedID string) (*BackfillResult, error) {
	return s.BackfillFeed(ctx, userID, feedID, backfillAllLimit)
}

// TranscribeAllInFeed asks cast2md to download and transcribe every episode of
// a feed that has no transcript yet.
//
// This is the one place vimmary makes cast2md do expensive work. It queues jobs
// there and returns; the episodes reach vimmary through the ordinary poll,
// because finishing a transcription updates the episode's updated_at and the
// watermark query picks it up. A feed that is not subscribed will therefore
// gain transcripts in cast2md but no summaries here.
func (s *Service) TranscribeAllInFeed(ctx context.Context, userID int, feedID string) (*cast2md.BatchResult, error) {
	if s.cast2md == nil {
		return nil, ErrPodcastDisabled
	}
	numericFeedID, err := strconv.Atoi(feedID)
	if err != nil {
		return nil, fmt.Errorf("invalid feed ID %q", feedID)
	}

	result, err := s.cast2md.ProcessFeed(ctx, numericFeedID)
	if err != nil {
		return nil, fmt.Errorf("queue transcription in cast2md: %w", err)
	}

	sub, subErr := s.db.GetSubscription(ctx, userID, feedID)
	subscribed := subErr == nil && sub.Enabled
	s.log.Info("cast2md transcription queued for feed",
		"feed_id", feedID, "user_id", userID,
		"queued", result.Queued, "skipped", result.Skipped, "subscribed", subscribed)
	return result, nil
}

// BackfillFeed summarizes the N most recently completed episodes of a feed
// without moving the watermark, so a backfill and the running poll do not
// interfere.
func (s *Service) BackfillFeed(ctx context.Context, userID int, feedID string, limit int) (*BackfillResult, error) {
	if s.cast2md == nil {
		return nil, ErrPodcastDisabled
	}
	numericFeedID, err := strconv.Atoi(feedID)
	if err != nil {
		return nil, fmt.Errorf("invalid feed ID %q", feedID)
	}
	if limit <= 0 {
		limit = 10
	}

	level := s.summaryCfg.DefaultLevel
	if sub, err := s.db.GetSubscription(ctx, userID, feedID); err == nil {
		level = sub.DetailLevel
	}

	eps, err := s.cast2md.ListCompleted(ctx, cast2md.ListCompletedOptions{
		FeedID: numericFeedID,
		Order:  cast2md.OrderUpdatedDesc,
		Limit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list episodes for backfill: %w", err)
	}

	result := &BackfillResult{}
	for _, ep := range eps {
		externalID := strconv.Itoa(ep.ID)
		if existing, err := s.db.GetBySourceID(ctx, userID, storage.SourcePodcast, externalID); err == nil {
			if existing.Status == "completed" || existing.Status == "processing" {
				result.Skipped++
				continue
			}
		}
		if _, err := s.EnsureEpisodeRow(ctx, userID, ep.ID, level); err != nil {
			s.log.Warn("backfill row creation failed", "episode_id", ep.ID, "error", err)
			result.Skipped++
			continue
		}
		s.ProcessEpisodeAsync(userID, ep.ID, level)
		result.Queued++
	}

	s.log.Info("podcast backfill queued",
		"feed_id", feedID, "user_id", userID, "queued", result.Queued, "skipped", result.Skipped)
	return result, nil
}
