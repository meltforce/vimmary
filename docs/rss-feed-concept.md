# RSS-Feed für vimmary — Konzept

## Ziel

RSS-Feed bereitstellen, über den ein RSS-Reader (z.B. Miniflux, Reeder, Feedly) die vimmary-Summaries als Artikel darstellen kann. Kein Extra-Aufwand mehr nötig — neues Video verarbeitet, Feed aktualisiert sich automatisch.

## Zugriffschutz: Unratbares Token in der URL

Kein Auth im klassischen Sinne. Stattdessen enthält die Feed-URL ein **zufällig generiertes, kryptographisch sicheres Token** (`feed_token`), das als einziger Zugriffschutz dient. Wer die URL kennt, hat Zugriff — wer sie nicht kennt, kann sie nicht erraten.

### Token-Anforderungen

- **32 Byte Zufall** (64 Hex-Zeichen) via `crypto/rand` — nicht erratbar
- **Unveränderlich:** Einmal generiert, wird das Token nie geändert (damit RSS-Reader-URLs stabil bleiben)
- **Unique Constraint** in der DB — garantiert, dass kein Token je doppelt vergeben wird
- **Lazy-generiert:** Wird beim ersten Zugriff auf die Settings-Seite erzeugt (wie `webhook_token`)
- **Separates Feld** `feed_token` in der `users`-Tabelle — unabhängig vom Webhook-Token

### Warum separates Token statt Webhook-Token?

- Feed-URL wird in Drittanbieter-Apps (RSS-Reader) hinterlegt — anderer Vertrauenskontext
- Webhook-Token hat Schreibzugriff (löst Verarbeitung aus), Feed-Token nur Lesezugriff
- Unabhängig widerrufbar falls nötig

## Feed-Format

**Atom 1.0** (statt RSS 2.0):
- Bessere Spezifikation, sauberer XML
- Breitere UTF-8-Unterstützung (relevant für mehrsprachige Summaries)
- Jeder gängige RSS-Reader unterstützt Atom

## Endpoint

```
GET /feed/atom/<feed_token>
```

- Außerhalb der Tailscale-Auth-Middleware (öffentlich erreichbar, Token ist der Schutz)
- Token als Pfad-Segment (sauberer als Query-Parameter, kein Risiko von URL-Trunkierung)
- Read-only, kein Rate-Limiting nötig (persönlicher Gebrauch, max ~10 Videos/Tag)

## Feed-Inhalt

### Feed-Metadaten

| Feld       | Wert                                      |
|------------|-------------------------------------------|
| Title      | `vimmary — Video Summaries`               |
| Subtitle   | `AI-generated summaries of YouTube videos`|
| Link       | vimmary Base-URL                          |
| Updated    | Timestamp des neuesten Videos             |
| ID         | vimmary Base-URL + `/feed/atom`           |

### Entry pro Video (nur `status = completed`)

| Feld        | Quelle                                              |
|-------------|------------------------------------------------------|
| ID          | Video-UUID als URN (`urn:uuid:<id>`)                 |
| Title       | `[Channel] Video-Titel`                              |
| Link        | YouTube-URL (`https://youtube.com/watch?v=<id>`)     |
| Published   | `created_at`                                         |
| Updated     | `updated_at`                                         |
| Categories  | Topics aus `metadata.topics`                         |
| Summary     | Erste 200 Zeichen der Summary (plain text)           |
| Content     | **Fulltext** (siehe unten)                           |

### Fulltext-Content (HTML)

Der Content enthält die Summary und strukturierte Metadaten — **kein Transkript**.

```html
<h2>Summary</h2>
<div><!-- Summary als HTML (Markdown → HTML konvertiert) --></div>

<h2>Key Points</h2>
<ul>
  <li><!-- aus metadata.key_points --></li>
</ul>

<h2>Action Items</h2>
<ul>
  <li><!-- aus metadata.action_items --></li>
</ul>

<p><a href="https://youtube.com/watch?v=...">Watch on YouTube</a></p>
```

Key Points und Action Items werden nur gerendert, wenn sie in der Metadata vorhanden und nicht leer sind.

## Pagination / Limits

- Default: **50 neueste Videos** (sortiert nach `created_at DESC`)
- Optional Query-Parameter: `?limit=100` (max 200)
- Kein Paging via `rel="next"` — bei max. 10 Videos/Tag reichen 50 Einträge für ~1 Woche

## Implementierung

### DB-Migration

Neues Feld in `users`-Tabelle:

```sql
ALTER TABLE users ADD COLUMN feed_token TEXT UNIQUE;
```

### Backend (Go)

#### 1. Neues Package `internal/feed/`

```
internal/feed/
├── atom.go        # Atom-XML-Generierung
└── handler.go     # HTTP-Handler
```

**`atom.go`:**
- Struct-Definitionen für Atom-Feed via `encoding/xml`
- Funktion `BuildFeed(videos []storage.Video, baseURL string) ([]byte, error)`
- Markdown → HTML-Konvertierung der Summary via `goldmark`

**`handler.go`:**
- `HandleAtomFeed(store *storage.DB, svc *service.Service) http.HandlerFunc`
- Token aus URL-Pfad extrahieren → User-ID ermitteln via `store.GetUserByFeedToken()`
- Videos laden via bestehende `ListRecent()` (Filter: `status=completed`)
- Feed generieren und als `application/atom+xml` zurückgeben
- 404 bei ungültigem Token (kein Unterschied zu "nicht gefunden" — kein Info-Leak)

#### 2. Storage-Erweiterung

In `internal/storage/users.go`:

```go
func (db *DB) GetOrCreateFeedToken(ctx context.Context, userID int) (string, error)
func (db *DB) GetUserByFeedToken(ctx context.Context, token string) (int, error)
```

Gleiches Pattern wie `GetOrCreateWebhookToken` / `GetUserByWebhookToken`.

#### 3. Route registrieren

In `internal/server/server.go`:

```go
// Feed route — no Tailscale auth, token in URL path is the access control
r.Get("/feed/atom/{token}", feed.HandleAtomFeed(s.store, s.svc))
```

### Frontend (React)

#### Settings-Seite erweitern

Neben dem bestehenden Webhook-URL-Bereich einen **RSS-Feed-Bereich** hinzufügen:

```
┌─────────────────────────────────────────┐
│ RSS Feed                                │
│                                         │
│ Feed URL:                               │
│ ┌─────────────────────────────────┐     │
│ │ https://vimmary.../feed/atom/...│  📋 │
│ └─────────────────────────────────┘     │
│                                         │
│ Copy this URL into your RSS reader to   │
│ receive video summaries automatically.  │
└─────────────────────────────────────────┘
```

- Neuer API-Endpoint `GET /api/v1/settings/feed` liefert das Feed-Token (lazy-generiert)
- Feed-URL aus Token + `window.location.origin` zusammensetzen
- Copy-Button (wie bei Webhook-URL)

## Abhängigkeiten

| Dependency       | Zweck                      | Bereits vorhanden? |
|------------------|----------------------------|--------------------|
| `encoding/xml`   | Atom-XML generieren        | Go stdlib          |
| `goldmark`       | Markdown → HTML            | Nein, neu          |

> `goldmark` ist die Standard-Go-Markdown-Library, klein und ohne CGO-Dependencies.

## Nicht im Scope

- **Transkript im Feed:** Bewusst ausgeschlossen — Summary + Metadaten reichen
- **Feed-Discovery** (`<link rel="alternate">`): Nicht nötig bei persönlichem Gebrauch
- **Conditional GET** (`ETag`, `If-Modified-Since`): Overkill für persönliches Volumen
- **WebSub/Push:** Unnötig, Reader pollen sowieso
- **Mehrere Feeds** (pro Channel/Topic): Kann später via Query-Filter ergänzt werden
- **Token-Rotation:** Aktuell nicht vorgesehen — Token ist unveränderlich

## CONCEPT.md-Ergänzung

Nach Implementierung den folgenden Abschnitt in CONCEPT.md ergänzen:

```markdown
## RSS Feed

- Atom 1.0 Feed unter `/feed/atom/<feed_token>`
- Fulltext: Summary (HTML) + Key Points + Action Items
- Kein Transkript im Feed
- Zugriff über kryptographisches Token in der URL (kein Auth nötig)
- Separates `feed_token` pro User (unabhängig vom Webhook-Token)
- 50 neueste completed Videos, konfigurierbar via `?limit=N`
```
