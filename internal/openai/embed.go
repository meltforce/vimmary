// Package openai calls OpenAI's embeddings endpoint through the Aperture
// gateway's direct-access path. It holds no API key: Aperture authorizes the
// request by the caller's tailnet identity and substitutes the key it holds.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	embedModel = "text-embedding-3-large"
	// embedDimensions matches the vector(1024) column and its HNSW index.
	// text-embedding-3 models are trained to be shortened, and pgvector's HNSW
	// index accepts at most 2,000 dimensions for the vector type, so the full
	// 3,072 would need a halfvec column and a rebuilt index for no measured gain.
	embedDimensions = 1024
)

// Embedder produces text-embedding-3-large vectors at 1024 dimensions.
type Embedder struct {
	url  string
	http *http.Client
}

// NewEmbedder returns an embedder that posts to the gateway at gatewayURL.
// The client must be able to reach the gateway, which for a tsnet node means
// one that dials over the node.
func NewEmbedder(gatewayURL string, client *http.Client) *Embedder {
	return &Embedder{
		url:  strings.TrimRight(gatewayURL, "/") + "/direct/openai/v1/embeddings",
		http: client,
	}
}

// Model identifies the vector space. Stored per row, it is what keeps search
// from comparing vectors of different models.
func (e *Embedder) Model() string {
	return fmt.Sprintf("%s@%d", embedModel, embedDimensions)
}

// Embed returns the embedding for text.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model":      embedModel,
		"input":      text,
		"dimensions": embedDimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("embed: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embed: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("embed: parse response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) != embedDimensions {
		return nil, fmt.Errorf("embed: expected one %d-dimensional vector", embedDimensions)
	}
	return result.Data[0].Embedding, nil
}
