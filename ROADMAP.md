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
