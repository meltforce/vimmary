# Roadmap

Open work, one line per item. A closed item is deleted. The rows with their
full notes as of 2026-11-13 are frozen in `archive/ROADMAP-2026-11-13.md`.

## Integration

- Register the MCP endpoint with tsmcp (`../tsmcp`)

## Reliability and deployment

- Remove the tsnet startup race (`cmd/vimmary/main.go`, or upstream)
- Cover `summarizeAndStore` end to end (`internal/service`)

## Structure and coverage

- Cover `ProcessVideo` (`internal/service/process.go:70`) — the next change to its branch logic, or the first defect on one of its failure paths
- Cover `internal/mcp` and `internal/feed` (both packages)
- Remove `karakeep.Client.GetBookmark` or show it is reachable (`internal/karakeep/client.go:42`)
- Decide whether `internal/server` gets handler tests (`internal/server`) — `internal/service` coverage stops rising

## Podcasts

- Drop `users.summary_prompt_medium` and `summary_prompt_deep` (a later migration (000011 is taken)) — `user_prompts` verified in production
- Verify the poller against a live subscription (deployment) — a feed is subscribed under Settings → Podcasts
- Summarize the longest episode on both providers (deployment) — a feed is subscribed, or run it by hand
- Move the integration config from YAML into the GUI, server names into the database (`internal/config`, `internal/storage`, Settings page) — next release round for vimmary and cast2md
- Write the user documentation and the project homepages for the integration (`README.md` here and in `../cast2md`, both homepages) — next release round for vimmary and cast2md

## Channels inbox

- Verify the channel poller against a live subscription (deployment) — a channel is followed under Channels
- Inbox retention job (`internal/storage/channels.go`) — `inbox_items` growth becomes visible

## Redesign

- Podcast Listen player — cast2md serves timed segments and an audio URL
- Clean up the four remaining `--rule-strong` call sites — touching any of those four screens

## Transcript player

- Bulk segment backfill for pre-000014 rows (`internal/service/segments.go`) — per-open fetching proves annoying in practice
- Keep the player mounted across tab switches (`web/src/pages/VideoDetailPage.tsx`) — losing the playback position on a tab switch proves annoying

## Web UI

- Bring `docs/index.html` onto the Shelf language (`docs/index.html`)
