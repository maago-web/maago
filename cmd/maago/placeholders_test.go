package main

import (
	"database/sql"
	"testing"
)

// The mapper numbers SQLite placeholders ?1…?n, repeating a number for a
// repeated parameter, and binds arguments in number order (M3 3번). That
// relies on modernc matching ?NNN to the argument of that ordinal.
func TestSQLiteNumberedPlaceholders(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var a, b, c string
	if err := db.QueryRow("SELECT ?2, ?1, ?1", "one", "two").Scan(&a, &b, &c); err != nil {
		t.Fatal(err)
	}
	if a != "two" || b != "one" || c != "one" {
		t.Errorf("got %s %s %s, want two one one", a, b, c)
	}
}
