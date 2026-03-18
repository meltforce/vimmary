# RSS-Feed für vimmary — Konzept

## Ziel

Fulltext-RSS-Feed bereitstellen, über den ein RSS-Reader (z.B. Miniflux, Reeder, Feedly) die vimmary-Summaries und Transkripte als Artikel darstellen kann. Kein Extra-Aufwand mehr nötig — neues Video verarbeitet, Feed aktualisiert sich automatisch.

## Herausforderung: Authentifizierung

vimmary nutzt Tailscale tsnet für Auth. RSS-Reader können aber kein Tailscale-Auth. Lösung: **Token-basierter Zugriff** — das gleiche Pattern wie der Karakeep-Webhook.

Jeder User hat bereits ein `webhook_token` in der DB. Dieses Token wird auch für den RSS-Feed verwendet (oder optional ein separates `feed_token`).

### Variante A: Webhook-Token wiederverwenden (empfohlen)

- Kein DB-Schema-Change nötig
- Einfach: ein Token für Webhook + Feed
- URL: `/feed/rss?token=<webhook_token>`

### Variante B: Separates Feed-Token

- Neues Feld `feed_token` in `users`-Tabelle
- Pro: Feed unabhängig vom Webhook widerrufbar
- Contra: Mehr Komplexität, Migration nötig

**Empfehlung:** Variante A — bei Bedarf später auf Variante B erweitern.

## Feed-Format

**Atom 1.0** (statt RSS 2.0):
- Bessere Spezifikation, sauberer XML
- Breitere UTF-8-Unterstützung (relevant für mehrsprachige Transkripte)
- Jeder gängige RSS-Reader unterstützt Atom

## Endpoint

```
GET /feed/atom?token=<webhook_token>
```

- Außerhalb der Tailscale-Auth-Middleware (wie `/webhook/karakeep`)
- Token als Query-Parameter (RSS-Reader unterstützen keine Header-Auth)
- Kein Rate-Limiting nötig (read-only, persönlicher Gebrauch)

## Feed-Inhalt

### Feed-Metadaten

| Feld       | Wert                                      |
|------------|-------------------------------------------|
| Title      | `vimmary — Video Summaries`               |
| Subtitle   | `AI-generated summaries of YouTube videos`|
| Author     | User-Login (Tailscale)                    |
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

<h2>Transcript</h2>
<details>
  <summary>Full transcript (click to expand)</summary>
  <p><!-- transcript als plain text, whitespace-preserving --></p>
</details>

<p><a href="https://youtube.com/watch?v=...">Watch on YouTube</a></p>
```

> **Hinweis:** Ob das `<details>`-Tag im RSS-Reader gerendert wird, hängt vom Reader ab. Als Fallback wird das Transkript einfach als Block angezeigt — auch akzeptabel.

## Pagination / Limits

- Default: **50 neueste Videos** (sortiert nach `created_at DESC`)
- Optional Query-Parameter: `?limit=100` (max 200)
- Kein Paging via `rel="next"` — bei max. 10 Videos/Tag reichen 50 Einträge für ~1 Woche

## Implementierung

### Backend (Go)

#### 1. Neues Package `internal/feed/`

```
internal/feed/
├── atom.go        # Atom-XML-Generierung
└── handler.go     # HTTP-Handler
```

**`atom.go`:**
- Struct-Definitionen für Atom-Feed (oder `encoding/xml` direkt)
- Funktion `BuildFeed(videos []storage.Video, baseURL string) ([]byte, error)`
- Markdown → HTML-Konvertierung der Summary (z.B. via `gomarkdown/markdown` oder `goldmark`)

**`handler.go`:**
- `HandleAtomFeed(store *storage.DB, svc *service.Service) http.HandlerFunc`
- Token aus Query validieren → User-ID ermitteln
- Videos laden via bestehende `ListRecent()` (Filter: `status=completed`)
- Feed generieren und als `application/atom+xml` zurückgeben

#### 2. Route registrieren

In `internal/server/server.go`:

```go
// Feed route — no Tailscale auth, uses per-user token (like webhook)
r.Get("/feed/atom", feed.HandleAtomFeed(s.store, s.svc))
```

#### 3. Keine DB-Migration nötig

Alles basiert auf bestehenden Daten (videos + users.webhook_token).

### Frontend (React)

#### Settings-Seite erweitern

Neben dem bestehenden Webhook-URL-Bereich einen **RSS-Feed-Bereich** hinzufügen:

```
┌─────────────────────────────────────────┐
│ RSS Feed                                │
│                                         │
│ Feed URL:                               │
│ ┌─────────────────────────────────┐     │
│ │ https://vimmary.../feed/atom?...│ 📋  │
│ └─────────────────────────────────┘     │
│                                         │
│ Copy this URL into your RSS reader      │
│ to get fulltext summaries delivered     │
│ automatically.                          │
└─────────────────────────────────────────┘
```

- Feed-URL dynamisch aus Webhook-Token + Base-URL zusammensetzen
- Copy-Button (wie bei Webhook-URL)
- Kein neuer API-Endpoint nötig — Token ist bereits auf der Settings-Seite vorhanden

## Abhängigkeiten

| Dependency       | Zweck                      | Bereits vorhanden? |
|------------------|----------------------------|--------------------|
| `encoding/xml`   | Atom-XML generieren        | Go stdlib          |
| `goldmark`       | Markdown → HTML            | Nein, neu          |

> `goldmark` ist die Standard-Go-Markdown-Library, klein und ohne CGO-Dependencies.

## Nicht im Scope

- **Feed-Discovery** (`<link rel="alternate" type="application/atom+xml">` im HTML-Head): Wäre nett, aber nicht nötig bei persönlichem Gebrauch
- **Conditional GET** (`If-Modified-Since`, `ETag`): Overkill für persönliches Volumen
- **Webhook/Push-Notification** an RSS-Reader (WebSub): Unnötig, Reader pollen sowieso
- **Mehrere Feeds** (z.B. pro Channel, pro Topic): Kann später ergänzt werden via Query-Filter

## Offene Fragen

1. **Transkript im Feed?** Das Transkript kann sehr lang sein (10.000+ Wörter). Soll es standardmäßig enthalten sein oder optional (`?transcript=true`)? Empfehlung: Standardmäßig ja, da "Fulltext-RSS" das Ziel ist.

2. **Separates Feed-Token?** Aktuell reicht das Webhook-Token. Falls der Feed jemals öffentlich/geteilt werden soll, wäre ein separates Token sicherer.

## CONCEPT.md-Ergänzung

Nach Implementierung den folgenden Abschnitt in CONCEPT.md ergänzen:

```markdown
## RSS Feed

- Atom 1.0 Feed unter `/feed/atom?token=<webhook_token>`
- Fulltext: Summary (HTML) + Key Points + Action Items + Transkript
- Außerhalb Tailscale-Auth (Token-basiert, wie Webhook)
- 50 neueste completed Videos, konfigurierbar via `?limit=N`
```
