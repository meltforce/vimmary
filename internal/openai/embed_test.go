package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbed_DirectPathAndDimensions(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		vec := make([]float32, embedDimensions)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": vec}}})
	}))
	defer srv.Close()

	e := NewEmbedder(srv.URL+"/", srv.Client())
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 1024 {
		t.Errorf("len = %d, want 1024", len(vec))
	}
	if gotPath != "/direct/openai/v1/embeddings" {
		t.Errorf("path = %q", gotPath)
	}
	// The gateway substitutes its own key; sending one would only leak a
	// placeholder into its logs.
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want none", gotAuth)
	}
	if gotBody["model"] != "text-embedding-3-large" || gotBody["dimensions"] != float64(1024) {
		t.Errorf("body = %v", gotBody)
	}
	if e.Model() != "text-embedding-3-large@1024" {
		t.Errorf("Model() = %q", e.Model())
	}
}

// A vector of the wrong length would be rejected by the vector(1024) column
// only at write time, after the summary call was paid for.
func TestEmbed_RejectsWrongDimension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": make([]float32, 3072)}}})
	}))
	defer srv.Close()

	if _, err := NewEmbedder(srv.URL, srv.Client()).Embed(context.Background(), "x"); err == nil {
		t.Fatal("want an error for a 3072-dimensional vector")
	}
}
