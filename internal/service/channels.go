package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/youtube"
)

// channelFeedSource is the part of the YouTube client the channel inbox uses —
// a seam in the style of settingsSource, so the poller and subscribe path are
// testable without the network. *youtube.Client satisfies it.
type channelFeedSource interface {
	ResolveChannel(ctx context.Context, input string) (*youtube.ChannelInfo, error)
	ResolveVideoChannel(ctx context.Context, videoID string) (*youtube.ChannelInfo, error)
	FetchChannelFeed(ctx context.Context, channelID string) ([]youtube.FeedEntry, error)
	IsShort(ctx context.Context, videoID string) (bool, error)
}

// SubscribeChannel resolves the pasted URL or handle, stores the subscription
// and runs its first poll in place, so the inbox fills before the response
// returns. The poll is best effort: the subscription is committed first, and a
// failure lands in last_error for the ticker to retry.
func (s *Service) SubscribeChannel(ctx context.Context, userID int, input string) (*storage.ChannelSubscription, error) {
	info, err := s.channels.ResolveChannel(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("resolve channel: %w", err)
	}

	title := info.Title
	if title == "" {
		title = info.ID
	}
	sub, err := s.db.UpsertChannelSubscription(ctx, userID, info.ID, title, info.ThumbnailURL)
	if err != nil {
		return nil, err
	}

	if err := s.pollChannel(ctx, *sub); err != nil {
		s.log.Warn("initial channel poll failed",
			"channel_id", sub.ChannelID, "user_id", userID, "error", err)
		if dbErr := s.db.SetChannelError(ctx, sub.ID, err.Error()); dbErr != nil {
			s.log.Error("failed to record channel poll error", "channel_id", sub.ChannelID, "error", dbErr)
		}
	} else if err := s.db.SetChannelPolled(ctx, sub.ID); err != nil {
		s.log.Error("failed to record channel poll", "channel_id", sub.ChannelID, "error", err)
	}

	return s.db.GetChannelSubscription(ctx, userID, sub.ID)
}

// ChannelImportResult reports what a Takeout import did.
type ChannelImportResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
}

// ImportChannels follows every channel in a Google Takeout subscriptions.csv.
// The CSV already carries ID and title, so no channel pages are fetched; the
// inboxes fill through one poll pass started in the background — running it
// inside the request would hold the response for two seconds per channel.
func (s *Service) ImportChannels(ctx context.Context, userID int, csvData string) (*ChannelImportResult, error) {
	channels, skipped, err := youtube.ParseTakeoutSubscriptions(csvData)
	if err != nil {
		return nil, err
	}

	result := &ChannelImportResult{Skipped: skipped}
	for _, ch := range channels {
		title := ch.Title
		if title == "" {
			title = ch.ID
		}
		if _, err := s.db.UpsertChannelSubscription(ctx, userID, ch.ID, title, ""); err != nil {
			s.log.Warn("channel import row failed", "channel_id", ch.ID, "error", err)
			result.Skipped++
			continue
		}
		result.Imported++
	}

	go s.pollChannelsOnce(context.Background())

	s.log.Info("channels imported from Takeout",
		"user_id", userID, "imported", result.Imported, "skipped", result.Skipped)
	return result, nil
}

// ListChannels returns the user's followed channels with their new-item counts.
func (s *Service) ListChannels(ctx context.Context, userID int) ([]storage.ChannelSubscription, error) {
	return s.db.ListChannelSubscriptions(ctx, userID)
}

// SetChannelEnabled pauses or resumes one subscription.
func (s *Service) SetChannelEnabled(ctx context.Context, userID, id int, enabled bool) error {
	return s.db.SetChannelEnabled(ctx, userID, id, enabled)
}

// DeleteChannel unfollows a channel; its inbox items cascade away with it.
func (s *Service) DeleteChannel(ctx context.Context, userID, id int) error {
	return s.db.DeleteChannelSubscription(ctx, userID, id)
}

// ListInbox returns one page of a user's inbox.
func (s *Service) ListInbox(ctx context.Context, userID int, state string, subscriptionID, limit, offset int) ([]storage.InboxItem, int, error) {
	return s.db.ListInboxItems(ctx, userID, state, subscriptionID, limit, offset)
}

// SummarizeInboxItem sends an inbox video through the ordinary pipeline.
// "Watch" and "Summarize" in the UI are both this call — watching just
// navigates to the returned row, whose detail page carries the player.
//
// The row is created synchronously rather than inside the queued job because
// the caller needs its UUID to navigate; ProcessVideo's existing-row branch
// then adopts it. EnsureVideoRow makes a second click converge on the same row.
func (s *Service) SummarizeInboxItem(ctx context.Context, userID, itemID int) (*storage.Video, error) {
	item, err := s.db.GetInboxItem(ctx, userID, itemID)
	if err != nil {
		return nil, err
	}

	video, err := s.db.EnsureVideoRow(ctx, &storage.Video{
		ID:           uuid.New(),
		UserID:       userID,
		YouTubeID:    item.YouTubeID,
		ThumbnailURL: youtube.ThumbnailURL(item.YouTubeID),
		PublishedAt:  item.PublishedAt,
		Title:        item.Title,
		Channel:      item.ChannelTitle,
		DetailLevel:  s.summaryCfg.DefaultLevel,
		Status:       "pending",
	})
	if err != nil {
		return nil, fmt.Errorf("ensure video row: %w", err)
	}

	if video.Status != "completed" && video.Status != "processing" {
		s.ProcessVideoAsync(userID, item.YouTubeID, "")
	}

	if err := s.db.SetInboxItemState(ctx, userID, itemID, storage.InboxStateQueued); err != nil {
		s.log.Warn("failed to mark inbox item queued", "item_id", itemID, "error", err)
	}
	return video, nil
}

// DismissInboxItem removes one item from triage. The row stays as dedup memory.
func (s *Service) DismissInboxItem(ctx context.Context, userID, itemID int) error {
	return s.db.SetInboxItemState(ctx, userID, itemID, storage.InboxStateDismissed)
}

// DismissAllInbox clears the whole triage list and returns how many it touched.
func (s *Service) DismissAllInbox(ctx context.Context, userID int) (int, error) {
	return s.db.DismissAllInbox(ctx, userID)
}
