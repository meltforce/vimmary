package youtube

import (
	"encoding/json"
	"slices"
	"testing"
)

// The fixtures are trimmed from watch pages fetched on 2026-11-08.
func TestSpokenLanguage(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{
			// CPtTdWBX888: German original with an English auto-dub, which
			// adds an "a.en" speech-recognition track beside "a.de".
			name: "auto-dubbed",
			json: `{
				"captions": {"playerCaptionsTracklistRenderer": {"captionTracks": [
					{"languageCode": "en", "kind": "asr"},
					{"languageCode": "de", "kind": "asr"}]}},
				"streamingData": {"adaptiveFormats": [
					{"audioTrack": {"displayName": "German (DE) original", "id": "de-DE.4"}},
					{"audioTrack": {"displayName": "English (US)", "id": "en-US.10", "isAutoDubbed": true}},
					{}]}}`,
			want: "de",
		},
		{
			// a__Rva9Wqek: German video with uploaded English and German
			// subtitles and no audio tracks.
			name: "uploaded subtitles, one asr track",
			json: `{"captions": {"playerCaptionsTracklistRenderer": {"captionTracks": [
				{"languageCode": "en"},
				{"languageCode": "de"},
				{"languageCode": "de", "kind": "asr"}]}}}`,
			want: "de",
		},
		{
			name: "single undubbed audio track without the original label",
			json: `{"streamingData": {"adaptiveFormats": [
				{"audioTrack": {"displayName": "Deutsch", "id": "de.4"}},
				{"audioTrack": {"displayName": "English", "id": "en.10", "isAutoDubbed": true}}]}}`,
			want: "de",
		},
		{
			name: "two asr tracks and no audio tracks",
			json: `{"captions": {"playerCaptionsTracklistRenderer": {"captionTracks": [
				{"languageCode": "en", "kind": "asr"},
				{"languageCode": "de", "kind": "asr"}]}}}`,
			want: "",
		},
		{
			name: "no captions",
			json: `{}`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p playerResponse
			if err := json.Unmarshal([]byte(tc.json), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := p.spokenLanguage(); got != tc.want {
				t.Errorf("spokenLanguage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTranscriptLanguages(t *testing.T) {
	sub := []string{"en", "de"}
	cases := []struct {
		spoken string
		want   []string
	}{
		{"", []string{"en", "de"}},
		{"en", []string{"en", "de"}},
		{"de", []string{"de", "en"}},
		{"fr", []string{"fr", "en", "de"}},
	}
	for _, tc := range cases {
		if got := transcriptLanguages(tc.spoken, sub); !slices.Equal(got, tc.want) {
			t.Errorf("transcriptLanguages(%q) = %v, want %v", tc.spoken, got, tc.want)
		}
	}
	if !slices.Equal(sub, []string{"en", "de"}) {
		t.Errorf("subLangs modified: %v", sub)
	}
}
