package youtube

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// FetchTranscript extracts captions via yt-dlp.
func (c *Client) FetchTranscript(ctx context.Context, youtubeID string) (*Transcript, error) {
	url := "https://www.youtube.com/watch?v=" + youtubeID

	tmpDir, err := os.MkdirTemp("", "vimmary-sub-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outTemplate := filepath.Join(tmpDir, "sub")
	langs := strings.Join(c.subLangs, ",")

	// Try manual subtitles first
	transcript, err := c.extractSubs(ctx, url, outTemplate, langs, false)
	if err == nil {
		return transcript, nil
	}

	// Fall back to auto-generated subtitles
	transcript, err = c.extractSubs(ctx, url, outTemplate, langs, true)
	if err != nil {
		return nil, fmt.Errorf("no captions available for %s: %w", youtubeID, err)
	}
	return transcript, nil
}

func (c *Client) extractSubs(ctx context.Context, url, outTemplate, langs string, auto bool) (*Transcript, error) {
	args := []string{
		"--skip-download",
		"--no-playlist",
		"--sub-format", "vtt/srt/best",
		"-o", outTemplate,
	}
	if auto {
		args = append(args, "--write-auto-sub")
	} else {
		args = append(args, "--write-sub")
	}
	args = append(args, "--sub-lang", langs, url)

	cmd := exec.CommandContext(ctx, c.ytdlpPath, args...)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp subtitles: %w", err)
	}

	// Find the subtitle file
	dir := filepath.Dir(outTemplate)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read temp dir: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".vtt") || strings.HasSuffix(name, ".srt") {
			content, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			text := parseSubtitles(string(content))
			if text == "" {
				continue
			}

			lang := detectLanguageFromFilename(name)
			source := "manual"
			if auto {
				source = "auto"
			}

			return &Transcript{
				Text:     text,
				Language: lang,
				Source:   source,
			}, nil
		}
	}

	return nil, fmt.Errorf("no subtitle files found")
}

var (
	vttTimestamp = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}\.\d{3}\s*-->`)
	srtTimestamp = regexp.MustCompile(`^\d{2}:\d{2}:\d{2},\d{3}\s*-->`)
	srtIndex    = regexp.MustCompile(`^\d+$`)
	vttTag      = regexp.MustCompile(`<[^>]+>`)
)

// parseSubtitles converts VTT/SRT content to plain text, deduplicating lines.
func parseSubtitles(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	seen := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines, timestamps, indexes, and VTT header
		if line == "" || line == "WEBVTT" || strings.HasPrefix(line, "Kind:") ||
			strings.HasPrefix(line, "Language:") || strings.HasPrefix(line, "NOTE") ||
			vttTimestamp.MatchString(line) || srtTimestamp.MatchString(line) ||
			srtIndex.MatchString(line) {
			continue
		}

		// Strip VTT formatting tags
		line = vttTag.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Deduplicate (auto-subs often repeat lines)
		if !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, " ")
}

// detectLanguageFromFilename extracts the language code from subtitle filenames.
// e.g. "sub.en.vtt" -> "en", "sub.de.vtt" -> "de"
func detectLanguageFromFilename(name string) string {
	parts := strings.Split(strings.TrimSuffix(name, filepath.Ext(name)), ".")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return "en"
}
