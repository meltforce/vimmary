package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/meltforce/vimmary/internal/storage"
	"github.com/meltforce/vimmary/internal/youtube"
)

// emptySegments is the stored negative cache: a fetch ran and InnerTube has no
// captions, so the next player open must not fetch again.
var emptySegments = json.RawMessage("[]")

// segmentFetchGap is the minimum spacing between on-demand InnerTube calls.
// It matches the floor of adaptiveDelay without putting the interactive path
// behind the batch queue, whose spacing grows to 45s under load.
const segmentFetchGap = 10 * time.Second

// toSegmentsJSON converts fetched caption lines into the stored wire format.
// It returns nil for empty input, which keeps the column NULL rather than
// writing a false negative cache.
func toSegmentsJSON(lines []youtube.Line) json.RawMessage {
	if len(lines) == 0 {
		return nil
	}
	segments := make([]storage.TranscriptSegment, len(lines))
	for i, line := range lines {
		segments[i] = storage.TranscriptSegment{Start: line.Start, Duration: line.Duration, Text: line.Text}
	}
	data, err := json.Marshal(segments)
	if err != nil {
		// A slice of three scalar fields cannot fail to marshal; the branch
		// exists so a future field addition surfaces as a nil instead of a panic.
		return nil
	}
	return data
}

// GetTranscriptSegments returns the timed transcript for a video, fetching and
// storing it on first use. Rows without a fetchable source (podcast rows,
// in-flight rows, rows without a YouTube ID) answer an empty array without
// storing anything, so ingest keeps the chance to fill the column properly.
func (s *Service) GetTranscriptSegments(ctx context.Context, userID int, id uuid.UUID) (json.RawMessage, error) {
	stored, err := s.db.GetVideoSegments(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		return stored, nil
	}

	video, err := s.db.GetVideo(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if video.Source != storage.SourceYouTube || video.YouTubeID == "" || video.Status != "completed" {
		return emptySegments, nil
	}

	if err := s.waitSegmentFetchSlot(ctx); err != nil {
		return nil, err
	}

	transcript, err := s.yt.FetchTranscript(ctx, video.YouTubeID)
	if err != nil {
		if isNoCaptionsError(err) {
			// Permanent: cache the emptiness so this row never fetches again.
			if storeErr := s.db.UpdateVideoSegments(ctx, video.ID, emptySegments); storeErr != nil {
				s.log.Warn("failed to store empty segments", "video_id", video.ID, "error", storeErr)
			}
			return emptySegments, nil
		}
		return nil, fmt.Errorf("segment fetch failed: %w", err)
	}

	segments := toSegmentsJSON(transcript.Lines)
	if segments == nil {
		segments = emptySegments
	}
	if err := s.db.UpdateVideoSegments(ctx, video.ID, segments); err != nil {
		return nil, fmt.Errorf("store segments: %w", err)
	}
	return segments, nil
}

// waitSegmentFetchSlot spaces on-demand InnerTube calls segmentFetchGap apart.
// The mutex only guards the bookkeeping; the wait itself is a cancellable
// select outside the critical section.
func (s *Service) waitSegmentFetchSlot(ctx context.Context) error {
	for {
		s.segMu.Lock()
		wait := segmentFetchGap - time.Since(s.segLast)
		if wait <= 0 {
			s.segLast = time.Now()
			s.segMu.Unlock()
			return nil
		}
		s.segMu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}
