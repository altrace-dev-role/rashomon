package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRS1a_R23_HookLineNamesNoPath: a sync the hook cannot write prints one
// line of fixed words and a class, naming no file of the project and no
// error text; the CLI form (pgraph sync ROOT) keeps the detail. The fixture
// is TestPW1b's type error, in a file whose name is the probe. Break: print
// the first line of the sanitized error, and the file name shows.
func TestRS1a_R23_HookLineNamesNoPath(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	root := repo(t, smallModule)
	if code, out, errs := hookRun(t, payload(root)); code != 0 || out != "" || errs != "" {
		t.Fatalf("first hook: exit %d, stdout %q, stderr %q; want 0 and silence", code, out, errs)
	}
	write(t, filepath.Join(root, "a", "zq7leak.go"), "package a\n\nvar Y int = \"s\"\n")
	code, out, errs := hookRun(t, payload(root))
	if code != 0 || out != "" || !strings.HasPrefix(errs, "pgraph: ") || strings.Count(errs, "\n") != 1 || len(errs) > 81 {
		t.Fatalf("a type error: exit %d, stdout %q, stderr %q; want 0, silence, one line", code, out, errs)
	}
	for _, leak := range []string{"zq7leak", ".go", "cannot use", root} {
		if strings.Contains(errs, leak) {
			t.Errorf("the hook line %q holds %q; want fixed words and a class, no path and no error text", errs, leak)
		}
	}
}
