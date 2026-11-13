package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /healthz on the real listener reports status and the commit the image was
// built from; "dev" outside a pipeline build.
func TestHealthzReportsBuildSHA(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct{ name, sha, want string }{
		{"pipeline build", sha, `{"status":"ok","sha":"` + sha + `"}`},
		{"local build", "dev", `{"status":"ok","sha":"dev"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newBase(tc.sha, log).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if rec.Body.String() != tc.want {
				t.Errorf("body = %q, want %q", rec.Body.String(), tc.want)
			}
		})
	}
}
