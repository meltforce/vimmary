package config

import (
	"fmt"

	mkconfig "github.com/meltforce/meltkit/pkg/config"
)

type Config struct {
	ExternalURL string `yaml:"external_url"`
	// HealthAddr is a loopback-only listener that exists so a container runtime
	// can tell "the process is up" from "the process is serving". With
	// Tailscale enabled the real listener lives on the tsnet netstack, which
	// nothing inside the container can dial, so a healthcheck against localhost
	// would fail on a perfectly healthy service. Empty disables it.
	HealthAddr    string                       `yaml:"health_addr"`
	Server        mkconfig.ServerConfig        `yaml:"server"`
	Database      mkconfig.DatabaseConfig      `yaml:"database"`
	Tailscale     mkconfig.TailscaleConfig     `yaml:"tailscale"`
	SecretBackend mkconfig.SecretBackendConfig `yaml:"secret_backend"`
	Secrets       map[string]string            `yaml:"secrets"`
	Search        SearchConfig                 `yaml:"search"`
	Summary       SummaryConfig                `yaml:"summary"`
	Karakeep      KarakeepConfig               `yaml:"karakeep"`
	YouTube       YouTubeConfig                `yaml:"youtube"`
	Cast2MD       Cast2MDConfig                `yaml:"cast2md"`
}

type SearchConfig struct {
	DefaultThreshold float64 `yaml:"default_threshold"`
	DefaultLimit     int     `yaml:"default_limit"`
	ScoreCutoffRatio float64 `yaml:"score_cutoff_ratio"`
}

type SummaryConfig struct {
	Provider     string `yaml:"provider"`      // "claude" or "mistral"
	ClaudeModel  string `yaml:"claude_model"`  // e.g. "claude-sonnet-4-6-latest"
	MistralModel string `yaml:"mistral_model"` // e.g. "mistral-large-latest"
	DefaultLevel string `yaml:"default_level"` // "medium" or "deep"
}

type KarakeepConfig struct {
	BaseURL string `yaml:"base_url"`
}

type YouTubeConfig struct {
	SubLangs []string `yaml:"sub_langs"` // preferred subtitle languages
}

// Cast2MDConfig configures the podcast source. cast2md is reachable over the
// tailnet and has no authentication, so there is no secret here.
type Cast2MDConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"` // e.g. "https://cast2md.coydog-fence.ts.net"
	// PollIntervalMinutes is an int rather than a time.Duration because
	// meltkit decodes with yaml.v3, which does not turn "15m" into a Duration.
	PollIntervalMinutes int `yaml:"poll_interval_minutes"`
	MaxPerPoll          int `yaml:"max_per_poll"`
	TimeoutSeconds      int `yaml:"timeout_seconds"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		HealthAddr: "127.0.0.1:8081",
		Search: SearchConfig{
			DefaultThreshold: 0.5,
			DefaultLimit:     10,
			ScoreCutoffRatio: 0.5,
		},
		Summary: SummaryConfig{
			Provider:     "claude",
			ClaudeModel:  "",
			MistralModel: "",
			DefaultLevel: "medium",
		},
		YouTube: YouTubeConfig{
			SubLangs: []string{"en", "de"},
		},
		Cast2MD: Cast2MDConfig{
			Enabled:             false,
			BaseURL:             "",
			PollIntervalMinutes: 15,
			MaxPerPoll:          25,
			TimeoutSeconds:      60,
		},
		Tailscale: mkconfig.TailscaleConfig{
			Enabled:  true,
			Hostname: "vimmary",
			StateDir: "tsnet-state",
		},
	}

	if err := mkconfig.Load(path, cfg); err != nil {
		return nil, err
	}

	mkconfig.ApplyEnvOverrides(&cfg.Server, &cfg.Database, &cfg.Tailscale, "VIMMARY")

	if err := cfg.Database.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}
	if !cfg.Tailscale.Enabled && cfg.Server.Port == 0 {
		return nil, fmt.Errorf("config validation: server.port is required when tailscale is disabled")
	}
	if cfg.Cast2MD.Enabled && cfg.Cast2MD.BaseURL == "" {
		return nil, fmt.Errorf("config validation: cast2md.base_url is required when cast2md is enabled")
	}

	return cfg, nil
}
