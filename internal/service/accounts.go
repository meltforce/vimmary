package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/vimmary/internal/storage"
)

// Account policies: who gets an account on a first OIDC login.
const (
	PolicyClosed   = "closed"    // only the admin creates accounts
	PolicyOIDCAuto = "oidc_auto" // a first OIDC login creates one
)

// Errors of account management. The REST layer maps them to status codes.
var (
	// ErrAccountInvalid wraps a rejected input; its message is for the user.
	ErrAccountInvalid = errors.New("invalid")
	// ErrAccountConflict: the login or username is taken.
	ErrAccountConflict = errors.New("login already taken")
	// ErrAccountSelf: an admin action an admin may not apply to themselves.
	ErrAccountSelf = errors.New("not allowed on your own account")
	// ErrAccountBuiltIn: user 1, the seeded `local` user, which MCP stdio mode
	// and DevIdentity act as.
	ErrAccountBuiltIn = errors.New("the built-in local user cannot be deleted")
	// ErrAccountHasItems: the user owns library rows and can only be disabled.
	ErrAccountHasItems = errors.New("user has library items; disable the account instead")
	// ErrAccountLastLogin: removing this login would leave no way to sign in.
	ErrAccountLastLogin = errors.New("this is your only way to sign in")
	// ErrAccountRateLimited: too many wrong current passwords.
	ErrAccountRateLimited = errors.New("too many failed attempts; try again later")
	// ErrAuthDisabled: the instance runs without an auth block.
	ErrAuthDisabled = errors.New("login is not configured")
)

// accountSource is the part of storage account management reads and writes.
type accountSource interface {
	GetUserAccount(ctx context.Context, id int) (storage.UserAccount, error)
	ListUserAccounts(ctx context.Context) ([]storage.UserAccount, error)
	GetUserIDByLogin(ctx context.Context, login string) (int, error)
	GetUserFlags(ctx context.Context, id int) (isAdmin, disabled bool, err error)
	CreateUser(ctx context.Context, login, displayName string, isAdmin bool) (int, error)
	AdminExists(ctx context.Context) (bool, error)
	PromoteFirstAdmin(ctx context.Context, id int) (bool, error)
	SetUserAdmin(ctx context.Context, id int, admin bool) error
	SetUserDisabled(ctx context.Context, id int, disabled bool) error
	TouchUser(ctx context.Context, id int, displayName string) error
	DeleteUser(ctx context.Context, id int) error
	GetPrimaryUser(ctx context.Context) (int, string, error)
	GetAppSetting(ctx context.Context, key string) (string, error)
	SetAppSetting(ctx context.Context, key, value string) error
}

// Authenticator is the part of meltkit's auth package account management
// uses. AuthBackend implements it over *auth.Auth.
type Authenticator interface {
	IsInitialAdmin(id auth.Identity) bool
	CreatePasswordUser(ctx context.Context, userID int64, username, password string) error
	SetPassword(ctx context.Context, userID int64, password string) error
	ChangePassword(ctx context.Context, userID int64, current, password string) error
	// PasswordUsername returns the user's local username, empty if the user
	// has no password login.
	PasswordUsername(ctx context.Context, userID int64) (string, error)
	ListSessions(ctx context.Context, userID int64) ([]auth.Session, error)
	RevokeSession(ctx context.Context, userID, sessionID int64) error
	RevokeUserSessions(ctx context.Context, userID int64) error
	ListIdentities(ctx context.Context, userID int64) ([]auth.IdentityLink, error)
	UnlinkIdentity(ctx context.Context, userID int64, issuer, subject string) error
	DeleteUser(ctx context.Context, userID int64) error
}

// AuthBackend joins *auth.Auth with its store, which answers whether a user
// has a password login.
type AuthBackend struct {
	*auth.Auth
	Store auth.Store
}

// PasswordUsername implements Authenticator.
func (b AuthBackend) PasswordUsername(ctx context.Context, userID int64) (string, error) {
	rec, err := b.Store.GetPasswordByUserID(ctx, userID)
	if errors.Is(err, auth.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return rec.Username, nil
}

// Accounts decides who may log in and who is admin, and manages users. It is
// the auth.UserResolver of the instance. Without an `auth:` block it holds no
// Authenticator and answers only IsAdmin.
type Accounts struct {
	db      accountSource
	auth    Authenticator
	methods []string
	log     *slog.Logger
}

// NewAccounts returns account management over db, without login until
// Attach.
func NewAccounts(db accountSource, log *slog.Logger) *Accounts {
	return &Accounts{db: db, log: log}
}

// Attach switches login on with the authenticator and its enabled methods. It
// is separate from NewAccounts because auth.New takes the Accounts as its
// resolver. Call it before serving.
func (a *Accounts) Attach(au Authenticator, methods []string) {
	a.auth = au
	a.methods = methods
}

// Enabled reports whether application login is configured.
func (a *Accounts) Enabled() bool { return a.auth != nil }

// Methods returns the enabled login methods.
func (a *Accounts) Methods() []string { return a.methods }

func (a *Accounts) hasMethod(m string) bool { return slices.Contains(a.methods, m) }

// IsAdmin reports whether the user may change service-wide settings and
// manage users.
//
// With login configured, only `users.is_admin` counts. Without it, the rule
// from before the flag existed still applies to an instance that has no
// admin yet: the primary user (the earliest login containing '@'), or anyone
// while there is none. The primary user is then recorded as admin, so the
// flag is in place when login is switched on.
func (a *Accounts) IsAdmin(ctx context.Context, userID int) (bool, error) {
	isAdmin, disabled, err := a.db.GetUserFlags(ctx, userID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return false, err
	}
	if isAdmin && !disabled {
		return true, nil
	}
	if a.Enabled() {
		return false, nil
	}
	exists, err := a.db.AdminExists(ctx)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	primaryID, _, err := a.db.GetPrimaryUser(ctx)
	if errors.Is(err, storage.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := a.db.PromoteFirstAdmin(ctx, primaryID); err != nil {
		a.log.Warn("recording primary user as admin failed", "user_id", primaryID, "error", err)
	}
	return userID == primaryID, nil
}

// IsDisabled reports whether the user is disabled. A missing user counts as
// disabled.
func (a *Accounts) IsDisabled(ctx context.Context, userID int) (bool, error) {
	_, disabled, err := a.db.GetUserFlags(ctx, userID)
	if errors.Is(err, storage.ErrNotFound) {
		return true, nil
	}
	return disabled, err
}

// Resolve implements auth.UserResolver.
//
// An identity already linked to a user logs that user in unless the user is
// disabled. A first OIDC login gets an account when it is the configured
// initial admin and no admin exists, or when the account policy is oidc_auto
// and its email domain is allowed. Its login is the email; an existing user
// with that login is not taken over (that is link_by_email's job).
func (a *Accounts) Resolve(ctx context.Context, id auth.Identity) (int64, error) {
	if id.UserID != 0 {
		acct, err := a.db.GetUserAccount(ctx, int(id.UserID))
		if errors.Is(err, storage.ErrNotFound) {
			return 0, auth.ErrNoAccount
		}
		if err != nil {
			return 0, err
		}
		if acct.Disabled() {
			a.log.Info("login refused: account disabled", "user_id", acct.ID)
			return 0, auth.ErrNoAccount
		}
		a.bootstrapAdmin(ctx, id, acct.ID)
		if err := a.db.TouchUser(ctx, acct.ID, id.Name); err != nil {
			a.log.Warn("recording login failed", "user_id", acct.ID, "error", err)
		}
		return id.UserID, nil
	}

	if id.Method != auth.MethodOIDC {
		return 0, auth.ErrNoAccount
	}
	login := strings.TrimSpace(id.Email)
	if login == "" {
		a.log.Info("login refused: OIDC identity without email", "issuer", id.Issuer, "subject", id.Subject)
		return 0, auth.ErrNoAccount
	}

	initial := false
	if a.auth != nil && a.auth.IsInitialAdmin(id) {
		exists, err := a.db.AdminExists(ctx)
		if err != nil {
			return 0, err
		}
		initial = !exists
	}
	if !initial {
		policy, err := a.Policy(ctx)
		if err != nil {
			return 0, err
		}
		if policy.Mode != PolicyOIDCAuto {
			a.log.Info("login refused: accounts are closed", "email", login)
			return 0, auth.ErrNoAccount
		}
		if !policy.allows(login) {
			a.log.Info("login refused: email domain not allowed", "email", login)
			return 0, auth.ErrNoAccount
		}
	}

	uid, err := a.db.CreateUser(ctx, login, id.Name, initial)
	if errors.Is(err, storage.ErrConflict) {
		a.log.Info("login refused: a user with this login exists and is not linked", "email", login)
		return 0, auth.ErrNoAccount
	}
	if err != nil {
		return 0, err
	}
	a.log.Info("account created by OIDC login", "user_id", uid, "email", login, "admin", initial)
	return int64(uid), nil
}

// bootstrapAdmin makes the user admin if the identity is the configured
// initial admin and no admin exists.
func (a *Accounts) bootstrapAdmin(ctx context.Context, id auth.Identity, userID int) {
	if a.auth == nil || !a.auth.IsInitialAdmin(id) {
		return
	}
	promoted, err := a.db.PromoteFirstAdmin(ctx, userID)
	if err != nil {
		a.log.Warn("initial admin promotion failed", "user_id", userID, "error", err)
		return
	}
	if promoted {
		a.log.Info("initial admin promoted", "user_id", userID)
	}
}

// UserIDByLogin implements auth.LoginLookup for link_by_email.
func (a *Accounts) UserIDByLogin(ctx context.Context, login string) (int64, error) {
	id, err := a.db.GetUserIDByLogin(ctx, login)
	if errors.Is(err, storage.ErrNotFound) {
		return 0, auth.ErrNoAccount
	}
	if err != nil {
		return 0, err
	}
	return int64(id), nil
}

// AccountPolicy is the admin's rule for first OIDC logins.
type AccountPolicy struct {
	Mode string `json:"mode"`
	// EmailDomains limits oidc_auto to these email domains; empty allows all.
	EmailDomains []string `json:"email_domains"`
}

func (p AccountPolicy) allows(email string) bool {
	if len(p.EmailDomains) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	return slices.Contains(p.EmailDomains, strings.ToLower(email[at+1:]))
}

// Policy returns the account policy; unset is closed.
func (a *Accounts) Policy(ctx context.Context) (AccountPolicy, error) {
	mode, err := a.db.GetAppSetting(ctx, storage.SettingAccountPolicy)
	if err != nil {
		return AccountPolicy{}, err
	}
	if mode != PolicyOIDCAuto {
		mode = PolicyClosed
	}
	raw, err := a.db.GetAppSetting(ctx, storage.SettingAccountDomains)
	if err != nil {
		return AccountPolicy{}, err
	}
	p := AccountPolicy{Mode: mode, EmailDomains: []string{}}
	for _, d := range strings.Split(raw, ",") {
		if d = strings.TrimSpace(d); d != "" {
			p.EmailDomains = append(p.EmailDomains, d)
		}
	}
	return p, nil
}

// SetPolicy validates and stores the account policy.
func (a *Accounts) SetPolicy(ctx context.Context, p AccountPolicy) (AccountPolicy, error) {
	if p.Mode != PolicyClosed && p.Mode != PolicyOIDCAuto {
		return AccountPolicy{}, fmt.Errorf("%w: mode must be %q or %q", ErrAccountInvalid, PolicyClosed, PolicyOIDCAuto)
	}
	domains := []string{}
	for _, d := range p.EmailDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" {
			continue
		}
		if strings.ContainsAny(d, "@, \t") || !strings.Contains(d, ".") {
			return AccountPolicy{}, fmt.Errorf("%w: %q is not an email domain", ErrAccountInvalid, d)
		}
		if !slices.Contains(domains, d) {
			domains = append(domains, d)
		}
	}
	if err := a.db.SetAppSetting(ctx, storage.SettingAccountPolicy, p.Mode); err != nil {
		return AccountPolicy{}, err
	}
	if err := a.db.SetAppSetting(ctx, storage.SettingAccountDomains, strings.Join(domains, ",")); err != nil {
		return AccountPolicy{}, err
	}
	return AccountPolicy{Mode: p.Mode, EmailDomains: domains}, nil
}

// UserView is a user as the admin's user list shows it.
type UserView struct {
	storage.UserAccount
	// PasswordUsername is the local username, empty without a password login.
	PasswordUsername string              `json:"password_username"`
	Identities       []auth.IdentityLink `json:"identities"`
}

func (a *Accounts) view(ctx context.Context, acct storage.UserAccount) (UserView, error) {
	v := UserView{UserAccount: acct, Identities: []auth.IdentityLink{}}
	var err error
	if v.PasswordUsername, err = a.auth.PasswordUsername(ctx, int64(acct.ID)); err != nil {
		return UserView{}, err
	}
	ids, err := a.auth.ListIdentities(ctx, int64(acct.ID))
	if err != nil {
		return UserView{}, err
	}
	if ids != nil {
		v.Identities = ids
	}
	return v, nil
}

// ListUsers returns every user with their login methods.
func (a *Accounts) ListUsers(ctx context.Context) ([]UserView, error) {
	if !a.Enabled() {
		return nil, ErrAuthDisabled
	}
	accts, err := a.db.ListUserAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]UserView, 0, len(accts))
	for _, acct := range accts {
		v, err := a.view(ctx, acct)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func checkNewPassword(pw string) error {
	if utf8.RuneCountInString(pw) < auth.MinPasswordLength {
		return fmt.Errorf("%w: the password needs at least %d characters", ErrAccountInvalid, auth.MinPasswordLength)
	}
	return nil
}

func mapAuthErr(err error) error {
	switch {
	case errors.Is(err, auth.ErrWeakPassword):
		return fmt.Errorf("%w: the password needs at least %d characters", ErrAccountInvalid, auth.MinPasswordLength)
	case errors.Is(err, auth.ErrExists):
		return ErrAccountConflict
	case errors.Is(err, auth.ErrInvalidCredentials):
		return fmt.Errorf("%w: the current password is wrong", ErrAccountInvalid)
	case errors.Is(err, auth.ErrRateLimited):
		return ErrAccountRateLimited
	}
	return err
}

// CreatePasswordUser creates a user whose login is username and gives it a
// password login.
func (a *Accounts) CreatePasswordUser(ctx context.Context, username, password string, admin bool) (UserView, error) {
	if !a.Enabled() {
		return UserView{}, ErrAuthDisabled
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return UserView{}, fmt.Errorf("%w: a username is required", ErrAccountInvalid)
	}
	if err := checkNewPassword(password); err != nil {
		return UserView{}, err
	}
	uid, err := a.db.CreateUser(ctx, username, "", admin)
	if errors.Is(err, storage.ErrConflict) {
		return UserView{}, ErrAccountConflict
	}
	if err != nil {
		return UserView{}, err
	}
	if err := a.auth.CreatePasswordUser(ctx, int64(uid), username, password); err != nil {
		if derr := a.db.DeleteUser(ctx, uid); derr != nil {
			a.log.Error("removing user after failed password creation failed", "user_id", uid, "error", derr)
		}
		return UserView{}, mapAuthErr(err)
	}
	acct, err := a.db.GetUserAccount(ctx, uid)
	if err != nil {
		return UserView{}, err
	}
	return a.view(ctx, acct)
}

// EnsurePasswordUser is the CLI's create-user: it creates the user, or gives
// an existing user without a password a password login under its login.
// admin sets the admin flag either way. created reports a new user.
func (a *Accounts) EnsurePasswordUser(ctx context.Context, username, password string, admin bool) (uid int, created bool, err error) {
	if !a.Enabled() {
		return 0, false, ErrAuthDisabled
	}
	username = strings.TrimSpace(username)
	id, err := a.db.GetUserIDByLogin(ctx, username)
	if errors.Is(err, storage.ErrNotFound) {
		v, err := a.CreatePasswordUser(ctx, username, password, admin)
		return v.ID, err == nil, err
	}
	if err != nil {
		return 0, false, err
	}
	existing, err := a.auth.PasswordUsername(ctx, int64(id))
	if err != nil {
		return 0, false, err
	}
	if existing != "" {
		return 0, false, fmt.Errorf("%w: user %q already has a password login; reset it in Settings", ErrAccountConflict, username)
	}
	if err := checkNewPassword(password); err != nil {
		return 0, false, err
	}
	if err := a.auth.CreatePasswordUser(ctx, int64(id), username, password); err != nil {
		return 0, false, mapAuthErr(err)
	}
	if admin {
		if err := a.db.SetUserAdmin(ctx, id, true); err != nil {
			return 0, false, err
		}
	}
	return id, false, nil
}

// SetUserPassword sets a user's password as the admin. A user without a
// password login gets one under its login. Existing sessions end.
func (a *Accounts) SetUserPassword(ctx context.Context, userID int, password string) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	if err := checkNewPassword(password); err != nil {
		return err
	}
	acct, err := a.db.GetUserAccount(ctx, userID)
	if err != nil {
		return err
	}
	existing, err := a.auth.PasswordUsername(ctx, int64(userID))
	if err != nil {
		return err
	}
	if existing == "" {
		return mapAuthErr(a.auth.CreatePasswordUser(ctx, int64(userID), acct.Login, password))
	}
	if err := a.auth.SetPassword(ctx, int64(userID), password); err != nil {
		return mapAuthErr(err)
	}
	return a.auth.RevokeUserSessions(ctx, int64(userID))
}

// SetUserDisabled disables or re-enables a user. Disabling ends the user's
// sessions; the admin cannot disable themselves.
func (a *Accounts) SetUserDisabled(ctx context.Context, actorID, userID int, disabled bool) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	if actorID == userID && disabled {
		return ErrAccountSelf
	}
	if err := a.db.SetUserDisabled(ctx, userID, disabled); err != nil {
		return err
	}
	if disabled {
		return a.auth.RevokeUserSessions(ctx, int64(userID))
	}
	return nil
}

// SetUserAdmin grants or revokes admin. The admin cannot revoke their own.
func (a *Accounts) SetUserAdmin(ctx context.Context, actorID, userID int, admin bool) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	if actorID == userID && !admin {
		return ErrAccountSelf
	}
	return a.db.SetUserAdmin(ctx, userID, admin)
}

// builtInUserID is the `local` user migration 000001 seeds.
const builtInUserID = 1

// DeleteUser removes a user that owns no library rows, with its sessions,
// identities and password. A user with library rows can only be disabled.
func (a *Accounts) DeleteUser(ctx context.Context, actorID, userID int) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	if actorID == userID {
		return ErrAccountSelf
	}
	if userID == builtInUserID {
		return ErrAccountBuiltIn
	}
	acct, err := a.db.GetUserAccount(ctx, userID)
	if err != nil {
		return err
	}
	if acct.Items > 0 {
		return ErrAccountHasItems
	}
	if err := a.auth.DeleteUser(ctx, int64(userID)); err != nil {
		return err
	}
	err = a.db.DeleteUser(ctx, userID)
	if errors.Is(err, storage.ErrConflict) {
		return ErrAccountHasItems
	}
	return err
}

// SessionView is one of the user's sessions.
type SessionView struct {
	auth.Session
	Current bool `json:"current"`
}

// IdentityView is one of the user's OIDC identities.
type IdentityView struct {
	auth.IdentityLink
	// CanUnlink is false when it is the user's last way to sign in.
	CanUnlink bool `json:"can_unlink"`
}

// AccountView is the signed-in user's own account page.
type AccountView struct {
	UserID           int            `json:"user_id"`
	Login            string         `json:"login"`
	Methods          []string       `json:"methods"`
	PasswordUsername string         `json:"password_username"`
	Sessions         []SessionView  `json:"sessions"`
	Identities       []IdentityView `json:"identities"`
}

// Account returns the user's own login methods and sessions. currentSession
// marks the session the request came with, 0 for none.
func (a *Accounts) Account(ctx context.Context, userID int, currentSession int64) (AccountView, error) {
	if !a.Enabled() {
		return AccountView{}, ErrAuthDisabled
	}
	acct, err := a.db.GetUserAccount(ctx, userID)
	if err != nil {
		return AccountView{}, err
	}
	pw, err := a.auth.PasswordUsername(ctx, int64(userID))
	if err != nil {
		return AccountView{}, err
	}
	sessions, err := a.auth.ListSessions(ctx, int64(userID))
	if err != nil {
		return AccountView{}, err
	}
	ids, err := a.auth.ListIdentities(ctx, int64(userID))
	if err != nil {
		return AccountView{}, err
	}
	v := AccountView{
		UserID:           userID,
		Login:            acct.Login,
		Methods:          a.methods,
		PasswordUsername: pw,
		Sessions:         make([]SessionView, 0, len(sessions)),
		Identities:       make([]IdentityView, 0, len(ids)),
	}
	for _, s := range sessions {
		v.Sessions = append(v.Sessions, SessionView{Session: s, Current: s.ID == currentSession})
	}
	for _, id := range ids {
		v.Identities = append(v.Identities, IdentityView{IdentityLink: id, CanUnlink: a.keepsLogin(pw != "", len(ids)-1)})
	}
	return v, nil
}

// keepsLogin reports whether a user with this password state and this many
// remaining OIDC identities can still sign in.
func (a *Accounts) keepsLogin(hasPassword bool, identities int) bool {
	return hasPassword && a.hasMethod(auth.MethodPassword) || identities > 0 && a.hasMethod(auth.MethodOIDC)
}

// RevokeSession ends one of the user's own sessions.
func (a *Accounts) RevokeSession(ctx context.Context, userID int, sessionID int64) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	err := a.auth.RevokeSession(ctx, int64(userID), sessionID)
	if errors.Is(err, auth.ErrNotFound) {
		return storage.ErrNotFound
	}
	return err
}

// ChangePassword changes the user's own password after checking the current
// one.
func (a *Accounts) ChangePassword(ctx context.Context, userID int, current, password string) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	if err := checkNewPassword(password); err != nil {
		return err
	}
	err := a.auth.ChangePassword(ctx, int64(userID), current, password)
	if errors.Is(err, auth.ErrNotFound) {
		return fmt.Errorf("%w: this account has no password login", ErrAccountInvalid)
	}
	return mapAuthErr(err)
}

// UnlinkIdentity removes one of the user's OIDC identities, unless it is the
// user's last way to sign in.
func (a *Accounts) UnlinkIdentity(ctx context.Context, userID int, issuer, subject string) error {
	if !a.Enabled() {
		return ErrAuthDisabled
	}
	pw, err := a.auth.PasswordUsername(ctx, int64(userID))
	if err != nil {
		return err
	}
	ids, err := a.auth.ListIdentities(ctx, int64(userID))
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(ids, func(l auth.IdentityLink) bool { return l.Issuer == issuer && l.Subject == subject }) {
		return storage.ErrNotFound
	}
	if !a.keepsLogin(pw != "", len(ids)-1) {
		return ErrAccountLastLogin
	}
	err = a.auth.UnlinkIdentity(ctx, int64(userID), issuer, subject)
	if errors.Is(err, auth.ErrNotFound) {
		return storage.ErrNotFound
	}
	return err
}
