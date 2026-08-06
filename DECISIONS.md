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
now on". The watermark lands on the newest of that first batch, so the batch is
never fetched twice. The alternative — summarizing a feed's whole history on
subscribe — turns one checkbox into hundreds of LLM calls, and is available as
an explicit action instead.

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
