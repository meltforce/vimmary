package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/meltforce/vimmary/internal/storage"
)

// channelPollInterval is fixed rather than configured: the per-channel RSS
// feed is a public CDN endpoint, so unlike the cast2md interval there is no
// deployment-specific load to tune for. 30 minutes is fast enough for an
// inbox and polite enough for a feed that updates a few times a week.
const channelPollInterval = 30 * time.Minute

// channelPollGap spaces the per-channel fetches inside one cycle.
const channelPollGap = 2 * time.Second

// StartChannelPoller runs the channel RSS poll loop until ctx is cancelled.
// Unlike the podcast poller it has no configuration gate — channels need no
// external service.
func (s *Service) StartChannelPoller(ctx context.Context) {
	s.log.Info("channel poller starting", "interval", channelPollInterval)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollStartDelay):
		}

		ticker := time.NewTicker(channelPollInterval)
		defer ticker.Stop()

		for {
			s.pollChannelsOnce(ctx)
			s.probeUnprobedInboxItems(ctx)
			s.backfillLibraryChannelArt(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// shortsProbePerCycle caps how many already-stored inbox items are probed per
// cycle — one request each, so a large untouched inbox is worked off over
// several cycles instead of in one burst.
const shortsProbePerCycle = 25

// probeUnprobedInboxItems dismisses Shorts among items that reached the inbox
// without a conclusive probe: rows inserted before the probe existed, and rows
// whose probe failed and were let through on purpose.
//
// Running it here rather than inside pollChannel is what makes the fail-open
// decision temporary. pollChannel probes a video once, on first sight, and
// never looks at the row again; this pass is the second look, and it keeps
// looking until the probe answers conclusively.
func (s *Service) probeUnprobedInboxItems(ctx context.Context) {
	items, err := s.db.ListUnprobedInboxItems(ctx, shortsProbePerCycle)
	if err != nil {
		s.log.Error("failed to list unprobed inbox items", "error", err)
		return
	}

	dismissed := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		short, err := s.channels.IsShort(ctx, item.YouTubeID)
		if err != nil {
			// Leave shorts_checked_at NULL so the next cycle retries.
			s.log.Warn("shorts probe failed, will retry",
				"video_id", item.YouTubeID, "error", err)
			continue
		}
		if err := s.db.MarkInboxItemProbed(ctx, item.ID); err != nil {
			s.log.Warn("failed to mark inbox item probed", "id", item.ID, "error", err)
		}
		if !short {
			continue
		}
		if err := s.db.SetInboxItemState(ctx, item.UserID, item.ID, storage.InboxStateDismissed); err != nil {
			s.log.Warn("failed to dismiss short", "video_id", item.YouTubeID, "error", err)
			continue
		}
		dismissed++
	}

	if dismissed > 0 {
		s.log.Info("shorts dismissed from the inbox", "count", dismissed, "probed", len(items))
	}
}

// artBackfillPerCycle caps how many unfollowed channels get their avatar
// resolved per poll cycle — two page fetches each, so the cap keeps a large
// Karakeep history from turning one cycle into a scrape run.
const artBackfillPerCycle = 20

// pollChannelsOnce advances every enabled subscription. The art backfill runs
// as its own step in the ticker loop, not here.
func (s *Service) pollChannelsOnce(ctx context.Context) {
	subs, err := s.db.ListEnabledChannelSubscriptions(ctx)
	if err != nil {
		s.log.Error("failed to list channel subscriptions", "error", err)
		return
	}

	for i, sub := range subs {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(channelPollGap):
			}
		}
		// The Takeout import creates subscriptions from the CSV alone — no
		// avatar. Fetch the channel page once here; after the first success
		// the column is filled and this branch never runs again.
		if sub.ThumbnailURL == "" {
			s.backfillChannelIdentity(ctx, sub)
		}
		if err := s.pollChannel(ctx, sub); err != nil {
			s.log.Warn("channel poll failed",
				"channel_id", sub.ChannelID, "user_id", sub.UserID, "error", err)
			if dbErr := s.db.SetChannelError(ctx, sub.ID, err.Error()); dbErr != nil {
				s.log.Error("failed to record channel poll error", "channel_id", sub.ChannelID, "error", dbErr)
			}
			continue
		}
		if err := s.db.SetChannelPolled(ctx, sub.ID); err != nil {
			s.log.Error("failed to record channel poll", "channel_id", sub.ChannelID, "error", err)
		}
	}
}

// backfillLibraryChannelArt resolves avatars for channels that exist only
// through their videos — Karakeep imports, manual submissions — where the
// channel name is all the rows carry. One of the channel's videos leads to
// the channel page; the result lands in the channel_art cache, a failure as
// a NULL row so the retry waits out the cool-down.
func (s *Service) backfillLibraryChannelArt(ctx context.Context) {
	missing, err := s.db.ListChannelsMissingArt(ctx, artBackfillPerCycle)
	if err != nil {
		s.log.Error("failed to list channels missing art", "error", err)
		return
	}

	for i, m := range missing {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(channelPollGap):
			}
		}
		info, err := s.channels.ResolveVideoChannel(ctx, m.YouTubeID)
		art := ""
		if err != nil {
			s.log.Warn("channel art resolution failed",
				"channel", m.Channel, "video_id", m.YouTubeID, "error", err)
		} else {
			art = info.ThumbnailURL
		}
		if err := s.db.UpsertChannelArt(ctx, m.UserID, m.Channel, art); err != nil {
			s.log.Warn("failed to store channel art", "channel", m.Channel, "error", err)
		}
	}
	if len(missing) > 0 {
		s.log.Info("channel art backfill pass finished", "channels", len(missing))
	}
}

// backfillChannelIdentity resolves a subscription's avatar and current title
// from its channel page. Best effort: a failed fetch leaves the column NULL
// and the next cycle retries.
func (s *Service) backfillChannelIdentity(ctx context.Context, sub storage.ChannelSubscription) {
	info, err := s.channels.ResolveChannel(ctx, "https://www.youtube.com/channel/"+sub.ChannelID)
	if err != nil || info.ThumbnailURL == "" {
		s.log.Warn("channel identity backfill failed",
			"channel_id", sub.ChannelID, "error", err)
		return
	}
	if err := s.db.UpdateChannelIdentity(ctx, sub.ID, info.Title, info.ThumbnailURL); err != nil {
		s.log.Warn("failed to store channel identity", "channel_id", sub.ChannelID, "error", err)
		return
	}
	s.log.Info("channel identity backfilled", "channel_id", sub.ChannelID, "title", info.Title)
}

// probeShort wraps the probe so a transient failure lets the video into the
// inbox instead of losing it — a Short that slips through costs one dismiss,
// a real video filtered by a flaky probe would just be gone. The second return
// says whether the answer was conclusive; an inconclusive one leaves
// shorts_checked_at NULL, and probeUnprobedInboxItems retries the row on a
// later cycle rather than letting the fail-open answer stand forever.
func (s *Service) probeShort(ctx context.Context, videoID string) (short, probed bool) {
	short, err := s.channels.IsShort(ctx, videoID)
	if err != nil {
		s.log.Warn("shorts probe failed, keeping video", "video_id", videoID, "error", err)
		return false, false
	}
	return short, true
}

// pollChannel reads one channel's feed and inserts what is new. There is no
// watermark: the (user, video) unique index is the seen-set, so the first poll
// after subscribing and every later poll are the same operation.
func (s *Service) pollChannel(ctx context.Context, sub storage.ChannelSubscription) error {
	entries, err := s.channels.FetchChannelFeed(ctx, sub.ChannelID)
	if err != nil {
		return err
	}

	inserted := 0
	for _, entry := range entries {
		// The cheap Shorts signal first: a #shorts title skips the entry
		// without a row and without a probe.
		if strings.Contains(strings.ToLower(entry.Title), "#shorts") {
			continue
		}

		// A video already in the library — summarized via Karakeep, manual
		// submission or an earlier inbox action — is not new to triage.
		if _, err := s.db.GetByYouTubeID(ctx, sub.UserID, entry.VideoID); err == nil {
			continue
		} else if !errors.Is(err, storage.ErrNotFound) {
			return err
		}

		published := entry.Published
		item := &storage.InboxItem{
			SubscriptionID: sub.ID,
			UserID:         sub.UserID,
			YouTubeID:      entry.VideoID,
			Title:          entry.Title,
		}
		if !published.IsZero() {
			item.PublishedAt = &published
		}
		fresh, err := s.db.InsertInboxItem(ctx, item)
		if err != nil {
			return err
		}
		if !fresh {
			continue
		}

		// Most Shorts carry no title tag, so every video is probed once, on
		// first sight — the row already exists, which is what limits the
		// probe to one request per video ever. A Short is dismissed rather
		// than deleted: the row is the dedup memory that keeps it out.
		short, probed := s.probeShort(ctx, entry.VideoID)
		if probed {
			// A conclusive answer either way, so probeUnprobedInboxItems does
			// not look at this row again.
			if err := s.db.MarkInboxItemProbed(ctx, item.ID); err != nil {
				s.log.Warn("failed to mark inbox item probed", "id", item.ID, "error", err)
			}
		}
		if short {
			if err := s.db.SetInboxItemState(ctx, sub.UserID, item.ID, storage.InboxStateDismissed); err != nil {
				s.log.Warn("failed to dismiss short", "video_id", entry.VideoID, "error", err)
			}
			continue
		}
		inserted++
	}

	if inserted > 0 {
		s.log.Info("inbox items added",
			"channel_id", sub.ChannelID, "user_id", sub.UserID, "count", inserted)
	}
	return nil
}
