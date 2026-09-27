package project

import (
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/maago-web/maago/internal/diag"
)

// Dialect names the project's database. Only the name lives here; how the
// dialects differ is internal/dialect's business.
type Dialect string

const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

const (
	urlKey  = "MAAGO_DATABASE_URL"
	example = urlKey + "=sqlite:app.db or " + urlKey + "=postgres://localhost:5432/myapp"
	formats = "write sqlite:FILE or postgres://HOST:PORT/DB"
)

// Database returns the project's database from MAAGO_DATABASE_URL (D-08):
// the environment variable if set, even to "", otherwise the root's .env
// file. lookupEnv is os.LookupEnv outside tests. Every malformed .env line
// is reported, even when the environment wins. With any diagnostic, the
// dialect and URL are empty. Diagnostics never show the URL: it may hold
// a password.
func Database(root string, lookupEnv func(string) (string, bool)) (Dialect, string, []diag.Diagnostic) {
	src, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", "", []diag.Diagnostic{diag.New(token.Position{}, "cannot read .env: "+diag.OneLine(err.Error()), "make sure .env can be read")}
	}
	vars, maybeURL, ds := parseEnv(strings.TrimPrefix(string(src), "\uFEFF")) // editors on Windows may add a BOM

	url, pos, from := "", token.Position{}, "environment variable "+urlKey
	if v, ok := lookupEnv(urlKey); ok {
		url = v
	} else if v, ok := vars[urlKey]; ok {
		url, pos, from = v.value, token.Position{Filename: ".env", Line: v.line}, urlKey
	} else {
		if !maybeURL {
			ds = append(ds, diag.New(token.Position{}, urlKey+" is not set", "add "+example+" to .env"))
		}
		return "", "", ds
	}

	scheme, rest, ok := strings.Cut(url, ":")
	var d Dialect
	switch {
	case url == "" && pos.Line == 0:
		ds = append(ds, diag.New(pos, from+" is empty", "set it to sqlite:FILE or postgres://HOST:PORT/DB, or unset it to use .env"))
	case url == "":
		ds = append(ds, diag.New(pos, from+" is empty", formats))
	case !ok || !schemePattern.MatchString(scheme): // a mistyped address shows no more than a scheme
		ds = append(ds, diag.New(pos, from+" has no scheme", formats))
	case scheme == "sqlite" && rest == "":
		ds = append(ds, diag.New(pos, from+" has no file name after sqlite:", formats))
	case scheme == "sqlite" && strings.HasPrefix(rest, "//"):
		ds = append(ds, diag.New(pos, from+` starts with "sqlite://"`, "write sqlite:FILE, as in sqlite:app.db"))
	case scheme == "sqlite" && (strings.Contains(rest, "?") || strings.HasPrefix(rest, "file:")):
		// The driver would read what follows ? as options, and file: as a URI.
		ds = append(ds, diag.New(pos, from+" has ? or file: in its file name", "write sqlite:FILE with a plain file name, as in sqlite:app.db"))
	case scheme == "sqlite":
		d = SQLite
	case (scheme == "postgres" || scheme == "postgresql") && strings.HasPrefix(rest, "//"):
		d = Postgres
	default:
		ds = append(ds, diag.New(pos, fmt.Sprintf("%s starts with %q, not sqlite:, postgres:// or postgresql://", from, scheme+":"), formats))
	}
	if len(ds) > 0 {
		return "", "", ds
	}
	return d, url, nil
}

var (
	keyPattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*$`) // RFC 3986
)

type envVar struct {
	value string
	line  int
}

// parseEnv reads KEY=VALUE lines, # comment lines and blank lines. A value
// may be wrapped in one pair of matching quotes; nothing is escaped.
// maybeURL reports a broken line that may have been MAAGO_DATABASE_URL,
// so that "not set" is not reported on top of it (D-61).
func parseEnv(src string) (vars map[string]envVar, maybeURL bool, ds []diag.Diagnostic) {
	vars = map[string]envVar{}
	for i, line := range strings.Split(src, "\n") {
		pos := token.Position{Filename: ".env", Line: i + 1}
		key := "" // unknown until the line is split
		fail := func(msg, fix string) {
			ds = append(ds, diag.New(pos, msg, fix))
			maybeURL = maybeURL || key == "" || key == urlKey
		}
		line = strings.TrimSpace(line) // also drops the \r of CRLF
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			fail(".env lines cannot start with export", `remove "export"`)
			continue
		}
		k, value, ok := strings.Cut(line, "=")
		if !ok {
			fail("line without =", "write KEY=VALUE")
			continue
		}
		if k = strings.TrimSpace(k); !keyPattern.MatchString(k) {
			fail("invalid key", "use letters, digits and _ in keys, not starting with a digit")
			continue
		}
		key, value = k, strings.TrimSpace(value)
		if q := value[:min(1, len(value))]; q == `"` || q == "'" {
			end := strings.Index(value[1:], q) + 1
			if end == 0 {
				fail("unclosed quote in the value of "+key, "close the quote")
				continue
			}
			if end != len(value)-1 {
				fail("text after the closing quote in the value of "+key, "remove the trailing comment")
				continue
			}
			value = value[1:end]
		} else if strings.ContainsAny(value, " \t") {
			fail("space in the unquoted value of "+key, "quote the value or remove the trailing comment")
			continue
		}
		if first, ok := vars[key]; ok {
			fail(fmt.Sprintf("%s is set twice; first on line %d", key, first.line), "remove one of the lines")
			continue
		}
		vars[key] = envVar{value, i + 1}
	}
	return vars, maybeURL, ds
}
