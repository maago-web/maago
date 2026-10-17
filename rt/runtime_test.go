package rt

import (
	"testing"
	"time"
)

func TestSQLiteTime(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	for _, c := range []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 9, 27, 12, 0, 5, 500_000_000, kst), "2026-09-27 03:00:05.500000"},
		{time.Date(2026, 9, 27, 12, 0, 5, 0, time.UTC), "2026-09-27 12:00:05.000000"},
		{time.Date(2026, 9, 27, 12, 0, 5, 123_456_789, time.UTC), "2026-09-27 12:00:05.123456"},
	} {
		if got := SQLiteTime(c.in); got != c.want {
			t.Errorf("SQLiteTime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScanSQLiteTime(t *testing.T) {
	want := time.Date(2026, 9, 27, 3, 0, 5, 250_000_000, time.UTC)
	for _, src := range []any{
		"2026-09-27 03:00:05.250000",
		"2026-09-27 03:00:05.25",
		[]byte("2026-09-27 03:00:05.25"),
		"2026-09-27T12:00:05.25+09:00",
		"2026-09-27T03:00:05.25Z",
	} {
		var got time.Time
		if err := ScanSQLiteTime(&got).Scan(src); err != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Errorf("%v: got %v %v, want %v in UTC", src, got, err, want)
		}
	}
	for _, src := range []any{nil, "2026-09-27 12:00:05.5 +0900 KST", "yesterday", int64(5)} {
		var got time.Time
		if err := ScanSQLiteTime(&got).Scan(src); err == nil {
			t.Errorf("%v: no error", src)
		}
	}
	p := &want
	if err := ScanNullSQLiteTime(&p).Scan(nil); err != nil || p != nil {
		t.Errorf("NULL: got %v %v, want nil", p, err)
	}
	if err := ScanNullSQLiteTime(&p).Scan("2026-09-27 03:00:05.25"); err != nil || p == nil || !p.Equal(want) {
		t.Errorf("got %v %v, want %v", p, err, want)
	}
}

func TestPlaceholders(t *testing.T) {
	if got := Placeholders("$", 3, 2); got != "$3, $4" {
		t.Errorf("got %q", got)
	}
	if got := Placeholders("?", 1, 1); got != "?1" {
		t.Errorf("got %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("n=0 did not panic")
		}
	}()
	Placeholders("$", 1, 0)
}

func TestSQLiteNullTime(t *testing.T) {
	if SQLiteNullTime(nil) != nil {
		t.Error("nil is not NULL")
	}
	at := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if got := SQLiteNullTime(&at); got != "2026-09-27 12:00:00.000000" {
		t.Errorf("got %v", got)
	}
}
