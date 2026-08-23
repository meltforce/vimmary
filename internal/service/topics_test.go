package service

import (
	"reflect"
	"testing"

	"github.com/meltforce/vimmary/internal/storage"
)

func topicCounts(names ...string) []storage.TopicCount {
	counts := make([]storage.TopicCount, len(names))
	for i, n := range names {
		counts[i] = storage.TopicCount{Topic: n, Count: 1}
	}
	return counts
}

func TestParseTopicMapping(t *testing.T) {
	mapping, err := parseTopicMapping("Here you go:\n```json\n{\"mapping\": {\"golang\": \"go\"}}\n```")
	if err != nil {
		t.Fatalf("parseTopicMapping: %v", err)
	}
	if mapping["golang"] != "go" {
		t.Errorf("mapping = %v", mapping)
	}

	if _, err := parseTopicMapping("no json here"); err == nil {
		t.Error("expected an error for an answer without JSON")
	}

	empty, err := parseTopicMapping(`{"mapping": {}}`)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty mapping = %v, err %v", empty, err)
	}
}

func TestSanitizeTopicMapping(t *testing.T) {
	counts := topicCounts("go", "golang", "go language", "testing", "ai")

	got := sanitizeTopicMapping(map[string]string{
		"golang":      "go",       // valid
		"go language": "golang",   // chain — flattens to go
		"testing":     "testing",  // identity — dropped
		"ai":          "invented", // target not in the list — dropped
		"unknown":     "go",       // source not in the list — dropped
		"":            "go",       // empty side — dropped
	}, counts)

	want := map[string]string{"golang": "go", "go language": "go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sanitized = %v, want %v", got, want)
	}
}

func TestSanitizeTopicMapping_CycleCollapses(t *testing.T) {
	counts := topicCounts("a", "b")
	got := sanitizeTopicMapping(map[string]string{"a": "b", "b": "a"}, counts)
	if len(got) != 0 {
		t.Errorf("a cycle must drop out entirely, got %v", got)
	}
}

func TestApplyTopicMapping(t *testing.T) {
	mapping := map[string]string{"golang": "go", "ml": "machine learning"}

	if got := applyTopicMapping([]string{"go", "testing"}, mapping); got != nil {
		t.Errorf("unchanged row returned %v, want nil", got)
	}

	got := applyTopicMapping([]string{"golang", "go", "ml"}, mapping)
	want := []string{"go", "machine learning"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rewritten = %v, want %v (merge dedupes, order kept)", got, want)
	}
}
