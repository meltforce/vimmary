#!/usr/bin/env bash
# Rejects migrations that destroy data or that golang-migrate cannot apply in
# order. Runs in scripts/lint.
#
# - Every NNNNNN_name.up.sql has a NNNNNN_name.down.sql and the reverse.
# - A new migration's version is above every version on the base, because
#   golang-migrate only applies versions above the database's current one.
# - New *.up.sql files contain no DROP, TRUNCATE or DELETE (this covers
#   ALTER ... DROP). SQL line comments are ignored. *.down.sql files are
#   exempt: undoing an additive migration drops what it added.
# - Existing files under migrations/, up and down, are not changed, renamed
#   or removed: an applied migration never runs again, so an edit would leave
#   production and a fresh database with different schemas.
#
#   scripts/check-migrations.sh [base-ref]     (default: origin/main)
set -euo pipefail
cd "$(dirname "$0")/.."

base="${1:-origin/main}"
merge_base=$(git merge-base "$base" HEAD)
fail=0
err() { echo "migrations: $*" >&2; fail=1; }

while IFS=$'\t' read -r status path rest; do
  [[ -n "$status" ]] || continue
  case "$status" in
    A) ;;
    *) err "${path}${rest:+ -> $rest} is ${status} relative to ${base}; add a new migration instead of changing an existing one" ;;
  esac
done < <(git diff --name-status --no-renames "$merge_base" HEAD -- migrations/)

shopt -s nullglob
for f in migrations/*.sql; do
  name=$(basename "$f")
  if ! [[ "$name" =~ ^[0-9]+_[a-z0-9_]+\.(up|down)\.sql$ ]]; then
    err "${f} is not named <version>_<name>.up.sql or .down.sql"
    continue
  fi
  case "$name" in
    *.up.sql) [[ -f "migrations/${name%.up.sql}.down.sql" ]] || err "${f} has no ${name%.up.sql}.down.sql" ;;
    *.down.sql) [[ -f "migrations/${name%.down.sql}.up.sql" ]] || err "${f} has no ${name%.down.sql}.up.sql" ;;
  esac
done

dups=$(for f in migrations/*.up.sql; do b=$(basename "$f"); echo "${b%%_*}"; done | sort | uniq -d)
[[ -z "$dups" ]] || err "more than one up migration with version ${dups//$'\n'/, }"

base_max=$(git ls-tree --name-only "$merge_base" migrations/ | sed -n 's|^migrations/\([0-9][0-9]*\)_.*\.up\.sql$|\1|p' | sort -n | tail -1)

while read -r path; do
  [[ -n "$path" && -f "$path" ]] || continue
  name=$(basename "$path")
  version=${name%%_*}
  if [[ -n "$base_max" ]] && (( 10#$version <= 10#$base_max )); then
    err "${path} has version ${version}, not above ${base_max} on ${base}; renumber it"
  fi
  [[ "$path" == *.up.sql ]] || continue
  if hits=$(sed 's/--.*$//' "$path" | grep -niwE 'DROP|TRUNCATE|DELETE'); then
    err "${path} contains a destructive statement; migrations must be additive:"
    sed 's/^/  /' <<<"$hits" >&2
  fi
done < <(git diff --name-only --diff-filter=A "$merge_base" HEAD -- 'migrations/*.sql')

if [[ $fail -ne 0 ]]; then
  exit 1
fi
echo "migrations ok"
