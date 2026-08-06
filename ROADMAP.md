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

## Podcasts

| Status | Item | Where | Trigger | Notes |
|---|---|---|---|---|
| `[open]` | Drop `users.summary_prompt_medium` and `summary_prompt_deep` | `migrations/000011_*` | `user_prompts` verified in production | Migration `000010` copies the two columns into `user_prompts` and leaves them in place, so `000010`'s down migration can restore them. Nothing reads the columns any more — `GetSummaryPrompts` and `SetSummaryPrompt` were moved to `user_prompts` in `internal/storage/prompts.go`. `storage.GetSummaryPrompts` and `storage.SetSummaryPrompt` in `internal/storage/users.go` are dead code that goes with them. |
| `[open]` | Run the manual verification sequence against production | deployment | vimmary and cast2md both deployed | The automated chain covers migrations, the cast2md client, the poller's watermark handling and the storage-layer source filters. Not covered without live services: the deep-link round trip from cast2md, the separation test across GUI and all three RSS feeds, the self-healing restart, and the longest-episode case (episode 11481, 4.66 h, 311 kB) on both providers to confirm `summary_output_tokens` stays below the 16000 `deep` budget. The steps are in the implementation plan at `~/.claude/plans/zwei-systeme-die-die-enumerated-blossom.md`. |
| `[open]` | Set `cast2md.vimmary_url` in cast2md's deployment | `../cast2md`, homelab config | vimmary deployed with podcasts enabled | Without it the **Summarize in vimmary** button on cast2md's episode page stays hidden. The setting defaults to empty and is safe to leave unset. |
