# Decisions

Decisions taken about vimmary, with the reasoning that led to them. One section
per decision, newest first.

A decision belongs here once it has been made — including decisions to *not* do
something, which are the ones most likely to be re-derived from scratch
otherwise. Open work lives in [`ROADMAP.md`](ROADMAP.md); postmortems live in
[`INCIDENTS.md`](INCIDENTS.md).

Structure per entry: **decision**, **reasoning**, **trigger to re-open**, and a
**revisions** log when the decision has changed. A revised decision is edited in
place with the old form recorded under revisions — the entry is not duplicated.

The entries below were reconstructed on 2026-08-06 from `CONCEPT.md`,
`docs/rss-feed-concept.md` and code comments, all of which predate this file.
Both documents were removed once their content had been distilled here; the last
revision containing them is `34d8a62`. Each decision date is the date of the
commit that implemented it, established with `git log -S` on the relevant
identifier rather than estimated.

---

## 2026-08-23 — topic upkeep is an LLM merge plus a reuse hint, not a taxonomy

**Decided:** 2026-08-23

**Decision.** Two mechanisms keep the topic tags from sprawling. Forward: every
summarize call carries the library's 40 most-used tags with the instruction to
reuse one where it fits (`summary.WithTopicReuseHint`, prepended so custom
prompts get it too). Backward: `POST /api/v1/videos/consolidate-topics`
(Settings → Summaries → "Consolidate topics") sends every tag with its usage
count to the configured provider, gets back an old→new merge mapping, and
rewrites `metadata->'topics'` on the affected rows. Summaries, key points and
embeddings stay untouched — embeddings are built from the summary text, not
the tags.

**Reasoning.** The tags are LLM-generated per video with no shared vocabulary,
so the set grew a near-synonym per video — measured on 2026-08-23: most tags
carried exactly one video, which makes them worthless for navigation. The
alternatives were worse: manual tag management is the part of Play the
operator rejected, and string heuristics (case, plural, hyphens) miss exactly
the merges that matter (translations, synonyms). The model that created the
tags is the right tool to merge them.

**Mechanics worth keeping:** the mapping is sanitized before it is applied —
targets must be existing tags, identity entries drop, chains flatten to their
terminal target, and a cycle drops every entry on it (the model contradicted
itself there; merging in arbitrary order would encode the contradiction).
`Summarizer` gained a generic `Complete(ctx, prompt, maxTokens)` for this —
the summary JSON contract does not fit a maintenance call.

**Trigger to re-open:** the reuse hint failing to hold the set stable (then
the hint needs to become a constraint, e.g. a closed vocabulary per run); a
consolidation run merging tags that should have stayed apart.

---

## 2026-08-23 — library navigation runs on channels and LLM topics, not manual tags

**Decided:** 2026-08-23

**Decision.** The two library screens gain a facet row: a channel select and
the LLM topic chips, fed by `GET /api/v1/videos/facets` (channels and topics
of completed rows, with counts). Selecting one sets the existing list-query
parameters, persisted in the URL. The inbox filters by followed channel the
same way. There are no user-managed tags.

**Reasoning.** The list API filtered by channel and topic since the podcast
split, but no UI control existed and the sort is fixed — the library was one
long newest-first feed. With channel subscriptions there is finally a
structure worth navigating. The two dimensions that need no upkeep already
exist: the channel column and the LLM-derived topics. Manual tags are the
part of Play the operator explicitly rejected (see the channels-inbox entry
above), so navigation must not reintroduce them through the back door.

**`channel_exact` next to the ILIKE `channel` parameter.** The existing
`channel` filter is a partial match, which is right for a typed search and
wrong for a facet: its values come from the column itself, and ILIKE would
conflate "Go" with "Google". The facet UI sends `channel_exact`; the old
parameter keeps its meaning for existing clients and MCP.

**Not done, deliberately:** a clickable channel name inside a feed row — the
row is one `<a>` to the detail page, and an interactive element inside an
interactive element fails both HTML and keyboard navigation. The select
covers the need.

**Trigger to re-open:** facet lists growing past what a select and a chip
row can carry (a channel page per channel would be the next shape); a want
for sort options.

---

## 2026-08-23 — the channels inbox: RSS polling, video-ID dedup, and no watch status

**Decided:** 2026-08-23

**Decision.** Users follow YouTube channels; new videos land in an inbox
(`/inbox`) for triage — summarize, watch (summarize and navigate to the
detail page, where the transcript player lives), or dismiss. Subscriptions
are managed under Settings → Channels. Two tables (migration `000015`):
`channel_subscriptions` and `inbox_items`, modeled on the podcast
subscription pattern.

**Reasoning, per part:**

- **Discovery via the public per-channel RSS feed**
  (`youtube.com/feeds/videos.xml?channel_id=UC…`, the newest ~15 entries),
  parsed with `encoding/xml` — no API key, no new dependency, and the poller
  never touches InnerTube, so following many channels puts zero pressure on
  the transcript pipeline's `adaptiveDelay` budget. InnerTube is reached only
  when an item is actually summarized, through the existing queue.
- **Channel resolution at subscribe time.** A `/channel/UC…` URL is parsed
  directly; an @handle or /c//user/ URL costs one fetch of the channel page,
  extracting `"channelId":"UC…"` plus `og:title`/`og:image` — the same
  scraping approach `FetchMetadata` already uses for watch pages.
- **Dedup by video ID, not a timestamp watermark.** RSS reorders entries and
  republishes them on edits, so a published-time watermark can skip or
  duplicate; `UNIQUE (user_id, youtube_id)` with `ON CONFLICT DO NOTHING` is
  exact. The inbox table itself is the seen-set: dismissed and queued rows
  stay, because deleting one still inside the feed's window would resurrect
  it. No retention job in v1 — rows are ~200 bytes and accumulate slowly;
  the trigger is visible growth (ROADMAP row).
- **A video already in the library never becomes an inbox item** — the
  poller skips entries `GetByYouTubeID` finds, in any status. Absence is the
  marker.
- **Subscribing imports the current feed window synchronously**, mirroring
  `SetPodcastSubscription` running its first poll inside the PUT. Dedup makes
  the first and every later poll the same operation, so there is no
  `initialized` flag.
- **"Watch" and "summarize" are one backend action** (operator decision):
  `SummarizeInboxItem` creates the row synchronously via `EnsureVideoRow` —
  the UI needs the UUID to navigate — and `ProcessVideo`'s existing-row
  branch adopts it. Side effect: inbox-sourced YouTube rows carry
  `published_at` and a title from day one.
- **Shorts are filtered by title heuristic only** (`#shorts`,
  case-insensitive). A duration check would cost an InnerTube call per new
  video; the `/shorts/` redirect probe is unofficial. An untagged Short costs
  one dismiss. The redirect probe is the recorded upgrade path.
- **No watch status, no tags, no folders — explicitly rejected.** The
  operator used all three in the Play app and found them a chore, not a
  benefit ("hat eh nur genervt", 2026-08-23). Navigation needs are served by
  channels and the LLM topics, which require no upkeep. The trigger to
  re-open is the operator asking, not a feature-parity argument.
- **Always-on, no feature flag.** Podcasts are gated because cast2md is an
  external service; channels need nothing external. An empty inbox explains
  itself and links to Settings.
- **No MCP tools in v1.** All logic sits in `internal/service/channels.go`,
  so an MCP tool later is wiring, not divergence; triage is an interactive
  surface, and an agent that wants a summary already has submission by URL.
- **Poll interval fixed at 30 minutes** with ~2 s between channels. The RSS
  endpoint is a public CDN; unlike the cast2md interval there is no
  deployment-specific load to make configurable.

**Trigger to re-open:** Shorts in the inbox proving annoying (redirect-probe
upgrade); `inbox_items` growth becoming visible (retention job); a want for
an unread badge or MCP tools.

**Revisions.**

- 2026-08-23: **Google Takeout import added** (`POST /api/v1/channels/import`,
  Settings → Channels). The user's YouTube account cannot be read directly:
  Google closed the Watch Later API to third parties in 2016 (the Play app
  documents the same limitation), and reading the subscription list live would
  require an OAuth app in a Google Cloud project with verification — out of
  proportion for one import. Takeout's `subscriptions.csv` carries channel ID
  and title, so the import performs no page fetches; the inboxes fill through
  one background poll pass rather than inside the request, which would hold
  the response for two seconds per channel.
- 2026-08-23: channel handle resolution reads the page's `rel="canonical"`
  link before the body's first `"channelId"` — the data island on a handle
  page can carry a localized sibling channel (`@veritasium` resolved to
  "Veritasium en Français" until this).

---

## 2026-08-23 — the transcript player: timed segments in a JSONB column, a lazy endpoint, and YouTube's IFrame API

**Decided:** 2026-08-23

**Decision.** The Transcript tab on `/video/:id` becomes a player for YouTube
rows: the video embeds via YouTube's IFrame API next to a timed transcript
whose current line highlights and follows playback, an in-pane search jumps
between hits, and clicking a line seeks the video. Timed segments live in a
new nullable `videos.transcript_segments JSONB` column (migration `000014`),
written at ingest by the InnerTube path and fetched on demand by
`Service.GetTranscriptSegments` for rows that predate the column. Podcast rows
and Voxtral-transcribed rows keep the plain transcript.

**Reasoning, per part:**

- **A column, not part of `metadata` and not a table.** `metadata` travels
  with every list query (`videoColumnsNoTranscript` includes it), and a
  three-hour video's segments are hundreds of KB. A separate table buys
  nothing: segments are never queried individually and share the transcript's
  lifecycle. The column is read by exactly one query (`GetVideoSegments`) and
  is absent from `videoColumns`, the same isolation the transcript gets from
  list queries.
- **Tri-state column.** `NULL` = never fetched; `'[]'` = fetched, InnerTube
  has no captions (a negative cache, so a Voxtral-transcribed row does not
  re-hit YouTube on every player open); an array = usable. The stored shape is
  the wire shape, `[{"s":12.4,"d":3.1,"t":"…"}]`, so the service passes the
  raw payload through without re-marshalling.
- **Fetch-on-open instead of a bulk backfill.** Opening the player on a
  pre-000014 row makes one InnerTube call ever and stores the result. A bulk
  backfill would burst hundreds of calls for videos that may never be opened
  again, competing with the ingest queue. The interactive path has its own
  10 s spacing (`waitSegmentFetchSlot`) rather than sitting behind the batch
  queue, whose spacing grows to 45 s under load.
- **A separate `GET /api/v1/videos/{id}/segments` endpoint.** The detail page
  polls `GET /videos/{id}` every 2 s while a row is in flight; attaching the
  segment array there would resend it on every poll. The GET materializes the
  cache on a miss — no POST/GET pair, because the fetch is idempotent.
- **MCP `get_video` does not carry segments.** The logic sits in the service
  layer, so both transports *can* reach it — the omission is payload policy,
  not behavioural divergence: an LLM client needs the transcript text, which
  it already gets, and thousands of timing tuples are context spent on an
  action (seeking a player) an MCP client cannot take.
- **The IFrame API script is a reintroduced external runtime dependency.**
  The 2026-08 Modernist migration removed the last one (the font CDN),
  because a tailnet client without a route to the CDN lost the typeface. This
  one is different in kind: a client that cannot reach youtube.com cannot
  play the video regardless — the `i.ytimg.com` thumbnails already assume
  that route — the script loads only when the player mounts, and a load
  failure degrades to the plain transcript with external timestamp links.
- **Inline YouTube Premium sign-in (as the Play app offers) is not
  buildable here and mostly unnecessary.** Play needs it because its native
  webview has no YouTube session. A browser embed uses the browser's own
  youtube.com cookies: a Premium login in the same browser carries into the
  embed where third-party cookies are allowed (Chrome), and where they are
  blocked (Safari ITP) no page-side mechanism exists to restore it.

**Trigger to re-open:** per-open fetching proving annoying in practice (that
is the bulk-backfill row in `ROADMAP.md`); an MCP client with a real use for
timed segments; the IFrame API script failing in a way the degradation does
not cover.

---

## 2026-08-07 — the two library screens are a media feed, not a table

**Decided:** 2026-08-07

**Decision.** `/` and `/podcasts` render one centred `--reading-w` (820px)
column with artwork on every row and the summary's opening lines as the row's
body. `web/src/components/FeedList.tsx` is that component and serves both
screens; it replaces `RowTable` and `RowList` in `VideoListPage.tsx` and the
equivalent inline pair in `PodcastListPage.tsx`. The desktop/mobile fork is gone
for these two pages — a feed row is already a stacked layout, so it needs no
second version — and `useIsDesktop()` survives there only for the filter bar's
flex basis. `VideoDetailPage`, `StatsPage`, everything under `pages/settings/`,
the API, the Go packages and the Atom feeds are untouched.

**Reasoning.** The table was built for a queue: eight columns, one line per row,
a status in every one of them. But these are things to watch and listen to, and
the two fields that tell you whether you want one — the thumbnail and the first
sentence of the summary — are exactly the two a table row could not carry.
Meanwhile the status column repeated `done` several hundred times.

**A `completed` row therefore shows no status mark.** `running`, `queued`,
`failed` and `no captions` do, right-aligned, with `statusClass` and
`statusLabel` from `display.ts` unchanged. When `error_message` is set it
replaces the excerpt in `--color-accent-700`.

**The lead item is promoted only on page 1 of an unfiltered, unsearched list.**
On page 2 the newest row is not new, and in a filtered or searched list the
first hit is not more important than the second — promoting it would be a claim
about ranking that the query does not support. Topics appear on the lead item
and nowhere else: on every row they become a fourth column of noise.

**`excerpt(summary, title)` is in `display.ts`, not in the component.** Stored
summaries open with `## Summary of <title>` or `Chapter-by-Chapter Summary of
…`, which is worth nothing to a reader who read the title two lines above. It
strips fenced blocks, leading headings and a leading line that restates the
title, then returns the first paragraph whole — the CSS clamp truncates, so the
line count follows the column width rather than a guessed character count.

**Thumbnails are not desaturated.** The system reserves `.grayscale` for
editorial photography; a thumbnail here is an identifier, and desaturating it
removes what makes it recognizable. This is the one deliberate exception to the
imagery rule.

**YouTube thumbnails are derived from the video ID, not fetched.**
`youtube.ThumbnailURL` returns `https://i.ytimg.com/vi/<id>/hqdefault.jpg`, and
`ProcessVideo` writes it when the row is created. `hqdefault` is the one variant
that always exists — `maxresdefault` and `sddefault` 404 on plenty of videos, and
a broken image is worse than a soft one. It is 480x360 with letterbox bars on a
16:9 upload, and those bars are exactly the 45px that `object-fit: cover` crops
off a 16:9 frame, so what shows is the 480x270 picture. Migration `000013`
backfills existing rows with the same expression.

This is the server change the handoff anticipated, and the first check for it was
wrong. The handoff's precondition was that the list endpoint return `summary` and
`thumbnail_url`; `videoColumnsNoTranscript` (`internal/storage/videos.go:100`)
trims the transcript and nothing else, so the field was confirmed present and the
work went ahead. But the column was only ever written by the podcast path
(`internal/service/podcast.go:113`) — the YouTube path never set it, so every
YouTube row was NULL and the feed rendered the neutral block for all of them.
**Verifying that a field is transported is not verifying that it is populated.**

The image is loaded by the browser from `i.ytimg.com`, so the feed is the second
place `web/` points outside the origin. That is not a new property: podcast
artwork already loads from the URL in the feed, which is the podcast host's CDN.
The rule the Modernist migration established was that no *asset the UI needs to
render correctly* comes from outside — the font. A missing thumbnail degrades to
the neutral block that a row without artwork shows anyway.

**Two places where the handoff and the reference disagreed with the code:**

1. *The first day band carries no 2px rule.* The reference closes the filter bar
   with a 2px line and opens the band without one; `.filters` in the stylesheet
   ends in a 1px hair rule. Stacking both would show a 3px edge, and changing
   `.filters` would move every other screen, so `.feed-band:first-child` drops
   its own rule instead.
2. *The search field refuses to shrink below 768px.* The chip row does not wrap
   there — it scrolls sideways, per the handoff — and with `flex: 1 1 100%` four
   chips squeezed the field to 46px. It is `flex: 0 0 100%`, so the field takes
   the full width and the chips run off the right edge.

**Trigger to re-open.** A third screen needs the same feed, at which point the
`variant` prop is doing work a component boundary should do instead. Or search
starts returning `thumbnail_url`, which would remove the one case where a row
shows the neutral block for a record that does have artwork.

---

## 2026-08-07 — the frontend gets a test runner

**Decided:** 2026-08-07

**Decision.** `web/` has vitest as a devDependency, `npm test` is `vitest run`,
and CI runs it after `npm run build` (`.forgejo/workflows/ci.yml`). It covers the
pure helpers in `web/src/display.ts` and nothing else. There is no component
renderer, no jsdom and no browser: adding one is a separate decision.

**Reasoning.** This closes the roadmap item that asked for a runner or a
decision against one. What settled it was `excerpt()` — the media feed handoff
specified it in prose with five ordered rules and asked for a test against one
real summary of each shape, and it is exactly the kind of function `tsc -b`
cannot judge. The first implementation passed the type check and failed two of
the five cases: the fenced-block regex used `$` under the `m` flag and matched
at the end of the first line, so a summary opening with a code fence returned
the fence's contents as its excerpt. Nothing else in the chain would have caught
that before it reached a row.

The scope stays narrow on purpose. A component renderer buys little here — the
two layout defects the Modernist migration introduced were found by looking at
screenshots, and they still would be.

**Trigger to re-open.** A UI regression that a pure-function test could not have
caught, which is the argument for a renderer and a DOM.

---

## 2026-08-07 — the web UI runs on the shared Modernist design language

**Decided:** 2026-08-07

**Decision.** `web/src/homelab.css` is vimmary's copy of the Modernist
stylesheet. The design language is shared across the homelab apps, but each app
holds its own copy and edits it; no app is the authority over the others. What
says where a rule belongs is the handoff package, which is issued from Claude
Design. A rule that is only ever vimmary's goes in `web/src/index.css` below the
import. `index.css` is now the Tailwind import, the
Archivo import, the `homelab.css` import, an `@theme inline` bridge and about
thirty lines of vimmary-only rules. The `--vim-*` token layer and the `.vim-*`
class layer are gone, along with Fraunces, Geist, JetBrains Mono, every radius,
the sticky blurred header, the coloured status pills, the drop cap and the
centred 1040px column.

Archivo is self-hosted through `@fontsource-variable/archivo`. **No font CDN.**
The server is reachable over Tailscale only, so a client without a route to
`fonts.googleapis.com` rendered the entire UI in `system-ui` — the three
`<link>` tags this removes were vimmary's only external runtime dependency.

There is one breakpoint, 768px, read in JS by `useIsDesktop()`
(`web/src/hooks/useMediaQuery.ts`) because desktop and phone are different
component trees rather than one tree reflowed. Above it a top `.nav`; below it a
fixed `.tabs` bar whose column count follows the item count.

**UI strings stay English.** The handoff specifies German labels throughout.
`CLAUDE.md` § Language makes English the repo's only language, FreeReps is
English, and vimmary had no German string in `web/` before this change. The
German in the handoff is treated as design placeholder text.

**Six places where the handoff describes something vimmary does not have:**

1. *Podcasts are absent from the handoff.* `PodcastListPage`, `PodcastNewPage`
   and `PodcastSection` follow the same four screens by analogy;
   `SourceBadge` became a `.tag .tag-neutral`, because the palette is mono and
   an icon is only used where language does not suffice.
2. *The nav identity slot reads `Settings`.* The handoff wants the Tailscale
   login at `margin-left: auto`. No endpoint returns it —
   `GET /api/v1/config/features` carries `podcasts`, `cast2md_url` and
   `is_admin` — and this change touches no Go.
3. *The prescribed nav* (`Videos · Search · Stats`, mobile `… · Queue · More`)
   names two routes vimmary does not have. It became `Videos · Podcasts* ·
   Stats` with search as a `.search` field in each list's filter bar, and
   `Podcasts` absent when cast2md is unconfigured, per the 2026-08-06 decision
   below.
4. *The filter chips are `All · Queue · Failed · No captions`,* not the
   handoff's `… · Deep`. Every chip has to be a `status` the list endpoint
   accepts; there is no detail-level filter, and filtering the current page
   client-side would report a count that is not the library's.
5. *The "last webhook" line in the Videos header is dropped.*
   `GET /api/v1/settings/webhook` returns the token, not a delivery timestamp.
6. *The Stats range control switches source, not time.* `fetchStats` takes a
   source and returns a fixed 30-day series; a `7d · 30d · 90d · 1y` control
   would have nothing behind it. `RangeControl` carries
   `Everything · Videos · Podcasts` instead, with the same `.seg`-to-`.sheet`
   collapse below 768px.

**Row actions moved off the list.** Retry, Transcribe and Delete are on the
detail page; the list keeps the bulk actions in its `.footer`. A row is one
target, not five.

**The app icon's mark must stay lighter than its field.** `web/scripts/build-icons.mjs`
renders `vm` in `#f3f2f2` on `#ec3013` and writes the five PNGs plus
`public/manifest.webmanifest`. iOS 18 derives the dark and tinted home-screen
variants from the single icon; with a dark mark on a mid field both collapse
toward black and the icon reads as an empty rounded square. This is the defect
FreeReps fixed in its `c7319b0`, and it is invisible on desktop. The script is
committed and run by hand (`npm run icons`) rather than wired into `npm run
build`: it needs Chrome and ImageMagick, which CI does not have, and the output
changes about once a year.

**Reasoning.** vimmary's UI was a one-off editorial design that matched nothing
else in the homelab, and the font CDN made it fail in a way peculiar to a
tailnet-only service. Adopting the shared language costs a rewrite of the six
page components once; keeping the divergence costs a second design to maintain
for as long as the service exists.

`web/src/homelab.css` is a copy rather than a dependency because there is no
package to depend on — the bundle is a handoff, and FreeReps carries its own
inlined copy for the same reason.

**Trigger to re-open.** The handoff bundle becomes a published package, which
would make the per-app copy unnecessary.

**Revisions.**

- *2026-08-07* — the stylesheet is editable per app. The original form of this
  decision was that `homelab.css` is copied verbatim, never edited here, and
  that FreeReps' `server/web/src/index.css` is the authority when the two
  disagree; a system change was to be made in FreeReps and re-copied. That rule
  was dropped the same day, on two grounds. It described a synchronisation
  nothing performed: FreeReps has no `homelab.css`, the two files had already
  diverged (650 lines against 638), and no step in either repo compares them.
  And it had no owner — the design is steered in Claude Design and issued as a
  handoff, so a repo has nothing to police. The immediate case was the media
  feed handoff, which assigns `--reading-w` and the `.feed*` block to the
  stylesheet because cast2md renders the same feed.

---

## 2026-08-07 — a testable storage dependency gets a narrow seam, not one interface over `storage.DB`

**Decided:** 2026-08-07 (`3fa3210`, `359551d`)

**Decision.** When a code path in `internal/service` needs to be testable
without Postgres, it gets its own interface over exactly the `storage.DB`
methods it calls, declared next to the field it replaces. There are three so
far, all in `internal/service/service.go`: `settingsSource` (two methods, the
summarizer path), `summarizerFactory` (the construction step) and
`searchSource` (two methods, `Search`). `Service.db` stays a concrete
`*storage.DB` for everything else.

There is deliberately no single interface over `storage.DB`. It would carry all
38 methods the service calls, every fake would implement all of them to exercise
one, and adding a query anywhere would break every fake.

**Reasoning.** `analysis/structure-report.md` section 4.1 recommended the single
interface, on the measurement that 15 of 17 functions at CC ≥ 10 sat at 0%
coverage and the two exceptions were the ones reached through `PodcastSource`.
The measurement holds; the shape does not follow from it. What the covered
functions have in common is a *substitutable dependency for their path*, not one
abstraction over the store — and `3fa3210` had already demonstrated the narrow
form on the summarizer path before the recommendation was acted on.

The result on the case that motivated it: `Search` went from 0.0% to 98.6%
behind a two-method interface, and `internal/service` from 13.4% to 26.2%.

**Trigger to re-open.** Enough seams accumulate that they overlap — three or
four interfaces naming the same methods — or a fake has to implement methods its
test does not use. Either is the signal that the boundary is in the wrong place.

---

## 2026-08-07 — `storage.ErrNotFound` replaces `pgx.ErrNoRows` at the boundary

**Decided:** 2026-08-07 (`b6ebaa6`)

**Decision.** `internal/storage` returns its own `ErrNotFound` for a lookup that
matches no row, and for an update or delete that affects none. Callers compare
with `errors.Is`, never with `==`. The translation sits in `scanVideo` and
`scanSubscription`, which every single-row lookup already passes through, and in
the three `RowsAffected() == 0` branches in `videos.go`. Only `internal/storage`
imports `jackc/pgx`.

Comparisons *inside* `internal/storage` against `pgx.ErrNoRows` stay — that
package owns the driver, which is where `CLAUDE.md`'s layer table puts it.

**Reasoning.** Two reasons, and the second is the one that decided it.

The layering reason is the one the structure report gives (section 2.2): three
transports and the service layer had to know which driver the storage layer
uses. The report weighed it by the blast radius of a driver swap, found 7 lines,
and recommended doing it opportunistically.

The reason it was worth doing on its own is that the existing comparisons were
`==`, so any wrapper between the query and the handler made them silently false
— and one already did. `ResummarizeAsync` wraps its lookup with
`fmt.Errorf("get video: %w", err)`, so `POST /api/v1/videos/{id}/resummarize`
on an unknown ID answered 500 where `handlers.go` intended 404. A sentinel that
callers compare with `errors.Is` cannot fail that way.

**Trigger to re-open.** None expected. If a caller ever needs to distinguish
"row absent" from "update matched nothing", those are one error today and would
have to become two.

---

## 2026-08-07 — every Settings section owns its queries and its errors

**Decided:** 2026-08-07 (`960ed8d`)

**Decision.** `web/src/pages/settings/` holds one file per concern — Karakeep,
LLM providers, summaries, RSS, podcasts — and each renders its own loading and
error state. `SettingsPage.tsx` is the page that stacks them and holds no query
and no state. A failing backend costs one card.

**Reasoning.** The page previously combined four queries into one
`isLoading`/`errorObj` pair. That is a hard conjunction: one failing query
replaced the whole Settings page with an error box, including the sections that
had loaded. `LLMSection` had already been written to opt out of it, because the
server answers 404 to a non-admin and the page would have gone with it — so the
exception existed before the rule was changed, and generalising it costs
nothing.

The split is also what the size measurement asked for: 1193 lines and 24 hooks
in one file, largest in the repository, against five concerns sharing no state
(`analysis/structure-report.md` 4.4).

**Trigger to re-open.** A section needs data another section already fetched.
React Query dedupes by key across components, so this is only a real problem if
two sections need to stay in step within one render.

---

## 2026-08-07 — no secret is fetched over the network during startup

**Decided:** 2026-08-07

**Decision.** vimmary makes no setec request. The database password is resolved
from `VIMMARY_POSTGRES_PASSWORD` in the environment, and the LLM API keys and
the summary provider live in `app_settings` and are read at the moment they are
used. setec keeps the database password for the *deploy*: Ansible reads
`docker/vimmary/db-password` and renders the stack's `.env`.

The setec package remains linked, because meltkit's `secrets` resolver imports
it and vimmary still uses that resolver for its environment-then-literal chain.
`InitSetecStore` is never called, so no setec code executes. Cutting the last
link would mean reimplementing the resolver here and diverging from every other
service in the homelab — not worth it for a code path that cannot run.

**Reasoning.** Every startup network call is a way for the process to hang
before it opens its listener, and one of them did — 6h23min on 2026-08-07, with
the container reporting up. Bounding the call turns that outage into a restart
loop, which is better and still leaves the class alive. Removing the call
removes the class. What is left in the startup path is tsnet, which cannot be
removed because it *is* the listener.

Deliberately not kept: a one-release bootstrap that copies the keys out of setec
into the database on first start. It would have avoided a manual step at the
cost of keeping the exact code path that caused the outage alive for one more
release. The keys were entered by hand instead.

All four `vimmary/*` secrets were deleted from setec on 2026-08-07, each once it
was provably redundant: `postgres-password` held the same value as
`docker/vimmary/db-password` (identical SHA-256), `karakeep-api-key` was read by
no code, `mistral-api-key` had been confirmed serving from `app_settings`, and
`claude-api-key` was revoked at Anthropic first. `docker/vimmary/db-password` is
the only vimmary secret left in setec, and Ansible is its only reader.

**Trigger to re-open.** A secret that has to be current at startup and cannot be
supplied by the deployment — none exists today.

---

## 2026-08-07 — API keys are stored in plain text, knowingly

**Decided:** 2026-08-07

**Decision.** The LLM API keys in `app_settings`, like `users.karakeep_api_key`
before them, are stored unencrypted. No application-level encryption is added.

**Reasoning.** This is a reduction, and naming it is the point of this entry.
The Mistral key previously lived in setec, encrypted at rest under the KMS key
in setec's unit on tsidp. It now sits in a Postgres column in the clear.

Measured on 2026-08-07 on the Proxmox node `walter`: the only backup job,
`backup-lebowski-daily`, runs with `all=1` to the `lebowski-pbs` store, Mon–Fri
at 01:00. `vimmary-lxc` and its `pgdata` volume are included, so the keys leave
the host daily.

Accepted anyway, because application-level encryption would put the decryption
key on the same host as the data — and host access already yields the database
password from `/opt/docker/stacks/vimmary/.env`. It would protect only the
off-host copy, and the effective place to protect that is the backup store,
where it covers every service rather than one column in one application.

A code comment on `SetKarakeepAPIKey` claimed the key was encrypted. It was
never true and has been corrected; that claim is the reason this entry exists
rather than a note.

**Trigger to re-open.** An unencrypted PBS datastore combined with a key whose
misuse costs money, or a key belonging to someone other than the operator.

---

## 2026-08-07 — a failed deploy alerts, and nothing acts on it overnight

**Decided:** 2026-08-07

**Decision.** No unattended remediation is added for a service that is down and
stays down. The alerting path is left as it is.

**Reasoning.** The 2026-08-07 outage looked like a missing alert and was not.
Uptime Kuma published `vimmary Down` at 22:27:29 at priority 5 on the `kuma`
topic, which is `isDefault` and reaches the phone; the parent group followed at
22:29:05, and CI's own deploy-failure message at 22:29:30. Three notifications
within three minutes, and the outage ran another six hours. Nothing about the
routing was wrong — the operator was asleep.

Since the failure class that produced it no longer exists, and any remaining
class is covered by the container restarting itself, building an agent that
restarts services at night would add a moving part to guard against something
that has not recurred.

**Trigger to re-open.** A second overnight outage in a class the restart policy
does not cover.

---

## 2026-08-06 — an unconfigured integration is invisible, not disabled

**Decided:** 2026-08-06

**Decision.** Both services stay fully operable alone. When the other side is
not configured, its half of the UI is absent — not greyed out, not showing an
error, not an empty page explaining a missing feature.

In vimmary that means: no Podcasts nav entry, no podcast routes registered, no
Podcasts section in Settings, no Videos/Podcasts prompt switch, no stats scope
switch, no source badge on cards and detail pages, and one RSS URL instead of
three. The strapline reads `youtube · read` again. `GET
/api/v1/config/features` is the single place the frontend asks, and it defaults
to off while loading, so a deployment without cast2md never flashes the podcast
UI. In cast2md it means the **Summarize in vimmary** button is not rendered
when `vimmary_url` is empty, which is how it was built.

The HTTP surface is not gated. `/api/v1/podcasts/*` answers 503 with a typed
error, and the podcast and combined Atom feeds stay mounted.

**Reasoning.** Someone who downloads either project on its own should get a
coherent application, not one carrying visible stubs for a service they have
never heard of. A disabled control still has to be explained; an absent one
does not.

The HTTP surface stays open because the two are different audiences. The feed
routes read the database, not cast2md, so they keep working for a reader that
subscribed while the integration was on — breaking a subscription URL because a
config key changed would be worse than serving an empty feed. And a typed 503
tells an API client what is wrong, which a 404 does not.

The stats scope is forced to `youtube` when the integration is off, rather than
left at "everything". Podcast rows can survive being switched off, and counting
them in the totals while the video list hides them would make the two pages
disagree.

**Trigger to re-open.** The integration config moves into the GUI and the
database, at which point "configured" becomes a runtime state a user can change
without a restart, and the frontend needs to react to it rather than read it
once.

---

## 2026-08-06 — podcast summaries live in vimmary, with cast2md as a transcript source

**Decided:** 2026-08-06

**Decision.** vimmary summarizes podcast episodes as well as YouTube videos.
cast2md keeps downloading and transcribing; it gained one additive query
extension (`since`, `feed_id` and `order` on `GET /api/episodes/status/{status}`)
and one optional link back. vimmary polls cast2md over the tailnet with a
persisted watermark. No summarization logic was added to cast2md.

The two kinds share one table, `videos`, discriminated by a `source` column, and
one processing path from the transcript onwards. They are separated at every
point a reader meets them: `GET /api/v1/videos` and MCP `list_recent` default to
`source=youtube`, the GUI has its own Podcasts page, every card and detail page
carries a type marker, and there are three RSS feeds instead of one.

**Reasoning.** Everything downstream of the transcript — summarizer interface,
per-user prompts, model registry, pgvector embeddings, hybrid search, Atom feed,
web UI — was already source-independent and only wired to YouTube. Rebuilding
that in cast2md would have been a second copy of six subsystems; the alternative
of a shared library for them is the same coupling with more moving parts.

Sharing the table rather than adding a `podcasts` table follows from where the
work is: search, stats, feed and UI all operate over one list, and two tables
would mean a UNION in every one of them. The cost is that source-blind queries
become bugs — `ListFailedVideos`, `ListVideosWithoutMetadata` and
`ListNoCaptionsVideos` all needed `AND source = 'youtube'`, because their callers
hand `YouTubeID` to the YouTube pipeline.

Polling rather than a webhook from cast2md keeps the direction of dependency
single: cast2md does not know vimmary exists, except for one optional link in a
template. The watermark is stored as text and always comes from cast2md's own
`updated_at`, because those timestamps are naive local time — a round trip
through Go's timezone handling is the one place episodes could be skipped
silently.

Subscribing summarizes the feed's three newest transcribed episodes and follows
along from there, with the count settable per feed and 0 restoring plain "from
now on". That first poll runs inside the PUT rather than waiting for the ticker,
because a checkbox that appears to do nothing for up to fifteen minutes reads as
broken. It is best effort — the subscription is committed first, and a failure
leaves the feed uninitialized for the poller to retry. The watermark lands on
the newest of that first batch, so the batch is never fetched twice. The
alternative — summarizing a feed's whole history on subscribe — turns one
checkbox into hundreds of LLM calls, and is available as an explicit action
instead.

**Trigger to re-open.** cast2md grows its own summarization, or the split of
concerns stops matching how the two services are actually operated.

**Revisions.** 2026-08-06, same day: subscribing was originally "from now on"
with nothing summarized on the first poll. A feed that produces no summary until
the show publishes again gives no sign that it works, so the first poll now
takes the three newest episodes by default (`podcast_subscriptions.initial_backfill`,
migration `000011`).

2026-08-06, same day: vimmary gained a "Transcribe all" action per feed, which
calls cast2md's `POST /api/queue/batch/feed/{id}/process`. This is the first
place vimmary makes cast2md do work rather than reading from it, and the only
non-GET in `internal/cast2md`. It does not change the direction of dependency —
cast2md still knows nothing about vimmary — but it does make vimmary a control
surface for cast2md's most expensive operation, which is why the button states
its episode count and asks first. The transcribed episodes arrive through the
ordinary poll, because finishing a transcription updates the episode's
`updated_at`.

**Revisions.** This revises the 2026-03 decision below ("a separate service, not
a cast2md extension"), whose reasoning was that podcasts belong to cast2md and
videos to vimmary. That boundary held for *transcription* and still does; it did
not hold for *summarization*, which is the trigger that entry named. The separate
services remain separate.

---

## 2026-05-17 — the GitHub release job stays inline until a third public project exists

**Decided:** 2026-05-17

**Decision.** The `release` job in `.forgejo/workflows/ci.yml` — ghcr.io push,
tag-propagation wait, GitHub release creation — is not extracted into the shared
`ci-workflows` repo yet. It stays duplicated between this repo and cast2md.

**Reasoning.** Rule of Three. Two near-identical copies is the point at which
extraction is tempting and premature: the shared shape is not yet visible,
because cast2md's variant additionally publishes to PyPI. Extracting now would
produce a workflow that has to carry a language-specific publish step for its
second caller, which is the abstraction being wrong rather than early.

**Trigger to re-open.** A third world-public project ships. At that point the
job moves to `ci-workflows/.forgejo/workflows/release-to-github.yml` using this
one as the template, and the language-specific publish stays with the caller.

---

## 2026-05-10 — Forgejo is canonical and GitHub is a push mirror

**Decided:** 2026-05-10

**Decision.** Source-of-truth development happens on the self-hosted Forgejo.
`github.com/meltforce/vimmary` is a push mirror. The CI release job waits for a
tag to appear on GitHub before creating the release there.

**Reasoning.** The mirror is asynchronous. A release created immediately after
the tag push references a tag GitHub does not have yet, and the API call fails.
The wait loop polls for up to five minutes and then fails loudly rather than
creating a release against a missing ref.

**Trigger to re-open.** The mirror direction reverses, or the release is created
from the Forgejo side instead.

---

## 2026-03-18 — the RSS feed is authenticated by an unguessable token in the URL

**Decided:** 2026-03-18 (implemented in `internal/feed/`)

**Decision.** The Atom feed at `/feed/atom/<feed_token>` is mounted outside the
Tailscale auth middleware. A 32-byte `crypto/rand` token in the URL path is the
only access control. The token is per-user, immutable once generated, lazily
created on first Settings-page access, and carries a unique constraint in the
database. It is a separate column from `webhook_token`.

**Reasoning.** An RSS reader cannot authenticate over Tailscale, so classical
auth would make the feature unusable — which is the whole point of the feature.
A path segment rather than a query parameter avoids URL truncation in readers.
An invalid token returns 404 rather than 403, so the response does not confirm
whether a token exists.

The token is separate from `webhook_token` for three reasons: the feed URL is
pasted into third-party apps and therefore sits in a different trust context;
the webhook token grants write access (it triggers processing) while the feed
token is read-only; and separate tokens can be revoked independently.

Immutability is deliberate — a rotating token breaks every subscribed reader
silently.

**Explicitly out of scope.** Transcripts in the feed (summary and metadata are
enough), feed discovery via `<link rel="alternate">`, conditional GET with ETag
or If-Modified-Since, WebSub push, per-channel or per-topic feeds, and token
rotation. All five are overhead at a volume of roughly ten videos per day.

**Trigger to re-open.** A token leaks, at which point rotation has to exist and
the immutability argument is outweighed. Or the feed becomes multi-tenant beyond
personal use.

---

## 2026-03-18 — Atom 1.0 rather than RSS 2.0

**Decided:** 2026-03-18

**Decision.** The feed is Atom 1.0.

**Reasoning.** The specification is tighter, the XML is cleaner, and UTF-8
handling is better defined — which matters because summaries are multilingual.
Every current reader supports Atom, so the compatibility argument for RSS 2.0
does not apply.

**Trigger to re-open.** A reader the operator actually uses fails on Atom.

---

## 2026-03-10 — video processing runs through one queue with an adaptive delay

**Decided:** 2026-03-10 (`internal/service/service.go`)

**Decision.** All video processing — webhooks, retries, bulk imports — goes
through a single queue: a buffered channel with capacity 100 and one worker. The
spacing between YouTube InnerTube calls is not fixed; `adaptiveDelay()` scales it
from 10 s to 45 s with queue depth.

**Reasoning.** YouTube returns 429 under bursts, and a bulk import of an existing
Karakeep library is exactly a burst. A single worker makes the rate limit
enforceable in one place instead of at every call site. The delay scales with
depth because a fixed 10 s is enough for normal operation and not enough during
an import — a fixed 45 s would make normal operation needlessly slow.

**Consequence to know about.** A bulk import that appears stalled is usually the
adaptive delay working as intended.

**Trigger to re-open.** YouTube publishes an actual documented rate limit, or the
volume grows past what one worker can clear in a day.

---

## 2026-03-09 — Karakeep writeback is delayed and additive

**Decided:** 2026-03-09 (`internal/service/process.go`)

**Decision.** The summary is written back to the Karakeep bookmark after a 30-second delay,
as a plain-text note with the vimmary detail URL as a prefix. The
`video-summarized` tag is added with a POST rather than by replacing the tag
list.

**Reasoning.** Karakeep's own crawler runs on a newly created bookmark and
overwrites the note if vimmary writes first — the delay is what makes the note
survive. The tag is added rather than set because a full replacement discards
Karakeep's AI-generated tags, which the operator wants to keep.

Plain text rather than Markdown because Karakeep's note field renders neither
consistently.

**Trigger to re-open.** Karakeep gains a documented completion event, which
would replace the delay with something deterministic.

---

## 2026-03 — a separate service, not a cast2md extension and not a Karakeep plugin

**Decided:** 2026-03-09

**Decision.** vimmary is its own Go service. The functionality was not added to
cast2md and was not built as a Karakeep plugin.

**Reasoning.** cast2md's domain is podcasts; adding video summaries would dilute
it into "media", which is the kind of scope drift that makes a service hard to
reason about later. Karakeep is third-party software, so a plugin is fragile
across its updates and puts the operator on someone else's release schedule.

**Trigger to re-open.** Karakeep gains a stable, documented plugin interface, or
cast2md and vimmary end up sharing more code than they differ in.

**Revisions.** 2026-08-06: the claim that podcasts are out of vimmary's domain
no longer holds. Summarization moved to vimmary for both kinds; see the
2026-08-06 entry above. The two services stay separate — cast2md transcribes,
vimmary summarizes.

---

## 2026-03 — shared infrastructure lives in meltkit, not in a copy

**Decided:** 2026-03-09

**Decision.** Database, config, secrets, middleware, server and MCP helpers come
from `github.com/meltforce/meltkit`, versioned through Go modules and shared with
totalrecall. totalrecall is the architectural blueprint: service layer, init
order, three-tier secret resolution, Tailscale auth and pgvector + JSONB storage
all follow its conventions.

**Reasoning.** The alternative is the same six packages diverging in three repos.
Go modules make the dependency explicit and the version pinned, so a change in
meltkit does not reach here until it is bumped.

**Cost accepted.** A fix needed in meltkit is a release there plus a bump here,
rather than an edit.

**Trigger to re-open.** meltkit becomes a bottleneck — a change needed in two
consumers at once with incompatible requirements.

---

## 2026-03 — business logic lives in the service layer, called by both transports

**Decided:** 2026-03-09

**Decision.** `internal/service/` holds all business logic. MCP tool handlers and
REST handlers both call service methods and contain no logic of their own.

**Reasoning.** Two transports over one feature set is exactly the shape that
produces divergent behaviour — a fix applied to the REST path and not the MCP
path is invisible until someone uses the other one. With the logic in one place
the divergence is not expressible.

**Trigger to re-open.** A transport needs behaviour the other genuinely must not
have.

---

## 2026-03 — the summary provider is configurable, embeddings are not

**Decided:** 2026-03-09

**Decision.** Summaries come from Claude or Mistral, selectable per config and
per user, with the concrete model loaded dynamically from the provider API.
Embeddings are always Mistral `mistral-embed` at 1024 dimensions.

**Reasoning.** Summary quality is a matter of taste and cost, and both change —
making it a setting costs one interface. Embeddings are not comparable that way:
the dimension is baked into the `vector(1024)` column and the HNSW index, so a
second provider means a migration and a full re-embed, not a config value.

**Trigger to re-open.** A re-embedding path exists, or an embedding provider
appears that is materially better at the same dimension.
