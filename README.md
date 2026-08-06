# vimmary

Summary service for YouTube videos and podcast episodes. Fetches video transcripts via YouTube's InnerTube API and podcast transcripts from [cast2md](https://github.com/meltforce/cast2md), generates LLM summaries, and stores everything in Postgres + pgvector for semantic search. Videos can be added manually via the web UI or automatically through [Karakeep](https://karakeep.app) webhooks; podcast episodes arrive through per-feed subscriptions or a deep link from cast2md.

Videos and podcasts stay separated in the UI and in RSS. `GET /api/v1/videos` defaults to videos only, and the original feed URL keeps serving videos alone — the combined view is an explicit third option, never the default.

## How it works

```
Karakeep ──webhook──▶ vimmary ──▶ fetch transcript ──▶ generate summary
Web UI ──manual URL──▶    │       (InnerTube / cast2md)        │
cast2md ──poll/deep-link─▶│                     ┌─────────────┼──────────────┐
                          │                     ▼             ▼              ▼
                          │                 pgvector      Karakeep        Web UI
                          │                 + search      writeback       display
                          │                 (videos only)                    │
                          ◀──── MCP tools ──────────────────────────────────┘
```

1. A YouTube video is bookmarked in Karakeep (webhook) or submitted manually via the web UI; a podcast episode is picked up by the cast2md poller or sent over from cast2md's episode page
2. vimmary fetches the transcript — via YouTube's InnerTube API for videos, from cast2md for episodes
3. An LLM (Claude or Mistral) generates a structured summary, using a prompt written for the source
4. The summary is stored with embeddings for semantic search
5. Results are written back to Karakeep (videos only) and displayed in the web UI

## Features

- **Manual URL submission** — paste any YouTube URL in the web UI to process it immediately
- **Automatic summaries** — triggered by Karakeep webhooks, no manual action needed
- **Bulk import** — import all existing YouTube bookmarks from Karakeep via Settings page
- **Two detail levels** — medium (automatic) and deep (on-demand via MCP or web UI)
- **Hybrid search** — keyword + semantic search with Reciprocal Rank Fusion
- **Adaptive rate limiting** — YouTube API delays scale with queue depth (10s–45s) to avoid 429s during bulk operations
- **Auto-retry** — transcript fetch failures are automatically retried with exponential backoff (2m/5m/10m, max 3 retries)
- **Retry all failed** — batch-retry all failed videos from the web UI
- **Podcast summaries** — subscribe to cast2md feeds per user; every episode transcribed from then on is summarized automatically, with per-source prompts and a separate worker queue
- **Podcast backfill** — pull the N most recent completed episodes of a feed on demand, without disturbing the running poll
- **MCP server** — 5 tools for searching and browsing summaries of both kinds
- **RSS feeds** — three Atom feeds per user (videos, podcasts, both), with per-user feed tokens for authentication
- **Web UI** — React frontend embedded in the Go binary (Videos, Podcasts, Stats, Settings pages)
- **Tailscale auth** — zero-config authentication via tsnet
- **Multi-user support** — per-user video libraries (same YouTube video can be bookmarked by multiple users independently)
- **Per-user Karakeep integration** — each user configures their own API key and webhook token via the Settings page
- **Bidirectional sync** — summaries written back to Karakeep notes; bookmark deletions in Karakeep remove videos from vimmary
- **Karakeep writeback** — plain-text summary with vimmary detail link, `video-summarized` tag added (preserves existing Karakeep AI tags)

## Scope

vimmary targets classic YouTube videos — talks, tutorials, conference recordings
— and podcast episodes that cast2md has already transcribed. Livestreams, Shorts
and playlists are deliberately not supported: their transcripts are either absent
or too unstructured for a useful summary.

vimmary does not download or transcribe audio for podcasts. That is cast2md's
job; vimmary reads the finished transcript over the tailnet.

It is sized for personal use, roughly ten videos per day at the upper end. That
assumption shows up in a few places on purpose — the feed serves 50 entries
without paging, there is no conditional GET, and processing runs through a single
worker. Summaries are primarily English with occasional German.

## Architecture

| Component    | Technology                            |
|-------------|---------------------------------------|
| Backend     | Go, chi router                        |
| Database    | PostgreSQL 16 + pgvector              |
| Embeddings  | Mistral (`mistral-embed`, 1024-dim)   |
| Summaries   | Claude API or Mistral (configurable)  |
| Auth        | Tailscale tsnet                       |
| Secrets     | setec                                 |
| Transcripts | YouTube InnerTube API (native Go)     |
| Search      | Hybrid: keyword + semantic with RRF   |
| MCP         | mcp-go, HTTP + stdio transports       |
| Frontend    | React + Vite (embedded in Go binary)  |

## Quick start

### Prerequisites

- Docker and Docker Compose
- A **Mistral API key** for embeddings ([console.mistral.ai](https://console.mistral.ai))
- A **Claude API key** or **Mistral API key** for summaries

### 1. Create a project directory

```bash
mkdir vimmary && cd vimmary
```

### 2. Fetch the compose file

```bash
curl -LO https://raw.githubusercontent.com/meltforce/vimmary/main/compose.example.yml
```

It runs `ghcr.io/meltforce/vimmary:latest` alongside pgvector. (`docker-compose.yml`
in the repo is the *development* stack — it pulls `:edge` from a
tailnet-internal registry and is not usable from outside.)

### 3. Create `config.yaml`

```yaml
external_url: "http://localhost:8080"

server:
  host: "0.0.0.0"
  port: 8080

database:
  host: db
  port: 5432
  name: vimmary
  user: vimmary

summary:
  provider: "claude"          # "claude" or "mistral"

youtube:
  sub_langs: [en]             # preferred transcript languages

secrets:
  postgres_password: "vimmary"
  mistral_api_key: "your-mistral-key"   # required (embeddings)
  claude_api_key: "your-claude-key"     # required if provider is "claude"
```

Host, port and database settings can be overridden from the environment:
`VIMMARY_SERVER_HOST`, `VIMMARY_SERVER_PORT`, `VIMMARY_DB_HOST`,
`VIMMARY_DB_PORT`, `VIMMARY_DB_NAME`, `VIMMARY_DB_USER`, `VIMMARY_DB_SSLMODE`,
`VIMMARY_TS_ENABLED`, `VIMMARY_TS_HOSTNAME`, `VIMMARY_TS_STATE_DIR`. Secrets are
read from `config.yaml` or from the configured secret backend, not from the
environment.

### 4. Start

```bash
# Postgres reads its password from .env; it has to match secrets.postgres_password
echo "POSTGRES_PASSWORD=vimmary" > .env
docker compose -f compose.example.yml up -d
```

Open `http://localhost:8080` and start adding videos. Migrations run automatically on startup.

### Local development

```bash
# Start only the database
docker compose up db

# Run the backend (requires Go 1.23+)
go run ./cmd/vimmary --config config.yaml

# Run the frontend with hot-reload (separate terminal)
cd web && npm install && npm run dev
```

## Setup Karakeep integration

1. Open vimmary's **Settings** page (Tailscale auth required)
2. Enter your **Karakeep API key** (from Karakeep Settings → API Keys)
3. Copy the generated **Webhook URL** and **Bearer Token**
4. In Karakeep Settings → Webhooks, create webhooks for `created` and `deleted` events
5. If Karakeep runs in Docker and vimmary is on Tailscale, add `CRAWLER_ALLOWED_INTERNAL_HOSTNAMES=.your-tailnet.ts.net` to Karakeep's env to allow webhook delivery

## Setup podcast summaries

Podcast transcripts come from [cast2md](https://github.com/meltforce/cast2md).
vimmary polls it; cast2md never calls vimmary. Both services must be on the same
tailnet, because cast2md has no authentication and is reachable only there.

1. In vimmary's config, set `cast2md.enabled: true` and `cast2md.base_url` to the cast2md host
2. Restart vimmary. Its poller starts 30 seconds later and does nothing until a feed is subscribed
3. Open **Settings → Podcasts**, tick the feeds you want and pick a detail level per feed

Subscribing means **from now on**. The first poll of a new subscription records
the newest completed episode's timestamp as its watermark and summarizes
nothing — older episodes are fetched with the per-feed **Backfill** button, which
does not move the watermark. Switching a feed off keeps its watermark, so
switching it back on later fetches the gap.

To summarize one episode without subscribing, set `vimmary_url` in cast2md's
settings. Completed episodes then carry a **Summarize in vimmary** button that
links to vimmary's `/podcasts/new?episode=<id>` page.

Podcasts have their own prompts, editable per level under **Settings →
Summaries** with the Videos/Podcasts switch. Editing one does not touch the
other.

## RSS feeds

vimmary provides Atom feeds of your summaries, including full summaries, key points, and action items. Each entry links back to the vimmary summary page and to the source — YouTube for videos, the cast2md episode page for podcasts.

There are three feeds behind one token:

| URL | Contents |
|---|---|
| `/feed/atom/<feed-token>` | Videos only. This is the original URL, so existing subscriptions are unaffected by podcast summaries appearing. |
| `/feed/atom/<feed-token>/podcasts` | Podcast episodes only. |
| `/feed/atom/<feed-token>/all` | Both. Every entry carries its type as its first `<category>`. |

An RSS reader cannot filter, which is why the split lives in the URL rather than in a query parameter.

1. Open vimmary's **Settings** page
2. Copy the URL of the feed you want from the **RSS** section (the token is generated automatically on first access)
3. Subscribe in your RSS reader
4. Optional: append `?limit=100` to fetch more than the default 50 entries (max 200)

Each user has their own feed token. The token is the only authentication — no Tailscale auth is needed for the feed URL, so it works with any RSS reader.

## MCP configuration

The MCP server is always available at `/mcp` (HTTP + SSE transport) and can also be started in stdio mode via `--mcp` flag for local use.

**HTTP (production):** Add vimmary as an MCP server in your client using the SSE endpoint:

```json
{
  "mcpServers": {
    "vimmary": {
      "url": "https://<your-vimmary-host>/mcp"
    }
  }
}
```

**Stdio (local development):**

```json
{
  "mcpServers": {
    "vimmary": {
      "command": "go",
      "args": ["run", "./cmd/vimmary", "--mcp", "--config", "config.yaml"]
    }
  }
}
```

Authentication is handled via Tailscale (HTTP mode) or defaults to user ID 1 (stdio mode).

## Build

```bash
# Build binary
CGO_ENABLED=0 go build -o vimmary ./cmd/vimmary

# Build Docker image
docker buildx build --platform linux/amd64 -t vimmary:local .
```

## MCP tools

| Tool              | Description                                      |
|-------------------|--------------------------------------------------|
| `search_videos`   | Hybrid search (keyword + semantic, RRF)          |
| `get_video`       | Retrieve full video details by ID                |
| `list_recent`     | Browse recent videos with filters                |
| `resummarize`     | Regenerate summary with different detail level   |
| `stats`           | Aggregate statistics, optionally per source      |

## Repository documents

| File | Holds |
|---|---|
| [`CLAUDE.md`](CLAUDE.md) | What the repo is, and the gotchas the file tree does not show. |
| [`DECISIONS.md`](DECISIONS.md) | Decisions taken, with their reasoning and the condition that would re-open them. |
| [`ROADMAP.md`](ROADMAP.md) | Open work only. |
| [`INCIDENTS.md`](INCIDENTS.md) | Postmortems. |

## Related projects

- [meltkit](https://github.com/meltforce/meltkit) — shared Go library (db, config, secrets, middleware, MCP)
- [totalrecall](https://github.com/meltforce/totalrecall) — personal knowledge system (architectural blueprint)

---

Source-of-truth development happens on a self-hosted Forgejo (private); [github.com/meltforce/vimmary](https://github.com/meltforce/vimmary) is the public mirror.
