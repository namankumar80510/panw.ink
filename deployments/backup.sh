#!/usr/bin/env bash
# Coordinated Pawn.Ink backup (§14.2): application DB + Collar system DB in
# one consistent snapshot, plus a Redis AOF/RDB copy for pending analytics.
# Read-only against the databases; writes only under BACKUP_DIR.
#
# Required env: BACKUP_DIR, MYSQL_DEFAULTS_FILE (a 0600 option file for a
# read-only backup account). Optional: APP_DATABASE, COLLAR_SYSTEM_DATABASE,
# REDIS_CLI ("redis-cli -p 6379"), REDIS_DATA_DIR, BACKUP_GPG_RECIPIENT.
set -euo pipefail

: "${BACKUP_DIR:?set BACKUP_DIR}"
: "${MYSQL_DEFAULTS_FILE:?set MYSQL_DEFAULTS_FILE (never pass passwords on the command line)}"
app_db="${APP_DATABASE:-pawn_ink}"
sys_db="${COLLAR_SYSTEM_DATABASE:-pawn_ink_collar}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dest="${BACKUP_DIR%/}/pawn-ink-${stamp}"

mkdir -p -- "$dest"
chmod 700 -- "$dest"

echo "dumping ${app_db} + ${sys_db} (single transaction)"
mariadb-dump --defaults-extra-file="$MYSQL_DEFAULTS_FILE" --single-transaction --routines --triggers \
  --hex-blob --default-character-set=utf8mb4 --databases "$app_db" "$sys_db" | gzip -9 > "$dest/databases.sql.gz"

if [[ -n "${REDIS_CLI:-}" && -n "${REDIS_DATA_DIR:-}" ]]; then
  echo "snapshotting Redis (BGSAVE) and copying AOF/RDB"
  before="$($REDIS_CLI LASTSAVE)"
  $REDIS_CLI BGSAVE >/dev/null
  for _ in $(seq 1 120); do [[ "$($REDIS_CLI LASTSAVE)" != "$before" ]] && break; sleep 1; done
  tar -C "$REDIS_DATA_DIR" -czf "$dest/redis.tar.gz" .
fi

( cd "$dest" && sha256sum -- * > SHA256SUMS )
if [[ -n "${BACKUP_GPG_RECIPIENT:-}" ]]; then
  for f in "$dest"/*.gz; do gpg --batch --yes -r "$BACKUP_GPG_RECIPIENT" -e -- "$f" && rm -f -- "$f"; done
fi
echo "backup written to $dest"
echo "Asset objects and the TOKEN_ENCRYPTION_KEY are backed up separately (docs/RUNBOOK.md)."
