package maago

import (
	"encoding/json"
	"testing"
	"time"
)

func TestUUID(t *testing.T) {
	a := NewUUID()
	time.Sleep(2 * time.Millisecond)
	b := NewUUID()
	if a[6]>>4 != 7 || a[8]>>6 != 2 {
		t.Errorf("%s: not version 7, variant 10", a)
	}
	if a.String() >= b.String() {
		t.Errorf("%s, made first, does not sort before %s", a, b)
	}
	if ms := time.Now().UnixMilli() - int64(a[0])<<40 - int64(a[1])<<32 - int64(a[2])<<24 - int64(a[3])<<16 - int64(a[4])<<8 - int64(a[5]); ms < 0 || ms > 1000 {
		t.Errorf("%s: its time is %d ms from now", a, ms)
	}
	for _, s := range []string{"", "0190a5c8-0000-7000-8000-00000000000", "0190a5c8-0000-7000-8000-00000000000fa", "0190a5c800000-7000-8000-00000000000f", "0190a5c8-0000-7000-8000-00000000000z", "{0190a5c8-0000-7000-8000-00000000000f}"} {
		if _, err := ParseUUID(s); err == nil {
			t.Errorf("ParseUUID(%q) took it", s)
		}
	}
	u, err := ParseUUID("0190A5C8-0000-7000-8000-00000000000F")
	if err != nil || u.String() != "0190a5c8-0000-7000-8000-00000000000f" {
		t.Fatalf("ParseUUID: %s %v", u, err)
	}
	v, _ := u.Value()
	for _, src := range []any{v, []byte(v.(string))} {
		var back UUID
		if err := back.Scan(src); err != nil || back != u {
			t.Errorf("Scan(%T): %s %v", src, back, err)
		}
	}
	if err := new(UUID).Scan(int64(1)); err == nil {
		t.Error("Scan took an int64")
	}
	b2, _ := json.Marshal(struct{ K UUID }{u})
	if string(b2) != `{"K":"0190a5c8-0000-7000-8000-00000000000f"}` {
		t.Errorf("JSON: %s", b2)
	}
	var in struct{ K UUID }
	if err := json.Unmarshal(b2, &in); err != nil || in.K != u {
		t.Errorf("from JSON: %s %v", in.K, err)
	}
}

func TestJSON(t *testing.T) {
	type meta struct {
		W    int      `json:"w"`
		Tags []string `json:"tags"`
	}
	j := JSON[meta]{meta{3, []string{"a"}}}
	v, err := j.Value()
	if err != nil || v != `{"w":3,"tags":["a"]}` {
		t.Fatalf("Value: %v %v", v, err)
	}
	for _, src := range []any{`{"tags": ["a"], "w": 3}`, []byte(`{"w":3,"tags":["a"]}`)} {
		var back JSON[meta]
		if err := back.Scan(src); err != nil || back.String() != v {
			t.Errorf("Scan(%T): %s %v", src, back.String(), err)
		}
	}
	if err := new(JSON[meta]).Scan(1); err == nil {
		t.Error("Scan took an int")
	}
	b, _ := json.Marshal(struct{ M JSON[meta] }{j})
	if string(b) != `{"M":{"w":3,"tags":["a"]}}` {
		t.Errorf("JSON: %s", b)
	}
	var in struct{ M JSON[meta] }
	if err := json.Unmarshal(b, &in); err != nil || in.M.V.W != 3 {
		t.Errorf("from JSON: %+v %v", in, err)
	}
	var none *JSON[meta]
	if none.String() != "" || j.String() != v {
		t.Error("String")
	}
}
