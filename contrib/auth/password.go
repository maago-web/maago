// Package auth is the security half of the auth app that maago add copies
// into a project: the parts that must be right, here so that go
// get -u fixes them for every project. It knows no user type and
// uses the standard library only.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Iterations is the PBKDF2 rounds of a new hash. Tests and the
// tutorial lower it before they serve anything; main leaves it alone. A
// hash with other rounds is redone at its next sign-in. Hash and Check
// panic when it is outside 1 to 10,000,000.
var Iterations = 600_000

// maxIterations bounds the rounds a stored hash may ask for: past it the
// hash is broken, so that a hash cannot make every sign-in slow.
const maxIterations = 10_000_000

// maxPassword is the longest password in bytes, UTF-8 as given;
// maxEncoded the longest stored hash Check reads.
const maxPassword, maxEncoded = 4096, 256

// ErrTooLong means the password is longer than 4096 bytes.
var ErrTooLong = errors.New("auth: password longer than 4096 bytes")

// hashers are the PBKDF2 hashes as Django writes them,
// name$rounds$salt$base64, by name.
var hashers = map[string]func() hash.Hash{
	"pbkdf2_sha256": sha256.New,
	"pbkdf2_sha1":   sha1.New, // read only: Django's old hasher
}

// dummySalt is the salt of the work Check does when there is no hash to
// check against.
const dummySalt = "maagodummysaltmaagodummy"

// Hash hashes password for storing: pbkdf2_sha256 with Iterations rounds
// and a new salt.
func Hash(password string) (string, error) {
	if len(password) > maxPassword {
		return "", ErrTooLong
	}
	return encode("pbkdf2_sha256", password, rand.Text(), rounds())
}

// Check reports whether password is the one encoded hashes, and if so
// whether the hash should be redone: another algorithm, or rounds other
// than Iterations. An empty encoded (no account, or no usable password)
// and one that Check does not read cost the work of a current hash and
// are false. A hash with other rounds or the old algorithm costs its own
// work until its first sign-in redoes it: until then its account answers
// in another time than no account.
func Check(password, encoded string) (ok, rehash bool) {
	if len(password) > maxPassword {
		return false, false // the caller's own input: nothing to hide
	}
	name, salt, n, read := parse(encoded)
	if !read {
		encode("pbkdf2_sha256", password, dummySalt, rounds())
		return false, false
	}
	again, err := encode(name, password, salt, n)
	if err != nil || subtle.ConstantTimeCompare([]byte(again), []byte(encoded)) != 1 {
		return false, false
	}
	return true, name != "pbkdf2_sha256" || n != Iterations
}

// parse reads name$rounds$salt$hash, as Django's decode does, with the
// rounds and the length bounded.
func parse(encoded string) (name, salt string, n int, ok bool) {
	f := strings.SplitN(encoded, "$", 4)
	if len(encoded) > maxEncoded || len(f) != 4 || f[2] == "" || hashers[f[0]] == nil {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(f[1])
	if err != nil || n < 1 || n > maxIterations {
		return "", "", 0, false
	}
	return f[0], f[2], n, true
}

// encode derives a key as long as the digest and writes it Django's way.
func encode(name, password, salt string, n int) (string, error) {
	h := hashers[name]
	key, err := pbkdf2.Key(h, password, []byte(salt), n, h().Size())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s$%d$%s$%s", name, n, salt, base64.StdEncoding.EncodeToString(key)), nil
}

func rounds() int {
	if Iterations < 1 || Iterations > maxIterations {
		panic("auth: Iterations is outside 1 to 10,000,000")
	}
	return Iterations
}

// PasswordRule checks a new password, with about the user's own words that
// it must not be like: the email, and what a project adds.
type PasswordRule func(password string, about []string) error

// ValidatePassword checks password against rules, and returns the errors
// of all that fail, joined. A password over 4096 bytes always fails, as
// Hash would.
func ValidatePassword(password string, about []string, rules ...PasswordRule) error {
	if len(password) > maxPassword {
		return errors.New("This password is too long. It must be 4096 bytes or fewer.")
	}
	var errs []error
	for _, r := range rules {
		errs = append(errs, r(password, about))
	}
	return errors.Join(errs...)
}

// MinLength fails a password of fewer than n characters.
func MinLength(n int) PasswordRule {
	return func(password string, about []string) error {
		if utf8.RuneCountInString(password) < n {
			return fmt.Errorf("This password is too short. It must contain at least %d characters.", n)
		}
		return nil
	}
}

// common is common.txt: one password a line, after its # lines.
//
//go:embed common.txt
var common string

// NotCommon fails a password on the list of 1,000 common ones in
// common.txt, from SecLists (MIT; the file says how they were picked),
// case aside. The list holds those of 8 characters or more: with a
// shorter MinLength, short common ones pass.
//
// ponytail: 1,000 passwords, not Django's 20,000.
func NotCommon(password string, about []string) error {
	if strings.Contains(common, "\n"+strings.ToLower(password)+"\n") {
		return errors.New("This password is too common.")
	}
	return nil
}

// NotSimilar fails a password that holds a word of about, case aside: a
// run of 3 or more letters or digits, such as ann of ann.lee@example.com.
// Of an email only the part before @ counts: every .com would ban com.
//
// ponytail: words held, not Django's similarity ratio.
func NotSimilar(password string, about []string) error {
	pw := strings.ToLower(password)
	for _, a := range about {
		if i := strings.LastIndex(a, "@"); i >= 0 {
			a = a[:i]
		}
		for _, w := range strings.FieldsFunc(strings.ToLower(a), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if utf8.RuneCountInString(w) >= 3 && strings.Contains(pw, w) {
				return errors.New("This password is too similar to your email.")
			}
		}
	}
	return nil
}
