[![CI](https://github.com/meltforce/vimmary/actions/workflows/ci.yml/badge.svg)](https://github.com/meltforce/vimmary/actions/workflows/ci.yml)

# vimmary

YouTube video summary service. Fetches transcripts via YouTube's InnerTube API, generates LLM summaries, and stores everything in Postgres + pgvector for semantic search. Videos can be added manually via the web UI or automatically through [Karakeep](https://karakeep.app) webhooks.

## How it works

```
Karakeep ──webhook──▶ vimmary ──▶ fetch transcript ──▶ generate summary
Web UI ──manual URL──▶    │                                   │
                          │                     ┌─────────────┼──────────────┐
                          │                     ▼             ▼              ▼
                          │                 pgvector      Karakeep        Web UI
                          │                 + search      writeback       display
                          │                                                 │
                          ◀──── MCP tools ──────────────────────────────────┘
```

1. A YouTube video is bookmarked in Karakeep (webhook) or submitted manually via the web UI
2. vimmary fetches the transcript via YouTube's InnerTube API
3. An LLM (Claude or Mistral) generates a structured summary
4. The summary is stored with embeddings for semantic search
5. Results are written back to Karakeep (if applicable) and displayed in the web UI

## Features

- **Manual URL submission** — paste any YouTube URL in the web UI to process it immediately
- **Automatic summaries** — triggered by Karakeep webhooks, no manual action needed
- **Bulk import** — import all existing YouTube bookmarks from Karakeep via Settings page
- **Two detail levels** — medium (automatic) and deep (on-demand via MCP or web UI)
- **Hybrid search** — keyword + semantic search with Reciprocal Rank Fusion
- **Adaptive rate limiting** — YouTube API delays scale with queue depth (10s–45s) to avoid 429s during bulk operations
- **Auto-retry** — transcript fetch failures are automatically retried with exponential backoff (2m/5m/10m, max 3 retries)
- **Retry all failed** — batch-retry all failed videos from the web UI
- **MCP server** — 6 tools for searching, browsing, and managing video summaries
- **Web UI** — React frontend embedded in the Go binary (Videos, Stats, Settings pages)
- **Tailscale auth** — zero-config authentication via tsnet
- **Multi-user support** — per-user video libraries (same YouTube video can be bookmarked by multiple users independently)
- **Per-user Karakeep integration** — each user configures their own API key and webhook token via the Settings page
- **Bidirectional sync** — summaries written back to Karakeep notes; bookmark deletions in Karakeep remove videos from vimmary
- **Karakeep writeback** — plain-text summary with vimmary detail link, `video-summarized` tag added (preserves existing Karakeep AI tags)

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

### 2. Create `docker-compose.yml`

```yaml
services:
  app:
    image: meltforce/vimmary:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
    depends_on:
      db:
        condition: service_healthy

  db:
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: vimmary
      POSTGRES_USER: vimmary
      POSTGRES_PASSWORD: vimmary
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: pg_isready -U vimmary
      interval: 5s
      retries: 5

volumes:
  pgdata:
```

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

All config values can also be set via `VIMMARY_*` environment variables (e.g. `VIMMARY_SECRETS_CLAUDE_API_KEY`).

### 4. Start

```bash
docker compose up -d
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

## Build

```bash
# Build binary
CGO_ENABLED=0 go build -o vimmary ./cmd/vimmary

# Build Docker image
docker buildx build --platform linux/amd64 -t meltforce/vimmary:edge .
```

## MCP tools

| Tool              | Description                                      |
|-------------------|--------------------------------------------------|
| `search_videos`   | Hybrid search (keyword + semantic, RRF)          |
| `get_video`       | Retrieve full video details by ID                |
| `list_recent`     | Browse recent videos with filters                |
| `resummarize`     | Regenerate summary with different detail level   |
| `stats`           | Aggregate statistics                             |
| `delete_video`    | Delete a video and its data                      |

## Related projects

- [meltkit](https://github.com/meltforce/meltkit) — shared Go library (db, config, secrets, middleware, MCP)
- [totalrecall](https://github.com/meltforce/totalrecall) — personal knowledge system (architectural blueprint)
