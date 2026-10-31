package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPF1x_SubdirectoryCwdResolvesFromTheRoot: `claude` started below the
// project root names a file from the root, and the note finds it: requests
// resolve against the project (the git root above the cwd), not the cwd. The
// control, the same request from the root, gives the same note.
func TestPF1x_SubdirectoryCwdResolvesFromTheRoot(t *testing.T) {
	var notes [2]string
	for i, sub := range []string{"", "internal"} {
		e := newSteerEnv(t)
		e.preflightProject()
		if err := os.Mkdir(filepath.Join(e.cwd, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, out := e.prompt(filepath.Join(e.cwd, sub), "fix the nil check in "+pf1bPath, "prompt-2")
		notes[i] = out.HookSpecificOutput.AdditionalContext
		if !strings.Contains(notes[i], "'"+pf1bPath+"'") || strings.Contains(notes[i], "no named path exists") {
			t.Errorf("cwd %q: the note does not resolve the path from the project root:\n%s", sub, notes[i])
		}
	}
	if notes[0] != notes[1] {
		t.Errorf("the subdirectory's note differs from the root's:\n%s\nvs\n%s", notes[1], notes[0])
	}
}
