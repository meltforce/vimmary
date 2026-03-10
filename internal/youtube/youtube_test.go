package youtube

import (
	"testing"
)

func TestNewClient_DefaultLanguages(t *testing.T) {
	c := NewClient(nil)
	if len(c.subLangs) != 2 || c.subLangs[0] != "en" || c.subLangs[1] != "de" {
		t.Errorf("default subLangs = %v, want [en de]", c.subLangs)
	}
}

func TestNewClient_CustomLanguages(t *testing.T) {
	c := NewClient([]string{"fr", "es"})
	if len(c.subLangs) != 2 || c.subLangs[0] != "fr" || c.subLangs[1] != "es" {
		t.Errorf("custom subLangs = %v, want [fr es]", c.subLangs)
	}
}

func TestParsePlayerResponse(t *testing.T) {
	// Test the regex used for metadata extraction
	html := []byte(`some stuff var ytInitialPlayerResponse = {"videoDetails":{"title":"Test Video","author":"Test Channel","lengthSeconds":"360"}}; more stuff`)

	matches := playerResponseRe.FindSubmatch(html)
	if matches == nil {
		t.Fatal("playerResponseRe did not match")
	}

	want := `{"videoDetails":{"title":"Test Video","author":"Test Channel","lengthSeconds":"360"}}`
	if string(matches[1]) != want {
		t.Errorf("regex captured:\n%s\nwant:\n%s", string(matches[1]), want)
	}
}

func TestParsePlayerResponse_NoMatch(t *testing.T) {
	html := []byte(`<html><body>no player response here</body></html>`)

	matches := playerResponseRe.FindSubmatch(html)
	if matches != nil {
		t.Error("expected no match for page without player response")
	}
}
