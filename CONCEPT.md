# vimmary — YouTube Video Summary Service

## Problem

Zwei Apps (Recall + Karakeep) nötig, um YouTube-Videos zu bookmarken und Summaries zu erhalten. Ziel: Nur noch Karakeep nutzen, Summaries entstehen automatisch im Hintergrund.

## Lösung

Eigenständiger Go-Service, der per Karakeep-Webhook getriggert wird. Erstellt automatisch Transkripte und Summaries für gebookmarkte YouTube-Videos. Ergebnisse werden in pgvector gespeichert, per MCP durchsuchbar gemacht und nach Karakeep zurückgeschrieben.

## Flow

```
Karakeep ──webhook "created"──▶ vimmary
                                  │
                                  ▼
                           YouTube URL? ──nein──▶ ignorieren
                                  │ ja
                                  ▼
                        Transkript holen
                        (yt-dlp captions, Whisper fallback)
                                  │
                                  ▼
                        Summary generieren (LLM)
                                  │
                        ┌─────────┼─────────┐
                        ▼         ▼         ▼
                   pgvector    Karakeep    Web UI
                   speichern   updaten     anzeigen
                               (note +
                               tag)
```

## Architektur-Entscheidungen

### Eigener Service (nicht cast2md-Erweiterung, nicht Karakeep-Plugin)
- cast2md ist konzeptionell "Podcasts" — Domäne nicht verwässern
- Karakeep ist Drittanbieter-Software — Customizing fragil bei Updates
- Eigener Service ist am saubersten und fokussiertesten

### Shared Go Module (`meltkit`)
- Gemeinsame Infrastruktur mit totalrecall wird in eigenes Repo extrahiert
- Zentrale Go-Library für alle zukünftigen Services
- Packages: pgvec, mistral, mcp, tsnet, setec, httpkit, …
- Versioniert via Go modules, eigenes Repo (`github.com/…/meltkit`)
- Services importieren nur benötigte Packages (`import "…/meltkit/pgvec"`)

### LLM-Provider konfigurierbar
- Summaries: Mistral ODER Claude API — per Config wählbar
- Embeddings: Mistral (`mistral-embed`, 1024-dim)
- Default: Claude API für Summaries (bessere Qualität bei langen Transkripten), Mistral für Embeddings

## Tech Stack

| Komponente           | Technologie                                  |
|----------------------|----------------------------------------------|
| Sprache              | Go                                           |
| HTTP                 | chi router                                   |
| DB                   | PostgreSQL 16 + pgvector                     |
| Embeddings           | Mistral (`mistral-embed`, 1024-dim)          |
| Summary              | Claude API oder Mistral (konfigurierbar)     |
| Auth                 | Tailscale tsnet                              |
| Secrets              | setec                                        |
| Transkript           | yt-dlp (YouTube Captions)                    |
| Transkript-Fallback  | Whisper (faster-whisper oder API)             |
| Search               | Hybrid: keyword + semantic mit RRF           |
| MCP                  | mcp-go, mounted at `/mcp`                    |
| Frontend             | Minimalistisches React/Vite (embedded in Go) |
| Deploy               | Docker + Komodo                              |

## Datenmodell

```sql
videos (
  id                    UUID PRIMARY KEY,
  user_id               INTEGER REFERENCES users(id),
  karakeep_bookmark_id  TEXT,
  youtube_id            TEXT UNIQUE,
  title                 TEXT,
  channel               TEXT,
  duration_seconds      INTEGER,
  language              TEXT,
  transcript            TEXT,
  summary               TEXT,
  detail_level          TEXT DEFAULT 'medium',
  embedding             vector(1024),
  metadata              JSONB,  -- {topics, action_items, key_points, tags}
  created_at            TIMESTAMPTZ,
  updated_at            TIMESTAMPTZ
)
```

## Karakeep-Integration

### Webhook (Eingang)
- Event: `created` (neuer Bookmark)
- Payload enthält `bookmarkId`, `url`, `type`, `operation`
- Payload ist minimal — Details via Karakeep REST API nachladen
- Auth: Bearer Token im `Authorization`-Header

### API (Rückschreiben)
- Summary als `note` auf dem Bookmark speichern
- Tag `video-summarized` setzen
- Karakeep REST API direkt aus Go aufrufen (kein Python-Wrapper)

## Transkript-Strategie

1. **YouTube Captions** (First Choice): `yt-dlp --write-auto-sub --sub-lang en,de --skip-download`
   - Englisch: Sehr gute Qualität
   - Deutsch: Brauchbar, Fehler bei Fachbegriffen möglich
   - Manuell hochgeladene Untertitel: Perfekt, wenn vorhanden
2. **Whisper** (Fallback): Nur wenn keine Captions existieren (selten bei YouTube)

## Summary-Stufen

| Stufe      | Beschreibung                                          | Trigger                          |
|------------|-------------------------------------------------------|----------------------------------|
| `medium`   | 3-5 Absätze, Key Points, Action Items                 | Automatisch bei Webhook          |
| `deep`     | Kapitelweise, Zitate, detaillierte Action Items       | Manuell via MCP-Tool oder Web UI |

## MCP Tools

1. `search_videos` — Semantic Search über Transkripte und Summaries
2. `get_video` — Einzelnes Video mit Transkript + Summary abrufen
3. `resummarize` — Summary neu generieren mit anderem Detail-Level oder Provider
4. `list_recent` — Letzte Videos mit Filtern (Kanal, Sprache, Topics)
5. `stats` — Aggregierte Statistiken

## Web UI

Minimalistisch, embedded in Go binary (React + Vite, wie totalrecall):

- **Video-Liste**: Letzte Videos mit Summary-Preview
- **Detail-Ansicht**: Formatierte Markdown-Summary
  - Button: "Copy as Markdown"
  - Button: "Download as Markdown"
  - Link zum Original-Video
- **Direkt-URL**: `https://vimmary.…/video/{youtube_id}` → formatierte Summary-Anzeige

## Deployment

- Docker Compose (Go App + pgvector)
- LXC-Container auf Proxmox
- Tailscale-vernetzt
- Komodo-managed
- Secrets via setec

## Repo-Struktur

```
github.com/meltforce/meltkit        ← Shared Go Library (pgvec, mistral, mcp, tsnet, setec, httpkit)
github.com/meltforce/totalrecall   ← Personal Knowledge System (importiert meltkit)
github.com/meltforce/vimmary       ← YouTube Summary Service (importiert meltkit)
```

## Volumen & Sprachen

- Max. 10 Videos pro Tag, typischerweise weniger
- Primär Englisch, manchmal Deutsch
- Nur klassische Videos (Talks, Tutorials) — keine Livestreams, Shorts, Playlists

## Offene Punkte (für Implementierung)

- [ ] `meltkit` Repo aufsetzen, gemeinsamen Code aus totalrecall extrahieren
- [ ] Whisper-Fallback: Lokaler Whisper oder API-basiert?
- [ ] Summary-Prompts designen (medium + deep)
- [ ] Karakeep Webhook in der UI registrieren
- [ ] Entscheidung: Braucht die Web UI Suche, oder reicht MCP?
