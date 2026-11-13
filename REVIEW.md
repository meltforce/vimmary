# Review rules

The review workflow reads this file from `main`. Nobody else reads the code
before it reaches production, so the verdict is the last judgement on it.

## Verdict

`request_changes` when any of these holds; otherwise `approve`.

1. **An acceptance criterion has no test**, or its test does not exercise
   what the criterion states. The PR text lists the criteria; a criterion
   without a matching test is a blocker.
2. **The PR text claims test output it does not show**, or the shown output
   does not come from the tests the PR names.
3. **A migration is not additive.** The previous image must start against the
   new schema: no dropped or renamed column or table, no new `NOT NULL`
   column without a default, no changed type the old code cannot read.
4. **A new environment variable, config key, volume or port is needed but
   not named in the PR text.** The production compose file and `config.yaml`
   live in the homelab repo and change only when the PR says so.
5. **`/version` (the deploy check) or the loopback `/healthz` (127.0.0.1:8081,
   the image's HEALTHCHECK) stops reporting the build SHA**, or reports healthy while the database is unreachable, or
   startup gains a network call before the HTTP listener other than tsnet
   (`run()` in `cmd/vimmary/main.go`).
6. **A check is weakened**: a test deleted or skipped without a replacement,
   anything under `scripts/` that a check runs, the hooks or deny rules in
   `.claude/settings.json`, or anything under `.github/` loosened.
7. **A secret, token or password is committed**, or logged.
8. **The change is a correctness bug you can name**: a code path that returns
   a wrong result, leaks a resource, or ignores an error the caller depends on.

Style, naming and structure preferences are not blockers. Mention them in the
summary if they matter; do not request changes for them.

## Dependency updates

A PR opened by `renovate[bot]`, whose commits are all by `renovate[bot]` and
whose diff touches only `go.mod`, `go.sum`, `web/package.json`,
`web/package-lock.json`, `Dockerfile`, `docker-compose.yml`, `.node-version`,
`compose*.y*ml` at the root, or `uses:`, `version:` and `image:` pins in
`.github/workflows/*.yml`, passes the `review` check without a Claude run
(`scripts/renovate-only.sh`, from `main`). `test`, `lint` and `e2e` still
gate it. Any other file in the diff, or a commit by anyone else, brings the
normal review.

## Summary

The owner reads the summary, not the diff. Write it in plain language:

- what the PR changes, in one or two sentences, as the user of the app sees it
- the verdict and, for `request_changes`, each blocker with file and line and
  what would resolve it
- anything the owner has to do outside this repo (a new variable or config key
  in the homelab repo, a manual database step)
