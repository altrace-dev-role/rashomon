package shape

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"strings"
	"testing"
)

// sink is a hash.Hash that keeps what is written, so canonical's stream can
// be compared with json.Marshal's bytes.
type sink struct{ bytes.Buffer }

func (s *sink) Sum(b []byte) []byte { return append(b, s.Bytes()...) }
func (s *sink) Size() int           { return 0 }
func (s *sink) BlockSize() int      { return 1 }

var _ hash.Hash = (*sink)(nil)

var canonSeeds = []string{
	`{"b":1,"a":[true,false,null,"x"],"c":{"z":"<&>","y":"\u2028\u2029\b\f\n\r\t\u0001\u007f"}}`,
	`{"n":[0,-0,1.5,1e21,1e-7,123456789012345678,0.000001,-1e-6,1E+2,2.5e-10]}`,
	"{\"s\":\"\xff\xfe\",\"\xc3\xa9\":\"\\ud800\",\"\U0001F600\":\"\\ud83d\\ude00\",\"\":\"\"}",
	`[]`, `{}`, `"x"`, `3`, `null`, `{"a":1,"a":2}`, ` { "k" : [ 1 , { "j" : "v" } ] } `,
}

func checkCanonical(t *testing.T, raw []byte) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		var s sink
		if canonical(raw, &s) || s.Len() != 0 {
			t.Fatalf("canonical accepted input json.Unmarshal rejects: %q", raw)
		}
		return
	}
	want, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var s sink
	if !canonical(raw, &s) || !bytes.Equal(s.Bytes(), want) {
		t.Fatalf("canonical(%q)\n got %q\nwant %q", raw, s.Bytes(), want)
	}
}

// TestCanonicalMatchesMarshal: the streamed canonical form is json.Marshal's,
// byte for byte, so every digest stays what it was.
func TestCanonicalMatchesMarshal(t *testing.T) {
	for _, in := range canonSeeds {
		checkCanonical(t, []byte(in))
	}
	big := `{"new_string":"` + strings.Repeat(`line <a & b>\n`, 1<<14) + `","file_path":"/x"}`
	checkCanonical(t, []byte(big))
}

// TestCanonicalDigestIsUnchanged pins the digest of an input to the value the
// Marshal-based canonical gave.
func TestCanonicalDigestIsUnchanged(t *testing.T) {
	key := []byte("k")
	in := json.RawMessage(`{"old_string":"a<b","file_path":"/p","new_string":"x\ny"}`)
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte("Edit\x00"))
	m.Write(body)
	if got, want := canonicalDigest(key, "Edit", in), hex.EncodeToString(m.Sum(nil)); got != want {
		t.Fatalf("digest moved: %s, was %s", got, want)
	}
	if got, want := canonicalDigest(key, "Edit", json.RawMessage(`{bad`)), digest(key, "Edit", []byte(`{bad`)); got != want {
		t.Fatalf("invalid input digest %s, want %s", got, want)
	}
}

func FuzzCanonicalMatchesMarshal(f *testing.F) {
	for _, s := range canonSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) { checkCanonical(t, raw) })
}
