#!/usr/bin/env bash
# Stop hook: runs scripts/test before a Claude session ends. Exit 2 on
# failure, because only exit 2 sends Claude back to work; the output goes to
# stderr, which Claude reads.
#
# It blocks once per stop. When Claude stops again while the hook is already
# active (stop_hook_active in the hook input) and the tests still fail, the
# hook lets the session end: a session that cannot or must not fix the
# failure otherwise loops on every stop. CI reports the failure either way.
#
# A clean working tree whose HEAD is on its upstream has nothing new to test,
# so the hook returns at once there.
set -uo pipefail
cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/..}" || exit 0

input=""
if [[ ! -t 0 ]]; then
  input=$(cat)
fi
retry=$(jq -r '.stop_hook_active // false' <<<"${input:-{\}}" 2>/dev/null || echo false)

if [[ -z "$(git status --porcelain 2>/dev/null)" ]] &&
  upstream=$(git rev-parse '@{upstream}' 2>/dev/null) &&
  [[ "$upstream" == "$(git rev-parse HEAD)" ]]; then
  exit 0
fi

if ! command -v go >/dev/null 2>&1; then
  echo "verify.sh: go not installed, skipping (CI still runs the checks)" >&2
  exit 0
fi

if ! out=$(scripts/test 2>&1); then
  printf 'scripts/test failed:\n%s\n' "$(tail -n 80 <<<"$out")" >&2
  if [[ "$retry" == "true" ]]; then
    echo "verify.sh: still failing after one round; not blocking again. CI will report it." >&2
    exit 0
  fi
  exit 2
fi
exit 0
