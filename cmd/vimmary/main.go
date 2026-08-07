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

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	migrateOnly := flag.Bool("migrate-only", false, "run migrations and exit")
	mcpMode := flag.Bool("mcp", false, "run as MCP server over stdio")
	flag.Parse()

	logOutput := os.Stdout
	if *mcpMode {
		logOutput = os.Stderr
	}
	log := slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("vimmary starting", "version", Version)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Start tsnet — it carries the public listener.
	var listener net.Listener
	var tsServer *tsnet.Server
	var tsnetHTTPClient *http.Client

	if cfg.Tailscale.Enabled && !*mcpMode {
		tsServer = &tsnet.Server{
			Hostname: cfg.Tailscale.Hostname,
			Dir:      cfg.Tailscale.StateDir,
		}
		if err := tsServer.Start(); err != nil {
			log.Error("tsnet start failed", "error", err)
			os.Exit(1)
		}
		defer func() { _ = tsServer.Close() }()

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
			log.Error("tsnet did not come up", "error", err)
			os.Exit(1)
		}
		log.Info("tsnet up", "hostname", cfg.Tailscale.Hostname,
			"state", tsStatus.BackendState, "ips", tsStatus.TailscaleIPs)

		tsnetHTTPClient = &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return tsServer.Dial(ctx, network, addr)
				},
			},
		}
		log.Info("tsnet started", "hostname", cfg.Tailscale.Hostname)
	} else {
		tsnetHTTPClient = http.DefaultClient
	}

	// The database password is the only secret resolved at startup, and it comes
	// from VIMMARY_POSTGRES_PASSWORD in the environment — the resolver checks
	// the environment before anything else, so no secret backend is configured.
	//
	// There is no setec client here any more. Fetching secrets over the tailnet
	// during startup is what left vimmary dead for 6h23min on 2026-08-07: setec
	// answered "access denied" to a node the tailnet had not yet placed, and the
	// store retried 16812 times without ever opening the listener. The LLM API
	// keys now live in app_settings and are read when they are used. Nothing in
	// this startup path touches the network except tsnet itself.
	resolver := secrets.NewResolver(cfg.Secrets, "VIMMARY")
	dbPassword, err := resolver.ResolveSecret("postgres_password")
	if err != nil {
		log.Error("failed to resolve postgres password", "error", err)
		os.Exit(1)
	}
	dsn := cfg.Database.DSN(dbPassword)

	// Run migrations
	if !*mcpMode {
		migrationsFS, err := fs.Sub(vimmary.MigrationsFS, "migrations")
		if err != nil {
			log.Error("failed to load embedded migrations", "error", err)
			os.Exit(1)
		}
		if err := db.RunMigrations(dsn, migrationsFS); err != nil {
			log.Error("migration failed", "error", err)
			os.Exit(1)
		}
		log.Info("migrations applied")
	}

	if *migrateOnly {
		log.Info("migrate-only: exiting")
		return
	}

	// Connect database
	ctx := context.Background()
	database, err := db.New(ctx, dsn, db.WithPgvector())
	if err != nil {
		log.Error("failed to connect database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	log.Info("database connected")

	store := storage.NewDB(database)

	// Init clients
	//
	// The LLM API keys are service-wide settings in app_settings, maintained in
	// the Settings page, so everything that needs one takes a function rather
	// than a value: a key entered in the UI has to work without a restart.
	//
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

	// Summarizers are built per summary from the key in app_settings, so there
	// is nothing to register here and nothing to check.
	//
	// There used to be a gate that exited non-zero when the configured default
	// provider had no key. It is gone on purpose: with the keys maintained in
	// the UI, a fresh install has none, and a service that refuses to start
	// cannot serve the page on which the key would be entered. A missing key is
	// now a failed summary with a message that says so, not a dead process.
	registry := models.NewRegistry(store.GetLLMKey, log)

	svc := service.New(store, registry, ytClient,
		podcastSrc, cfg.Cast2MD, cfg.Karakeep.BaseURL, cfg.ExternalURL, mc, mc,
		cfg.Search, cfg.Summary, log)

	// MCP stdio mode
	if *mcpMode {
		log.Info("starting MCP stdio server")
		mcpSrv := vimmarymcp.New(svc, Version, log)
		if err := mcpserver.ServeStdio(mcpSrv,
			mcpserver.WithStdioContextFunc(func(ctx context.Context) context.Context {
				return vimmarymcp.WithUserID(ctx, 1)
			}),
		); err != nil {
			log.Error("MCP stdio server error", "error", err)
			os.Exit(1)
		}
		return
	}

	// Poll cast2md for newly transcribed episodes. No-op when cast2md is off.
	pollCtx, stopPoller := context.WithCancel(context.Background())
	defer stopPoller()
	svc.StartPodcastPoller(pollCtx)

	// HTTP server
	srv := server.New(svc, store, Version, log)

	// Mount MCP
	mcpSrv := vimmarymcp.New(svc, Version, log)
	srv.SetMCP(mcpSrv, func(ctx context.Context, r *http.Request) context.Context {
		uid, _ := middleware.UserIDFromContext(r)
		return vimmarymcp.WithUserID(ctx, uid)
	})

	// Serve embedded frontend
	webDist, err := fs.Sub(vimmary.WebFS, "web/dist")
	if err != nil {
		log.Error("failed to load embedded frontend", "error", err)
		os.Exit(1)
	}
	srv.SetFrontend(webDist)

	// Finish tsnet setup or fall back to plain HTTP
	if tsServer != nil {
		lc, err := tsServer.LocalClient()
		if err != nil {
			log.Error("tsnet local client failed", "error", err)
			os.Exit(1)
		}
		srv.SetTailscale(lc, store)

		listener, err = tsServer.ListenTLS("tcp", ":443")
		if err != nil {
			log.Error("tsnet listen failed", "error", err)
			os.Exit(1)
		}
		log.Info("tsnet server listening", "hostname", cfg.Tailscale.Hostname, "tls", true)
	} else {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			log.Error("listen failed", "addr", addr, "error", err)
			os.Exit(1)
		}
		log.Info("server starting", "addr", addr, "mode", "dev (no tailscale)")
	}

	// The health listener comes up only now, with initialisation finished. A
	// container runtime cannot reach the tsnet listener, so this loopback
	// endpoint is what tells it whether the service actually serves.
	if cfg.HealthAddr != "" {
		healthSrv, err := server.StartHealthListener(ctx, cfg.HealthAddr, Version, store, log)
		if err != nil {
			log.Error("health listener failed to start", "addr", cfg.HealthAddr, "error", err)
			os.Exit(1)
		}
		defer func() { _ = healthSrv.Close() }()
	}

	httpSrv := &http.Server{Handler: srv}

	go func() {
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info("shutting down", "signal", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
	log.Info("server stopped")
}
