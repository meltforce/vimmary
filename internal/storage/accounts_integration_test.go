package storage_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/vimmary/internal/storage"
)

func TestUserAccounts(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// The auth tables migrate into the same database beside vimmary's own.
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultDSN
	}
	if err := auth.RunMigrations(dsn); err != nil {
		t.Fatalf("auth migrations: %v", err)
	}

	suffix := uuid.NewString()[:8]
	adminLogin, memberLogin := "admin-"+suffix, "member-"+suffix
	var created []int
	t.Cleanup(func() {
		for _, id := range created {
			_, _ = store.Pool.Exec(ctx, `DELETE FROM videos WHERE user_id = $1`, id)
			_, _ = store.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
		}
	})

	adminID, err := store.CreateUser(ctx, adminLogin, "Admin", true)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	created = append(created, adminID)
	memberID, err := store.CreateUser(ctx, memberLogin, "", false)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	created = append(created, memberID)

	if _, err := store.CreateUser(ctx, memberLogin, "", false); !errors.Is(err, storage.ErrConflict) {
		t.Errorf("duplicate login: err = %v, want ErrConflict", err)
	}
	if id, err := store.GetUserIDByLogin(ctx, memberLogin); err != nil || id != memberID {
		t.Errorf("GetUserIDByLogin = %d, %v", id, err)
	}
	if _, err := store.GetUserIDByLogin(ctx, "nobody-"+suffix); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("unknown login: err = %v, want ErrNotFound", err)
	}

	// An admin exists, so nobody else is promoted.
	if ok, err := store.PromoteFirstAdmin(ctx, memberID); err != nil || ok {
		t.Errorf("PromoteFirstAdmin = %v, %v; want false", ok, err)
	}

	if err := store.SetUserDisabled(ctx, memberID, true); err != nil {
		t.Fatal(err)
	}
	isAdmin, disabled, err := store.GetUserFlags(ctx, memberID)
	if err != nil || isAdmin || !disabled {
		t.Errorf("flags = %v, %v, %v; want not admin, disabled", isAdmin, disabled, err)
	}
	if err := store.SetUserDisabled(ctx, memberID, false); err != nil {
		t.Fatal(err)
	}
	if _, disabled, _ := store.GetUserFlags(ctx, memberID); disabled {
		t.Error("still disabled after re-enabling")
	}
	if err := store.SetUserDisabled(ctx, 1<<30, true); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("unknown user: err = %v, want ErrNotFound", err)
	}

	// A user with a library row cannot be deleted; without one it can.
	video := &storage.Video{ID: uuid.New(), UserID: memberID, YouTubeID: "acct_" + suffix, DetailLevel: "medium", Status: "processing"}
	if err := store.InsertVideo(ctx, video); err != nil {
		t.Fatalf("InsertVideo: %v", err)
	}
	acct, err := store.GetUserAccount(ctx, memberID)
	if err != nil || acct.Items != 1 {
		t.Errorf("GetUserAccount = %+v, %v; want 1 item", acct, err)
	}
	if err := store.DeleteUser(ctx, memberID); !errors.Is(err, storage.ErrConflict) {
		t.Errorf("delete with items: err = %v, want ErrConflict", err)
	}
	if err := store.DeleteVideo(ctx, memberID, video.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser(ctx, memberID); err != nil {
		t.Errorf("delete without items: %v", err)
	}
	if _, err := store.GetUserAccount(ctx, memberID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("deleted user: err = %v, want ErrNotFound", err)
	}

	users, err := store.ListUserAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range users {
		if u.ID == adminID {
			found = u.IsAdmin && u.Login == adminLogin && u.DisplayName == "Admin"
		}
	}
	if !found {
		t.Error("ListUserAccounts misses the admin")
	}
}
