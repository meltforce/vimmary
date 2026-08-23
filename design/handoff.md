# vimmary redesign — handoff

Direction D, "Shelf", from Claude Design, 2026-08-23. This is the package the
implementation session worked from; it does not re-argue the decisions. It
supersedes `design/redesign-brief.md`, which was removed when the package
landed. Visual reference: `Vimmary Shelf.dc.html`, thirteen artboards, desktop
and mobile.

**Implemented on 2026-08-23.** Where the implementation had to deviate, and
why, is in `DECISIONS.md` under that date. Everything below is the package as
it arrived.

## Files in this package

| File | Went to | Note |
| --- | --- | --- |
| `vimmary.css` | `web/src/vimmary.css`, replacing `homelab.css` | The system. Class names unchanged from homelab.css, so no component got renamed. |
| `index.css` | the rule section of `web/src/index.css` | App-specific surfaces only. The Tailwind import and `@theme inline` block at the top of the file were kept. |

vimmary leaves the shared Modernist language with this change. The other homelab
apps keep their copy of `homelab.css`; nothing synchronises between apps, so the
divergence costs nothing mechanically.

## Fonts

Self-hosted, for the same reason Archivo was: a Tailscale client has no route to
a font CDN.

```
npm i @fontsource-variable/bricolage-grotesque \
      @fontsource-variable/public-sans \
      @fontsource-variable/source-serif-4
npm rm @fontsource-variable/archivo
```

```css
@import "@fontsource-variable/bricolage-grotesque/wght.css";
@import "@fontsource-variable/public-sans/wght.css";
@import "@fontsource-variable/source-serif-4/wght.css";
```

- **Bricolage Grotesque** — display: page titles, card titles, figures.
- **Public Sans** — every UI string, label, chip, button.
- **Source Serif 4** — summary body text, feed excerpts, transcript cue text.
  Decided: the reader gets its own face, so reading text and chrome never blur.

## Palette

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--color-bg` | `#f2ece4` | `#17130f` | the paper the cards sit on |
| `--color-card` | `#ffffff` | `#221d18` | every card |
| `--color-surface` | `#e5dccf` | `#2c261f` | pill-nav ground, chips, wells |
| `--color-text` | `#241f1a` | `#f5efe7` | ink |
| `--color-text-2` | `#6b6157` | `#9d9186` | all secondary copy |
| `--color-ink-surface` | `#241f1a` | `#2c241d` | dark accent cards |
| `--color-accent` | `#b3532c` | `#d9814f` | links, labels, primary action |
| `--color-amber` | `#d9a066` | `#d9a066` | new-badges, highlights, action on ink |

Shape: cards `22px` (`20px` under 768), media inside cards `14px`, pills fully
round, card shadow `0 2px 10px rgba(36,31,26,0.07)`.

Two contrast rules that were violated during the design and cost a round each:

- `--color-text-2` is the floor for text. Anything lighter (`--color-neutral-500`
  and up the scale) is decoration only — mono labels on placeholder artwork.
- Never dim a row with `opacity` to say "in progress". The outlined status chip
  carries the state; opacity takes the row's metadata below AA.

Dark mode is a full twin, not only the dark accent cards. It is driven by the
existing `[data-theme]` attribute from `theme.tsx`, including `auto`.

## Decisions this package encodes

- **Summary body in Source Serif 4.** Open question 1, settled.
- **Full dark theme.** Open question 2, settled: `[data-theme="dark"]` plus the
  `prefers-color-scheme` block for `auto`.
- **Pill nav on both trees.** Open question 3, settled: the mobile bottom tab bar
  goes away. `Layout.tsx` renders one `.pillnav` for both branches of
  `useIsDesktop()`; on the phone it scrolls horizontally. The `h-[76px]` spacer
  and the `.tabs` block go with it.
- **Stats joins the card language.** Open question 4, settled: figures are cards,
  top channels carry artwork, `.table` loses its own frame and lives in a card.
- **App icon re-derived, variant B.** Open question 5, settled: in
  `web/scripts/build-icons.mjs` set `FIELD = "#241f1a"`, `MARK = "#d9a066"`, and
  the face to Bricolage Grotesque 700. Geometry unchanged — the bar keeps the
  mark's own measure, so if the face's width changes, the font size follows the
  source ratio (`280/512` of the canvas) rather than the bar being trimmed. The
  mark must stay lighter than the field; that constraint is why amber on ink and
  not ink on amber.
- **The media-type marker is shape, not colour.** Round avatar = video channel,
  rounded-square cover = podcast show, in every mixed context (search, stats,
  rails, kickers).
- **Rules are gone.** Modernist's 2px/1px pair is replaced by card edges. The
  `--rule-strong` / `--rule-hair` variables still resolve, because components
  pass them inline (`borderTop: "var(--rule-strong)"`); both are now a hairline,
  so those call sites can be cleaned up screen by screen instead of in one pass.

## Per-screen specs

**Library — `VideoListPage`, `PodcastListPage`** (artboards 1, 6, 13)
Date-headed dashboard: `longDate(new Date())` as the display title, then a
single line of state ("3 new summaries · 5 waiting in the inbox"). Two figures
top right, not four — the other two moved to Stats. Then the paste-a-URL line
(videos only), status chips, topic chips, and the feed. Day bands become a
kicker plus a hairline. The lead card keeps its promotion rule from `FeedList`
(page 1, unfiltered, unsearched only). Podcast rows use square covers and the
show name in `--color-accent`; video rows use 16:9 with the duration badge.
Left column: the inbox teaser (dark card) above the channel rail, from 1280px.

**Reader — `VideoDetailPage`** (artboards 2, 5)
Reading card plus a 336px rail. Tabs are `Summary / Chapters / Watch`; the
Topics tab is **cut** — topic pills live in the rail, where they already were,
and a full tab was too much screen for a handful of tags. `Watch` replaces
`Transcript` as the label so the tab and the rail's "Open player" button read as
one destination. The rail's dark player card hides while the player is open. On
the phone the reading text drops the card and runs on the paper ground; the
player teaser sits *after* the summary, not before it.

**Player — `TranscriptPlayer`** (artboards 2, 5, 7)
Head pins, cue pane scrolls, cue rows are timestamp + text and clickable. The
search field moved **out** of the dark player block: it belongs to the
transcript, so it sits on the paper ground under a "Transcript · N lines"
kicker, as a white pill with a 1.5px ink border and the hit counter in rust —
the loudest control in the column. Active cue is a filled amber row; a search
hit is an amber `<mark>` plus an inset amber ring on the row, so the two stay
distinguishable.

**Inbox — `InboxPage`** (artboards 8, 13)
Cards with three visible actions: Watch (primary), Summarize, Dismiss. Channel
chips with counts, "Dismiss all" in the header, "Shorts are filtered
automatically" as a note rather than a setting. Queued rows keep full contrast
and an outlined chip. On the phone the three actions are equal-width 44px pills
and Dismiss is outlined, not ghost.

**Channels & shows** (artboards 3, 13)
Two card shelves: video channels with round avatars, podcast shows with square
covers, amber new-badges, the 63 Karakeep one-shot channels folded into a
dashed "+63" card. A dark inbox bar closes the page. Follow-by-handle and the
Takeout import stay where they are. This screen is currently Settings →
Channels; promoting it to the nav is part of the redesign.

**Stats — `StatsPage`** (artboards 9, 13)
Four figure cards (Summaries, Runtime watched, Saved vs 1×, Completed %), Saved
on the dark surface. 30-day chart with today's bar in amber. Top channels with
artwork and bars, top topics as weighted pills, by-status with rust for failure
only, and the failed list as a card — title, the error message verbatim, date.
The scope control (`RangeControl`) becomes a `.seg` pill group.

**Settings — `SettingsPage`** (artboards 10, 13)
Desktop: rail card left (Identity, Karakeep, Channels, Feed, Summaries, LLM,
Podcasts), section card right. `set-row` keeps its label column at 168px; tokens
render in mono on a paper-grounded well with a rust Copy action. Mobile: one
scrolling page of section cards, no rail, label above value. The `.switch` is
now round — a deliberate change from `primitives.tsx`'s "never the native
rounded control", because in a pill-and-card system the square toggle was the
last hard edge.

**New episode — `PodcastNewPage`** (artboards 11, 13)
One centred card: cover, kicker, title, feed metadata, detail level as a pill
group instead of a `<select>`, then the action and the feed description in
serif.

## Not in this package

- **Podcast Listen player** (artboard 7) — designed, deliberately not shippable
  yet. It needs two things vimmary does not have: timed segments from cast2md
  (`GetTranscript` asks for `?format=txt`, "without timestamps") and an audio
  source (`source_url` is the cast2md episode page, and no enclosure is stored).
  The storage shape already fits: `transcript_segments` is `{start, duration,
  text}` and `toSegmentsJSON` writes exactly that, so the vimmary-side change is
  replacing the early `return emptySegments` for podcast rows in
  `GetTranscriptSegments`. The tab is labelled `Listen`, not `Watch`.
  Note: cast2md returns one text stream with no speaker labels, which shows on a
  panel show.
- A dark mobile screen exists for Home only (artboard 13); the other phone
  screens inherit the same tokens and need no separate spec.
