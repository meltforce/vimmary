package feed

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"time"

	"github.com/meltforce/vimmary/internal/storage"
	"github.com/yuin/goldmark"
)

// Atom XML structs

type atomFeed struct {
	XMLName  xml.Name    `xml:"feed"`
	XMLNS    string      `xml:"xmlns,attr"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	Link     []atomLink  `xml:"link"`
	Updated  string      `xml:"updated"`
	ID       string      `xml:"id"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title      string         `xml:"title"`
	Links      []atomLink     `xml:"link"`
	ID         string         `xml:"id"`
	Published  string         `xml:"published"`
	Updated    string         `xml:"updated"`
	Summary    string         `xml:"summary"`
	Content    atomContent    `xml:"content"`
	Categories []atomCategory `xml:"category"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

// BuildFeed generates an Atom 1.0 XML feed from the given videos.
func BuildFeed(videos []storage.Video, baseURL string) ([]byte, error) {
	feed := atomFeed{
		XMLNS:    "http://www.w3.org/2005/Atom",
		Title:    "vimmary — Video Summaries",
		Subtitle: "AI-generated summaries of YouTube videos",
		Link: []atomLink{
			{Href: baseURL, Rel: "alternate", Type: "text/html"},
		},
		ID: baseURL + "/feed/atom",
	}

	if len(videos) > 0 {
		feed.Updated = videos[0].UpdatedAt.Format(time.RFC3339)
	} else {
		feed.Updated = time.Now().Format(time.RFC3339)
	}

	md := goldmark.New()

	for _, v := range videos {
		if v.Status != "completed" {
			continue
		}

		vimmaryURL := fmt.Sprintf("%s/videos/%s", baseURL, v.ID)
		entry := atomEntry{
			Title: fmt.Sprintf("[%s] %s", v.Channel, v.Title),
			Links: []atomLink{
				{Href: vimmaryURL, Rel: "alternate", Type: "text/html"},
				{Href: fmt.Sprintf("https://youtube.com/watch?v=%s", v.YouTubeID), Rel: "related"},
			},
			ID:        fmt.Sprintf("urn:uuid:%s", v.ID),
			Published: v.CreatedAt.Format(time.RFC3339),
			Updated:   v.UpdatedAt.Format(time.RFC3339),
		}

		// Summary: first 200 chars of plain text
		summaryText := v.Summary
		if len(summaryText) > 200 {
			summaryText = summaryText[:200] + "..."
		}
		entry.Summary = summaryText

		// Content: full HTML
		content, err := buildContent(md, v, baseURL)
		if err != nil {
			return nil, fmt.Errorf("build content for %s: %w", v.ID, err)
		}
		entry.Content = atomContent{Type: "html", Body: content}

		// Categories from topics
		var meta struct {
			Topics      []string `json:"topics"`
			KeyPoints   []string `json:"key_points"`
			ActionItems []string `json:"action_items"`
		}
		if len(v.Metadata) > 0 {
			_ = json.Unmarshal(v.Metadata, &meta)
		}
		for _, topic := range meta.Topics {
			entry.Categories = append(entry.Categories, atomCategory{Term: topic})
		}

		feed.Entries = append(feed.Entries, entry)
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		return nil, fmt.Errorf("encode atom feed: %w", err)
	}
	return buf.Bytes(), nil
}

func buildContent(md goldmark.Markdown, v storage.Video, baseURL string) (string, error) {
	var buf bytes.Buffer

	// Summary as HTML
	buf.WriteString("<h2>Summary</h2>\n<div>")
	var htmlBuf bytes.Buffer
	if err := md.Convert([]byte(v.Summary), &htmlBuf); err != nil {
		return "", err
	}
	buf.Write(htmlBuf.Bytes())
	buf.WriteString("</div>\n")

	// Parse metadata
	var meta struct {
		KeyPoints   []string `json:"key_points"`
		ActionItems []string `json:"action_items"`
	}
	if len(v.Metadata) > 0 {
		_ = json.Unmarshal(v.Metadata, &meta)
	}

	if len(meta.KeyPoints) > 0 {
		buf.WriteString("<h2>Key Points</h2>\n<ul>\n")
		for _, kp := range meta.KeyPoints {
			buf.WriteString("  <li>")
			renderInlineMarkdown(md, &buf, kp)
			buf.WriteString("</li>\n")
		}
		buf.WriteString("</ul>\n")
	}

	if len(meta.ActionItems) > 0 {
		buf.WriteString("<h2>Action Items</h2>\n<ul>\n")
		for _, ai := range meta.ActionItems {
			buf.WriteString("  <li>")
			renderInlineMarkdown(md, &buf, ai)
			buf.WriteString("</li>\n")
		}
		buf.WriteString("</ul>\n")
	}

	fmt.Fprintf(&buf, `<p><a href="%s/videos/%s">View summary in vimmary</a> · <a href="https://youtube.com/watch?v=%s">Watch on YouTube</a></p>`, baseURL, v.ID, v.YouTubeID)

	return buf.String(), nil
}

// renderInlineMarkdown converts a Markdown string to HTML and strips the wrapping <p> tags
// so it can be used inline (e.g. inside <li> elements).
func renderInlineMarkdown(md goldmark.Markdown, buf *bytes.Buffer, text string) {
	var tmp bytes.Buffer
	if err := md.Convert([]byte(text), &tmp); err != nil {
		buf.WriteString(html.EscapeString(text))
		return
	}
	// goldmark wraps output in <p>...</p>\n — strip it for inline use
	out := tmp.Bytes()
	out = bytes.TrimPrefix(out, []byte("<p>"))
	out = bytes.TrimSuffix(out, []byte("</p>\n"))
	buf.Write(out)
}
