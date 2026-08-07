# Roadmap

**This file contains open work only.** Every row carries the status token
`[open]`. Closed work is not struck through here — it is removed and lives in
[`DECISIONS.md`](DECISIONS.md) (decisions, with their reasoning) or
[`INCIDENTS.md`](INCIDENTS.md) (postmortems).

Columns: **Status** is always `[open]`. **Where** names the artifact the work
touches. **Trigger** carries the condition for items that are deliberately
deferred, and is empty for items that are simply pending. **Notes** carries the
reasoning.

Before closing an item, check its entry for residual work, dates or triggers —
each becomes its own `[open]` row before the entry is moved out.

## Integration

| Status | Item | Where | Trigger | Notes |
|---|---|---|---|---|
| `[open]` | Register the MCP endpoint with tsmcp | `../tsmcp` | | The last remaining item from the original deployment list; Ansible catalog, setec secrets and health check are all in place (`homelab/configuration/docker-stacks/stacks/vimmary.yml`). A search of the tsmcp repo on 2026-08-06 found no vimmary entry. Until then the MCP endpoint is reachable over Tailscale directly at `/mcp`, which is what the README documents. |

## Reliability and deployment

| Status | Item | Where | Trigger | Notes |
|---|---|---|---|---|
| `[open]` | Remove the tsnet startup race | `cmd/vimmary/main.go`, or upstream | | Measured on 2026-08-07 over 15 starts: `AuthLoop: state is Running; done` lost 4 of 4, `Starting; done` won 11 of 11. **vimmary no longer gives the race a way to stop the process** — nothing dials over the node during startup since the setec client was removed — so this is no longer an availability item. It still costs a lost start on any future component that does dial early. `tsServer.Up()` cannot serve as the readiness condition, because the AuthLoop short-circuits to `Running` from persisted state before a current netmap exists; a condition that reflects the netmap would be needed. See INCIDENTS.md, 2026-08-07. The call now sits in `startTailscale`, not in `main`. |
| `[open]` | Cover `summarizeAndStore` end to end | `internal/service` | | The seam added on 2026-08-07 covers provider and key resolution without a database. What it does not reach is the funnel below it: model resolution, custom prompts, token accounting and the embedding call all read `storage.DB` directly. The shape to follow is now settled — a narrow interface over the methods that one path uses, as `settingsSource` and `searchSource` do (`DECISIONS.md`, 2026-08-07) — so this no longer needs the local database the way it would have. |

## Structure and coverage

Everything here comes from `analysis/structure-report.md`, measured 2026-08-06
at `105edde` and re-measured 2026-08-07 at `960ed8d` (section 6). Section 4 of
that report is closed; these are the items its numbers left behind.

| Status | Item | Where | Trigger | Notes |
|---|---|---|---|---|
| `[open]` | Cover `ProcessVideo` | `internal/service/process.go:70` | | CC 23, 143 lines, 15 commits, 0.0% — the highest-complexity Go function still untested. It needs `yt *youtube.Client` behind an interface, the way `Search` needed `searchSource`, and additionally reaches an LLM and Karakeep, so it needs more than one seam. `359551d` is the worked example. |
| `[open]` | Cover `internal/mcp` and `internal/feed` | both packages | | Both at 0.0%, 16 functions between them. `DECISIONS.md` 2026-03 asserts REST and MCP must not diverge, and nothing today would detect it if they did — the structure report states this as measured (section 4.7), not conjectured. `BuildFeed` (`internal/feed/atom.go:66`, CC 10) is the single largest piece of untested logic in the two. |
| `[open]` | Remove `karakeep.Client.GetBookmark` or show it is reachable | `internal/karakeep/client.go:42` | | The one candidate `go run golang.org/x/tools/cmd/deadcode@latest -test ./...` reports, unchanged since 2026-08-06. `deadcode` analyses reachability from `main` and the test binaries, so a call through reflection or an unknown interface would be a false positive; that is what has to be ruled out before deleting. |
| `[open]` | Decide whether `internal/server` gets handler tests | `internal/server` | `internal/service` coverage stops rising | 40 of 40 functions at 0.0%, 658 lines. The handlers are thin — no function reaches CC 10 except `handleSetLLMSettings` — so the value is in the wiring (status codes, source defaults, admin gating), not in logic. Worth doing after the service layer, not instead of it. |

## Podcasts

| Status | Item | Where | Trigger | Notes |
|---|---|---|---|---|
| `[open]` | Drop `users.summary_prompt_medium` and `summary_prompt_deep` | a later migration (000011 is taken) | `user_prompts` verified in production | Migration `000010` copies the two columns into `user_prompts` and leaves them in place, so `000010`'s down migration can restore them. Nothing reads the columns any more — `GetSummaryPrompts` and `SetSummaryPrompt` were moved to `user_prompts` in `internal/storage/prompts.go`. `storage.GetSummaryPrompts` and `storage.SetSummaryPrompt` in `internal/storage/users.go` are dead code that goes with them. |
| `[open]` | Verify the poller against a live subscription | deployment | a feed is subscribed under Settings → Podcasts | Confirmed on 2026-08-06 after rollout: the deep-link round trip (cast2md button → preview → summary), the separation across `GET /api/v1/videos` and all three RSS feeds, and two episodes summarized end to end. Still unconfirmed, because no feed is subscribed yet: that a new subscription's first poll only sets a watermark, that the next tick picks up a freshly transcribed episode, that backfill leaves the watermark alone, and that `requeueStalePodcasts` recovers rows after a hard restart. |
| `[open]` | Summarize the longest episode on both providers | deployment | a feed is subscribed, or run it by hand | Episode 11481 (4.66 h, 311 kB of `format=txt`) at `deep` on Claude and on Mistral. Check that `summary_input_tokens` lands in the expected 70–110k range, that `summary_output_tokens` stays below the 16000 budget, and that the summary ends with complete JSON. The first live runs were 62 min and 36 min at `medium` — 1949 and 1735 output tokens, so nowhere near the limit that motivated the change. |
| `[open]` | Move the integration config from YAML into the GUI, server names into the database | `internal/config`, `internal/storage`, Settings page | next release round for vimmary and cast2md | Today `cast2md.enabled` and `cast2md.base_url` live in `config.yaml`, which means editing an Ansible template and redeploying to point vimmary at a different cast2md. The Karakeep API key already sits in the database and is set through the Settings page; the cast2md address should work the same way. Same on the other side for `vimmary_url` — cast2md has a Settings page, but the key is not in `_get_configurable_settings()`, so it is env-only. Keep the YAML as the initial value so an unattended deployment still works. |
| `[open]` | Write the user documentation and the project homepages for the integration | `README.md` here and in `../cast2md`, both homepages | next release round for vimmary and cast2md | The README covers setup; what is missing is the view from outside — what the pairing buys someone who finds either project on its own, and that both run standalone. Neither homepage mentions the other project. Worth doing before the next release, on the assumption that someone downloads it. |
