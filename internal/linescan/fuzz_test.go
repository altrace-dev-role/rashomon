package linescan

import (
	"bytes"
	"strings"
	"testing"
)

// FuzzScan holds Scanner to two oracles over arbitrary input and bounds: a
// plain split on '\n', under which a line is over the bound when it and its
// newline are more than max bytes (a final line with none, when it is max
// bytes or more); and, when no line is over the bound, bufio.Scanner's own
// ScanLines, line for line. The read buffer is smaller than most lines here,
// so the assembly path is the one exercised.
func FuzzScan(f *testing.F) {
	for _, s := range []string{"", "a\nb", "a\r\n\r\nb\r", strings.Repeat("x", 70) + "\nok\n", "\n\n\n"} {
		f.Add([]byte(s), uint8(8))
		f.Add([]byte(s), uint8(64))
	}
	f.Fuzz(func(t *testing.T, in []byte, bound uint8) {
		max := int(bound%64) + 1
		got, skipped, err := scanAll(t, string(in), 16, max)
		if err != nil {
			t.Fatalf("a read from memory failed: %v", err)
		}
		want, long := split(in, max)
		if skipped != long || !sameSteps(got, want) {
			t.Fatalf("Scan(%q, max %d) = %v (skipped %d), want %v (skipped %d)", in, max, got, skipped, want, long)
		}
		if long == 0 {
			ref, err := bufioLines(t, string(in), max)
			if err != nil || !sameSteps(got, ref) {
				t.Fatalf("Scan(%q, max %d) = %v, bufio.Scanner read %v (%v)", in, max, got, ref, err)
			}
		}
	})
}

func split(in []byte, max int) ([]step, int) {
	var out []step
	long := 0
	for len(in) > 0 {
		line, rest, nl := bytes.Cut(in, []byte{'\n'})
		in = rest
		if over := len(line)+1 > max || (!nl && len(line) >= max); over {
			out = append(out, step{tooLong: true})
			long++
			continue
		}
		out = append(out, step{line: string(bytes.TrimSuffix(line, []byte{'\r'}))})
	}
	return out, long
}

// TestRubricPERF6_LinescanFuzzSeeds runs FuzzScan's oracles over a fixed set of inputs
// and every bound up to 64, as a plain test a mutant can name.
func TestRubricPERF6_LinescanFuzzSeeds(t *testing.T) {
	for _, s := range []string{"", "a\nb", "a\r\n\r\nb\r", strings.Repeat("x", 70) + "\nok\n", "\n\n\n", "\r\r\n", "ab\ncd\r\nef"} {
		for max := 1; max <= 64; max++ {
			got, skipped, err := scanAll(t, s, 16, max)
			want, long := split([]byte(s), max)
			if err != nil || skipped != long || !sameSteps(got, want) {
				t.Fatalf("Scan(%q, max %d) = %v (skipped %d, %v), want %v", s, max, got, skipped, err, want)
			}
			if long == 0 {
				if ref, err := bufioLines(t, s, max); err != nil || !sameSteps(got, ref) {
					t.Fatalf("Scan(%q, max %d) = %v, bufio.Scanner read %v (%v)", s, max, got, ref, err)
				}
			}
		}
	}
}
