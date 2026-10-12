package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

// CookieName is the name of the session cookie.
const CookieName = "session"

// tokenLen is the length of a token: 32 random bytes in base64url.
var tokenLen = base64.RawURLEncoding.EncodedLen(32)

// NewToken returns a new random token, for the cookie or a mail, and its
// hash, for the database (D-128): a stolen database holds no token.
func NewToken() (token, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken is the hash of token that the database holds.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SessionCookie is the cookie that carries token for maxAge, at path:
// maago.Prefix(), or "/" without one. A negative maxAge removes it.
func SessionCookie(token, path string, maxAge time.Duration) *http.Cookie {
	age := int(maxAge / time.Second)
	if maxAge < 0 {
		age = -1
	}
	return &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     path,
		MaxAge:   age,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}

// FromSession makes a maago.Config.Auth of session, which returns the
// user of a session's hash, and token, the user of an API token's hash
// (D-129). A request with Authorization: Bearer is the token's, whatever
// cookie it brings: a frontend on another origin may send a stale one.
// Without, it is the session cookie's. A nil token reads no header. A
// request whose token or cookie is not well-formed, or whose hash find
// does not have (sql.ErrNoRows: expired, removed), has no user.
func FromSession[U any](session, token func(ctx context.Context, hash string) (U, error)) func(*http.Request) (any, error) {
	return func(r *http.Request) (any, error) {
		find, value := session, ""
		if t, ok := bearer(r); ok && token != nil {
			find, value = token, t
		} else if c, err := r.Cookie(CookieName); err == nil {
			value = c.Value
		}
		if !wellFormed(value) {
			return nil, nil
		}
		u, err := find(r.Context(), HashToken(value))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, nil
		case err != nil:
			return nil, err
		}
		return u, nil
	}
}

// bearer is the token of r's Authorization: Bearer header, if it has
// one; a Bearer header without one token has the token "".
func bearer(r *http.Request) (string, bool) {
	f := strings.Fields(r.Header.Get("Authorization"))
	if len(f) == 0 || !strings.EqualFold(f[0], "Bearer") {
		return "", false
	}
	if len(f) != 2 {
		return "", true
	}
	return f[1], true
}

// wellFormed reports whether token can be one NewToken made.
func wellFormed(token string) bool {
	_, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return len(token) == tokenLen && err == nil
}

// SafeNext is next if it is a path of this site, else fallback (D-128):
// it starts with one /, not // or /\, which browsers read as another
// host, and holds no control character nor an encoded / or \.
func SafeNext(next, fallback string) string {
	lower := strings.ToLower(next)
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") ||
		strings.ContainsFunc(next, func(r rune) bool { return r < 0x20 || r == 0x7f }) ||
		strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
		return fallback
	}
	return next
}
