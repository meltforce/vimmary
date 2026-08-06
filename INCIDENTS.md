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

**Cause.** `tsnet.Server.Start()` does not wait for the node to come up — it
starts the backend and returns. `cmd/vimmary/main.go` called it and went
straight on to initialise the setec store, which dials over that node. The init
order is config → tsnet → setec → resolve secrets → migrations → DB → services →
HTTP listener, so everything after step two races the tailnet.

Losing the race is not recoverable. tsnet reports `AuthLoop: state is Running`
from persisted state in the same millisecond it starts, so the first setec
request really goes out over the network and reaches setec — which cannot yet
identify the peer and answers a plain `access denied`. The store retries on its
own schedule and keeps getting the same answer. The process never reaches its
listener.

Winning it looks different in the log: `tsnet: backend in state NoState`, a
transport error rather than a rejection, and the retry a moment later succeeds.
Both `docker restart` recoveries took that path.

The race is old; what was new was how often it ran. The container had been up 39
hours before that day and was then recreated five times in three hours — two
deploys through CI, two through Ansible, plus restarts. Two of those five lost.

**The fix.** `tsServer.Up(ctx)` before the setec resolver is initialised, with a
90 s bound. It waits for the node to reach Running with an address, which closes
the window in which anything dials over a node the tailnet does not know yet. It
does not *prove* setec will resolve the identity — it removes the case where the
request provably went out too early.

**Why nothing caught it.** Two gaps, both now closed.

The container had no healthcheck, so Docker had no opinion and dockhand showed
container state — which was, correctly, "running". A healthcheck was not
straightforward to add: with Tailscale enabled the listener lives on the tsnet
netstack, which nothing inside the container can dial, so a probe against
localhost would fail on a healthy service. vimmary now opens a loopback health
endpoint (`health_addr`, default `127.0.0.1:8081`) as the last step of startup,
and the compose file probes it with a 120 s `start_period`. The listener
existing is the signal; a start still resolving secrets answers nothing.

CI reported the deploy as successful, because the `deploy` job checked that
`docker compose up -d` returned and nothing else. The shared workflow it calls
has always had a health step — it is gated on `if: inputs.health_url != ''`,
and neither vimmary nor cast2md ever passed that input, so it never ran in
either repo. Both now do; for vimmary it polls `/version`, whose 503 on an
unreachable database `curl -sfS` turns into a failed deploy. Thirty attempts
every five seconds covers the 120 s `start_period`.

That alone would have caught this outage. A `deploy-gate` job covers the two
cases it cannot see: a deploy that never ran (a skipped `uses:` job counts as
success and skips its health step with it — the 2026-08-01 incident), and a
deploy where `:edge` never moved, where the previous build answers the health
check perfectly. Telling those apart needs the build string, which is why
`/version` reports it — and why the Dockerfile now links `VERSION` into the
binary at all. Until this day every build reported `dev`.

**Not the cause, though each looked like one.** All four secrets existed in
setec and were readable from a workstation; setec itself was up and answered;
the tsnet node kept its identity across the restart, with `tag:vimmary` and no
key expiry. The persistent `access denied` invites a hunt for an ACL change, and
there was none.

Nor was anything on the setec side updated. Checked when the second occurrence
made that the obvious suspect: the setec unit and tailscaled on `tsidp` had both
been running since 2026-08-01, the tsidp container was four days old, and
vimmary's `tailscale.com` was already at v1.102.2, the newest stable release.

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
