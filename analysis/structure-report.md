# Structure report

Static structural analysis of the vimmary repository, run 2026-08-06 against
commit `105edde`. The analysis works on metrics and import relations, not on
code content; no file was read in full except the documents listed in section 1.

Measurement and interpretation are kept apart throughout. A statement marked
**Measured** is a number reproducible with the command given in the appendix. A
statement marked **Conjecture** is an inference drawn from those numbers and was
not verified against the code.

Sections 1–3 and the appendix state what is the case and make no
recommendations. Section 4 is separate and does recommend, drawing only on the
numbers above it.

**Added after the first pass.** Test coverage (section 5.10) was measured in a
second run, after the first pass had counted test *lines* only. The two are not
interchangeable: `internal/service` carries 961 test lines and covers 13.4% of
its statements. Section 2.5 and section 4 both rest on the coverage figures, not
on the line counts.

---

## 1. Intended architecture

Source: `CLAUDE.md`, `DECISIONS.md` (entries of 2026-03 and 2026-08-06),
`README.md`. This is stated architecture, not conjecture — each claim below
carries the document that asserts it.

| Layer | Packages | Asserted rule | Source |
|---|---|---|---|
| Entry point | `cmd/vimmary` | Fixed init order: config → tsnet → setec resolver → resolve secrets → migrations → DB → services → HTTP server. | `CLAUDE.md`, "tsnet starts before secrets" |
| Transport | `internal/server`, `internal/mcp`, `internal/feed` | Calls service methods and "contain no logic of their own". A behaviour in one transport and not the other is a bug. | `DECISIONS.md` 2026-03, "business logic lives in the service layer" |
| Business logic | `internal/service` | Holds all business logic. Called from both transports. | `DECISIONS.md` 2026-03; `CLAUDE.md` |
| Adapters | `internal/youtube`, `internal/cast2md`, `internal/karakeep`, `internal/mistral`, `internal/summary`, `internal/models` | External systems reached behind interfaces. `PodcastSource` is held as an interface type, never as a typed nil pointer. | `CLAUDE.md`, "`PodcastSource` must be a nil interface" |
| Persistence | `internal/storage` | Postgres + pgvector. Carries the `source` discriminator; source-blind queries are bugs. | `CLAUDE.md`, "`videos` holds both kinds of row" |
| Shared infrastructure | `github.com/meltforce/meltkit` (external module) | db, config, secrets, middleware, server, MCP helpers. Not vendored. | `DECISIONS.md` 2026-03, "shared infrastructure lives in meltkit" |

The implied dependency direction is `cmd` → transport → service → adapters →
storage. No document states the rule in that form; it follows from the layer
descriptions above and is treated here as the yardstick for section 2.

One further asserted rule bears on the measurements: the Atom feed route is
mounted **outside** the Tailscale auth middleware and the URL token is the only
access control (`CLAUDE.md`; `internal/server/server.go:39`). Token resolution
in the feed layer is therefore intended, not accidental.

Frontend layering is **conjecture** — no document states one. Directory names
suggest `main.tsx` → `App.tsx` → `pages/` → `components/` → `api.ts`.

---

## 2. Cycles and layer violations

### 2.1 Cycles

**Measured.** None, on either side.

- Go, package level: 0 cycles. The Go compiler rejects import cycles, so this
  is a property of the language, not evidence about this codebase. Reported for
  completeness.
- Go, file level: **not applicable.** Go imports are per-package, not per-file;
  there is no file-level import edge to test.
- TypeScript, file level: 0 cycles across 19 files
  (`madge --circular --extensions ts,tsx src`).

### 2.2 The driver error type crosses the storage boundary — 7 call sites

**Measured.** `jackc/pgx/v5` is imported by 7 non-test files. Four are inside
`internal/storage`, which is where the layer table places the driver. Three are
outside it, all in the transport layer:

| File and line | Symbol used |
|---|---|
| `internal/server/handlers.go:116` | `pgx.ErrNoRows` |
| `internal/server/handlers.go:153` | `pgx.ErrNoRows` |
| `internal/server/handlers.go:178` | `pgx.ErrNoRows` |
| `internal/server/handlers.go:203` | `pgx.ErrNoRows` |
| `internal/server/handlers.go:228` | `pgx.ErrNoRows` |
| `internal/feed/handler.go:63` | `pgx.ErrNoRows` |
| `internal/mcp/tools.go:72` | `pgx.ErrNoRows` |

`pgx.ErrNoRows` is the only pgx symbol referenced outside `internal/storage`;
no connection, transaction, or row type crosses the boundary.

**Rule bearing on this.** The layer table places Postgres in
`internal/storage`. `DECISIONS.md` 2026-03 states transport handlers "contain no
logic of their own".

**Conjecture.** Mapping "row not found" to a 404 is the shared behaviour at all
seven sites, and all three transports had to learn the driver's sentinel value
to do it. A storage-level sentinel would confine that knowledge to one package.
The blast radius of a driver swap is 7 lines in 3 packages, which is small in
absolute terms — the claim here is about the direction of the dependency, not
its cost today.

### 2.3 Two transports hold a `*storage.DB` and call it directly — 3 call sites

**Measured.** `internal/server` and `internal/feed` each hold a `*storage.DB`
alongside their `*service.Service`:

- `internal/server/server.go:18` — field `store *storage.DB`
- `internal/feed/handler.go:38,43,49,53` — parameter `store *storage.DB`

Methods invoked on it, outside `internal/storage`:

| File and line | Call | Note |
|---|---|---|
| `internal/feed/handler.go:61` | `store.GetUserByFeedToken` | Resolves the feed token to a user ID |
| `internal/server/server.go:46` | `s.store.GetUserByWebhookToken` | Passed as a function value into `karakeep.WebhookHandler` |
| `internal/server/health.go:22` | `s.store.Ping` | Health probe |

That is the complete set — 3 direct storage calls from transport packages
across the whole repository.

**Rule bearing on this.** `DECISIONS.md` 2026-03: transports call service
methods. Token resolution is an authentication decision, not a transport
concern, under the layer table in section 1.

**Counter-evidence, measured.** `internal/mcp` imports `internal/storage` but
invokes no method on it — it uses the package only for the types `ListFilters`,
`SourceYouTube` and `SourcePodcast`. The third transport does not take this
route.

**Conjecture.** `store.Ping` at `health.go:22` is a liveness probe rather than
business logic, and routing it through the service layer would add an
indirection with no decision in it. The two token lookups are the substantive
cases: both answer "which user is this request", which is the same question the
Tailscale middleware answers for every other route.

### 2.4 No inverted edges

**Measured.** The full internal import graph is 25 edges. Every edge runs
`cmd` → transport → service → adapters/storage, or transport → storage
(section 2.3). No edge runs against the direction implied in section 1 — in
particular, `internal/service` imports no transport package, and
`internal/storage` imports nothing from `internal/`.

**Measured.** `internal/service` has exactly one non-stdlib, non-meltkit
external dependency: `google/uuid`. The transport frameworks (`go-chi/chi`,
`mark3labs/mcp-go`) appear only in `internal/server`, `internal/feed`,
`internal/mcp` and `cmd/vimmary`.

### 2.5 Testability ends at the interface boundary

**Measured.** Six packages contain no test file at all. Test lines per package
(`*_test.go`, `wc -l`), with statement coverage from section 5.10:

| Package | Non-test LOC | Test LOC | Coverage | Funcs at 0% |
|---|---|---|---|---|
| `internal/server` | 986 | **0** | 0.0% | 40 of 40 |
| `cmd/vimmary` | 299 | **0** | 0.0% | — |
| `internal/mcp` | 273 | **0** | 0.0% | 7 of 7 |
| `internal/feed` | 328 | **0** | 0.0% | 9 of 9 |
| `internal/models` | 218 | **0** | 0.0% | 6 of 6 |
| `internal/mistral` | 200 | **0** | 0.0% | 6 of 6 |
| `internal/service` | 1903 | 961 | **13.4%** | 51 of 64 |
| `internal/storage` | 1038 | 663 | 37.3% | 24 of 52 |
| `internal/summary` | 532 | 222 | 38.6% | 5 of 13 |
| `internal/cast2md` | 326 | 269 | 82.9% | 1 of 15 |
| `internal/karakeep` | 271 | 248 | 29.5% | 5 of 7 |
| `internal/youtube` | 221 | 43 | 3.9% | 3 of 4 |
| `internal/config` | 110 | 151 | 90.9% | 0 of 1 |

**Measured.** The three packages carrying the layer violations of sections 2.2
and 2.3 — `server`, `feed`, `mcp` — are three of the six with zero test lines.
Combined, those six hold 2304 non-test lines, 45% of the 5105 non-test Go lines
in the repository.

**Measured.** Coverage tracks the declaration of each dependency in
`internal/service/service.go:56`, not the age or size of the code:

| Field | Declared as | Coverage of its callers |
|---|---|---|
| `cast2md PodcastSource` | interface | `pollSubscription` 88.9%, `EnsureEpisodeRow` 76.5%, `TranscribeAllInFeed` 75.0%, `SetPodcastSubscription` 61.5% |
| `embedder Embedder`, `transcriber Transcriber`, `summarizers map[string]summary.Summarizer` | interface | partially covered |
| `db *storage.DB` | concrete | `Search` 0.0%, `GetStats` 0.0% |
| `yt *youtube.Client` | concrete | `FetchTranscript` 0.0%, `ProcessVideo` 0.0% |

`internal/service/podcast_poll_test.go:24` defines a `fakeSource` implementing
`PodcastSource`; the comment at line 95 records that these tests stop at the
queue and never reach `db`. The same boundary repeats one level up:
`server.New(svc *service.Service, store *storage.DB, ...)`
(`internal/server/server.go:23`) takes both concretely.

**Measured.** Of the 17 Go functions at CC ≥ 10 (section 5.6), 15 are at 0.0%
coverage. The two that are not — `pollSubscription` (CC 15, 88.9%) and
`SetPodcastSubscription` (CC 14, 61.5%) — both sit behind `PodcastSource`.

**Measured.** The `internal/storage` figure of 37.3% is reachable only with a
local database. The `verify` skill (`.claude/skills/verify/SKILL.md`) records
that those tests skip when `CI` is set, so the coverage effective in CI is lower
than the table states.

**Conjecture.** The distribution reads as a consequence of the constructor
signature rather than of testing discipline: the podcast path written on
2026-08-06 is the one subsystem whose external dependency was declared as an
interface, and it is the one subsystem with coverage. `CLAUDE.md` documents that
`PodcastSource` is an interface for an unrelated reason — nil handling — so the
testability appears to be a side effect of that decision.

---

## 3. Hotspot ranking

Crossing size, complexity and change pressure. **The change-pressure axis is
weaker here than the brief assumes**: the repository's entire history is 114
commits between 2026-03-09 and 2026-08-06, five months. The 18-month window
therefore captures everything, and no file exists that is "large but untouched
for 18 months". Commit counts below are counts over the full history; the
percentage is of all 114 commits.

Thresholds used, with their source:

- **Function length > 80 lines** — given in the analysis brief.
- **Cyclomatic complexity ≥ 10** — the conventional McCabe threshold, and
  `gocyclo`'s own documented "worth looking at" grade. Measured at `-over 9`.
- **File size** — no external threshold is asserted. Files are ranked by
  measured line count relative to the rest of this repository.

| # | Path | Lines | Complexity | Commits | Suspicion (conjecture) |
|---|---|---|---|---|---|
| 1 | `web/src/pages/SettingsPage.tsx` | 1029 | 4 components ≥ CC 11 (36, 28, 14, 11); 22 hook calls | 11 (9.6%) | Largest file in the repository, and the only one high on all three axes. 22 `useState`/`useQuery`/`useMutation` calls in one file is 2.4× the next highest (VideoListPage, 9). Settings carries Karakeep keys, prompts, models, feed tokens and podcast subscriptions — five unrelated concerns measured in one module. |
| 2 | `cmd/vimmary/main.go` | 299 | `main` CC 34, 264 lines | 15 (13.2%) | Highest complexity in the Go code, and 264 of the file's 299 lines sit in a single function. `CLAUDE.md` documents a fixed six-step init order plus the `tsServer.Up()` sequencing that caused two outages on 2026-08-06 — all of it inside this one function, with 0 test lines in the package. |
| 3 | `web/src/pages/VideoListPage.tsx` | 760 | `VideoListPage` CC 74, 558 lines | 10 (8.8%) | Highest single-function complexity measured anywhere in the repository. See the JSX caveat below. |
| 4 | `web/src/pages/VideoDetailPage.tsx` | 673 | `VideoDetailPage` CC 71, 636 lines | 10 (8.8%) | Second-highest complexity; 636 of 673 lines in one component, 12 hook calls. |
| 5 | `internal/server/handlers.go` | 566 | max CC 9 — below threshold | 16 (14.0%) | Second-most-changed file in the repository, and large, but no function reaches CC 10 and none exceeds 80 lines. Reads as many thin handlers rather than complex ones. Carries 5 of the 7 `pgx.ErrNoRows` sites from 2.2. Package has 0 test lines. |
| 6 | `internal/service/process.go` | 271 | `ProcessVideo` CC 23, 143 lines | 15 (13.2%) | The processing path both sources share, per `DECISIONS.md` 2026-08-06. Third-highest Go complexity, and one of the three most-changed Go files. |
| 7 | `internal/storage/videos.go` | 609 | `GetStats` CC 23, 135 lines | 12 (10.5%) | Largest Go file. Complexity concentrates in one reporting query; the remaining ~474 lines carry no function above CC 9. |
| 8 | `internal/server/server.go` | 100 | max CC 9 | **17 (14.9%)** | Highest change pressure in the repository against the smallest size in this table. Every new route touches it, which is what a router file does. Carries the layer violation at line 46. |
| 9 | `internal/service/service.go` | 493 | `adaptiveDelay` and others all < CC 10 | 14 (12.3%) | Large and frequently changed, but no function exceeds 80 lines or CC 9 — the size is spread across many small functions. |
| 10 | `web/src/api.ts` | 430 | no function ≥ CC 10 | 14 (12.3%) | Highest fan-in in the frontend: imported by 9 of 19 files. Changes here reach the widest surface, though the file itself measures as flat. |
| 11 | `internal/service/search.go` | 181 | `Search` CC 23, 133 lines | 5 (4.4%) | Complexity equal to `ProcessVideo` and `GetStats`, at a third of the change pressure. RRF fusion of two result sets in one function. |
| 12 | `internal/service/podcast.go` | 420 | `SetPodcastSubscription` CC 14; `ProcessEpisode` CC 12 | 4 (3.5%) | Written 2026-08-06; low commit count reflects age, not stability. |
| 13 | `internal/mistral/client.go` | 200 | `post` CC 17, `Transcribe` CC 12 | ≤ 4 | High complexity, low change pressure, 0 test lines. Ranked low because the change axis is near zero; retry and error handling in `post` accounts for the CC. |
| 14 | `internal/feed/atom.go` | 230 | `BuildFeed` CC 10, 82 lines | 7 (6.1%) | At both thresholds exactly, on all three axes moderate. |

**Caveat on frontend complexity, measured.** The CC figures for `.tsx`
components include JSX conditional rendering — `&&`, `?:` and `??` inside the
returned markup each count as a branch, exactly as they do in the Go
measurement. CC 74 for `VideoListPage` therefore mixes control flow with
conditional markup and is **not** comparable to CC 34 for `main`. The hook
counts in the appendix are the size signal for these files that does not have
this problem.

**Conjecture on rank 5 vs. rank 2.** `handlers.go` changes more often than
`main.go` (16 vs. 15) and is nearly twice its size, yet ranks lower, because
its complexity distribution is flat: 566 lines with no function above CC 9
means the change surface is wide but each change is local. `main.go`
concentrates 264 lines and CC 34 into one function whose ordering constraints
are documented as having produced two outages, and the package has no tests.

---

## 4. Recommendations

Unlike sections 1–3, this section interprets. Each item names the measurement it
rests on, so it can be re-checked when the numbers change.

The coverage measurement changed the ranking. Read from test *lines*, the
finding was "five or six packages lack tests" and the natural order was to write
tests per package. Read from coverage, the gaps in `server`, `mcp`, `feed` and
`service` have a single shared cause, and the order changes accordingly.

### 4.1 Put `*storage.DB` behind an interface in the service — first

**Rests on.** Section 2.5: 15 of 17 functions at CC ≥ 10 sit at 0.0% coverage,
and the two that do not are the two reached through the `PodcastSource`
interface. `internal/service/service.go:57` declares `db *storage.DB`
concretely; `internal/server/server.go:23` repeats the pattern with both
`*service.Service` and `*storage.DB`.

**Why first.** This is the constraint, not the symptom. As long as `Service.db`
is a concrete type, every function touching the database needs a live Postgres
to exercise, which is why `internal/service` reaches 13.4% across 961 test
lines. Writing handler tests before this lands means writing tests that also
need a database.

**Scope.** The interface only needs the methods the service actually calls; the
existing `Embedder` and `Transcriber` declarations in the same file
(`service.go:29,34`) are the pattern to follow. `podcast_poll_test.go:24` shows
the resulting test shape with `fakeSource`.

**Not recommended as part of this.** `internal/storage` keeping its concrete
`*pgx` usage — the interface belongs on the consumer side, and the four in-package
pgx imports are where the layer table places them.

### 4.2 Cover `Search` next

**Rests on.** `internal/service/search.go:31`, CC 23, 133 lines, 0.0% coverage.
`search_test.go` already exists at 214 lines and covers the fusion helpers
(`TestRRFMerge`, `TestRRFScoring_RankOrder`), so the RRF arithmetic is tested and
the function wrapping it is not.

**Why this one before `ProcessVideo`.** `Search` combines two result sets and
returns; it calls no external system beyond the database. `ProcessVideo`
(CC 23, 15 commits, 0.0%) additionally reaches YouTube, an LLM and Karakeep, so
it needs `yt *youtube.Client` (`service.go:61`) behind an interface as well —
worth doing, but after 4.1 has proven the pattern on the cheaper case.

### 4.3 Split `main()` into named init steps

**Rests on.** `cmd/vimmary/main.go:36`, CC 34 — the highest in the Go code —
with 264 of the file's 299 lines in one function, 15 commits, 0.0% coverage.
`INCIDENTS.md` records two outages on 2026-08-06 caused by the ordering inside
this function.

**Scope.** The six steps `CLAUDE.md` already names — config, tsnet, setec
resolver, secret resolution, migrations, services — become six functions, which
puts the ordering guarantee in the code rather than only in prose, and gives
`tsServer.Up()` after `Start()` a place a test can hold it.

### 4.4 Split `SettingsPage.tsx` along its five concerns

**Rests on.** 1029 lines, the largest file in the repository; 22 hook calls,
2.4× the next highest (`VideoListPage`, 9); 11 commits.

**Why it is low-risk.** The five concerns — Karakeep key, prompts, model
selection, feed token, podcast subscriptions — share no state, each holding its
own queries and mutations. `PodcastFeedRow` (line 369), `PromptEditor` (line
235) and `ModelSelector` (line 148) are already separate components inside the
file, so the split lines exist.

**Independent of 4.1–4.3.** Different codebase, no shared blocker; it can run in
parallel or not at all without affecting the others.

### 4.5 Introduce `storage.ErrNotFound` opportunistically

**Rests on.** Section 2.2: 7 comparisons against `pgx.ErrNoRows` in 3 transport
packages.

**Not worth its own change.** 7 lines, and no driver migration is planned. Worth
doing when `handlers.go` is being edited anyway, since 5 of the 7 sites are
there.

### 4.6 Explicitly not recommended

Each of these appears in section 3 with a number that invites action, and the
number does not support it.

| Item | Measurement | Why it is left alone |
|---|---|---|
| `VideoListPage` CC 74 | Highest CC measured | The figure counts JSX conditional rendering and is not comparable to Go CC (section 3 caveat). Its hook count, 9, is unremarkable. |
| `handlers.go` 566 lines | 2nd-most-changed file | No function reaches CC 10 or 80 lines. This is width, not a knot. Splitting it moves lines without reducing any measured quantity. |
| `GetStats` CC 23 | 3rd-highest Go CC | One reporting query. It is at 0.0% coverage, so it is a test candidate under 4.1 — not a restructuring candidate. |
| The 3 direct storage calls (2.3) | 3 call sites | `store.Ping` is a liveness probe; routing it through the service adds an indirection containing no decision. The two token lookups are worth folding in only if authentication is being touched for another reason. |
| `internal/mistral/client.go` `post` CC 17 | 2 functions ≥ CC 12 | Retry and error handling, ≤ 4 commits over the full history. Complexity without change pressure. |

### 4.7 What this analysis did not measure

- **Whether REST and MCP actually diverge.** `DECISIONS.md` 2026-03 asserts they
  must not. The measurement here shows only that no test would detect it: both
  transports sit at 0.0%.
- **Coverage quality.** `go test -cover` reports statement coverage, which does
  not distinguish an asserted result from an executed line.
- **Frontend coverage.** No test runner is configured under `web/`; this was not
  measured and no claim is made about it.

---

## 5. Raw data

Every command below was run from the repository root at commit `105edde` on
2026-08-06, Go 1.26.5, on darwin/arm64.

### 5.1 Import graph between top-level packages

```
go list -f '{{.ImportPath}} {{join .Imports " "}}' ./... \
  | awk '{src=$1; for(i=2;i<=NF;i++) if ($i ~ /vimmary\//) print src" -> "$i}' \
  | sed 's|github.com/meltforce/vimmary/||g'
```

25 internal edges across 13 packages:

```
cmd/vimmary      -> internal/cast2md  internal/config  internal/mcp
cmd/vimmary      -> internal/mistral  internal/models  internal/server
cmd/vimmary      -> internal/service  internal/storage internal/summary
cmd/vimmary      -> internal/youtube
internal/feed    -> internal/service  internal/storage
internal/mcp     -> internal/service  internal/storage
internal/server  -> internal/feed     internal/karakeep internal/service internal/storage
internal/service -> internal/cast2md  internal/config   internal/karakeep
internal/service -> internal/models   internal/storage  internal/summary
internal/service -> internal/youtube
```

Edge weight, counted as non-test files importing each target
(`grep -rn "vimmary/internal/<pkg>\"" --include='*.go' . | grep -v _test.go | wc -l`):

```
12  internal/storage      3  internal/youtube      1  internal/server
 6  internal/service      3  internal/cast2md      1  internal/mistral
 4  internal/summary      2  internal/models       1  internal/mcp
 4  internal/karakeep     2  internal/config       1  internal/feed
```

`internal/storage` is imported by 5 of 13 packages; every other package is
imported by at most 4 files.

Frontend graph (`npx madge --json --extensions ts,tsx src`), fan-in:

```
9  api.ts                 4  utils.ts              1  each of the 7 pages,
6  features.ts            2  components/VideoCard     Layout, ThemeToggle,
6  components/SourceBadge                            ErrorBoundary, App.tsx
6  components/LoadingSkeleton
```

Fan-out: `App.tsx` 9, `VideoDetailPage` 5, `VideoListPage` 5, `VideoCard` 4,
`PodcastListPage` 4, `PodcastNewPage` 4, `SettingsPage` 4, `StatsPage` 3,
`Layout` 2, `main.tsx` 2, `SourceBadge` 1, `features.ts` 1.

### 5.2 Cycles

```
npx madge --circular --extensions ts,tsx src
```

```
Processed 19 files (283ms)
✔ No circular dependency found!
```

Go package level: 0, enforced by the compiler. Go file level: not applicable
(no per-file imports in Go).

### 5.3 Layer violations

```
grep -rn "jackc/pgx" --include='*.go' . | grep -v _test.go
grep -rn "pgx\.[A-Za-z]*" --include='*.go' internal/server internal/feed internal/mcp internal/service cmd/
grep -rn "\*storage\.DB" --include='*.go' internal/server internal/mcp internal/feed | grep -v _test.go
grep -rn "store\.[A-Z][A-Za-z]*(" --include='*.go' internal/server internal/feed | grep -v _test.go
```

Results in sections 2.2 and 2.3.

External non-stdlib dependencies per package
(`go list -f '{{join .Imports "\n"}}' ./<pkg>`, filtered to third-party):

```
internal/cast2md   (none)          internal/mistral   (none)
internal/karakeep  (none)          internal/models    (none)
internal/summary   (none)          internal/config    meltkit
internal/service   google/uuid     internal/youtube   horiagug/youtube-transcript-api-go
internal/feed      chi, pgx, goldmark
internal/mcp       uuid, pgx, mcp-go, meltkit
internal/server    chi, uuid, pgx, meltkit
internal/storage   uuid, pgx, meltkit, pgvector-go
cmd/vimmary        mcp-go, meltkit, tailscale.com/tsnet
```

### 5.4 Module sizes

```
find . -path ./web/node_modules -prune -o -name '*.go' -print | xargs wc -l | sort -rn
cd web && find src -name '*.ts' -o -name '*.tsx' | xargs wc -l | sort -rn
```

Go, top 15 (9278 lines total including tests):

```
681  internal/service/podcast_poll_test.go     271  internal/service/process.go
609  internal/storage/videos.go                269  internal/cast2md/client_test.go
566  internal/server/handlers.go               248  internal/karakeep/webhook_test.go
516  internal/storage/podcasts_integration_test.go  230  internal/feed/atom.go
493  internal/service/service.go               220  internal/server/podcast_handlers.go
420  internal/service/podcast.go               218  internal/models/models.go
326  internal/cast2md/client.go                214  internal/service/search_test.go
314  internal/service/podcast_poll.go          201  internal/summary/claude.go
299  cmd/vimmary/main.go
```

TypeScript, all 18 files (4778 lines total):

```
1029  src/pages/SettingsPage.tsx      82  src/components/SourceBadge.tsx
 760  src/pages/VideoListPage.tsx     70  src/components/Layout.tsx
 673  src/pages/VideoDetailPage.tsx   64  src/utils.ts
 492  src/pages/StatsPage.tsx         56  src/App.tsx
 430  src/api.ts                      55  src/components/ThemeToggle.tsx
 341  src/components/VideoCard.tsx    55  src/components/LoadingSkeleton.tsx
 316  src/pages/PodcastListPage.tsx   45  src/components/ErrorBoundary.tsx
 254  src/pages/PodcastNewPage.tsx    30  src/features.ts
                                      25  src/main.tsx
```

Go lines per package, non-test / test / file count:

```
1903 /  961 / 11  internal/service      299 /   0 /  1  cmd/vimmary
1038 /  663 /  7  internal/storage      273 /   0 /  2  internal/mcp
 986 /    0 /  4  internal/server       271 / 248 /  3  internal/karakeep
 532 /  222 /  6  internal/summary      221 /  43 /  5  internal/youtube
 328 /    0 /  2  internal/feed         218 /   0 /  1  internal/models
 326 /  269 /  2  internal/cast2md      200 /   0 /  1  internal/mistral
                                        110 / 151 /  2  internal/config
```

### 5.5 Function lengths over 80 lines

Go, via a `go/ast` script (`scratchpad/astmetrics/main.go`, walking all
non-test `.go` files and measuring `FuncDecl` start-to-end lines):

```
264  cmd/vimmary/main.go:36            main
143  internal/service/process.go:70    *Service.ProcessVideo
135  internal/storage/videos.go:475    *DB.GetStats
133  internal/service/search.go:31     *Service.Search
 82  internal/service/podcast_poll.go:133  *Service.pollSubscription
 82  internal/feed/atom.go:66          BuildFeed
```

Distribution over 225 non-test Go functions: 6 over 80 lines, 9 at 51–80,
27 at 31–50, 183 at 30 or fewer.

TypeScript, via a script using the repo's own `typescript` package
(`scratchpad/tsmetrics.js`):

```
636  cc=71  src/pages/VideoDetailPage.tsx:38   VideoDetailPage
558  cc=74  src/pages/VideoListPage.tsx:203    VideoListPage
461  cc=29  src/pages/StatsPage.tsx:32         StatsPage
345  cc=36  src/pages/SettingsPage.tsx:685     SettingsPage
307  cc=37  src/components/VideoCard.tsx:35    VideoCard
269  cc=28  src/pages/SettingsPage.tsx:369     PodcastFeedRow
240  cc=33  src/pages/PodcastListPage.tsx:77   PodcastListPage
221  cc=18  src/pages/PodcastNewPage.tsx:15    PodcastNewPage
157  cc= 6  src/pages/VideoListPage.tsx:45     EmptyState
118  cc=11  src/pages/SettingsPage.tsx:235     PromptEditor
 86  cc=14  src/pages/SettingsPage.tsx:148     ModelSelector
```

Distribution over 241 functions (including nested and inline): 11 over 80
lines, 3 at 51–80, 14 at 31–50, 213 at 30 or fewer.

No Go class/struct-length measurement is reported: Go has no class construct,
and the per-file and per-package line counts above cover the equivalent.

### 5.6 Cyclomatic complexity

```
go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -over 9 \
  $(find . -name '*.go' -not -name '*_test.go' -not -path './web/*')
```

All 17 Go functions at CC 10 or above:

```
34  main                            cmd/vimmary/main.go:36
23  (*Service).ProcessVideo         internal/service/process.go:70
23  (*Service).Search               internal/service/search.go:31
23  (*DB).GetStats                  internal/storage/videos.go:475
17  (*Client).post                  internal/mistral/client.go:121
15  (*Service).pollSubscription     internal/service/podcast_poll.go:133
14  (*Service).SetPodcastSubscription  internal/service/podcast.go:310
12  (*Client).Transcribe            internal/mistral/client.go:59
12  (*Service).ProcessEpisode       internal/service/podcast.go:144
12  (*Client).FetchTranscript       internal/youtube/transcript.go:11
11  (*ClaudeSummarizer).Summarize   internal/summary/claude.go:38
11  (*MistralSummarizer).Summarize  internal/summary/mistral.go:29
11  (*Service).BackfillFeed         internal/service/podcast_poll.go:267
11  (*Service).ImportKarakeepBookmarks  internal/service/import.go:18
11  (*Registry).fetchMistralModels  internal/models/models.go:153
10  BuildFeed                       internal/feed/atom.go:66
10  (*Client).ListBookmarks         internal/karakeep/client.go:106
```

TypeScript CC at 10 or above, from the same `typescript`-based script (counting
`if`, `?:`, loops, `case`, `catch`, `&&`, `||`, `??` — the same node classes
gocyclo counts, applied to the TS AST):

```
74  VideoListPage      src/pages/VideoListPage.tsx:203
71  VideoDetailPage    src/pages/VideoDetailPage.tsx:38
37  VideoCard          src/components/VideoCard.tsx:35
36  SettingsPage       src/pages/SettingsPage.tsx:685
33  PodcastListPage    src/pages/PodcastListPage.tsx:77
29  StatsPage          src/pages/StatsPage.tsx:32
28  PodcastFeedRow     src/pages/SettingsPage.tsx:369
18  PodcastNewPage     src/pages/PodcastNewPage.tsx:15
14  ModelSelector      src/pages/SettingsPage.tsx:148
11  PromptEditor       src/pages/SettingsPage.tsx:235
```

See the caveat in section 3: these figures include JSX conditional rendering
and are not comparable to the Go figures.

Hook density per component (`grep -c` for each hook), the size signal without
the JSX problem:

```
SettingsPage.tsx      useState=6 useQuery=7 useMutation=9 useEffect=0  total=22
VideoDetailPage.tsx   useState=6 useQuery=3 useMutation=2 useEffect=1  total=12
VideoListPage.tsx     useState=3 useQuery=3 useMutation=3 useEffect=0  total= 9
VideoCard.tsx         useState=1 useQuery=0 useMutation=3 useEffect=0  total= 4
StatsPage.tsx         useState=0 useQuery=2 useMutation=2 useEffect=0  total= 4
PodcastNewPage.tsx    useState=1 useQuery=1 useMutation=1 useEffect=1  total= 4
PodcastListPage.tsx   useState=1 useQuery=2 useMutation=0 useEffect=0  total= 3
```

### 5.7 Change pressure

```
git log --format= --name-only --since='18 months ago' | sort | uniq -c | sort -rn | head -30
```

The repository's full history is 114 commits, 2026-03-09 to 2026-08-06. The
18-month window captures all of it, so these counts are lifetime counts.

```
17  internal/server/server.go        11  web/src/pages/SettingsPage.tsx
16  internal/server/handlers.go      11  CLAUDE.md
15  README.md                        10  web/src/pages/VideoListPage.tsx
15  internal/service/process.go      10  web/src/pages/VideoDetailPage.tsx
15  cmd/vimmary/main.go              10  web/src/components/VideoCard.tsx
14  web/src/api.ts                   10  internal/config/config.go
14  internal/service/service.go      10  Dockerfile
13  go.mod                            8  web/package.json
13  .forgejo/workflows/ci.yml         8  internal/mcp/server.go
12  internal/storage/videos.go        8  docs/index.html
12  go.sum                            7  web/src/pages/StatsPage.tsx
                                      7  internal/summary/claude.go
                                      7  internal/service/search_test.go
                                      7  internal/feed/atom.go
```

### 5.8 Dead code candidates

**Go — one candidate.**

```
go run golang.org/x/tools/cmd/deadcode@latest -test ./...
```

```
internal/karakeep/client.go:42:18: unreachable func: Client.GetBookmark
```

Reported as a candidate. `deadcode` performs reachability analysis from `main`
and the test binaries, so a function reached only via reflection or an
interface it does not know about would be a false positive here.

**TypeScript — measurement failed, no candidates reported.**

```
cd web && npx ts-prune -p tsconfig.json
```

`ts-prune` reported 29 exports in `src/api.ts` as unused. This was verified and
is wrong: `listVideos`, named among them, is imported at
`src/pages/VideoListPage.tsx:5`, `src/pages/PodcastListPage.tsx:4` and
`src/pages/StatsPage.tsx:3`. The repo's imports carry the `.ts` extension
(`tsconfig.json` sets `allowImportingTsExtensions: true`), which `ts-prune` does
not resolve. The tool's output is discarded in full rather than filtered, and
no dead-code candidates are reported for the frontend.

### 5.9 Public surface per package

Measured with the `go/ast` script: distinct exported names of each internal
package referenced from **other** packages.

```
storage   9  DB ListFilters NewDB PodcastSubscription SourcePodcast
             SourceYouTube Video VideoMatch VideoStats
             importers: cmd/vimmary, feed, mcp, server, service

cast2md   8  BatchResult Episode Feed ListCompletedOptions New
             OrderUpdatedAsc OrderUpdatedDesc StatusCompleted
             importers: cmd/vimmary, service

service   8  DefaultInitialBackfill ErrPodcastDisabled HybridMatch
             IsEpisodeNotReady New PodcastFeed PodcastSource Service
             importers: cmd/vimmary, feed, mcp, server

summary   6  DefaultPromptFor LangSameAsTranscript NewClaudeSummarizer
             NewMistralSummarizer Request Summarizer
             importers: cmd/vimmary, service

config    4  Cast2MDConfig Load SearchConfig SummaryConfig
feed      3  HandleCombinedFeed HandlePodcastFeed HandleVideoFeed
karakeep  3  ExtractYouTubeID NewClient WebhookHandler
models    3  Model NewRegistry Registry
youtube   3  Client ExtractAudio NewClient
mcp       2  New WithUserID
server    2  New StartHealthListener
mistral   1  NewClient
```

**Measured.** The widest surface is `internal/storage` at 9 distinct names.
Of those, 4 are type or constant names (`Video`, `VideoMatch`, `VideoStats`,
`PodcastSubscription`), 2 are the source constants, 1 is a filter struct, and
2 are `DB` and `NewDB`.

**Conjecture.** No package in this repository shows the pattern the brief
describes as "a module with no interface". The largest package by line count,
`internal/service` at 1903 non-test lines, exposes 8 names; the widest surface,
`storage` at 9, is mostly data types rather than behaviour.

### 5.10 Test coverage

Measured in a second run on 2026-08-06, after sections 5.1–5.9. The storage
tests require Postgres on `127.0.0.1:5434` and do not skip locally, so the
database was started first and stopped afterwards:

```
docker compose up -d db
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out
docker compose stop db
```

Statement coverage per package:

```
internal/config    90.9%     internal/karakeep  29.5%
internal/cast2md   82.9%     internal/youtube    3.9%
internal/summary   38.6%     cmd/vimmary         0.0%
internal/storage   37.3%     internal/feed       0.0%
internal/service   13.4%     internal/mcp        0.0%
                             internal/mistral    0.0%
                             internal/models     0.0%
                             internal/server     0.0%
```

Functions at 0.0%, as a share of each package's functions:

```
server   40 of 40      storage   24 of 52
service  51 of 64      summary    5 of 13
feed      9 of  9      karakeep   5 of  7
mcp       7 of  7      youtube    3 of  4
models    6 of  6      cast2md    1 of 15
mistral   6 of  6      config     0 of  1
```

Coverage of the 17 functions at CC ≥ 10 (section 5.6), in the same order:

```
 0.0%  main                            CC 34   cmd/vimmary/main.go:36
 0.0%  (*Service).ProcessVideo         CC 23   internal/service/process.go:70
 0.0%  (*Service).Search               CC 23   internal/service/search.go:31
 0.0%  (*DB).GetStats                  CC 23   internal/storage/videos.go:475
 0.0%  (*Client).post                  CC 17   internal/mistral/client.go:121
88.9%  (*Service).pollSubscription     CC 15   internal/service/podcast_poll.go:133
61.5%  (*Service).SetPodcastSubscription CC 14 internal/service/podcast.go:310
 0.0%  (*Client).Transcribe            CC 12   internal/mistral/client.go:59
 0.0%  (*Service).ProcessEpisode       CC 12   internal/service/podcast.go:144
 0.0%  (*Client).FetchTranscript       CC 12   internal/youtube/transcript.go:11
 0.0%  (*ClaudeSummarizer).Summarize   CC 11   internal/summary/claude.go:38
 0.0%  (*MistralSummarizer).Summarize  CC 11   internal/summary/mistral.go:29
 0.0%  (*Service).BackfillFeed         CC 11   internal/service/podcast_poll.go:267
 0.0%  (*Service).ImportKarakeepBookmarks CC 11 internal/service/import.go:18
 0.0%  (*Registry).fetchMistralModels  CC 11   internal/models/models.go:153
 0.0%  BuildFeed                       CC 10   internal/feed/atom.go:66
 0.0%  (*Client).ListBookmarks         CC 10   internal/karakeep/client.go:106
```

The 13 covered functions in `internal/service`, which is the full set:

```
100.0%  ProcessEpisodeAsync      76.5%  EnsureEpisodeRow
100.0%  queueEpisode             75.0%  TranscribeAllInFeed
100.0%  stripMarkdown            66.7%  maxPerPoll
100.0%  New                      61.5%  SetPodcastSubscription
 88.9%  pollSubscription         50.0%  enqueueEpisode
                                 33.3%  parsePublished
                                 11.1%  processWorker
                                  7.1%  episodeWorker
```

Ten of the thirteen belong to the podcast path. The YouTube path — `ProcessVideo`,
`Search`, `TranscribeVideo`, `TranscribeAllNoCaptions`, `ImportKarakeepBookmarks`,
`getSummarizer` — is at 0.0% throughout.

**No frontend coverage is reported.** `web/package.json` defines three scripts
(`dev`, `build`, `preview`) and carries no test runner or testing library in its
dependencies, so there is nothing to measure.

---

## Reproducing this report

The two ad-hoc scripts are not committed. They were written to the session
scratchpad and are described in full above (section 5.5): a `go/ast` walker
measuring `FuncDecl` line spans and cross-package selector references, and a
`typescript`-based equivalent for `.ts`/`.tsx`. Every other measurement is a
single shell command, quoted at the point it is used.
