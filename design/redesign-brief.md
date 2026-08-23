# vimmary redesign brief — for Claude Design

Working material for the redesign session. This file describes the app, the
goal, the chosen direction and the constraints; the repo around it is the
authoritative source for current screens and vocabulary. When the handoff
package lands, this brief is superseded by it.

Reference canvas with the direction sketches (three screens in the chosen
direction): https://claude.ai/code/artifact/2539c619-de59-4a35-91d2-460f95dd042b

## What vimmary is

A self-hosted service that turns bookmarked YouTube videos and transcribed
podcast episodes into searchable LLM summaries. It is a **reading and triage
tool for media**, not a media server: the artifacts users spend time with are
summaries, chapters and transcripts; the videos and episodes are the way in.

The user-facing surfaces, in order of time spent:

1. **Library** (`/` videos, `/podcasts` episodes) — a reverse-chronological
   feed of summarized items with hybrid search, status chips, topic chips and
   a channel filter (avatar rail on wide viewports, horizontal strip below).
2. **Detail / reader** (`/video/:id`, `/podcast/:id`) — the summary as
   reading text, plus tabs for chapters, topics, and the transcript. For
   YouTube rows the transcript tab is a **synchronized player**: embedded
   video and search field pinned on top, timed transcript underneath, the
   current line highlighted and clickable for seeking.
3. **Inbox** (`/inbox`) — new videos from followed channels, waiting for
   triage: Watch (summarize + open detail), Summarize, Dismiss. Shorts are
   filtered automatically.
4. **Channels** (Settings → Channels today) — follow YouTube channels by
   handle/URL or Google-Takeout import; podcast feeds are subscribed under
   Settings → Podcasts.
5. **Stats** — counts, runtime saved, activity, top channels/topics.
6. **Settings** — Karakeep, channels, feeds, prompts, LLM keys; one section
   per concern.

Content shapes that the design must carry:

- YouTube thumbnails are 16:9 (`hqdefault` with letterbox bars — the app
  crops them with `object-fit: cover`). Channel avatars exist for every
  channel (subscription avatar or cached; video-thumbnail fallback while the
  cache warms). Podcast covers are square.
- Summaries are markdown, typically 3–10 paragraphs; deep summaries carry
  `##` chapter headers, blockquotes and a references list.
- Processing states exist and must stay visible but quiet: `pending`,
  `processing`, `failed`, `no_captions`. A completed row shows no status.
- Both media types appear in mixed contexts (search results, stats), so the
  video/podcast distinction needs a consistent, lightweight marker — in the
  chosen direction: round avatar = video channel, rounded-square cover =
  podcast show.

## Goal of the redesign

vimmary currently runs on the shared homelab "Modernist" language (Archivo,
hard edges, 820 px reading column, red accent) — a document-first system that
serves the text apps well but fights a media app: artwork is cropped into a
text layout, video and podcasts feel bolted side by side, and the library
reads as a document list rather than a media shelf.

The decision: **vimmary leaves the shared language and gets its own,
media-first system.** The other homelab apps keep Modernist; nothing
synchronizes stylesheets between apps, so the divergence has no mechanical
cost (each app owns its copy).

Success criteria:

- Video and podcast content feel like **one library with two media types**,
  not two bolted-on lists — this is the primary reason direction D won.
- Artwork (thumbnails, avatars, covers) carries the browsing surfaces.
- The reading experience keeps first-class typography — vimmary is for
  reading; the redesign must not trade the reader away for media chrome.
- Triage (inbox) and playback (synchronized transcript player) are visible,
  reachable actions, not buried tabs.

## Chosen direction: "D — Shelf"

Warm, friendly, podcast-app-genre. The three sketch artboards on the canvas
(D home, D — Channels & Shows, D — Reader) are the visual reference; their
working values:

- **Ground** `#f2ece4` (warm paper); **ink** `#241f1a`; secondary text
  `#6b6157`; pill-nav ground `#e5dccf`.
- **Surfaces**: white cards, radius 22 px, shadow `0 2px 10px
  rgba(36,31,26,0.07)`; media inside cards radius 14 px; pills fully round.
  Dark cards (`#241f1a`) as accent surfaces for inbox/player modules with
  amber `#d9a066` highlights.
- **Accents**: rust `#b3532c` for links/labels, amber `#d9a066` for
  new-badges and primary actions on dark.
- **Type**: Bricolage Grotesque for display/headlines, Public Sans for UI.
  Proposed (open question): Source Serif 4 for summary body text.
- **Navigation**: segmented pill nav (Videos / Podcasts / Inbox / Channels),
  date-headed dashboard as the home ("Sunday, August 23 — 3 new summaries · 5
  waiting in the inbox").
- **Channels & shows**: card shelves — video channels with round avatars,
  podcast shows with square covers, amber new-badges, one-shot Karakeep
  channels collapsed into a "+63" card; a dark inbox teaser bar below.
- **Reader**: white reading card (kicker with avatar, display title, pill
  tabs, body text with soft amber highlights) + right rail with dark player
  card ("Watch with live transcript"), chapters with timestamps, topic pills.

Genre references, and what to take from each (do not clone any of them):

- **Snipd** — the closest product cousin (AI summaries, chapters, highlights
  over podcasts): chapter/highlight presentation, summary-first cards.
- **Pocket Casts** — warm audio-app language, artwork-forward rows, calm
  density.
- **Readwise Reader** — the reading pane: serious typography beside a
  metadata rail.
- Explicitly avoid the YouTube look itself; direction A explored it and was
  rejected as "yet another YouTube frontend".

## Constraints from the codebase

- **Stack**: React 19 SPA, Tailwind 4 + two plain stylesheets —
  `web/src/homelab.css` (the system, will be replaced by the new one) and
  `web/src/index.css` (app-specific rules). The handoff package decides
  where each rule lands, as before.
- **Breakpoint discipline**: exactly one breakpoint, 768 px, read in JS
  (`useIsDesktop()`); desktop and mobile are separate component trees, not
  one reflowed layout. The mobile shell today is a bottom tab bar. Design
  both trees. (The channel rail additionally appears only ≥1280 px today.)
- **Fonts must be self-hostable** (@fontsource packages). Clients reach the
  app over Tailscale and may have no route to a font CDN — the current
  Archivo is self-hosted for exactly this reason. Bricolage Grotesque,
  Public Sans and Source Serif 4 are all on Fontsource.
- **UI strings are English**, repo-wide rule.
- **Screens to cover** (all exist, see `web/src/pages/`): VideoListPage,
  PodcastListPage, InboxPage, VideoDetailPage (tabs: summary, chapters,
  transcript player, topics), StatsPage, SettingsPage (+ sections),
  PodcastNewPage. Shared components: FeedList, FacetFilters (channel
  rail/strip + topic chips + "Others" fold), TranscriptPlayer (sticky
  video+search, cue rows, active-cue highlight, in-transcript search marks),
  PageHeader, ConfirmDialog, Toast, status chips, stat strip.
- **Player specifics that must survive**: video and search field pin
  together while the transcript scrolls; cue rows are timestamp + text,
  clickable; the active cue and search hits need distinct highlights.
- **App icon**: currently `vm` in `#f3f2f2` on `#ec3013`
  (`web/public/app-icon/`); the mark must stay lighter than its field (iOS
  dark/tinted variants collapse otherwise). Redesign may keep or replace it —
  if replaced, respect that constraint.

## Open questions for the design phase

1. Serif (Source Serif 4) vs. Public Sans for summary body text.
2. Dark mode: the current system is light-only; is the warm-paper language
   worth a dark twin, or does the dark-card accent surface suffice?
3. Mobile shell: keep the bottom tab bar, or adopt the pill nav?
4. Does Stats join the card language or stay a plain figures page?
5. Keep the `#ec3013` app icon as a deliberate brand remnant, or re-derive
   it from the new palette?

## Deliverable

A handoff package in the established form: the new system stylesheet (to
replace vimmary's `homelab.css` copy), app-specific rules with their
assignments, font packages to install, and per-screen specs for the surfaces
listed above — enough that the implementation session can execute without
re-deriving design decisions.
