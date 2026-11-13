#!/usr/bin/env bash
# Test for the failure path of E2E_NATIVE=1 scripts/e2e. Runs in the lint
# check. A config the app cannot parse makes it exit during startup; the run
# must fail, print the app's log and leave no database container behind.
# Without Docker it is skipped, except in CI.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 1
# shellcheck source=scripts/lib/common.sh
source scripts/lib/common.sh

if ! docker info >/dev/null 2>&1; then
  if is_ci; then
    echo "FAIL e2e-native: Docker is not reachable"
    exit 1
  fi
  echo "skip e2e-native: Docker is not reachable"
  exit 0
fi

dir=$(mktemp -d)
trap 'rm -r -- "$dir"' EXIT
sed 's/^seed_fixtures: true$/seed_fixtures: not-a-bool/' e2e/vimmary.yaml >"$dir/bad.yaml"

out=$(E2E_NATIVE=1 E2E_CONFIG="$dir/bad.yaml" scripts/e2e 2>&1)
rc=$?
fail=0
check() { # check <name> <command...>
  local name=$1; shift
  if "$@"; then echo "ok   e2e-native: $name"; else echo "FAIL e2e-native: $name"; fail=1; fi
}
left=$(docker ps -aq --filter name=vimmary-e2e-native-)
check "a broken config fails the run (exit $rc)" test "$rc" -ne 0
check "the run names the early exit" grep -q 'vimmary exited during startup' <<<"$out"
check "the app log is printed" grep -q 'msg="startup failed"' <<<"$out"
check "no database container remains" test -z "$left"
if [[ $fail -ne 0 ]]; then
  tail -n 30 <<<"$out"
fi
exit "$fail"
