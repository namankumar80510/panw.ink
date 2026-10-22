package db

import (
	"testing"
	"time"
)

func TestTimeScan(t *testing.T) {
	var v Time
	if err := v.Scan([]byte("2026-10-22 16:01:44.123456")); err != nil || !v.Valid || v.Time.Nanosecond() != 123456000 || v.Time.Location() != time.UTC {
		t.Fatalf("%v %v", v, err)
	}
	if err := v.Scan(nil); err != nil || v.Valid {
		t.Fatal("NULL must be invalid")
	}
	if err := v.Scan("2026-10-22 16:01:44"); err != nil || v.Time.Hour() != 16 {
		t.Fatal(err)
	}
	if err := v.Scan("garbage"); err == nil {
		t.Fatal("garbage must fail")
	}
}
