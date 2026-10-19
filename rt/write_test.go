package rt

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/maago-web/maago"
	"github.com/maago-web/maago/internal/rtcore"
)

// pgError and liteError have the methods, and the text, of the drivers'
// errors: pgx's *pgconn.PgError and modernc's *sqlite.Error.
type pgError struct{ code, msg string }

func (e pgError) Error() string    { return "ERROR: " + e.msg + " (SQLSTATE " + e.code + ")" }
func (e pgError) SQLState() string { return e.code }

type liteError struct {
	code int
	msg  string
}

func (e liteError) Error() string { return fmt.Sprintf("constraint failed: %s (%d)", e.msg, e.code) }
func (e liteError) Code() int     { return e.code }

// Write tells which of a model's unique constraints refused it, by the
// name PostgreSQL gives, or the columns SQLite lists; one constraint is
// the one, whatever the message; another error passes as it is.
func TestWriteUnique(t *testing.T) {
	pg := []UniqueKey{{[]string{"Slug"}, "post_slug_key"}, {[]string{"BoardID", "Title"}, "post_board_id_title_key"}}
	lite := []UniqueKey{{[]string{"BoardID"}, "post.board_id"}, {[]string{"BoardID", "Title"}, "post.board_id, post.title"}}
	other := errors.New("disk full")
	for _, c := range []struct {
		name   string
		keys   []UniqueKey
		err    error
		fields []string // nil: not a UniqueError, or of no key
		unique bool
	}{
		{"pg, the second", pg, pgError{"23505", `duplicate key value violates unique constraint "post_board_id_title_key"`}, []string{"BoardID", "Title"}, true},
		{"pg, translated", pg, pgError{"23505", `중복된 키 값이 "post_slug_key" 고유 제약 조건을 위반함`}, []string{"Slug"}, true},
		{"pg, wrapped", pg, fmt.Errorf("insert: %w", pgError{"23505", `duplicate key value violates unique constraint "post_slug_key"`}), []string{"Slug"}, true},
		{"pg, a name no key has", pg, pgError{"23505", `duplicate key value violates unique constraint "post_pkey"`}, nil, true},
		{"pg, a name a key's is the start of", pg, pgError{"23505", `duplicate key value violates unique constraint "post_slug_key2"`}, nil, true},
		{"pg, another constraint", pg, pgError{"23503", `insert or update on table "post" violates foreign key constraint "post_board_id_fkey"`}, nil, false},
		{"sqlite, the shorter of two that start alike", lite, liteError{2067, "UNIQUE constraint failed: post.board_id"}, []string{"BoardID"}, true},
		{"sqlite, the longer", lite, liteError{2067, "UNIQUE constraint failed: post.board_id, post.title"}, []string{"BoardID", "Title"}, true},
		{"sqlite, columns no key has", lite, liteError{2067, "UNIQUE constraint failed: post.slug"}, nil, true},
		{"sqlite, the primary key", lite, liteError{1555, "UNIQUE constraint failed: post.id"}, nil, false},
		{"one key: no message read", []UniqueKey{{[]string{"Email"}, "user_email_key"}}, pgError{"23505", "anything"}, []string{"Email"}, true},
		{"no keys", nil, liteError{2067, "UNIQUE constraint failed: post.x"}, nil, true},
		{"another error", pg, other, nil, false},
	} {
		err := Write(context.Background(), testDB, c.keys, func() error { return c.err })
		var u *maago.UniqueError
		if errors.As(err, &u) != c.unique || c.unique && (!slices.Equal(u.Fields, c.fields) || !errors.Is(err, c.err)) || !c.unique && err != c.err {
			t.Errorf("%s: %#v", c.name, err)
		}
	}
	if err := Write(context.Background(), testDB, pg, func() error { return nil }); err != nil {
		t.Error(err)
	}
}

// Fail puts a unique constraint's error on the one field of the form it
// is over, or on the form; any other error it leaves.
func TestFail(t *testing.T) {
	ctx := rtcore.WithSettings(context.Background(), rtcore.NewSettings(rtcore.Messages{Taken: "이미 있습니다"}, rtcore.Messages(maago.DefaultMessages), nil))
	for _, c := range []struct {
		name        string
		err         error
		ok          bool
		title, form string
		unbound     bool
	}{
		{"the field", &maago.UniqueError{Fields: []string{"Title"}}, true, "이미 있습니다", "", false},
		{"one field of two in the form", &maago.UniqueError{Fields: []string{"BoardID", "Title"}}, true, "이미 있습니다", "", false},
		{"two fields in the form", &maago.UniqueError{Fields: []string{"Title", "Body"}}, true, "", "이미 있습니다", false},
		{"no field in the form", &maago.UniqueError{Fields: []string{"Slug"}}, true, "", "이미 있습니다", false},
		{"no constraint known", &maago.UniqueError{}, true, "", "이미 있습니다", false},
		{"wrapped", fmt.Errorf("save: %w", &maago.UniqueError{Fields: []string{"Title"}}), true, "이미 있습니다", "", false},
		{"another error", errors.New("disk full"), false, "", "", false},
		{"nil", nil, false, "", "", false},
		{"unbound: the default", &maago.UniqueError{Fields: []string{"Title"}}, true, "This value is already taken.", "", true},
	} {
		var f maago.Form
		var title, body string
		if !c.unbound {
			FinishForm(ctx, &f, true, true)
		}
		ok := Fail(&f, c.err, map[string]*string{"Title": &title, "Body": &body})
		form := ""
		if len(f.Errors) > 0 {
			form = f.Errors[0]
		}
		if ok != c.ok || title != c.title || body != "" || form != c.form || len(f.Errors) > 1 {
			t.Errorf("%s: %v %q %q %q", c.name, ok, title, body, f.Errors)
		}
	}
}
