package server

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/meltforce/meltkit/pkg/auth"
	"github.com/meltforce/meltkit/pkg/middleware"
	mkserver "github.com/meltforce/meltkit/pkg/server"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

type fakeWhois struct{ login string }

func (f fakeWhois) WhoIs(context.Context, string) (*apitype.WhoIsResponse, error) {
	return &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{LoginName: f.login, DisplayName: f.login},
		Node:        &tailcfg.Node{ComputedName: "laptop", Name: "laptop.tail.ts.net."},
	}, nil
}

type fakeUsers struct{ id int }

func (f fakeUsers) GetOrCreateUser(context.Context, string, string) (int, error) { return f.id, nil }
func (f fakeUsers) GetPrimaryUser(context.Context) (int, string, error)          { return f.id, "", nil }

type nopResolver struct{}

func (nopResolver) Resolve(context.Context, auth.Identity) (int64, error) {
	return 0, auth.ErrNoAccount
}

// tokenStore keeps personal access tokens in memory. The embedded Store is
// nil: no request in these tests carries a session cookie to a route that
// reads it, so only the token methods are called.
type tokenStore struct {
	auth.Store
	tokens []auth.Token
}

func (m *tokenStore) CreateToken(_ context.Context, t *auth.Token) error {
	t.ID = int64(len(m.tokens) + 1)
	m.tokens = append(m.tokens, *t)
	return nil
}

func (m *tokenStore) GetToken(_ context.Context, h []byte, now time.Time) (auth.Token, error) {
	for _, t := range m.tokens {
		if bytes.Equal(t.TokenHash, h) && (t.ExpiresAt.IsZero() || t.ExpiresAt.After(now)) {
			return t, nil
		}
	}
	return auth.Token{}, auth.ErrNotFound
}

func (m *tokenStore) TouchToken(context.Context, int64, time.Time) error { return nil }

func (m *tokenStore) ListTokens(_ context.Context, uid int64) ([]auth.Token, error) {
	var out []auth.Token
	for _, t := range m.tokens {
		if t.UserID == uid {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *tokenStore) DeleteToken(context.Context, int64, int64) error { return auth.ErrNotFound }

// identityServer builds a Server with only what identity() and SetMCP read.
func identityServer(t *testing.T, withAuth bool, disabled bool) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Server: mkserver.New(mkserver.WithLogger(log)), log: log}
	s.isDisabled = func(context.Context, int) (bool, error) { return disabled, nil }
	if withAuth {
		a, err := auth.New(auth.Config{BaseURL: "http://localhost:8080", Methods: []string{auth.MethodPassword}},
			&tokenStore{}, nopResolver{}, auth.WithLogger(log), auth.WithTokenPrefix("vimmary"))
		if err != nil {
			t.Fatal(err)
		}
		s.auth = a
	}
	return s
}

// newToken creates a token of user 7 with these scopes.
func newToken(t *testing.T, s *Server, scopes ...string) string {
	t.Helper()
	secret, _, err := s.auth.CreateToken(context.Background(), 7, "test", scopes, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func serveIdentity(s *Server, req *http.Request) (status, uid int) {
	uid = -1
	h := s.identity(http.HandlerFunc(unauthenticated))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ = middleware.UserIDFromContext(r)
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, uid
}

func TestIdentityWithoutSession(t *testing.T) {
	tests := []struct {
		name     string
		auth     bool
		tailnet  bool
		disabled bool
		status   int
		uid      int
	}{
		// Without an auth block nothing changes: DevIdentity off the tailnet,
		// WhoIs on it.
		{name: "no auth, no tailnet", auth: false, tailnet: false, status: http.StatusNoContent, uid: 1},
		{name: "no auth, tailnet", auth: false, tailnet: true, status: http.StatusNoContent, uid: 7},
		// With login configured, a request without a session falls back to
		// WhoIs only when tsnet runs; off the tailnet DevIdentity would make
		// it user 1.
		{name: "auth, no tailnet", auth: true, tailnet: false, status: http.StatusUnauthorized, uid: -1},
		{name: "auth, tailnet", auth: true, tailnet: true, status: http.StatusNoContent, uid: 7},
		{name: "auth, tailnet, disabled user", auth: true, tailnet: true, disabled: true, status: http.StatusForbidden, uid: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := identityServer(t, tt.auth, tt.disabled)
			if tt.tailnet {
				s.SetTailscale(fakeWhois{login: "alice@example.com"}, fakeUsers{id: 7})
			}
			status, uid := serveIdentity(s, httptest.NewRequest(http.MethodGet, "/api/v1/videos", nil))
			if status != tt.status || uid != tt.uid {
				t.Errorf("status %d, user %d; want %d, %d", status, uid, tt.status, tt.uid)
			}
		})
	}
}

func TestIdentityWithToken(t *testing.T) {
	tests := []struct {
		name     string
		scopes   []string
		bad      bool
		disabled bool
		status   int
		uid      int
	}{
		{name: "api token", scopes: []string{auth.ScopeAPI}, status: http.StatusNoContent, uid: 7},
		{name: "mcp token", scopes: []string{auth.ScopeMCP}, status: http.StatusForbidden, uid: -1},
		{name: "both scopes", scopes: []string{auth.ScopeMCP, auth.ScopeAPI}, status: http.StatusNoContent, uid: 7},
		{name: "unknown token", bad: true, status: http.StatusUnauthorized, uid: -1},
		{name: "disabled user", scopes: []string{auth.ScopeAPI}, disabled: true, status: http.StatusForbidden, uid: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := identityServer(t, true, tt.disabled)
			// The bearer path wins over WhoIs: a tailnet caller with a bad
			// token is refused rather than identified by its node.
			s.SetTailscale(fakeWhois{login: "alice@example.com"}, fakeUsers{id: 9})
			secret := "vimmary_pat_unknown"
			if !tt.bad {
				secret = newToken(t, s, tt.scopes...)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/videos", nil)
			req.Header.Set("Authorization", "Bearer "+secret)
			status, uid := serveIdentity(s, req)
			if status != tt.status || uid != tt.uid {
				t.Errorf("status %d, user %d; want %d, %d", status, uid, tt.status, tt.uid)
			}
		})
	}
}

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

// serveMCP mounts /mcp through s.SetMCP and sends an initialize request. uid
// is the user the injector saw, -1 if the request did not reach it.
func serveMCP(t *testing.T, s *Server, header http.Header) (status, uid int, rec *httptest.ResponseRecorder) {
	t.Helper()
	uid = -1
	s.SetMCP(mcpserver.NewMCPServer("vimmary-test", "0"), func(ctx context.Context, r *http.Request) context.Context {
		uid, _ = middleware.UserIDFromContext(r)
		return ctx
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initializeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range header {
		req.Header[k] = v
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec.Code, uid, rec
}

func TestMCPAuthentication(t *testing.T) {
	bearer := func(secret string) http.Header { return http.Header{"Authorization": {"Bearer " + secret}} }

	t.Run("no auth block: as before", func(t *testing.T) {
		// Off the tailnet DevIdentity (user 1), on it WhoIs.
		s := identityServer(t, false, false)
		if status, uid, _ := serveMCP(t, s, nil); status != http.StatusOK || uid != 1 {
			t.Errorf("no tailnet: status %d, user %d; want 200, 1", status, uid)
		}
		s = identityServer(t, false, false)
		s.SetTailscale(fakeWhois{login: "alice@example.com"}, fakeUsers{id: 7})
		if status, uid, _ := serveMCP(t, s, nil); status != http.StatusOK || uid != 7 {
			t.Errorf("tailnet: status %d, user %d; want 200, 7", status, uid)
		}
	})

	t.Run("mcp token", func(t *testing.T) {
		s := identityServer(t, true, false)
		status, uid, rec := serveMCP(t, s, bearer(newToken(t, s, auth.ScopeMCP)))
		if status != http.StatusOK || uid != 7 {
			t.Errorf("status %d, user %d; want 200, 7: %s", status, uid, rec.Body)
		}
	})

	t.Run("api token lacks scope", func(t *testing.T) {
		s := identityServer(t, true, false)
		status, uid, rec := serveMCP(t, s, bearer(newToken(t, s, auth.ScopeAPI)))
		if status != http.StatusForbidden || uid != -1 {
			t.Errorf("status %d, user %d; want 403", status, uid)
		}
		if h := rec.Header().Get("WWW-Authenticate"); !strings.Contains(h, "insufficient_scope") {
			t.Errorf("WWW-Authenticate = %q", h)
		}
	})

	t.Run("session cookie alone, no tailnet", func(t *testing.T) {
		// A session does not satisfy scope mcp, so the cookie is not even
		// looked up.
		s := identityServer(t, true, false)
		status, uid, rec := serveMCP(t, s, http.Header{"Cookie": {"session=whatever"}})
		if status != http.StatusUnauthorized || uid != -1 {
			t.Errorf("status %d, user %d; want 401", status, uid)
		}
		if h := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(h, "Bearer") {
			t.Errorf("WWW-Authenticate = %q", h)
		}
	})

	t.Run("no credentials, tailnet: WhoIs", func(t *testing.T) {
		s := identityServer(t, true, false)
		s.SetTailscale(fakeWhois{login: "alice@example.com"}, fakeUsers{id: 9})
		if status, uid, _ := serveMCP(t, s, nil); status != http.StatusOK || uid != 9 {
			t.Errorf("status %d, user %d; want 200, 9", status, uid)
		}
	})

	t.Run("disabled user's token", func(t *testing.T) {
		s := identityServer(t, true, true)
		status, uid, _ := serveMCP(t, s, bearer(newToken(t, s, auth.ScopeMCP)))
		if status != http.StatusForbidden || uid != -1 {
			t.Errorf("status %d, user %d; want 403", status, uid)
		}
	})

	t.Run("disabled user over WhoIs", func(t *testing.T) {
		s := identityServer(t, true, true)
		s.SetTailscale(fakeWhois{login: "alice@example.com"}, fakeUsers{id: 9})
		if status, uid, _ := serveMCP(t, s, nil); status != http.StatusForbidden || uid != -1 {
			t.Errorf("status %d, user %d; want 403", status, uid)
		}
	})
}
