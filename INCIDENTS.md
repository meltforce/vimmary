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
