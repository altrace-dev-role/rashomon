package shape

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/test/perf/pathological"
)

// pathologicalLines is the tool input of a Bash call whose line is one of
// each shape that has driven the shape of a call past its memory
// (pathological.Lines), as many units of it as an 8 MiB hook payload holds.
// The input is encoded as Claude Code encodes it, without escaping &, < and >
// as \u0026 and the like.
func pathologicalLines(t testing.TB) []struct {
	name string
	in   json.RawMessage
} {
	encode := func(line string) []byte {
		var b bytes.Buffer
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(map[string]string{"command": line}); err != nil {
			t.Fatal(err)
		}
		return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
	}
	var out []struct {
		name string
		in   json.RawMessage
	}
	for _, l := range pathological.Lines() {
		per := len(encode(l.Unit)) - len(encode(""))
		n := (8<<20 - len(encode(l.Prefix+l.Suffix))) / per
		out = append(out, struct {
			name string
			in   json.RawMessage
		}{l.Name, encode(l.Prefix + strings.Repeat(l.Unit, n) + l.Suffix)})
	}
	return out
}

// allocated is how many bytes f allocates.
func allocated(f func()) uint64 {
	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	f()
	runtime.ReadMemStats(&m1)
	return m1.TotalAlloc - m0.TotalAlloc
}

// TestRES29_ShapeOfEveryPathologicalLineIsBounded: each of the hook's
// readings of a Bash call's line -- Derive, LeadingDirectory and Hosts --
// allocates a few times the payload at most, whatever the 8 MiB line is made
// of: Derive 40 MiB, the others 24, each including one decoding of the
// command. The token bound (RES-9) held one shape, short words, and no
// other. 8 MiB of backslash-newlines allocated 322 MiB in Derive, keeping one
// offset per continuation and joining the line three times. One 8 MiB word
// of $a allocated 175 MiB, copied byte by byte into a buffer that grew by a
// quarter at a time. Hosts tokenized each line of a line of lines within the
// bound and the whole past it, 452 MiB for `ssh a` lines, and every name
// after https:// cost a parse error. And each reader decoded the command
// into a buffer grown from nothing and copied it again. With the hook's own
// intake those drove a hook to 87, 84 and 178 MiB of RSS against RES-29's
// 64; cmd/rashomon's TestRES29_PathologicalInputsStayBounded now runs the
// hook on every shape here and pins it within 64.
func TestRES29_ShapeOfEveryPathologicalLineIsBounded(t *testing.T) {
	for _, tc := range pathologicalLines(t) {
		if len(tc.in) > 8<<20 {
			t.Fatalf("%s: the input is %d bytes, past an 8 MiB payload", tc.name, len(tc.in))
		}
		for _, r := range []struct {
			name  string
			read  func()
			bound uint64
		}{
			{"Derive", func() { Derive("Bash", tc.in, benchKey) }, 40 << 20},
			{"LeadingDirectory", func() { LeadingDirectory("Bash", tc.in) }, 24 << 20},
			{"Hosts", func() { Hosts("Bash", tc.in) }, 24 << 20},
		} {
			if alloc := allocated(r.read); alloc > r.bound {
				t.Errorf("%s: %s allocated %d MiB, want at most %d", tc.name, r.name, alloc>>20, r.bound>>20)
			}
		}
	}
}

// TestRES29_ContinuationsPastTheKeptOffsets: joinContinuations keeps
// maxJoins offsets and no more. Up to them a here-document after the
// continuations is read and the runner after its body found; one past them
// where a backslash-newline was removed is no longer known, and the reading
// of the body gives up, so the masked status is null rather than a guess.
// The program is named either way.
func TestRES29_ContinuationsPastTheKeptOffsets(t *testing.T) {
	line := func(n int) string {
		return "echo " + strings.Repeat("a\\\n", n) + "; cat <<E\nx\nE\nmake test 2>&1 | tail"
	}
	if r := joinContinuations(line(maxJoins)); r.lost || len(r.joins) != maxJoins {
		t.Errorf("%d continuations: %d kept, lost %v; want all kept", maxJoins, len(r.joins), r.lost)
	}
	if r := joinContinuations(line(maxJoins + 1)); !r.lost || len(r.joins) != maxJoins {
		t.Errorf("%d continuations: %d kept, lost %v; want %d kept and the rest lost", maxJoins+1, len(r.joins), r.lost, maxJoins)
	}
	s := Derive("Bash", bashInput(t, line(maxJoins)), benchKey)
	if s.StatusMasked == nil || *s.StatusMasked != MaskedTest {
		t.Errorf("at maxJoins: status_masked %v, want %q", show(s.StatusMasked), MaskedTest)
	}
	s = Derive("Bash", bashInput(t, line(maxJoins+1)), benchKey)
	if s.StatusMasked != nil {
		t.Errorf("one past maxJoins: status_masked %v, want null", show(s.StatusMasked))
	}
	if s.Program == nil || *s.Program != "echo" {
		t.Errorf("one past maxJoins: program %v, want echo", show(s.Program))
	}
}

// TestRES29_DeriveReadsTheLineThreeTimes: Derive lexes a shell line three
// times -- for argc, the program and the masked status -- and counts the
// leading cds from the program search's tokens rather than lexing a fourth
// time (P-12's fold). Past the decoding of the command, it allocates at most
// three and a half times one reading of the line, on two lines where each
// reading costs megabytes: one word of 2.8 million escaped blanks, copied by
// each reading, and 2.8 million backslash-newlines, joined by two of them.
// A fourth reading puts it at four.
func TestRES29_DeriveReadsTheLineThreeTimes(t *testing.T) {
	for _, tc := range pathologicalLines(t) {
		if tc.name != "escaped blanks" && tc.name != "backslash-newlines" {
			continue
		}
		cmd, ok := commandField(tc.in)
		if !ok {
			t.Fatalf("%s: no command", tc.name)
		}
		decode := allocated(func() { commandField(tc.in) })
		reading := allocated(func() { tokenizeProgram(cmd) })
		derive := allocated(func() { Derive("Bash", tc.in, benchKey) })
		if ratio := float64(derive-decode) / float64(reading); ratio > 3.5 {
			t.Errorf("%s: Derive allocated %.2f readings' worth past the decoding, want at most 3.5", tc.name, ratio)
		}
	}
}
