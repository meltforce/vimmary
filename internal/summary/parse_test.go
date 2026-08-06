package summary

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSummaryJSON_Complete(t *testing.T) {
	sum, err := parseSummaryJSON(`{"text":"Body","topics":["a"],"key_points":["k"],"action_items":[]}`)
	if err != nil {
		t.Fatalf("parseSummaryJSON: %v", err)
	}
	if sum.Text != "Body" {
		t.Errorf("Text = %q, want %q", sum.Text, "Body")
	}
	if len(sum.Topics) != 1 || sum.Topics[0] != "a" {
		t.Errorf("Topics = %v", sum.Topics)
	}
	if sum.ActionItems == nil {
		t.Error("ActionItems should be an empty slice, not nil")
	}
}

func TestParseSummaryJSON_WrappedInCodeFence(t *testing.T) {
	sum, err := parseSummaryJSON("Here you go:\n```json\n{\"text\":\"Body\"}\n```\n")
	if err != nil {
		t.Fatalf("parseSummaryJSON: %v", err)
	}
	if sum.Text != "Body" {
		t.Errorf("Text = %q, want %q", sum.Text, "Body")
	}
}

// A response that hits the output token limit ends inside the JSON object. It
// has to surface as an error — treating it as raw text would store a cut-off
// summary as if it were complete.
func TestParseSummaryJSON_Truncated(t *testing.T) {
	truncated := `{"text":"A long summary that stops mid-sen`
	if _, err := parseSummaryJSON(truncated); !errors.Is(err, errTruncatedJSON) {
		t.Fatalf("parseSummaryJSON(truncated) error = %v, want errTruncatedJSON", err)
	}
}

func TestParseSummaryJSON_NestedObjectsAreNotTruncated(t *testing.T) {
	nested := `{"text":"Body","extra":{"a":{"b":1}},"topics":["x"]}`
	sum, err := parseSummaryJSON(nested)
	if err != nil {
		t.Fatalf("parseSummaryJSON: %v", err)
	}
	if sum.Text != "Body" {
		t.Errorf("Text = %q", sum.Text)
	}
}

// A response with no JSON at all is still usable as plain prose, so it keeps
// falling through to the raw-text path.
func TestParseSummaryJSON_NoJSON(t *testing.T) {
	sum, err := parseSummaryJSON("Just some prose without any object.")
	if err != nil {
		t.Fatalf("parseSummaryJSON: %v", err)
	}
	if !strings.Contains(sum.Text, "Just some prose") {
		t.Errorf("Text = %q", sum.Text)
	}
	if sum.Topics == nil || sum.KeyPoints == nil || sum.ActionItems == nil {
		t.Error("slices should be empty, not nil")
	}
}
