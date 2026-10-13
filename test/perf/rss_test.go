//go:build perf

package perf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// The harness runs the release-flag binary and reads each child's peak RSS
// from wait4's rusage, the figure /usr/bin/time prints (bytes on darwin,
// KiB on Linux), and takes the median of nine runs (archive 85.3.13). It is
// behind the perf tag because it builds a binary and runs it some hundred
// times; the pure comparisons it decides with are in gate.go and run with
// every test.

var (
	rashomonBin string
	root        string
)

const runs = 9

func TestMain(m *testing.M) {
	code, err := buildAndRun(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "perf:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func buildAndRun(m *testing.M) (int, error) {
	var err error
	root, err = filepath.Abs("../..")
	if err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp("", "rashomon-perf-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	rashomonBin = filepath.Join(dir, "rashomon")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", rashomonBin, "./cmd/rashomon")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("building the release binary: %v\n%s", err, out)
	}
	return m.Run(), nil
}

// platform is this runner's budgets platform key.
func platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// tolerance is the fraction over budget this runner is allowed today.
func tolerance(t *testing.T, b budget.Budgets) float64 {
	t.Helper()
	p, ok := b.Platforms[platform()]
	if !ok {
		t.Skipf("no budgets platform for %s", platform())
	}
	return Tolerance(p.Tolerance, p.ToleranceUntil, p.ToleranceAfter, time.Now())
}

// overBudget reports a failure with the schedule's note when the schedule,
// not the code, is what moved.
func overBudget(t *testing.T, b budget.Budgets, id string, measured, budgetMiB float64) {
	t.Helper()
	p := b.Platforms[platform()]
	tol := tolerance(t, b)
	if !OverBudget(measured, budgetMiB, tol) {
		t.Logf("%s: %.2f MiB against %.2f (+%.0f%%)", id, measured, budgetMiB, tol*100)
		return
	}
	note := ToleranceNote(measured, budgetMiB, p.Tolerance, p.ToleranceUntil, p.ToleranceAfter, time.Now())
	if note != "" {
		note = "; " + note
	}
	t.Errorf("%s: median peak RSS %.2f MiB over budget %.2f MiB (+%.0f%%)%s", id, measured, budgetMiB, tol*100, note)
}

// maxRSSMiB runs the binary once with stdin and env, requires exit 0, and
// returns its peak RSS in MiB.
func maxRSSMiB(t *testing.T, stdin []byte, env []string, args ...string) float64 {
	t.Helper()
	cmd := exec.Command(rashomonBin, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("rashomon %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Fatal("no rusage for the child")
	}
	rss := float64(ru.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return rss / (1 << 20)
}

// scratch is an isolated store root with the proxy variables stripped.
func scratch(t *testing.T) (home string, env []string) {
	t.Helper()
	home = t.TempDir()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "RASHOMON_HOME", "GODEBUG":
			continue
		}
		env = append(env, kv)
	}
	return home, append(env, "RASHOMON_HOME="+home)
}

// recordLines counts the lines of every records file under the store, the
// work-done check: a hook that refused its payload writes none.
func recordLines(t *testing.T, home string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, "records.ndjson") {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			n += bytes.Count(b, []byte("\n"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func medianRSS(t *testing.T, mk func(i int) []byte, env []string, args ...string) float64 {
	t.Helper()
	var xs []float64
	for i := 0; i < runs; i++ {
		xs = append(xs, maxRSSMiB(t, mk(i), env, args...))
	}
	m, _ := Median(xs)
	return m
}

// TestRES28_MaxRSSFollowsThePayloadBudget: hook and post peak RSS stay within
// 6.5 MiB + 2.2 MiB per MiB of payload on every case that gates from this
// PR; the cases that gate from P-10 are skipped by name with the reason,
// never silently (RES-28).
func TestRES28_MaxRSSFollowsThePayloadBudget(t *testing.T) {
	b := loadBudgets(t)
	for _, c := range b.RSSCases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			if c.GatesFrom != "P-08" {
				t.Skipf("gates from %s: intake copies the payload about eight times until RES-4's read bound lands (archive 85.1.2)", c.GatesFrom)
			}
			home, env := scratch(t)
			mk := func(i int) []byte {
				id := fmt.Sprintf("toolu_%02d", i)
				var p []byte
				var err error
				if c.Entry == "hook" {
					p, err = payload.Pre(c.PayloadBytes, id)
				} else {
					p, err = payload.Post(c.PayloadBytes, id)
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(p) != c.PayloadBytes {
					t.Fatalf("payload is %d bytes, want %d", len(p), c.PayloadBytes)
				}
				return p
			}
			before := recordLines(t, home)
			got := medianRSS(t, mk, env, c.Entry)
			if after := recordLines(t, home); after < before+runs {
				t.Fatalf("%d records after %d runs: the entry did not do its work", after-before, runs)
			}
			overBudget(t, b, c.ID, got, c.BudgetMiB)
		})
	}
}

// TestRES28b_EveryEntryStaysWithinItsRow: every entry row's peak-RSS p50
// holds on this platform at its payload class; a platform without its own
// figures (Linux before its baseline run) uses the reference row and says so.
func TestRES28b_EveryEntryStaysWithinItsRow(t *testing.T) {
	b := loadBudgets(t)
	overrides := b.Platforms[platform()].Entries
	for _, e := range b.Entries {
		e := e
		t.Run(e.ID, func(t *testing.T) {
			budgetP50 := e.MaxRSSMiBP50
			if o, ok := overrides[e.ID]; ok {
				budgetP50 = o.P50
			}
			home, env := scratch(t)
			mk := entryPayload(t, e)
			if strings.HasPrefix(e.ID, "recap.stop") {
				seed(t, env, 100)
			}
			before := recordLines(t, home)
			got := medianRSS(t, mk, env, e.Command...)
			if e.Command[0] == "hook" || e.Command[0] == "post" {
				if after := recordLines(t, home); after < before+runs {
					t.Fatalf("%d records after %d runs: the entry did not do its work", after-before, runs)
				}
			}
			overBudget(t, b, e.ID, got, budgetP50)
		})
	}
}

// entryPayload builds the payload an entry row's class names.
func entryPayload(t *testing.T, e budget.Entry) func(i int) []byte {
	t.Helper()
	return func(i int) []byte {
		id := fmt.Sprintf("toolu_%02d", i)
		var p []byte
		var err error
		switch e.PayloadClass {
		case "bash_short":
			if e.Command[0] == "hook" {
				p, err = payload.BashPreExact(e.PayloadBytes, id)
			} else {
				p, err = payload.BashPostExact(e.PayloadBytes, id)
			}
		case "edit_50k":
			p, err = payload.EditPre(e.PayloadBytes, id)
		case "edit_314k":
			p, err = payload.EditPost(e.PayloadBytes, id)
		case "stop_100":
			p = payload.Stop("Done: the tests pass.")
		case "prompt":
			p = payload.Prompt("continue")
		case "session_start":
			p = payload.SessionStart()
		case "session_end":
			p = payload.SessionEnd()
		default:
			t.Fatalf("entry %s: unknown payload class %q", e.ID, e.PayloadClass)
		}
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
}

// seed records calls into the store through the binary itself, so a recap
// row is measured on the session size its budget names.
func seed(t *testing.T, env []string, calls int) {
	t.Helper()
	for i := 0; i < calls; i++ {
		id := fmt.Sprintf("toolu_seed_%04d", i)
		maxRSSMiB(t, payload.BashPre("git status --short", id), env, "hook")
		maxRSSMiB(t, payload.BashPost("git status --short", id, " M README.md\n"), env, "post")
	}
}

// TestRES2_InitBudget: process init allocates at most 64 KiB and runs no GC
// cycle, read from the runtime's own trace of the release binary (RES-2).
func TestRES2_InitBudget(t *testing.T) {
	_, env := scratch(t)
	cmd := exec.Command(rashomonBin, "version")
	cmd.Env = append(env, "GODEBUG=inittrace=1,gctrace=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("rashomon version: %v\n%s", err, stderr.String())
	}
	tr, err := ParseInitTrace(stderr.String())
	if err != nil {
		t.Fatal(err)
	}
	if tr.Packages == 0 {
		t.Fatal("the trace reported no package init: GODEBUG did not reach the binary")
	}
	if tr.Bytes > InitBudgetBytes {
		t.Errorf("init allocated %d bytes over %d packages, budget %d", tr.Bytes, tr.Packages, InitBudgetBytes)
	}
	if tr.GCLines > 0 {
		t.Errorf("%d GC cycles during init", tr.GCLines)
	}
	t.Logf("init: %d packages, %d bytes, %d allocs, %d gc lines", tr.Packages, tr.Bytes, tr.Allocs, tr.GCLines)
}
