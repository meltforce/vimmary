package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// handleVersion reports the build and whether the database answers. It is
// unauthenticated on purpose: a commit hash is already public, and a deploy
// gate has no Tailscale identity to authenticate with.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	status := "ok"
	code := http.StatusOK
	if err := s.store.Ping(ctx); err != nil {
		status = "database unreachable"
		code = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": status,
		"build":  s.version,
	})
}

// DBPinger is the part of the storage layer the health check uses.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// StartHealthListener serves a health endpoint on a loopback address, so a
// container runtime can distinguish a process that is up from one that is
// serving.
//
// It exists because the real listener runs on the tsnet netstack when Tailscale
// is enabled, and nothing inside the container can dial that — a Docker
// healthcheck against localhost would fail on a healthy service. Without one,
// a start that hangs before the HTTP server (resolving secrets, running
// migrations) leaves a container that looks green and serves nothing. That is
// exactly what happened on 2026-08-06; see INCIDENTS.md.
//
// Call this only once initialisation has finished. The listener coming up is
// itself the signal: if the process is still resolving secrets, nothing
// answers, which is the point.
func StartHealthListener(ctx context.Context, addr, version string, db DBPinger, log *slog.Logger) (io.Closer, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// A reachable database is part of being able to serve, so the check
		// covers it rather than only reporting that the goroutine is alive.
		pingCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		status := "ok"
		code := http.StatusOK
		if db != nil {
			if err := db.Ping(pingCtx); err != nil {
				status = "database unreachable: " + err.Error()
				code = http.StatusServiceUnavailable
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": status,
			"build":  version,
		})
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error("health listener stopped", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	log.Info("health listener started", "addr", addr)
	return srv, nil
}
