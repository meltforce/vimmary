#!/usr/bin/env bash
# Decides whether a pull request's diff touches dependency pins only. Reads
# the JSON array of GET /repos/{repo}/pulls/{pr}/files on stdin; exit 0 means
# dependency-only, exit 1 names the first file that is not.
#
# Dependency files: go.mod, go.sum, web/package.json, web/package-lock.json,
# Dockerfile, .node-version, compose*.yml / compose*.yaml at the root, and
# .github/workflows/*.yml where every changed line is a `uses: …@…`, a
# `version:` or an `image: …:<tag>` pin. A file GitHub sends without a patch
# (too large, binary) counts as other.
#
# claude-review.yml runs this script from the base branch, after checking
# that the PR author and every commit author is renovate[bot].
set -euo pipefail

files=$(cat)
count=$(jq 'length' <<<"$files")
[[ "$count" -gt 0 ]] || { echo "no files"; exit 1; }

pin_line='^[+-][[:space:]]*(-[[:space:]]+)?(uses:[[:space:]]*[^[:space:]]+@[^[:space:]]+([[:space:]]+#.*)?|(go-|node-)?version:[[:space:]]*[^[:space:]]+|image:[[:space:]]*[^[:space:]]+:[^[:space:]]+)[[:space:]]*$'

for i in $(seq 0 $((count - 1))); do
  name=$(jq -r ".[$i].filename" <<<"$files")
  status=$(jq -r ".[$i].status" <<<"$files")
  if [[ "$status" != modified ]]; then
    echo "$name is $status, not modified"
    exit 1
  fi
  case "$name" in
    go.mod | go.sum | web/package.json | web/package-lock.json | Dockerfile | .node-version) continue ;;
    compose*.yml | compose*.yaml)
      [[ "$name" != */* ]] && continue
      ;;
    .github/workflows/*.yml)
      if [[ "${name#.github/workflows/}" != */* ]] && jq -e ".[$i] | has(\"patch\")" >/dev/null <<<"$files"; then
        changed=$(jq -r ".[$i].patch" <<<"$files" | grep -E '^[+-]' | grep -vE '^(\+\+\+|---)( |$)' || true)
        if [[ -n "$changed" ]] && ! grep -vqE "$pin_line" <<<"$changed"; then
          continue
        fi
      fi
      ;;
  esac
  echo "$name is not a dependency pin"
  exit 1
done
echo "dependency files only ($count)"
