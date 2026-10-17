//go:build unix

package acceptance

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Hook entries are supervised on unix only (internal/supervise's Supported);
// elsewhere an entry runs in-process and none of these hold.

// Mirrors internal/fault, internal/supervise and internal/hookio.
const (
	pointSuperviseChild = "supervise.child"
	pointSuperviseSpawn = "supervise.spawn"
	pointPostAfterFlush = "post.after_flush"
	supervisorLogEnv    = "RASHOMON_SUPERVISOR_LOG"
	internalErrorLine   = "rashomon: internal error in a hook entry; nothing was emitted"
	emittedOutput       = `{"systemMessage":"rashomon fault: emitted"}`
	supervisedToken     = "--supervised-child"
	sessionEndEnv       = "CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS"
	maxPayloadBytes     = 8 << 20
	parentBudgetMiB     = 4.5
)

// supervisorLog is the fault build's record of each supervised invocation:
// a "child pid=N" line from every child at its start, and from every
// supervisor at its end a line naming the child and both processes' peaks.
type supervisorLog struct{ path string }

func newSupervisorLog(t *testing.T) supervisorLog {
	return supervisorLog{path: filepath.Join(t.TempDir(), "supervisor.log")}
}

func (l supervisorLog) env() string { return supervisorLogEnv + "=" + l.path }

// lines reads the log, one map of key=value fields per line, with the
// line's first word under "who".
func (l supervisorLog) lines(t *testing.T) []map[string]string {
	t.Helper()
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		m := map[string]string{"who": fields[0]}
		for _, kv := range fields[1:] {
			k, v, _ := strings.Cut(kv, "=")
			m[k] = v
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (l supervisorLog) of(t *testing.T, who string) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, m := range l.lines(t) {
		if m["who"] == who {
			out = append(out, m)
		}
	}
	return out
}

// childPIDs are the pids every child wrote at its start.
func (l supervisorLog) childPIDs(t *testing.T) []int {
	t.Helper()
	var pids []int
	for _, m := range l.of(t, "child") {
		pid, err := strconv.Atoi(m["pid"])
		if err != nil {
			t.Fatalf("child line with no pid: %v", m)
		}
		pids = append(pids, pid)
	}
	return pids
}

// requireGone fails unless every pid has stopped existing within budget,
// and kills any that has not, so a failing test leaves nothing running.
func requireGone(t *testing.T, pids []int, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for _, pid := range pids {
		for syscall.Kill(pid, 0) == nil {
			if time.Now().After(deadline) {
				syscall.Kill(pid, syscall.SIGKILL) //nolint:errcheck // cleanup of a survivor the test already failed on
				t.Errorf("child %d outlived its hook", pid)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// supervisorLines reads <store>/supervisor.ndjson.
func (e *env) supervisorLines() []record {
	e.t.Helper()
	return readNDJSON(e.t, filepath.Join(e.home, "supervisor.ndjson"))
}

func (e *env) killedCoverage(sessionID, phase string) int {
	e.t.Helper()
	n := 0
	for _, c := range e.coverage(sessionID, phase) {
		if c.str("reason") == "hook_killed" {
			n++
		}
	}
	return n
}

func requireInternalError(t *testing.T, res result) {
	t.Helper()
	if res.exitCode != 0 || res.stdout != "" || res.stderr != internalErrorLine+"\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q; want 0, nothing, and only the fixed line", res.exitCode, res.stdout, res.stderr)
	}
}

// TestSTR4_FatalErrorInChildExitsZeroSilently: a Go runtime fatal error is
// the failure no recover catches, and exits 2 -- the control shows it does,
// run as the child with nothing supervising it. Supervised, the same fault,
// in the child before the entry runs and inside the entry's own guarded
// body, exits 0 with nothing on stdout, only the fixed stderr line (a fatal
// trace can carry the payload), a hook_killed coverage record for the call
// carrying the payload's session id, and a supervisor.ndjson line of codes.
func TestSTR4_FatalErrorInChildExitsZeroSilently(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	t.Run("control: unsupervised, the fatal error exits 2", func(t *testing.T) {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		defer r.Close()
		clock := "rashomon-supervisor 1 " + strconv.FormatInt(time.Now().UnixNano(), 10) + " 4000000000\n"
		if _, err := w.WriteString(clock); err != nil {
			t.Fatal(err)
		}
		cmd := e.command(defaultPayload().build(t), []string{"RASHOMON_FAULT=" + pointSuperviseChild + ":fatal=stack"},
			append([]string{supervisedToken, "hook"}, e.installArgs()...)...)
		cmd.ExtraFiles = []*os.File{r}
		res := e.wait(cmd)
		if res.exitCode != 2 || !strings.Contains(res.stderr, "fatal error") {
			t.Fatalf("the child alone exited %d with stderr %q; want 2 and a runtime fatal error, or the cases below prove nothing",
				res.exitCode, res.stderr)
		}
	})

	for _, point := range []string{pointSuperviseChild, pointHookStart} {
		t.Run(point, func(t *testing.T) {
			before := len(e.coverage(testSession, "call"))
			requireInternalError(t, e.hook(defaultPayload().build(t), "RASHOMON_FAULT="+point+":fatal=stack"))
			cov := e.coverage(testSession, "call")
			if len(cov) != before+1 {
				t.Fatalf("%d call coverage records, want %d", len(cov), before+1)
			}
			got := cov[len(cov)-1]
			assertKeySet(t, got, coverageKeys)
			if got.str("reason") != "hook_killed" || got.str("state") != "unverified" || got.str("session_id") != testSession {
				t.Errorf("coverage %s, want hook_killed, unverified, %s", got.raw, testSession)
			}
			lines := e.supervisorLines()
			if len(lines) == 0 {
				t.Fatal("no supervisor.ndjson line")
			}
			last := lines[len(lines)-1]
			if last.str("entry") != "hook" || last.str("event") != "PreToolUse" || last.str("outcome") != "exit_nonzero" || last.fields["child_exit"] != 2.0 {
				t.Errorf("supervisor line %s, want hook, PreToolUse, exit_nonzero, child_exit 2", last.raw)
			}
		})
	}
}

// TestSTR4b_EnvCannotSkipTheSupervisor: no environment variable makes an
// entry the supervised child -- a repository's settings can set the
// environment of every hook -- and neither does the argv token without the
// parent's pipe at fd 3. Each of these is still supervised, so a fatal error
// in it still exits 0.
func TestSTR4b_EnvCannotSkipTheSupervisor(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	fault := "RASHOMON_FAULT=" + pointSuperviseChild + ":fatal=stack"
	for _, c := range []struct {
		name string
		env  []string
		args []string
	}{
		{"an environment variable", []string{fault, "RASHOMON_SUPERVISED=1", "RASHOMON_SUPERVISED_CHILD=1"}, []string{"hook"}},
		{"the argv token alone", []string{fault}, []string{supervisedToken, "hook"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			requireInternalError(t, e.run(defaultPayload().build(t), c.env, append(c.args, e.installArgs()...)...))
		})
	}
}

// TestSTR4c_SpawnFailureIsSilentAndRecorded: a child that cannot be started
// leaves exit 0, nothing on stdout, a supervisor_spawn_failed record -- and
// no declaration, because the entry's logic never runs in the parent.
func TestSTR4c_SpawnFailureIsSilentAndRecorded(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	requireInternalError(t, e.hook(defaultPayload().build(t), "RASHOMON_FAULT="+pointSuperviseSpawn+":fail=1"))
	if got := e.declarations(testSession); len(got) != 0 {
		t.Errorf("%d declarations: the entry ran in the parent", len(got))
	}
	cov := e.coverage(testSession, "call")
	if len(cov) != 1 || cov[0].str("reason") != "supervisor_spawn_failed" {
		t.Fatalf("call coverage %v, want one supervisor_spawn_failed record", cov)
	}
	assertKeySet(t, cov[0], coverageKeys)
	if lines := e.supervisorLines(); len(lines) != 1 || lines[0].str("outcome") != "spawn_failed" {
		t.Errorf("supervisor lines %v, want one spawn_failed", lines)
	}
}

// TestSTR6_HungChildKilledBeforeEntryTimeout: a child still running at the
// hard stop -- the installed timeout less 1 s, 4 s for hook -- is killed
// with its group and recorded before Claude Code's own timeout, whether it
// hangs on its own or on a payload that never ends; the kill-and-trace path,
// timed from spawn at 64-way, stays inside its 100 ms margin; and a child
// whose parent is gone stands down before any new append.
func TestSTR6_HungChildKilledBeforeEntryTimeout(t *testing.T) {
	t.Run("a hung child", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		log := newSupervisorLog(t)
		cmd := e.command(defaultPayload().build(t), []string{"RASHOMON_FAULT=" + pointHookAfterDeclaration + ":hang=30", log.env()},
			append([]string{"hook"}, e.installArgs()...)...)
		requireKilledInTime(t, e, cmd, log)
	})
	t.Run("a payload that never ends", func(t *testing.T) {
		// The stdin read and the child share one budget: a payload cut off
		// after its ids, from a writer that never closes, still ends at the
		// hard stop, and the record carries the ids that did arrive.
		e := newEnv(t)
		e.watched(testSession)
		log := newSupervisorLog(t)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		defer r.Close()
		body := defaultPayload().build(t)
		cut := strings.Index(body, `"tool_input"`)
		if cut < 0 || !strings.Contains(body[:cut], `"session_id":"`+testSession+`"`) {
			t.Fatalf("the payload does not carry its session id before tool_input: %s", body)
		}
		if _, err := w.WriteString(body[:cut]); err != nil {
			t.Fatal(err)
		}
		cmd := e.command("", []string{log.env()}, append([]string{"hook"}, e.installArgs()...)...)
		cmd.Stdin = r
		requireKilledInTime(t, e, cmd, log)
	})
	t.Run("64-way kill and trace", killAndTraceAt64)
	t.Run("an orphaned child stands down", orphanStandsDown)
}

// requireKilledInTime runs a hook entry whose child cannot finish: it must
// return between the 4 s hard stop and the 5 s entry timeout, with the
// fixed line, a hook_killed record, a SIGKILL supervisor line, and no child
// left running.
func requireKilledInTime(t *testing.T, e *env, cmd *exec.Cmd, log supervisorLog) {
	t.Helper()
	start := time.Now()
	res := e.wait(cmd)
	took := time.Since(start)
	requireInternalError(t, res)
	if took < 4*time.Second || took >= 5*time.Second {
		t.Errorf("the hook returned after %v; the hard stop is 4 s and Claude Code's timeout 5 s", took)
	}
	if n := e.killedCoverage(testSession, "call"); n != 1 {
		t.Errorf("%d hook_killed call coverage records, want 1", n)
	}
	lines := e.supervisorLines()
	if len(lines) != 1 || lines[0].str("outcome") != "killed_at_hard_stop" || lines[0].str("signal") != "SIGKILL" {
		t.Errorf("supervisor lines %v, want one killed_at_hard_stop by SIGKILL", lines)
	}
	pids := log.childPIDs(t)
	if len(pids) != 1 {
		t.Fatalf("child pids %v, want one", pids)
	}
	requireGone(t, pids, time.Second)
}

// killAndTraceAt64: SessionEnd's budget, lowered by Claude Code's own
// variable to 500 ms, makes a 400 ms hard stop. 64 children hang past it at
// once; the time from spawn to each supervisor's exit, less the hard stop,
// is the margin's whole charge -- process start, kill, reap, the record's
// append and flush, exit -- and its p99 is at most 100 ms.
func killAndTraceAt64(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	log := newSupervisorLog(t)
	const ways = 64
	const hardStop = 400 * time.Millisecond
	late := make([]time.Duration, ways)
	var wg sync.WaitGroup
	for i := range ways {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := e.command(e.sessionPayload("end", "sess-k"+strconv.Itoa(i)),
				[]string{"RASHOMON_FAULT=" + pointSuperviseChild + ":hang=30", sessionEndEnv + "=500", log.env()},
				append([]string{"probe", "end"}, e.installArgs()...)...)
			start := time.Now()
			if err := cmd.Run(); err != nil {
				t.Errorf("way %d: %v", i, err)
				return
			}
			late[i] = time.Since(start) - hardStop
		}()
	}
	wg.Wait()
	sort.Slice(late, func(i, j int) bool { return late[i] < late[j] })
	p99 := late[(ways*99+99)/100-1]
	t.Logf("kill and trace at %d-way, from spawn, past the 400 ms hard stop: p50 %v, p99 %v, max %v",
		ways, late[ways/2], p99, late[ways-1])
	if p99 > 100*time.Millisecond {
		t.Errorf("kill-and-trace p99 %v exceeds the 100 ms margin", p99)
	}
	killed := 0
	for i := range ways {
		killed += e.killedCoverage("sess-k"+strconv.Itoa(i), "end")
	}
	if killed != ways {
		t.Errorf("%d of %d killed probes left a hook_killed record", killed, ways)
	}
	requireGone(t, log.childPIDs(t), 2*time.Second)
}

// orphanStandsDown: SIGKILL the supervisor while its child hangs after the
// declaration. The child, its parent gone, appends nothing more -- no
// terminal, no coverage -- and exits.
func orphanStandsDown(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	log := newSupervisorLog(t)
	cmd := e.command(defaultPayload().build(t), []string{"RASHOMON_FAULT=" + pointHookAfterDeclaration + ":hang=1", log.env()},
		append([]string{"hook"}, e.installArgs()...)...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "the declaration to land", func() bool { return len(e.declarations(testSession)) == 1 })
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait() //nolint:errcheck // killed on purpose; its status is the signal
	pids := log.childPIDs(t)
	if len(pids) != 1 {
		t.Fatalf("child pids %v, want one", pids)
	}
	requireGone(t, pids, 3*time.Second)
	if got := e.terminals(testSession); len(got) != 0 {
		t.Errorf("the orphaned child appended %d terminals after its parent had gone", len(got))
	}
	if got := e.coverage(testSession, "call"); len(got) != 0 {
		t.Errorf("%d call coverage records after the parent had gone, want none", len(got))
	}
}

// TestREC55_ForcedTimeoutLeavesATrace: a killed child leaves hook_killed
// coverage, written after a newline when the file's last line is torn, so
// the fragment stays one unreadable line and the record reads whole; the
// session reads unverified with the reason named, and the end that was
// killed does not count as recorded.
func TestREC55_ForcedTimeoutLeavesATrace(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	covPath := filepath.Join(e.home, "runs", testSession, "coverage.ndjson")
	f, err := os.OpenFile(covPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	// What a child killed mid-append leaves: half a line.
	if _, err := f.WriteString(`{"type":"coverage","schema_version":3,"sess`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	requireInternalError(t, e.run(e.sessionPayload("end", testSession),
		[]string{"RASHOMON_FAULT=" + pointSuperviseChild + ":hang=30", sessionEndEnv + "=500"},
		append([]string{"probe", "end"}, e.installArgs()...)...))

	body, err := os.ReadFile(covPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("the supervisor's record joined the torn line: %v\n%s", err, body)
	}
	if last["reason"] != "hook_killed" || last["phase"] != "end" {
		t.Errorf("last coverage line %v, want hook_killed for the end", last)
	}
	rep := e.report(testSession)
	if rep.Coverage.State != "unverified" || !e.hasReason(rep, "hook_killed") {
		t.Errorf("report coverage %s %v, want unverified with hook_killed", rep.Coverage.State, rep.Coverage.Reasons)
	}
	if rep.Coverage.EndRecorded {
		t.Error("a killed end probe reads as the session's end having been recorded")
	}
}

// TestRES8_SupervisorStreamsStdin: the supervisor streams the payload
// through one 64 KiB buffer, so its own peak does not grow with the payload:
// an 8 MiB payload, the intake cap, costs it under 1 MiB more than a small
// one, and its peak stays within 4.5 MiB, the parent's budget, past which
// the same-binary supervisor is to give way to a launcher. The child gets
// that payload whole, and one byte over the cap is still the child's to
// refuse, as payload_too_large. The peaks are each process's own, read by
// the supervisor for itself (RUSAGE_SELF), since wait4 would report the
// larger of the two; the fault build's floor is the release build's.
func TestRES8_SupervisorStreamsStdin(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	log := newSupervisorLog(t)

	small := defaultPayload()
	small.ToolUseID = "toolu_small"
	if res := e.hook(small.build(t), log.env()); res.exitCode != 0 || res.stderr != "" {
		t.Fatalf("small: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	big := defaultPayload()
	big.ToolUseID = "toolu_big"
	big.ToolInput = map[string]any{"command": ""}
	base := len(big.build(t))
	big.ToolInput = map[string]any{"command": strings.Repeat("x", maxPayloadBytes-base)}
	body := big.build(t)
	if len(body) != maxPayloadBytes {
		t.Fatalf("payload of %d bytes, want exactly %d", len(body), maxPayloadBytes)
	}
	if res := e.hook(body, log.env()); res.exitCode != 0 || res.stderr != "" {
		t.Fatalf("8 MiB: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// The declaration keeps the 8 MiB command, a line longer than
	// readNDJSON's scanner takes, so its id is looked for in the raw file.
	records, err := os.ReadFile(filepath.Join(e.home, "runs", testSession, "records.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(records), `"tool_use_id":"toolu_big"`) {
		t.Error("the 8 MiB payload did not reach the child whole")
	}

	if res := e.hook(body+" ", log.env()); res.exitCode != 0 {
		t.Fatalf("8 MiB + 1: exit %d", res.exitCode)
	}
	// Refused without parsing, the payload names no session to the child.
	tooLarge := 0
	for _, c := range e.coverage("unattributed", "call") {
		if c.str("reason") == "payload_too_large" {
			tooLarge++
		}
	}
	if tooLarge != 1 {
		t.Errorf("%d payload_too_large records for one payload a byte over the cap, want 1", tooLarge)
	}

	parents := log.of(t, "parent")
	if len(parents) != 3 {
		t.Fatalf("supervisor log %v, want three parent lines", parents)
	}
	mib := func(m map[string]string) float64 {
		v, err := strconv.ParseFloat(m["parent_maxrss"], 64)
		if err != nil || v <= 0 {
			t.Fatalf("parent_maxrss in %v", m)
		}
		return v / (1 << 20)
	}
	smallMiB, bigMiB := mib(parents[0]), mib(parents[1])
	t.Logf("supervisor peak (fault build): %.2f MiB for a small payload, %.2f MiB for 8 MiB", smallMiB, bigMiB)
	if bigMiB-smallMiB >= 1 {
		t.Errorf("an 8 MiB payload cost the supervisor %.2f MiB more than a small one: it is not streaming", bigMiB-smallMiB)
	}
	if peak := max(smallMiB, bigMiB); peak > parentBudgetMiB {
		t.Errorf("the supervisor's own peak is %.2f MiB, over the parent's %.1f MiB budget: the launcher decision is due", peak, parentBudgetMiB)
	}
}

// TestREC34_TheFullSyncPrecedesStdoutThroughTheSupervisor proves group
// commit's order against a hook entry that prints: the one full sync is
// after the last append and outside every lock, and stdout comes after it.
// post prints here through the fault build's emit, after its flush; the
// supervisor logs "stdout" just before it forwards what the child printed.
func TestREC34_TheFullSyncPrecedesStdoutThroughTheSupervisor(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	syncLog := filepath.Join(t.TempDir(), "sync.log")
	res := e.post(defaultPost().build(t), "RASHOMON_FAULT="+pointPostAfterFlush+":emit", syncLogEnv+"="+syncLog)
	if res.exitCode != 0 || res.stderr != "" || res.stdout != emittedOutput {
		t.Fatalf("exit %d, stdout %q, stderr %q; want the emitted answer forwarded", res.exitCode, res.stdout, res.stderr)
	}
	events := readSyncLog(t, syncLog)
	if len(events) < 2 || events[len(events)-1].kind != "stdout" {
		t.Fatalf("sync log %v: stdout is not its last line, after the appends", events)
	}
	fulls, stdouts := 0, 0
	for i, ev := range events {
		switch ev.kind {
		case "full":
			fulls++
			if ev.locked || i != len(events)-2 {
				t.Errorf("event %d of %d: a full sync that is inside a lock or not the last thing before stdout: %v", i, len(events), events)
			}
		case "stdout":
			stdouts++
		}
	}
	if fulls != flushesPerProcess() || stdouts != 1 {
		t.Errorf("%d full syncs and %d stdout writes, want %d and 1: %v", fulls, stdouts, flushesPerProcess(), events)
	}

	// The control: output the gate refuses is never forwarded.
	if res := e.post(defaultPost().build(t), "RASHOMON_FAULT="+pointPostAfterFlush+":emit=junk"); res.exitCode != 0 || res.stdout != "" {
		t.Errorf("refused output: exit %d, stdout %q", res.exitCode, res.stdout)
	}
}
