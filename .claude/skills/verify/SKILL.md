---
name: verify
description: Run vimmary's checks before committing — scripts/lint (gofmt, vet, golangci-lint, govulncheck, migration rules, script tests, frontend build) and scripts/test (race tests against Postgres + pgvector, frontend tests), scripts/e2e for startup, config, migration or Dockerfile changes. Triggers — "verifizier das", "läuft der build", "prüf ob das durchgeht", "run the checks", "verify before commit", "does this still build". Also reach for it after changing anything under internal/, web/ or migrations/. Not for running the app — that is the run skill.
---

# Verify

The checks are the entry scripts CI runs; run them from the repo root.

```bash
scripts/setup   # toolchain and npm dependencies, if the SessionStart hook has not run
scripts/lint
scripts/test
scripts/e2e     # for a change to startup, config, migrations or the Dockerfile
```

- `scripts/test` starts `compose.test.yaml` (pgvector) when
  `TEST_DATABASE_URL` is unset and removes it afterwards; it needs Docker.
- `scripts/lint` compares new migrations with `origin/main`
  (`LINT_BASE_REF` overrides it), so fetch first.
- `scripts/e2e` builds two images (the working tree and `origin/main`) and
  runs the upgrade test; `E2E_UPGRADE=0` skips the image of `main`, and
  `E2E_BASE_URL=<url>` runs only the HTTP checks against a running instance
  started with `seed_fixtures: true`.

## What this does not cover

- Transcript fetching hits YouTube's InnerTube API and summarising hits a paid
  provider; neither runs in the checks. A change to `internal/youtube/` or
  `internal/summary/` needs a real video run through the running app.
- The MCP surface has no test. Tool handlers are thin wrappers over
  `internal/service/`, which is where the coverage sits.
- A layout change is verified by looking at it, on the preview or locally.
