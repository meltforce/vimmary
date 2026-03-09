[![CI](https://github.com/meltforce/vimmary/actions/workflows/ci.yml/badge.svg)](https://github.com/meltforce/vimmary/actions/workflows/ci.yml)

# vimmary

YouTube video summary service. Receives [Karakeep](https://karakeep.app) webhooks for new bookmarks, fetches transcripts via YouTube's InnerTube API, generates LLM summaries, and stores everything in Postgres + pgvector for semantic search.

## How it works

```
Karakeep ──webhook──▶ vimmary ──▶ fetch transcript ──▶ generate summary
                                                            │
                                              ┌─────────────┼──────────────┐
                                              ▼             ▼              ▼
                                          pgvector      Karakeep        Web UI
                                          + search      writeback       display
```

1. A YouTube video is bookmarked in Karakeep
2. Karakeep sends a webhook to vimmary
3. vimmary fetches the transcript via YouTube's InnerTube API
4. An LLM (Claude or Mistral) generates a structured summary
5. The summary is stored with embeddings for semantic search
6. Results are written back to Karakeep and displayed in the web UI

## Features

- **Automatic summaries** — triggered by Karakeep webhooks, no manual action needed
- **Two detail levels** — medium (automatic) and deep (on-demand via MCP or web UI)
- **Hybrid search** — keyword + semantic search with Reciprocal Rank Fusion
- **MCP server** — 6 tools for searching, browsing, and managing video summaries
- **Web UI** — React frontend embedded in the Go binary
- **Tailscale auth** — zero-config authentication via tsnet

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

```bash
# Start PostgreSQL with pgvector
docker compose up db

# Copy and edit config
cp config.example.yaml config.yaml
# Edit config.yaml with your API keys

# Run the server
go run ./cmd/vimmary --config config.yaml
```

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

## Deployment

Deployed via Docker Compose on a Tailscale-connected host. Config is mounted externally, secrets resolved via setec.

```bash
# Production deploy (via Ansible)
cd configuration/docker-stacks && ./run.sh --limit totalrecall-lxc -e target_stack=vimmary
```

## Related projects

- [meltkit](https://github.com/meltforce/meltkit) — shared Go library (db, config, secrets, middleware, MCP)
- [totalrecall](https://github.com/meltforce/totalrecall) — personal knowledge system (architectural blueprint)
