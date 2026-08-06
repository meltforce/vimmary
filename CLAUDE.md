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

**tsnet starts before secrets, because setec is reached over it.** The init
order in `cmd/vimmary/main.go` is config → tsnet → setec resolver → resolve
secrets → migrations → DB → services → HTTP server. Moving secret resolution
earlier leaves setec without a transport.

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

**Karakeep API keys are per-user and live in the database**, configured through
the Settings page — they are not setec secrets. The global secrets are exactly
`vimmary/postgres-password`, `vimmary/mistral-api-key` and
`vimmary/claude-api-key`.

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
`cmd/vimmary/main.go` declares the variable with the interface type for exactly
that reason.

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
