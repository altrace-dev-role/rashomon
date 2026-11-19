package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// refusedLine5 is the warning pylite's refusal of refusedPy prints after
// the path, the bracket of void_invoices left open at line 5.
const refusedLine5 = ":5 not indexed: a bracket opened at this line is not closed\n"

// refusedPy is a Python file the extractor refuses at line 5.
const refusedPy = "def refund_ledger(entry):\n    return entry\n\n\ndef void_invoices(rows:\n    return rows\n"

// refusedPrintTree writes under dir a Python unit host with one file the
// extractor reads and, at each slash path in bad, refusedPy.
func refusedPrintTree(t *testing.T, dir string, bad ...string) {
	t.Helper()
	files := map[string]string{
		".rashomon/graph.json": `{"units":[{"path":"host","modules":["host"],"lang":"py"}]}`,
		"host/host.py":         "def canonical_name(user):\n    return user.strip()\n",
	}
	for _, rel := range bad {
		files[rel] = refusedPy
	}
	for rel, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestGraphIndex_RefusalLinesStopAtMaxRefusedNamed: `graph index` over a
// tree with n refused files prints a warning line for each of the first
// graph.MaxRefusedNamed by path, the files the index header names, and
// for the rest one line that counts them: at exactly MaxRefusedNamed no
// count line, at one more "1 more", at two more "2 more"; the summary
// still counts every refused file. Break: print a line for every refused
// file; write the count line at exactly MaxRefusedNamed; write it only
// past one more.
func TestGraphIndex_RefusalLinesStopAtMaxRefusedNamed(t *testing.T) {
	for _, n := range []int{graph.MaxRefusedNamed, graph.MaxRefusedNamed + 1, graph.MaxRefusedNamed + 2} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			dir := t.TempDir()
			bad := make([]string, n)
			for i := range bad {
				bad[i] = fmt.Sprintf("host/r%02d.py", i)
			}
			refusedPrintTree(t, dir, bad...)
			var out strings.Builder
			if err := graphIndex([]string{dir}, &out); err != nil {
				t.Fatalf("graph index = %v\n%s", err, out.String())
			}
			var want strings.Builder
			for _, rel := range bad[:graph.MaxRefusedNamed] {
				want.WriteString("warning: " + rel + refusedLine5)
			}
			if more := n - graph.MaxRefusedNamed; more > 0 {
				fmt.Fprintf(&want, "warning: %d more file(s) not indexed\n", more)
			}
			fmt.Fprintf(&want, "indexed %d file(s), %d refused: ", n+1, n)
			if got := out.String(); !strings.HasPrefix(got, want.String()) || strings.Count(got, "\n") != strings.Count(want.String(), "\n")+1 {
				t.Errorf("graph index printed:\n%s\nwant:\n%s...", got, want.String())
			}
			_, h, err := graph.LoadFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			if h.Refused != n || len(h.RefusedFiles) != graph.MaxRefusedNamed || h.RefusedFiles[graph.MaxRefusedNamed-1].Path != bad[graph.MaxRefusedNamed-1] {
				t.Errorf("header refused %d, named %#v; want %d, the first %d by path, the last %s",
					h.Refused, h.RefusedFiles, n, graph.MaxRefusedNamed, bad[graph.MaxRefusedNamed-1])
			}
		})
	}
}

// TestGraphIndex_ARefusalLineQuotesAFormatRune: the warning line of a
// refused file whose path holds U+202E, the right-to-left override, names
// the path as a Go quoted string, so the line holds the escape and not the
// rune, which would turn the rest of the line around on a terminal; the
// index header keeps the path as it is. The rune is written here as an
// escape, so this file holds none. Break: quote only a path holding a
// control rune, which a format rune is not; or print the path as it is.
func TestGraphIndex_ARefusalLineQuotesAFormatRune(t *testing.T) {
	dir := t.TempDir()
	const name = "host/bil\u202eling.py"
	refusedPrintTree(t, dir, name)
	var out strings.Builder
	if err := graphIndex([]string{dir}, &out); err != nil {
		t.Fatalf("graph index = %v\n%q", err, out.String())
	}
	want := `warning: "host/bil\u202eling.py"` + refusedLine5
	if got := out.String(); !strings.HasPrefix(got, want) || strings.ContainsRune(got, '\u202e') {
		t.Errorf("graph index printed:\n%q\nwant it to start %q and to hold no U+202E", got, want)
	}
	_, h, err := graph.LoadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.RefusedFiles) != 1 || h.RefusedFiles[0].Path != name {
		t.Errorf("header names %#v; want the one path as it is, %q", h.RefusedFiles, name)
	}
}

// TestRefusalLine_QuotesWhatIsNotPrintable: refusalLine names the path and
// the reason of a refused file as they are when each is printable UTF-8,
// a letter outside ASCII included, and each as a Go quoted string when it
// is not: a newline in a reason cannot start a line of its own, a bad byte
// in a reason is escaped, and the path of a refusal with no line is
// quoted as the path of one with a line is. Break: print the reason as it
// is; print the path of a refusal with no line as it is; drop the UTF-8
// test; print the reason of a refusal with no line as it is.
func TestRefusalLine_QuotesWhatIsNotPrintable(t *testing.T) {
	for _, c := range []struct {
		f    graph.Refusal
		want string
	}{
		{graph.Refusal{Path: "app/x.py", Line: 3, Reason: "a def with no colon after its parameters"},
			"warning: app/x.py:3 not indexed: a def with no colon after its parameters\n"},
		{graph.Refusal{Path: "app/caf\u00e9.py", Reason: "permission denied"},
			"warning: app/caf\u00e9.py not indexed: permission denied\n"},
		{graph.Refusal{Path: "app/x.py", Line: 3, Reason: "bad\nindexed 9 file(s)"},
			`warning: app/x.py:3 not indexed: "bad\nindexed 9 file(s)"` + "\n"},
		{graph.Refusal{Path: "app/x.py", Line: 3, Reason: "bad \xff byte"},
			`warning: app/x.py:3 not indexed: "bad \xff byte"` + "\n"},
		{graph.Refusal{Path: "app/x\ty.py", Reason: "permission denied"},
			`warning: "app/x\ty.py" not indexed: permission denied` + "\n"},
		{graph.Refusal{Path: "app/x.py", Reason: "bad\nindexed 9 file(s)"},
			`warning: app/x.py not indexed: "bad\nindexed 9 file(s)"` + "\n"},
	} {
		if got := refusalLine(c.f); got != c.want {
			t.Errorf("refusalLine(%#v) = %q; want %q", c.f, got, c.want)
		}
	}
}
