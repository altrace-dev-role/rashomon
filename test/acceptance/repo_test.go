package acceptance

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoSourceFileIsGitIgnored exists because a `coverage.*` pattern once
// ignored internal/hook/coverage.go, which would have committed a tree that
// does not compile. A pattern that hides a source file is a bug in the
// repository, and this is the only place it can be caught before a commit.
func TestNoSourceFileIsGitIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	files, err := filepath.Glob(filepath.Join(moduleRoot, "*", "*", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	more, _ := filepath.Glob(filepath.Join(moduleRoot, "*", "*", "*", "*.go"))
	files = append(files, more...)
	if len(files) < 10 {
		t.Fatalf("found only %d source files; the glob is not looking in the right place", len(files))
	}

	cmd := exec.Command("git", "check-ignore", "--stdin")
	cmd.Dir = moduleRoot
	cmd.Stdin = strings.NewReader(strings.Join(files, "\n"))
	out, _ := cmd.Output() // exit 1 means nothing is ignored, which is the pass
	if ignored := strings.TrimSpace(string(out)); ignored != "" {
		t.Errorf("source files are git-ignored:\n%s", ignored)
	}
}
