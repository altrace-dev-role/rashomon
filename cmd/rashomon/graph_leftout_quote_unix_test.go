//go:build unix

package main

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// TestGraphIndex_AnEnvironmentLineQuotesAControlOrFormatRune (L2): the
// warning line of an environment whose directory name holds a control rune
// (a newline) or a format rune (U+202E, the right-to-left override) names
// the path as a Go quoted string, so the newline cannot write a line of its
// own and the override cannot turn the rest of the line around on a
// terminal; the index header keeps the path as it is. The runes are written
// here as escapes, so this file holds neither. Unix only: Windows refuses a
// newline in a file name. Break: print the path as it is.
func TestGraphIndex_AnEnvironmentLineQuotesAControlOrFormatRune(t *testing.T) {
	for _, c := range []struct {
		name, env, quoted string
	}{
		{"a newline", "host/e\nx", `"host/e\nx"`},
		{"the right-to-left override", "host/e\u202ex", `"host/e\u202ex"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			refusedPrintTree(t, dir)
			putFiles(t, dir, map[string]string{
				c.env + "/pyvenv.cfg": "home = /usr/bin\n",
				c.env + "/lib/m.py":   "x = 1\n",
			})
			var out strings.Builder
			if err := graphIndex([]string{dir}, &out); err != nil {
				t.Fatalf("graph index = %v\n%q", err, out.String())
			}
			want := "warning: " + c.quoted + "/ not indexed: " + lang.LeftEnvironment + " (holds pyvenv.cfg)\n" + "indexed 1 file(s): "
			got := out.String()
			if !strings.HasPrefix(got, want) || strings.Count(got, "\n") != 2 || strings.ContainsRune(got, '\u202e') {
				t.Errorf("graph index printed:\n%q\nwant it to start %q, to hold 2 lines and no U+202E", got, want)
			}
			_, h, err := graph.LoadFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			if h.LeftOut != 1 || len(h.LeftOutDirs) != 1 || h.LeftOutDirs[0].Path != c.env {
				t.Errorf("header left out %d, named %#v; want 1, the one path as it is, %q", h.LeftOut, h.LeftOutDirs, c.env)
			}
		})
	}
}
