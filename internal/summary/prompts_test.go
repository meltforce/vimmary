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

func TestDefaultPrompt(t *testing.T) {
	medium := DefaultPrompt("medium")
	deep := DefaultPrompt("deep")

	if !strings.Contains(medium, "3-5 paragraphs") {
		t.Error("DefaultPrompt(medium) should contain '3-5 paragraphs'")
	}
	if !strings.Contains(deep, "chapter-by-chapter") {
		t.Error("DefaultPrompt(deep) should contain 'chapter-by-chapter'")
	}
}

func TestBuildPrompt(t *testing.T) {
	template := "Title: {{TITLE}}\n{{LANGUAGE}}\nTranscript: {{TRANSCRIPT}}"
	result := BuildPrompt(template, "My Video", "en", "Hello world")

	if !strings.Contains(result, "Title: My Video") {
		t.Error("BuildPrompt should replace {{TITLE}}")
	}
	if !strings.Contains(result, "English") {
		t.Error("BuildPrompt should replace {{LANGUAGE}} with language instruction")
	}
	if !strings.Contains(result, "Transcript: Hello world") {
		t.Error("BuildPrompt should replace {{TRANSCRIPT}}")
	}
	if strings.Contains(result, "{{") {
		t.Error("BuildPrompt should not leave any placeholders")
	}
}

func TestBuildPrompt_CustomTemplate(t *testing.T) {
	custom := "Summarize {{TITLE}} in {{LANGUAGE}} from: {{TRANSCRIPT}}"
	result := BuildPrompt(custom, "Test", "de", "transcript text")

	if !strings.Contains(result, "Summarize Test") {
		t.Error("custom template should replace {{TITLE}}")
	}
	if !strings.Contains(result, "German") {
		t.Error("custom template should replace {{LANGUAGE}} with German instruction")
	}
	if !strings.Contains(result, "transcript text") {
		t.Error("custom template should replace {{TRANSCRIPT}}")
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
