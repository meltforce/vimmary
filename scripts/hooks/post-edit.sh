#!/usr/bin/env bash
# PostToolUse hook for Edit/Write/MultiEdit: for a changed .go file, gofmt -l
# on the file and go vet on its package. Exit 2 with the findings on stderr
# makes Claude fix them before it goes on.
set -uo pipefail
cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}"

file=$(jq -r '.tool_input.file_path // empty')
[[ "$file" == *.go && -f "$file" ]] || exit 0
command -v go >/dev/null 2>&1 || exit 0

if unformatted=$(gofmt -l "$file") && [[ -n "$unformatted" ]]; then
  echo "gofmt: $file is not formatted; run gofmt -w $file" >&2
  exit 2
fi

# web.go embeds the generated web/dist; vet of the root package needs it.
[[ -d web/dist ]] || { mkdir -p web/dist && touch web/dist/.gitkeep; }

pkg="./$(dirname "${file#"$PWD"/}")"
if ! out=$(go vet "$pkg" 2>&1); then
  printf 'go vet %s failed:\n%s\n' "$pkg" "$out" >&2
  exit 2
fi

exit 0
