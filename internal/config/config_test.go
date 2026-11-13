package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Create a minimal valid config file
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
database:
  host: localhost
  port: 5432
  name: vimmary
  user: vimmary
  password: test
tailscale:
  enabled: false
server:
  port: 8080
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Check defaults
	if cfg.Search.DefaultThreshold != 0.5 {
		t.Errorf("SearchConfig.DefaultThreshold = %f, want 0.5", cfg.Search.DefaultThreshold)
	}
	if cfg.Search.DefaultLimit != 10 {
		t.Errorf("SearchConfig.DefaultLimit = %d, want 10", cfg.Search.DefaultLimit)
	}
	if cfg.Search.ScoreCutoffRatio != 0.5 {
		t.Errorf("SearchConfig.ScoreCutoffRatio = %f, want 0.5", cfg.Search.ScoreCutoffRatio)
	}
	if cfg.Summary.Provider != "claude" {
		t.Errorf("SummaryConfig.Provider = %q, want %q", cfg.Summary.Provider, "claude")
	}
	if cfg.Summary.DefaultLevel != "medium" {
		t.Errorf("SummaryConfig.DefaultLevel = %q, want %q", cfg.Summary.DefaultLevel, "medium")
	}
	if len(cfg.YouTube.SubLangs) != 2 || cfg.YouTube.SubLangs[0] != "en" {
		t.Errorf("YouTube.SubLangs = %v, want [en de]", cfg.YouTube.SubLangs)
	}
}

func TestLoad_OverrideDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
database:
  host: localhost
  port: 5432
  name: vimmary
  user: vimmary
  password: test
tailscale:
  enabled: false
server:
  port: 8080
search:
  default_threshold: 0.5
  default_limit: 20
summary:
  provider: mistral
  default_level: deep
youtube:
  sub_langs: [fr, es]
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Search.DefaultThreshold != 0.5 {
		t.Errorf("SearchConfig.DefaultThreshold = %f, want 0.5", cfg.Search.DefaultThreshold)
	}
	if cfg.Search.DefaultLimit != 20 {
		t.Errorf("SearchConfig.DefaultLimit = %d, want 20", cfg.Search.DefaultLimit)
	}
	if cfg.Summary.Provider != "mistral" {
		t.Errorf("SummaryConfig.Provider = %q, want %q", cfg.Summary.Provider, "mistral")
	}
	if cfg.Summary.DefaultLevel != "deep" {
		t.Errorf("SummaryConfig.DefaultLevel = %q, want %q", cfg.Summary.DefaultLevel, "deep")
	}
	if len(cfg.YouTube.SubLangs) != 2 || cfg.YouTube.SubLangs[0] != "fr" {
		t.Errorf("YouTube.SubLangs = %v, want [fr es]", cfg.YouTube.SubLangs)
	}
}

func TestLoad_MissingDB(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
tailscale:
  enabled: false
server:
  port: 8080
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgFile)
	if err == nil {
		t.Error("Load() should fail with missing database config")
	}
}

func TestLoad_TailscaleDisabledNoPort(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
database:
  host: localhost
  port: 5432
  name: vimmary
  user: vimmary
  password: test
tailscale:
  enabled: false
`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgFile)
	if err == nil {
		t.Error("Load() should fail when tailscale disabled and no port specified")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() should fail for nonexistent file")
	}
}

func TestLoad_Embedding(t *testing.T) {
	base := `
database:
  host: localhost
  port: 5432
  name: vimmary
  user: vimmary
  password: test
tailscale:
  enabled: false
server:
  port: 8080
`
	cases := []struct {
		name    string
		extra   string
		wantErr bool
		want    string
	}{
		{name: "default is mistral", extra: "", want: "mistral"},
		{name: "openai through aperture", extra: "aperture:\n  base_url: http://ai.example.ts.net\nembedding:\n  provider: openai\n", want: "openai"},
		{name: "openai without aperture", extra: "embedding:\n  provider: openai\n", wantErr: true},
		{name: "unknown provider", extra: "embedding:\n  provider: voyage\n", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgFile := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(cfgFile, []byte(base+tc.extra), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(cfgFile)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want a validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Embedding.Provider != tc.want {
				t.Errorf("Embedding.Provider = %q, want %q", cfg.Embedding.Provider, tc.want)
			}
		})
	}
}

func TestLoad_Auth(t *testing.T) {
	base := `
database:
  host: localhost
  port: 5432
  name: vimmary
  user: vimmary
tailscale:
  enabled: false
server:
  port: 8080
`
	load := func(t *testing.T, extra string) (*Config, error) {
		t.Helper()
		f := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(f, []byte(base+extra), 0644); err != nil {
			t.Fatal(err)
		}
		return Load(f)
	}

	cfg, err := load(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth != nil {
		t.Errorf("Auth = %+v without an auth block, want nil", cfg.Auth)
	}

	cfg, err = load(t, `
auth:
  base_url: http://localhost:8080
  methods: [password, oidc]
  session_ttl: 48h
  initial_admin: root
  oidc:
    issuer: https://idp.example.com
    client_id: vimmary
    link_by_email: true
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth == nil || cfg.Auth.SessionTTL.Hours() != 48 || !cfg.Auth.OIDC.LinkByEmail || cfg.Auth.InitialAdmin != "root" {
		t.Errorf("Auth = %+v", cfg.Auth)
	}
	if cfg.Auth.TokenPrefix != "vimmary" {
		t.Errorf("TokenPrefix = %q, want the default vimmary", cfg.Auth.TokenPrefix)
	}

	cfg, err = load(t, "auth:\n  base_url: http://localhost:8080\n  methods: [password]\n  token_prefix: vim2\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.TokenPrefix != "vim2" {
		t.Errorf("TokenPrefix = %q, want vim2", cfg.Auth.TokenPrefix)
	}
	if _, err := load(t, "auth:\n  base_url: http://localhost:8080\n  methods: [password]\n  token_prefix: Bad-Prefix\n"); err == nil {
		t.Error("invalid token_prefix accepted")
	}

	if _, err := load(t, "auth:\n  base_url: http://vimmary.example.com\n  methods: [password]\n"); err == nil {
		t.Error("http base_url on a non-loopback host accepted")
	}
	if _, err := load(t, "auth:\n  base_url: https://vimmary.example.com\n  methods: [oidc]\n"); err == nil {
		t.Error("oidc without issuer accepted")
	}
}
