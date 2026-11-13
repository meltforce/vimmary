#!/usr/bin/env bash
# Test for scripts/resolve-sha.sh. Runs in the lint check. Builds a throwaway
# repository with two commits on main and one on a side branch.
set -uo pipefail
script="$(cd "$(dirname "$0")" && pwd)/resolve-sha.sh"

repo=$(mktemp -d)
trap 'rm -r -- "$repo"' EXIT
cd "$repo"
git init -q -b main
git -c user.name=t -c user.email=t@t commit -q --allow-empty -m one
first=$(git rev-parse HEAD)
git -c user.name=t -c user.email=t@t commit -q --allow-empty -m two
head=$(git rev-parse HEAD)
git checkout -q -b side
git -c user.name=t -c user.email=t@t commit -q --allow-empty -m side
side=$(git rev-parse HEAD)
git checkout -q main
missing=0123456789abcdef0123456789abcdef01234567
fail=0

expect() { # expect <name> <want exit> <want stdout> <args...>
  local name=$1 want_rc=$2 want_out=$3; shift 3
  local out rc
  out=$("$script" "$@" 2>/dev/null); rc=$?
  if [[ $rc -eq $want_rc && "$out" == "$want_out" ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: exit $rc (want $want_rc), output '$out' (want '$want_out')"
    fail=1
  fi
}

expect "push: GITHUB_SHA when no input"        0 "$head"  ""        "$head" main
expect "dispatch: input wins over GITHUB_SHA"  0 "$first" "$first"  "$head" main
expect "dispatch: commit on a side branch"     1 ""       "$side"   "$head" main
expect "dispatch: hash of no commit"           1 ""       "$missing" "$head" main
expect "short hash rejected"                   1 ""       "${first:0:7}" "$head" main
expect "uppercase hash rejected"               1 ""       "$(tr a-f A-F <<<"$first")" "$head" main
expect "41 characters rejected"                1 ""       "${first}0" "$head" main
expect "non-hex rejected"                      1 ""       "zzzz${first:4}" "$head" main
expect "both empty rejected"                   1 ""       ""        ""      main
expect "injection attempt rejected"            1 ""       "$first; rm -rf /" "$head" main

exit $fail
