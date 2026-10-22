-- Identity, browser-session bridge, one-time tokens, audit, mail outbox and
-- site settings (IMPLEMENTATION.md §5.1, §5.5). All times are UTC.

-- +goose Up
CREATE TABLE users (
  id                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  email                VARCHAR(254) NOT NULL,
  handle               VARCHAR(40) NULL,
  display_name         VARCHAR(80) NOT NULL,
  password_hash        VARCHAR(255) NOT NULL,
  email_verified_at    DATETIME(6) NULL,
  state                ENUM('active','suspended','deleted') NOT NULL DEFAULT 'active',
  is_active            BOOLEAN GENERATED ALWAYS AS (state = 'active') STORED,
  role                 ENUM('reader','author','moderator','owner') NOT NULL DEFAULT 'reader',
  locale               VARCHAR(20) NULL,
  timezone             VARCHAR(60) NOT NULL DEFAULT 'UTC',
  reader_settings_json JSON NULL,
  created_at           DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at           DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  deleted_at           DATETIME(6) NULL,
  UNIQUE KEY uq_users_email (email),
  UNIQUE KEY uq_users_handle (handle),
  KEY idx_users_role_state (role, state),
  CONSTRAINT ck_users_email_lower CHECK (CAST(email AS BINARY) = CAST(LOWER(email) AS BINARY)),
  CONSTRAINT ck_users_email_len CHECK (CHAR_LENGTH(email) BETWEEN 3 AND 254),
  CONSTRAINT ck_users_handle CHECK (handle IS NULL OR handle REGEXP '^[a-z0-9_]{3,40}$'),
  CONSTRAINT ck_users_display_name CHECK (CHAR_LENGTH(display_name) BETWEEN 1 AND 80),
  CONSTRAINT ck_users_deleted CHECK ((state = 'deleted') = (deleted_at IS NOT NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Pawn.Ink's cookie -> Collar token bridge. Never exposed through Collar.
CREATE TABLE browser_sessions (
  id                        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id                   BIGINT UNSIGNED NOT NULL,
  token_hash                BINARY(32) NOT NULL,
  csrf_token_hash           BINARY(32) NOT NULL,
  collar_session_id         VARCHAR(80) NOT NULL,
  access_token_ciphertext   VARBINARY(4096) NOT NULL,
  access_expires_at         DATETIME(6) NOT NULL,
  refresh_token_ciphertext  VARBINARY(4096) NOT NULL,
  refresh_version           BIGINT UNSIGNED NOT NULL DEFAULT 0,
  expires_at                DATETIME(6) NOT NULL,
  revoked_at                DATETIME(6) NULL,
  last_seen_at              DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  user_agent_summary        VARCHAR(120) NULL,
  created_at                DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_browser_sessions_token (token_hash),
  KEY idx_browser_sessions_user (user_id, expires_at),
  KEY idx_browser_sessions_expires (expires_at),
  CONSTRAINT fk_browser_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE auth_tokens (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id     BIGINT UNSIGNED NOT NULL,
  type        ENUM('verify_email','reset_password') NOT NULL,
  token_hash  BINARY(32) NOT NULL,
  expires_at  DATETIME(6) NOT NULL,
  consumed_at DATETIME(6) NULL,
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_auth_tokens_hash (token_hash),
  KEY idx_auth_tokens_user (user_id, type, expires_at),
  KEY idx_auth_tokens_expires (expires_at),
  CONSTRAINT fk_auth_tokens_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE audit_logs (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  actor_user_id BIGINT UNSIGNED NULL,
  action        VARCHAR(100) NOT NULL,
  object_type   VARCHAR(60) NOT NULL,
  object_id     BIGINT UNSIGNED NULL,
  before_json   JSON NULL,
  after_json    JSON NULL,
  request_id    VARCHAR(60) NOT NULL DEFAULT '',
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  KEY idx_audit_object (object_type, object_id, created_at),
  KEY idx_audit_actor (actor_user_id, created_at),
  KEY idx_audit_created (created_at),
  CONSTRAINT fk_audit_actor FOREIGN KEY (actor_user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Transactional mail only (verification, reset). template_data_json holds the
-- one-time link until the message is sent, then it is scrubbed.
CREATE TABLE email_outbox (
  id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id            BIGINT UNSIGNED NULL,
  recipient          VARCHAR(254) NOT NULL,
  template           VARCHAR(100) NOT NULL,
  template_data_json JSON NOT NULL,
  state              ENUM('queued','sent','failed') NOT NULL DEFAULT 'queued',
  attempts           SMALLINT UNSIGNED NOT NULL DEFAULT 0,
  next_attempt_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  last_error_code    VARCHAR(100) NULL,
  created_at         DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  sent_at            DATETIME(6) NULL,
  KEY idx_email_outbox_state (state, next_attempt_at),
  KEY idx_email_outbox_user (user_id, created_at),
  CONSTRAINT fk_email_outbox_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE site_settings (
  `key`      VARCHAR(120) NOT NULL PRIMARY KEY,
  value_json JSON NOT NULL,
  updated_by BIGINT UNSIGNED NULL,
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_site_settings_user FOREIGN KEY (updated_by) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Durable UTC buckets for Pawn.Ink's recurring scheduler (§12.3). A row is
-- inserted in the same transaction that enqueues the Collar job, so a bucket
-- is enqueued at most once even across restarts and multiple workers.
CREATE TABLE scheduler_runs (
  name        VARCHAR(80) NOT NULL,
  bucket      VARCHAR(40) NOT NULL,
  job_id      BIGINT UNSIGNED NULL,
  enqueued_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (name, bucket),
  KEY idx_scheduler_runs_enqueued (enqueued_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Read-only, staff-safe projection of users for the admin dashboard (§7.5).
-- No password hash, no reader settings.
CREATE VIEW admin_user_summaries AS
  SELECT id, email, handle, display_name, role, state, email_verified_at, created_at, updated_at
  FROM users;

-- +goose Down
DROP VIEW IF EXISTS admin_user_summaries;
DROP TABLE IF EXISTS scheduler_runs;
DROP TABLE IF EXISTS site_settings;
DROP TABLE IF EXISTS email_outbox;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS auth_tokens;
DROP TABLE IF EXISTS browser_sessions;
DROP TABLE IF EXISTS users;
