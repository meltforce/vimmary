package summary

import (
	"strings"
	"testing"
)

func TestPromptForLevel(t *testing.T) {
	tests := []struct {
		level    string
		contains string
	}{
		{"medium", "3-5 paragraphs"},
		{"deep", "chapter-by-chapter"},
		{"", "3-5 paragraphs"},       // default to medium
		{"unknown", "3-5 paragraphs"}, // unknown defaults to medium
	}

	for _, tt := range tests {
		t.Run("level="+tt.level, func(t *testing.T) {
			got := promptForLevel(tt.level)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("promptForLevel(%q) should contain %q", tt.level, tt.contains)
			}
		})
	}
}

func TestLanguageInstruction(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"en", "English"},
		{"EN", "English"},
		{"en-US", "English"},
		{"de", "German"},
		{"de-DE", "German"},
		{"fr", "French"},
		{"es", "Spanish"},
		{"", "English"},
		{"ja", "same language as the transcript"},
		{"zh", "same language as the transcript"},
	}

	for _, tt := range tests {
		t.Run("lang="+tt.lang, func(t *testing.T) {
			got := languageInstruction(tt.lang)
			if !strings.Contains(got, tt.want) {
				t.Errorf("languageInstruction(%q) = %q, want to contain %q", tt.lang, got, tt.want)
			}
		})
	}
}
