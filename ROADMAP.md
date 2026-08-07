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
| `[open]` | Remove the tsnet startup race instead of bounding it | `cmd/vimmary/main.go` | | The 30 s bound and the restart policy shorten a lost race to one restart cycle; they do not prevent it. Measured on 2026-08-07: 4 of 15 starts lost. `tsServer.Up()` cannot serve as the readiness condition, because the AuthLoop short-circuits to `Running` from persisted state before a current netmap exists. Needs either a condition that reflects the netmap, or a setec store that separates "denied because the peer is not yet known" from "denied". See INCIDENTS.md, 2026-08-07. |
| `[open]` | Add `claude_api_key` to the Ansible config template | `../homelab` `configuration/docker-stacks/configs/templates/vimmary/config.yaml.j2` | | `vimmary/claude-api-key` exists in setec at version 2, the template does not reference it, so every start logs `claude api key not available, claude summarizer disabled` and production summarizes on Mistral alone. `summary.provider: mistral` in the same template would also have to change for Claude to be reachable. |
| `[open]` | Drop `karakeep_api_key` from the secret list | `../homelab` config template, `config.yaml` | | Karakeep keys are per-user and live in `users.karakeep_api_key` (`internal/storage/users.go`); nothing resolves the key from the resolver. Listing it in `secrets:` makes `InitSetecStore` request it at startup, so it is a third of the startup dependency on setec for a value no code reads. |
| `[open]` | Reconcile the two setec secrets holding the database password | `../homelab` stack catalog, config template | | `docker/vimmary/db-password` renders `POSTGRES_PASSWORD` into the host `.env` for the db container; `vimmary/postgres-password` is what the app fetches. Both exist in setec, nothing couples them, and a rotation of one alone leaves the app unable to reach its own database. It also means setec adds no confidentiality for this secret — the plaintext copy on the host is required for the db container to initialise. |
| `[open]` | Correct the deploy comment in the deployed compose file | `../homelab` `docker/stacks/vimmary/compose.yaml` | | Its header names `./run.sh --limit totalrecall-lxc -e target_stack=vimmary`; the host is `vimmary-lxc`, which is what `config.yaml.j2` and `stacks/vimmary.yml` both say. |
| `[open]` | Reconcile the CI health budgets with the restart cycle | `.forgejo/workflows/ci.yml` | the race above is removed | The comment on `health_url` cites "the 120 s `start_period` in the compose file", which no longer exists — the probe now lives in the `Dockerfile` with `--start-period=150s`. With the 30 s bound a lost race exits and restarts, so a deploy has roughly a 1 in 4 chance of a red pipeline that self-heals within minutes. Left strict on purpose while the race exists: the noise is the signal. |
| `[open]` | Route deploy failures to a topic that is not shared with routine traffic | `.forgejo/workflows/ci.yml` | | `notify-deploy-failure` published "vimmary CI deploy FAILED (c10511e)" to `ntfy.coydog-fence.ts.net/claude` at 22:29:30 on 2026-08-06, three minutes after the outage began, and the outage ran another six hours. The same topic carries "Ansible deploy needed" and "converge: 0 errors". Detection worked; the path from detection to action did not. |

## Podcasts

| Status | Item | Where | Trigger | Notes |
|---|---|---|---|---|
| `[open]` | Drop `users.summary_prompt_medium` and `summary_prompt_deep` | a later migration (000011 is taken) | `user_prompts` verified in production | Migration `000010` copies the two columns into `user_prompts` and leaves them in place, so `000010`'s down migration can restore them. Nothing reads the columns any more — `GetSummaryPrompts` and `SetSummaryPrompt` were moved to `user_prompts` in `internal/storage/prompts.go`. `storage.GetSummaryPrompts` and `storage.SetSummaryPrompt` in `internal/storage/users.go` are dead code that goes with them. |
| `[open]` | Verify the poller against a live subscription | deployment | a feed is subscribed under Settings → Podcasts | Confirmed on 2026-08-06 after rollout: the deep-link round trip (cast2md button → preview → summary), the separation across `GET /api/v1/videos` and all three RSS feeds, and two episodes summarized end to end. Still unconfirmed, because no feed is subscribed yet: that a new subscription's first poll only sets a watermark, that the next tick picks up a freshly transcribed episode, that backfill leaves the watermark alone, and that `requeueStalePodcasts` recovers rows after a hard restart. |
| `[open]` | Summarize the longest episode on both providers | deployment | a feed is subscribed, or run it by hand | Episode 11481 (4.66 h, 311 kB of `format=txt`) at `deep` on Claude and on Mistral. Check that `summary_input_tokens` lands in the expected 70–110k range, that `summary_output_tokens` stays below the 16000 budget, and that the summary ends with complete JSON. The first live runs were 62 min and 36 min at `medium` — 1949 and 1735 output tokens, so nowhere near the limit that motivated the change. |
| `[open]` | Move the integration config from YAML into the GUI, server names into the database | `internal/config`, `internal/storage`, Settings page | next release round for vimmary and cast2md | Today `cast2md.enabled` and `cast2md.base_url` live in `config.yaml`, which means editing an Ansible template and redeploying to point vimmary at a different cast2md. The Karakeep API key already sits in the database and is set through the Settings page; the cast2md address should work the same way. Same on the other side for `vimmary_url` — cast2md has a Settings page, but the key is not in `_get_configurable_settings()`, so it is env-only. Keep the YAML as the initial value so an unattended deployment still works. |
| `[open]` | Write the user documentation and the project homepages for the integration | `README.md` here and in `../cast2md`, both homepages | next release round for vimmary and cast2md | The README covers setup; what is missing is the view from outside — what the pairing buys someone who finds either project on its own, and that both run standalone. Neither homepage mentions the other project. Worth doing before the next release, on the assumption that someone downloads it. |
