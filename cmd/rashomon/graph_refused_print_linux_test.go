package main

import (
	"strings"
	"testing"
)

// TestGraphIndex_ARefusalLineQuotesAPathNotValidUTF8: the warning line of
// a refused file whose name is not valid UTF-8 names the path as a Go
// quoted string, its bad byte written as an escape. Linux takes such a
// name; the file systems of macOS refuse it, so this case is Linux only.
// Break: drop the UTF-8 test and keep the printable one, which reads a bad
// byte as U+FFFD, a printable rune, and lets the byte through.
func TestGraphIndex_ARefusalLineQuotesAPathNotValidUTF8(t *testing.T) {
	dir := t.TempDir()
	refusedPrintTree(t, dir, "host/bad\xffname.py")
	var out strings.Builder
	if err := graphIndex([]string{dir}, &out); err != nil {
		t.Fatalf("graph index = %v\n%q", err, out.String())
	}
	want := `warning: "host/bad\xffname.py"` + refusedLine5
	if got := out.String(); !strings.HasPrefix(got, want) {
		t.Errorf("graph index printed:\n%q\nwant it to start %q", got, want)
	}
}
