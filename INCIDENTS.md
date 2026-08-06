# Incidents

Postmortems for things that broke in vimmary. Newest first.

An entry belongs here when the cause was not obvious from the symptom, so that
the next occurrence is recognised rather than re-derived. A change that worked as
intended is not an incident — that is a commit message, and a decision with
reasoning belongs in [`DECISIONS.md`](DECISIONS.md).

Incidents whose root cause lies in the homelab infrastructure are recorded in
`homelab/INCIDENTS.md`, which covers every affected repo in one place. An entry
here carries the vimmary-specific effect and enough of the cause to be readable
on its own, plus a pointer — not a copy.

---

## 2026-08-06 — the container ran, the service did not, and nothing said so

**Effect.** After the CI deploy of `69f7bfd` at 17:50 UTC, vimmary answered
nothing: no web UI, no API, no Atom feed. It was noticed because an RSS reader
stopped receiving podcast entries — not by any monitoring. The outage lasted
about nine minutes and ended with a manual `docker restart`.

**Symptom.** `docker ps` showed the container up. dockhand showed the stack
green. The tsnet node answered `tailscale ping`. But port 443 on that node was
closed, and the container log held 294 consecutive lines of

```
[store] error fetching "vimmary/postgres-password": access denied (retrying)
```

and nothing else since startup.

**Cause.** The init order in `cmd/vimmary/main.go` is config → tsnet → setec →
resolve secrets → migrations → DB → services → HTTP listener. tsnet reported
`state is Running` in the same second it started, before the control plane had
finished with it, and the setec store's first request went out from an identity
setec did not accept. The store retried on its own schedule and kept getting the
same answer; it did not recover on its own. The process therefore never reached
the listener.

The restart was enough. On the second start the same race produced
`tsnet: backend in state NoState` instead, the retry succeeded a moment later,
and the service came up normally. The difference between the two failure modes
is what tsnet had already told the control plane when the first request left.

**Why nothing caught it.** Two gaps, both now closed.

The container had no healthcheck, so Docker had no opinion and dockhand showed
container state — which was, correctly, "running". A healthcheck was not
straightforward to add: with Tailscale enabled the listener lives on the tsnet
netstack, which nothing inside the container can dial, so a probe against
localhost would fail on a healthy service. vimmary now opens a loopback health
endpoint (`health_addr`, default `127.0.0.1:8081`) as the last step of startup,
and the compose file probes it with a 120 s `start_period`. The listener
existing is the signal; a start still resolving secrets answers nothing.

CI reported the deploy as successful, because the `deploy` job checks that
`docker compose up -d` returned, not that the service serves. A `deploy-gate`
job now polls `/version` for two minutes and requires both the commit's own
build string and a reachable database. That endpoint is new: the Dockerfile did
not link `VERSION` into the binary, so every build reported `dev` and no
downstream check could tell one deploy from another.

**Not the cause, though it looked like one.** All four secrets existed in setec
and were readable from a workstation; setec itself was up and answered; the
tsnet node kept its identity across the restart, with `tag:vimmary` and no key
expiry. The persistent `access denied` invites a hunt for an ACL change, and
there was none.

**Residual risk.** The remedy detects the condition, it does not prevent it. If
the setec store's retry loop wedges again, the healthcheck turns the container
unhealthy and the deploy gate fails the pipeline, but recovery is still a
restart. A `restart: unless-stopped` policy does not act on health. Making the
container restart itself on a failed healthcheck, or giving the resolver a
bounded retry that exits non-zero, would close that — neither is in place.

---

## 2026-08-01 — CI reported success while building and deploying nothing since June

**Effect on vimmary.** The `:edge` image was frozen at the build of
**2026-06-13**. Every commit to `main` for seven weeks reported a green pipeline
and changed nothing on the deploy target. The container kept running the June
image.

**Root cause.** The `build-deploy` job in `.forgejo/workflows/ci.yml` invokes a
reusable workflow with `uses:` and carried
`if: github.event_name == 'push' && github.ref == 'refs/heads/main'`. A condition
on a `uses:` job is evaluated in the **called** workflow, where the event is
named `workflow_call` — the comparison was therefore structurally always false.
Both child jobs were skipped, and a job whose children are all `skipped` counts
as `success`.

The change is datable to early June 2026 and is presumed to be a Forgejo update
that altered `if:` evaluation for `workflow_call`. cast2md and nutrak were
affected the same way; homelib was not, because it gates via
`on: push: branches: [main]` and carries no `if:` at all.

**Fix.** `e56f9ef` — the `github.event_name` comparison is removed.
`github.ref == 'refs/heads/main'` alone gates correctly, because on pull requests
the ref is `refs/pull/N/merge`. The condition now carries a comment saying why
the event comparison must not come back.

**What this means for reading CI here.** A green pipeline is not evidence that a
deploy happened. The evidence is the artifact: the image date in the registry,
or the age of the running container.

**Full postmortem** — including the detection path and the lessons that apply
across all four app repos — is in `homelab/INCIDENTS.md` under
*2026-08-01 (evening) — App repo CI built and deployed nothing since June, with a
green pipeline*.
