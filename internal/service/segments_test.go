package service

import (
	"testing"

	"github.com/meltforce/vimmary/internal/youtube"
)

func TestToSegmentsJSON(t *testing.T) {
	tests := []struct {
		name  string
		lines []youtube.Line
		want  string
	}{
		{
			name:  "nil input stays nil so the column stays NULL",
			lines: nil,
			want:  "",
		},
		{
			name:  "empty slice stays nil",
			lines: []youtube.Line{},
			want:  "",
		},
		{
			name: "compact keys and float round-trip",
			lines: []youtube.Line{
				{Start: 0, Duration: 2.5, Text: "hello"},
				{Start: 12.4, Duration: 3.1, Text: "world"},
			},
			want: `[{"s":0,"d":2.5,"t":"hello"},{"s":12.4,"d":3.1,"t":"world"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toSegmentsJSON(tt.lines)
			if string(got) != tt.want {
				t.Errorf("toSegmentsJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}
