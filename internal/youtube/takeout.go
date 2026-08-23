package youtube

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
)

// takeoutChannelIDRe validates one channel ID cell.
var takeoutChannelIDRe = regexp.MustCompile(`^UC[\w-]{22}$`)

// ParseTakeoutSubscriptions reads the subscriptions.csv a Google Takeout
// export of YouTube data contains: a header row plus one row per channel with
// the columns Channel Id, Channel Url, Channel Title. Rows that carry no
// valid channel ID are skipped rather than failing the import — Takeout has
// shipped trailing blank lines and localized headers before.
func ParseTakeoutSubscriptions(data string) ([]ChannelInfo, int, error) {
	reader := csv.NewReader(strings.NewReader(data))
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("parse subscriptions CSV: %w", err)
	}

	var channels []ChannelInfo
	skipped := 0
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		id := strings.TrimSpace(record[0])
		if !takeoutChannelIDRe.MatchString(id) {
			// The header row lands here too, so it is not counted as skipped.
			if !strings.EqualFold(id, "Channel Id") && id != "" {
				skipped++
			}
			continue
		}
		title := ""
		if len(record) >= 3 {
			title = strings.TrimSpace(record[len(record)-1])
		}
		channels = append(channels, ChannelInfo{ID: id, Title: title})
	}

	if len(channels) == 0 {
		return nil, skipped, fmt.Errorf("no channel IDs found — expected Takeout's subscriptions.csv with a Channel Id column")
	}
	return channels, skipped, nil
}
