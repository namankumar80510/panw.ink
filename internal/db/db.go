// Package db holds small helpers for explicit, fully qualified SQL against
// the Pawn.Ink application database. Collar's pools have no default
// database, so every table name is qualified.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Tables qualifies table names of the application database.
type Tables struct {
	db string
}

// NewTables returns Tables for the physical database name, which must be a
// trusted identifier (validated by config).
func NewTables(database string) Tables {
	return Tables{db: database}
}

// Database is the physical database name.
func (t Tables) Database() string { return t.db }

// T returns `db`.`table`.
func (t Tables) T(table string) string {
	return quote(t.db) + "." + quote(table)
}

func quote(id string) string {
	return "`" + strings.ReplaceAll(id, "`", "``") + "`"
}

// InTx runs fn inside a transaction on db, committing on success.
func InTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// Audit writes one audit_logs row inside tx. before/after must already be
// free of secrets; they are stored as JSON.
func Audit(ctx context.Context, tx *sql.Tx, t Tables, actorUserID *uint64, action, objectType string, objectID *uint64, before, after []byte, requestID string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO "+t.T("audit_logs")+
		" (actor_user_id, action, object_type, object_id, before_json, after_json, request_id) VALUES (?,?,?,?,?,?,?)",
		actorUserID, action, objectType, objectID, nullJSON(before), nullJSON(after), requestID)
	if err != nil {
		return fmt.Errorf("db: audit %s: %w", action, err)
	}
	return nil
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
