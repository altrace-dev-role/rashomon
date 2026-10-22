package jsonscan

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

type got struct {
	path      int
	value     string
	truncated bool
}

func stream(t *testing.T, in string, paths []Path, oneByte bool) ([]got, error) {
	t.Helper()
	var r io.Reader = strings.NewReader(in)
	if oneByte {
		r = iotest.OneByteReader(r)
	}
	var out []got
	n, err := Stream(r, paths, func(i int, v []byte, tr bool) bool {
		out = append(out, got{i, string(v), tr})
		return true
	})
	if err == nil && n != int64(len(in)) {
		t.Fatalf("read %d of %d bytes", n, len(in))
	}
	return out, err
}

// TestRES7_StreamCopiesOnlyWantedPaths: Stream copies the values at the
// wanted paths and nothing else, across any window boundary. Break: match a
// path without comparing its key, and other values are copied.
func TestRES7_StreamCopiesOnlyWantedPaths(t *testing.T) {
	in := `{"session_id":"s-1","tool_calls":[{"tool_use_id":"t1","tool_name":"Bash"},{"tool_use_id":"t2"}],` +
		`"last_assistant_message":"` + strings.Repeat("a", 40) + `\n` + strings.Repeat("b", 40) + `","x":null,"n":-1.5e3}`
	paths := []Path{
		{Elems: []string{"session_id"}, Max: 64, Mode: CaptureHead},
		{Elems: []string{"tool_calls", "[]", "tool_use_id"}, Max: 64, Mode: CaptureHead},
		{Elems: []string{"last_assistant_message"}, Max: 16, Mode: CaptureHeadTail},
		{Elems: []string{"x"}, Max: 8, Mode: CaptureHead},
		{Elems: []string{"n"}, Max: 8, Mode: CaptureHead},
		{Elems: []string{"tool_calls"}, Mode: CapturePresence},
		{Elems: []string{"last_assistant_message"}, Max: 32, Mode: CaptureLines},
	}
	want := []got{
		{0, "s-1", false},
		{5, "", false},
		{1, "t1", false},
		{1, "t2", false},
		{2, strings.Repeat("a", 16), true},
		{2, strings.Repeat("b", 16), true},
		{6, strings.Repeat("a", 32), true},
		{6, strings.Repeat("a", 8), false},
		{6, strings.Repeat("b", 32), true},
		{6, strings.Repeat("b", 8), false},
		{4, "-1.5e3", false},
	}
	for _, oneByte := range []bool{false, true} {
		out, err := stream(t, in, paths, oneByte)
		if err != nil {
			t.Fatal(err)
		}
		// HeadTail is delivered when the value ends, after its Lines pieces.
		if len(out) != len(want) {
			t.Fatalf("oneByte=%v: got %d calls %+v, want %d", oneByte, len(out), out, len(want))
		}
		seen := map[got]int{}
		for _, g := range out {
			seen[g]++
		}
		for _, w := range want {
			if seen[w] == 0 {
				t.Errorf("oneByte=%v: missing %+v in %+v", oneByte, w, out)
			}
			seen[w]--
		}
	}
}

func TestStreamHeadTailFitsInOnePiece(t *testing.T) {
	out, err := stream(t, `{"m":"0123456789abcdef"}`, []Path{{Elems: []string{"m"}, Max: 10, Mode: CaptureHeadTail}}, true)
	if err != nil || len(out) != 1 || out[0].value != "0123456789abcdef" || out[0].truncated {
		t.Fatalf("got %+v %v", out, err)
	}
}

func TestStreamCapturesAContainerAsItsText(t *testing.T) {
	out, err := stream(t, `{"o":{"a": [1, "x"]} , "p":true}`, []Path{
		{Elems: []string{"o"}, Max: 64, Mode: CaptureHead},
		{Elems: []string{"o", "a", "[]"}, Max: 64, Mode: CaptureTail},
		{Elems: []string{"p"}, Max: 64, Mode: CaptureHead},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []got{{1, "1", false}, {1, "x", false}, {0, `{"a": [1, "x"]}`, false}, {2, "true", false}}
	if len(out) != len(want) {
		t.Fatalf("got %+v", out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("call %d = %+v, want %+v", i, out[i], want[i])
		}
	}
}

func TestStreamReportsTheFirstSyntaxErrorWithItsOffset(t *testing.T) {
	for _, c := range []struct {
		in     string
		offset int64
	}{
		{`{"a":tru}`, 5},
		{`{"a" 1}`, 5},
		{`{"a":"x` + "\x01" + `"}`, 7},
		{`[1,]`, 3},
		{`{"a":01}`, 6},
		{`{"a":"\q"}`, 6},
		{`{} {}x`, 5},
		{`1 2`, -1},
		{`{}{}`, 2},
		{``, 0},
		{`{"a":`, 5},
	} {
		_, err := stream(t, c.in, nil, true)
		var se *SyntaxError
		switch {
		case c.offset < 0 && err != nil:
			t.Errorf("Stream(%q) = %v, want nil", c.in, err)
		case c.offset >= 0 && !errors.As(err, &se):
			t.Errorf("Stream(%q) = %v, want a SyntaxError", c.in, err)
		case c.offset >= 0 && se.Offset != c.offset:
			t.Errorf("Stream(%q) offset %d, want %d (%v)", c.in, se.Offset, c.offset, err)
		}
	}
}

func TestStreamDepthMatchesEncodingJSON(t *testing.T) {
	ok := strings.Repeat("[", maxDepth) + strings.Repeat("]", maxDepth)
	if !json.Valid([]byte(ok)) || json.Valid([]byte("["+ok+"]")) {
		t.Fatalf("encoding/json's nesting limit is not %d", maxDepth)
	}
	if _, err := stream(t, ok, nil, false); err != nil {
		t.Fatalf("depth %d: %v", maxDepth, err)
	}
	deep := "[" + ok + "]"
	if _, err := stream(t, deep, nil, false); err == nil {
		t.Fatalf("depth %d accepted", maxDepth+1)
	}
}

func TestStreamStopsWhenTheCallbackSaysSo(t *testing.T) {
	n := 0
	_, err := Stream(strings.NewReader(`{"a":"1","a":"2"} garbage`), []Path{{Elems: []string{"a"}, Max: 4, Mode: CaptureHead}},
		func(int, []byte, bool) bool { n++; return false })
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestStreamPassesReadErrors(t *testing.T) {
	boom := errors.New("boom")
	_, err := Stream(io.MultiReader(strings.NewReader(`{"a":`), iotest.ErrReader(boom)), nil, func(int, []byte, bool) bool { return true })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
