//go:build perf

package perf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// The harness runs the release-flag binary and reads each child's peak RSS
// from wait4's rusage, the figure /usr/bin/time prints (bytes on darwin,
// KiB on Linux), and takes the median of nine runs (archive 85.3.13). It is
// behind the perf tag because it builds a binary and runs it some hundred
// times; the pure comparisons it decides with are in gate.go and runs.go and
// run with every test.

var (
	rashomonBin string
	root        string
	// ran holds every gate a comparison was made for (runs.go's names);
	// RASHOMON_PERF_REQUIRE_ALL=1 fails a run that skipped one it owed.
	ran = map[string]bool{}
)

const runs = 9

func TestMain(m *testing.M) {
	code, err := buildAndRun(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "perf:", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// buildAndRun refuses a toolchain other than the one the rows were measured
// under before it builds anything (exit 2), then runs the tests and, when the
// run is required to be whole, fails it for every owed gate that did not run.
func buildAndRun(m *testing.M) (int, error) {
	var err error
	root, err = filepath.Abs("../..")
	if err != nil {
		return 0, err
	}
	b, err := readBudgets(filepath.Join(root, "test", "perf", "budgets.json"))
	if err != nil {
		return 2, err
	}
	if err := b.CheckToolchain(runtime.Version()); err != nil {
		return 2, err
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
	code := m.Run()
	if os.Getenv("RASHOMON_PERF_REQUIRE_ALL") == "1" && code == 0 {
		if missing := Ungated(b, ran); len(missing) > 0 {
			return 1, fmt.Errorf("RASHOMON_PERF_REQUIRE_ALL: %d owed gates did not run: %s", len(missing), strings.Join(missing, ", "))
		}
	}
	return code, nil
}

func readBudgets(path string) (budget.Budgets, error) {
	var b budget.Budgets
	raw, err := os.ReadFile(path)
	if err != nil {
		return b, err
	}
	return b, json.Unmarshal(raw, &b)
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

// overBudget records gate as run and reports a failure, with the schedule's
// note when the schedule, not the code, is what moved.
func overBudget(t *testing.T, b budget.Budgets, gate, id string, measured, budgetMiB float64) {
	t.Helper()
	p := b.Platforms[platform()]
	tol := tolerance(t, b)
	ran[gate] = true
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
// returns its peak RSS in MiB and what it printed.
func maxRSSMiB(t *testing.T, stdin []byte, env []string, args ...string) (float64, []byte) {
	t.Helper()
	cmd := exec.Command(rashomonBin, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
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
	return rss / (1 << 20), stdout.Bytes()
}

// rig is one measurement's isolated world: a store pinned to payload's
// identity with the settings entries installed (so coverage resolves the
// healthy path, not hook_entry_absent), a Claude Code config directory of its
// own, and the proxy variables stripped.
type rig struct {
	home      string
	env       []string
	installID string
}

func newRig(t *testing.T) rig {
	t.Helper()
	home, cfg := t.TempDir(), t.TempDir()
	if err := payload.Pin(home); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := settings.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Apply(doc, install.Spec{Executable: rashomonBin, InstallID: st.InstallID()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "settings.json"), doc.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "RASHOMON_HOME", "GODEBUG", "CLAUDE_CONFIG_DIR", "RASHOMON_MANAGED_SETTINGS_PATH":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "RASHOMON_HOME="+home, "CLAUDE_CONFIG_DIR="+cfg, "RASHOMON_MANAGED_SETTINGS_PATH="+filepath.Join(cfg, "absent"))
	return rig{home: home, env: env, installID: st.InstallID()}
}

// args is command as an installed settings entry runs it.
func (r rig) args(command []string) []string {
	return append(append([]string(nil), command...), install.Marker, r.installID)
}

// lines maps every file under the store named name to its lines.
func (r rig) lines(t *testing.T, name string) map[string][][]byte {
	t.Helper()
	out := map[string][][]byte{}
	err := filepath.WalkDir(r.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) != name {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = bytes.SplitAfter(b, []byte("\n"))
		if n := len(out[path]); n > 0 && len(out[path][n-1]) == 0 {
			out[path] = out[path][:n-1]
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// added is what the store gained between two reads of one file name.
func added(before, after map[string][][]byte) [][]byte {
	paths := make([]string, 0, len(after))
	for p := range after {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var out [][]byte
	for _, p := range paths {
		out = append(out, after[p][min(len(before[p]), len(after[p])):]...)
	}
	return out
}

// medianRSS runs command runs times on mk's payloads and returns the median
// peak RSS. Every run must pass runs.go's WorkDone on what it added to the
// store and printed; recap.json is removed before each, so each recap is the
// turn's first, as the recap benchmarks do.
func (r rig) medianRSS(t *testing.T, command []string, mk func(i int) []byte) float64 {
	t.Helper()
	var xs []float64
	for i := 0; i < runs; i++ {
		if err := os.Remove(filepath.Join(r.home, "recap.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		in := mk(i)
		recs, cov := r.lines(t, store.FileRecords), r.lines(t, store.FileCoverage)
		rss, stdout := maxRSSMiB(t, in, r.env, r.args(command)...)
		a := Added{Records: len(added(recs, r.lines(t, store.FileRecords))), Coverage: added(cov, r.lines(t, store.FileCoverage)), Stdout: stdout}
		if err := WorkDone(command, payload.Session, a); err != nil {
			t.Fatalf("run %d of %s did not do its work: %v", i, strings.Join(command, " "), err)
		}
		xs = append(xs, rss)
	}
	m, _ := Median(xs)
	return m
}

// TestRES28_MaxRSSFollowsThePayloadBudget: hook and post peak RSS stay within
// 6.5 MiB + 2.2 MiB per MiB of payload on every case that gates in this tree
// (CaseGates); the cases that gate from P-10 are skipped by name with the
// reason, never silently (RES-28).
func TestRES28_MaxRSSFollowsThePayloadBudget(t *testing.T) {
	b := loadBudgets(t)
	for _, c := range b.RSSCases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			if !CaseGates(c) {
				t.Skipf("gates from %s: intake copies the payload about eight times until RES-4's read bound lands (archive 85.1.2)", c.GatesFrom)
			}
			r := newRig(t)
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
			got := r.medianRSS(t, []string{c.Entry}, mk)
			overBudget(t, b, CaseGate(c.ID), c.ID, got, c.BudgetMiB)
		})
	}
}

// TestRES28b_EveryEntryStaysWithinItsRow: every entry row's peak-RSS p50
// holds on this platform at its payload class, on the store SeedFor names; a
// platform without its own figures (Linux before its baseline run) uses the
// reference row and says so.
func TestRES28b_EveryEntryStaysWithinItsRow(t *testing.T) {
	b := loadBudgets(t)
	overrides := b.Platforms[platform()].Entries
	for _, e := range b.Entries {
		e := e
		t.Run(e.ID, func(t *testing.T) {
			budgetP50 := e.MaxRSSMiBP50
			if o, ok := overrides[e.ID]; ok {
				budgetP50 = o.P50
			} else {
				t.Logf("%s: no %s override; gating against the reference row", e.ID, platform())
			}
			sd, err := SeedFor(e)
			if err != nil {
				t.Fatal(err)
			}
			r := newRig(t)
			r.seed(t, sd)
			got := r.medianRSS(t, e.Command, entryPayload(t, e))
			overBudget(t, b, EntryGate(e.ID), e.ID, got, budgetP50)
		})
	}
}

// seed writes the store an entry row is measured on.
func (r rig) seed(t *testing.T, sd Seed) {
	t.Helper()
	if sd.Calls > 0 {
		if _, err := payload.WriteSession(r.home, sd.Calls, payload.Prompts); err != nil {
			t.Fatal(err)
		}
	}
	if sd.ProbeStart {
		start := []string{"probe", "start"}
		before := r.lines(t, store.FileCoverage)
		_, out := maxRSSMiB(t, payload.SessionStart(), r.env, r.args(start)...)
		a := Added{Coverage: added(before, r.lines(t, store.FileCoverage)), Stdout: out}
		if err := WorkDone(start, payload.Session, a); err != nil {
			t.Fatalf("seeding a started session: %v", err)
		}
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
			p = payload.Stop(payload.FinalMessage)
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

// TestRES2_InitBudget: process init allocates at most 64 KiB and runs no GC
// cycle, read from the runtime's own trace of the release binary (RES-2).
func TestRES2_InitBudget(t *testing.T) {
	cmd := exec.Command(rashomonBin, "version")
	cmd.Env = append(newRig(t).env, "GODEBUG=inittrace=1,gctrace=1")
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
	ran[InitGate] = true
	t.Logf("init: %d packages, %d bytes, %d allocs, %d gc lines", tr.Packages, tr.Bytes, tr.Allocs, tr.GCLines)
}
