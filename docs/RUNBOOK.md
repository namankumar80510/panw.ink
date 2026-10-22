# Runbook

Operational procedures. **This project may be developed on a machine that also
runs production services.** Before any deployment or database change, inspect
`pwd`, `git status`, `ss -ltnp`, `systemctl list-units --type=service`, the
Nginx sites and the existing databases. Never touch unrelated services,
volumes, buckets or certificates, and never run `DROP DATABASE`, `TRUNCATE`
or `docker compose down -v` against anything but the disposable dev stack.

## Phase 0 host inspection (2026-10-22, development machine)

- Toolchain: Go 1.26.0 (builds use the go1.26.10 toolchain), Node 22.22.3 with npm 12.0.2, Docker 29.9.1 with Compose 5.6.0.
- Host services: MariaDB 10.11.14 on 127.0.0.1:3306, Redis on 127.0.0.1/::1:6379, PostgreSQL on loopback:5432,
  a web server on *:80/*:443 with an admin endpoint on 127.0.0.1:2019, CUPS on 631.
  None of them were modified.
- Pawn.Ink development uses its own Compose stack on 13306/16379/19000/19001/11025/18025 (loopback only) and API port 14021 (verified free).

## First-time production setup (operator, with approval)

1. Create the `pawn-ink` system user and `/srv/pawn-ink/releases/<version>`, plus `/srv/pawn-ink/current` as a symlink.
2. Create the databases and accounts (adapt the host and passwords; never commit them):
   ```sql
   CREATE DATABASE pawn_ink CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
   CREATE DATABASE pawn_ink_collar CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
   CREATE USER 'pawn_app'@'127.0.0.1' IDENTIFIED BY '<runtime secret>';
   GRANT SELECT, INSERT, UPDATE, DELETE ON pawn_ink.* TO 'pawn_app'@'127.0.0.1';
   GRANT SELECT, INSERT, UPDATE, DELETE ON pawn_ink_collar.* TO 'pawn_app'@'127.0.0.1';
   CREATE USER 'pawn_migrator'@'127.0.0.1' IDENTIFIED BY '<migration secret>';
   GRANT ALL PRIVILEGES ON pawn_ink.* TO 'pawn_migrator'@'127.0.0.1';
   GRANT ALL PRIVILEGES ON pawn_ink_collar.* TO 'pawn_migrator'@'127.0.0.1';
   ```
   Set `default_time_zone = '+00:00'` and `binlog_format = MIXED|ROW`.
3. Redis: `appendonly yes`, `appendfsync everysec` (about 1 s loss window on a host crash), `maxmemory-policy noeviction`, and a persistent data directory.
4. Write `/etc/pawn-ink/pawn-ink.env` (0640 root:pawn-ink) from `.env.example`. Use fresh `openssl rand -hex 32` values, `APP_ENV=production`, `APP_BASE_URL=https://pawn.ink`, a loopback `HTTP_ADDR` (= `COLLAR_ADDR`), `TRUSTED_PROXY_CIDRS=127.0.0.1/32`, `COLLAR_CORS_ORIGINS=https://pawn.ink`, and an empty `WEB_DIST_DIR`.
5. Migrate (from the release directory, with the env loaded): `pawn-ink-migrate status`, then `pawn-ink-migrate up`, `pawn-ink-collar migrate`, `pawn-ink-collar config check`.
6. Install the `deployments/*.service.example` units. Propose the Nginx server block to the operator, then run `nginx -t` and reload only after approval.
7. Owner: `pawn-ink-maintenance owner-bootstrap --email <owner>` (prompts for a password). Operator superuser, only if needed: `pawn-ink-collar superuser create`.
8. Run `SMOKE_URL=https://pawn.ink scripts/smoke.sh`.

## Routine operations

| Task | Command / procedure |
|---|---|
| Grant author/moderator | `pawn-ink-maintenance set-role --email x --role author` (audited) |
| Reset a lost password | Verify the person out of band, then `pawn-ink-maintenance reset-password --email x`. Revokes their browser sessions. |
| Revoke all sessions of a user | The user can use “Sign out everywhere”; operators can set `state='suspended'`, which takes effect on the next request (a custom command arrives in Phase 8). |
| Reload Collar after a migration | `systemctl kill -s HUP pawn-ink` |
| Inspect stalled jobs | Operator superuser session: `GET /api/v1/admin/jobs` (not available to the app owner). Mail failures: `SELECT id, state, attempts, last_error_code FROM pawn_ink.email_outbox WHERE state <> 'sent'`. |
| Backups | `deployments/backup.sh` (see header). Restore check: `deployments/restore-check.sh <dir> <suffix>`, which restores into new databases only. |
| Rotate `TOKEN_ENCRYPTION_KEY` | Not yet automated. Rotating today invalidates browser sessions (users sign in again) and, from Phase 5, stored Patreon tokens. Plan a re-encryption migration before rotating. |
| Rotate HMAC keys | `SESSION_HMAC_KEY` signs everyone out and voids outstanding email links. `CSRF_HMAC_KEY` only forces a page reload. |
| Deploy / roll back | Build with `make build`, copy `bin/` to a new release dir, take a backup, migrate, switch the `current` symlink, `systemctl restart pawn-ink pawn-ink-worker`, run the smoke checks. Roll back by pointing the symlink at the previous release; down migrations are never automatic. |
