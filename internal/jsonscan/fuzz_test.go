package jsonscan

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"
)

var fuzzSeeds = []string{
	`{"session_id":"s","tool_input":{"command":"go test ./..."},"a":"x\ny","n":1}`,
	`{"a":"😀é\\\"","a":[1,{"b":null}],"tool_input":"\ud800"}`,
	"{\"a\":\"\xff\xfe\",\"n\":-0.5e+7,\"missing\":false}",
	`{"c":[{"x":1},"s",null,2],"d":{"e":"0123456789abcdefghij"},"d":{"e":null}}`,
	`null`, `[]`, `{}`, `{"a":}`, `{"a":"\u12"}`, `{"a":1}{"a":2}`, `{"a":1} {"a":2}`, `{"a":"x\ry"} `,
}

// decodeString is the oracle's view of a string span.
func decodeString(t *testing.T, raw []byte) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("oracle cannot decode %q: %v", raw, err)
	}
	return s
}

func FuzzRES5_TopMatchesStdlib(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add([]byte(s))
	}
	keys := []string{"session_id", "tool_input", "a", "missing", "n"}
	f.Fuzz(func(t *testing.T, data []byte) {
		out := make([]Span, testKeys.Len())
		err := Top(data, testKeys, out)
		if !json.Valid(data) {
			if err == nil {
				t.Fatalf("Top accepted invalid JSON %q", data)
			}
			return
		}
		var m map[string]json.RawMessage
		if uerr := json.Unmarshal(data, &m); uerr != nil {
			if !errors.Is(err, ErrNotObject) {
				t.Fatalf("Top(%q) = %v, oracle %v", data, err, uerr)
			}
			return
		}
		if err != nil {
			t.Fatalf("Top(%q) = %v, oracle accepts it", data, err)
		}
		for i, k := range keys {
			raw, ok := m[k]
			if ok != out[i].Present() || !bytes.Equal(raw, out[i].Raw(data)) {
				t.Fatalf("key %q: span %q present=%v, oracle %q present=%v", k, out[i].Raw(data), out[i].Present(), raw, ok)
			}
			if out[i].Kind == String {
				checkStringReaders(t, data, out[i], decodeString(t, raw))
			}
		}
	})
}

// checkStringReaders holds Head, Tail and Lines to the oracle's decoding.
func checkStringReaders(t *testing.T, data []byte, s Span, want string) {
	t.Helper()
	big := len(want) + 8
	if h, tr, err := Head(data, s, big, make([]byte, 0, big)); err != nil || tr || string(h) != want {
		t.Fatalf("Head = %q %v %v, want %q", h, tr, err, want)
	}
	if tl, tr, err := Tail(data, s, big, make([]byte, big)); err != nil || tr || string(tl) != want {
		t.Fatalf("Tail = %q %v %v, want %q", tl, tr, err, want)
	}
	const small = 5
	h, tr, _ := Head(data, s, small, make([]byte, 0, small))
	if !strings.HasPrefix(want, string(h)) || len(h) > small || tr != (len(h) < len(want)) {
		t.Fatalf("Head %d = %q %v of %q", small, h, tr, want)
	}
	tl, tr, _ := Tail(data, s, small, make([]byte, small))
	if !strings.HasSuffix(want, string(tl)) || len(tl) > small || tr != (len(want) > small) {
		t.Fatalf("Tail %d = %q %v of %q", small, tl, tr, want)
	}
	var lines []string
	var cur []byte
	if err := Lines(data, s, make([]byte, 7), func(l []byte, more bool) bool {
		cur = append(cur, l...)
		if !more {
			lines, cur = append(lines, string(cur)), cur[:0]
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(lines, "\n"); joined != strings.TrimSuffix(want, "\n") || (want != "" && len(lines) == 0) {
		t.Fatalf("Lines = %q, want %q", lines, want)
	}
}

var fuzzPaths = []Path{
	{Elems: []string{"a"}, Max: 1 << 20, Mode: CaptureHead},
	{Elems: []string{"a"}, Mode: CapturePresence},
	{Elems: []string{"n"}, Max: 1 << 20, Mode: CaptureTail},
	{Elems: []string{"c", "[]"}, Max: 1 << 20, Mode: CaptureHead},
	{Elems: []string{"d", "e"}, Max: 8, Mode: CaptureHeadTail},
	{Elems: []string{"a"}, Max: 8, Mode: CaptureLines},
}

// members lists an object's members in order, duplicates included, with the
// oracle's raw value for each.
func members(t *testing.T, raw []byte) (keys []string, vals []json.RawMessage) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, nil
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("oracle key: %v", err)
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			t.Fatalf("oracle value: %v", err)
		}
		keys, vals = append(keys, tok.(string)), append(vals, v)
	}
	return keys, vals
}

// expect is the oracle's view of what Stream passes for one value, and
// whether it is decoded text rather than JSON text.
func expect(t *testing.T, raw json.RawMessage) (v string, text, ok bool) {
	if string(raw) == "null" {
		return "", false, false
	}
	if raw[0] == '"' {
		return decodeString(t, raw), true, true
	}
	return string(raw), false, true
}

// oracleCalls lists, per path, the calls Stream must make, in order.
func oracleCalls(t *testing.T, data []byte) map[int][]got {
	want := map[int][]got{}
	add := func(g ...got) {
		for _, x := range g {
			want[x.path] = append(want[x.path], x)
		}
	}
	keys, vals := members(t, data)
	for i, k := range keys {
		v, text, ok := expect(t, vals[i])
		switch {
		case k == "a" && ok:
			add(got{1, "", false}, got{0, v, false})
			add(splitLines(v, 8, text)...)
		case k == "n" && ok:
			add(got{2, v, false})
		case k == "c" && vals[i][0] == '[':
			var elems []json.RawMessage
			if err := json.Unmarshal(vals[i], &elems); err != nil {
				t.Fatal(err)
			}
			for _, e := range elems {
				if ev, _, ok := expect(t, e); ok {
					add(got{3, ev, false})
				}
			}
		case k == "d":
			ik, iv := members(t, vals[i])
			for j := range ik {
				if ev, etext, ok := expect(t, iv[j]); ok && ik[j] == "e" {
					add(headTail(ev, 8, etext)...)
				}
			}
		}
	}
	return want
}

func cutAt(v string, max int, text bool) int {
	if !text {
		return min(max, len(v))
	}
	return runeCut([]byte(v), max)
}

func headTail(v string, max int, text bool) []got {
	h := cutAt(v, max, text)
	if len(v)-h <= max {
		return []got{{4, v, false}}
	}
	tail := v[len(v)-max:]
	for i := 1; i < 4 && len(tail) > 0 && !utf8.RuneStart(tail[0]); i++ {
		tail = tail[1:]
	}
	return []got{{4, v[:h], true}, {4, tail, true}}
}

func splitLines(v string, max int, text bool) []got {
	var out []got
	if v == "" {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSuffix(v, "\n"), "\n") {
		for len(line) > max {
			k := cutAt(line, max, text)
			out = append(out, got{5, line[:k], true})
			line = line[k:]
		}
		out = append(out, got{5, line, false})
	}
	return out
}

func FuzzRES7_StreamMatchesStdlib(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var calls [2]map[int][]got
		var errs [2]error
		for i, r := range []io.Reader{bytes.NewReader(data), iotest.OneByteReader(bytes.NewReader(data))} {
			calls[i] = map[int][]got{}
			_, errs[i] = Stream(r, fuzzPaths, func(p int, v []byte, tr bool) bool {
				calls[i][p] = append(calls[i][p], got{p, string(v), tr})
				return true
			})
		}
		if (errs[0] == nil) != (errs[1] == nil) || (errs[0] != nil && errs[0].Error() != errs[1].Error()) {
			t.Fatalf("window boundaries changed the verdict: %v / %v", errs[0], errs[1])
		}
		if errs[0] == nil && !reflect.DeepEqual(calls[0], calls[1]) {
			t.Fatalf("window boundaries changed the values:\n%+v\n%+v", calls[0], calls[1])
		}
		// Before a syntax error, CaptureLines may have passed pieces of a
		// value as its bytes arrived; how many depends on the window, so on
		// an error one run's calls are a prefix of the other's per path.
		for p := range fuzzPaths {
			a, b := calls[0][p], calls[1][p]
			if len(a) > len(b) {
				a, b = b, a
			}
			if !slices.Equal(a, b[:len(a)]) {
				t.Fatalf("path %d: %+v is not a prefix of %+v", p, a, b)
			}
		}
		valid := json.Valid(data)
		if _, err := StreamOne(bytes.NewReader(data), fuzzPaths, func(int, []byte, bool) bool { return true }); (err == nil) != valid {
			t.Fatalf("StreamOne(%q) = %v, json.Valid = %v", data, err, valid)
		}
		if valid && errs[0] != nil {
			t.Fatalf("Stream rejected valid %q: %v", data, errs[0])
		}
		if !valid {
			if errs[0] == nil {
				checkSequence(t, data)
			}
			return
		}
		if want := oracleCalls(t, data); !reflect.DeepEqual(want, calls[0]) {
			t.Fatalf("%q:\n got %+v\nwant %+v", data, calls[0], want)
		}
	})
}

// checkSequence: Stream accepted input json.Valid rejects, so it must be a
// whitespace-separated sequence of values encoding/json's decoder accepts.
func checkSequence(t *testing.T, data []byte) {
	dec := json.NewDecoder(bytes.NewReader(data))
	n := 0
	for {
		var v json.RawMessage
		err := dec.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Stream accepted %q, the decoder rejects it: %v", data, err)
		}
		n++
	}
	if n < 2 {
		t.Fatalf("Stream accepted %q as %d values", data, n)
	}
}
