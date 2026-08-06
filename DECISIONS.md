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
