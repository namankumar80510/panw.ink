package db

import (
	"fmt"
	"time"
)

// Collar's pools never set parseTime (collar internal/db/dsn.go), so DATETIME
// columns read through App.DB()/Worker.DB() arrive as text. Time scans them
// as UTC; all Pawn.Ink times are UTC (§3.2 item 14).
type Time struct {
	Time  time.Time
	Valid bool // false for SQL NULL
}

var timeLayouts = []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", "2006-01-02"}

// Scan implements sql.Scanner.
func (t *Time) Scan(v any) error {
	*t = Time{}
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		t.Time, t.Valid = x.UTC(), true
		return nil
	case []byte:
		return t.parse(string(x))
	case string:
		return t.parse(x)
	}
	return fmt.Errorf("db: cannot scan %T into db.Time", v)
}

func (t *Time) parse(s string) error {
	for _, l := range timeLayouts {
		if p, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			t.Time, t.Valid = p, true
			return nil
		}
	}
	return fmt.Errorf("db: unrecognised datetime %q", s)
}
