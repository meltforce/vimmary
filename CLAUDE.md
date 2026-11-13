# vimmary — video and podcast summary service

A Go service that turns bookmarked YouTube videos and podcast episodes already
transcribed by cast2md into searchable LLM summaries (Postgres + pgvector; web
UI, MCP endpoint, Atom feeds). It is not a transcription service: livestreams,
Shorts and playlists are out of scope, and it never downloads podcast audio.

`scripts/setup`, `scripts/test`, `scripts/lint` and `scripts/e2e` are the
checks; CI, the hooks in `.claude/settings.json` and sessions call only these.
Run `scripts/lint` and `scripts/test` before committing, `scripts/e2e` as well
for a change to startup, config, migrations, the Dockerfile or `web/`. Where
`docker build` fails (a cloud session), `E2E_NATIVE=1 scripts/e2e` runs it
without the image and without the upgrade test.

## Gotchas

Build and deploy
- `web/dist/` is generated and embedded (`//go:embed`); `go build` fails on a
  fresh checkout. Build the frontend or `mkdir -p web/dist && touch
  web/dist/.gitkeep`.
- `docker-compose.yml` is the development stack: it builds the working tree,
  and its database on port 5434 is what the storage tests use without
  `TEST_DATABASE_URL`. `compose.example.yml` is the public one (`ghcr.io`,
  from `release.yml`). Neither follows the other.
- A merge to `main` deploys to smokey through `deploy.yml` and
  `pipeline-deploy` (homelab), which waits up to 120 s for
  `http://127.0.0.1:8080/version` inside the container to report the full
  commit SHA: the image is built with `VERSION=<sha>`, a tag with
  `VERSION=<tag>`.
- **No network call before the HTTP listener, except tsnet.** `run()` in
  `cmd/vimmary/main.go` is the init order, and every `defer` stays in `run`.
  A secret fetched over the tailnet at startup once left the container up with
  no listener for hours.
- The health endpoint is a loopback listener opened as the last startup step:
  the real listener is on the tsnet netstack, which nothing in the container
  can dial. The `HEALTHCHECK` lives in the `Dockerfile`, not in a compose file.
- setec holds the database password, but Ansible renders it into `.env`;
  vimmary never talks to setec although the package is linked through meltkit.
- `meltkit` is a separate repo: a change there is a release there plus a bump
  here.

Code rules
- Business logic lives in `internal/service/` and is called from both MCP and
  REST handlers. A behaviour in only one transport is a bug.
- Outside `internal/storage`: `storage.ErrNotFound` with `errors.Is`, never
  `pgx.ErrNoRows`; only that package imports `jackc/pgx`.
- A storage dependency that needs testing gets a narrow seam
  (`settingsSource`, `summarizerFactory`, `searchSource`), not one interface
  over `storage.DB`.
- `PodcastSource` must be a nil interface, not a nil `*cast2md.Client`: a
  typed nil turns "podcasts disabled" into a nil dereference.
- A Claude answer is its text blocks, not `content[0]` (a `thinking` block
  comes first). `max_tokens` depends on the level, with headroom for thinking;
  a truncated JSON response is an error, never stored as text.

Data model
- **`videos` holds YouTube and podcast rows; a source-blind query is a bug.**
  Podcast rows carry NULL in `youtube_id` (pass `nil`, not `""`). Queries that
  feed the YouTube pipeline carry `AND source = 'youtube'`.
- `GET /api/v1/videos`, MCP `list_recent` and `/feed/atom/{token}` default to
  videos only; changing a default changes what existing clients and feed
  subscriptions see.
- **Transcripts and summaries are shared across users' rows** for one
  `(source, external_id)`: `AdoptSharedContent` at ingest (SQL, because
  `embedding` has no struct field) and the `Update*` methods for derived
  fields address every such row. `UpdateVideoStatus` does not fan out.
- The summary prompt and model preference are service-wide but stored per
  user; Karakeep key, webhook token and feed token stay private.
- `seed_fixtures: true` writes `internal/storage/fixtures/preview.json` for
  user 1 when `videos` is empty. The preview and e2e run with it, without
  `auth:` and without Tailscale.
- Every embedding carries its model (`videos.embedding_model`); a changed
  `embedding.provider` removes rows from semantic search until
  `reembedStale` has redone them.
- API keys live in the database in plain text and are read when used, so a key
  entered in Settings works without a restart. There is deliberately no
  startup check for a key. The Mistral key also feeds the transcriber and the
  embedder. With `aperture.base_url` set, Claude needs no key.
- Admin is `users.is_admin`; non-admins get 404 on admin routes. Without an
  `auth:` block an instance with no admin falls back to the primary user
  (`storage.GetPrimaryUser`) and records it; with one, only the flag counts.
  Tagged tailnet nodes still resolve to the primary user through WhoIs.
- With an `auth:` block, a request without a bearer token or session falls
  back to WhoIs only while tsnet runs; off the tailnet meltkit's identity
  middleware is DevIdentity (user 1). User 1 (`local`) is what MCP stdio mode
  acts as and cannot be deleted.
- Personal access tokens (`vimmary_pat_…`) exist only with `auth:`. `/mcp`
  takes scope `mcp`, the REST group scope `api`; a session satisfies `api`
  only, so a browser cookie on `/mcp` gets 401 off the tailnet. A bearer
  request never reaches the WhoIs fallback. Without `auth:`, `/mcp` is
  meltkit's WhoIs mount, unchanged.
- The bearer path does not ask the resolver: disabling a user revokes their
  tokens, and `refuseDisabledToken` refuses any that survived. Token requests
  carry an empty `UserInfo`; login and name come from the user table. Machine
  clients' tokens belong to the admin user (`vimmary auth create-token`).
- The Atom feed route is outside the Tailscale auth middleware; the token in
  the path is the only access control, and an invalid one answers 404.

External services
- The YouTube channel feed answers 404 for a live channel in about half of all
  requests; `FetchChannelFeed` retries (404, 5xx, no response). Only a 404 on
  every attempt over several cycles means a wrong or deleted channel.
- The InnerTube spacing is adaptive (10–45 s with queue depth); a bulk import
  that looks stalled is usually that. Two queues, two workers, so a long
  podcast does not block YouTube jobs.
- The Shorts probe is fail-open on first sight; `probeUnprobedInboxItems`
  revisits rows with `shorts_checked_at` NULL.
- YouTube thumbnails are derived from the video ID (`hqdefault`), podcast
  thumbnails come from the feed.
- Karakeep writeback waits 30 s so Karakeep's crawler finishes first, and adds
  the `video-summarized` tag with a POST so AI tags survive.
- The cast2md watermark is opaque text taken from cast2md's response, never
  from vimmary's clock or through Go time parsing.
- A new podcast subscription starts uninitialized: the first poll (run inside
  the PUT, best effort) reads `initial_backfill` episodes and adopts the newest
  as watermark. Backfill and summarize-all never move the watermark.
- `Transcribe all` (`Client.ProcessFeed`) is the only call that makes cast2md
  do work; its results arrive through the ordinary poll, so only for a
  subscribed feed.

Frontend
- `web/src/vimmary.css` is vimmary's own system stylesheet ("Shelf"), edited
  here; rules only vimmary needs go in `web/src/index.css`. The design arrives
  as a handoff package (`design/handoff.md`). Fonts are self-hosted.
- `Layout`'s `<main>` is a flex column: a self-centring block directly under it
  needs `width: 100%`, or it shrinks to its content. `.nav` overrides
  `max-width` and `margin-inline` instead, to paint across the window.
- The Watch tab (`.detail-page.is-player`) is a separate layout: the nav is
  sticky, offsets use `--nav-h` from `useNavHeight()`, `.player-head` takes
  `--color-card`, and the stacked video is capped at `46vh`.
- `/` and `/podcasts` render one `FeedList`; the lead item is promoted only on
  page 1 of an unfiltered list.
- Each Settings section owns its queries and error state; the page holds none.
- The app icon's mark must stay lighter than its field, or the iOS dark and
  tinted variants collapse to an empty square (`npm run icons`).

Decisions and incidents cited by date are in `archive/`.

## Git

GitHub `meltforce/vimmary` is canonical. Work on a `claude/*` branch, open a
PR from the template and enable auto-merge (squash) when opening it; it merges
once `test`, `lint`, `e2e` and `review` are green, and a merge deploys to
production without a further step. No push to `main`. Stage paths explicitly,
never `git add -A` — parallel sessions share this checkout.

Every push to a `claude/*` branch is deployed to the preview
(https://knox.coydog-fence.ts.net:8091/, tailnet only, fixture data); the
result is a comment on the PR and in `gh run list --workflow preview.yml`.

The production compose file and `config.yaml` live in the homelab repo. A PR
that needs a new variable, config key or volume names it in the PR text;
without it the health check fails and the deploy rolls back. A rollback
(`gh workflow run deploy.yml -f sha=<earlier main sha>`) resets only the
image, so the previous image has to start on the new schema.

## Documents

Do not write documentation that restates the code. Reasoning goes into the
commit message, or into a comment next to the code when a later reader would
otherwise undo the change. Everything written into the repo is English.
