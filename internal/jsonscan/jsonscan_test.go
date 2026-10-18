package jsonscan

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

var testKeys = NewKeySet("session_id", "tool_input", "a", "missing", "n")

func top(t *testing.T, payload string) []Span {
	t.Helper()
	out := make([]Span, testKeys.Len())
	if err := Top([]byte(payload), testKeys, out); err != nil {
		t.Fatalf("Top(%q): %v", payload, err)
	}
	return out
}

func TestTopFindsKeysAndTheLastDuplicateWins(t *testing.T) {
	p := `{"a":"first", "session_id" : "s-1","tool_input":{"command":"go test"},"a":"last","n":null}`
	out := top(t, p)
	want := map[int]string{0: `"s-1"`, 1: `{"command":"go test"}`, 2: `"last"`, 4: `null`}
	kinds := map[int]Kind{0: String, 1: Object, 2: String, 4: Null}
	for i, w := range want {
		if got := string(out[i].Raw([]byte(p))); got != w || out[i].Kind != kinds[i] {
			t.Errorf("slot %d = %q kind %d, want %q kind %d", i, got, out[i].Kind, w, kinds[i])
		}
	}
	if out[3].Present() {
		t.Errorf("missing key reported present: %+v", out[3])
	}
}

func TestTopMatchesAnEscapedKey(t *testing.T) {
	out := top(t, `{"session_id":"s"}`)
	if out[0].Kind != String {
		t.Fatalf("escaped key not matched: %+v", out[0])
	}
}

// A NewKeySet matches keys exactly after unescaping, as a map decode does;
// the struct-field match is NewFoldKeySet's (fold_test.go).
func TestTopDoesNotFoldKeyCase(t *testing.T) {
	if out := top(t, `{"SESSION_ID":"s","Session_Id":"t"}`); out[0].Present() {
		t.Fatalf("SESSION_ID matched session_id: %+v", out[0])
	}
}

func TestTopNullIsAllAbsent(t *testing.T) {
	for _, s := range top(t, " null ") {
		if s.Present() {
			t.Fatalf("null payload produced a span: %+v", s)
		}
	}
}

func TestTopRejectsInvalidAndNonObjects(t *testing.T) {
	out := make([]Span, testKeys.Len())
	for _, c := range []struct {
		in   string
		want error
	}{
		{`{"a":`, ErrInvalid},
		{``, ErrInvalid},
		{`{"a":1}x`, ErrInvalid},
		{`[1]`, ErrNotObject},
		{`"s"`, ErrNotObject},
	} {
		if err := Top([]byte(c.in), testKeys, out); !errors.Is(err, c.want) {
			t.Errorf("Top(%q) = %v, want %v", c.in, err, c.want)
		}
	}
}

// TestRES5_TopDefersToJSONValid: the walker never decides validity, so
// input json.Valid rejects is rejected even where the walk alone would find
// its keys. Break: skip json.Valid in Top, and these are accepted.
func TestRES5_TopDefersToJSONValid(t *testing.T) {
	out := make([]Span, testKeys.Len())
	for _, in := range []string{`{"a":"\u12"}`, `{"a":1}{"a":2}`, `{"a":tru}`, `{"a":01}`, "{\"a\":\"\x01\"}", `{"a":1,}`} {
		if err := Top([]byte(in), testKeys, out); !errors.Is(err, ErrInvalid) {
			t.Errorf("Top(%q) = %v, want ErrInvalid", in, err)
		}
	}
}

// TestTopAllocatesNothingPerValue measures the walk, not Top as a whole: Top
// validates with encoding/json's Valid before walking, and Valid's scanner
// comes from a package-level sync.Pool (encoding/json/scanner.go). Under
// -race that pool's Put drops its argument on about 1 in 4 calls on purpose
// ("Randomly drop x on floor", sync/pool.go) so the race detector can catch
// Pool misuse, and under testing.AllocsPerRun's forced GOMAXPROCS(1) plus
// real CPU contention (a shared CI runner) that was measured driving 48-68
// of 50 Top calls to a fresh allocation - not jsonscan's doing, since
// measuring the walk alone over the same payload, under the same contended
// -race build, held at 0 of 50. Field walks the same code Top does
// (walkObject) once a buffer is already known valid, so it isolates that
// part without going through Valid a second time.
func TestTopAllocatesNothingPerValue(t *testing.T) {
	p := []byte(`{"session_id":"s","tool_input":{"command":"` + strings.Repeat("x\\n", 4096) + `"},"a":[1,2,{"b":"c"}],"z":true}`)
	whole := Span{Off: 0, Len: int32(len(p)), Kind: Object}
	out := make([]Span, testKeys.Len())
	if n := testing.AllocsPerRun(50, func() {
		if err := Field(p, whole, testKeys, out); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Fatalf("the walk allocated %.1f times per run, want 0", n)
	}
	// Top adds json.Valid to the walk. Valid borrows a scanner from a
	// sync.Pool, which keeps it warm without -race, so Top as a whole
	// allocates nothing there; under -race the pool drops values at random.
	if raceEnabled {
		return
	}
	if n := testing.AllocsPerRun(50, func() {
		if err := Top(p, testKeys, out); err != nil {
			t.Fatal(err)
		}
	}); n != 0 {
		t.Fatalf("Top allocated %.1f times per run, want 0", n)
	}
}

func TestFieldReadsInsideAnObjectSpan(t *testing.T) {
	p := []byte(`{"tool_input":{"a":"x","n":2,"session_id":{"deep":1}},"a":"outer"}`)
	out := top(t, string(p))
	in := make([]Span, testKeys.Len())
	if err := Field(p, out[1], testKeys, in); err != nil {
		t.Fatal(err)
	}
	if got := string(in[2].Raw(p)); got != `"x"` {
		t.Errorf("tool_input.a = %q", got)
	}
	if got := string(in[0].Raw(p)); got != `{"deep":1}` {
		t.Errorf("tool_input.session_id = %q", got)
	}
	if err := Field(p, out[2], testKeys, in); !errors.Is(err, ErrNotObject) {
		t.Errorf("Field over a string = %v, want ErrNotObject", err)
	}
	if err := Field(p, Span{Off: 1 << 20, Len: 4, Kind: Object}, testKeys, in); !errors.Is(err, ErrSpan) {
		t.Errorf("Field over a span outside the buffer = %v, want ErrSpan", err)
	}
}

func stringSpan(t *testing.T, value string) ([]byte, Span) {
	t.Helper()
	p := []byte(`{"a":` + value + `}`)
	return p, top(t, string(p))[2]
}

func TestHeadAndTailUnescapeWithoutSplittingACharacter(t *testing.T) {
	p, s := stringSpan(t, `"ab\né😀z\"q"`) // decodes to "ab\né😀z\"q", 12 bytes
	full := "ab\né\U0001F600z\"q"
	out, tr, err := Head(p, s, 64, make([]byte, 0, 64))
	if err != nil || tr || string(out) != full {
		t.Fatalf("Head whole = %q %v %v", out, tr, err)
	}
	out, tr, err = Head(p, s, 6, make([]byte, 0, 6)) // the emoji does not fit after "ab\né"
	if string(out) != "ab\né" || !tr || err != nil {
		t.Fatalf("Head 6 = %q %v %v", out, tr, err)
	}
	out, tr, err = Tail(p, s, 64, make([]byte, 64))
	if string(out) != full || tr || err != nil {
		t.Fatalf("Tail whole = %q %v %v", out, tr, err)
	}
	out, tr, err = Tail(p, s, 6, make([]byte, 6)) // the last 6 bytes start inside the emoji
	if string(out) != "z\"q" || !tr || err != nil {
		t.Fatalf("Tail 6 = %q %v %v", out, tr, err)
	}
	if _, _, err := Head(p, top(t, `{"a":1}`)[2], 4, nil); !errors.Is(err, ErrNotString) {
		t.Fatalf("Head of a number = %v", err)
	}
}

func TestHeadMatchesEncodingJSONOnBrokenText(t *testing.T) {
	p, s := stringSpan(t, "\"\\ud800\\u0041\xff\\udc00\"")
	out, _, err := Head(p, s, 64, nil)
	if err != nil || string(out) != "�A��" {
		t.Fatalf("Head = %q %v", out, err)
	}
}

func collectLines(t *testing.T, p []byte, s Span, work int) []string {
	t.Helper()
	var lines []string
	var cur bytes.Buffer
	if err := Lines(p, s, make([]byte, work), func(line []byte, more bool) bool {
		cur.Write(line)
		if !more {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestLinesSplitsOnNewlinesAndPassesLongLinesInPieces(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{`"a\nbb\n\nccc"`, []string{"a", "bb", "", "ccc"}},
		{`"a\n"`, []string{"a"}},
		{`""`, nil},
		{`"\n"`, []string{""}},
		{`"x\u000ay"`, []string{"x", "y"}},
	} {
		p, s := stringSpan(t, c.in)
		if got := collectLines(t, p, s, 64); strings.Join(got, "|") != strings.Join(c.want, "|") || len(got) != len(c.want) {
			t.Errorf("Lines(%s) = %q, want %q", c.in, got, c.want)
		}
	}

	p, s := stringSpan(t, `"abcdefghij\nk"`)
	var pieces []string
	if err := Lines(p, s, make([]byte, 4), func(line []byte, more bool) bool {
		pieces = append(pieces, string(line)+map[bool]string{true: "+", false: "."}[more])
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(pieces, " "); got != "abcd+ efgh+ ij. k." {
		t.Fatalf("pieces = %q", got)
	}
}

func TestLinesStopsWhenTheCallbackSaysSo(t *testing.T) {
	p, s := stringSpan(t, `"a\nb\nc"`)
	n := 0
	if err := Lines(p, s, make([]byte, 8), func([]byte, bool) bool { n++; return false }); err != nil || n != 1 {
		t.Fatalf("Lines after a false return: n=%d err=%v", n, err)
	}
}
