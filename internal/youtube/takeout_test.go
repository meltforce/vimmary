package youtube

import "testing"

func TestParseTakeoutSubscriptions(t *testing.T) {
	csv := `Channel Id,Channel Url,Channel Title
UCabcdefghijklmnopqrstuv,http://www.youtube.com/channel/UCabcdefghijklmnopqrstuv,Some Channel
UC0123456789_-abcdefghij,http://www.youtube.com/channel/UC0123456789_-abcdefghij,"Name, with comma"
not-a-channel,http://example.com,Broken row

`
	channels, skipped, err := ParseTakeoutSubscriptions(csv)
	if err != nil {
		t.Fatalf("ParseTakeoutSubscriptions: %v", err)
	}
	if len(channels) != 2 {
		t.Fatalf("got %d channels, want 2", len(channels))
	}
	if channels[0].ID != "UCabcdefghijklmnopqrstuv" || channels[0].Title != "Some Channel" {
		t.Errorf("channel 0 = %+v", channels[0])
	}
	if channels[1].Title != "Name, with comma" {
		t.Errorf("quoted title = %q", channels[1].Title)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1 (the broken row; header and blank lines are not skips)", skipped)
	}
}

func TestParseTakeoutSubscriptions_NoChannels(t *testing.T) {
	if _, _, err := ParseTakeoutSubscriptions("just some text\nwithout ids"); err == nil {
		t.Fatal("expected an error for content without channel IDs")
	}
}
