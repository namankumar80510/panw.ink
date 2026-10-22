-- entries <-> entry_revisions reference each other, so the pointer FKs are
-- added after both tables exist (§5.6). The composite FKs also guarantee a
-- pointer names a revision of the same entry.

-- +goose Up
ALTER TABLE entries
  ADD CONSTRAINT fk_entries_draft_revision FOREIGN KEY (current_draft_revision_id, id)
    REFERENCES entry_revisions (id, entry_id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  ADD CONSTRAINT fk_entries_published_revision FOREIGN KEY (published_revision_id, id)
    REFERENCES entry_revisions (id, entry_id) ON DELETE RESTRICT ON UPDATE RESTRICT;

-- +goose Down
ALTER TABLE entries
  DROP FOREIGN KEY fk_entries_published_revision,
  DROP FOREIGN KEY fk_entries_draft_revision;
