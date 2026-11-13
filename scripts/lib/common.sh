# shellcheck shell=bash
# Sourced by scripts/test, scripts/lint and scripts/e2e. Not executable.

cd "$(dirname "${BASH_SOURCE[0]}")/../.." || exit 1

# Docker Desktop on macOS installs its CLI here without adding it to PATH.
if ! command -v docker >/dev/null 2>&1 && [[ -x /Applications/Docker.app/Contents/Resources/bin/docker ]]; then
  PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH"
fi

# web.go embeds web/dist, which is generated; Go does not compile without it.
ensure_web_dist() {
  if [[ ! -d web/dist ]]; then
    mkdir -p web/dist
    touch web/dist/.gitkeep
  fi
}

# npm ci when node_modules is missing or older than the lockfile.
ensure_npm_deps() {
  if [[ ! -f web/node_modules/.package-lock.json || web/package-lock.json -nt web/node_modules/.package-lock.json ]]; then
    (cd web && npm ci --no-audit --no-fund)
  fi
}

is_ci() { [[ "${CI:-}" == "true" ]]; }

step() { printf '\n==> %s\n' "$*"; }

# playwright <args>: the Playwright CLI of e2e/, with browsers in Playwright's
# default cache. A cloud session presets PLAYWRIGHT_BROWSERS_PATH to a browser
# build that need not match e2e/'s Playwright version, and
# PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD, which would keep the matching one out.
playwright() {
  (cd e2e && env -u PLAYWRIGHT_BROWSERS_PATH -u PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD \
    ./node_modules/.bin/playwright "$@")
}

# ensure_playwright: e2e/'s npm dependencies and the Chromium build of its
# Playwright version, with the system libraries it needs on Linux.
ensure_playwright() {
  if [[ ! -f e2e/node_modules/.package-lock.json || e2e/package-lock.json -nt e2e/node_modules/.package-lock.json ]]; then
    (cd e2e && npm ci --no-audit --no-fund --loglevel=error)
  fi
  if [[ "$(uname -s)" == Linux ]]; then
    playwright install --with-deps chromium
  else
    playwright install chromium
  fi
}
