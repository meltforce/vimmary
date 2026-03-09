package config

import (
	"fmt"

	mkconfig "github.com/meltforce/meltkit/pkg/config"
)

type Config struct {
	ExternalURL   string                       `yaml:"external_url"`
	Server        mkconfig.ServerConfig        `yaml:"server"`
	Database      mkconfig.DatabaseConfig      `yaml:"database"`
	Tailscale     mkconfig.TailscaleConfig     `yaml:"tailscale"`
	SecretBackend mkconfig.SecretBackendConfig `yaml:"secret_backend"`
	Secrets       map[string]string            `yaml:"secrets"`
	Search        SearchConfig                 `yaml:"search"`
	Summary       SummaryConfig                `yaml:"summary"`
	Karakeep      KarakeepConfig               `yaml:"karakeep"`
	YouTube       YouTubeConfig                `yaml:"youtube"`
}

type SearchConfig struct {
	DefaultThreshold float64 `yaml:"default_threshold"`
	DefaultLimit     int     `yaml:"default_limit"`
}

type SummaryConfig struct {
	Provider     string `yaml:"provider"`      // "claude" or "mistral"
	ClaudeModel  string `yaml:"claude_model"`  // e.g. "claude-sonnet-4-5-20250514"
	MistralModel string `yaml:"mistral_model"` // e.g. "mistral-large-latest"
	DefaultLevel string `yaml:"default_level"` // "medium" or "deep"
}

type KarakeepConfig struct {
	BaseURL string `yaml:"base_url"`
}

type YouTubeConfig struct {
	SubLangs []string `yaml:"sub_langs"` // preferred subtitle languages
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Search: SearchConfig{
			DefaultThreshold: 0.3,
			DefaultLimit:     10,
		},
		Summary: SummaryConfig{
			Provider:     "claude",
			ClaudeModel:  "claude-sonnet-4-5-20250514",
			MistralModel: "mistral-large-latest",
			DefaultLevel: "medium",
		},
		YouTube: YouTubeConfig{
			SubLangs: []string{"en", "de"},
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

	return cfg, nil
}
