package linescan

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

type step struct {
	line    string
	tooLong bool
}

func scanAll(t testing.TB, in string, initial, max int) ([]step, int, error) {
	t.Helper()
	s := New(strings.NewReader(in), initial, max)
	var out []step
	for s.Scan() {
		if s.TooLong() && len(s.Bytes()) != 0 {
			t.Fatalf("a line over the bound hands back %d bytes", len(s.Bytes()))
		}
		out = append(out, step{string(s.Bytes()), s.TooLong()})
	}
	return out, s.Skipped(), s.Err()
}

// bufioLines is what bufio.Scanner with ScanLines reads from in under the same
// bound, for the inputs it can read to the end.
func bufioLines(t testing.TB, in string, max int) ([]step, error) {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(in))
	sc.Buffer(nil, max)
	var out []step
	for sc.Scan() {
		out = append(out, step{line: sc.Text()})
	}
	return out, sc.Err()
}

func sameSteps(a, b []step) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestLinescan_ReadsWhatBufioReads: with no line over the bound, the lines
// are bufio.Scanner's ScanLines lines exactly -- a final line with no newline
// included, a trailing \r dropped, an empty line kept.
func TestLinescan_ReadsWhatBufioReads(t *testing.T) {
	for _, in := range []string{
		"", "\n", "a", "a\n", "a\nb", "a\r\nb\r\n", "\r\n\r\n", "a\n\nb\n", "x\ry\n", "\r",
		strings.Repeat("abc\n", 50), strings.Repeat("y", 63) + "\n" + strings.Repeat("z", 63),
	} {
		want, err := bufioLines(t, in, 64)
		if err != nil {
			t.Fatalf("premise: bufio cannot read %q: %v", in, err)
		}
		got, skipped, err := scanAll(t, in, 16, 64)
		if err != nil || skipped != 0 || !sameSteps(got, want) {
			t.Errorf("%q: read %v (skipped %d, err %v), want %v", in, got, skipped, err, want)
		}
	}
}

// TestLinescan_ALineOverTheBoundIsSteppedOver: the bound is bufio.Scanner's,
// max counting the newline, so a line of max-1 bytes is read and one of max
// is not -- with or without a newline after it. The line over it is one step,
// marked TooLong and counted, and every line after it reads as usual.
func TestLinescan_ALineOverTheBoundIsSteppedOver(t *testing.T) {
	const max = 64
	fits, over := strings.Repeat("f", max-1), strings.Repeat("o", max)
	for _, tc := range []struct {
		in   string
		want []step
	}{
		{fits + "\nnext\n", []step{{fits, false}, {"next", false}}},
		{over + "\nnext\n", []step{{"", true}, {"next", false}}},
		{fits, []step{{fits, false}}},
		{over, []step{{"", true}}},
		{"a\n" + strings.Repeat("o", 10*max) + "\r\nb\n" + over + "\n", []step{{"a", false}, {"", true}, {"b", false}, {"", true}}},
		// The \r is a byte of the line, as it is to bufio.Scanner.
		{fits[1:] + "\r\n", []step{{fits[1:], false}}},
		{fits + "\r\n", []step{{"", true}}},
	} {
		got, skipped, err := scanAll(t, tc.in, 16, max)
		long := 0
		for _, s := range tc.want {
			if s.tooLong {
				long++
			}
		}
		if err != nil || skipped != long || !sameSteps(got, tc.want) {
			t.Errorf("%.20q...: read %v (skipped %d, err %v), want %v (skipped %d)", tc.in, got, skipped, err, tc.want, long)
		}
	}
}

// TestLinescan_HoldsNoMoreThanItsBound: a line over the bound is discarded as
// it is read, never assembled, so what the scanner holds stays at max however
// long the line.
func TestLinescan_HoldsNoMoreThanItsBound(t *testing.T) {
	// Not a power of two, so doubling from the read buffer would pass it.
	const max = 1000
	in := strings.Repeat("f", max-1) + "\n" + strings.Repeat("x", 1<<20) + "\nok\n"
	s := New(strings.NewReader(in), 16, max)
	lines := 0
	for s.Scan() {
		lines++
		if c := cap(s.buf); c > max {
			t.Fatalf("line %d: holding %d bytes for a %d-byte bound", lines, c, max)
		}
	}
	if lines != 3 || s.Skipped() != 1 || s.Err() != nil {
		t.Fatalf("%d lines, skipped %d, err %v; want 3, 1 and nil", lines, s.Skipped(), s.Err())
	}
}

// failOnce delivers its first part, fails once, then would deliver the rest.
type failOnce struct {
	parts  []string
	err    error
	failed bool
}

func (r *failOnce) Read(p []byte) (int, error) {
	if len(r.parts) == 0 {
		return 0, io.EOF
	}
	if len(r.parts) == 1 && !r.failed {
		r.failed = true
		return 0, r.err
	}
	n := copy(p, r.parts[0])
	r.parts = r.parts[1:]
	return n, nil
}

// TestLinescan_AReadErrorEndsTheScan: an error from the reader is not a line;
// Scan stops and Err says why, as bufio.Scanner does. The lines before it are
// read, and the partial line the error cut short is not handed back.
func TestLinescan_AReadErrorEndsTheScan(t *testing.T) {
	boom := errors.New("boom")
	s := New(io.MultiReader(strings.NewReader("a\nb\npart"), iotest.ErrReader(boom)), 16, 64)
	var got []string
	for s.Scan() {
		got = append(got, string(s.Bytes()))
	}
	if !errors.Is(s.Err(), boom) || strings.Join(got, ",") != "a,b" {
		t.Fatalf("read %q, err %v; want a,b and the reader's error", got, s.Err())
	}
	// A reader that would deliver more after its error is not read again:
	// the scan ended at the error.
	s = New(&failOnce{parts: []string{"a\n", "b\n"}, err: boom}, 16, 64)
	got = nil
	for s.Scan() {
		got = append(got, string(s.Bytes()))
	}
	if !errors.Is(s.Err(), boom) || strings.Join(got, ",") != "a" {
		t.Fatalf("read %q, err %v; want a and the reader's error", got, s.Err())
	}
	if s.Scan() {
		t.Fatalf("Scan after an error read %q", s.Bytes())
	}
	// Data delivered with io.EOF in the same call is still read.
	s = New(iotest.DataErrReader(strings.NewReader("a\nb")), 16, 64)
	got = nil
	for s.Scan() {
		got = append(got, string(s.Bytes()))
	}
	if s.Err() != nil || strings.Join(got, ",") != "a,b" {
		t.Fatalf("data with EOF: read %q, err %v", got, s.Err())
	}
}
