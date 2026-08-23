package youtube

import (
	"testing"
	"time"
)

func TestNormalizeChannelURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
		fails bool
	}{
		{input: "@somehandle", want: "https://www.youtube.com/@somehandle"},
		{input: "somehandle", want: "https://www.youtube.com/@somehandle"},
		{input: "https://www.youtube.com/@somehandle", want: "https://www.youtube.com/@somehandle"},
		{input: "https://www.youtube.com/c/SomeName", want: "https://www.youtube.com/c/SomeName"},
		{input: "https://www.youtube.com/user/oldname", want: "https://www.youtube.com/user/oldname"},
		{input: "www.youtube.com/@somehandle", want: "https://www.youtube.com/@somehandle"},
		{input: "https://vimeo.com/whoever", fails: true},
	}
	for _, tt := range tests {
		got, err := normalizeChannelURL(tt.input)
		if tt.fails {
			if err == nil {
				t.Errorf("normalizeChannelURL(%q) should fail", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeChannelURL(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("normalizeChannelURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseChannelPage(t *testing.T) {
	body := []byte(`<html><head>
		<meta property="og:title" content="Some Creator">
		<meta property="og:image" content="https://yt3.ggpht.com/abc=s900">
		</head><body><script>var x = {"channelId":"UCabcdefghijklmnopqrstuv","other":1};</script></body></html>`)
	info := parseChannelPage(body)
	if info.ID != "UCabcdefghijklmnopqrstuv" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Title != "Some Creator" {
		t.Errorf("Title = %q", info.Title)
	}
	if info.ThumbnailURL != "https://yt3.ggpht.com/abc=s900" {
		t.Errorf("ThumbnailURL = %q", info.ThumbnailURL)
	}
}

// The canonical link names the page's own channel; the body's first
// "channelId" can be a localized sibling (observed on @veritasium, whose data
// island carried "Veritasium en Français" while the canonical link held the
// real channel).
func TestParseChannelPage_CanonicalWins(t *testing.T) {
	body := []byte(`<html><head>
		<link rel="canonical" href="https://www.youtube.com/channel/UCcanonical0000000000000">
		</head><body>{"channelId":"UClocalized0000000000000"}</body></html>`)
	info := parseChannelPage(body)
	if info.ID != "UCcanonical0000000000000" {
		t.Errorf("ID = %q, want the canonical link's channel", info.ID)
	}
}

// A captured (abbreviated) real feed shape: Atom with the yt namespace.
const feedFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns="http://www.w3.org/2005/Atom">
 <title>Channel uploads</title>
 <entry>
  <id>yt:video:abc123DEF45</id>
  <yt:videoId>abc123DEF45</yt:videoId>
  <title>A normal upload</title>
  <published>2026-08-20T15:04:05+00:00</published>
 </entry>
 <entry>
  <id>yt:video:ghi678JKL90</id>
  <yt:videoId>ghi678JKL90</yt:videoId>
  <title>Broken date entry</title>
  <published>not-a-date</published>
 </entry>
</feed>`

func TestParseChannelFeed(t *testing.T) {
	entries, err := parseChannelFeed([]byte(feedFixture))
	if err != nil {
		t.Fatalf("parseChannelFeed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].VideoID != "abc123DEF45" || entries[0].Title != "A normal upload" {
		t.Errorf("entry 0 = %+v", entries[0])
	}
	want := time.Date(2026, 8, 20, 15, 4, 5, 0, time.UTC)
	if !entries[0].Published.Equal(want) {
		t.Errorf("published = %v, want %v", entries[0].Published, want)
	}
	// An unparsable date keeps the entry with the zero time rather than
	// failing the feed.
	if !entries[1].Published.IsZero() {
		t.Errorf("entry with a broken date should carry the zero time, got %v", entries[1].Published)
	}
}

func TestParseChannelFeed_Invalid(t *testing.T) {
	if _, err := parseChannelFeed([]byte("not xml at all <")); err == nil {
		t.Fatal("parseChannelFeed should fail on invalid XML")
	}
}

func TestClassifyShortsProbe(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		location string
		short    bool
		fails    bool
	}{
		{name: "200 is a Short", status: 200, short: true},
		{name: "303 to /watch is a regular video", status: 303, location: "https://www.youtube.com/watch?v=x"},
		{name: "302 to /watch is a regular video", status: 302, location: "https://www.youtube.com/watch?v=x&pp=y"},
		{name: "consent redirect is unknown", status: 302, location: "https://consent.youtube.com/m?continue=x", fails: true},
		{name: "server error is unknown", status: 503, fails: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			short, err := classifyShortsProbe(tt.status, tt.location)
			if tt.fails {
				if err == nil {
					t.Error("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyShortsProbe: %v", err)
			}
			if short != tt.short {
				t.Errorf("short = %v, want %v", short, tt.short)
			}
		})
	}
}
