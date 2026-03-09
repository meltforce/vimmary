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

	vimmary "github.com/meltforce/vimmary"
	"github.com/meltforce/vimmary/internal/config"
	vimmarymcp "github.com/meltforce/vimmary/internal/mcp"
	"github.com/meltforce/vimmary/internal/mistral"
	"github.com/meltforce/vimmary/internal/server"
	"github.com/meltforce/vimmary/internal/service"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
	"github.com/meltforce/vimmary/internal/youtube"
	"github.com/meltforce/meltkit/pkg/db"
	"github.com/meltforce/meltkit/pkg/middleware"
	"github.com/meltforce/meltkit/pkg/secrets"
	mcpserver "github.com/mark3labs/mcp-go/server"
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

	// Start tsnet — setec needs it.
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

		tsnetHTTPClient = &http.Client{
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

	// Init secrets resolver
	resolver := secrets.NewResolver(cfg.Secrets, "VIMMARY")
	if cfg.SecretBackend.Type == "setec" {
		if err := resolver.InitSetecStore(context.Background(), tsnetHTTPClient, cfg.SecretBackend.SetecURL); err != nil {
			log.Error("init setec store", "error", err)
			os.Exit(1)
		}
	}

	// Resolve secrets
	dbPassword, err := resolver.ResolveSecret("postgres_password")
	if err != nil {
		log.Error("failed to resolve postgres password", "error", err)
		os.Exit(1)
	}
	mistralKey, err := resolver.ResolveSecret("mistral_api_key")
	if err != nil {
		log.Error("failed to resolve mistral api key", "error", err)
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
	mc := mistral.NewClient(mistralKey)
	ytClient := youtube.NewClient(cfg.YouTube.SubLangs)

	// Init summarizer
	var summarizer summary.Summarizer
	switch cfg.Summary.Provider {
	case "mistral":
		summarizer = summary.NewMistralSummarizer(mistralKey, cfg.Summary.MistralModel)
	default:
		claudeKey, err := resolver.ResolveSecret("claude_api_key")
		if err != nil {
			log.Error("failed to resolve claude api key", "error", err)
			os.Exit(1)
		}
		summarizer = summary.NewClaudeSummarizer(claudeKey, cfg.Summary.ClaudeModel)
	}

	svc := service.New(store, summarizer, ytClient, cfg.Karakeep.BaseURL, cfg.ExternalURL, mc, cfg.Search, cfg.Summary, log)

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

	// HTTP server
	srv := server.New(svc, store, log)

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
