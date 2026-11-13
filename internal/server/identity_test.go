package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

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

// identityServer builds a Server with only what identity() reads.
func identityServer(t *testing.T, withAuth bool, disabled bool) *Server {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Server: mkserver.New(mkserver.WithLogger(log)), log: log}
	s.isDisabled = func(context.Context, int) (bool, error) { return disabled, nil }
	if withAuth {
		// No request below carries a session cookie, so the store is never
		// queried.
		a, err := auth.New(auth.Config{BaseURL: "http://localhost:8080", Methods: []string{auth.MethodPassword}},
			auth.NewPgxStore(nil), nopResolver{}, auth.WithLogger(log))
		if err != nil {
			t.Fatal(err)
		}
		s.auth = a
	}
	return s
}

func serveIdentity(s *Server) (status, uid int) {
	uid = -1
	h := s.identity(http.HandlerFunc(unauthenticated))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ = middleware.UserIDFromContext(r)
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/videos", nil))
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
			status, uid := serveIdentity(s)
			if status != tt.status || uid != tt.uid {
				t.Errorf("status %d, user %d; want %d, %d", status, uid, tt.status, tt.uid)
			}
		})
	}
}
