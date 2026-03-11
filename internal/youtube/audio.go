package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ExtractAudio downloads audio from a YouTube video using yt-dlp.
// Returns the path to the downloaded MP3 file and a cleanup function.
func ExtractAudio(ctx context.Context, youtubeID string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "vimmary-audio-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	outPath := filepath.Join(dir, youtubeID+".mp3")
	url := "https://www.youtube.com/watch?v=" + youtubeID

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "5",
		"-o", outPath,
		"--no-playlist",
		"--no-warnings",
		url,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("yt-dlp failed: %w: %s", err, string(output))
	}

	if _, err := os.Stat(outPath); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("audio file not found after extraction: %w", err)
	}

	return outPath, cleanup, nil
}
