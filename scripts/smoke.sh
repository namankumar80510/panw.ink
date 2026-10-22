#!/usr/bin/env bash
# Post-deploy smoke checks (read-only). Usage: SMOKE_URL=https://pawn.ink scripts/smoke.sh
set -euo pipefail

base="${SMOKE_URL:-http://127.0.0.1:14021}"
fail=0

check() { # name expected_status path [grep]
  local name="$1" want="$2" path="$3" pattern="${4:-}" body code
  body="$(mktemp)"
  code="$(curl -sS -o "$body" -w '%{http_code}' "${base}${path}" || echo 000)"
  if [[ "$code" != "$want" ]] || { [[ -n "$pattern" ]] && ! grep -q -- "$pattern" "$body"; }; then
    echo "FAIL $name: $path -> $code"; fail=1
  else
    echo "ok   $name"
  fi
  rm -f "$body"
}

check healthz 200 /healthz '"status":"ok"'
check readyz 200 /readyz '"database":"ok"'
check public-config 200 /api/v1/pawn/x/public-config '"app_name"'
check reader-shell 200 /stories '<div id="app">'
check reader-deep-link 200 /fiction/any-work/ch/any-chapter '<div id="app">'
check studio-shell 200 /studio/works 'dashboard'
check api-json-404 404 /api/v1/pawn/x/does-not-exist '"NOT_FOUND"'
check asset-404 404 /missing-asset.js
check native-login-blocked 404 /api/v1/auth/refresh
check me-anonymous 401 /api/v1/pawn/x/auth/me '"UNAUTHENTICATED"'

exit "$fail"
