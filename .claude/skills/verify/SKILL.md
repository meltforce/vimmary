---
name: verify
description: Run vimmary's full verification chain before committing — Go build, vet, race tests, golangci-lint, govulncheck, and the frontend build. Triggers — "verifizier das", "läuft der build", "prüf ob das durchgeht", "run the checks", "verify before commit", "does this still build". Also reach for it after changing anything under internal/, web/ or migrations/. Not for running the app — that is the run skill.
---

# Verify

The chain CI runs, in the order that fails fastest. Run it from the repo root
before committing.

## 1. The embed stub comes first

`web.go` carries `//go:embed all:web/dist`, and `.gitignore` excludes that
directory. **Every Go command below fails with an embed error on a fresh
checkout** until `web/dist` exists — this is not a broken build, it is a missing
directory.

Either build the frontend for real (step 5, needed anyway if you touched `web/`),
or create the stub CI uses:

```bash
mkdir -p web/dist && touch web/dist/.gitkeep
```

## 2. Build and vet

```bash
CGO_ENABLED=0 go build -o /tmp/vimmary ./cmd/vimmary
go vet ./...
```

`CGO_ENABLED=0` matches the Docker image, which is built without a C toolchain.
A build that only passes with cgo enabled will fail in CI.

## 3. Tests

```bash
go test -race ./...
```

**The storage tests need a database and do not skip locally.**
`internal/storage/videos_integration_test.go` connects to
`postgres://vimmary:vimmary@localhost:5434/vimmary`, which is what
`docker compose up db` exposes — note the port is **5434**, not 5432. Start it
first:

```bash
docker compose up -d db
```

Override with `TEST_DATABASE_URL` if the database lives elsewhere. The test skips
only when `CI` is set, which means **CI does not cover the storage layer** — a
regression there is caught locally or not at all.

## 4. Lint and vulnerabilities

```bash
golangci-lint run
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

There is no `.golangci.yml` in this repo, so both CI and a local run use the
default linter set. CI pins `version: latest` for the action, so a golangci-lint
release can introduce findings without anything in this repo changing — when
lint fails on code you did not touch, check the version before chasing the
finding.

`govulncheck` is `continue-on-error` in CI. It reports; it does not gate.

## 5. Frontend

Required when anything under `web/` changed, and it replaces the stub from step 1.

```bash
cd web && npm ci --no-audit --no-fund && npm run build
```

`npm run build` is `tsc -b && vite build`, so a type error fails the build. CI
pins Node 22.23.2.

## What this does not cover

- **Migrations are not verified here.** They run on service startup; a broken
  migration surfaces when the app boots, not in this chain.
- **No end-to-end path.** Transcript fetching hits YouTube's InnerTube API and
  summarising hits a paid provider, so neither is exercised by the tests. A
  change to `internal/youtube/` or `internal/summary/` needs a real video run
  through the running app.
- **The MCP surface has no test.** Tool handlers are thin wrappers over
  `internal/service/`, which is where the coverage sits.
