package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/vimmary/internal/storage"
)

// fakeAccounts is an in-memory accountSource.
type fakeAccounts struct {
	users    map[int]*storage.UserAccount
	settings map[string]string
	nextID   int
}

func newFakeAccounts(users ...storage.UserAccount) *fakeAccounts {
	f := &fakeAccounts{users: map[int]*storage.UserAccount{}, settings: map[string]string{}, nextID: 100}
	for i := range users {
		u := users[i]
		f.users[u.ID] = &u
	}
	return f
}

func (f *fakeAccounts) GetUserAccount(_ context.Context, id int) (storage.UserAccount, error) {
	u, ok := f.users[id]
	if !ok {
		return storage.UserAccount{}, storage.ErrNotFound
	}
	return *u, nil
}

func (f *fakeAccounts) ListUserAccounts(context.Context) ([]storage.UserAccount, error) {
	var out []storage.UserAccount
	for _, u := range f.users {
		out = append(out, *u)
	}
	return out, nil
}

func (f *fakeAccounts) GetUserIDByLogin(_ context.Context, login string) (int, error) {
	for _, u := range f.users {
		if u.Login == login {
			return u.ID, nil
		}
	}
	return 0, storage.ErrNotFound
}

func (f *fakeAccounts) GetUserFlags(_ context.Context, id int) (bool, bool, error) {
	u, ok := f.users[id]
	if !ok {
		return false, false, storage.ErrNotFound
	}
	return u.IsAdmin, u.Disabled(), nil
}

func (f *fakeAccounts) CreateUser(_ context.Context, login, name string, admin bool) (int, error) {
	if _, err := f.GetUserIDByLogin(context.Background(), login); err == nil {
		return 0, storage.ErrConflict
	}
	f.nextID++
	f.users[f.nextID] = &storage.UserAccount{ID: f.nextID, Login: login, DisplayName: name, IsAdmin: admin}
	return f.nextID, nil
}

func (f *fakeAccounts) AdminExists(context.Context) (bool, error) {
	for _, u := range f.users {
		if u.IsAdmin {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAccounts) PromoteFirstAdmin(ctx context.Context, id int) (bool, error) {
	if ok, _ := f.AdminExists(ctx); ok {
		return false, nil
	}
	f.users[id].IsAdmin = true
	return true, nil
}

func (f *fakeAccounts) SetUserAdmin(_ context.Context, id int, admin bool) error {
	f.users[id].IsAdmin = admin
	return nil
}

func (f *fakeAccounts) SetUserDisabled(_ context.Context, id int, disabled bool) error {
	u, ok := f.users[id]
	if !ok {
		return storage.ErrNotFound
	}
	if disabled {
		now := u.CreatedAt
		u.DisabledAt = &now
	} else {
		u.DisabledAt = nil
	}
	return nil
}

func (f *fakeAccounts) TouchUser(context.Context, int, string) error { return nil }

func (f *fakeAccounts) DeleteUser(_ context.Context, id int) error {
	delete(f.users, id)
	return nil
}

// GetPrimaryUser follows storage: the earliest login containing '@', here the
// lowest ID.
func (f *fakeAccounts) GetPrimaryUser(context.Context) (int, string, error) {
	best := 0
	for id, u := range f.users {
		if strings.Contains(u.Login, "@") && (best == 0 || id < best) {
			best = id
		}
	}
	if best == 0 {
		return 0, "", storage.ErrNotFound
	}
	return best, f.users[best].Login, nil
}

func (f *fakeAccounts) GetAppSetting(_ context.Context, k string) (string, error) {
	return f.settings[k], nil
}

func (f *fakeAccounts) SetAppSetting(_ context.Context, k, v string) error {
	f.settings[k] = v
	return nil
}

// fakeAuth is an in-memory Authenticator.
type fakeAuth struct {
	initialAdmin string
	passwords    map[int64]string // user ID → username
	identities   map[int64][]auth.IdentityLink
	revoked      []int64
	deleted      []int64
	tokens       []auth.Token
	nextToken    int64
}

func newFakeAuth() *fakeAuth {
	return &fakeAuth{passwords: map[int64]string{}, identities: map[int64][]auth.IdentityLink{}}
}

func (f *fakeAuth) IsInitialAdmin(id auth.Identity) bool {
	if f.initialAdmin == "" {
		return false
	}
	if id.Method == auth.MethodOIDC {
		return strings.EqualFold(id.Email, f.initialAdmin)
	}
	return strings.EqualFold(id.Subject, f.initialAdmin)
}

func (f *fakeAuth) CreatePasswordUser(_ context.Context, uid int64, username, _ string) error {
	for _, u := range f.passwords {
		if strings.EqualFold(u, username) {
			return auth.ErrExists
		}
	}
	f.passwords[uid] = username
	return nil
}

func (f *fakeAuth) SetPassword(context.Context, int64, string) error { return nil }
func (f *fakeAuth) ChangePassword(context.Context, int64, string, string) error {
	return nil
}

func (f *fakeAuth) PasswordUsername(_ context.Context, uid int64) (string, error) {
	return f.passwords[uid], nil
}

func (f *fakeAuth) ListSessions(context.Context, int64) ([]auth.Session, error) { return nil, nil }
func (f *fakeAuth) RevokeSession(context.Context, int64, int64) error           { return nil }

func (f *fakeAuth) RevokeUserSessions(_ context.Context, uid int64) error {
	f.revoked = append(f.revoked, uid)
	return nil
}

func (f *fakeAuth) ListIdentities(_ context.Context, uid int64) ([]auth.IdentityLink, error) {
	return f.identities[uid], nil
}

func (f *fakeAuth) UnlinkIdentity(_ context.Context, uid int64, issuer, subject string) error {
	ids := f.identities[uid]
	for i, l := range ids {
		if l.Issuer == issuer && l.Subject == subject {
			f.identities[uid] = append(ids[:i], ids[i+1:]...)
			return nil
		}
	}
	return auth.ErrNotFound
}

func (f *fakeAuth) DeleteUser(_ context.Context, uid int64) error {
	f.deleted = append(f.deleted, uid)
	return nil
}

func (f *fakeAuth) CreateToken(_ context.Context, uid int64, name string, scopes []string, expiresAt time.Time) (string, auth.Token, error) {
	if strings.TrimSpace(name) == "" || len(scopes) == 0 {
		return "", auth.Token{}, fmt.Errorf("%w: name and scope are required", auth.ErrInvalidTokenRequest)
	}
	f.nextToken++
	t := auth.Token{ID: f.nextToken, UserID: uid, Name: name, Scopes: scopes, CreatedAt: time.Now(), ExpiresAt: expiresAt}
	f.tokens = append(f.tokens, t)
	return fmt.Sprintf("vimmary_pat_%d", t.ID), t, nil
}

func (f *fakeAuth) ListTokens(_ context.Context, uid int64) ([]auth.Token, error) {
	var out []auth.Token
	for _, t := range f.tokens {
		if t.UserID == uid {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeAuth) RevokeToken(_ context.Context, uid, id int64) error {
	for i, t := range f.tokens {
		if t.ID == id && t.UserID == uid {
			f.tokens = append(f.tokens[:i], f.tokens[i+1:]...)
			return nil
		}
	}
	return auth.ErrNotFound
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func enabledAccounts(db *fakeAccounts, au *fakeAuth, methods ...string) *Accounts {
	a := NewAccounts(db, quietLog())
	a.Attach(au, methods)
	return a
}

const issuer = "https://idp.example.com"

func oidcLogin(email string) auth.Identity {
	return auth.Identity{Method: auth.MethodOIDC, Issuer: issuer, Subject: "sub-" + email, Email: email}
}

func TestResolveLinkedUser(t *testing.T) {
	ctx := context.Background()
	db := newFakeAccounts(storage.UserAccount{ID: 5, Login: "bob"})
	a := enabledAccounts(db, newFakeAuth(), auth.MethodPassword)

	uid, err := a.Resolve(ctx, auth.Identity{Method: auth.MethodPassword, Subject: "bob", UserID: 5})
	if err != nil || uid != 5 {
		t.Fatalf("Resolve = %d, %v; want 5", uid, err)
	}
	if _, err := a.Resolve(ctx, auth.Identity{Method: auth.MethodPassword, Subject: "ghost", UserID: 9}); !errors.Is(err, auth.ErrNoAccount) {
		t.Errorf("missing user: err = %v, want ErrNoAccount", err)
	}
}

func TestResolveDisabledUser(t *testing.T) {
	ctx := context.Background()
	db := newFakeAccounts(storage.UserAccount{ID: 5, Login: "bob"})
	a := enabledAccounts(db, newFakeAuth(), auth.MethodPassword, auth.MethodOIDC)
	if err := db.SetUserDisabled(ctx, 5, true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Resolve(ctx, auth.Identity{Method: auth.MethodPassword, Subject: "bob", UserID: 5}); !errors.Is(err, auth.ErrNoAccount) {
		t.Errorf("password login of disabled user: err = %v, want ErrNoAccount", err)
	}
	id := oidcLogin("bob@example.com")
	id.UserID = 5
	if _, err := a.Resolve(ctx, id); !errors.Is(err, auth.ErrNoAccount) {
		t.Errorf("linked OIDC login of disabled user: err = %v, want ErrNoAccount", err)
	}
	if disabled, _ := a.IsDisabled(ctx, 5); !disabled {
		t.Error("IsDisabled = false, want true")
	}
	if disabled, _ := a.IsDisabled(ctx, 404); !disabled {
		t.Error("IsDisabled for a missing user = false, want true")
	}
}

func TestResolveFirstOIDCLoginPolicy(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		policy  AccountPolicy
		email   string
		created bool
	}{
		{name: "closed", policy: AccountPolicy{Mode: PolicyClosed}, email: "new@example.com"},
		{name: "oidc_auto", policy: AccountPolicy{Mode: PolicyOIDCAuto}, email: "new@example.com", created: true},
		{name: "allowed domain", policy: AccountPolicy{Mode: PolicyOIDCAuto, EmailDomains: []string{"Example.com"}}, email: "new@EXAMPLE.com", created: true},
		{name: "other domain", policy: AccountPolicy{Mode: PolicyOIDCAuto, EmailDomains: []string{"example.com"}}, email: "new@evil.example.org"},
		{name: "suffix is not the domain", policy: AccountPolicy{Mode: PolicyOIDCAuto, EmailDomains: []string{"example.com"}}, email: "new@notexample.com"},
		{name: "no email", policy: AccountPolicy{Mode: PolicyOIDCAuto}, email: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newFakeAccounts(storage.UserAccount{ID: 1, Login: "admin@example.com", IsAdmin: true})
			a := enabledAccounts(db, newFakeAuth(), auth.MethodOIDC)
			if _, err := a.SetPolicy(ctx, tt.policy); err != nil {
				t.Fatal(err)
			}
			uid, err := a.Resolve(ctx, oidcLogin(tt.email))
			if !tt.created {
				if !errors.Is(err, auth.ErrNoAccount) {
					t.Fatalf("err = %v, want ErrNoAccount", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			u, _ := db.GetUserAccount(ctx, int(uid))
			if u.Login != tt.email || u.IsAdmin {
				t.Errorf("created %+v, want login %q, not admin", u, tt.email)
			}
		})
	}
}

func TestResolveDoesNotTakeOverExistingLogin(t *testing.T) {
	ctx := context.Background()
	// A user created by WhoIs, not linked: without link_by_email a first OIDC
	// login with the same email must not become that user.
	db := newFakeAccounts(storage.UserAccount{ID: 2, Login: "carol@example.com"})
	a := enabledAccounts(db, newFakeAuth(), auth.MethodOIDC)
	if _, err := a.SetPolicy(ctx, AccountPolicy{Mode: PolicyOIDCAuto}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Resolve(ctx, oidcLogin("carol@example.com")); !errors.Is(err, auth.ErrNoAccount) {
		t.Errorf("err = %v, want ErrNoAccount", err)
	}
}

func TestInitialAdminBootstrap(t *testing.T) {
	ctx := context.Background()

	t.Run("first OIDC login while no admin exists", func(t *testing.T) {
		db := newFakeAccounts()
		au := newFakeAuth()
		au.initialAdmin = "root@example.com"
		a := enabledAccounts(db, au, auth.MethodOIDC)
		// Accounts are closed, the initial admin gets one anyway.
		uid, err := a.Resolve(ctx, oidcLogin("root@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if ok, _ := a.IsAdmin(ctx, int(uid)); !ok {
			t.Error("initial admin is not admin")
		}
		// A second login of someone else stays refused.
		if _, err := a.Resolve(ctx, oidcLogin("other@example.com")); !errors.Is(err, auth.ErrNoAccount) {
			t.Errorf("other login: err = %v, want ErrNoAccount", err)
		}
	})

	t.Run("no bootstrap once an admin exists", func(t *testing.T) {
		db := newFakeAccounts(storage.UserAccount{ID: 1, Login: "owner@example.com", IsAdmin: true})
		au := newFakeAuth()
		au.initialAdmin = "root@example.com"
		a := enabledAccounts(db, au, auth.MethodOIDC)
		if _, err := a.Resolve(ctx, oidcLogin("root@example.com")); !errors.Is(err, auth.ErrNoAccount) {
			t.Errorf("err = %v, want ErrNoAccount", err)
		}
	})

	t.Run("existing password user is promoted", func(t *testing.T) {
		db := newFakeAccounts(storage.UserAccount{ID: 3, Login: "root"})
		au := newFakeAuth()
		au.initialAdmin = "root"
		a := enabledAccounts(db, au, auth.MethodPassword)
		if _, err := a.Resolve(ctx, auth.Identity{Method: auth.MethodPassword, Subject: "root", UserID: 3}); err != nil {
			t.Fatal(err)
		}
		if ok, _ := a.IsAdmin(ctx, 3); !ok {
			t.Error("initial admin password user is not admin")
		}
	})

	t.Run("first visitor never becomes admin", func(t *testing.T) {
		db := newFakeAccounts()
		a := enabledAccounts(db, newFakeAuth(), auth.MethodOIDC)
		if _, err := a.SetPolicy(ctx, AccountPolicy{Mode: PolicyOIDCAuto}); err != nil {
			t.Fatal(err)
		}
		uid, err := a.Resolve(ctx, oidcLogin("first@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if ok, _ := a.IsAdmin(ctx, int(uid)); ok {
			t.Error("first visitor became admin")
		}
	})
}

func TestIsAdmin(t *testing.T) {
	ctx := context.Background()

	t.Run("login configured: only the flag counts", func(t *testing.T) {
		// No user at all: before the flag, everyone was admin here.
		a := enabledAccounts(newFakeAccounts(), newFakeAuth(), auth.MethodPassword)
		if ok, _ := a.IsAdmin(ctx, 1); ok {
			t.Error("IsAdmin with no users = true, want false")
		}
		db := newFakeAccounts(storage.UserAccount{ID: 1, Login: "local"}, storage.UserAccount{ID: 2, Login: "a@example.com"})
		a = enabledAccounts(db, newFakeAuth(), auth.MethodPassword)
		if ok, _ := a.IsAdmin(ctx, 2); ok {
			t.Error("primary user without flag is admin")
		}
	})

	t.Run("disabled admin is not admin", func(t *testing.T) {
		db := newFakeAccounts(storage.UserAccount{ID: 2, Login: "a@example.com", IsAdmin: true})
		a := enabledAccounts(db, newFakeAuth(), auth.MethodPassword)
		_ = db.SetUserDisabled(ctx, 2, true)
		if ok, _ := a.IsAdmin(ctx, 2); ok {
			t.Error("disabled admin is admin")
		}
	})

	t.Run("without login: the primary-user rule", func(t *testing.T) {
		// Only the seeded local user: everyone is admin, as before.
		a := NewAccounts(newFakeAccounts(storage.UserAccount{ID: 1, Login: "local"}), quietLog())
		if ok, _ := a.IsAdmin(ctx, 1); !ok {
			t.Error("local run: IsAdmin = false, want true")
		}
		// A primary user without the flag (created after the migration) is
		// admin and gets the flag; others are not admin.
		db := newFakeAccounts(storage.UserAccount{ID: 1, Login: "local"},
			storage.UserAccount{ID: 2, Login: "a@example.com"}, storage.UserAccount{ID: 3, Login: "b@example.com"})
		a = NewAccounts(db, quietLog())
		if ok, _ := a.IsAdmin(ctx, 3); ok {
			t.Error("non-primary user is admin")
		}
		if ok, _ := a.IsAdmin(ctx, 2); !ok {
			t.Error("primary user is not admin")
		}
		if !db.users[2].IsAdmin {
			t.Error("primary user was not recorded as admin")
		}
	})
}

func TestAdminActions(t *testing.T) {
	ctx := context.Background()
	newSetup := func() (*fakeAccounts, *fakeAuth, *Accounts) {
		db := newFakeAccounts(
			storage.UserAccount{ID: 1, Login: "admin", IsAdmin: true},
			storage.UserAccount{ID: 2, Login: "member", Items: 3},
			storage.UserAccount{ID: 3, Login: "empty"},
		)
		au := newFakeAuth()
		return db, au, enabledAccounts(db, au, auth.MethodPassword)
	}

	t.Run("disable ends sessions and tokens, not on self", func(t *testing.T) {
		db, au, a := newSetup()
		for _, uid := range []int{1, 2, 2} {
			if _, err := a.CreateToken(ctx, uid, "client", []string{auth.ScopeMCP}, time.Time{}); err != nil {
				t.Fatal(err)
			}
		}
		if err := a.SetUserDisabled(ctx, 1, 1, true); !errors.Is(err, ErrAccountSelf) {
			t.Errorf("self disable: err = %v", err)
		}
		if err := a.SetUserDisabled(ctx, 1, 2, true); err != nil {
			t.Fatal(err)
		}
		if !db.users[2].Disabled() || len(au.revoked) != 1 || au.revoked[0] != 2 {
			t.Errorf("disabled=%v revoked=%v", db.users[2].Disabled(), au.revoked)
		}
		if len(au.tokens) != 1 || au.tokens[0].UserID != 1 {
			t.Errorf("tokens after disabling user 2: %+v, want only user 1's", au.tokens)
		}
		if _, err := a.CreateToken(ctx, 2, "again", []string{auth.ScopeMCP}, time.Time{}); !errors.Is(err, ErrAccountInvalid) {
			t.Errorf("token for disabled user: err = %v, want ErrAccountInvalid", err)
		}
	})

	t.Run("delete only without library items", func(t *testing.T) {
		db, au, a := newSetup()
		if err := a.DeleteUser(ctx, 1, 2); !errors.Is(err, ErrAccountHasItems) {
			t.Errorf("delete with items: err = %v", err)
		}
		if err := a.DeleteUser(ctx, 1, 1); !errors.Is(err, ErrAccountSelf) {
			t.Errorf("delete self: err = %v", err)
		}
		if err := a.DeleteUser(ctx, 3, 1); !errors.Is(err, ErrAccountBuiltIn) {
			t.Errorf("delete user 1: err = %v", err)
		}
		if err := a.DeleteUser(ctx, 1, 3); err != nil {
			t.Fatal(err)
		}
		if _, ok := db.users[3]; ok || len(au.deleted) != 1 {
			t.Errorf("user 3 still present or auth rows kept: %v", au.deleted)
		}
	})

	t.Run("create password user", func(t *testing.T) {
		db, au, a := newSetup()
		if _, err := a.CreatePasswordUser(ctx, "dave", "short", false); !errors.Is(err, ErrAccountInvalid) {
			t.Errorf("short password: err = %v", err)
		}
		if _, err := a.CreatePasswordUser(ctx, "member", "long enough password", false); !errors.Is(err, ErrAccountConflict) {
			t.Errorf("taken login: err = %v", err)
		}
		v, err := a.CreatePasswordUser(ctx, "dave", "long enough password", true)
		if err != nil {
			t.Fatal(err)
		}
		if !db.users[v.ID].IsAdmin || au.passwords[int64(v.ID)] != "dave" || v.PasswordUsername != "dave" {
			t.Errorf("created %+v", v)
		}
	})

	t.Run("revoke own admin refused", func(t *testing.T) {
		_, _, a := newSetup()
		if err := a.SetUserAdmin(ctx, 1, 1, false); !errors.Is(err, ErrAccountSelf) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestEnsurePasswordUser(t *testing.T) {
	ctx := context.Background()
	// A WhoIs-created user gets a password login under its login.
	db := newFakeAccounts(storage.UserAccount{ID: 4, Login: "erin@example.com"})
	au := newFakeAuth()
	a := enabledAccounts(db, au, auth.MethodPassword)
	uid, created, err := a.EnsurePasswordUser(ctx, "erin@example.com", "long enough password", true)
	if err != nil || created || uid != 4 {
		t.Fatalf("EnsurePasswordUser = %d, %v, %v", uid, created, err)
	}
	if au.passwords[4] != "erin@example.com" || !db.users[4].IsAdmin {
		t.Errorf("password %q admin %v", au.passwords[4], db.users[4].IsAdmin)
	}
	if _, _, err := a.EnsurePasswordUser(ctx, "erin@example.com", "long enough password", false); !errors.Is(err, ErrAccountConflict) {
		t.Errorf("second call: err = %v, want ErrAccountConflict", err)
	}
}

func TestUnlinkKeepsALogin(t *testing.T) {
	ctx := context.Background()
	link := func(sub string) auth.IdentityLink { return auth.IdentityLink{Issuer: issuer, Subject: sub, UserID: 5} }

	db := newFakeAccounts(storage.UserAccount{ID: 5, Login: "f@example.com"})
	au := newFakeAuth()
	au.identities[5] = []auth.IdentityLink{link("a"), link("b")}
	a := enabledAccounts(db, au, auth.MethodOIDC, auth.MethodPassword)

	if err := a.UnlinkIdentity(ctx, 5, issuer, "a"); err != nil {
		t.Fatalf("unlink with another identity left: %v", err)
	}
	if err := a.UnlinkIdentity(ctx, 5, issuer, "b"); !errors.Is(err, ErrAccountLastLogin) {
		t.Errorf("unlink last identity: err = %v, want ErrAccountLastLogin", err)
	}
	au.passwords[5] = "f"
	if err := a.UnlinkIdentity(ctx, 5, issuer, "b"); err != nil {
		t.Errorf("unlink with a password left: %v", err)
	}
	if err := a.UnlinkIdentity(ctx, 5, issuer, "zzz"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("unknown identity: err = %v, want ErrNotFound", err)
	}

	// A password does not count when the password method is off.
	au.identities[5] = []auth.IdentityLink{link("c")}
	a = enabledAccounts(db, au, auth.MethodOIDC)
	if err := a.UnlinkIdentity(ctx, 5, issuer, "c"); !errors.Is(err, ErrAccountLastLogin) {
		t.Errorf("password method off: err = %v, want ErrAccountLastLogin", err)
	}
}

func TestSetPolicyValidates(t *testing.T) {
	ctx := context.Background()
	a := enabledAccounts(newFakeAccounts(), newFakeAuth(), auth.MethodOIDC)
	if _, err := a.SetPolicy(ctx, AccountPolicy{Mode: "open"}); !errors.Is(err, ErrAccountInvalid) {
		t.Errorf("bad mode: err = %v", err)
	}
	if _, err := a.SetPolicy(ctx, AccountPolicy{Mode: PolicyOIDCAuto, EmailDomains: []string{"a@b.com"}}); !errors.Is(err, ErrAccountInvalid) {
		t.Errorf("bad domain: err = %v", err)
	}
	p, err := a.SetPolicy(ctx, AccountPolicy{Mode: PolicyOIDCAuto, EmailDomains: []string{" @Example.com ", "example.com", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.EmailDomains) != 1 || p.EmailDomains[0] != "example.com" {
		t.Errorf("domains = %v", p.EmailDomains)
	}
	got, _ := a.Policy(ctx)
	if got.Mode != PolicyOIDCAuto || len(got.EmailDomains) != 1 {
		t.Errorf("Policy = %+v", got)
	}
}

func TestTokens(t *testing.T) {
	ctx := context.Background()
	db := newFakeAccounts(
		storage.UserAccount{ID: 1, Login: "local"},
		storage.UserAccount{ID: 2, Login: "admin@example.com", IsAdmin: true},
		storage.UserAccount{ID: 3, Login: "bob"},
	)
	au := newFakeAuth()
	a := enabledAccounts(db, au, auth.MethodOIDC)

	tok, err := a.CreateTokenForLogin(ctx, "admin@example.com", "tsmcp", []string{auth.ScopeMCP}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Secret == "" || tok.UserID != 2 || tok.Expired {
		t.Errorf("CreateTokenForLogin = %+v", tok)
	}
	if _, err := a.CreateTokenForLogin(ctx, "ghost", "x", []string{auth.ScopeMCP}, time.Time{}); !errors.Is(err, ErrAccountInvalid) {
		t.Errorf("unknown login: err = %v, want ErrAccountInvalid", err)
	}
	if _, err := a.CreateToken(ctx, 2, " ", []string{auth.ScopeMCP}, time.Time{}); !errors.Is(err, ErrAccountInvalid) {
		t.Errorf("empty name: err = %v, want ErrAccountInvalid", err)
	}
	if _, err := a.CreateToken(ctx, 3, "bob's", []string{auth.ScopeAPI}, time.Time{}); err != nil {
		t.Fatal(err)
	}

	list, err := a.ListTokens(ctx, 2)
	if err != nil || len(list) != 1 || list[0].Name != "tsmcp" {
		t.Fatalf("ListTokens(2) = %+v, %v", list, err)
	}
	// Another user's token is not found, and stays.
	if err := a.RevokeToken(ctx, 3, tok.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("revoke other user's token: err = %v, want ErrNotFound", err)
	}
	if err := a.RevokeToken(ctx, 2, tok.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := a.ListTokens(ctx, 2); len(list) != 0 {
		t.Errorf("tokens after revoke: %+v", list)
	}

	off := NewAccounts(db, quietLog())
	if _, err := off.ListTokens(ctx, 2); !errors.Is(err, ErrAuthDisabled) {
		t.Errorf("without login: err = %v, want ErrAuthDisabled", err)
	}
}
