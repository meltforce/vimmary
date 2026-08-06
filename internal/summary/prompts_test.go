package summary

import (
	"strings"
	"testing"
)

func TestPromptFor(t *testing.T) {
	tests := []struct {
		source   string
		level    string
		contains string
	}{
		{"youtube", "medium", "3-5 paragraphs"},
		{"youtube", "deep", "chapter-by-chapter"},
		{"youtube", "", "3-5 paragraphs"},        // default to medium
		{"youtube", "unknown", "3-5 paragraphs"}, // unknown defaults to medium
		{"podcast", "medium", "This is a conversation"},
		{"podcast", "deep", "segment-by-segment"},
		{"podcast", "", "This is a conversation"},
		{"", "medium", "3-5 paragraphs"}, // unknown source falls back to video
		{"unknown", "deep", "chapter-by-chapter"},
	}

	for _, tt := range tests {
		t.Run(tt.source+"/"+tt.level, func(t *testing.T) {
			got := promptFor(tt.source, tt.level)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("promptFor(%q, %q) should contain %q", tt.source, tt.level, tt.contains)
			}
		})
	}
}

func TestDefaultPromptFor(t *testing.T) {
	if !strings.Contains(DefaultPromptFor("youtube", "medium"), "3-5 paragraphs") {
		t.Error("video medium prompt should contain '3-5 paragraphs'")
	}
	if !strings.Contains(DefaultPromptFor("youtube", "deep"), "chapter-by-chapter") {
		t.Error("video deep prompt should contain 'chapter-by-chapter'")
	}
	if !strings.Contains(DefaultPromptFor("podcast", "deep"), "## References") {
		t.Error("podcast deep prompt should ask for a References section")
	}
}

// Every built-in prompt has to carry all three placeholders, otherwise
// BuildPrompt silently drops the transcript or the language instruction.
func TestPromptPlaceholders(t *testing.T) {
	for _, source := range []string{"youtube", "podcast"} {
		for _, level := range []string{"medium", "deep"} {
			t.Run(source+"/"+level, func(t *testing.T) {
				tmpl := DefaultPromptFor(source, level)
				for _, ph := range []string{"{{TITLE}}", "{{LANGUAGE}}", "{{TRANSCRIPT}}"} {
					if !strings.Contains(tmpl, ph) {
						t.Errorf("prompt is missing %s", ph)
					}
				}
				built := BuildPrompt(tmpl, "Some Show — Episode 12", "de", "transcript body")
				if strings.Contains(built, "{{") {
					t.Error("BuildPrompt left a placeholder unreplaced")
				}
				if !strings.Contains(built, "Some Show — Episode 12") {
					t.Error("BuildPrompt did not substitute the title")
				}
				if !strings.Contains(built, "transcript body") {
					t.Error("BuildPrompt did not substitute the transcript")
				}
			})
		}
	}
}

// The podcast path passes LangSameAsTranscript because cast2md reports no
// language for an episode.
func TestLangSameAsTranscript(t *testing.T) {
	got := languageInstruction(LangSameAsTranscript)
	if !strings.Contains(got, "same language as the transcript") {
		t.Errorf("languageInstruction(%q) = %q", LangSameAsTranscript, got)
	}
}

func TestMaxOutputTokens(t *testing.T) {
	if got := maxOutputTokens("medium"); got != 4096 {
		t.Errorf("medium budget = %d, want 4096", got)
	}
	if got := maxOutputTokens("deep"); got != 16000 {
		t.Errorf("deep budget = %d, want 16000", got)
	}
	if got := maxOutputTokens(""); got != 4096 {
		t.Errorf("empty level budget = %d, want the medium budget 4096", got)
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
