package dialect

import (
	"testing"

	"github.com/maago-web/maago/internal/model"
)

func TestLiterals(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{Number(model.Int64, "007"), "7"},
		{Number(model.Int64, "-0"), "0"},
		{Number(model.Int32, "-2147483648"), "-2147483648"},
		{Number(model.Float64, "1.50"), "1.5"},
		{Number(model.Float64, "1e3"), "1000"},
		{Number(model.Float64, "-0.0"), "-0"},
		{String("it's"), "'it''s'"},
		{String(""), "''"},
		{Quote("order"), `"order"`},
		{Quote(`a"b`), `"a""b"`},
	} {
		if c.got != c.want {
			t.Errorf("got %s, want %s", c.got, c.want)
		}
	}
}
