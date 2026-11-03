//go:build !windows

// buildchanged.py's tests use sweep_test.go's helpers (write, mutant,
// importEntry), so they build where it does: every GOOS but windows, whose
// vet and staticcheck runs would otherwise find the helpers undefined.
package mutation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildchangedFixture is a throwaway module in a git repository of its own:
// x.go, a test that calls it, and a mutant file whose one row builds,
// committed as the base. It returns the directory and the base commit.
func buildchangedFixture(t *testing.T, baseRows string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH; the fixture needs a repository")
	}
	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module fixture\n\ngo 1.25\n")
	write(t, filepath.Join(dir, "x.go"), "package fixture\n\nimport (\n\t\"strings\"\n)\n\n"+
		"func F(v int) int {\n\tw := v + 1\n\treturn w\n}\n\n"+
		"func G(s string) string { return strings.ToUpper(s) }\n")
	write(t, filepath.Join(dir, "x_test.go"), "package fixture\n\nimport \"testing\"\n\n"+
		"func TestFixture_(t *testing.T) {\n\tif F(1) != 2 || G(\"a\") != \"A\" {\n\t\tt.Fatal(\"F or G\")\n\t}\n}\n")
	mutants := filepath.Join(dir, "test", "mutation", "mutants")
	if err := os.MkdirAll(mutants, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(mutants, "T-1_fixture.py"), baseRows)
	fixtureGit(t, dir, "init", "-q")
	fixtureGit(t, dir, "add", "-A")
	fixtureGit(t, dir, "commit", "-q", "-m", "base")
	return dir, strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// buildchanged runs buildchanged.py against base from dir and returns its
// stdout, stderr and exit code.
func buildchanged(t *testing.T, dir, base string) (string, string, int) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not on PATH; the check cannot run here")
	}
	script, err := filepath.Abs("buildchanged.py")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, script, base)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("buildchanged.py did not finish within 2m; stderr:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "Traceback (most recent call last)") {
		t.Fatalf("buildchanged.py crashed instead of reporting:\n%s", stderr.String())
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return stdout.String(), stderr.String(), exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), 0
}

// TestDEL2h_EveryChangedRowBuilds (MASKED-BUILD2): a row a change adds or
// edits, or whose IMPORTS entry it adds, is compiled, and one that does not
// build is refused by name. A row the change leaves alone is not compiled
// here, broken or not: the sweep judges every row, and this check is the
// author's, at the change. The tree is put back after each row.
func TestDEL2h_EveryChangedRowBuilds(t *testing.T) {
	builds := mutant("T-1 at base, builds", "x.go", "return w", "return w - 1", "TestFixture_")
	broken := mutant("T-1 at base, a local left unused", "x.go", "return w", "return v", "TestFixture_")
	cases := []struct {
		name       string
		base, head string
		wantCode   int
		refused    string // the row a refusal names, and the compiler's words
		words      string
	}{
		{"a new row that leaves a local unused", builds,
			builds + mutant("T-1 new, unused", "x.go", "return w", "return v", "TestFixture_"),
			1, "T-1 new, unused", "declared and not used"},
		{"a new row that builds", builds,
			builds + mutant("T-1 new, builds", "x.go", "return w", "return w + 1", "TestFixture_"),
			0, "", ""},
		{"an edited replacement that stops building", builds,
			mutant("T-1 at base, builds", "x.go", "return w", "return v", "TestFixture_"),
			1, "T-1 at base, builds", "declared and not used"},
		{"an IMPORTS entry added to a row that keeps the package's use", builds,
			builds + importEntry("T-1 at base, builds", "x.go", "\t\"strings\"\n", ""),
			1, "T-1 at base, builds", "undefined: strings"},
		{"a broken row the change leaves alone", broken,
			broken + mutant("T-1 new, builds", "x.go", "return w", "return w + 1", "TestFixture_"),
			0, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, base := buildchangedFixture(t, tc.base)
			write(t, filepath.Join(dir, "test", "mutation", "mutants", "T-1_fixture.py"), tc.head)
			stdout, stderr, code := buildchanged(t, dir, base)
			if code != tc.wantCode {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, stdout, stderr)
			}
			if tc.refused != "" && (!strings.Contains(stderr, tc.refused) || !strings.Contains(stderr, tc.words)) {
				t.Errorf("the refusal does not name %q with %q:\n%s", tc.refused, tc.words, stderr)
			}
			if tc.refused == "" && strings.Contains(stdout+stderr, "T-1 at base, a local left unused") {
				t.Errorf("a row the change left alone was judged:\n%s%s", stdout, stderr)
			}
			if tree := fixtureGit(t, dir, "status", "--porcelain", "--", "x.go"); tree != "" {
				t.Errorf("x.go was not put back: %q", tree)
			}
		})
	}
}

// TestDEL2i_ATargetWithChangesIsRefused: putting a target back is a git
// checkout, which would lose edits not yet committed, so the check refuses
// to edit a target with changes and leaves it as it was.
func TestDEL2i_ATargetWithChangesIsRefused(t *testing.T) {
	builds := mutant("T-1 at base, builds", "x.go", "return w", "return w - 1", "TestFixture_")
	dir, base := buildchangedFixture(t, builds)
	write(t, filepath.Join(dir, "test", "mutation", "mutants", "T-1_fixture.py"),
		builds+mutant("T-1 new, builds", "x.go", "return w", "return w + 1", "TestFixture_"))
	edited := readFile(t, filepath.Join(dir, "x.go")) + "\n// an edit not yet committed\n"
	write(t, filepath.Join(dir, "x.go"), edited)
	_, stderr, code := buildchanged(t, dir, base)
	if code != 2 || !strings.Contains(stderr, "x.go") {
		t.Fatalf("exit %d with x.go edited, want 2 naming it; stderr:\n%s", code, stderr)
	}
	if got := readFile(t, filepath.Join(dir, "x.go")); got != edited {
		t.Errorf("the edit to x.go was lost:\n%s", got)
	}
}
