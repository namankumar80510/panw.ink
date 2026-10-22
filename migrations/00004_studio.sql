-- Publishing studio support (IMPLEMENTATION.md §5.4 entry_access, §7.5, §11).

-- +goose Up

-- Early-access tier requirement per entry. Only meaningful before public_at.
-- creator_connection_id gains its FK when creator_connections arrives with
-- the Patreon integration; until then early entries name only a rank.
CREATE TABLE entry_access (
  entry_id              BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  owner_user_id         BIGINT UNSIGNED NOT NULL,
  creator_connection_id BIGINT UNSIGNED NULL,
  minimum_access_rank   SMALLINT UNSIGNED NOT NULL,
  created_at            DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at            DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_entry_access_entry_owner FOREIGN KEY (entry_id, owner_user_id) REFERENCES entries (id, owner_user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT ck_entry_access_rank CHECK (minimum_access_rank >= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Re-encoded image variants. Only variants of 'ready' assets are served.
CREATE TABLE asset_variants (
  asset_id      BIGINT UNSIGNED NOT NULL,
  variant       ENUM('full','card') NOT NULL,
  owner_user_id BIGINT UNSIGNED NOT NULL,
  storage_key   VARCHAR(400) NOT NULL,
  mime          VARCHAR(100) NOT NULL,
  width         INT UNSIGNED NOT NULL,
  height        INT UNSIGNED NOT NULL,
  byte_size     BIGINT UNSIGNED NOT NULL,
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (asset_id, variant),
  UNIQUE KEY uq_asset_variants_key (storage_key),
  CONSTRAINT fk_asset_variants_asset FOREIGN KEY (asset_id) REFERENCES assets (id) ON DELETE RESTRICT,
  CONSTRAINT fk_asset_variants_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT,
  CONSTRAINT ck_asset_variants_mime CHECK (mime IN ('image/jpeg','image/png'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Assets need an owner-matching unique key for cover/avatar ownership FKs
-- and a validation error code for the studio.
ALTER TABLE assets
  ADD COLUMN error_code VARCHAR(60) NULL AFTER state,
  ADD UNIQUE KEY uq_assets_owner (id, owner_user_id);

-- Read-only studio projection: entries with their release state inputs and
-- current draft/published revision summaries, without revision bodies.
CREATE VIEW studio_entries AS
  SELECT e.id, e.work_id, e.work_kind, e.owner_user_id, e.volume_id, e.ordinal, e.slug, e.title, e.summary,
         e.workflow, e.early_at, e.public_at, e.current_draft_revision_id, e.published_revision_id,
         d.version AS draft_version, d.word_count AS draft_word_count, d.created_at AS draft_saved_at,
         p.version AS published_version, p.word_count AS published_word_count,
         a.minimum_access_rank, e.last_edited_at, e.created_at, e.updated_at
  FROM entries e
  LEFT JOIN entry_revisions d ON d.id = e.current_draft_revision_id
  LEFT JOIN entry_revisions p ON p.id = e.published_revision_id
  LEFT JOIN entry_access a ON a.entry_id = e.id;

-- Revision history without bodies (bodies come from entry_revisions by id).
CREATE VIEW studio_revisions AS
  SELECT r.id, r.entry_id, r.owner_user_id, r.version, r.word_count, r.estimated_read_seconds,
         r.editor_user_id, r.change_note, r.created_at
  FROM entry_revisions r;

-- +goose Down
DROP VIEW IF EXISTS studio_revisions;
DROP VIEW IF EXISTS studio_entries;
ALTER TABLE assets DROP INDEX uq_assets_owner, DROP COLUMN error_code;
DROP TABLE IF EXISTS asset_variants;
DROP TABLE IF EXISTS entry_access;
