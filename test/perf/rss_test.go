//go:build perf

package perf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/spec"
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
// stripped from the rig's env (newRig), so the only other GODEBUG a measured
// child has is concurrentGCEnv, which a concurrentGCCases row sets itself and
// which measuredEnv leaves in place of gcstoptheworld=2.
func measuredEnv(env []string, goos string) []string {
	if goos != "linux" {
		return env
	}
	out := append(append([]string(nil), env...), "GOMAXPROCS=1")
	for _, kv := range env {
		if strings.HasPrefix(kv, "GODEBUG=") {
			return out
		}
	}
	return append(out, "GODEBUG=gcstoptheworld=2")
}

// concurrentGCCases are the rows whose Linux reading is taken at GOMAXPROCS=1
// with the collector left concurrent (concurrentGCEnv). The one row is the
// prompt's graph read: the prompt entry runs under the hook's 32 MiB soft
// limit (applyHookRuntimeLimits) and the store it loads is 16.9 MiB live on
// the synthetic index, so the heap goal sits at the live heap. The runtime's
// GC CPU limiter keeps such a process moving by skipping mark assists
// (runtime/mgcmark.go), but gcstoptheworld=2 runs every cycle with the world
// stopped, so there are no assists to skip and each new span starts another
// full cycle: one reading ran past three minutes and was killed, where the
// concurrent collector at one P reads it in 0.25 s. Users run the concurrent
// collector. The compact read model (PLAN, M2) fits the load in the limit's
// headroom, and the row leaves this list with it.
var concurrentGCCases = map[string]bool{graphPromptCase: true}

// concurrentGCEnv names the default collector, so measuredEnv adds no
// gcstoptheworld=2 to a concurrentGCCases row.
const concurrentGCEnv = "GODEBUG=gcstoptheworld=0"

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
// caller passed, since its rows are the reference machine's. The one
// exception is pinned by name: a concurrentGCCases row keeps GOMAXPROCS=1 and
// its own concurrentGCEnv, and recap.pending.graph8MiB is the only such row.
// Break: set them on every platform, drop either, let a row's GODEBUG lose
// GOMAXPROCS=1, or add a row to the exception, and this fails.
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
	got = measuredEnv([]string{"A=1", concurrentGCEnv}, "linux")
	if len(got) != 3 || got[1] != concurrentGCEnv || got[2] != "GOMAXPROCS=1" {
		t.Errorf("concurrent-GC linux env = %q, want A=1, %s and GOMAXPROCS=1, no gcstoptheworld=2", got, concurrentGCEnv)
	}
	if len(concurrentGCCases) != 1 || !concurrentGCCases[graphPromptCase] {
		t.Errorf("concurrent-GC rows = %v, want %s alone", concurrentGCCases, graphPromptCase)
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
			var control func()
			command := []string{c.Entry}
			if OrderSpecCase(c.ID) {
				dir := r.orderSpec(t)
				// Each measured post is preceded by its call's PreToolUse, as a
				// session's are: the order rule judges the post against the
				// call's own declaration. The pre is not measured.
				mk = func(i int) []byte {
					id := fmt.Sprintf("toolu_%02d", i)
					maxRSSMiB(t, payload.BashPreAt("git status --short", id, dir), r.env, r.args([]string{"hook"})...)
					p, err := payload.BashPostExactAt(c.PayloadBytes, id, dir)
					if err != nil {
						t.Fatal(err)
					}
					return p
				}
				r.premise(t, mk(99))
			}
			if c.Entry == "recap" {
				switch c.ID {
				case graphPromptCase:
					// The prompt's graph read (W-M1.8): the recap.pending
					// row's session and a prompt in the vocabulary of a
					// synthetic index of PayloadBytes.
					r.seed(t, Seed{Calls: pendingSeedCalls})
					command = []string{"recap", "pending"}
					mk, control = r.graphPrompt(t, c.PayloadBytes)
				case pgraphPromptCase:
					// The prompt's symbol read (PW1b-ON-230 step 3): the
					// recap.pending row's session and a prompt naming a
					// symbol the vouched 3 MiB log proves.
					r.seed(t, Seed{Calls: pendingSeedCalls})
					command = []string{"recap", "pending"}
					mk, control = r.pgraphPrompt(t, c.PayloadBytes)
				case pgraphCase:
					// The Stop's platform read (PW1b-ON-230 step 2): a
					// started session and one edit in a module whose .pgraph
					// holds a log of PayloadBytes the walk file vouches for.
					// Not the seeded session: its coverage sentence wins
					// over the line the premise reads (recap.Line).
					r.seed(t, Seed{ProbeStart: true})
					mk, control = r.pgraphStop(t, c.PayloadBytes)
				default:
					// The Stop's transcript scan (CE-1): the real recap entry
					// on a seeded session, over a transcript of PayloadBytes,
					// with a delivered note the transcript does not hold.
					r.seed(t, Seed{Calls: recapSeedCalls})
					mk = r.scanStop(t, c.PayloadBytes)
				}
			}
			if c.ID == blastCase {
				// The post's blast-radius read (ARCH-INLINE1): an Edit of a
				// Go file in a module whose .pgraph holds a vouched log of
				// PayloadBytes, in a session steering speaks to.
				r.env = append(r.env, blastEnv...)
				mk, control = r.blastPost(t, c.PayloadBytes)
			}
			if concurrentGCCases[c.ID] {
				r.env = append(r.env, concurrentGCEnv)
			}
			got, attributable, floorWhy := r.medianRSS(t, command, mk)
			if !attributable {
				reportOnlyRSS(t, CaseGate(c.ID), c.ID, got, floorWhy)
				return
			}
			if control != nil {
				// The same run with nothing to read, logged, so each platform's
				// log says what the read adds, the figure its row is from.
				control()
				if base, ok, _ := r.medianRSS(t, command, mk); ok {
					t.Logf("%s: %.2f MiB; the same run with nothing to read: %.2f MiB; the read adds %.2f MiB", c.ID, got, base, got-base)
				}
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

// orderSpecDoc is the spec of the order cases: one order rule whose then is
// the payload's own command (git status), so the measured call is the Then
// call of an order rule with no after_hit before it, the path that reads the
// run and raises a finding. It is a rule the call matches, not a rule beside
// it: the cost measured is the order rule's, not the spec's discovery alone.
const orderSpecDoc = `{"rashomon_spec": 1, "rules": [{"id": "status-first", "kind": "order",
 "then": {"argv": ["git", "status"]}, "after": {"argv": ["go", "test"]},
 "why": "perf row: the Then call of an order rule", "hard_control": "CI"}]}`

// orderSpec gives the rig a project whose repository spec is orderSpecDoc,
// trusted in the rig's store as `rashomon spec trust` binds it, found through
// CLAUDE_PROJECT_DIR, and returns the project's directory, the cwd of the
// case's payloads (a payload's cwd must lie in the project for its spec to be
// found). The directory is under /tmp, short, so the 540 B class is still
// reachable: the fixed part of the payload grows with the cwd.
func (r *rig) orderSpec(t *testing.T) string {
	t.Helper()
	made, err := os.MkdirTemp("/tmp", "rpo-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(made) })
	dir, err := filepath.EvalSymlinks(made)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(orderSpecDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	_, digest, err := spec.Parse([]byte(orderSpecDoc))
	if err != nil {
		t.Fatal(err)
	}
	b := &spec.Bindings{}
	b.Bind(spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digest, BoundAtMS: 1})
	if err := hook.WriteSpecBindings(r.home, b); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSteerDisclosureMarker(r.home, 2); err != nil {
		t.Fatal(err)
	}
	r.env = append(append([]string(nil), r.env...), "CLAUDE_PROJECT_DIR="+dir, "AI_AGENT=claude-code_2-1-280_harness")
	return dir
}

// premise checks, with one call of its own before anything is measured, that
// the order case's call reaches the rule: the steer step raised the ordering
// finding for it (the note is printed). The call is the session's first.
func (r rig) premise(t *testing.T, in []byte) {
	t.Helper()
	_, out := maxRSSMiB(t, in, r.env, r.args([]string{"post"})...)
	if !bytes.Contains(out, []byte("spec rule status-first")) {
		t.Fatalf("the order case's premise call printed %q: it did not reach the order rule", out)
	}
}

// recapSeedCalls is the session size recap.stop's row is measured on
// (BenchmarkRecap/100), so the scan case differs from it by the scan alone;
// pendingSeedCalls is recap.pending's (BenchmarkRecapPending/10), so the
// prompt's pgraph case differs from it by the read alone.
const (
	recapSeedCalls   = 100
	pendingSeedCalls = 10
)

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
	if err := os.WriteFile(path, scanTranscript(size, payload.Session, time.Now().UTC()), 0o600); err != nil {
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
// Every record carries sessionID (the scan reads a record of another session
// as unreadable) and the timestamp at: the note is put a minute before each
// run, so the last record is well past the note's time plus the scan's slack,
// and the scan may judge the note absent.
func scanTranscript(size int, sessionID string, at time.Time) []byte {
	const step = 1 << 20 // linescan's bound in steer.DefaultScanLimits
	stamp := at.Format("2006-01-02T15:04:05.000Z")
	fill := func(n int, kind string) []byte {
		head := `{"type":"user","sessionId":"` + sessionID + `","timestamp":"` + stamp + `","message":{"role":"user","content":"` + kind
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

// pgraphCase is the Stop's platform read case (PW1b-ON-230 step 2),
// pgraphPromptCase the prompt's symbol read case (step 3), and pgraphModule
// the module their logs describe.
const (
	pgraphCase       = "recap.stop.pgraph3MiB"
	pgraphPromptCase = "recap.pending.pgraph3MiB"
	pgraphModule     = "example.com/perf/m"
)

// pgraphStop returns the payload builder of the pgraph case and its control.
// It adds to the started session one Edit of a Go file, recorded through the
// installed hook and post entries, in a module whose .pgraph holds a log of
// exactly size bytes, its digests and a walk file vouching for both, as a
// sync writes them (pgraphFiles): the Stop then takes that module as the
// project, decodes the whole log and hashes the edited package's files. A
// premise run checks the Stop names the package, so a case whose read stood
// down fails rather than reads light; the measured Stops are that Stop
// again. control moves .pgraph away: the same Stop with nothing to read, the
// figure the row's delta is from.
func (r rig) pgraphStop(t *testing.T, size int) (mk func(i int) []byte, control func()) {
	t.Helper()
	src := map[string]string{
		"go.mod":                   "module " + pgraphModule + "\n\ngo 1.25.0\n",
		".github/workflows/ci.yml": "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n",
		"internal/p000/f.go":       "package p000\n\nfunc Func0000() {}\n",
		"internal/p000/f_test.go":  "package p000\n",
	}
	mod, state := pgraphTree(t, size, src, nil)
	transcript := filepath.Join(t.TempDir(), "transcript.jsonl")
	file := filepath.Join(mod, "internal", "p000", "f.go")
	edit := map[string]any{"file_path": file, "old_string": "package", "new_string": "package", "replace_all": false}
	for _, step := range []struct{ entry, event string }{{"hook", "PreToolUse"}, {"post", "PostToolUse"}} {
		p := map[string]any{"hook_event_name": step.event, "session_id": payload.Session, "transcript_path": transcript,
			"cwd": mod, "permission_mode": "default", "tool_name": "Edit", "tool_input": edit, "tool_use_id": "toolu_pgraph"}
		if step.entry == "post" {
			p["tool_response"] = map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
		}
		in, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(rashomonBin, r.args([]string{step.entry})...)
		cmd.Stdin, cmd.Env = bytes.NewReader(in), r.env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("recording the edit through %s: %v\n%s", step.entry, err, out)
		}
	}
	stop, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": payload.Session, "transcript_path": transcript,
		"cwd": mod, "stop_hook_active": false, "last_assistant_message": payload.FinalMessage})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rashomonBin, r.args([]string{"recap"})...)
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(stop), r.env, &stderr
	out, err := cmd.Output()
	if err != nil || !bytes.Contains(out, []byte("go test ./internal/p000/...")) {
		rep := exec.Command(rashomonBin, "report", "--session", payload.Session)
		rep.Env = r.env
		report, _ := rep.CombinedOutput()
		t.Fatalf("premise: the Stop does not name the edited package, so it read no log (%v):\n%s\nstderr: %s\nreport --session:\n%s", err, out, stderr.Bytes(), report)
	}
	return func(int) []byte { return stop }, func() {
		if err := os.Rename(state, state+".off"); err != nil {
			t.Fatal(err)
		}
	}
}

// pgraphTree writes src under a new directory, with a .git and a .pgraph
// holding a log of exactly size bytes, extra's records first, its digests
// and a walk file vouching for both, as a sync writes them (pgraphFiles).
// It returns the module's directory and its .pgraph.
func pgraphTree(t *testing.T, size int, src map[string]string, extra [][3]string) (mod, state string) {
	t.Helper()
	mod, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	logb, digests := pgraphFiles(size, src, extra)
	state = filepath.Join(mod, ".pgraph")
	files := map[string][]byte{
		".git/HEAD":              []byte("ref: refs/heads/main\n"),
		".pgraph/pgraph.log":     logb,
		".pgraph/pgraph.digests": digests,
		".pgraph/pgraph.walk": []byte("pgraph-walk v0\nwalk\t" + sha256Hex(append(append([]byte(nil), logb...), digests...)) +
			"\nlog\t" + sha256Hex(logb) + "\ndigests\t" + sha256Hex(digests) + "\n"),
		".pgraph/.gitignore": []byte("*\n"),
	}
	for name, body := range src {
		files[name] = []byte(body)
	}
	for name, body := range files {
		p := filepath.Join(mod, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return mod, state
}

// pgraphPrompt returns the payload builder of the prompt's pgraph case and
// its control. The prompt names CountRows, which the log has p000 declare
// and p001 and p002 call, in a module whose .pgraph holds a log of exactly
// size bytes in the Stop case's shape; the three packages' files are on
// disk as the digests list them, so the read proves the symbol. The payload
// is recap.pending's with this module as its cwd. A premise run checks the
// steer log holds the symbols line, which only a proven symbol gives, so a
// case whose read stood down fails rather than reads light. control moves
// .pgraph away: the same prompt with nothing to read.
func (r rig) pgraphPrompt(t *testing.T, size int) (mk func(i int) []byte, control func()) {
	t.Helper()
	src := map[string]string{
		"go.mod":                  "module " + pgraphModule + "\n\ngo 1.25.0\n",
		"internal/p000/f.go":      "package p000\n\nfunc CountRows() int { return 1 }\n",
		"internal/p000/f_test.go": "package p000\n",
	}
	decl := "func:" + pgraphModule + "/internal/p000.CountRows"
	extra := [][3]string{{"pkg:" + pgraphModule + "/internal/p000", "declares", decl}}
	for i, caller := range []string{"SumRows", "Twice"} {
		p := fmt.Sprintf("p%03d", i+1)
		src["internal/"+p+"/f.go"] = "package " + p + "\n\nimport \"" + pgraphModule + "/internal/p000\"\n\nfunc " + caller + "() int { return p000.CountRows() }\n"
		for _, f := range []string{"f_test.go", "g.go", "h.go"} {
			src["internal/"+p+"/"+f] = "package " + p + "\n"
		}
		id := "func:" + pgraphModule + "/internal/" + p + "." + caller
		extra = append(extra, [3]string{"pkg:" + pgraphModule + "/internal/" + p, "declares", id}, [3]string{id, "calls", decl})
	}
	mod, state := pgraphTree(t, size, src, extra)
	var fields map[string]any
	if err := json.Unmarshal(payload.Prompt("make CountRows return 2"), &fields); err != nil {
		t.Fatal(err)
	}
	fields["cwd"] = mod
	prompt, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rashomonBin, r.args([]string{"recap", "pending"})...)
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(prompt), r.env, &stderr
	out, err := cmd.Output()
	st, serr := store.Open(r.home)
	if serr != nil {
		t.Fatal(serr)
	}
	steerLog, _ := os.ReadFile(filepath.Join(st.RunDir(payload.Session), store.FileSteer))
	if err != nil || !bytes.Contains(steerLog, []byte("symbols named: ")) {
		t.Fatalf("premise: the prompt's note holds no symbols line, so the read proved nothing (%v):\n%s\nstderr: %s", err, out, stderr.Bytes())
	}
	return func(int) []byte { return prompt }, func() {
		if err := os.Rename(state, state+".off"); err != nil {
			t.Fatal(err)
		}
	}
}

// pgraphFiles is a log of exactly size bytes and its digests, as a sync of a
// module of 120 packages under pgraphModule writes them, extra's records
// (subject, predicate, object) first: each other record is one
// package declaring a function or one function calling another, the shape
// and line length of this repository's own log (3.0 MB, 54 KB of digests),
// every record an assertion, so the whole log is live. src's Go files are
// listed under their own sha256, so the edited package reads fresh; the
// other files are listed and not there, as nothing reads them.
func pgraphFiles(size int, src map[string]string, extra [][3]string) (logb, digests []byte) {
	const pkgs = 120
	pkg := func(i int) string { return fmt.Sprintf("%s/internal/p%03d", pgraphModule, i) }
	fn := func(i, k int) string { return fmt.Sprintf("func:%s.Func%04d", pkg(i), k) }
	rec := func(subject, predicate, object string) string {
		return "+\t" + subject + "\t" + predicate + "\t" + object + "\tPROVEN\tdarwin/arm64/cgo=0/tags=\t" +
			strings.Repeat("c0ffee12", 5) + "\tgotypes@v2\n"
	}
	var b strings.Builder
	b.WriteString("pgraph-log v0\n")
	for _, e := range extra {
		b.WriteString(rec(e[0], e[1], e[2]))
	}
	for k := 0; ; k++ {
		i := k % pkgs
		next := rec("pkg:"+pkg(i), "declares", fn(i, k)) + rec(fn(i, k), "calls", fn((i*7+3)%pkgs, k/2))
		if b.Len()+len(next)+512 > size {
			break
		}
		b.WriteString(next)
	}
	pad := rec("pkg:"+pkg(0), "declares", fn(0, 9999))
	b.WriteString(rec("pkg:"+pkg(0), "declares", fn(0, 9999)+strings.Repeat("x", size-b.Len()-len(pad))))
	var d strings.Builder
	d.WriteString("pgraph-digests v0\n")
	for i := 0; i < pkgs; i++ {
		names := []string{"f.go", "f_test.go", "g.go", "h.go"}
		if i == 0 {
			names = names[:2] // the edited package: src's two files
		}
		for _, f := range names {
			rel := fmt.Sprintf("internal/p%03d/%s", i, f)
			body, ok := src[rel]
			if !ok {
				body = rel
			}
			d.WriteString(pkg(i) + "\t" + rel + "\t" + sha256Hex([]byte(body)) + "\n")
		}
	}
	return []byte(b.String()), []byte(d.String())
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
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

// initRuns is how many fresh processes TestRES2_InitBudget traces; the gate
// reads the fewest bytes among them, as TestColdAllocBudget reads the least
// of its runs. Init's figure is not one value per binary: fifteen runs of
// one binary (d4f5d05e) read 54088 to 54728 bytes, the whole spread in
// internal/godebug, crypto/internal/fips140 and net/netip in 160-byte steps
// (the seeded trie nodes coldLevelGapBytes's note counts for sync.Map), and
// on the macOS runner one product source read 65104 (92f9836b), then 65744
// against 65536 and 64944 on its rerun (run 42521625623, one commit, one
// image, one toolchain). A regression in init is the same bytes in every
// process, so it lifts the fewest as much as any run; the trie's draw lifts
// only some.
const initRuns = 5

// TestRES2_InitBudget: process init allocates at most 64 KiB and runs no GC
// cycle, read from the runtime's own trace of the release binary (RES-2):
// the bytes at the fewest of initRuns processes, and GC in every one.
func TestRES2_InitBudget(t *testing.T) {
	env := append(newRig(t).env, "GODEBUG=inittrace=1,gctrace=1")
	all := make([]int64, initRuns)
	var low InitTrace
	for i := range all {
		cmd := exec.Command(rashomonBin, "version")
		cmd.Env = env
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
		if tr.GCLines > 0 {
			t.Errorf("run %d: %d GC cycles during init", i+1, tr.GCLines)
		}
		all[i] = tr.Bytes
		if i == 0 || tr.Bytes < low.Bytes {
			low = tr
		}
	}
	if low.Bytes > InitBudgetBytes {
		t.Errorf("init allocated %d bytes over %d packages at the fewest of %d runs (runs %v), budget %d", low.Bytes, low.Packages, initRuns, all, InitBudgetBytes)
	}
	ran[InitGate] = true
	t.Logf("init: %d packages, %d bytes at the fewest of %d runs (runs %v), %d allocs, %d gc lines", low.Packages, low.Bytes, initRuns, all, low.Allocs, low.GCLines)
}
