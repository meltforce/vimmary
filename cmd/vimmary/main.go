package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	mkconfig "github.com/meltforce/meltkit/pkg/config"
	"github.com/meltforce/meltkit/pkg/db"
	"github.com/meltforce/meltkit/pkg/middleware"
	"github.com/meltforce/meltkit/pkg/secrets"
	vimmary "github.com/meltforce/vimmary"
	"github.com/meltforce/vimmary/internal/cast2md"
	"github.com/meltforce/vimmary/internal/config"
	vimmarymcp "github.com/meltforce/vimmary/internal/mcp"
	"github.com/meltforce/vimmary/internal/mistral"
	"github.com/meltforce/vimmary/internal/models"
	"github.com/meltforce/vimmary/internal/server"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/youtube"
	"tailscale.com/tsnet"
)

var Version = "dev"

// The startup sequence is the subject of two entries in INCIDENTS.md, both from
// 2026-08-06, and both about the order rather than about any single step. It is
// therefore spread over the named functions below in the order they must run:
//
//	newLogger -> config.Load -> startTailscale -> openDatabase -> buildService
//	-> buildHTTPServer -> openListener -> health listener -> serve
//
// run() is that sequence and nothing else. Two properties are load-bearing:
//
//   - Every defer belongs to run, not to the function that created the thing it
//     closes. A defer inside startTailscale would close the node the moment
//     startTailscale returned.
//   - The health listener comes last, because its existence is the readiness
//     signal a container runtime reads. Anything moved after it reports ready
//     before it is.
func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	migrateOnly := flag.Bool("migrate-only", false, "run migrations and exit")
	mcpMode := flag.Bool("mcp", false, "run as MCP server over stdio")
	flag.Parse()

	log := newLogger(*mcpMode)
	log.Info("vimmary starting", "version", Version)

	if err := run(*configPath, *migrateOnly, *mcpMode, log); err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

// newLogger writes to stderr in MCP stdio mode, because stdout carries the
// protocol there and a log line on it corrupts the stream.
func newLogger(mcpMode bool) *slog.Logger {
	out := os.Stdout
	if mcpMode {
		out = os.Stderr
	}
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func run(configPath string, migrateOnly, mcpMode bool, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	tsServer, tsnetHTTPClient, err := startTailscale(cfg.Tailscale, mcpMode, log)
	if err != nil {
		return err
	}
	if tsServer != nil {
		defer func() { _ = tsServer.Close() }()
	}

	database, done, err := openDatabase(cfg, migrateOnly, mcpMode, log)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	defer database.Close()

	store := storage.NewDB(database)
	svc := buildService(cfg, store, tsnetHTTPClient, log)

	if mcpMode {
		return runMCPStdio(svc, log)
	}

	// Poll cast2md for newly transcribed episodes. No-op when cast2md is off.
	pollCtx, stopPoller := context.WithCancel(context.Background())
	defer stopPoller()
	svc.StartPodcastPoller(pollCtx)
	// Poll the RSS feeds of followed YouTube channels for the inbox.
	svc.StartChannelPoller(pollCtx)

	srv, err := buildHTTPServer(svc, store, log)
	if err != nil {
		return err
	}

	listener, err := openListener(cfg, tsServer, srv, store, log)
	if err != nil {
		return err
	}

	// The health listener comes up only now, with initialisation finished. A
	// container runtime cannot reach the tsnet listener, so this loopback
	// endpoint is what tells it whether the service actually serves.
	if cfg.HealthAddr != "" {
		healthSrv, err := server.StartHealthListener(context.Background(), cfg.HealthAddr, Version, store, log)
		if err != nil {
			return fmt.Errorf("health listener on %s: %w", cfg.HealthAddr, err)
		}
		defer func() { _ = healthSrv.Close() }()
	}

	return serve(&http.Server{Handler: srv}, listener, log)
}

// startTailscale brings the tsnet node up and returns it together with an HTTP
// client that dials over it. With Tailscale disabled, or in MCP stdio mode, it
// returns a nil server and the default client — callers test the server for nil
// to decide whether there is a tsnet listener to open.
func startTailscale(cfg mkconfig.TailscaleConfig, mcpMode bool, log *slog.Logger) (*tsnet.Server, *http.Client, error) {
	if !cfg.Enabled || mcpMode {
		return nil, http.DefaultClient, nil
	}

	tsServer := &tsnet.Server{Hostname: cfg.Hostname, Dir: cfg.StateDir}
	if err := tsServer.Start(); err != nil {
		return nil, nil, fmt.Errorf("tsnet start: %w", err)
	}

	// Start() returns while the node may still be without a current netmap.
	// Up() waits for it to reach Running with an address, which is what
	// ListenTLS below needs.
	//
	// It is kept for that reason alone. It was added on 2026-08-06 against
	// the setec startup race and did not prevent it — when tsnet loads its
	// persisted state the AuthLoop short-circuits to Running and Up()
	// returns satisfied within milliseconds, before the tailnet knows the
	// node. Nothing dials over this node during startup any more, so that
	// race no longer has a way to stop the process; see INCIDENTS.md,
	// 2026-08-07.
	upCtx, cancelUp := context.WithTimeout(context.Background(), 90*time.Second)
	tsStatus, err := tsServer.Up(upCtx)
	cancelUp()
	if err != nil {
		_ = tsServer.Close()
		return nil, nil, fmt.Errorf("tsnet did not come up: %w", err)
	}
	log.Info("tsnet up", "hostname", cfg.Hostname,
		"state", tsStatus.BackendState, "ips", tsStatus.TailscaleIPs)

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return tsServer.Dial(ctx, network, addr)
			},
		},
	}
	return tsServer, client, nil
}

// openDatabase resolves the password, runs the migrations and connects. The
// bool it returns is true when --migrate-only asked for the migrations alone,
// in which case there is no connection and run() is finished.
//
// The database password is the only secret resolved at startup, and it comes
// from VIMMARY_POSTGRES_PASSWORD in the environment — the resolver checks the
// environment before anything else, so no secret backend is configured.
//
// There is no setec client here any more. Fetching secrets over the tailnet
// during startup is what left vimmary dead for 6h23min on 2026-08-07: setec
// answered "access denied" to a node the tailnet had not yet placed, and the
// store retried 16812 times without ever opening the listener. The LLM API keys
// now live in app_settings and are read when they are used. Nothing in this
// startup path touches the network except tsnet itself.
func openDatabase(cfg *config.Config, migrateOnly, mcpMode bool, log *slog.Logger) (*db.DB, bool, error) {
	resolver := secrets.NewResolver(cfg.Secrets, "VIMMARY")
	dbPassword, err := resolver.ResolveSecret("postgres_password")
	if err != nil {
		return nil, false, fmt.Errorf("resolve postgres password: %w", err)
	}
	dsn := cfg.Database.DSN(dbPassword)

	// MCP stdio mode attaches to a database another process already migrated.
	if !mcpMode {
		migrationsFS, err := fs.Sub(vimmary.MigrationsFS, "migrations")
		if err != nil {
			return nil, false, fmt.Errorf("load embedded migrations: %w", err)
		}
		if err := db.RunMigrations(dsn, migrationsFS); err != nil {
			return nil, false, fmt.Errorf("migration: %w", err)
		}
		log.Info("migrations applied")
	}

	if migrateOnly {
		log.Info("migrate-only: exiting")
		return nil, true, nil
	}

	database, err := db.New(context.Background(), dsn, db.WithPgvector())
	if err != nil {
		return nil, false, fmt.Errorf("connect database: %w", err)
	}
	log.Info("database connected")
	return database, false, nil
}

// buildService wires the adapters the service reaches external systems through.
//
// The LLM API keys are service-wide settings in app_settings, maintained in the
// Settings page, so everything that needs one takes a function rather than a
// value: a key entered in the UI has to work without a restart.
//
// There used to be a gate that exited non-zero when the configured default
// provider had no key. It is gone on purpose: with the keys maintained in the
// UI, a fresh install has none, and a service that refuses to start cannot
// serve the page on which the key would be entered. A missing key is now a
// failed summary with a message that says so, not a dead process.
func buildService(cfg *config.Config, store *storage.DB, tsnetHTTPClient *http.Client, log *slog.Logger) *service.Service {
	// This client is the embedder and the transcriber. It reads the same Mistral
	// setting the Mistral summarizer uses, so changing that key in the UI changes
	// all three — which is intended, and worth knowing before treating it as a
	// summary-only setting.
	mc := mistral.NewClient(func(ctx context.Context) (string, error) {
		return store.GetLLMKey(ctx, "mistral")
	})
	ytClient := youtube.NewClient(cfg.YouTube.SubLangs)

	// cast2md sits inside the tailnet, so it needs the tsnet transport. The
	// variable is interface-typed and left nil when the feature is off — a nil
	// *cast2md.Client assigned to the interface would not compare equal to nil,
	// and the service would call methods on it.
	var podcastSrc service.PodcastSource
	if cfg.Cast2MD.Enabled {
		podcastSrc = cast2md.New(cfg.Cast2MD.BaseURL, tsnetHTTPClient,
			15*time.Second, time.Duration(cfg.Cast2MD.TimeoutSeconds)*time.Second)
		log.Info("cast2md client configured", "base_url", cfg.Cast2MD.BaseURL)
	}

	registry := models.NewRegistry(store.GetLLMKey, log)

	return service.New(store, registry, ytClient,
		podcastSrc, cfg.Cast2MD, cfg.Karakeep.BaseURL, cfg.ExternalURL, mc, mc,
		cfg.Search, cfg.Summary, log)
}

// runMCPStdio serves the MCP protocol over stdin and stdout and returns when
// the peer closes the stream. There is no HTTP listener and no poller in this
// mode; the identity middleware has no request to read, so every call is the
// primary user.
func runMCPStdio(svc *service.Service, log *slog.Logger) error {
	log.Info("starting MCP stdio server")
	mcpSrv := vimmarymcp.New(svc, Version, log)
	err := mcpserver.ServeStdio(mcpSrv,
		mcpserver.WithStdioContextFunc(func(ctx context.Context) context.Context {
			return vimmarymcp.WithUserID(ctx, 1)
		}),
	)
	if err != nil {
		return fmt.Errorf("MCP stdio server: %w", err)
	}
	return nil
}

// buildHTTPServer assembles the router: the REST and feed routes, the MCP
// endpoint and the embedded frontend. It opens no listener — that is
// openListener, and keeping the two apart is what lets the health listener come
// after everything is mounted.
func buildHTTPServer(svc *service.Service, store *storage.DB, log *slog.Logger) (*server.Server, error) {
	srv := server.New(svc, store, Version, log)

	mcpSrv := vimmarymcp.New(svc, Version, log)
	srv.SetMCP(mcpSrv, func(ctx context.Context, r *http.Request) context.Context {
		uid, _ := middleware.UserIDFromContext(r)
		return vimmarymcp.WithUserID(ctx, uid)
	})

	webDist, err := fs.Sub(vimmary.WebFS, "web/dist")
	if err != nil {
		return nil, fmt.Errorf("load embedded frontend: %w", err)
	}
	srv.SetFrontend(webDist)

	return srv, nil
}

// openListener returns the socket the HTTP server serves on: TLS on the tsnet
// netstack when Tailscale is up, a plain local port otherwise. SetTailscale is
// called here rather than in buildHTTPServer because the local client only
// exists once the node is up.
func openListener(cfg *config.Config, tsServer *tsnet.Server, srv *server.Server, store *storage.DB, log *slog.Logger) (net.Listener, error) {
	if tsServer == nil {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen on %s: %w", addr, err)
		}
		log.Info("server starting", "addr", addr, "mode", "dev (no tailscale)")
		return listener, nil
	}

	lc, err := tsServer.LocalClient()
	if err != nil {
		return nil, fmt.Errorf("tsnet local client: %w", err)
	}
	srv.SetTailscale(lc, store)

	listener, err := tsServer.ListenTLS("tcp", ":443")
	if err != nil {
		return nil, fmt.Errorf("tsnet listen: %w", err)
	}
	log.Info("tsnet server listening", "hostname", cfg.Tailscale.Hostname, "tls", true)
	return listener, nil
}

// serve runs until SIGINT or SIGTERM, then drains for up to ten seconds. A
// listener error other than ErrServerClosed comes back as the return value
// rather than exiting from the goroutine, so run()'s defers still fire.
func serve(httpSrv *http.Server, listener net.Listener, log *slog.Logger) error {
	serveErr := make(chan error, 1)
	go func() {
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("server: %w", err)
		}
	case sig := <-quit:
		log.Info("shutting down", "signal", sig)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Error("shutdown error", "error", err)
		}
	}

	log.Info("server stopped")
	return nil
}
