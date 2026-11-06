package summary

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// claudeServer answers every request with body and records the last request.
func claudeServer(t *testing.T, body string) (*httptest.Server, *map[string]any, *http.Header) {
	t.Helper()
	var gotBody map[string]any
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &gotBody, &gotHeader
}

// The 5.5 models put a thinking block first, with empty text by default. The
// summary is in the text block after it; reading content[0] stored an empty
// summary as completed.
func TestClaudeSummarize_ReadsTextBlocksAfterThinking(t *testing.T) {
	srv, _, _ := claudeServer(t, `{
		"stop_reason": "end_turn",
		"content": [
			{"type": "thinking", "thinking": ""},
			{"type": "text", "text": "{\"text\": \"the summary\", \"topics\": [\"go\"]}"}
		],
		"usage": {"input_tokens": 10, "output_tokens": 20}
	}`)
	c := NewClaudeSummarizer("k", srv.URL, nil)

	sum, err := c.Summarize(context.Background(), Request{Source: "youtube", Level: "medium", Model: "claude-sonnet-5-5"})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if sum.Text != "the summary" {
		t.Errorf("Text = %q, want %q", sum.Text, "the summary")
	}
	if sum.Usage.OutputTokens != 20 {
		t.Errorf("OutputTokens = %d, want 20", sum.Usage.OutputTokens)
	}
}

func TestClaudeSummarize_RefusalIsAnError(t *testing.T) {
	srv, _, _ := claudeServer(t, `{
		"stop_reason": "refusal",
		"stop_details": {"type": "refusal", "category": "cyber", "explanation": "declined"},
		"content": [],
		"usage": {"input_tokens": 10, "output_tokens": 0}
	}`)
	c := NewClaudeSummarizer("k", srv.URL, nil)

	_, err := c.Summarize(context.Background(), Request{Source: "youtube", Level: "medium"})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	if !strings.Contains(err.Error(), "cyber") {
		t.Errorf("err = %v, want the category in the message", err)
	}
}

// A response cut off at max_tokens can end after a complete-looking JSON
// object in the thinking-free case, so the stop reason is checked rather than
// left to the JSON parser.
func TestClaudeSummarize_MaxTokensIsAnError(t *testing.T) {
	srv, _, _ := claudeServer(t, `{
		"stop_reason": "max_tokens",
		"content": [{"type": "text", "text": "{\"text\": \"cut"}],
		"usage": {"input_tokens": 10, "output_tokens": 16096}
	}`)
	c := NewClaudeSummarizer("k", srv.URL, nil)

	_, err := c.Summarize(context.Background(), Request{Source: "youtube", Level: "medium"})
	if !errors.Is(err, errTruncatedJSON) {
		t.Fatalf("err = %v, want errTruncatedJSON", err)
	}
}

func TestClaudeRequest_BudgetModelAndFallbacks(t *testing.T) {
	ok := `{"stop_reason":"end_turn","content":[{"type":"text","text":"{\"text\":\"x\"}"}],"usage":{}}`

	t.Run("default model gets the fallback and the thinking headroom", func(t *testing.T) {
		srv, body, header := claudeServer(t, ok)
		c := NewClaudeSummarizer("k", srv.URL, nil)
		if _, err := c.Summarize(context.Background(), Request{Source: "podcast", Level: "medium"}); err != nil {
			t.Fatalf("Summarize: %v", err)
		}
		if got := (*body)["model"]; got != DefaultClaudeModel {
			t.Errorf("model = %v, want %q", got, DefaultClaudeModel)
		}
		if got := (*body)["max_tokens"]; got != float64(4096+claudeThinkingHeadroom) {
			t.Errorf("max_tokens = %v, want %d", got, 4096+claudeThinkingHeadroom)
		}
		if got := (*body)["fallbacks"]; got != "default" {
			t.Errorf("fallbacks = %v, want \"default\"", got)
		}
		if got := header.Get("anthropic-beta"); got != "server-side-fallback-2026-07-01" {
			t.Errorf("anthropic-beta = %q", got)
		}
	})

	// Sending `fallbacks` to a model that does not accept it is a 400.
	t.Run("haiku gets no fallback", func(t *testing.T) {
		srv, body, header := claudeServer(t, ok)
		c := NewClaudeSummarizer("k", srv.URL, nil)
		if _, err := c.Summarize(context.Background(), Request{Source: "youtube", Level: "deep", Model: "claude-haiku-5-5"}); err != nil {
			t.Fatalf("Summarize: %v", err)
		}
		if _, present := (*body)["fallbacks"]; present {
			t.Error("fallbacks sent to claude-haiku-5-5")
		}
		if header.Get("anthropic-beta") != "" {
			t.Error("anthropic-beta sent to claude-haiku-5-5")
		}
		if got := (*body)["max_tokens"]; got != float64(16000+claudeThinkingHeadroom) {
			t.Errorf("max_tokens = %v, want %d", got, 16000+claudeThinkingHeadroom)
		}
	})
}
