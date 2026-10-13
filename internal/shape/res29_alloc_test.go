package shape

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// pathologicalLines is the tool input of a Bash call whose line is one of
// each shape that has driven the shape of a call past its memory: one unit
// repeated, behind echo where the unit is not a command of its own, as many
// times as an 8 MiB hook payload holds. The input is encoded as Claude Code
// encodes it, without escaping &, < and > as \u0026 and the like.
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
	fill := func(prefix, unit, suffix string) json.RawMessage {
		per := len(encode(unit)) - len(encode(""))
		n := (8<<20 - len(encode(prefix+suffix))) / per
		return encode(prefix + strings.Repeat(unit, n) + suffix)
	}
	return []struct {
		name string
		in   json.RawMessage
	}{
		{"short words", fill("echo", " a", "")},
		{"backslash-newlines", fill("echo ", "a\\\n", "")},
		{"one word of $a", fill("echo ", "$a", "")},
		{"escaped blanks", fill("echo ", "\\ ", "")},
		{"quoted blanks", fill("echo ", "'' ", "")},
		{"double-quoted blanks", fill("echo ", `"" `, "")},
		{"one double-quoted word", fill("echo \"", "a", "\"")},
		{"redirections", fill("", "<", "")},
		{"cd a&&", fill("", "cd a&&", "")},
		{"a here-document", fill("python3 - <<'E'\n", "w1 w2 w3 w4 w5 w6 w7 w8 w9 w0\n", "E\n")},
		{"substitutions", fill("echo ", "$(a) ", "")},
		{"assignments", fill("", "X_TOKEN=abc ", "")},
		{"empty lines", fill("echo", "\n", "")},
		{"short lines", fill("echo", "\na", "")},
		{"ssh lines", fill("", "ssh a\n", "")},
		{"urls", fill("curl", " https://a.b/", "")},
	}
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
// 64; every shape here now peaks under 58.
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
