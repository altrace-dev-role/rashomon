//go:build perf

package perf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/steer"
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
	// Before the reaper exists, so it and every child inherit the flag; see
	// peakrss.DisableTHP for what it takes out of a reading. A host that
	// refuses it fails the run rather than measuring with 2 MiB steps in.
	if err := peakrss.DisableTHP(); err != nil {
		return 0, err
	}
	fmt.Fprintln(os.Stderr, "perf:", hostSettings())
	if reaper, err = peakrss.NewReaper(dir); err != nil && !errors.Is(err, peakrss.ErrUnavailable) {
		return 0, err
	}
	return Settle(m.Run(), os.Getenv(RequireAllEnv) == "1", b, ran)
}

// hostSettings is one line naming what a reading depends on that the harness
// does not set: the kernel's THP mode and the CPUs the test process may use,
// so a bimodal row's runner can be read from its log rather than guessed
// (budgets.json, linux/amd64's set_from). Off Linux, or where sysfs is
// absent, the THP part says so.
func hostSettings() string {
	thp := "not read"
	if b, err := os.ReadFile("/sys/kernel/mm/transparent_hugepage/enabled"); err == nil {
		thp = strings.TrimSpace(string(b))
	}
	return fmt.Sprintf("%s, THP sysfs %s (disabled for the harness and its children), GOMAXPROCS %d of %d CPUs, measured children under %v",
		platform(), thp, runtime.GOMAXPROCS(0), runtime.NumCPU(), measuredEnv(nil, runtime.GOOS))
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

// leakBytes is what the harness holds while it measures, and leakLimitMiB is
// where a reading would show it: a child whose reading carried the harness's
// peak would read 96 MiB or more, so a reading under half of that is a
// reading that left the harness out.
const (
	leakBytes    = 96 << 20
	leakLimitMiB = 48
	// leakPayload is the child's work: `rashomon hook` on a 4 MiB payload
	// reads 16.5 to 21.2 MiB on ubuntu-latest (budgets.json, hook.4MiB) and
	// 14 to 18 on this repository's reference Linux VM, which is over 10 MiB
	// above the reaper's own peak wherever that peak has been read (about 6
	// MiB on ubuntu-latest, 2.6 on the VM) and under half the limit.
	leakPayload = 4 << 20
)

// TestRES28p_AReadingLeavesOutTheHarnesssOwnPeak: the harness's reading of
// `rashomon hook` on a 4 MiB payload stays under 48 MiB while the harness
// itself holds 96 MiB. On Linux a child's ru_maxrss is at least its vforking
// parent's peak, so a harness that read the child's rusage itself would read
// its own size.
//
// The child is one whose own peak clears the reaper's by a wide margin, and
// the reading must be attributable (maxRSSMiB fails on a floor reading, as it
// is meant to here): a reading at the reaper's floor is the reaper's, proves
// nothing about the harness, and is what `rashomon version` gave on
// ubuntu-latest in three of eleven runs: a process that does almost nothing
// peaks there within 0.2 MiB of the reaper (5.77 to 5.91 MiB against 5.97 to
// 5.98 in the three; 5.23 and 7.24 to 7.38 in the others, the 2 MiB steps
// every row on that runner shows). The test is not relaxed to accept a floor
// reading: it still fails on one, and a harness that leaks its peak reads far
// above the floor, so it is the attributable reading that tells the two
// apart.
func TestRES28p_AReadingLeavesOutTheHarnesssOwnPeak(t *testing.T) {
	ballast := make([]byte, leakBytes)
	for i := 0; i < len(ballast); i += os.Getpagesize() {
		ballast[i] = 1
	}
	r := newRig(t)
	in, err := payload.Pre(leakPayload, "toolu_00")
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != leakPayload {
		t.Fatalf("payload is %d bytes, want %d", len(in), leakPayload)
	}
	command := []string{"hook"}
	before := r.lines(t, store.FileRecords)
	got, stdout := maxRSSMiB(t, in, r.env, r.args(command)...)
	runtime.KeepAlive(ballast)
	// A reading of a call that stood down early would be a small child's, not
	// the one this measures.
	a := Added{Records: len(added(before, r.lines(t, store.FileRecords))), Stdout: stdout}
	if err := WorkDone(command, payload.Session, a); err != nil {
		t.Fatalf("the measured call did not do its work: %v", err)
	}
	if got >= leakLimitMiB {
		t.Errorf("rashomon hook read %.2f MiB beside a harness holding 96 MiB: the harness's peak is in the reading", got)
	}
	t.Logf("rashomon hook on %d MiB: %.2f MiB beside a 96 MiB harness", leakPayload>>20, got)
}

// measuredEnv is env plus the runtime settings every Linux reading is taken
// under: GOMAXPROCS=1 and GODEBUG=gcstoptheworld=2. They take out the second
// source of bimodal readings besides transparent huge pages
// (peakrss.DisableTHP). With a second P the runtime's background GC workers,
// sweeper and scavenger run beside the goroutine that reads the payload, and
// whether a cycle (and its sweep) has freed the chunk buffers before the next
// allocation touches new pages decides whether `rashomon hook` peaks one
// payload higher. On this repository's 4-core VM hook.8MiB read 22.1 to 22.9
// MiB in 17 of 40 runs and 28.9 to 30.6 in 23 (the second mode is one 8 MiB
// buffer more) and hook.4MiB 13.8 to 14.7 or 16.0 to 18.6. GOMAXPROCS=1
// alone collapses those two but leaves hook.1MiB in two clusters 0.7 MiB apart
// (7.3 to 7.5 and 8.0 to 8.3, about even) and an occasional 17.5 at 4 MiB (1
// to 2 of 40), presumably because the background sweeper still interleaves
// with the reader on one P. gcstoptheworld=2 stops the concurrent mark and the
// concurrent sweep, and with both set all 18 cases and entries read one cluster
// in 60 runs each (at most 0.9 MiB wide), with THP "always" and "madvise".
// GOGC pinned does nothing here (the binary pins GOGC 100 and a 32 MiB limit
// itself, applyHookRuntimeLimits), and a forced collection is already in the
// binary (hookio.Read, from 1 MiB), so neither is a control.
//
// This is not what a user runs: users have cores and a concurrent collector,
// and the concurrent peak is the one they can see (up to one payload higher
// at 4 and 8 MiB). The gate holds the serialised peak, the one a change in
// what the code holds moves, not a change in when the runtime frees it.
// Darwin is left as it was: its rows are the reference machine's and
// Karthik re-measures them (budgets.json, platforms.darwin/arm64). GODEBUG is
// stripped from the rig's env (newRig), so this is the only GODEBUG a
// measured child has.
func measuredEnv(env []string, goos string) []string {
	if goos != "linux" {
		return env
	}
	return append(append([]string(nil), env...), "GOMAXPROCS=1", "GODEBUG=gcstoptheworld=2")
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
	cmd.Env = measuredEnv(env, runtime.GOOS)
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
	// A consent naming record_scope v5, as watch writes it, with its marker:
	// the hook then records the content fields and the probe finds the
	// marker matching, the path every watched install runs. Without it the
	// hook would withhold them (1.2q) and these rows would read a lighter
	// path than the one they budget.
	if err := consent.Append(home, consent.Record{
		V: consent.RecordVersion, ScopeVersion: disclose.ScopeVersion, RecordScope: consent.RecordScopeV5,
		Steer: consent.SteerOn, Via: consent.ViaWatchYes, At: 1760000000000,
	}); err != nil {
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
	// CLAUDE_CODE_ENTRYPOINT=cli: see test/acceptance/harness_test.go's
	// environ for why every hook-invoking test env needs this now that H1
	// (internal/hostguard) is wired into the five entry points -- without
	// it, every rig-driven hook/post/probe call reads as the unknown
	// harness and stands down before doing the work these RSS gates meter.
	env = append(env, "RASHOMON_HOME="+home, "CLAUDE_CONFIG_DIR="+cfg, "RASHOMON_MANAGED_SETTINGS_PATH="+filepath.Join(cfg, "absent"), "CLAUDE_CODE_ENTRYPOINT=cli")
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
	// The nine readings, not only their median: a median that sits in one of
	// two modes (post.edit_314k and recap.pending read about 2 MiB apart on
	// ubuntu-latest across CI runs) cannot be told from a drift of the floor
	// by one number.
	t.Logf("%s: nine peak RSS readings in MiB: %.2f", strings.Join(command, " "), xs)
	return m, true, ""
}

// TestRES28u_MeasuredEnvPinsOneProcOnLinuxOnly: every Linux reading is taken
// under GOMAXPROCS=1 and GODEBUG=gcstoptheworld=2 (measuredEnv), the
// caller's own environment is not altered, and darwin's is exactly what the
// caller passed, since its rows are the reference machine's. Break: set them on
// every platform, or drop either, and this fails.
func TestRES28u_MeasuredEnvPinsOneProcOnLinuxOnly(t *testing.T) {
	in := make([]string, 1, 4) // spare capacity: an append in place would show in in[:2]
	in[0] = "A=1"
	got := measuredEnv(in, "linux")
	if len(got) != 3 || got[0] != "A=1" || got[1] != "GOMAXPROCS=1" || got[2] != "GODEBUG=gcstoptheworld=2" {
		t.Errorf("linux env = %q, want A=1, GOMAXPROCS=1 and GODEBUG=gcstoptheworld=2 last", got)
	}
	if in[:3][1] != "" {
		t.Errorf("the caller's slice was changed: %q", in[:3])
	}
	if got := measuredEnv(in, "darwin"); len(got) != 1 || got[0] != "A=1" {
		t.Errorf("darwin env = %q, want it untouched", got)
	}
}

// TestRES28_MaxRSSFollowsThePayloadBudget: hook and post peak RSS stay within
// each case's line in budgets.json (6.5 MiB plus the case's slope per MiB of
// payload; RES-28's 2.2 unless a basis says otherwise) on every case whose PR
// has landed (CaseGates): the small cases from P-08, the 1, 4 and 8 MiB cases
// from P-10's bounded intake. A case from a PR not yet landed is skipped by
// name, never silently (RES-28).
func TestRES28_MaxRSSFollowsThePayloadBudget(t *testing.T) {
	b := loadBudgets(t)
	for _, c := range b.RSSCases {
		c := c
		t.Run(c.ID, func(t *testing.T) {
			if !CaseGates(c) {
				t.Skipf("gates from %s, which has not landed", c.GatesFrom)
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
			if c.Entry == "recap" {
				// The Stop's transcript scan (CE-1): the real recap entry on a
				// seeded session, over a transcript of PayloadBytes, with a
				// delivered note the transcript does not hold.
				r.seed(t, Seed{Calls: recapSeedCalls})
				mk = r.scanStop(t, c.PayloadBytes)
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
				// The reading rides in the failure: the row this asks for is
				// the platform's own measurement, and a failure that did not
				// say what was read left nobody a way to write it.
				t.Errorf("%s: read %.2f MiB; %s", c.ID, got, why)
			}
		})
	}
}

// recapSeedCalls is the session size recap.stop's row is measured on
// (BenchmarkRecap/100), so the scan case differs from it by the scan alone.
const recapSeedCalls = 100

// scanStop returns the payload builder of the recap.stop scan case: a
// transcript of exactly size bytes, of 2 KiB records with a 1 MiB line (held,
// at the step-over's edge) and a 1 MiB + 1 line (stepped over) every 2 MiB, and
// no attachment holding the note, so the scan reads all of it and finds
// nothing, the cost of an absent note. Before each run it resets the
// session's steer log to one settled delivered note: the Stop appends an
// evidence line, after which a later run would judge nothing and scan nothing.
// A run's evidence line is checked before the reset, so a run that did not
// scan fails the case.
func (r rig) scanStop(t *testing.T, size int) func(i int) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, scanTranscript(size), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(r.home)
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(st.RunDir(payload.Session), store.FileSteer)
	stop, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": payload.Session, "transcript_path": path,
		"cwd": payload.CWD, "stop_hook_active": false, "last_assistant_message": payload.FinalMessage})
	if err != nil {
		t.Fatal(err)
	}
	note := steer.Line{Type: steer.LineType, V: steer.LineVersion, ID: "note-1", SessionID: payload.Session,
		AtUnixMS: time.Now().Add(-time.Minute).UnixMilli(), Event: "PostToolUseFailure",
		State: steer.LineStateDelivered, ModelText: "rashomon record: a note that is not in the transcript"}
	return func(i int) []byte {
		if i > 0 {
			b, err := os.ReadFile(log)
			if err != nil || !bytes.Contains(b, []byte(`"evidence":"absent"`)) {
				t.Fatalf("run %d did not scan the transcript: no absent evidence line in the steer log (%v)", i-1, err)
			}
		}
		if err := os.Remove(log); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		note.AtUnixMS = time.Now().Add(-time.Minute).UnixMilli()
		l := note
		if err := st.AppendSteer(payload.Session, func(io.ReaderAt) ([]store.SteerLine, error) { return []store.SteerLine{&l}, nil }); err != nil {
			t.Fatal(err)
		}
		if err := st.Flush(); err != nil {
			t.Fatal(err)
		}
		return stop
	}
}

// scanTranscript is size bytes of transcript lines, each ending in a newline.
func scanTranscript(size int) []byte {
	const step = 1 << 20 // linescan's bound in steer.DefaultScanLimits
	fill := func(n int, kind string) []byte {
		head := `{"type":"user","timestamp":"2026-10-30T00:00:00.000Z","message":{"role":"user","content":"` + kind
		tail := `"}}` + "\n"
		return []byte(head + strings.Repeat("x", n-len(head)-len(tail)) + tail)
	}
	out := make([]byte, 0, size)
	for k := 0; len(out) < size; k++ {
		var line []byte
		switch {
		case k%1000 == 500: // held: step-1 bytes and its newline are the longest line read
			line = fill(step, "held ")
		case k%1000 == 501: // one over: stepped over
			line = fill(step+1, "over ")
		default:
			line = fill(2048, "")
		}
		if rest := size - len(out); len(line) > rest {
			if line = fill(max(rest, 200), "last "); rest < 200 {
				line = bytes.Repeat([]byte("\n"), rest) // blank lines: the scan skips them
			}
		}
		out = append(out, line...)
	}
	return out
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
				t.Errorf("%s: read %.2f MiB; %s", e.ID, got, why)
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
