package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Model represents an available LLM model.
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
}

type cachedModels struct {
	models    []Model
	fetchedAt time.Time
}

// Providers the registry knows how to query. It replaces iterating over the
// configured keys: keys now come from the database and can appear or disappear
// while the process runs, so the set of *possible* providers has to be stated
// rather than derived from what happened to be configured at startup.
var Providers = []string{"claude", "mistral"}

// KeyFunc supplies a provider's API key at call time. Keys are service-wide
// settings maintained in the Settings page, so they cannot be captured once at
// construction. An unconfigured provider yields an empty string, not an error.
type KeyFunc func(ctx context.Context, provider string) (string, error)

// Registry discovers and caches available models from provider APIs.
type Registry struct {
	mu       sync.Mutex
	cache    map[string]*cachedModels
	cacheTTL time.Duration
	keyFor   KeyFunc
	http     *http.Client
	log      *slog.Logger
}

// NewRegistry creates a model registry that resolves API keys per call.
func NewRegistry(keyFor KeyFunc, log *slog.Logger) *Registry {
	return &Registry{
		cache:    make(map[string]*cachedModels),
		cacheTTL: 5 * time.Minute,
		keyFor:   keyFor,
		http:     &http.Client{Timeout: 10 * time.Second},
		log:      log,
	}
}

// ListModels returns available models for a provider, using a cached result if
// fresh. A provider without a configured key returns nothing and no error —
// that is how it stays out of the Settings page's model list.
func (r *Registry) ListModels(ctx context.Context, provider string) ([]Model, error) {
	apiKey, err := r.keyFor(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("api key for %q: %w", provider, err)
	}
	if apiKey == "" {
		return nil, nil
	}

	r.mu.Lock()
	cached, ok := r.cache[provider]
	if ok && time.Since(cached.fetchedAt) < r.cacheTTL {
		r.mu.Unlock()
		return cached.models, nil
	}
	r.mu.Unlock()

	models, err := r.fetchModels(ctx, provider, apiKey)
	if err != nil {
		// Return stale cache on error
		if cached != nil {
			r.log.Warn("model fetch failed, using stale cache", "provider", provider, "error", err)
			return cached.models, nil
		}
		return nil, err
	}

	r.mu.Lock()
	r.cache[provider] = &cachedModels{models: models, fetchedAt: time.Now()}
	r.mu.Unlock()

	return models, nil
}

// ListAllModels returns models from every provider that has a key configured,
// tagged with the provider name. Providers without a key contribute nothing.
func (r *Registry) ListAllModels(ctx context.Context) []Model {
	var all []Model
	for _, provider := range Providers {
		models, err := r.ListModels(ctx, provider)
		if err != nil {
			r.log.Warn("failed to list models for provider", "provider", provider, "error", err)
			continue
		}
		all = append(all, models...)
	}
	return all
}

func (r *Registry) fetchModels(ctx context.Context, provider, apiKey string) ([]Model, error) {
	switch provider {
	case "claude":
		return r.fetchClaudeModels(ctx, apiKey)
	case "mistral":
		return r.fetchMistralModels(ctx, apiKey)
	default:
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
}

func (r *Registry) fetchClaudeModels(ctx context.Context, apiKey string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/models?limit=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude models API: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude models API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse claude models: %w", err)
	}

	var models []Model
	for _, m := range result.Data {
		models = append(models, Model{
			ID:          m.ID,
			DisplayName: m.DisplayName,
			Provider:    "claude",
		})
	}
	return models, nil
}

func (r *Registry) fetchMistralModels(ctx context.Context, apiKey string) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.mistral.ai/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mistral models API: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mistral models API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			ID           string   `json:"id"`
			Capabilities struct {
				CompletionChat bool `json:"completion_chat"`
			} `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse mistral models: %w", err)
	}

	// Only keep key "latest" models from Mistral
	wantPrefixes := []string{
		"mistral-tiny-latest",
		"mistral-small-latest",
		"mistral-medium-latest",
		"mistral-large-latest",
		"codestral-latest",
	}

	var models []Model
	for _, m := range result.Data {
		if !m.Capabilities.CompletionChat {
			continue
		}
		matched := false
		for _, prefix := range wantPrefixes {
			if m.ID == prefix {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		models = append(models, Model{
			ID:          m.ID,
			DisplayName: m.ID,
			Provider:    "mistral",
		})
	}
	return models, nil
}

