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

## 2026-08-07 — the same race, for 6h23min, with the detection removed the evening before

**Effect.** vimmary refused connections on 100.69.223.64:443 from 22:27:29 on
2026-08-06 to 04:50:29 on 2026-08-07 CEST — 6 hours 23 minutes. Uptime Kuma
recorded three earlier flaps between 19:51 and 20:16, which are the 2026-08-06
incident below. Only vimmary was affected; the other 35 monitors stayed up, and
the tsnet node answered ICMP throughout.

**Cause — the 2026-08-06 remedy does not prevent the race.** The start at
22:26:19 ran `edge-c10511e`, which contains `95c5b01` and therefore
`tsServer.Up()`. `Up()` reported success:

```
2026/08/06 20:26:19 AuthLoop: state is Running; done
level=INFO msg="tsnet up" state=Running ips="[100.69.223.64 fd7a:115c:a1e0::cf34:df40]"
```

It returned 15 ms after `Start()`, and every setec request that followed was
answered `access denied`.

`Up()` waits for `BackendState == Running` with an address. When tsnet loads its
persisted state the AuthLoop short-circuits — `state is Running; done` — and
that condition is already true before the node has a current netmap. The
condition being waited on is the condition that holds in the failure case.

**The discriminating signal is the AuthLoop line, not the backend state.**
Across the 15 starts between 2026-08-06 18:32 and 2026-08-07 04:49:

| AuthLoop | starts | listener opened |
|---|---|---|
| `Starting; done` | 11 | 11 |
| `Running; done` | 4 | 0 |

The separation is complete over all 15. The loss rate is 4 in 15. setec logged
nothing: the unit has run unchanged since 2026-08-01 and wrote no entry in the
22:20–22:35 window, so the rejection is not confirmable from that side.

**Why it lasted 6h23min rather than one restart.** Three things, in order of
how much each cost.

`resolver.InitSetecStore` was called with `context.Background()`.
`setec.NewStore` retries for as long as its context lives, so a start that got
`access denied` retried until the process was killed. The process never reached
`StartHealthListener`, and `restart: unless-stopped` does not act on a process
that has not exited.

The container had no healthcheck. `docker inspect` reported `Health=null`
throughout. The probe existed for 94 seconds the previous evening: homelab
commit `a7bc4cb` added it at 20:08:53, the container recreated at 20:09:22 lost
the race and was correctly reported unhealthy, and the auto-redeploy read that
unhealthy container as a fault of the commit that introduced the check and
reverted it as `0bc1091` at 20:10:27. The one control that would have caught
this outage was removed by an automation reacting to the failure mode the
control was built to detect, 2 hours 16 minutes before the outage began.

CI did detect it. The `health_url` step failed after 150 s and
`notify-deploy-failure` published to `https://ntfy.coydog-fence.ts.net/claude`
at 22:29:30 — three minutes after the outage began. That topic also carries
routine traffic ("Ansible deploy needed", "converge: 0 errors").

**How it ended.** Not on its own. The homelab `converge` sweep
(`cron: '47 2 * * *'` UTC) recreated the container at 04:49:37; `docker inspect`
shows `Created` and `StartedAt` both at 02:49:37 UTC with `RestartCount=0`. That
start drew `Starting; done` and won. First 200 at 04:50:29.

**What this corrects in the 2026-08-06 entry below.** Two statements there are
wrong. "It removes the case where the request provably went out too early" —
it does not; the 22:26:19 start went out after `Up()` returned success. "vimmary
now opens a loopback health endpoint … and the compose file probes it with a
120 s `start_period`" — the endpoint was added and works, the compose probe was
reverted 94 seconds after it landed and never reached a second deploy.

**The fix.**

- `cmd/vimmary/main.go` bounds the setec store init at 30 s and exits non-zero
  on expiry, against a resolution that takes under 100 ms when the race is won.
  Recovery moves to `restart: unless-stopped`, and each restart is a fresh draw
  on a race that 11 of 15 starts won.
- The `HEALTHCHECK` moves into the `Dockerfile`, where no deployment-side revert
  reaches it. It probes `127.0.0.1:8081/healthz`, the default of `health_addr`.
  A deployment that moves `health_addr` has to override the `HEALTHCHECK` with
  it.

**Residual risk.** The race itself is untouched — the remedy shortens the
outage, it does not prevent the lost start. Every deploy still has roughly a 1
in 4 chance of a start that exits and restarts, which will show as a failed CI
health step even where the restart recovers within minutes. Removing the race
needs a readiness condition that reflects the netmap rather than the backend
state, or a setec store that distinguishes "denied because the peer is not yet
known" from "denied".

**Closed the same day, by removing the dependency rather than the race.** The
retries told the rest of the story: 16812 of them, evenly spaced over the whole
6h23min, all answered the same way. A window that a longer wait closes would
have closed. Reading setec's source settled what the answer meant — a failed
identity lookup returns 500 `unable to identify caller`, and we got 403, so
setec identified the node and found no permissions in the `CapMap` its own
netmap carried for that peer. The stale state was on setec's side, out of reach
of anything vimmary could wait for or retry.

vimmary therefore no longer fetches secrets at startup. The database password
comes from `VIMMARY_POSTGRES_PASSWORD`, the LLM API keys live in `app_settings`
and are read when used, and no setec client is linked into the binary. The
failure class is gone, not shortened. The tsnet race still exists and still
belongs to `ROADMAP.md`; what changed is that vimmary no longer gives it a way
to stop the process.

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
