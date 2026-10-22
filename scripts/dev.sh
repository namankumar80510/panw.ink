#!/usr/bin/env bash
# Development helper. Never touches production services.
#
#   scripts/dev.sh env    # write .env from .env.example with random secrets (refuses to overwrite)
#   scripts/dev.sh run    # Parcel watch + Go API + worker, stopped together with Ctrl-C
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

rand_hex() { openssl rand -hex "$1"; }

cmd_env() {
  if [[ -e .env ]]; then
    echo ".env already exists; refusing to overwrite it." >&2
    exit 1
  fi
  local jwt sess csrf enc rank ana
  jwt="$(rand_hex 32)"; sess="$(rand_hex 32)"; csrf="$(rand_hex 32)"
  enc="$(rand_hex 32)"; rank="$(rand_hex 32)"; ana="$(rand_hex 32)"
  sed \
    -e "s#^COLLAR_DSN=.*#COLLAR_DSN='pawn_app:pawn-app-dev@tcp(127.0.0.1:13306)/'#" \
    -e "s#^COLLAR_ADMIN_DSN=.*#COLLAR_ADMIN_DSN='pawn_migrator:pawn-migrator-dev@tcp(127.0.0.1:13306)/'#" \
    -e "s#^COLLAR_JWT_SECRET=.*#COLLAR_JWT_SECRET=${jwt}#" \
    -e "s#^SESSION_HMAC_KEY=.*#SESSION_HMAC_KEY=${sess}#" \
    -e "s#^CSRF_HMAC_KEY=.*#CSRF_HMAC_KEY=${csrf}#" \
    -e "s#^TOKEN_ENCRYPTION_KEY=.*#TOKEN_ENCRYPTION_KEY=${enc}#" \
    -e "s#^RANKING_HMAC_KEY=.*#RANKING_HMAC_KEY=${rank}#" \
    -e "s#^ANALYTICS_HMAC_KEY=.*#ANALYTICS_HMAC_KEY=${ana}#" \
    -e "s#^S3_ACCESS_KEY=.*#S3_ACCESS_KEY=pawnminio#" \
    -e "s#^S3_SECRET_KEY=.*#S3_SECRET_KEY=pawnminio-dev-secret#" \
    -e "s#^WEB_DIST_DIR=.*#WEB_DIST_DIR=./web/dist#" \
    .env.example > .env
  chmod 600 .env
  echo "wrote .env (mode 600) with fresh development secrets"
}

cmd_run() {
  [[ -e .env ]] || { echo "run 'scripts/dev.sh env' first" >&2; exit 1; }
  local pids=()
  cleanup() { kill "${pids[@]}" 2>/dev/null || true; wait 2>/dev/null || true; }
  trap cleanup EXIT INT TERM
  (cd web && npm run watch) & pids+=($!)
  go run ./cmd/server & pids+=($!)
  go run ./cmd/worker & pids+=($!)
  wait -n
}

case "${1:-}" in
  env) cmd_env ;;
  run) cmd_run ;;
  *) echo "usage: $0 env|run" >&2; exit 2 ;;
esac
