package auth

import (
	"strings"
	"testing"
)

// Made by Django 6.1.2's own hashers, not by a copy of their formula (the
// two 1,500,000-round salts were first drawn by make_password):
//
//	uv run --with django python -c 'import django; from django.conf import settings
//	settings.configure(PASSWORD_HASHERS=["django.contrib.auth.hashers.PBKDF2PasswordHasher",
//	    "django.contrib.auth.hashers.PBKDF2SHA1PasswordHasher"]); django.setup()
//	from django.contrib.auth.hashers import PBKDF2PasswordHasher as S, PBKDF2SHA1PasswordHasher as S1
//	print(S().encode("correct horse 말", "aYSIytyUCgzofJRmO1We2a", 1500000))
//	print(S1().encode("correct horse 말", "406thGJeuxyGk1uqPvzxpi", 1500000))
//	print(S().encode("low", "lowsalt1234567890ab", 1000))
//	print(S1().encode("low", "lowsalt1234567890ab", 1000))'
const (
	djangoSHA256  = "pbkdf2_sha256$1500000$aYSIytyUCgzofJRmO1We2a$fVfPGP8i+9vZx59N8U74hAJp7UQgNs2rcPL5YMXi2ZI="
	djangoSHA1    = "pbkdf2_sha1$1500000$406thGJeuxyGk1uqPvzxpi$qagStGZmc+ZqGLeNV/X1fOf0Gww="
	djangoLow     = "pbkdf2_sha256$1000$lowsalt1234567890ab$HpL7HLrngOx7jWOmHbbxpBf02g4/+jjnVGwF8t3MJvY="
	djangoLowSHA1 = "pbkdf2_sha1$1000$lowsalt1234567890ab$FCbA1J14Exq3OYBLrlM69kCJN9w="
)

// low sets Iterations for one test, as tests of an app do.
func low(t *testing.T, n int) {
	old := Iterations
	Iterations = n
	t.Cleanup(func() { Iterations = old })
}

func TestCheckDjango(t *testing.T) {
	low(t, 1000)
	for _, c := range []struct {
		encoded, password string
		ok, rehash        bool
	}{
		{djangoSHA256, "correct horse 말", true, true}, // 1,500,000 rounds, not 1000: redone, down as well (D-127)
		{djangoSHA1, "correct horse 말", true, true},
		{djangoLowSHA1, "low", true, true}, // the old algorithm at Iterations: redone for the algorithm alone
		{djangoLow, "low", true, false},    // this algorithm and these rounds
		{djangoSHA256, "correct horse", false, false},
		{djangoLow, "Low", false, false},
	} {
		if ok, rehash := Check(c.password, c.encoded); ok != c.ok || rehash != c.rehash {
			t.Errorf("Check(%q, %.20s…) = %v, %v; want %v, %v", c.password, c.encoded, ok, rehash, c.ok, c.rehash)
		}
	}
	Iterations = 2000 // more rounds than the hash's: redone too
	if ok, rehash := Check("low", djangoLow); !ok || !rehash {
		t.Errorf("fewer rounds than Iterations: %v, %v", ok, rehash)
	}
}

func TestHash(t *testing.T) {
	low(t, 1000)
	a, err := Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Hash("secret")
	if a == b || !strings.HasPrefix(a, "pbkdf2_sha256$1000$") {
		t.Errorf("%s %s: want new salts, Django's form", a, b)
	}
	if ok, rehash := Check("secret", a); !ok || rehash {
		t.Errorf("own hash: %v, %v", ok, rehash)
	}
	if ok, _ := Check("Secret", a); ok {
		t.Error("another password passed")
	}
}

// Whatever Check cannot read is false, never an error or a panic.
func TestCheckRefuses(t *testing.T) {
	low(t, 1000)
	for _, encoded := range []string{
		"",                         // no account
		"md5$1$salt$hash",          // an algorithm it does not read
		"pbkdf2_sha256$1000$salt",  // three fields
		"pbkdf2_sha256$abc$salt$x", // rounds not a number
		"pbkdf2_sha256$0$salt$x",
		"pbkdf2_sha256$-5$salt$x",
		"pbkdf2_sha256$99999999999999999999$salt$x",
		strings.Replace(djangoLow, "HpL7", "HpL8", 1), // the right form, the wrong key
	} {
		if ok, rehash := Check("low", encoded); ok || rehash {
			t.Errorf("%q: %v, %v", encoded, ok, rehash)
		}
	}
}

// Hashes that would check if a guard were missing: the refusal is the
// guard's, not a mismatch.
func TestCheckRefusesMatching(t *testing.T) {
	low(t, 1000)
	long := strings.Repeat("a", maxPassword+1)
	longPassword, _ := encode("pbkdf2_sha256", long, "salt", 1000)
	longSalt, _ := encode("pbkdf2_sha256", "low", strings.Repeat("s", maxEncoded), 1000)
	for _, c := range []struct{ why, password, encoded string }{
		// hashlib.pbkdf2_hmac("sha256", b"low", b"salt", 10000001), base64
		{"rounds over the bound", "low", "pbkdf2_sha256$10000001$salt$rpv1RHa3N/HMp8gtrBTxc7dW9QGq3d0j8X5X3BP07pk="},
		// hashlib.pbkdf2_hmac("sha256", b"low", b"", 1000), base64
		{"no salt", "low", "pbkdf2_sha256$1000$$4utlKOexHizNhCxSmh+0C4HA1YWZqWrMNzMQqW9xd4E="},
		{"password over 4096 bytes", long, longPassword},
		{"hash over 256 bytes", "low", longSalt},
	} {
		if ok, rehash := Check(c.password, c.encoded); ok || rehash {
			t.Errorf("%s: %v, %v", c.why, ok, rehash)
		}
	}
}

// 4096 bytes of UTF-8, as given: no normalizing, no counting of letters.
func TestLength(t *testing.T) {
	low(t, 1000)
	max := strings.Repeat("말", 1365) + "a" // 3*1365+1 = 4096 bytes
	h, err := Hash(max)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := Check(max, h); !ok {
		t.Error("4096 bytes did not check")
	}
	if _, err := Hash(max + "a"); err != ErrTooLong {
		t.Errorf("4097 bytes: %v", err)
	}
}

func TestIterationsOutOfRange(t *testing.T) {
	for _, n := range []int{0, maxIterations + 1} {
		func() {
			low(t, n)
			defer func() {
				if recover() == nil {
					t.Errorf("%d: no panic", n)
				}
			}()
			Hash("x")
		}()
	}
}

func TestPasswordRules(t *testing.T) {
	about := []string{"Ann.Lee@example.com"}
	rules := []PasswordRule{MinLength(8), NotCommon, NotSimilar}
	for _, c := range []struct {
		password string
		want     []string // the start of each error, in order
	}{
		{"correct horse battery", nil},
		{"가나다라마바사아", nil},                                   // 8 characters, 24 bytes
		{"가나다라마바사", []string{"This password is too short"}}, // 7
		{"Password", []string{"This password is too common"}},
		{"BASEBALL", []string{"This password is too common"}},
		{"deeznutz", []string{"This password is too common"}}, // the list's last
		{"my-ann-pass", []string{"This password is too similar"}},
		{"leeway forever", []string{"This password is too similar"}},
		{"example-pass", nil}, // the domain does not count
		{"welcome home", nil},
		{"an-le-xx-ok", nil}, // pieces under 3 characters are no words
		{"ann", []string{"This password is too short", "This password is too similar"}},
		{strings.Repeat("a", 4097), []string{"This password is too long"}},
	} {
		err := ValidatePassword(c.password, about, rules...)
		var got []string
		if err != nil {
			got = strings.Split(err.Error(), "\n")
		}
		ok := len(got) == len(c.want)
		for i := 0; ok && i < len(got); i++ {
			ok = strings.HasPrefix(got[i], c.want[i])
		}
		if !ok {
			t.Errorf("%.20q: %v, want %v", c.password, got, c.want)
		}
	}
	if err := ValidatePassword("short", nil); err != nil {
		t.Errorf("no rules: %v", err)
	}
}
