package main

import (
	"slices"
	"testing"
	"time"
)

func TestParseExpiry(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		err  bool
	}{
		{in: "", want: 0},
		{in: "90d", want: 90 * 24 * time.Hour},
		{in: "720h", want: 720 * time.Hour},
		{in: "0d", err: true},
		{in: "-5h", err: true},
		{in: "xd", err: true},
		{in: "soon", err: true},
	}
	for _, tt := range tests {
		got, err := parseExpiry(tt.in)
		if (err != nil) != tt.err || got != tt.want {
			t.Errorf("parseExpiry(%q) = %v, %v; want %v, error %v", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestScopeFlags(t *testing.T) {
	var s scopeFlags
	_ = s.Set("mcp")
	_ = s.Set("api, mcp")
	if want := []string{"mcp", "api", "mcp"}; !slices.Equal(s, want) {
		t.Errorf("scopes = %v, want %v", s, want)
	}
}
