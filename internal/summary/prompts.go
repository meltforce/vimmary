package summary

const mediumPrompt = `You are a video summary assistant. Summarize the following video transcript.

Video title: %s

Create a summary with:
- 3-5 paragraphs covering the main content
- Key points as a bullet list
- Action items or takeaways (if applicable)
- Topic tags (3-7 lowercase tags)

Return ONLY valid JSON with these fields:
{
  "text": "The summary text in markdown format",
  "topics": ["tag1", "tag2"],
  "key_points": ["point 1", "point 2"],
  "action_items": ["item 1"]
}

Transcript:
%s`

const deepPrompt = `You are a video summary assistant. Create a detailed, chapter-by-chapter summary of the following video transcript.

Video title: %s

Create a comprehensive summary with:
- Chapter-by-chapter breakdown with headers
- Key quotes where relevant (use blockquotes)
- Detailed key points
- Specific action items and takeaways
- Topic tags (5-10 lowercase tags)

Return ONLY valid JSON with these fields:
{
  "text": "The detailed summary in markdown format with ## chapter headers",
  "topics": ["tag1", "tag2"],
  "key_points": ["point 1", "point 2"],
  "action_items": ["item 1"]
}

Transcript:
%s`

func promptForLevel(level string) string {
	if level == "deep" {
		return deepPrompt
	}
	return mediumPrompt
}
