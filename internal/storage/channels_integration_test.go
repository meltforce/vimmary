package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meltforce/vimmary/internal/storage"
)

func TestChannelSubscriptionLifecycle(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	channelID := "UCtest_lifecycle_0000000"
	t.Cleanup(func() {
		_, _ = store.Pool.Exec(ctx,
			`DELETE FROM channel_subscriptions WHERE user_id = 1 AND channel_id = $1`, channelID)
	})

	sub, err := store.UpsertChannelSubscription(ctx, 1, channelID, "First Title", "https://img")
	if err != nil {
		t.Fatalf("UpsertChannelSubscription: %v", err)
	}
	if !sub.Enabled || sub.Title != "First Title" || sub.ThumbnailURL != "https://img" {
		t.Errorf("subscription = %+v", sub)
	}

	// Disable, then re-follow: the upsert revives and refreshes it.
	if err := store.SetChannelEnabled(ctx, 1, sub.ID, false); err != nil {
		t.Fatalf("SetChannelEnabled: %v", err)
	}
	revived, err := store.UpsertChannelSubscription(ctx, 1, channelID, "New Title", "")
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if revived.ID != sub.ID {
		t.Errorf("re-following created row %d, want the original %d", revived.ID, sub.ID)
	}
	if !revived.Enabled || revived.Title != "New Title" {
		t.Errorf("revived = %+v, want re-enabled with the fresh title", revived)
	}

	// Foreign-user access answers ErrNotFound.
	if _, err := store.GetChannelSubscription(ctx, 2, sub.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("foreign-user get err = %v, want ErrNotFound", err)
	}
	if err := store.SetChannelEnabled(ctx, 2, sub.ID, true); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("foreign-user enable err = %v, want ErrNotFound", err)
	}
	if err := store.DeleteChannelSubscription(ctx, 2, sub.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("foreign-user delete err = %v, want ErrNotFound", err)
	}

	// Error bookkeeping round trip.
	if err := store.SetChannelError(ctx, sub.ID, "status 404"); err != nil {
		t.Fatalf("SetChannelError: %v", err)
	}
	got, _ := store.GetChannelSubscription(ctx, 1, sub.ID)
	if got.LastError != "status 404" || got.LastPolledAt == nil {
		t.Errorf("after error: %+v", got)
	}
	if err := store.SetChannelPolled(ctx, sub.ID); err != nil {
		t.Fatalf("SetChannelPolled: %v", err)
	}
	got, _ = store.GetChannelSubscription(ctx, 1, sub.ID)
	if got.LastError != "" {
		t.Errorf("last_error = %q, want it cleared", got.LastError)
	}
}

func TestInboxItemsLifecycle(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()
	channelID := "UCtest_inbox_00000000000"
	t.Cleanup(func() {
		_, _ = store.Pool.Exec(ctx,
			`DELETE FROM channel_subscriptions WHERE user_id = 1 AND channel_id = $1`, channelID)
	})

	sub, err := store.UpsertChannelSubscription(ctx, 1, channelID, "Inbox Channel", "")
	if err != nil {
		t.Fatalf("UpsertChannelSubscription: %v", err)
	}

	older := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)
	first := &storage.InboxItem{SubscriptionID: sub.ID, UserID: 1, YouTubeID: "inbtest_1", Title: "Older", PublishedAt: &older}
	second := &storage.InboxItem{SubscriptionID: sub.ID, UserID: 1, YouTubeID: "inbtest_2", Title: "Newer", PublishedAt: &newer}

	for _, item := range []*storage.InboxItem{first, second} {
		fresh, err := store.InsertInboxItem(ctx, item)
		if err != nil {
			t.Fatalf("InsertInboxItem: %v", err)
		}
		if !fresh {
			t.Fatalf("insert of %s reported not fresh", item.YouTubeID)
		}
	}

	// The conflict is the dedup: same video again reports not fresh.
	fresh, err := store.InsertInboxItem(ctx, first)
	if err != nil {
		t.Fatalf("duplicate InsertInboxItem: %v", err)
	}
	if fresh {
		t.Error("duplicate insert reported fresh, want the conflict to swallow it")
	}

	items, total, err := store.ListInboxItems(ctx, 1, "", sub.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListInboxItems: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("list = %d/%d, want 2/2", len(items), total)
	}
	if items[0].YouTubeID != "inbtest_2" {
		t.Errorf("first item = %s, want the newest", items[0].YouTubeID)
	}
	if items[0].ChannelTitle != "Inbox Channel" {
		t.Errorf("channel title = %q", items[0].ChannelTitle)
	}

	// State transition removes the item from the new list.
	if err := store.SetInboxItemState(ctx, 1, items[0].ID, storage.InboxStateQueued); err != nil {
		t.Fatalf("SetInboxItemState: %v", err)
	}
	if err := store.SetInboxItemState(ctx, 2, items[1].ID, storage.InboxStateQueued); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("foreign-user state change err = %v, want ErrNotFound", err)
	}
	_, total, _ = store.ListInboxItems(ctx, 1, "", sub.ID, 0, 0)
	if total != 1 {
		t.Errorf("new list holds %d, want 1 after queuing one", total)
	}

	// DismissAll reports how many it touched and leaves queued rows alone.
	dismissed, err := store.DismissAllInbox(ctx, 1)
	if err != nil {
		t.Fatalf("DismissAllInbox: %v", err)
	}
	if dismissed != 1 {
		t.Errorf("dismissed = %d, want 1", dismissed)
	}

	// The new-count join on the subscription list.
	subs, err := store.ListChannelSubscriptions(ctx, 1)
	if err != nil {
		t.Fatalf("ListChannelSubscriptions: %v", err)
	}
	for _, s := range subs {
		if s.ID == sub.ID && s.NewCount != 0 {
			t.Errorf("new_count = %d, want 0 after triage", s.NewCount)
		}
	}

	// Unfollow cascades the items away.
	if err := store.DeleteChannelSubscription(ctx, 1, sub.ID); err != nil {
		t.Fatalf("DeleteChannelSubscription: %v", err)
	}
	var remaining int
	if err := store.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM inbox_items WHERE subscription_id = $1`, sub.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Errorf("%d inbox items survived the unfollow, want 0", remaining)
	}
}
