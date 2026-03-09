# vimmary

YouTube video summary service. Receives Karakeep webhooks for new bookmarks, fetches transcripts (YouTube captions → Whisper fallback), generates LLM summaries, and stores everything in Postgres + pgvector for semantic search via MCP.

See `CONCEPT.md` for full project concept and design decisions.

## Architecture

- **Go** service with Tailscale auth (tsnet WhoIs)
- **Postgres + pgvector** for storage + vector similarity search
- **Mistral API** for embeddings (`mistral-embed`, 1024-dim)
- **Claude API or Mistral** for summaries (configurable)
- **yt-dlp** for YouTube caption extraction, Whisper as fallback
- **MCP** (HTTP/SSE via tsmcp) + **REST API** — both call shared service layer
- **Karakeep webhooks** as trigger, Karakeep API for read/writeback
- **Minimal web UI** (React + Vite, embedded in Go binary) for summary display
- **Docker Compose** deployment (Go app + `pgvector/pgvector:pg16`)

## Project Layout

```
cmd/vimmary/main.go              # Entry point, HTTP server, Tailscale setup
internal/config/config.go        # YAML + VIMMARY_* env var config
internal/karakeep/client.go      # Karakeep REST API client
internal/karakeep/webhook.go     # Karakeep webhook handler
internal/youtube/youtube.go      # Client struct + types
internal/youtube/transcript.go   # yt-dlp caption extraction, VTT/SRT parsing
internal/youtube/metadata.go     # yt-dlp --dump-json metadata extraction
internal/summary/summary.go      # Summarizer interface + Summary type
internal/summary/claude.go       # Claude API summarizer
internal/summary/mistral.go      # Mistral API summarizer
internal/summary/prompts.go      # Prompt templates (medium/deep)
internal/service/service.go      # Service struct + dependencies
internal/service/process.go      # ProcessVideo (transcript → summary → embed → store → writeback)
internal/service/search.go       # Hybrid search (keyword + semantic, RRF), ListRecent, GetVideo, Stats
internal/service/resummarize.go  # Resummarize with different detail level
internal/storage/storage.go      # DB wrapper (embedded meltkit db.DB)
internal/storage/users.go        # GetOrCreateUser, GetPrimaryUser
internal/storage/videos.go       # InsertVideo, GetByYouTubeID, UpdateSummary, SearchVideos, etc.
internal/mistral/client.go       # Mistral API client (embeddings only)
internal/mcp/server.go           # MCP server setup + tool definitions
internal/mcp/tools.go            # MCP tool handlers
internal/server/server.go        # HTTP server (chi + meltkit)
internal/server/handlers.go      # REST API handlers
migrations/                      # SQL migrations
web/                             # React + Vite frontend (embedded, not yet implemented)
```

## Patterns (follow totalrecall conventions)

- **Config**: YAML file + `VIMMARY_*` env var overrides. Secrets via `secrets.NewResolver()` + `ResolveSecret()`.
- **Secrets**: setec (production, over Tailscale) → env vars → literal config values. Secret names: `vimmary/postgres-password`, `vimmary/mistral-api-key`, `vimmary/claude-api-key`, `vimmary/karakeep-api-key`.
- **Database**: `pgxpool` with pgvector type registration via `AfterConnect`. Migrations via `golang-migrate`.
- **Auth**: Tailscale `tsnet.WhoIs()` → `GetOrCreateUser()`. Dev mode falls back to user_id=1.
- **MCP**: `mcp-go` library. Tools defined as `mcp.NewTool()`, handlers in `tools.go`. User ID injected via context.
- **HTTP**: `chi` router. MCP mounted at `/mcp`. REST API at `/api/v1/*`. Identity middleware wraps both.
- **Service layer**: `internal/service/` contains all business logic. MCP handlers and REST handlers both call service methods — never duplicate logic.
- **Init order**: Load config → start tsnet → init setec → resolve secrets → migrations → connect DB → create services.

## Key Commands

```bash
# Dev: start postgres
docker compose up db

# Dev: run server
go run ./cmd/vimmary --config config.yaml

# Build
CGO_ENABLED=0 go build -o vimmary ./cmd/vimmary

# Docker build (from Apple Silicon, target amd64)
docker buildx build --platform linux/amd64 -t meltforce/vimmary:edge --push .
```

## Database

- Uses `pgvector/pgvector:pg16` Docker image
- Videos table: `youtube_id` (unique), `transcript` (text), `summary` (text), `detail_level` (medium/deep), `embedding` (vector 1024), `metadata` (jsonb), `status` (pending/processing/completed/failed)
- Metadata JSONB: `{"topics": [...], "action_items": [...], "key_points": [...]}`
- Vector search via cosine similarity (HNSW index), hybrid search with RRF (keyword + semantic)
- Karakeep link: `karakeep_bookmark_id` for writeback reference

## Related Projects

- **meltkit** (`../meltkit/`) — Shared Go library. Provides pgvec, mistral, mcp, tsnet, setec, httpkit packages. Must be extracted from totalrecall before vimmary implementation starts.
- **totalrecall** (`../totalrecall/`) — Personal knowledge system. Same architecture pattern, primary source for meltkit extraction.
- **cast2md** (`../cast2md/`) — Podcast transcription service (Python). Similar domain (audio → transcript → search), but different stack. No code sharing, conceptual reference only.
- **Karakeep** — Self-hosted bookmarking app. Trigger source (webhooks) and writeback target (API). Runs at `karakeep.leo-royal.ts.net`.
- **tsmcp** (`../tsmcp/`) — OAuth/MCP proxy. vimmary MCP endpoint will be registered here.

## Relevant Documentation

- Karakeep API: `https://docs.karakeep.app/`
- Karakeep webhooks: Events `created`, `edited`, `crawled`, `ai tagged`, `deleted`. Payload: `{jobId, bookmarkId, userId, url, type, operation}`. Auth via Bearer token.
- yt-dlp: `https://github.com/yt-dlp/yt-dlp` — Caption extraction: `yt-dlp --write-auto-sub --sub-lang en,de --skip-download`
- mcp-go: `https://github.com/mark3labs/mcp-go`
- pgvector-go: `https://github.com/pgvector/pgvector-go`

## Dependencies

Same stack as totalrecall plus yt-dlp (system binary) and Karakeep API client:
- `go-chi/chi/v5`, `jackc/pgx/v5`, `mark3labs/mcp-go`, `pgvector/pgvector-go`
- `golang-migrate/migrate/v4`, `google/uuid`, `yaml.v3`, `tailscale.com`
- `tailscale/setec` — secret management over Tailscale
- Shared packages from `meltkit`

## Status

Backend implemented (Steps 1-8). Compiles and builds. Remaining:
- [ ] Web UI (Step 9): React + Vite frontend
- [ ] Deployment (Step 10): Ansible, tsmcp registration, setec secrets, Karakeep webhook registration
- [ ] End-to-end testing with real YouTube video
