-- Fiction catalogue and content (IMPLEMENTATION.md §3, §5.2, §5.6).
--
-- Invariants enforced by the database, not only by services:
--  * a short story has exactly one entry, ordinal 1: entries carry work_kind
--    through a composite FK to works(id, kind), plus CHECK + UNIQUE(work_id, ordinal);
--  * volumes and volume assignment exist only for novels; an entry's volume
--    belongs to the same work (composite FK);
--  * collections contain only short stories (collection_items.work_kind);
--  * owner_user_id projections match their parents (composite FKs), so
--    Collar rules can use one-level ownership checks (§5.6);
--  * scheduled/published entries have public_at and a published revision;
--    early_at <= public_at with at most 30 days of early lead (§3.2).
-- Revision pointers (entries -> entry_revisions) are added in 00003.

-- +goose Up
CREATE TABLE assets (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  owner_user_id BIGINT UNSIGNED NOT NULL,
  storage_key   VARCHAR(400) NOT NULL,
  mime          VARCHAR(100) NOT NULL,
  byte_size     BIGINT UNSIGNED NOT NULL,
  width         INT UNSIGNED NULL,
  height        INT UNSIGNED NULL,
  sha256        BINARY(32) NULL,
  alt_text      VARCHAR(300) NOT NULL DEFAULT '',
  state         ENUM('pending','ready','quarantined','deleted') NOT NULL DEFAULT 'pending',
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  deleted_at    DATETIME(6) NULL,
  UNIQUE KEY uq_assets_storage_key (storage_key),
  KEY idx_assets_owner (owner_user_id, created_at),
  CONSTRAINT fk_assets_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT,
  CONSTRAINT ck_assets_mime CHECK (mime IN ('image/jpeg','image/png','image/webp')),
  CONSTRAINT ck_assets_size CHECK (byte_size <= 5242880)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pen_names (
  id                            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  owner_user_id                 BIGINT UNSIGNED NOT NULL,
  slug                          VARCHAR(100) NOT NULL,
  name                          VARCHAR(140) NOT NULL,
  bio                           TEXT NOT NULL DEFAULT '',
  avatar_asset_id               BIGINT UNSIGNED NULL,
  website_url                   VARCHAR(500) NULL,
  patreon_creator_connection_id BIGINT UNSIGNED NULL, -- FK added with creator_connections (Patreon phase)
  state                         ENUM('active','hidden') NOT NULL DEFAULT 'active',
  created_at                    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at                    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_pen_names_slug (slug),
  UNIQUE KEY uq_pen_names_owner (id, owner_user_id),
  KEY idx_pen_names_owner (owner_user_id),
  CONSTRAINT fk_pen_names_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT,
  CONSTRAINT fk_pen_names_avatar FOREIGN KEY (avatar_asset_id) REFERENCES assets (id) ON DELETE RESTRICT,
  CONSTRAINT ck_pen_names_slug CHECK (slug REGEXP '^[a-z0-9]+(-[a-z0-9]+)*$' AND CHAR_LENGTH(slug) BETWEEN 2 AND 100),
  CONSTRAINT ck_pen_names_name CHECK (CHAR_LENGTH(name) BETWEEN 1 AND 140),
  CONSTRAINT ck_pen_names_website CHECK (website_url IS NULL OR website_url REGEXP '^https?://')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE author_collaborators (
  pen_name_id BIGINT UNSIGNED NOT NULL,
  user_id     BIGINT UNSIGNED NOT NULL,
  permission  ENUM('editor','manager') NOT NULL,
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (pen_name_id, user_id),
  KEY idx_author_collaborators_user (user_id),
  CONSTRAINT fk_author_collaborators_pen FOREIGN KEY (pen_name_id) REFERENCES pen_names (id) ON DELETE RESTRICT,
  CONSTRAINT fk_author_collaborators_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE genres (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  slug          VARCHAR(80) NOT NULL,
  name          VARCHAR(100) NOT NULL,
  enabled       BOOLEAN NOT NULL DEFAULT TRUE,
  display_order INT NOT NULL DEFAULT 0,
  UNIQUE KEY uq_genres_slug (slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO genres (slug, name, display_order) VALUES
  ('fantasy', 'Fantasy', 10), ('progression-fantasy', 'Progression fantasy', 20), ('cultivation', 'Cultivation', 30),
  ('isekai', 'Isekai', 40), ('romance', 'Romance', 50), ('mystery', 'Mystery', 60),
  ('psychological-thriller', 'Psychological thriller', 70), ('crime', 'Crime', 80), ('horror', 'Horror', 90),
  ('sci-fi', 'Science fiction', 100), ('literary', 'Literary', 110), ('comedy', 'Comedy', 120),
  ('adventure', 'Adventure', 130), ('drama', 'Drama', 140), ('historical', 'Historical', 150), ('other', 'Other', 1000);

CREATE TABLE tags (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  slug         VARCHAR(80) NOT NULL,
  label        VARCHAR(100) NOT NULL,
  category     ENUM('theme','trope','setting','content','other') NOT NULL DEFAULT 'other',
  is_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
  state        ENUM('approved','proposed','rejected') NOT NULL DEFAULT 'proposed',
  created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_tags_slug (slug),
  KEY idx_tags_state (state, category),
  CONSTRAINT ck_tags_slug CHECK (slug REGEXP '^[a-z0-9]+(-[a-z0-9]+)*$')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE works (
  id                    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  pen_name_id           BIGINT UNSIGNED NOT NULL,
  owner_user_id         BIGINT UNSIGNED NOT NULL,
  kind                  ENUM('novel','short_story') NOT NULL,
  slug                  VARCHAR(160) NOT NULL,
  title                 VARCHAR(240) NOT NULL,
  subtitle              VARCHAR(240) NULL,
  synopsis              TEXT NOT NULL DEFAULT '',
  cover_asset_id        BIGINT UNSIGNED NULL,
  primary_genre_id      BIGINT UNSIGNED NOT NULL,
  publication_status    ENUM('ongoing','completed','hiatus','cancelled') NOT NULL DEFAULT 'ongoing',
  visibility            ENUM('draft','listed','unlisted','hidden') NOT NULL DEFAULT 'draft',
  age_rating            ENUM('general','teen','mature') NOT NULL DEFAULT 'teen',
  language_code         VARCHAR(16) NOT NULL DEFAULT 'en',
  content_warnings_json JSON NULL,
  first_public_at       DATETIME(6) NULL,
  last_public_update_at DATETIME(6) NULL,
  completed_at          DATETIME(6) NULL,
  is_featured           BOOLEAN NOT NULL DEFAULT FALSE,
  -- Listing rollups over entries whose public_at has passed. Maintained by
  -- the rollup refresh; never used for access decisions (DECISIONS D-016).
  public_entry_count    INT UNSIGNED NOT NULL DEFAULT 0,
  public_word_count     INT UNSIGNED NOT NULL DEFAULT 0,
  rollups_at            DATETIME(6) NULL,
  created_at            DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at            DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  deleted_at            DATETIME(6) NULL,
  UNIQUE KEY uq_works_slug (slug),
  UNIQUE KEY uq_works_kind (id, kind),
  UNIQUE KEY uq_works_owner (id, owner_user_id),
  KEY idx_works_listing (kind, visibility, last_public_update_at),
  KEY idx_works_genre (primary_genre_id, publication_status),
  KEY idx_works_pen_name (pen_name_id, created_at),
  KEY idx_works_owner (owner_user_id),
  KEY idx_works_first_public (visibility, first_public_at),
  FULLTEXT KEY ft_works_text (title, synopsis),
  CONSTRAINT fk_works_pen_name_owner FOREIGN KEY (pen_name_id, owner_user_id) REFERENCES pen_names (id, owner_user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_works_owner FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE RESTRICT,
  CONSTRAINT fk_works_cover FOREIGN KEY (cover_asset_id) REFERENCES assets (id) ON DELETE RESTRICT,
  CONSTRAINT fk_works_genre FOREIGN KEY (primary_genre_id) REFERENCES genres (id) ON DELETE RESTRICT,
  CONSTRAINT ck_works_slug CHECK (slug REGEXP '^[a-z0-9]+(-[a-z0-9]+)*$' AND CHAR_LENGTH(slug) BETWEEN 3 AND 160),
  CONSTRAINT ck_works_title CHECK (CHAR_LENGTH(title) BETWEEN 1 AND 240),
  CONSTRAINT ck_works_synopsis CHECK (CHAR_LENGTH(synopsis) <= 4000),
  CONSTRAINT ck_works_story_complete CHECK (kind = 'novel' OR publication_status IN ('ongoing','completed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE work_tags (
  work_id BIGINT UNSIGNED NOT NULL,
  tag_id  BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (work_id, tag_id),
  KEY idx_work_tags_tag (tag_id),
  CONSTRAINT fk_work_tags_work FOREIGN KEY (work_id) REFERENCES works (id) ON DELETE RESTRICT,
  CONSTRAINT fk_work_tags_tag FOREIGN KEY (tag_id) REFERENCES tags (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE volumes (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  work_id       BIGINT UNSIGNED NOT NULL,
  work_kind     ENUM('novel','short_story') NOT NULL DEFAULT 'novel',
  owner_user_id BIGINT UNSIGNED NOT NULL,
  number        SMALLINT UNSIGNED NOT NULL,
  title         VARCHAR(160) NOT NULL,
  description   TEXT NULL,
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_volumes_number (work_id, number),
  UNIQUE KEY uq_volumes_work (id, work_id),
  CONSTRAINT fk_volumes_work_kind FOREIGN KEY (work_id, work_kind) REFERENCES works (id, kind) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_volumes_work_owner FOREIGN KEY (work_id, owner_user_id) REFERENCES works (id, owner_user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT ck_volumes_novel CHECK (work_kind = 'novel'),
  CONSTRAINT ck_volumes_number CHECK (number >= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE entries (
  id                        BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  work_id                   BIGINT UNSIGNED NOT NULL,
  work_kind                 ENUM('novel','short_story') NOT NULL,
  owner_user_id             BIGINT UNSIGNED NOT NULL,
  volume_id                 BIGINT UNSIGNED NULL,
  ordinal                   INT UNSIGNED NOT NULL,
  slug                      VARCHAR(160) NOT NULL,
  title                     VARCHAR(240) NOT NULL,
  summary                   TEXT NULL,
  workflow                  ENUM('draft','scheduled','published','archived') NOT NULL DEFAULT 'draft',
  early_at                  DATETIME(6) NULL,
  public_at                 DATETIME(6) NULL,
  current_draft_revision_id BIGINT UNSIGNED NULL,
  published_revision_id     BIGINT UNSIGNED NULL,
  last_edited_at            DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  created_at                DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at                DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_entries_ordinal (work_id, ordinal),
  UNIQUE KEY uq_entries_slug (work_id, slug),
  UNIQUE KEY uq_entries_owner (id, owner_user_id),
  KEY idx_entries_work_public (work_id, workflow, public_at),
  KEY idx_entries_early (workflow, early_at),
  KEY idx_entries_public (workflow, public_at),
  KEY idx_entries_volume (volume_id, work_id),
  CONSTRAINT fk_entries_work_kind FOREIGN KEY (work_id, work_kind) REFERENCES works (id, kind) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_entries_work_owner FOREIGN KEY (work_id, owner_user_id) REFERENCES works (id, owner_user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_entries_volume FOREIGN KEY (volume_id, work_id) REFERENCES volumes (id, work_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT ck_entries_story_single CHECK (work_kind = 'novel' OR ordinal = 1),
  CONSTRAINT ck_entries_story_volume CHECK (work_kind = 'novel' OR volume_id IS NULL),
  CONSTRAINT ck_entries_ordinal CHECK (ordinal >= 1),
  CONSTRAINT ck_entries_slug CHECK (slug REGEXP '^[a-z0-9]+(-[a-z0-9]+)*$' AND CHAR_LENGTH(slug) BETWEEN 1 AND 160),
  CONSTRAINT ck_entries_release CHECK (workflow IN ('draft','archived') OR (public_at IS NOT NULL AND published_revision_id IS NOT NULL)),
  CONSTRAINT ck_entries_early CHECK (early_at IS NULL OR (public_at IS NOT NULL AND early_at <= public_at AND TIMESTAMPDIFF(SECOND, early_at, public_at) <= 2592000))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE entry_revisions (
  id                     BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  entry_id               BIGINT UNSIGNED NOT NULL,
  owner_user_id          BIGINT UNSIGNED NOT NULL,
  version                INT UNSIGNED NOT NULL,
  markdown               LONGTEXT NOT NULL,
  rendered_html          LONGTEXT NOT NULL,
  plain_text             LONGTEXT NULL,
  word_count             INT UNSIGNED NOT NULL,
  estimated_read_seconds INT UNSIGNED NOT NULL,
  checksum               BINARY(32) NOT NULL,
  editor_user_id         BIGINT UNSIGNED NOT NULL,
  change_note            VARCHAR(250) NULL,
  created_at             DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_entry_revisions_version (entry_id, version),
  UNIQUE KEY uq_entry_revisions_entry (id, entry_id),
  CONSTRAINT fk_entry_revisions_entry_owner FOREIGN KEY (entry_id, owner_user_id) REFERENCES entries (id, owner_user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_entry_revisions_editor FOREIGN KEY (editor_user_id) REFERENCES users (id) ON DELETE RESTRICT,
  CONSTRAINT ck_entry_revisions_size CHECK (OCTET_LENGTH(markdown) <= 2097152)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE collections (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  pen_name_id    BIGINT UNSIGNED NOT NULL,
  owner_user_id  BIGINT UNSIGNED NOT NULL,
  slug           VARCHAR(160) NOT NULL,
  title          VARCHAR(240) NOT NULL,
  description    TEXT NOT NULL DEFAULT '',
  cover_asset_id BIGINT UNSIGNED NULL,
  visibility     ENUM('draft','listed','unlisted','hidden') NOT NULL DEFAULT 'draft',
  created_at     DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at     DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_collections_slug (slug),
  KEY idx_collections_visibility (visibility, updated_at),
  CONSTRAINT fk_collections_pen_name_owner FOREIGN KEY (pen_name_id, owner_user_id) REFERENCES pen_names (id, owner_user_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_collections_cover FOREIGN KEY (cover_asset_id) REFERENCES assets (id) ON DELETE RESTRICT,
  CONSTRAINT ck_collections_slug CHECK (slug REGEXP '^[a-z0-9]+(-[a-z0-9]+)*$' AND CHAR_LENGTH(slug) BETWEEN 3 AND 160)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE collection_items (
  collection_id BIGINT UNSIGNED NOT NULL,
  work_id       BIGINT UNSIGNED NOT NULL,
  work_kind     ENUM('novel','short_story') NOT NULL DEFAULT 'short_story',
  ordinal       INT UNSIGNED NOT NULL,
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (collection_id, work_id),
  UNIQUE KEY uq_collection_items_ordinal (collection_id, ordinal),
  KEY idx_collection_items_work (work_id),
  CONSTRAINT fk_collection_items_collection FOREIGN KEY (collection_id) REFERENCES collections (id) ON DELETE RESTRICT,
  CONSTRAINT fk_collection_items_work_kind FOREIGN KEY (work_id, work_kind) REFERENCES works (id, kind) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT ck_collection_items_story CHECK (work_kind = 'short_story')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE slug_redirects (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  entity_type ENUM('work','entry','author','collection') NOT NULL,
  old_path    VARCHAR(500) NOT NULL,
  new_path    VARCHAR(500) NOT NULL,
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uq_slug_redirects_old (old_path),
  CONSTRAINT ck_slug_redirects_cycle CHECK (old_path <> new_path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS slug_redirects;
DROP TABLE IF EXISTS collection_items;
DROP TABLE IF EXISTS collections;
DROP TABLE IF EXISTS entry_revisions;
DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS volumes;
DROP TABLE IF EXISTS work_tags;
DROP TABLE IF EXISTS works;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS genres;
DROP TABLE IF EXISTS author_collaborators;
DROP TABLE IF EXISTS pen_names;
DROP TABLE IF EXISTS assets;
