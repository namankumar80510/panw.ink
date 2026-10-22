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

## 00002_catalogue, 00003_entry_revision_pointers

| Table | Purpose | Key constraints |
|---|---|---|
| `assets` | Uploaded image metadata (validation pipeline in Phase 3) | JPEG/PNG/WebP ≤ 5 MB |
| `pen_names` | Public identities; many per owner | slug pattern, `UNIQUE(id, owner_user_id)` for ownership FKs |
| `author_collaborators` | Future invited collaborators | PK (pen_name_id, user_id) |
| `genres` | Seeded list (§5.2) | one primary genre per work |
| `tags`, `work_tags` | Moderated tags; only `approved` are public | intersection filtering |
| `works` | Novel or short story | `kind` immutable in practice; `(pen_name_id, owner_user_id)` → pen_names; FULLTEXT(title, synopsis); listing rollups (D-016) |
| `volumes` | Optional novel volumes | `work_kind='novel'` via FK to works(id, kind) |
| `entries` | Chapters, or the single story text | short story ⇒ ordinal 1 only; volume belongs to the same work; release CHECKs; ownership FK |
| `entry_revisions` | Immutable Markdown + sanitized HTML + counts | `(entry_id, version)` unique; ≤ 2 MB; ownership FK to entries |
| `collections`, `collection_items` | Anthologies of short stories | items must be `short_story` |
| `slug_redirects` | Old → canonical paths | no self-redirects |

00003 adds `entries.(current_draft_revision_id|published_revision_id, id) →
entry_revisions(id, entry_id)`, so a pointer can only name a revision of the
same entry.
