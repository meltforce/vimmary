package service

import "testing"

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "headers",
			input: "## Chapter 1\nSome text\n### Section",
			want:  "Chapter 1\nSome text\nSection",
		},
		{
			name:  "bold",
			input: "This is **important** text",
			want:  "This is important text",
		},
		{
			name:  "italic",
			input: "This is *emphasized* text",
			want:  "This is emphasized text",
		},
		{
			name:  "links",
			input: "Click [here](https://example.com) for more",
			want:  "Click here for more",
		},
		{
			name:  "bullets",
			input: "- Item one\n* Item two\n  - Nested",
			want:  "- Item one\n- Item two\n- Nested",
		},
		{
			name:  "backticks",
			input: "Use `go test` command",
			want:  "Use go test command",
		},
		{
			name:  "combined",
			input: "## **Bold Header**\n- [Link](url) with `code`",
			want:  "Bold Header\n- Link with code",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "plain text",
			input: "Just plain text",
			want:  "Just plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("stripMarkdown(%q) =\n%q\nwant:\n%q", tt.input, got, tt.want)
			}
		})
	}
}
