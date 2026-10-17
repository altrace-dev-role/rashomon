//go:build !windows

package mutation

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The CI Fuzz job (rubric fix list entry 23, test-1 and errors-7), read the
// way TestDEL17_EveryJobHasATimeout reads the workflows: by its lines.

// fuzzCommand returns the fuzz job's `go test ... -fuzz` line.
func fuzzCommand(t *testing.T, doc string) string {
	t.Helper()
	var found []string
	for _, ln := range strings.Split(doc, "\n") {
		if strings.Contains(ln, "go test ") && strings.Contains(ln, " -fuzz ") {
			found = append(found, strings.TrimSpace(ln))
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one `go test ... -fuzz` line in ci.yaml, found %d: %q", len(found), found)
	}
	return found[0]
}

func ciWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRubricTest1_FuzzJobBoundsMinimization: every new interesting input is
// minimized before the fuzzer moves on, for up to a minute by default. On
// FuzzTranscriptLines, whose inputs each write a file a line, one such input
// took the whole 30-second window: 112 execs and nothing new, against 28,363
// execs and 167 new inputs with minimization bounded to ten runs. A target
// that spends its window minimizing is a target CI does not fuzz.
func TestRubricTest1_FuzzJobBoundsMinimization(t *testing.T) {
	cmd := fuzzCommand(t, ciWorkflow(t))
	fuzzTime := regexp.MustCompile(`-fuzztime (\d+)s\b`).FindStringSubmatch(cmd)
	if fuzzTime == nil {
		t.Fatalf("fuzz command has no -fuzztime in seconds: %s", cmd)
	}
	m := regexp.MustCompile(`-fuzzminimizetime (\d+)(x|s)\b`).FindStringSubmatch(cmd)
	if m == nil {
		t.Fatalf("fuzz command leaves minimization at its one-minute default, which can take a target's whole window: %s", cmd)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	window, err := strconv.Atoi(fuzzTime[1])
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case m[2] == "x" && (n < 1 || n > 100):
		t.Errorf("-fuzzminimizetime %dx: want 1 to 100 runs per input", n)
	case m[2] == "s" && time.Duration(n)*time.Second*4 > time.Duration(window)*time.Second:
		t.Errorf("-fuzzminimizetime %ds is over a quarter of the %ds window", n, window)
	}
}

// TestRubricTest1_FuzzJobFloorIsTheTreeCount: the job finds its targets by
// grep and fails if it finds fewer than its floor, which only means something
// while the floor is the number the tree holds. A floor left behind when
// targets are added lets the search lose that many unnoticed.
func TestRubricTest1_FuzzJobFloorIsTheTreeCount(t *testing.T) {
	m := regexp.MustCompile(`(?m)^\s*test "\$found" -ge (\d+)\s*$`).FindStringSubmatch(ciWorkflow(t))
	if m == nil {
		t.Fatal(`the fuzz job has no 'test "$found" -ge N' floor`)
	}
	floor, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	root := moduleRoot(t)
	fuzzFunc := regexp.MustCompile(`(?m)^func Fuzz`)
	targets := 0
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		targets += len(fuzzFunc.FindAll(data, -1))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if floor != targets {
		t.Errorf("the fuzz job's floor is %d, the tree holds %d `func Fuzz` targets", floor, targets)
	}
}
