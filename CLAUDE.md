# vimmary — video and podcast summary service

A Go service that turns bookmarked YouTube videos and transcribed podcast
episodes into searchable LLM summaries. Video transcripts come from YouTube's
InnerTube API, podcast transcripts from cast2md, summaries from Claude or
Mistral, storage is Postgres + pgvector. Triggers are Karakeep webhooks, manual
URL submission, cast2md feed subscriptions and a deep link from cast2md; results
are served over a web UI, an MCP endpoint and three Atom feeds.

It is not a transcription service — the domain is classic YouTube videos (talks,
tutorials) plus podcast episodes cast2md has already transcribed. Livestreams,
Shorts and playlists are out of scope by decision, and vimmary never downloads
audio for podcasts.

## Gotchas

**`web/dist/` is generated and embedded into the binary.** `web.go` carries
`//go:embed all:web/dist`, so `go build ./cmd/vimmary` fails outright when the
directory does not exist — this is what a fresh checkout looks like, because
`.gitignore` excludes it. Either build the frontend first (`cd web && npm ci &&
npm run build`) or create the stub the CI creates:
`mkdir -p web/dist && touch web/dist/.gitkeep`.

**Two compose files, different audiences.** `docker-compose.yml` is the
development stack and pulls `:edge` from a tailnet-internal registry, which is
unreachable from outside the tailnet. `compose.example.yml` is the one the
README points the public at; it pulls `ghcr.io/meltforce/vimmary:latest`.
Changing one does not change the other.

**Nothing in the startup path fetches a secret over the network, and that is the
point.** The init order in `cmd/vimmary/main.go` is config → tsnet → resolve the
database password from the environment → migrations → DB → services → HTTP
listener, and `run()` *is* that order — each step is a named function
(`startTailscale`, `openDatabase`, `buildService`, `buildHTTPServer`,
`openListener`), so the sequence is readable in one screen rather than inferred
from 254 lines. Every `defer` stays in `run`; moving one into the function that
created the thing it closes closes it immediately. The only remaining network
dependency is tsnet itself. Until
2026-08-07 vimmary read three secrets from setec over the tsnet node during
startup, and a node the tailnet had not yet placed got `access denied` — an
answer the setec store retries forever. That produced a 6h23min outage with the
container up and no listener; see INCIDENTS.md. Reintroducing any network call
before the listener reintroduces that failure class.

**The LLM API keys live in `app_settings` and are read when they are used.**
`Service.getSummarizer` builds a summarizer per summary from the key in the
database, the way `writeBackToKarakeep` builds its client — so a key entered in
Settings → LLM providers works without a restart. There is deliberately no
startup check that the configured provider has a key: a service that refuses to
start cannot serve the page on which the key would be entered. A missing key is
a failed summary with a message saying so.

**A storage dependency that needs testing gets a narrow seam, not one interface
over `storage.DB`.** `internal/service/service.go` declares three:
`settingsSource` (two methods, the summarizer path), `summarizerFactory` and
`searchSource` (two methods, `Search`). `New` wires each to the real
implementation; a test replaces only the one its path needs. The *why* is that a
single interface would carry all 38 methods the service calls, every fake would
implement all of them to exercise one, and a new query anywhere would break every
fake. `Service.db` stays concrete for everything else — see `DECISIONS.md`,
2026-08-07, which also records that `analysis/structure-report.md` recommended
the single interface and why that was not followed.

**`storage.ErrNotFound`, never `pgx.ErrNoRows`, outside `internal/storage`.**
The translation sits in `scanVideo` and `scanSubscription`, which every
single-row lookup passes through, and in the `RowsAffected() == 0` branches.
Compare with `errors.Is`; the `==` comparisons this replaced were one wrapper
away from being wrong and one of them was, answering 500 where the handler
intended 404. Only `internal/storage` imports `jackc/pgx`, and a `grep` for it
outside that package is the check.

**`web/src/vimmary.css` is vimmary's own system stylesheet — "Shelf", not the
shared Modernist language.** vimmary left that language on 2026-08-23
(`DECISIONS.md`, `design/handoff.md`): warm paper ground, white cards, dark
accent surfaces, artwork-forward browsing, serif reading text. The other homelab
apps keep their `homelab.css`; nothing synchronises between apps and none is the
authority over another, so the divergence costs nothing mechanically. The design
is still steered in Claude Design and arrives as a handoff package, which is
what says where a rule belongs: a rule the handoff assigns to the system goes in
`vimmary.css`, a rule only vimmary ever needs goes in `web/src/index.css` below
the import. The why for editing rather than re-copying: the earlier form of the
rule ("never edit; change FreeReps and re-copy") described a synchronisation
nothing performed — FreeReps has no `homelab.css` at all and the two files had
already diverged.

Bricolage Grotesque (display), Public Sans (UI) and Source Serif 4 (reading) are
self-hosted through `@fontsource-variable/*` — a font CDN is unreachable for a
client with no route to it, and the whole UI then renders in `system-ui`. Dark
mode is a full twin driven by `[data-theme]` from `theme.tsx`, `auto` included.
One breakpoint, 768px, read in JS by `useIsDesktop()`, because desktop and phone
are different component trees rather than one reflowed.

**The Shorts filter probes once on first sight and once again later.**
`pollChannel` probes every newly inserted inbox item (`internal/youtube/shorts.go`
— `/shorts/{id}` answers 200 for a Short and redirects for a regular video) and
is deliberately fail-open: a flaky probe lets the video in rather than losing a
real one. `probeUnprobedInboxItems` in `internal/service/channel_poll.go` is what
makes that decision temporary — it revisits `new` rows whose
`shorts_checked_at` is NULL, 25 per cycle, and only a conclusive answer sets the
column. Without the second pass a Short that slipped through stayed forever, and
so did every row inserted before the filter existed: six of them were still
listed when this was written, from a poll 2h43min before the filter landed.

**A 404 from the channel feed usually means nothing.**
`https://www.youtube.com/feeds/videos.xml?channel_id=…` answers 404 for a live
channel with a correct ID in roughly half of all requests — measured 2026-08-25
over 12 consecutive requests each for two channels, unchanged across User-Agent,
Accept header, consent cookie, HTTP/1.1 and the www-less host, with Google's
generic error page as the body rather than YouTube's. `FetchChannelFeed`
therefore retries three times (`channelFeedRetryDelays`), which is what takes the
per-cycle failure probability to ~12%; a retry on 403 or 429 would only spend
requests, so `retryableFeedStatus` covers 404, 5xx and requests without a
response. A channel ID that answers 404 on every attempt across several cycles
is the deleted-or-mistyped case the message reads like.

**Blocks that centre themselves need `width: 100%` under `<main>`.**
`.page-head`, `.hero` and `.filters` hold their measure with
`max-width: var(--reading-w)` plus `margin-inline: auto`. That centres correctly
in a block container; `Layout`'s `<main>` is a flex column, and there an auto
inline margin shrinks the item to its content and centres *that* — the Channels,
Stats and Settings heads rendered at 382px in a 1440px window. The three rules
sit at the end of `index.css`; a new self-centring block placed directly under
`<main>` needs the same.

**The Watch tab is a different layout, not a wider reading column.**
`.detail-page.is-player` (set from `VideoDetailPage` when the transcript tab is
open) takes a 1600px measure, and above 1100px `.player-grid` puts the video and
the transcript side by side: the video pins, the cue pane scrolls inside itself.
Three things follow from that and none of them is optional. The nav is
`position: sticky` — a 700-line transcript scrolled it out of the window and left
no way back except the browser's back button. `--nav-h` is published by
`useNavHeight()` in `Layout.tsx` and is what the pinned player head and rail
offset against; a hard-coded number would hide a strip of video or leave a gap.
And `.player-head` takes `--color-card`, not the system's `--color-bg`, because
on the reading card that painted a beige frame around the video. Stacked below
1100px the frame is capped at `46vh` worth of width, or the pinned video covers
the window and nothing of the transcript can scroll into view.

**`thumbnail_url` is filled by two different mechanisms, and for a long time by
only one.** Podcast rows take it from the feed's `image_url`
(`internal/service/podcast.go:113`); YouTube rows derive it from the video ID
through `youtube.ThumbnailURL` — `https://i.ytimg.com/vi/<id>/hqdefault.jpg` —
because InnerTube's metadata response carries no thumbnail. `hqdefault` is the
only variant that always exists, and its letterbox bars are exactly what the
feed's `object-fit: cover` crops off. Until 2026-08-07 the YouTube path did not
write the column at all, so every video row was NULL and the new media feed
showed the neutral "no art" block for all of them; migration `000013` backfilled
them. The check that missed it verified that the list query *returns* the column
(`videoColumnsNoTranscript`) without asking whether anything *writes* it.

**The two library screens are a media feed, not a table.** `/` and `/podcasts`
both render `web/src/components/FeedList.tsx` — one centred `--reading-w` column,
artwork on every row, `excerpt(summary, title)` as the row's body — and the
desktop/mobile fork is gone for those two, because a feed row is already a
stacked layout. A `completed` row shows no status mark: that is the norm, and the
column it replaced repeated `done` several hundred times. The lead item is
promoted only on page 1 of an unfiltered, unsearched list, since promoting the
first row of a filtered list would be a claim about ranking that the query does
not support.

**The app icon's mark must stay lighter than its field.**
`web/scripts/build-icons.mjs` (`npm run icons`, needs Chrome and ImageMagick, not
wired into the build) writes `web/public/app-icon/*` — `vm` in `#f3f2f2` on
`#ec3013`. iOS 18 derives the dark and tinted home-screen variants from that one
file, and a dark mark on a mid field collapses to an empty rounded square in
both. The failure is invisible on desktop; FreeReps shipped it once and fixed it
in `c7319b0`.

**Each Settings section owns its queries and its own error state**
(`web/src/pages/settings/`, one file per concern). The page holds none. The why:
a shared `isLoading`/`errorObj` pair is a hard conjunction, so one failing
backend blanked the whole page — `LLMSection` already had to opt out because the
server answers 404 to a non-admin.

**The Mistral key is not only the summarizer's.** The same `app_settings` value
feeds `internal/mistral.Client`, which is both the embedder and the podcast
transcriber. Changing it in the UI changes all three, which is intended —
`DECISIONS.md` records that embeddings are deliberately not swappable.

**Admin is the primary user, and that rule predates the Settings page.**
`storage.GetPrimaryUser` — first login containing `@`, by `created_at` — is part
of meltkit's `UserStore` interface and is what the identity middleware already
resolves every tagged device to. The service-wide settings reuse it rather than
adding a second notion of owner. Non-admins get 404, not 403.

**The `HEALTHCHECK` lives in the `Dockerfile`, not in a compose file**, because a
compose probe is owned by whichever repo holds the deployment. The one added to
homelab on 2026-08-06 lasted 94 seconds before an auto-rollback removed it, and
vimmary then ran dead with nothing watching.

**The health endpoint is on loopback, and it is deliberate.** With Tailscale
enabled the real listener runs on the tsnet netstack, which nothing inside the
container can dial — a healthcheck against localhost would fail on a perfectly
healthy service. `server.StartHealthListener` opens `health_addr` (default
`127.0.0.1:8081`) as the *last* step of startup, so the listener existing is
itself the readiness signal. `/version` on the public router carries the build
string for the CI deploy gate; `Version` reaches the binary through the
Dockerfile's `ARG VERSION` and `-X main.Version`, and reads `dev` without it.

**The Atom feed route is mounted outside the Tailscale auth middleware**
(`internal/server/server.go:39`). The 32-byte token in the URL path is the only
access control, by design — an RSS reader cannot authenticate over Tailscale. An
invalid token returns 404 rather than 403, so the response does not confirm that
a token exists.

**Business logic lives in `internal/service/` and is called from both
transports.** MCP tool handlers (`internal/mcp/tools.go`) and REST handlers
(`internal/server/handlers.go`) both call service methods. A behaviour
implemented in one handler and not the other is a bug, not a feature gap.

**Karakeep writeback waits 30 seconds** (`internal/service/process.go:227`) so
Karakeep's own crawler finishes first; writing earlier loses the note. The
`video-summarized` tag is added with a POST rather than a full tag replacement,
which is what preserves Karakeep's AI-generated tags.

**The YouTube rate limit is adaptive, not fixed.** `adaptiveDelay()` in
`internal/service/service.go:96` scales the spacing between InnerTube calls from
10 s to 45 s with queue depth. A bulk import that looks stalled is usually this
working as intended.

**Every API key lives in the database and is configured through the Settings
page.** Karakeep keys are per-user (`users.karakeep_api_key`); the LLM keys and
the summary provider are service-wide (`app_settings`) and only the primary user
sees them. All of them are stored in plain text — a decision with its cost and
its trigger recorded in `DECISIONS.md`, not an oversight.

**setec still holds the database password, but vimmary never talks to it.**
Ansible reads `docker/vimmary/db-password` at deploy time and renders it into
the stack's `.env`; the container receives it as `VIMMARY_POSTGRES_PASSWORD`,
which meltkit's resolver checks before anything else.

Precisely: `InitSetecStore` is never called, so no setec code runs and no
request is made. The package is still *linked* — meltkit's `secrets` resolver
imports it, and vimmary uses that resolver for the environment-then-literal
chain. `go list -deps ./cmd/vimmary` therefore still names
`github.com/tailscale/setec/client/setec`. Linked and unreachable is not the
same as absent, and only the second would be worth the divergence from the
resolver every other service here uses.

**`videos` holds both kinds of row, and source-blind queries are bugs.** A
`source` column discriminates `youtube` from `podcast`; podcast rows carry NULL
in `youtube_id` (that is why `InsertVideo` passes `nil`, not `""` — the
`UNIQUE(user_id, youtube_id)` index from `000003` treats NULLs as distinct but
not empty strings). `ListFailedVideos`, `ListVideosWithoutMetadata` and
`ListNoCaptionsVideos` therefore carry `AND source = 'youtube'`: their callers
hand `v.YouTubeID` to the YouTube pipeline, and a podcast row there produces a
job with an empty ID.

**The separation between videos and podcasts is a default, not a filter the
caller must remember.** `GET /api/v1/videos` and MCP `list_recent` default to
`source=youtube`; `/feed/atom/{token}` stays videos-only and the combined feed is
a separate path. Anything that changes those defaults changes what existing
clients and existing RSS subscriptions see.

**The cast2md watermark is opaque text, deliberately.** cast2md writes naive
local timestamps (`TIMESTAMP` without zone, from `datetime.now().isoformat()`).
`podcast_subscriptions.watermark` stores the string as it came back and always
takes it from cast2md's response, never from vimmary's clock — a round trip
through Go's timezone handling is the one place episodes would be skipped
without any error.

**A new podcast subscription starts uninitialized on purpose.** The first poll
reads `initial_backfill` episodes with `order=updated_desc`, summarizes them and
adopts the *newest* of them as the watermark — so the batch is never fetched
twice. `initial_backfill = 0` restores plain "from now on". Backfill,
summarize-all and transcribe-all never move the watermark.

**`SetPodcastSubscription` runs that first poll itself**, inside the PUT, so the
backfill does not wait up to a poll interval. It is best effort: the
subscription is committed first, and a failure leaves `initialized` false for
the ticker to retry. It runs only when the feed is enabled *and* uninitialized,
so editing the detail level of a running subscription does not re-summarize its
backfill.

**`Transcribe all` is the only call that makes cast2md do work.** Everything
else reads. `Client.ProcessFeed` is the single non-GET in `internal/cast2md`,
and the episodes it produces reach vimmary through the ordinary poll, because
finishing a transcription updates the episode's `updated_at`. On a feed that is
not subscribed, they never arrive.

**Two queues, two workers** (`internal/service/service.go`). `adaptiveDelay()`
compensates for YouTube's rate limit, which does not apply to cast2md, and a
three-hour episode on a shared worker would block every YouTube job for the
length of its LLM call.

**`PodcastSource` must be a nil interface, not a nil `*cast2md.Client`.** Every
podcast entry point tests `s.cast2md == nil`; a typed nil pointer stored in the
interface is not nil and would turn "podcasts disabled" into a nil dereference.
`buildService` in `cmd/vimmary/main.go` declares the variable with the interface
type for exactly that reason.

**`max_tokens` is level-dependent** (`internal/summary/claude.go`): 4096 for
`medium`, 16000 for `deep`. A truncated response ends inside its JSON object, and
`parseSummaryJSON` returns an error for that instead of falling back to raw text
— the old fallback stored cut-off summaries as if they were complete.

**`meltkit` is a separate repo, not a vendored directory.**
`github.com/meltforce/meltkit` supplies db, config, secrets, middleware, server
and MCP helpers. A change needed there is a release there plus a bump here.

## Repo documents

These documents carry state over time. The axis is where a thing *is*, not what
it is about.

| File | Holds |
|---|---|
| `ROADMAP.md` | Open work only. Status token `[open]`. |
| `DECISIONS.md` | Decisions taken, including decisions not to do something — those are the ones most likely to be re-derived from scratch otherwise. |
| `INCIDENTS.md` | Postmortems for things that broke. Newest first. |

`analysis/structure-report.md` is not one of these and does not carry state: it
is a measurement, dated and pinned to a commit, with the command for every
number. Sections are appended when it is re-measured rather than edited, because
a figure and the commit it was taken at belong together. Anything it *implies*
becomes a row in `ROADMAP.md` or an entry in `DECISIONS.md`.

**The movement rule.** When an item closes it is *removed* from `ROADMAP.md`,
and its reasoning moves to whichever document above holds that kind of thing.
Nothing is struck through — a struck-through row is a row that should have been
moved. Status tokens are exactly `[open]`, `[done YYYY-MM-DD]`,
`[dropped YYYY-MM-DD]`; emoji never carry status.

**Before closing an item, read its entry for residual work, dates, or
triggers.** Each of those becomes its own `[open]` row before the entry leaves
the roadmap. This is the step that gets skipped, and skipping it is how
finished-looking work quietly loses its tail.

**No new top-level documents** unless the concern is genuinely orthogonal to the
ones above. Everything else lives inside a project directory.

**Operational exceptions never live in documentation.** Excluding a host from a
run, skipping a check for one case — those belong in configuration, in a
condition, in the inventory. Documentation may reference them; it may not
replace them.

Every rule written here carries a one-line **why**, so it can be revisited when
the context that produced it changes.

## Language

English is the only language used inside this repo. This is not a style
preference — German prose describing English identifiers forces a translation
layer ("Rolle" in the text, `role` in the YAML) and breaks keyword search
between an explanation and the code it explains.

Applies to every document, every code comment and docstring in every language
present, log messages, error strings, user-facing CLI output, identifiers
(variables, functions, roles, tags, unit names, secret paths), and commit
messages.

Number and date formats follow the English convention: `1.82 GB` (decimal
point), `217,226` (comma as thousands separator), `2026-08-06` (ISO 8601, never
`06.08.2026`).

The only exception is a verbatim quote of external output — an upstream error
message, vendor documentation — which keeps its original wording.

Conversation language is independent of this and follows the operator.

## Git workflow

Single developer. **`main` is the only long-lived branch** — commit straight to
`main`, never open a PR for this repo. This overrides the harness default of
branching before committing.

**Commit and push autonomously** once a coherent change is complete and
verified. No approval needed per commit.

**Stage explicitly, never `git add -A`.** Parallel sessions run in this same
checkout; a blanket add sweeps up their work in progress and commits it under
your message. Name the paths you touched.

### Parallel sessions

Isolate concurrent sessions with worktrees:

```bash
claude --worktree <name>          # .claude/worktrees/<name>, branch worktree-<name>
```

A worktree branch is **ephemeral plumbing, not a feature branch**. Never push it
as a branch, never open a PR from it. Land the work on `main`:

```bash
git fetch origin
git rebase origin/main
git push origin HEAD:main          # a rejection means another session landed first
```

Rebase and push again rather than forcing.

Do **not** start background sessions for work that edits this repo — a
background session commits, pushes its own branch and opens a draft PR without
asking, and is hard-wired never to push to `main`. Background sessions are fine
for read-only investigation.

**If a harness rule conflicts with this, this file wins.** `main` *is* the review
surface here and `git revert` is the undo. Say plainly which rule you are
setting aside, then land the work. Do not stop at "the commit is ready, please
push it yourself" — that hands back a half-finished task.

### Mirroring

Forgejo is canonical. `github.com/meltforce/vimmary` is a push mirror and is
asynchronous — a tag pushed here appears there a minute or so later, which is
why the release job in CI waits for it before creating the GitHub release.

## Skills

A skill lives in exactly one home, decided by *what it touches* — not by where
you were when you wrote it.

| Home | For |
|---|---|
| `.claude/skills/<name>/` in this repo | Skills that depend on this project: its scripts, its services, its MCP servers. Committed here. |
| `~/.claude/skills/<name>/` | Skills that work anywhere and carry no project dependency. |

Decide the home before writing. If the skill would fail outside this project, it
belongs here.

**Those two paths are the only places Claude Code looks**, plus a plugin's own
`skills/` directory. A `skills/` directory at the repo root is not a discovery
location — a skill placed there is invisible until something links it into
`.claude/skills/`, and a link that is not committed exists on one machine only.

`.gitignore` therefore uses `.claude/*` with an explicit `!.claude/skills/`
negation, which is what keeps session state out while committing the skills.

A `description` carries the literal trigger phrases that should invoke the
skill, in the languages they are spoken in, plus the cases that should *not*
invoke it. The description is the only part loaded into every session, so it
does the whole job of routing.

An entry under `.claude/skills/` may be a symlink to a directory elsewhere on
disk — Claude Code follows it and reads `SKILL.md` from the target. That is
worth using when a skill genuinely has to live somewhere else; it is not worth
using to keep the source outside `.claude/`, because the link then has to be
recreated on every checkout.

## Verification

Run the `verify` skill in `.claude/skills/verify/` before committing. The steps are kept
there rather than here because they are needed rarely and are long, and in this
file they would occupy every session's context.
