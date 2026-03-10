package summary

import "strings"

const mediumPrompt = `You are a video summary assistant. Summarize the following video transcript.

Video title: {{TITLE}}

{{LANGUAGE}}

Create a summary with:
- 3-5 paragraphs covering the main content
- Key points as a bullet list
- Action items or takeaways (if applicable)
- Topic tags (3-7 lowercase tags)

Formatting: Use **bold** for emphasis. Do not use *italic*.

Return ONLY valid JSON with these fields:
{
  "text": "The summary text in markdown format",
  "topics": ["tag1", "tag2"],
  "key_points": ["point 1", "point 2"],
  "action_items": ["item 1"]
}

Transcript:
{{TRANSCRIPT}}`

const deepPrompt = `You are a video summary assistant. Create a detailed, chapter-by-chapter summary of the following video transcript.

Video title: {{TITLE}}

{{LANGUAGE}}

Create a comprehensive summary with:
- Chapter-by-chapter breakdown with headers
- Key quotes where relevant (use blockquotes)
- Detailed key points
- Specific action items and takeaways
- Topic tags (5-10 lowercase tags)

Formatting: Use **bold** for emphasis. Do not use *italic*.

Return ONLY valid JSON with these fields:
{
  "text": "The detailed summary in markdown format with ## chapter headers",
  "topics": ["tag1", "tag2"],
  "key_points": ["point 1", "point 2"],
  "action_items": ["item 1"]
}

Transcript:
{{TRANSCRIPT}}`

func promptForLevel(level string) string {
	if level == "deep" {
		return deepPrompt
	}
	return mediumPrompt
}

// DefaultPrompt returns the default prompt template for a given level.
func DefaultPrompt(level string) string {
	return promptForLevel(level)
}

// BuildPrompt replaces named placeholders in a prompt template with actual values.
func BuildPrompt(template, title, language, transcript string) string {
	r := strings.NewReplacer(
		"{{TITLE}}", title,
		"{{LANGUAGE}}", languageInstruction(language),
		"{{TRANSCRIPT}}", transcript,
	)
	return r.Replace(template)
}

func languageInstruction(lang string) string {
	// Normalize "de-DE" → "de", "en-US" → "en", etc.
	base, _, _ := strings.Cut(strings.ToLower(lang), "-")
	switch base {
	case "en", "":
		return "Write the entire summary in English."
	case "de":
		return "Write the entire summary in German (Deutsch)."
	case "fr":
		return "Write the entire summary in French (Français)."
	case "es":
		return "Write the entire summary in Spanish (Español)."
	default:
		return "Write the entire summary in the same language as the transcript."
	}
}
