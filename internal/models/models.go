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

// Registry discovers and caches available models from provider APIs.
type Registry struct {
	mu       sync.Mutex
	cache    map[string]*cachedModels
	cacheTTL time.Duration
	apiKeys  map[string]string
	http     *http.Client
	log      *slog.Logger
}

// NewRegistry creates a model registry with API keys for each provider.
func NewRegistry(claudeAPIKey, mistralAPIKey string, log *slog.Logger) *Registry {
	keys := make(map[string]string)
	if claudeAPIKey != "" {
		keys["claude"] = claudeAPIKey
	}
	if mistralAPIKey != "" {
		keys["mistral"] = mistralAPIKey
	}
	return &Registry{
		cache:    make(map[string]*cachedModels),
		cacheTTL: 5 * time.Minute,
		apiKeys:  keys,
		http:     &http.Client{Timeout: 10 * time.Second},
		log:      log,
	}
}

// ListModels returns available models for a provider, using a cached result if fresh.
func (r *Registry) ListModels(ctx context.Context, provider string) ([]Model, error) {
	if _, ok := r.apiKeys[provider]; !ok {
		return nil, nil
	}

	r.mu.Lock()
	cached, ok := r.cache[provider]
	if ok && time.Since(cached.fetchedAt) < r.cacheTTL {
		r.mu.Unlock()
		return cached.models, nil
	}
	r.mu.Unlock()

	models, err := r.fetchModels(ctx, provider)
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

// ListAllModels returns models from all configured providers, tagged with provider name.
func (r *Registry) ListAllModels(ctx context.Context) []Model {
	var all []Model
	for provider := range r.apiKeys {
		models, err := r.ListModels(ctx, provider)
		if err != nil {
			r.log.Warn("failed to list models for provider", "provider", provider, "error", err)
			continue
		}
		all = append(all, models...)
	}
	return all
}

func (r *Registry) fetchModels(ctx context.Context, provider string) ([]Model, error) {
	switch provider {
	case "claude":
		return r.fetchClaudeModels(ctx)
	case "mistral":
		return r.fetchMistralModels(ctx)
	default:
		return nil, fmt.Errorf("unknown provider: %q", provider)
	}
}

func (r *Registry) fetchClaudeModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/models?limit=100", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", r.apiKeys["claude"])
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

func (r *Registry) fetchMistralModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.mistral.ai/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKeys["mistral"])

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

