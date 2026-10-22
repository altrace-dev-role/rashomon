//go:build perf

package perf

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
	"github.com/altrace-dev-role/rashomon/test/perf/peakrss"
)

// The harness runs the release-flag binary and reads each child's peak RSS
// from wait4's rusage, the figure /usr/bin/time prints, through
// test/perf/peakrss, which reads it in each platform's unit and keeps this
// test binary's own peak out of it on Linux; it takes the median of nine runs
// (archive 85.3.13). It is behind the perf tag because it builds a binary and
// runs it some hundred times; the pure comparisons it decides with are in
// gate.go and runs.go and run with every test.

var (
	rashomonBin string
	root        string
	reaper      *peakrss.Reaper
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
// under before it builds anything (Preflight, exit 2), then runs the tests
// and settles the code: a run required to be whole fails for every owed gate
// that did not run. harness_test.go pins these calls by reading this file.
func buildAndRun(m *testing.M) (int, error) {
	var err error
	root, err = filepath.Abs("../..")
	if err != nil {
		return 0, err
	}
	b, err := Preflight(filepath.Join(root, "test", "perf", "budgets.json"), runtime.Version())
	if err != nil {
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
	if reaper, err = peakrss.NewReaper(dir); err != nil && !errors.Is(err, peakrss.ErrUnavailable) {
		return 0, err
	}
	return Settle(m.Run(), os.Getenv(RequireAllEnv) == "1", b, ran)
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

// reportOnlyRSS records gate as run without comparing measured with
// anything: the platform has no reading of its own to gate against, and
// gating against another platform's row would be a guess, not a budget.
func reportOnlyRSS(t *testing.T, gate, id string, measured float64, why string) {
	t.Helper()
	ran[gate] = true
	t.Logf("%s: %.2f MiB, report-only on %s: %s", id, measured, platform(), why)
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

// TestRES28p_AReadingLeavesOutTheHarnesssOwnPeak: the harness's reading of
// `rashomon version` stays under 48 MiB while the harness itself holds 96 MiB.
// On Linux a child's ru_maxrss is at least its vforking parent's peak, so a
// harness that read the child's rusage itself would read its own size.
func TestRES28p_AReadingLeavesOutTheHarnesssOwnPeak(t *testing.T) {
	ballast := make([]byte, 96<<20)
	for i := 0; i < len(ballast); i += os.Getpagesize() {
		ballast[i] = 1
	}
	got, _ := maxRSSMiB(t, nil, newRig(t).env, "version")
	runtime.KeepAlive(ballast)
	if got >= 48 {
		t.Errorf("rashomon version read %.2f MiB beside a harness holding 96 MiB: the harness's peak is in the reading", got)
	}
	t.Logf("rashomon version: %.2f MiB beside a 96 MiB harness", got)
}

// runOnce runs the binary once with stdin and env and returns its peak RSS
// in MiB, what it printed, what it wrote to stderr, and the reaper's error,
// if any. A platform with no ru_maxrss skips by name (peakrss.ErrUnavailable,
// which no caller can usefully act on); any other error, including one
// errors.Is(err, peakrss.ErrBelowFloor) (the reaper could not attribute this
// particular reading to the child, test/perf/peakrss), is returned for the
// caller to decide rather than failed here.
func runOnce(t *testing.T, stdin []byte, env []string, args ...string) (rss float64, stdout, stderr []byte, err error) {
	t.Helper()
	cmd := exec.Command(rashomonBin, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	rss, err = reaper.Run(cmd)
	if errors.Is(err, peakrss.ErrUnavailable) {
		t.Skip(err)
	}
	return rss, outBuf.Bytes(), errBuf.Bytes(), err
}

// maxRSSMiB runs the binary once with stdin and env, requires a reading the
// reaper could attribute to it, and returns its peak RSS in MiB and what it
// printed. Callers that measure the harness's own mechanism (TestRES28p_) or
// seed a store (rig.seed) want exactly this all-or-nothing behaviour; a run
// gated per platform (rig.medianRSS) wants runOnce directly, so it can tell
// "the reaper's own floor rules this out" apart from a real failure.
func maxRSSMiB(t *testing.T, stdin []byte, env []string, args ...string) (float64, []byte) {
	t.Helper()
	rss, stdout, stderr, err := runOnce(t, stdin, env, args...)
	if err != nil {
		t.Fatalf("rashomon %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return rss, stdout
}

// floorOutcome reports whether err is the reaper refusing to attribute this
// one reading to the child (peakrss.ErrBelowFloor): true with the reaper's
// own reason to report, rather than a per-platform list guessed in advance
// of ever measuring (archive 85.4.4 found that list reporting two entries as
// unmeasurable on a runner where they had, in fact, read above its floor).
// false, "" for every other outcome, nil included -- the caller still Fatals
// on any other error.
func floorOutcome(err error) (notAttributable bool, why string) {
	if errors.Is(err, peakrss.ErrBelowFloor) {
		return true, err.Error()
	}
	return false, ""
}

// TestRES28s_FloorOutcomeNamesTheReapersOwnReason: nil and an ordinary error
// are not the floor (medianRSS still Fatals on those); only
// peakrss.ErrBelowFloor, wrapped or bare, reports with the reaper's own
// reason. This is checked against synthetic errors, not a real low-floor
// runner, which this harness cannot manufacture on demand (RES-28s).
func TestRES28s_FloorOutcomeNamesTheReapersOwnReason(t *testing.T) {
	if got, why := floorOutcome(nil); got || why != "" {
		t.Errorf("floorOutcome(nil) = %v, %q, want false, \"\"", got, why)
	}
	if got, why := floorOutcome(fmt.Errorf("some other failure")); got || why != "" {
		t.Errorf("floorOutcome(other) = %v, %q, want false, \"\"", got, why)
	}
	if got, _ := floorOutcome(peakrss.ErrBelowFloor); !got {
		t.Error("floorOutcome(peakrss.ErrBelowFloor) = false, want true")
	}
	wrapped := fmt.Errorf("peakrss: the child's peak 2.10 MiB does not exceed the reaper's own 2.10 MiB, so it may be the reaper's: %w", peakrss.ErrBelowFloor)
	if got, why := floorOutcome(wrapped); !got || !strings.Contains(why, "reaper's own") {
		t.Errorf("floorOutcome(wrapped) = %v, %q, want true naming the reaper's own peak", got, why)
	}
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
//
// attributable is false, with why from the reaper (RES-28s), the moment any
// run's reading cannot be told apart from the reaper's own (peakrss.
// ErrBelowFloor): the caller must report that run-only rather than gate or
// fail on it, and nothing further is measured, since every run on this
// platform hits the same floor.
func (r rig) medianRSS(t *testing.T, command []string, mk func(i int) []byte) (median float64, attributable bool, why string) {
	t.Helper()
	var xs []float64
	for i := 0; i < runs; i++ {
		if err := os.Remove(filepath.Join(r.home, "recap.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		in := mk(i)
		recs, cov := r.lines(t, store.FileRecords), r.lines(t, store.FileCoverage)
		rss, stdout, stderr, err := runOnce(t, in, r.env, r.args(command)...)
		if notAttributable, floorWhy := floorOutcome(err); notAttributable {
			return 0, false, floorWhy
		}
		if err != nil {
			t.Fatalf("rashomon %s: %v\n%s", strings.Join(command, " "), err, stderr)
		}
		a := Added{Records: len(added(recs, r.lines(t, store.FileRecords))), Coverage: added(cov, r.lines(t, store.FileCoverage)), Stdout: stdout}
		if err := WorkDone(command, payload.Session, a); err != nil {
			t.Fatalf("run %d of %s did not do its work: %v", i, strings.Join(command, " "), err)
		}
		xs = append(xs, rss)
	}
	m, _ := Median(xs)
	return m, true, ""
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
			got, attributable, floorWhy := r.medianRSS(t, []string{c.Entry}, mk)
			if !attributable {
				reportOnlyRSS(t, CaseGate(c.ID), c.ID, got, floorWhy)
				return
			}
			budgetMiB, gated, reportOnly, why := caseBudget(platform(), c, b.Platforms[platform()].CaseBudgets)
			switch {
			case gated:
				overBudget(t, b, CaseGate(c.ID), c.ID, got, budgetMiB)
			case reportOnly:
				reportOnlyRSS(t, CaseGate(c.ID), c.ID, got, why)
			default:
				ran[CaseGate(c.ID)] = true
				t.Errorf("%s: %s", c.ID, why)
			}
		})
	}
}

// TestRES28b_EveryEntryStaysWithinItsRow: every entry row's peak-RSS p50
// holds on this platform at its payload class, on the store SeedFor names; a
// platform with no reading of its own (an entry Linux's floor cannot
// attribute, or one nobody has measured there yet) is reported, not gated
// against another platform's row (entryBudget).
func TestRES28b_EveryEntryStaysWithinItsRow(t *testing.T) {
	b := loadBudgets(t)
	overrides := b.Platforms[platform()].Entries
	for _, e := range b.Entries {
		e := e
		t.Run(e.ID, func(t *testing.T) {
			sd, err := SeedFor(e)
			if err != nil {
				t.Fatal(err)
			}
			r := newRig(t)
			r.seed(t, sd)
			got, attributable, floorWhy := r.medianRSS(t, e.Command, entryPayload(t, e))
			if !attributable {
				reportOnlyRSS(t, EntryGate(e.ID), e.ID, got, floorWhy)
				return
			}
			budgetP50, gated, reportOnly, why := entryBudget(platform(), e, overrides)
			switch {
			case gated:
				overBudget(t, b, EntryGate(e.ID), e.ID, got, budgetP50)
			case reportOnly:
				reportOnlyRSS(t, EntryGate(e.ID), e.ID, got, why)
			default:
				ran[EntryGate(e.ID)] = true
				t.Errorf("%s: %s", e.ID, why)
			}
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
