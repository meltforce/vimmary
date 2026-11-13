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
- **Podcast backfill** — subscribing summarizes the 3 newest episodes by default; per feed you can pull the last N, summarize the whole back catalogue, or ask cast2md to transcribe the rest of the feed. None of it disturbs the running poll.
- **MCP server** — 5 tools for searching and browsing summaries of both kinds
- **RSS feeds** — three Atom feeds per user (videos, podcasts, both), with per-user feed tokens for authentication
- **Web UI** — React frontend embedded in the Go binary (Videos, Inbox, Podcasts, Channels, Stats, Settings)
- **Tailscale identity** — on a tailnet (tsnet), the calling device's Tailscale login is the account; no password
- **Login** (optional `auth:` block) — sessions with local passwords, an OIDC provider, or both; works without Tailscale. User management, account policy and personal access tokens in Settings
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
| Embeddings  | OpenAI `text-embedding-3-large` at 1024 dims via Aperture, or Mistral `mistral-embed` (configurable) |
| Summaries   | Claude API (directly or via Aperture) or Mistral (configurable) |
| Auth        | Tailscale tsnet; optional login via meltkit `pkg/auth` |
| Secrets     | LLM API keys in the database (Settings → LLM); DB password and OIDC client secret from the environment or `config.yaml` |
| Transcripts | YouTube InnerTube API (native Go)     |
| Search      | Hybrid: keyword + semantic with RRF   |
| MCP         | mcp-go, HTTP + stdio transports       |
| Frontend    | React + Vite (embedded in Go binary)  |

## Quick start

### Prerequisites

- Docker and Docker Compose
- A **Mistral API key** for embeddings ([console.mistral.ai](https://console.mistral.ai)), unless embeddings run through Aperture (`embedding.provider: openai`)
- A **Claude API key** or the Mistral key for summaries

The keys are not part of the configuration: they are entered in **Settings →
LLM** once vimmary runs, stored in the database and used without a restart.

### 1. Create a project directory

```bash
mkdir vimmary && cd vimmary
```

### 2. Fetch the compose file

```bash
curl -LO https://raw.githubusercontent.com/meltforce/vimmary/main/compose.example.yml
```

It runs `ghcr.io/meltforce/vimmary:latest` alongside pgvector. (`docker-compose.yml`
in the repo is the *development* stack — it builds the working tree.)

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

# tsnet is on by default and would wait for a Tailscale login. This setup
# serves plain HTTP on server.port instead; see "Access and login".
tailscale:
  enabled: false

summary:
  provider: "claude"          # "claude" or "mistral"; Settings → LLM overrides it

# Optional, for a tailnet with a Tailscale Aperture gateway. Claude then needs
# no API key, and embedding.provider "openai" becomes available.
# aperture:
#   base_url: "http://ai.<tailnet>.ts.net"
# embedding:
#   provider: "openai"        # "mistral" (default) or "openai"; a change
#                             # re-embeds the library in the background

youtube:
  sub_langs: [en]             # caption preference after the spoken language

secrets:
  postgres_password: "vimmary"   # or VIMMARY_POSTGRES_PASSWORD
```

`config.example.yaml` in the repo lists every key with its default.

Host, port and database settings can be overridden from the environment:
`VIMMARY_SERVER_HOST`, `VIMMARY_SERVER_PORT`, `VIMMARY_DB_HOST`,
`VIMMARY_DB_PORT`, `VIMMARY_DB_NAME`, `VIMMARY_DB_USER`, `VIMMARY_DB_SSLMODE`,
`VIMMARY_TS_ENABLED`, `VIMMARY_TS_HOSTNAME`, `VIMMARY_TS_STATE_DIR`. The two
secrets read at startup, `postgres_password` and `oidc_client_secret`, come
from `VIMMARY_POSTGRES_PASSWORD` / `VIMMARY_OIDC_CLIENT_SECRET` first and from
`secrets:` in `config.yaml` otherwise.

### 4. Start

```bash
# Postgres reads its password from .env; it has to match secrets.postgres_password
echo "POSTGRES_PASSWORD=vimmary" > .env
docker compose -f compose.example.yml up -d
```

Open `http://localhost:8080`, enter the API keys under **Settings → LLM**, and
start adding videos. Migrations run automatically on startup.

This configuration has **no access control**: without Tailscale and without an
`auth:` block every request acts as the built-in user `local`, who is admin.
That is fine on `localhost`; the compose file publishes port 8080 on all
interfaces, so anything reachable beyond the machine needs a login (next
section) or Tailscale.

## Access and login

Who a request belongs to depends on two settings: whether vimmary runs as a
Tailscale node (`tailscale.enabled`, default `true`) and whether
`config.yaml` has an `auth:` block.

| Tailscale | `auth:` | Who is the user |
|---|---|---|
| on | — | The calling device's Tailscale login (WhoIs). Each tailnet user gets their own library; nothing to sign in to. |
| on | yes | A session (password or OIDC) or a personal access token; a tailnet request without either still falls back to WhoIs. |
| off | yes | A session or a personal access token. Anything else gets 401, and the web UI goes to `/login`. |
| off | — | Nobody is checked: every request is user `local` (user 1). Local development and single-user `localhost` only. |

With Tailscale on, vimmary listens only on the tailnet (`https://<hostname>.<tailnet>.ts.net`,
TLS from Tailscale); `server.port` is not opened. Without it, vimmary serves
plain HTTP on `server.host:server.port`; put a TLS-terminating reverse proxy in
front for anything but `localhost`.

### Password login

```yaml
auth:
  base_url: "https://vimmary.example.com"   # public origin; http only on localhost
  methods: [password]
  # session_ttl: 720h
  # trusted_proxies: ["10.0.0.0/8"]         # proxies whose X-Forwarded-For is read
```

The first admin is created on the command line; the password is prompted for,
or read from stdin:

```bash
docker compose -f compose.example.yml exec app ./vimmary --config /data/config.yaml auth create-user --admin <username>
```

Further users are created by an admin under **Settings → Users**.

### OIDC login

```yaml
auth:
  base_url: "https://vimmary.example.com"
  methods: [oidc]                 # or [oidc, password]
  initial_admin: "you@example.com"  # the email of the first admin
  oidc:
    issuer: "https://idp.example.com"
    client_id: "vimmary"
    client_secret: ""             # or VIMMARY_OIDC_CLIENT_SECRET
    display_name: "Single sign-on"
```

Register `<base_url>/auth/oidc/callback` as the redirect URI at the provider.
The first login of `initial_admin` creates the admin account. Whether a first
login of anyone else creates an account (optionally limited to email domains)
or needs an admin to create it first is the account policy under **Settings →
Users**. With OIDC as the only method, `/login` hands over to the provider
directly.

### Sessions and tokens

The nav carries a sign-out button whenever the browser has a session; Settings
→ Account lists the sessions of every browser, the linked logins and the
password. A request recognised through Tailscale has no session, so there is
nothing to sign out of.

Personal access tokens (`vimmary_pat_…`) exist only with an `auth:` block.
They carry a scope: `api` for the REST API under `/api/v1`, `mcp` for `/mcp`.
A token is created under Settings → Account or with `vimmary auth create-token`
(see [MCP configuration](#mcp-configuration)).

### Local development

```bash
# Start only the database
docker compose up db

# Run the backend (Go version from go.mod)
go run ./cmd/vimmary --config config.yaml

# Run the frontend with hot-reload (separate terminal)
cd web && npm install && npm run dev
```

## Setup Karakeep integration

1. Open vimmary's **Settings → Karakeep** page
2. Enter your **Karakeep API key** (from Karakeep Settings → API Keys)
3. Copy the generated **Webhook URL** and **Bearer Token**
4. In Karakeep Settings → Webhooks, create webhooks for `created` and `deleted` events
5. Karakeep must be able to reach the webhook URL. The URL authenticates with its bearer token, not with Tailscale or a login. If Karakeep runs in Docker and vimmary is on Tailscale, add `CRAWLER_ALLOWED_INTERNAL_HOSTNAMES=.your-tailnet.ts.net` to Karakeep's env to allow webhook delivery

## Submit a video without Karakeep

`GET /submit?url=<video URL>` queues a video for the signed-in user and
answers a redirect to the list, which makes it usable as a share-sheet target,
an iOS Shortcut, or a bookmarklet. It authenticates like the web UI: the
browser's session, or the device's Tailscale login on a tailnet. Without
either, it redirects to `/login` and comes back afterwards.

```
https://vimmary.<tailnet>.ts.net/submit?url=https://youtu.be/dQw4w9WgXcQ
```

Nothing is written back to Karakeep for a video submitted this way, because
there is no bookmark to write to.

The web UI's own form posts `{"url": "..."}` to `POST /api/v1/videos`, which is
the same path for a JSON client. It answers `202` with the `youtube_id` only,
because the row is inserted when the worker picks the job up;
`GET /api/v1/videos?youtube_id=<id>` returns that row once it exists, and its
`id` is the page `/video/<id>`.

## Setup podcast summaries

Podcast transcripts come from [cast2md](https://github.com/meltforce/cast2md).
vimmary polls it; cast2md never calls vimmary. Both services must be on the same
tailnet, because cast2md has no authentication and is reachable only there.

1. In vimmary's config, set `cast2md.enabled: true` and `cast2md.base_url` to the cast2md host
2. Restart vimmary. Its poller starts 30 seconds later and does nothing until a feed is subscribed
3. Open **Settings → Podcasts**, tick the feeds you want and pick a detail level per feed

Subscribing summarizes the feed's **3 newest** transcribed episodes and follows
along from there. That first poll runs when you tick the box, not at the next
tick, so the summaries are queued within seconds. The number is per feed —
`on subscribe: none` restores plain "from now on", where the first poll only
records a watermark. Either way the watermark ends up at the newest episode of
that first batch, so nothing is summarized twice. Switching a feed off keeps its
watermark, so switching it back on later fetches the gap through the ordinary
poll.

Three ways to reach older episodes, none of which moves the watermark:

| Action | Does |
|---|---|
| **Backfill** (last 5/10/25/50) | Summarizes that many of the newest transcribed episodes. |
| **Summarize all** | Summarizes every episode cast2md has a transcript for. Only model calls. |
| **Transcribe all** | Asks cast2md to download and transcribe the rest of the feed. They appear here as they finish, through the ordinary poll — but only if the feed is subscribed. |

**Summarize all** and **Transcribe all** each state their episode count and ask
before running. Transcribe all is the one action that makes cast2md do work
rather than just reading from it; on a large feed it can mean hundreds of
downloads and Whisper runs.

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

Each user has their own feed token. The token is the only authentication — the feed URL needs neither Tailscale nor a login, so it works with any RSS reader.

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

Without an `auth:` block, `/mcp` identifies the caller by Tailscale like the
web UI. With one, it takes a personal access token with scope `mcp`
(`Authorization: Bearer vimmary_pat_…`); a browser session does not count, and
tailnet callers without a token are still identified by Tailscale while tsnet
runs. A token comes from Settings → Account or from the command line:

```bash
vimmary --config config.yaml auth create-token --user <login> --name tsmcp --scope mcp [--expires 90d]
```

A token with scope `api` is accepted by the REST API under `/api/v1`.

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

Stdio mode has no authentication and acts as user 1 (`local`).

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

## Checks

| Command | What it checks |
|---|---|
| `scripts/setup` | installs the toolchain (Go, golangci-lint, govulncheck, Node, npm dependencies); runs at session start |
| `scripts/test` | Go tests with the race detector, the storage and service tests against Postgres + pgvector (`TEST_DATABASE_URL`, or `compose.test.yaml`), the frontend tests |
| `scripts/lint` | gofmt, go vet, golangci-lint, govulncheck, the migration rules (`scripts/check-migrations.sh`), the script tests, the frontend type check and build |
| `scripts/e2e` | the built image with pgvector and the fixture data: `/version` and the loopback `/healthz` report the SHA, the API and the frontend answer, the Playwright tests in `e2e/` pass; upgrade from the image of `main` and back |

CI runs `test`, `lint` and `e2e` on the head commit of every pull request,
and `review` (Claude, rules in `REVIEW.md`); all four are required checks.
A merge to `main` deploys to production without a further step; production
compose and `config.yaml` live in the homelab repo.

Every push to a `claude/**` branch is built and deployed to the preview host
(`preview.yml`, environment `preview`): https://knox.coydog-fence.ts.net:8091/,
tailnet only, fixture data (`seed_fixtures: true`). The newest push wins; the
PR gets a comment with the result.

A tag `v*` publishes `ghcr.io/meltforce/vimmary:<tag>` and `:latest` and a
GitHub release (`release.yml`).

## Related projects

- [meltkit](https://github.com/meltforce/meltkit) — shared Go library (db, config, secrets, middleware, MCP)
- [totalrecall](https://github.com/meltforce/totalrecall) — personal knowledge system (architectural blueprint)

