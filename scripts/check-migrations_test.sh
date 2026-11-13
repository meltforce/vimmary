#!/usr/bin/env bash
# Test for scripts/check-migrations.sh. Runs in scripts/lint. Each case builds
# a throwaway repository whose main holds one migration pair, applies a change
# on a branch and expects a verdict.
set -uo pipefail
src="$(cd "$(dirname "$0")" && pwd)/check-migrations.sh"

tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
fail=0
n=0

git_() { git -c user.name=t -c user.email=t@t -c init.defaultBranch=main "$@"; }

# case <name> <want exit> <shell commands run on the branch>
case_() {
  local name=$1 want=$2 change=$3 repo rc
  n=$((n + 1))
  repo="$tmp/$n"
  mkdir -p "$repo/scripts" "$repo/migrations"
  cp "$src" "$repo/scripts/"
  (
    cd "$repo" || exit 1
    git_ init -q
    echo "CREATE TABLE t (id int);" >migrations/000001_init.up.sql
    echo "DROP TABLE t;" >migrations/000001_init.down.sql
    git_ add -A && git_ commit -qm base
    git_ checkout -qb change
    eval "$change"
    git_ add -A && git_ commit -qm change --allow-empty
  )
  (cd "$repo" && scripts/check-migrations.sh main >/dev/null 2>&1)
  rc=$?
  if [[ $rc -eq $want ]]; then
    echo "ok   $name"
  else
    echo "FAIL $name: exit $rc, want $want"
    fail=1
  fi
}

case_ "no change"                       0 ':'
case_ "additive pair"                   0 'echo "ALTER TABLE t ADD COLUMN c int;" >migrations/000002_c.up.sql; echo "ALTER TABLE t DROP COLUMN c;" >migrations/000002_c.down.sql'
case_ "DROP in a new up"                1 'echo "ALTER TABLE t DROP COLUMN id;" >migrations/000002_c.up.sql; echo "SELECT 1;" >migrations/000002_c.down.sql'
case_ "DELETE in a new up"              1 'echo "DELETE FROM t;" >migrations/000002_c.up.sql; echo "SELECT 1;" >migrations/000002_c.down.sql'
case_ "TRUNCATE in a new up"            1 'echo "TRUNCATE t;" >migrations/000002_c.up.sql; echo "SELECT 1;" >migrations/000002_c.down.sql'
case_ "DROP in a comment only"          0 'echo "-- no DROP here" >migrations/000002_c.up.sql; echo "SELECT 1;" >>migrations/000002_c.up.sql; echo "SELECT 1;" >migrations/000002_c.down.sql'
case_ "up without down"                 1 'echo "SELECT 1;" >migrations/000002_c.up.sql'
case_ "down without up"                 1 'echo "SELECT 1;" >migrations/000002_c.down.sql'
case_ "existing up edited"              1 'echo "-- edit" >>migrations/000001_init.up.sql'
case_ "existing down edited"            1 'echo "-- edit" >>migrations/000001_init.down.sql'
case_ "existing pair removed"           1 'rm migrations/000001_init.*'
case_ "new version not above base"      1 'echo "SELECT 1;" >migrations/000000_early.up.sql; echo "SELECT 1;" >migrations/000000_early.down.sql'
case_ "duplicate version"               1 'echo "SELECT 1;" >migrations/000001_other.up.sql; echo "SELECT 1;" >migrations/000001_other.down.sql'
case_ "misnamed file"                   1 'echo "SELECT 1;" >migrations/2_c.sql'

exit $fail
