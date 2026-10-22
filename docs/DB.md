# Database reference

Two databases on one MariaDB server (same-transaction outbox/job writes):

- `pawn_ink`: application schema, owned by Goose migrations in `migrations/`.
- `pawn_ink_collar`: Collar system tables (sessions, outbox, cursors, jobs,
  hooks), owned by `collar migrate`.

Accounts: the runtime account (`COLLAR_DSN`) has CRUD only on both databases.
The migration account (`COLLAR_ADMIN_DSN`) owns DDL. `cmd/migrate` refuses
pending migrations with destructive DDL unless `--allow-destructive
--backup-ref=<id>` is given, and never runs down migrations.

## 00001_core

| Table | Purpose | Notes |
|---|---|---|
| `users` | App accounts (Collar auth collection) | Email is lowercase and unique (CHECK). `is_active` is generated from `state='active'` and is Collar's `activeField`. `role` ∈ reader/author/moderator/owner. CHECKs on handle, display name, and deleted ⇔ `deleted_at`. |
| `browser_sessions` | Cookie → Collar token bridge | HMAC token hash, AES-GCM token ciphertexts, `refresh_version`, `revoked_at`. Not exposed to Collar. |
| `auth_tokens` | Email verification / password reset | HMAC token hash, single use (`consumed_at`). Not exposed. |
| `audit_logs` | Security and admin audit | No secrets. Owner-readable through Collar. |
| `email_outbox` | Transactional mail queue | Link scrubbed after send or failure. Not exposed. |
| `site_settings` | Allow-listed mutable settings | Secrets never stored here. |
| `scheduler_runs` | Durable UTC buckets for the recurring scheduler | PK (name, bucket). |
| `admin_user_summaries` | View: staff-safe user list | No hash, no reader settings. |

All timestamps are `DATETIME(6)` UTC. Collar's pools return them as text;
scan them with `internal/db.Time` (DECISIONS D-005). Valuable references use
`ON DELETE RESTRICT`.
