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

// The podcast prompts differ from the video ones in what they have to cope
// with: conversation rather than presentation, 30 minutes to 3 hours of it, no
// chapter marks, and speaker labels that cannot be relied on. The JSON shape is
// identical so parseSummaryJSON needs no special case.

const podcastMediumPrompt = `You are a podcast summary assistant. Summarize the following podcast episode transcript.

Episode: {{TITLE}}

{{LANGUAGE}}

This is a conversation, not a presentation. The transcript has no chapter marks
and any speaker labels in it may be wrong or missing. Work out from the content
who is speaking.

Skip entirely: advertising and sponsor reads, subscription and review appeals,
and the recurring intro and outro banter. They carry no content.

Create a summary with:
- 3-5 paragraphs covering what was actually discussed
- The hosts and any guests, introduced by name and by what they do
- Positions attributed to the person who held them, by name
- Disagreements left standing as disagreements rather than smoothed into consensus
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

const podcastDeepPrompt = `You are a podcast summary assistant. Create a detailed, segment-by-segment summary of the following podcast episode transcript.

Episode: {{TITLE}}

{{LANGUAGE}}

This is a conversation, not a presentation. The transcript has no chapter marks
and any speaker labels in it may be wrong or missing. Work out from the content
who is speaking, and derive the segment boundaries from where the subject
changes.

Skip entirely: advertising and sponsor reads, subscription and review appeals,
and the recurring intro and outro banter. They carry no content.

Create a comprehensive summary with:
- A segment-by-segment breakdown with ## headers, one per subject
- The hosts and any guests, introduced by name and by what they do
- Positions attributed to the person who held them, by name
- Disagreements left standing as disagreements, with both sides stated
- Key quotes where they carry the argument (use blockquotes, name the speaker)
- Detailed key points
- Specific action items and takeaways
- A final "## References" section listing every person, book, paper, tool,
  company and link mentioned. Omit the section if nothing was mentioned.
- Topic tags (5-10 lowercase tags)

Formatting: Use **bold** for emphasis. Do not use *italic*.

Return ONLY valid JSON with these fields:
{
  "text": "The detailed summary in markdown format with ## segment headers",
  "topics": ["tag1", "tag2"],
  "key_points": ["point 1", "point 2"],
  "action_items": ["item 1"]
}

Transcript:
{{TRANSCRIPT}}`

// LangSameAsTranscript asks for the transcript's own language. cast2md reports
// no language for an episode, so the podcast path passes this rather than an
// empty string, which would mean English.
const LangSameAsTranscript = "same"

// promptFor returns the built-in prompt template for a source and level.
// An unknown source falls back to the video prompts.
func promptFor(source, level string) string {
	if source == "podcast" {
		if level == "deep" {
			return podcastDeepPrompt
		}
		return podcastMediumPrompt
	}
	if level == "deep" {
		return deepPrompt
	}
	return mediumPrompt
}

// DefaultPromptFor returns the default prompt template for a source and level.
func DefaultPromptFor(source, level string) string {
	return promptFor(source, level)
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
