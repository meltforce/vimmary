package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/meltforce/vimmary/internal/config"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/summary"
)

// fakeSettings stands in for storage.DB on the summarizer path. Only the two
// methods getSummarizer needs are implemented; that is the whole point of the
// settingsSource interface.
type fakeSettings struct {
	keys map[string]string // provider -> API key
	app  map[string]string // app_settings key -> value
	err  error             // returned by both methods when set
}

func (f *fakeSettings) GetLLMKey(_ context.Context, provider string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.keys[provider], nil
}

func (f *fakeSettings) GetAppSetting(_ context.Context, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.app[key], nil
}

// stubSummarizer records which provider and key it was built from, so a test
// can assert what getSummarizer resolved without calling a provider API.
type stubSummarizer struct {
	provider string
	apiKey   string
}

func (s *stubSummarizer) Summarize(_ context.Context, _ summary.Request) (*summary.Summary, error) {
	return &summary.Summary{Text: "stub"}, nil
}

func (s *stubSummarizer) Complete(_ context.Context, _ string, _ int) (string, error) {
	return "{}", nil
}

// newTestService builds a Service with both seams replaced and no database.
// configProvider is what config.yaml would supply as the initial provider.
func newTestService(settings *fakeSettings, configProvider string) *Service {
	return &Service{
		settings: settings,
		newSummarizer: func(provider, apiKey string) (summary.Summarizer, error) {
			return &stubSummarizer{provider: provider, apiKey: apiKey}, nil
		},
		summaryCfg: config.SummaryConfig{Provider: configProvider, DefaultLevel: "medium"},
		log:        slog.Default(),
	}
}

func TestGetSummarizer_UsesTheRequestedProvider(t *testing.T) {
	svc := newTestService(&fakeSettings{
		keys: map[string]string{"mistral": "mistral-key", "claude": "claude-key"},
	}, "mistral")

	sum, provider, err := svc.getSummarizer(context.Background(), "claude")
	if err != nil {
		t.Fatalf("getSummarizer: %v", err)
	}
	if provider != "claude" {
		t.Errorf("provider = %q, want %q", provider, "claude")
	}
	stub, ok := sum.(*stubSummarizer)
	if !ok {
		t.Fatalf("summarizer type = %T, want *stubSummarizer", sum)
	}
	// The key handed to the factory has to be the requested provider's, not the
	// default provider's — mixing those would authenticate against the wrong API.
	if stub.apiKey != "claude-key" {
		t.Errorf("apiKey = %q, want %q", stub.apiKey, "claude-key")
	}
}

// An empty provider means "whatever is configured". The stored value wins over
// the config file, which is what makes the Settings page effective.
func TestGetSummarizer_EmptyProviderPrefersTheStoredValue(t *testing.T) {
	svc := newTestService(&fakeSettings{
		keys: map[string]string{"mistral": "mistral-key", "claude": "claude-key"},
		app:  map[string]string{storage.SettingSummaryProvider: "mistral"},
	}, "claude")

	_, provider, err := svc.getSummarizer(context.Background(), "")
	if err != nil {
		t.Fatalf("getSummarizer: %v", err)
	}
	if provider != "mistral" {
		t.Errorf("provider = %q, want the stored %q, not the config %q", provider, "mistral", "claude")
	}
}

// With nothing stored, the config file is the initial value — an unattended
// deployment has to summarize before anyone opens the Settings page.
func TestGetSummarizer_EmptyProviderUsesConfigWhenNothingStored(t *testing.T) {
	svc := newTestService(&fakeSettings{
		keys: map[string]string{"claude": "claude-key"},
	}, "claude")

	_, provider, err := svc.getSummarizer(context.Background(), "")
	if err != nil {
		t.Fatalf("getSummarizer: %v", err)
	}
	if provider != "claude" {
		t.Errorf("provider = %q, want %q", provider, "claude")
	}
}

// The message a user sees when no key is configured. This is the path that
// replaced the startup gate: a missing key must fail the summary and say why,
// not stop the process.
func TestGetSummarizer_MissingKeyNamesTheProviderAndTheRemedy(t *testing.T) {
	svc := newTestService(&fakeSettings{keys: map[string]string{}}, "mistral")

	_, _, err := svc.getSummarizer(context.Background(), "mistral")
	if err == nil {
		t.Fatal("getSummarizer with no key: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "mistral") {
		t.Errorf("error %q does not name the provider", err)
	}
	if !strings.Contains(err.Error(), "Settings") {
		t.Errorf("error %q does not say where to fix it", err)
	}
}

func TestGetSummarizer_PropagatesASettingsFailure(t *testing.T) {
	sentinel := errors.New("database is down")
	svc := newTestService(&fakeSettings{err: sentinel}, "mistral")

	if _, _, err := svc.getSummarizer(context.Background(), "mistral"); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want it to wrap %v", err, sentinel)
	}
}

// The real factory, not the stub: it guards the mapping from a provider name to
// a concrete summarizer, which models.Providers has to agree with.
func TestNewSummarizer_MapsProviderNamesToImplementations(t *testing.T) {
	claude, err := newSummarizer("claude", "k")
	if err != nil {
		t.Fatalf("newSummarizer(claude): %v", err)
	}
	if _, ok := claude.(*summary.ClaudeSummarizer); !ok {
		t.Errorf("claude -> %T, want *summary.ClaudeSummarizer", claude)
	}

	mistral, err := newSummarizer("mistral", "k")
	if err != nil {
		t.Fatalf("newSummarizer(mistral): %v", err)
	}
	if _, ok := mistral.(*summary.MistralSummarizer); !ok {
		t.Errorf("mistral -> %T, want *summary.MistralSummarizer", mistral)
	}

	if _, err := newSummarizer("gpt", "k"); err == nil {
		t.Error("newSummarizer(gpt): want an error for an unknown provider")
	}
}

func TestAvailableProviders_ReportsOnlyConfiguredOnes(t *testing.T) {
	svc := newTestService(&fakeSettings{
		keys: map[string]string{"mistral": "mistral-key"},
	}, "mistral")

	got := svc.AvailableProviders(context.Background())
	if len(got) != 1 || got[0] != "mistral" {
		t.Errorf("AvailableProviders = %v, want [mistral]", got)
	}
}

// GetLLMSettings feeds the Settings page. It must report whether a key is set
// and never the key itself — the struct has no field for one, and this test
// exists so adding one is a deliberate act rather than an accident.
func TestGetLLMSettings_ReportsPresenceNotTheKeys(t *testing.T) {
	svc := newTestService(&fakeSettings{
		keys: map[string]string{"mistral": "mistral-key"},
		app:  map[string]string{storage.SettingSummaryProvider: "mistral"},
	}, "claude")

	got, err := svc.GetLLMSettings(context.Background())
	if err != nil {
		t.Fatalf("GetLLMSettings: %v", err)
	}
	if !got.MistralConfigured {
		t.Error("MistralConfigured = false, want true")
	}
	if got.AnthropicConfigured {
		t.Error("AnthropicConfigured = true, want false")
	}
	if got.Provider != "mistral" {
		t.Errorf("Provider = %q, want %q", got.Provider, "mistral")
	}
}
