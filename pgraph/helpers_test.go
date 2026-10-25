package pgraph

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const testModule = "testdata/mod"

// repoRoot is the main module this module is nested in; the checkout it sits
// in is the fixed snapshot for the self-scan tests. Skipped when pgraph has
// been moved out of the repository.
func repoRoot(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "go.mod"))
	if err != nil || !strings.HasPrefix(string(b), "module github.com/altrace-dev-role/rashomon\n") {
		t.Skip("pgraph is not nested in the rashomon module")
	}
	return ".."
}

func loadT(t *testing.T, root string) *Program {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p, err := Load(ctx, root, Config{})
	if err != nil {
		t.Fatalf("Load(%s): %v", root, err)
	}
	return p
}

// goList is the independent oracle for the package set: the go tool itself,
// run in the same forced environment.
func goList(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = root
	cmd.Env = LoaderEnv(os.Environ(), Config{})
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	lines := strings.Fields(string(out))
	sort.Strings(lines)
	return lines
}

// copyTree copies src to a fresh temp dir so a test can edit it.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}
