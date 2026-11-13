#!/usr/bin/env bash
# Prints the commit deploy.yml deploys: the workflow_dispatch input when given,
# otherwise the pushed commit. The commit must be a full 40-character
# lowercase hex hash and reachable from main; anything else ends with exit 1
# before a build starts.
#
# The main check matters for workflow_dispatch: deploy.yml runs from main and
# so passes the environment `production`, but `sha` can name any commit in
# the repository, including one on a branch that no review has seen.
#
#   scripts/resolve-sha.sh "<input sha or empty>" "<GITHUB_SHA>" [main ref]
#
# The main ref defaults to origin/main; the checkout needs the history
# (fetch-depth: 0).
set -euo pipefail

sha="${1:-}"
[[ -n "$sha" ]] || sha="${2:-}"
main_ref="${3:-origin/main}"

if ! [[ "$sha" =~ ^[0-9a-f]{40}$ ]]; then
  echo "sha must be a full 40-character lowercase commit hash, got '${sha}'" >&2
  exit 1
fi
if ! git cat-file -e "${sha}^{commit}" 2>/dev/null; then
  echo "sha ${sha} is not a commit in this repository" >&2
  exit 1
fi
if ! git merge-base --is-ancestor "$sha" "$main_ref"; then
  echo "sha ${sha} is not on ${main_ref}; only merged commits are deployed" >&2
  exit 1
fi
printf '%s\n' "$sha"
