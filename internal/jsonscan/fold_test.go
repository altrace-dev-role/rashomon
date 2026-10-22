package jsonscan

import (
	"encoding/json"
	"strings"
	"testing"
)

// structNames is what decoding into a struct with these two fields gives:
// the oracle every fold test below compares with.
type structNames struct {
	SessionID string `json:"session_id"`
	Message   string `json:"last_assistant_message"`
}

var foldKeys = NewFoldKeySet("session_id", "last_assistant_message")

// TestRES5_FoldKeySetMatchesStructFieldNames: a fold KeySet finds a key
// wherever decoding into a struct field of that name finds it, in any case,
// under Unicode simple folding, the last match winning. Break: compare keys
// exactly, and every case variant is lost.
func TestRES5_FoldKeySetMatchesStructFieldNames(t *testing.T) {
	for _, in := range []string{
		`{"Last_Assistant_Message":"hi","SESSION_ID":"s"}`,
		`{"session_id":"a","SESSION_ID":"b","Session_Id":"c"}`,
		`{"SESSION_ID":"b","session_id":"a"}`,
		"{\"ſession_id\":\"long s\"}",
		`{"\u017fession_id":"escaped long s","LAST_assistant_MESSAGE":"m"}`,
		`{"session_idx":"no","session_i":"no"}`,
	} {
		var want structNames
		if err := json.Unmarshal([]byte(in), &want); err != nil {
			t.Fatalf("%s: oracle: %v", in, err)
		}
		out := make([]Span, foldKeys.Len())
		if err := Top([]byte(in), foldKeys, out); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		got := structNames{SessionID: headString(t, in, out[0]), Message: headString(t, in, out[1])}
		if got != want {
			t.Errorf("%s: got %+v, want %+v as a struct decode gives", in, got, want)
		}
	}
}

// TestRES5_FoldKeySetMatchesALongerEncodedKey: a folded character can take
// more bytes than the one it matches (U+212A KELVIN SIGN is three bytes and
// matches k), so a key is not dropped for outrunning the wanted key's length.
func TestRES5_FoldKeySetMatchesALongerEncodedKey(t *testing.T) {
	keys := NewFoldKeySet("kk")
	in := "{\"\u212a\u212a\":\"kelvin\"}" // KELVIN SIGN twice, as UTF-8
	var want struct {
		KK string `json:"kk"`
	}
	if err := json.Unmarshal([]byte(in), &want); err != nil || want.KK != "kelvin" {
		t.Fatalf("oracle: %+v %v", want, err)
	}
	for _, p := range []string{in, `{"\u212A\u212a":"kelvin"}`} {
		out := make([]Span, 1)
		if err := Top([]byte(p), keys, out); err != nil {
			t.Fatal(err)
		}
		if got := headString(t, p, out[0]); got != "kelvin" {
			t.Errorf("%s: got %q, want kelvin", p, got)
		}
	}
}

// TestRES5_StreamFoldPathMatchesStructFieldNames is the same rule for a
// streamed path that sets Fold; a path without it still matches exactly.
func TestRES5_StreamFoldPathMatchesStructFieldNames(t *testing.T) {
	in := `{"SESSION_ID":"s","Last_Assistant_Message":"hi","session_ID":"t"}`
	paths := []Path{
		{Elems: []string{"session_id"}, Max: 64, Mode: CaptureHead, Fold: true},
		{Elems: []string{"last_assistant_message"}, Max: 64, Mode: CaptureHead, Fold: true},
		{Elems: []string{"session_id"}, Max: 64, Mode: CaptureHead},
	}
	out, err := stream(t, in, paths, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []got{{0, "s", false}, {1, "hi", false}, {0, "t", false}}
	if len(out) != len(want) {
		t.Fatalf("got %+v, want %+v", out, want)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, out[i], want[i])
		}
	}
}

func headString(t *testing.T, buf string, s Span) string {
	t.Helper()
	if s.Kind != String {
		return ""
	}
	out, _, err := Head([]byte(buf), s, 3*int(s.Len), nil)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Clone(string(out))
}
