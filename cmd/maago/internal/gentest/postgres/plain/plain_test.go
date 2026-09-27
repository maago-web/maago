package plain

import "testing"

import "github.com/maago-web/maago/cmd/maago/internal/gentest"

func TestPlain(t *testing.T) {
	ctx, db := gentest.Open(t, "internal/testdata/plain")
	for _, body := range []string{"", "long note", ""} {
		if _, err := InsertNote(ctx, db, NewNoteInput{Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	for long, want := range map[bool]int{false: 3, true: 1} {
		if ns, err := Notes(ctx, db, NotesParams{Long: long}); len(ns) != want || err != nil {
			t.Errorf("Notes long=%v: %v %v, want %d", long, ns, err, want)
		}
	}
	if n, err := DeleteEmptyNotes(ctx, db); n != 2 || err != nil {
		t.Errorf("DeleteEmptyNotes: %d %v, want 2", n, err)
	}
}
