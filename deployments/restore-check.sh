#!/usr/bin/env bash
# Restores a backup into NEW, isolated databases and sanity-checks them
# (§17.2 step 3). Refuses to target the live database names.
#
#   MYSQL_DEFAULTS_FILE=... deployments/restore-check.sh /path/to/pawn-ink-<stamp> restorecheck
set -euo pipefail

src="${1:?backup directory}"
suffix="${2:?suffix for the scratch databases, e.g. restorecheck}"
: "${MYSQL_DEFAULTS_FILE:?set MYSQL_DEFAULTS_FILE}"
live_app="${APP_DATABASE:-pawn_ink}"
live_sys="${COLLAR_SYSTEM_DATABASE:-pawn_ink_collar}"
app="${live_app}_${suffix}"
sys="${live_sys}_${suffix}"
[[ "$app" != "$live_app" && "$sys" != "$live_sys" ]] || { echo "refusing to restore over live databases" >&2; exit 1; }

( cd "$src" && sha256sum -c SHA256SUMS )
mysql=(mariadb --defaults-extra-file="$MYSQL_DEFAULTS_FILE")
for db in "$app" "$sys"; do
  if [[ -n "$("${mysql[@]}" -N -e "SHOW DATABASES LIKE '$db'")" ]]; then
    echo "scratch database $db already exists; choose another suffix" >&2; exit 1
  fi
done
gunzip -c "$src/databases.sql.gz" \
  | sed -e "s/\`${live_app}\`/\`${app}\`/g" -e "s/\`${live_sys}\`/\`${sys}\`/g" \
  | "${mysql[@]}"
"${mysql[@]}" -e "SELECT 'users' t, COUNT(*) FROM \`${app}\`.users UNION ALL SELECT 'audit_logs', COUNT(*) FROM \`${app}\`.audit_logs UNION ALL SELECT 'goose_version', MAX(version_id) FROM \`${app}\`.goose_db_version"
echo "restored into ${app} / ${sys}. Inspect, then drop them manually when done."
