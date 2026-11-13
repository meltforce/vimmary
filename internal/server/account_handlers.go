package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/meltkit/pkg/middleware"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
)

// identity authenticates the identity group. Without an auth block it is
// meltkit's WhoIs middleware, unchanged. With one, a session comes first;
// a request without a session goes to WhoIs only while tsnet runs, and to
// unauth otherwise.
func (s *Server) identity(unauth http.Handler) func(http.Handler) http.Handler {
	if s.auth == nil {
		return s.IdentityMiddleware()
	}
	return s.auth.Middleware(auth.WithFallback(s.tailnetFallback(unauth)))
}

// tailnetFallback resolves a request without a session by Tailscale WhoIs and
// refuses disabled users, or hands it to unauth when tsnet is not running.
// SetTailscale runs after the routes are built, so the decision is made per
// request.
func (s *Server) tailnetFallback(unauth http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		whois := s.IdentityMiddleware()(s.refuseDisabled(next))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.tailnet {
				unauth.ServeHTTP(w, r)
				return
			}
			whois.ServeHTTP(w, r)
		})
	}
}

// refuseDisabled answers 403 for a user an admin disabled. Sessions of a
// disabled user are deleted, so only the WhoIs path needs the check.
func (s *Server) refuseDisabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := middleware.UserIDFromContext(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		disabled, err := s.isDisabled(r.Context(), uid)
		if err != nil {
			s.log.Error("disabled check failed", "user_id", uid, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check account"})
			return
		}
		if disabled {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "account disabled"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unauthenticated(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
}

// authMethod names how the request was authenticated: the session's method,
// "tailscale" for the WhoIs fallback, empty without login configured.
func (s *Server) authMethod(r *http.Request) string {
	if s.auth == nil {
		return ""
	}
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		return sess.Method
	}
	return "tailscale"
}

func (s *Server) accountRoutes(r chi.Router) {
	// Own account.
	r.Get("/api/v1/account", s.handleGetAccount)
	r.Delete("/api/v1/account/sessions/{id}", s.handleRevokeSession)
	r.Put("/api/v1/account/password", s.handleChangePassword)
	r.Post("/api/v1/account/identities/unlink", s.handleUnlinkIdentity)
	// A form POST, not fetch: it answers a redirect to the OIDC provider.
	r.Post("/account/link-oidc", s.handleLinkOIDC)

	// User management, admin only.
	r.Get("/api/v1/admin/users", s.handleListUsers)
	r.Post("/api/v1/admin/users", s.handleCreateUser)
	r.Put("/api/v1/admin/users/{id}", s.handleUpdateUser)
	r.Put("/api/v1/admin/users/{id}/password", s.handleSetUserPassword)
	r.Delete("/api/v1/admin/users/{id}", s.handleDeleteUser)
	r.Get("/api/v1/admin/account-policy", s.handleGetAccountPolicy)
	r.Put("/api/v1/admin/account-policy", s.handleSetAccountPolicy)
}

// writeAccountError maps account management errors to status codes.
func (s *Server) writeAccountError(w http.ResponseWriter, err error, logMsg string) {
	status := http.StatusInternalServerError
	msg := err.Error()
	switch {
	case errors.Is(err, service.ErrAccountInvalid):
		status = http.StatusBadRequest
		msg = strings.TrimPrefix(msg, service.ErrAccountInvalid.Error()+": ")
	case errors.Is(err, service.ErrAccountSelf):
		status = http.StatusBadRequest
	case errors.Is(err, service.ErrAccountConflict),
		errors.Is(err, service.ErrAccountHasItems),
		errors.Is(err, service.ErrAccountBuiltIn),
		errors.Is(err, service.ErrAccountLastLogin):
		status = http.StatusConflict
	case errors.Is(err, service.ErrAccountRateLimited):
		status = http.StatusTooManyRequests
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, service.ErrAuthDisabled):
		status = http.StatusNotFound
		msg = "not found"
	default:
		s.log.Error(logMsg, "error", err)
		msg = logMsg
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	var current int64
	if sess, ok := auth.SessionFromContext(r.Context()); ok {
		current = sess.ID
	}
	acct, err := s.svc.Accounts().Account(r.Context(), uid, current)
	if err != nil {
		s.writeAccountError(w, err, "failed to load account")
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (s *Server) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	sid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || sid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if err := s.svc.Accounts().RevokeSession(r.Context(), uid, sid); err != nil {
		s.writeAccountError(w, err, "failed to revoke session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	var body struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.svc.Accounts().ChangePassword(r.Context(), uid, body.Current, body.Password); err != nil {
		s.writeAccountError(w, err, "failed to change password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "changed"})
}

func (s *Server) handleUnlinkIdentity(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	var body struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.svc.Accounts().UnlinkIdentity(r.Context(), uid, body.Issuer, body.Subject); err != nil {
		s.writeAccountError(w, err, "failed to unlink identity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "unlinked"})
}

// handleLinkOIDC starts the OIDC flow that links a provider identity to the
// signed-in user, then returns to the account settings.
func (s *Server) handleLinkOIDC(w http.ResponseWriter, r *http.Request) {
	uid, ok := mustUserID(w, r)
	if !ok {
		return
	}
	s.auth.StartOIDCLink(w, r, int64(uid), "/settings?tab=account")
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}
	users, err := s.svc.Accounts().ListUsers(r.Context())
	if err != nil {
		s.writeAccountError(w, err, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	user, err := s.svc.Accounts().CreatePasswordUser(r.Context(), body.Username, body.Password, body.IsAdmin)
	if err != nil {
		s.writeAccountError(w, err, "failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

// handleUpdateUser changes the disabled state and the admin flag; an absent
// field is left alone.
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.mustAdmin(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Disabled *bool `json:"disabled"`
		IsAdmin  *bool `json:"is_admin"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	acc := s.svc.Accounts()
	if body.Disabled != nil {
		if err := acc.SetUserDisabled(r.Context(), actor, id, *body.Disabled); err != nil {
			s.writeAccountError(w, err, "failed to update user")
			return
		}
	}
	if body.IsAdmin != nil {
		if err := acc.SetUserAdmin(r.Context(), actor, id, *body.IsAdmin); err != nil {
			s.writeAccountError(w, err, "failed to update user")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleSetUserPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.svc.Accounts().SetUserPassword(r.Context(), id, body.Password); err != nil {
		s.writeAccountError(w, err, "failed to set password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.mustAdmin(w, r)
	if !ok {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	if err := s.svc.Accounts().DeleteUser(r.Context(), actor, id); err != nil {
		s.writeAccountError(w, err, "failed to delete user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleGetAccountPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}
	p, err := s.svc.Accounts().Policy(r.Context())
	if err != nil {
		s.writeAccountError(w, err, "failed to read account policy")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleSetAccountPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mustAdmin(w, r); !ok {
		return
	}
	var body service.AccountPolicy
	if !decodeBody(w, r, &body) {
		return
	}
	p, err := s.svc.Accounts().SetPolicy(r.Context(), body)
	if err != nil {
		s.writeAccountError(w, err, "failed to save account policy")
		return
	}
	writeJSON(w, http.StatusOK, p)
}
