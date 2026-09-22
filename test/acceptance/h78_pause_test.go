package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestH78_PauseTakesEffectOnTheNextCallInTheSameSession: a hook call before
// pause is recorded, the very next one after pause is not, and a call after
// resume is recorded again -- all inside one session, with no restart.
//
// Break (to see this fail): cache the paused state for the process lifetime,
// e.g. read it once in main() and pass a bool down, instead of checkPaused
// re-reading the marker on every invocation. Each `rashomon hook` here is its
// own process already, so the more faithful break is to move checkPaused's
// read behind a sync.Once or a package-level variable seeded on first use --
// either way, the second hook call would keep recording because "the process"
// (this test's notion of one) never re-checks.
func TestH78_PauseTakesEffectOnTheNextCallInTheSameSession(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	before := defaultPayload()
	before.ToolUseID = "toolu_before"
	e.mustHook(before.build(t))

	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	duringPayload := defaultPayload()
	duringPayload.ToolUseID = "toolu_during"
	if res := e.hook(duringPayload.build(t)); res.exitCode != 0 {
		t.Fatalf("hook while paused: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	after := defaultPayload()
	after.ToolUseID = "toolu_after"
	e.mustHook(after.build(t))

	decls := e.declarations(testSession)
	if len(decls) != 2 {
		t.Fatalf("got %d declarations, want 2 (before pause and after resume); "+
			"the paused call in between must not appear: %+v", len(decls), decls)
	}
	got := map[string]bool{}
	for _, d := range decls {
		got[d.str("tool_use_id")] = true
	}
	if !got["toolu_before"] || !got["toolu_after"] {
		t.Errorf("declarations are %v, want toolu_before and toolu_after and nothing else", got)
	}
	if got["toolu_during"] {
		t.Errorf("the call made while paused was recorded")
	}

	callCoverage := e.coverage(testSession, "call")
	var pausedCount int
	for _, c := range callCoverage {
		if c.str("reason") == "recording_paused" {
			pausedCount++
		}
	}
	if pausedCount != 1 {
		t.Errorf("got %d call-phase coverage records with reason recording_paused, want 1", pausedCount)
	}
}

// TestH79_PauseReachesAnEntryThatNamesNoInstallID: a plugin-owned hook entry
// invokes the binary without --install at all (spec: "A plugin invocation
// names no --install, which already records normally"). Pause has to catch
// that call too, or a plugin-only install could never be paused -- Part 1's
// own justification for the file over a settings edit.
//
// Break (to see this fail): implement pause as a settings edit -- e.g. have
// `rashomon pause` narrow the installed PreToolUse matcher to something that
// never fires, the way an entry someone hand-edited would be -- and this
// call, which carries no --install and so is not looking at any entry at
// all, keeps recording straight through it.
func TestH79_PauseReachesAnEntryThatNamesNoInstallID(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// e.hook() would append --install <id> because a store now exists; a
	// plugin's own entry never does, so this call is built and run directly,
	// exactly as install.go's own comment describes that invocation.
	payload := defaultPayload()
	payload.ToolUseID = "toolu_plugin_origin"
	res := e.run(payload.build(t), nil, "hook")
	if res.exitCode != 0 {
		t.Fatalf("hook with no --install: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	if got := len(e.declarations(testSession)); got != 0 {
		t.Errorf("got %d declarations from a plugin-shaped call while paused, want 0", got)
	}
	found := false
	for _, c := range e.coverage(testSession, "call") {
		if c.str("reason") == "recording_paused" {
			found = true
		}
	}
	if !found {
		t.Errorf("no call-phase coverage record carries recording_paused for the no-install-id invocation")
	}
}

// TestH80_PausedSessionWithAStoreWritesRecordingPaused is H-80 through the
// compiled binary: with a store present, a paused hook call writes a
// coverage record carrying recording_paused rather than nothing.
//
// Break: return early inside checkPaused before the AppendCoverage call (see
// hook.RecordPaused). The gap then renders identically to a crash -- no
// coverage record at all for that invocation -- which is indistinguishable
// from the handler never having run.
func TestH80_PausedSessionWithAStoreWritesRecordingPaused(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.hook(defaultPayload().build(t)); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	cov := e.coverage(testSession, "call")
	if len(cov) != 1 {
		t.Fatalf("got %d call coverage records, want 1", len(cov))
	}
	if got := cov[0].str("reason"); got != "recording_paused" {
		t.Errorf("reason is %q, want recording_paused", got)
	}
	if got := cov[0].str("state"); got != "unverified" {
		t.Errorf("state is %q, want unverified", got)
	}
	assertKeySet(t, cov[0], coverageKeys)
}

// TestH81_PausedMachineWithNoStoreCreatesNothing: on a machine that has never
// recorded, pause, then run a full session's worth of hook invocations
// (PreToolUse, PostToolUse, SessionStart, SessionEnd). None of them may mint
// an install identity -- install.json and install.key, the two files that
// make store.Open's identity real, and the runs/ directory that comes with
// recording anything.
//
// The store root itself (e.home, i.e. RASHOMON_HOME) DOES end up existing
// after this test: `pause` itself creates it to hold its own marker file,
// under the same rule installMetaFile-based checks use everywhere else in
// this program -- a bare directory holding only the pause marker is not "a
// store" by this codebase's own test (storeInstalled, OpenExisting, status's
// "present" line all key on install.json, never on the root's existence).
// The spec's own words confirm this is the intended shape: "status covers
// that case -- it reports paused state without a store", which could not be
// true if the marker itself could never be written anywhere. What THIS test
// asserts is the thing H-81 actually names as the failure: that no hook
// invocation, while paused, ever escalates that bare marker into a real
// store.
//
// Break: move checkPaused's call after openForHook in cmdHook/cmdPost/
// cmdProbe. Every paused tool call then opens the store first and only
// notices it is paused afterward, minting install.json and install.key on
// the very first invocation.
func TestH81_PausedMachineWithNoStoreCreatesNothing(t *testing.T) {
	e := newEnv(t)
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// A full session, with no install id available to name -- there is no
	// store to have printed one -- exactly as an entry installed before
	// --install existed, or a plugin's own entry, would invoke this program.
	if res := e.probe("start", testSession); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.run(defaultPayload().build(t), nil, "hook"); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.run(defaultPost().build(t), nil, "post"); res.exitCode != 0 {
		t.Fatalf("post: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("end", testSession); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	for _, name := range []string{"install.json", "install.key", "runs"} {
		if _, err := os.Stat(filepath.Join(e.home, name)); err == nil {
			t.Errorf("%s exists after a fully paused session; no hook call may create it", name)
		}
	}
	if got := e.declarations(testSession); len(got) != 0 {
		t.Errorf("got %d declarations from a paused session with no store, want 0", len(got))
	}
	if got := e.gaps(); len(got) != 0 {
		t.Errorf("got %d gap records from a paused session with no store, want 0: %+v", len(got), got)
	}
}

// TestPauseResumeWindow_RendersAsANamedUnknownGap is the security-review
// addendum on top of Part 2's own H-78..H-82: a coverage record alone is not
// enough, because between pause and resume THE HOOKS DO NOT RUN AT ALL, so a
// paused stretch with no tool calls in it writes no coverage record either --
// on disk it would be identical to a stretch where the agent simply did
// nothing. `pause` and `resume` each leave a gap record of their own for
// exactly this reason, and the report must render the window as a named,
// bounded unknown rather than a clean zero or a silent absence.
//
// Break: delete the AppendGap call in cmdResume (or cmdPause). The paused
// window then has no trace anywhere a report looks, and a machine that
// recorded nothing because it was told to stop is indistinguishable from one
// that recorded nothing because the agent used no tools.
func TestPauseResumeWindow_RendersAsANamedUnknownGap(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	beforePause := defaultPayload()
	beforePause.ToolUseID = "toolu_before_pause"
	e.mustHook(beforePause.build(t))

	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	afterResume := defaultPayload()
	afterResume.ToolUseID = "toolu_after_resume"
	e.mustHook(afterResume.build(t))

	sessions := e.reportAll()
	var pausedSession *reportSession
	for i := range sessions {
		if sessions[i].SessionID == "recording-paused" {
			pausedSession = &sessions[i]
		}
	}
	if pausedSession == nil {
		t.Fatalf("no session named recording-paused in the report; the paused window left no trace: %+v", sessions)
	}
	// Two records, not one: pause writes a zero-width marker as evidence the
	// window began even if resume never follows, and resume writes the closed
	// span a reader actually wants. Both carry the same reason.
	if len(pausedSession.Gaps) != 2 {
		t.Fatalf("recording-paused session carries %d gaps, want 2 (pause's marker and resume's closed span): %+v",
			len(pausedSession.Gaps), pausedSession.Gaps)
	}
	minFrom, maxTo := 0.0, 0.0
	for i, g := range pausedSession.Gaps {
		if got := g["reason"]; got != "paused" {
			t.Errorf("gap %d reason is %v, want %q", i, got, "paused")
		}
		from, _ := g["from_unix_ms"].(float64)
		to, _ := g["to_unix_ms"].(float64)
		if to < from {
			t.Errorf("gap %d has to (%v) before from (%v)", i, to, from)
		}
		if i == 0 || from < minFrom {
			minFrom = from
		}
		if i == 0 || to > maxTo {
			maxTo = to
		}
	}
	if maxTo < minFrom {
		t.Errorf("the two gap records together do not cover a real span: earliest from=%v, latest to=%v", minFrom, maxTo)
	}
	if pausedSession.Coverage.State != "unverified" {
		t.Errorf("recording-paused session's coverage state is %q, want unverified -- "+
			"a paused window must never render as a clean pass", pausedSession.Coverage.State)
	}
	if !e.hasReason(*pausedSession, "gap") {
		t.Errorf("recording-paused session's coverage reasons %v do not include \"gap\"", pausedSession.Coverage.Reasons)
	}
	if pausedSession.Declarations.Recorded != 0 {
		t.Errorf("recording-paused session recorded %d declarations, want 0 -- it is not a session, it is evidence", pausedSession.Declarations.Recorded)
	}

	// The real session on either side of the pause is untouched by any of
	// this: two declarations, one before and one after.
	real := e.report(testSession)
	if real.Declarations.Recorded != 2 {
		t.Errorf("the real session recorded %d declarations, want 2 (before pause, after resume)", real.Declarations.Recorded)
	}
}

// TestPausedSessionProbeIsNotReportedAsRecorded: a SessionStart or SessionEnd
// probe that found recording paused still writes a start- or end-phase
// coverage record (reason recording_paused), because the gap has to show up in
// that session's own report. That record is evidence the probe was SKIPPED,
// not that it ran, so the report must not count it as the session's start or
// end having been recorded.
//
// Found by review: watch, pause, probe start, resume, probe end rendered
// "start recorded: yes" and "hook entry at start: present" on the same screen
// as probe_absent's "no session start was recorded" -- a report contradicting
// itself, with start_recorded=true in the JSON beside it.
//
// Break (to see this fail): in report.go build()'s coverage loop, let a
// recording_paused record reach the PhaseStart/PhaseEnd cases again, e.g.
// delete the recording_paused check in front of the switch.
func TestPausedSessionProbeIsNotReportedAsRecorded(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// The start probe runs while paused; the end probe after resume.
	const pausedStart = "session-paused-start"
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("start", pausedStart); res.exitCode != 0 {
		t.Fatalf("probe start while paused: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("end", pausedStart); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// The mirror: the start probe runs normally, the end probe while paused.
	const pausedEnd = "session-paused-end"
	if res := e.probe("start", pausedEnd); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("end", pausedEnd); res.exitCode != 0 {
		t.Fatalf("probe end while paused: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// Precondition: the paused probes did leave their coverage records, so
	// what follows is about how the report reads them, not about their absence.
	for _, c := range []struct{ session, phase string }{{pausedStart, "start"}, {pausedEnd, "end"}} {
		found := false
		for _, r := range e.coverage(c.session, c.phase) {
			if r.str("reason") == "recording_paused" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: no %s-phase coverage record carries recording_paused", c.session, c.phase)
		}
	}

	s := e.report(pausedStart)
	if s.Coverage.StartRecorded {
		t.Errorf("%s: start_recorded is true, but the start probe was skipped by pause", pausedStart)
	}
	if s.Coverage.HookEntryAtStart != "unknown" {
		t.Errorf("%s: hook_entry_at_start is %q, want unknown: no start probe resolved it", pausedStart, s.Coverage.HookEntryAtStart)
	}
	if !s.Coverage.EndRecorded {
		t.Errorf("%s: end_recorded is false, but the end probe ran after resume", pausedStart)
	}
	for _, want := range []string{"recording_paused", "probe_absent"} {
		if !e.hasReason(s, want) {
			t.Errorf("%s: reasons %v lack %s", pausedStart, s.Coverage.Reasons, want)
		}
	}

	s = e.report(pausedEnd)
	if s.Coverage.EndRecorded {
		t.Errorf("%s: end_recorded is true, but the end probe was skipped by pause", pausedEnd)
	}
	if s.Coverage.HookEntryAtEnd != "unknown" {
		t.Errorf("%s: hook_entry_at_end is %q, want unknown: no end probe resolved it", pausedEnd, s.Coverage.HookEntryAtEnd)
	}
	if !s.Coverage.StartRecorded {
		t.Errorf("%s: start_recorded is false, but the start probe ran before pause", pausedEnd)
	}
	for _, want := range []string{"recording_paused", "run_not_closed"} {
		if !e.hasReason(s, want) {
			t.Errorf("%s: reasons %v lack %s", pausedEnd, s.Coverage.Reasons, want)
		}
	}

	// The text form a person reads: the line under the reasons must agree
	// with them.
	res := e.run("", nil, "report", "--session", pausedStart)
	if res.exitCode != 0 {
		t.Fatalf("report text: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "start recorded: no") {
		t.Errorf("text report does not say \"start recorded: no\" for a paused start:\n%s", res.stdout)
	}
}
