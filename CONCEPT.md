# vimmary — YouTube Video Summary Service

## Problem

Zwei Apps (Recall + Karakeep) nötig, um YouTube-Videos zu bookmarken und Summaries zu erhalten. Ziel: Nur noch Karakeep nutzen, Summaries entstehen automatisch im Hintergrund.

## Lösung

Eigenständiger Go-Service, der per Karakeep-Webhook oder Bulk-Import getriggert wird. Erstellt automatisch Transkripte und Summaries für gebookmarkte YouTube-Videos. Ergebnisse werden in pgvector gespeichert, per MCP + Web UI durchsuchbar gemacht und nach Karakeep zurückgeschrieben.

## Flow

```
Karakeep ──webhook "created"──▶ vimmary
                     oder                │
Settings ──"Import bookmarks"──▶         │
                                  ▼
                           YouTube URL? ──nein──▶ ignorieren
                                  │ ja
                                  ▼
                        ┌── Processing Queue (10s Spacing) ──┐
                        │                                     │
                        ▼                                     │
                  Transkript holen                            │
                  (YouTube InnerTube API)                     │
                        │                                     │
                        ▼                                     │
                  Summary generieren (LLM)                    │
                        │                                     │
              ┌─────────┼─────────┐                           │
              ▼         ▼         ▼                           │
         pgvector    Karakeep    Web UI                       │
         speichern   updaten     anzeigen                     │
                     (note +                                  │
                     tag)                                     │
                        └─────────────────────────────────────┘
```

## Architektur-Entscheidungen

### Eigener Service (nicht cast2md-Erweiterung, nicht Karakeep-Plugin)
- cast2md ist konzeptionell "Podcasts" — Domäne nicht verwässern
- Karakeep ist Drittanbieter-Software — Customizing fragil bei Updates
- Eigener Service ist am saubersten und fokussiertesten

### Shared Go Module (`meltkit`)
- Gemeinsame Infrastruktur mit totalrecall in eigenem Repo
- Packages: `pkg/db`, `pkg/config`, `pkg/secrets`, `pkg/middleware`, `pkg/server`, `pkg/mcp`
- Versioniert via Go modules (`github.com/meltforce/meltkit`)

### LLM-Provider konfigurierbar
- Summaries: Mistral ODER Claude API — per Config + per User wählbar
- Embeddings: Mistral (`mistral-embed`, 1024-dim)
- Modelle: Dynamisch von Provider-APIs geladen, per User konfigurierbar

### Processing Queue
- Alle Video-Verarbeitung (Webhooks, Retries, Imports) läuft über eine zentrale Queue
- 10s Mindestabstand zwischen YouTube API Calls um 429 Rate Limiting zu vermeiden
- Buffered Channel (Kapazität 100), ein Worker

## Tech Stack

| Komponente           | Technologie                                  |
|----------------------|----------------------------------------------|
| Sprache              | Go                                           |
| HTTP                 | chi router + meltkit server                  |
| DB                   | PostgreSQL 16 + pgvector                     |
| Embeddings           | Mistral (`mistral-embed`, 1024-dim)          |
| Summary              | Claude API oder Mistral (konfigurierbar)     |
| Auth                 | Tailscale tsnet (multi-user)                 |
| Secrets              | setec (prod) → env vars → literal (dev)      |
| Transkript           | YouTube InnerTube API (native Go library)    |
| Search               | Hybrid: keyword + semantic mit RRF           |
| MCP                  | mcp-go, mounted at `/mcp`                    |
| Frontend             | React + Vite (embedded in Go binary)         |
| Deploy               | Docker Compose + GitHub Actions CI/CD        |

## Datenmodell

```sql
videos (
  id                    UUID PRIMARY KEY,
  user_id               INTEGER REFERENCES users(id),
  karakeep_bookmark_id  TEXT,
  youtube_id            TEXT,             -- UNIQUE(user_id, youtube_id)
  title                 TEXT,
  channel               TEXT,
  duration_seconds      INTEGER,
  language              TEXT,
  transcript            TEXT,
  summary               TEXT,
  detail_level          TEXT DEFAULT 'medium',
  summary_provider      TEXT,             -- "claude" oder "mistral"
  summary_model         TEXT,             -- konkretes Modell
  summary_input_tokens  INTEGER,
  summary_output_tokens INTEGER,
  embedding             vector(1024),
  metadata              JSONB,            -- {topics, action_items, key_points}
  status                TEXT DEFAULT 'pending',  -- pending/processing/completed/failed
  error_message         TEXT,
  created_at            TIMESTAMPTZ,
  updated_at            TIMESTAMPTZ
)

users (
  id              SERIAL PRIMARY KEY,
  tailscale_id    TEXT UNIQUE,
  webhook_token   TEXT UNIQUE,            -- per-user Bearer Token für Webhooks
  karakeep_api_key TEXT,                  -- per-user Karakeep API Key
  claude_model    TEXT,                   -- bevorzugtes Claude-Modell
  mistral_model   TEXT,                   -- bevorzugtes Mistral-Modell
  summary_prompt_medium TEXT,             -- Custom Prompt (medium)
  summary_prompt_deep   TEXT,             -- Custom Prompt (deep)
)
```

## Karakeep-Integration

### Webhook (Eingang)
- Events: `created` (neuer Bookmark) und `deleted` (Bookmark gelöscht)
- Auth: Per-user Bearer Token im `Authorization`-Header
- Nur YouTube-URLs werden verarbeitet, Rest wird ignoriert

### Bulk-Import
- `POST /api/v1/settings/karakeep/import` — importiert alle bestehenden YouTube-Bookmarks
- Paginiert über Karakeep API, filtert YouTube-URLs, queued neue Videos
- Bereits verarbeitete Videos werden übersprungen (Bookmark-ID wird ggf. nachgetragen)

### Writeback (Rückschreiben)
- Summary als `note` auf dem Bookmark (plain text, Markdown gestrippt)
- vimmary Detail-URL + Titel als Prefix
- Tag `video-summarized` setzen (POST, additiv — preserves AI tags)
- 30s Delay damit Karakeeps Crawler zuerst fertig wird

### Backlink
- Video-Detail zeigt "View in Karakeep" → `{base_url}/dashboard/preview/{bookmark_id}`

## Transkript-Strategie

- **YouTube InnerTube API** via native Go-Library (`youtube-transcript-api-go`)
- Konfigurierbare Sprach-Präferenzen (default: `en`, `de`)
- Manuelle Untertitel bevorzugt vor auto-generierten
- Kein Whisper-Fallback implementiert (bei YouTube selten nötig)

## Summary-Stufen

| Stufe      | Beschreibung                                          | Trigger                          |
|------------|-------------------------------------------------------|----------------------------------|
| `medium`   | 3-5 Absätze, Key Points, Action Items                 | Automatisch bei Webhook/Import   |
| `deep`     | Kapitelweise, Zitate, detaillierte Action Items       | Manuell via MCP-Tool oder Web UI |

## MCP Tools

1. `search_videos` — Hybrid Search (keyword + semantic) über Summaries und Transkripte
2. `get_video` — Einzelnes Video mit Transkript + Summary abrufen
3. `resummarize` — Summary neu generieren mit anderem Detail-Level, Sprache oder Provider
4. `list_recent` — Letzte Videos mit Filtern (Kanal, Sprache, Topics) + Pagination
5. `stats` — Aggregierte Statistiken (Counts, Channels, Topics, Daily Activity)

## Web UI

React + Vite, embedded in Go binary:

- **Video-Liste**: Letzte Videos mit Summary-Preview (Markdown gestrippt), Topics, Status
  - Suchfeld (Hybrid Search)
  - YouTube-URL direkt submitten
  - Pagination
  - Auto-Refresh bei processing Videos
- **Detail-Ansicht**: Formatierte Markdown-Summary
  - Buttons: "Copy as Markdown", "Download .md"
  - Links: "Watch on YouTube", "View in Karakeep"
  - Key Points, Action Items, Topics
  - Resummarize (Level, Sprache, Provider wählbar)
  - Transcript (collapsible)
  - Retry (bei Failed)
- **Stats-Seite**: Counts, Top Channels, Top Topics, Daily Activity
- **Settings-Seite**:
  - Karakeep API Key + Import-Button
  - Model-Auswahl (per Provider, dynamisch geladen)
  - Custom Summary Prompts (medium + deep)
  - Webhook-URL + Bearer Token (copy-paste ready)

## Deployment

- Docker Compose (Go App + pgvector) auf Proxmox LXC
- Tailscale-vernetzt (`vimmary.leo-royal.ts.net`)
- GitHub Actions CI/CD: Build → Test → Lint → Docker Push → SSH Deploy
- Secrets via setec über Tailscale
- Ansible-managed Config (`configuration/docker-stacks`)

## Repo-Struktur

```
github.com/meltforce/meltkit        ← Shared Go Library (db, config, secrets, middleware, server, mcp)
github.com/meltforce/totalrecall   ← Personal Knowledge System (importiert meltkit)
github.com/meltforce/vimmary       ← YouTube Summary Service (importiert meltkit)
```

## Volumen & Sprachen

- Max. 10 Videos pro Tag, typischerweise weniger
- Primär Englisch, manchmal Deutsch
- Nur klassische Videos (Talks, Tutorials) — keine Livestreams, Shorts, Playlists
