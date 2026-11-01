package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// svRun is one run directory status reads: whether it holds a call, how old
// its records file is, and the events whose fired marker it holds.
type svRun struct {
	id    string
	calls bool
	age   time.Duration
	fired []string
}

// svRemedy is a missing-marker line's remedy where the event exists on every
// supported Claude Code: the pause or stand-down, the untrusted folder, and
// the entry's absence.
const svRemedy = "recording was paused or stood down, or this folder is not trusted, or run `rashomon watch` to reinstall its entry"

// svInstalled is a store at a fresh root with the settings entries for every
// event installed, holding runs. The markers are written as files, not by
// MarkFired, so status is read against the file contract alone.
func svInstalled(t *testing.T, runs ...svRun) *store.Store {
	t.Helper()
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	benchInstall(t, root, cfg)
	t.Chdir(t.TempDir())
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, r := range runs {
		if err := os.MkdirAll(st.RunDir(r.id), 0o700); err != nil {
			t.Fatal(err)
		}
		for _, e := range r.fired {
			if err := os.WriteFile(filepath.Join(st.RunDir(r.id), store.FileFiredPrefix+e), []byte("1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if !r.calls {
			continue
		}
		if err := st.AppendDeclaration(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
			SessionID: r.id, ToolUseID: "tu-1", ToolName: "Bash"}); err != nil {
			t.Fatal(err)
		}
		at := now.Add(-r.age)
		if err := os.Chtimes(filepath.Join(st.RunDir(r.id), store.FileRecords), at, at); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// svStatus is status's output for args.
func svStatus(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := cmdStatusVerb(args, &out); err != nil {
		t.Fatalf("status %q: %v", args, err)
	}
	return out.String()
}

// svBlock is the header line starting with head and the n lines after it.
func svBlock(t *testing.T, out, head string, n int) []string {
	t.Helper()
	lines := strings.Split(out, "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, head) })
	if i < 0 || i+n >= len(lines) {
		t.Fatalf("status has no %q block of %d lines:\n%s", head, n, out)
	}
	return lines[i : i+n+1]
}

// TestSV1_StatusNamesAnEventThatNeverFired: an installed event every session
// with tool calls fires, that fired in none of them, is "no marker" with
// the causes that can leave one (never "not firing": no cause is proven). Stop is judged over the sessions before the
// newest, which may be mid-turn; the failure events fire only on a failure,
// so a 0 there is a count, never "no marker". Break: drop the never-fired
// check, and a recorder whose Stop entry is gone reads as healthy.
func TestSV1_StatusNamesAnEventThatNeverFired(t *testing.T) {
	everyone := []string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit"}
	svInstalled(t,
		svRun{"s-old", true, 48 * time.Hour, append(slices.Clone(everyone), "SessionEnd")},
		svRun{"s-new", true, time.Hour, everyone})
	out := svStatus(t)
	if want := "  Stop: no marker: 0 of 1 earlier sessions with a marker; " + svRemedy + "\n"; !strings.Contains(out, want) {
		t.Errorf("status does not name Stop as not firing:\n%s", out)
	}
	if n := strings.Count(out, "no marker"); n != 1 || strings.Contains(out, "not firing") {
		t.Errorf("status says no marker %d times, want once (Stop):\n%s", n, out)
	}

	// The newest session alone: its Stop may not have come yet.
	svInstalled(t, svRun{"s-only", true, time.Hour, everyone})
	out = svStatus(t)
	if !strings.Contains(out, "  Stop: not in the newest session; no earlier session\n") || strings.Contains(out, "no marker") {
		t.Errorf("a live first session reads as Stop not firing:\n%s", out)
	}
}

// TestSV1p_UpgradeIsNotNotFiring: sessions run before the upgrade carry no
// marker, so they are not evidence an entry stopped: three of them and one
// new session fired Stop in none, and status names none of the events and
// says how many sessions predate the markers (it once said "Stop: not
// firing: 0 of 3 earlier sessions" and told the user to reinstall). Once an
// earlier session with markers lacks Stop, that is said, over that session
// alone. Break: count every session with calls, and the first reads as an
// entry gone.
func TestSV1p_UpgradeIsNotNotFiring(t *testing.T) {
	everyone := []string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit"}
	svInstalled(t,
		svRun{"s-1", true, 96 * time.Hour, nil},
		svRun{"s-2", true, 72 * time.Hour, nil},
		svRun{"s-3", true, 48 * time.Hour, nil},
		svRun{"s-new", true, time.Hour, everyone})
	out := svStatus(t)
	if strings.Contains(out, "no marker") || strings.Contains(out, "not firing") || strings.Contains(out, "reinstall") {
		t.Errorf("a store holding only pre-marker sessions reads as an entry gone:\n%s", out)
	}
	if want := "events: newest session s-new; 4 sessions with tool calls in the last 7 days; 3 earlier sessions predate the markers\n"; !strings.Contains(out, want) {
		t.Errorf("status does not say how many sessions predate the markers (want %q):\n%s", want, out)
	}
	if !strings.Contains(out, "  Stop: not in the newest session; no earlier session\n") {
		t.Errorf("Stop is judged over unmarked sessions:\n%s", out)
	}
	var j struct {
		Before int `json:"sessions_before_markers"`
	}
	if err := json.Unmarshal([]byte(svStatus(t, "--json")), &j); err != nil || j.Before != 3 {
		t.Errorf("--json sessions_before_markers = %d (%v), want 3", j.Before, err)
	}

	svInstalled(t,
		svRun{"s-1", true, 96 * time.Hour, nil},
		svRun{"s-2", true, 72 * time.Hour, everyone},
		svRun{"s-new", true, time.Hour, everyone})
	out = svStatus(t)
	if want := "  Stop: no marker: 0 of 1 earlier sessions with a marker; " + svRemedy + "\n"; !strings.Contains(out, want) ||
		!strings.Contains(out, "; 1 earlier session predates the markers\n") {
		t.Errorf("one marked earlier session without Stop:\n%s", out)
	}
}

// TestSV1q_RemedyNamesTheCausesThatApply: a missing marker's remedy names the
// pause or stand-down, the untrusted folder and the reinstall, and names a
// Claude Code version only for an event newer than the oldest supported one,
// which no installed event is today. Break: name the version for every event
// again, and PreToolUse is said to need a Claude Code newer than the floor.
func TestSV1q_RemedyNamesTheCausesThatApply(t *testing.T) {
	for _, e := range install.Events() {
		got := firedRemedy(e)
		if strings.Contains(got, "Claude Code before") {
			t.Errorf("%s: remedy names a version no installed event needs: %q", e, got)
		}
		for _, want := range []string{"paused or stood down", "not trusted", "rashomon watch"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: remedy %q lacks %q", e, got, want)
			}
		}
	}
	eventSince["Stop"] = "2.9.0"
	defer delete(eventSince, "Stop")
	if got := firedRemedy("Stop"); !strings.HasPrefix(got, "Claude Code before 2.9.0 lacks this event, or ") {
		t.Errorf("an event newer than the floor: remedy = %q, want the version cause", got)
	}
	if got := firedRemedy("PreToolUse"); strings.Contains(got, "Claude Code before") {
		t.Errorf("PreToolUse names a version: %q", got)
	}
}

// TestSV1r_StatusJSONSaysWhichBlocksAreUnknown: a block status could not
// read is null in --json, as an absent one is, so "unknown" names it. Break:
// drop the list, and an unreadable consent file reads as none on file.
func TestSV1r_StatusJSONSaysWhichBlocksAreUnknown(t *testing.T) {
	st := svInstalled(t, svRun{"s1", true, time.Hour, nil})
	var j struct {
		Unknown []string `json:"unknown"`
		Consent any      `json:"consent"`
	}
	if err := json.Unmarshal([]byte(svStatus(t, "--json")), &j); err != nil || j.Unknown == nil || len(j.Unknown) != 0 {
		t.Errorf("a readable store: unknown = %v (%v), want []", j.Unknown, err)
	}
	consentFile := filepath.Join(st.Root(), store.FileConsent)
	if err := os.Remove(consentFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(consentFile, 0o700); err != nil {
		t.Fatal(err)
	}
	j.Unknown = nil
	if err := json.Unmarshal([]byte(svStatus(t, "--json")), &j); err != nil || !slices.Equal(j.Unknown, []string{"consent"}) || j.Consent != nil {
		t.Errorf("an unreadable consent file: unknown = %v (%v), consent = %v, want [consent] and null", j.Unknown, err, j.Consent)
	}
}

// TestSV1s_LastStopIsAStopsCheckpoint: a run whose last checkpoint was cut at
// another point is not shown as its last Stop's. Break: read the last
// checkpoint whatever its point, and a commit's record is said to be the
// Stop's.
func TestSV1s_LastStopIsAStopsCheckpoint(t *testing.T) {
	st := svInstalled(t, svRun{"s1", true, time.Hour, nil})
	if err := st.AppendRecord("s1", &verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion,
		SessionID: "s1", Point: verify.PointCommit, PointSeq: 1, Recording: verify.Complete}); err != nil {
		t.Fatal(err)
	}
	if out := svStatus(t); !strings.Contains(out, "last stop: no Stop has checked session s1\n") {
		t.Errorf("a commit's checkpoint is read as the last Stop's:\n%s", out)
	}
}

// TestSV1b_StatusCountsPerEvent: per event, the sessions with tool calls in
// the last 7 days it fired in, beside whether it fired in this session (the
// newest run). A run older than 7 days and a run with no call are not
// counted; Stop and SessionEnd count over the earlier sessions only. Break:
// count every run directory, and a week-old session or a probe-only run
// moves the count.
func TestSV1b_StatusCountsPerEvent(t *testing.T) {
	svInstalled(t,
		svRun{"s-a", true, 72 * time.Hour, []string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "SessionStart", "SessionEnd", "Stop", "UserPromptSubmit"}},
		svRun{"s-b", true, 48 * time.Hour, []string{"PreToolUse", "PostToolUse", "SessionStart", "SessionEnd", "Stop", "UserPromptSubmit"}},
		svRun{"s-c", true, time.Hour, []string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit", "StopFailure"}},
		svRun{"s-week", true, 8 * 24 * time.Hour, store.FiredEvents()},
		svRun{"s-nocalls", false, 0, []string{"SessionStart", "SessionEnd"}})
	got := svBlock(t, svStatus(t), "events: ", 8)
	want := []string{
		"events: newest session s-c; 3 sessions with tool calls in the last 7 days",
		"  PreToolUse: fired in the newest session; 3 of 3 sessions",
		"  PostToolUse: fired in the newest session; 3 of 3 sessions",
		"  PostToolUseFailure: not in the newest session; 1 of 3 sessions (fires only when a tool call fails)",
		"  SessionStart: fired in the newest session; 3 of 3 sessions",
		"  SessionEnd: not in the newest session; 2 of 2 earlier sessions",
		"  Stop: not in the newest session; 2 of 2 earlier sessions",
		"  StopFailure: fired in the newest session; 1 of 3 sessions (fires only when a turn ends in an API error)",
		"  UserPromptSubmit: fired in the newest session; 3 of 3 sessions",
	}
	if !slices.Equal(got, want) {
		t.Errorf("events block:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestSV1h_StoreFiredEventsAreTheInstalledOnes: the store's marker names
// are the installer's events, in its order, so status reads a marker for
// every event watch installs and for no other.
func TestSV1h_StoreFiredEventsAreTheInstalledOnes(t *testing.T) {
	if got, want := store.FiredEvents(), install.Events(); !slices.Equal(got, want) {
		t.Errorf("store.FiredEvents() = %q, install.Events() = %q", got, want)
	}
}

// TestSV1i_EachEntryMarksItsEvent: every installed entry marks the event it
// ran for in its session's run, the one command of post and of recap
// telling its two events apart by the payload; a paused machine marks
// nothing. Break: mark in hook alone, and status reads every other event as
// not firing.
func TestSV1i_EachEntryMarksItsEvent(t *testing.T) {
	root, installID := benchStarted(t)
	marker := []string{install.Marker, installID}
	bash := json.RawMessage(`{"command":"go test ./..."}`)
	call := func(event, session string, response json.RawMessage) benchToolPayload {
		head := newBenchHead(event)
		head.SessionID = session
		return benchToolPayload{benchHead: head, PromptID: "prompt-1", ToolName: "Bash", ToolInput: bash,
			ToolUseID: benchToolUseID, ToolResponse: response}
	}
	failure := func(session string) []byte {
		return benchJSON(t, struct {
			benchToolPayload
			Error string `json:"error"`
		}{call("PostToolUseFailure", session, nil), "exit status 1"})
	}
	turn := func(event, session string) []byte {
		head := newBenchHead(event)
		head.SessionID = session
		var prompt, reason string
		switch event {
		case "UserPromptSubmit":
			prompt = "go on"
		case "SessionEnd":
			reason = "exit"
		}
		return benchJSON(t, struct {
			benchHead
			Prompt string `json:"prompt,omitempty"`
			Reason string `json:"reason,omitempty"`
		}{head, prompt, reason})
	}
	drive := func(session string) {
		var out, errOut bytes.Buffer
		response := json.RawMessage(`{"stdout":"ok","stderr":"","interrupted":false,"isImage":false}`)
		cmdHook(marker, bytes.NewReader(benchJSON(t, call("PreToolUse", session, nil))), &errOut)
		cmdPost(marker, bytes.NewReader(benchJSON(t, call("PostToolUse", session, response))), &out, &errOut)
		cmdPost(marker, bytes.NewReader(failure(session)), &out, &errOut)
		cmdRecap(marker, bytes.NewReader(turn("Stop", session)), &out, &errOut)
		cmdRecap(marker, bytes.NewReader(turn("StopFailure", session)), &out, &errOut)
		cmdRecap(append([]string{"pending"}, marker...), bytes.NewReader(turn("UserPromptSubmit", session)), &out, &errOut)
		cmdProbe(append([]string{"end"}, marker...), bytes.NewReader(turn("SessionEnd", session)), &out, &errOut)
	}
	drive(benchSessionID)
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if a, err := st.Activity(st.DirName(benchSessionID)); err != nil || !slices.Equal(a.Fired, store.FiredEvents()) {
		t.Errorf("after every entry ran, fired = %q (%v), want every event", a.Fired, err)
	}

	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatal(err)
	}
	drive("s-paused")
	if a, err := st.Activity(st.DirName("s-paused")); err != nil || len(a.Fired) != 0 {
		t.Errorf("while paused, fired = %q (%v), want none", a.Fired, err)
	}
}

// TestSV1l_StatusSaysHarnessStandDowns: a key of harness_counts.json whose
// sessions stood down is a mismatch status names with its count; a key only
// seen is not, and none says none. Break: drop the count, and a recorder
// standing down for a foreign harness every session reads as not firing
// with no reason given.
func TestSV1l_StatusSaysHarnessStandDowns(t *testing.T) {
	st := svInstalled(t)
	if out := svStatus(t); !strings.Contains(out, "harness mismatches: none\n") {
		t.Errorf("no counter file and status does not say none:\n%s", out)
	}
	now := time.Now()
	for _, s := range []string{"c1", "c2"} {
		if _, err := store.RecordHarnessMismatch(st.Root(), "claude@cursor", s, []string{"cursor_version"}, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.RecordHarnessSeen(st.Root(), "claude@unknown", "u1", now); err != nil {
		t.Fatal(err)
	}
	got := svBlock(t, svStatus(t), "harness mismatches:", 1)
	if want := []string{"harness mismatches:", "  claude@cursor: stood down in 2 of 2 sessions seen"}; !slices.Equal(got, want) {
		t.Errorf("harness block = %q, want %q", got, want)
	}
}

// TestSV1m_StatusSaysTheLastStopsRecording: the newest session's last Stop
// checkpoint says whether its record was complete, and an incomplete one is
// said with the notes that say why; a session no Stop checked says so.
// Break: print the recording word alone, and "incomplete" never says what
// is missing.
func TestSV1m_StatusSaysTheLastStopsRecording(t *testing.T) {
	st := svInstalled(t, svRun{"s1", true, time.Hour, nil})
	if out := svStatus(t); !strings.Contains(out, "last stop: no Stop has checked session s1\n") {
		t.Errorf("no checkpoint and status does not say so:\n%s", out)
	}
	cp := &verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion, SessionID: "s1",
		Point: "stop", PointSeq: 1, Recording: verify.Incomplete}
	if err := st.AppendRecord("s1", cp); err != nil {
		t.Fatal(err)
	}
	if out := svStatus(t); !strings.Contains(out, "last stop: session s1 recorded incomplete at seq 1: declaration_without_outcome, coverage_absent\n") {
		t.Errorf("an incomplete Stop is not said with its notes:\n%s", out)
	}
	cp = &verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion, SessionID: "s1",
		Point: "stop", PointSeq: 2, Recording: verify.Complete}
	if err := st.AppendRecord("s1", cp); err != nil {
		t.Fatal(err)
	}
	if out := svStatus(t); !strings.Contains(out, "last stop: session s1 recorded complete at seq 2\n") {
		t.Errorf("the latest checkpoint is not the one read:\n%s", out)
	}
}

// TestSV1n_StatusSaysTheConsentAgreed: the last act of consent on file,
// its screen version and steer disclosure against this build's, and what to
// run when it is older. Break: compare one version, and a consent to an
// older steer disclosure reads as current.
func TestSV1n_StatusSaysTheConsentAgreed(t *testing.T) {
	st := svInstalled(t)
	at := int64(1_793_000_000_000)
	agreed := time.UnixMilli(at).UTC().Format(time.RFC3339)
	for _, c := range []struct {
		scope, steerV int
		suffix        string
	}{
		{disclose.ScopeVersion, steer.DisclosureVersion, "\n"},
		{disclose.ScopeVersion, steer.DisclosureVersion - 1, ": run `rashomon consent` to see what changed\n"},
	} {
		if err := consent.Append(st.Root(), consent.Record{V: consent.RecordVersion, ScopeVersion: c.scope,
			RecordScope: consent.RecordScopeV5, SteerDisclosure: c.steerV, Steer: consent.SteerOn,
			Via: consent.ViaConsentYes, At: at}); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("consent: screen version %d, steer disclosure %d, agreed %s; this build shows %d, %d%s",
			c.scope, c.steerV, agreed, disclose.ScopeVersion, steer.DisclosureVersion, c.suffix)
		if out := svStatus(t); !strings.Contains(out, want) {
			t.Errorf("status does not say %q:\n%s", want, out)
		}
	}
}

// TestSV1p_StatusNamesTheSteerHold (STEER-DISCLOSE-YES1, R32): when no
// consent on file reaches this build's steer disclosure, the hook holds every
// note, and status says so and how to lift it, rather than "to see what
// changed" on an install where nothing changed. It reads the hold as the hook
// does, from the highest steer_disclosure on file, so a later record at an
// older disclosure does not bring it back. Break: read the last record's
// disclosure, or say nothing of the hold.
func TestSV1p_StatusNamesTheSteerHold(t *testing.T) {
	st := svInstalled(t)
	at := int64(1_793_000_000_000)
	agreed := time.UnixMilli(at).UTC().Format(time.RFC3339)
	const held = ": steer notes are held until you see the steer disclosure: run `rashomon consent` in your own terminal\n"
	for _, c := range []struct {
		name          string
		scope, steerV int
		suffix        string
	}{
		{"a --yes install before STEER-DISCLOSE-YES1", disclose.ScopeVersion, 0, held},
		{"an older screen as well", disclose.ScopeVersion - 1, 0, held},
		{"the disclosure seen", disclose.ScopeVersion, steer.DisclosureVersion, "\n"},
		{"a later record at an older disclosure", disclose.ScopeVersion, steer.DisclosureVersion - 1, ": run `rashomon consent` to see what changed\n"},
	} {
		if err := consent.Append(st.Root(), consent.Record{V: consent.RecordVersion, ScopeVersion: c.scope,
			RecordScope: consent.RecordScopeV5, SteerDisclosure: c.steerV, Steer: consent.SteerOn,
			Via: consent.ViaWatchYes, At: at}); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("consent: screen version %d, steer disclosure %d, agreed %s; this build shows %d, %d%s",
			c.scope, c.steerV, agreed, disclose.ScopeVersion, steer.DisclosureVersion, c.suffix)
		if out := svStatus(t); !strings.Contains(out, want) {
			t.Errorf("%s: status does not say %q:\n%s", c.name, want, out)
		}
	}
}

// svKeys is the sorted keys of a decoded JSON object, or nil if v is not one.
func svKeys(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// TestSV1f_EveryVerbHasJSONWithClosedKeys: status and lessons, reached by
// dispatch, print one JSON object with exactly the keys named here, each
// row and nested object too, so a consumer can rely on a field being there
// and on none appearing unannounced. Break: drop a field from one --json, or
// leave a verb out of the dispatch table.
func TestSV1f_EveryVerbHasJSONWithClosedKeys(t *testing.T) {
	st := svInstalled(t, svRun{"s1", true, time.Hour, []string{"PreToolUse"}})
	if _, err := store.RecordHarnessMismatch(st.Root(), "claude@cursor", "c1", []string{"cursor_version"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendRecord("s1", &verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion,
		SessionID: "s1", Point: "stop", PointSeq: 1, Recording: verify.Incomplete}); err != nil {
		t.Fatal(err)
	}
	jsonOf := func(args ...string) map[string]any {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := run(args, strings.NewReader(""), &out, &errOut); code != exitOK {
			t.Fatalf("%q: exit %d, stderr %q", args, code, errOut.String())
		}
		var m map[string]any
		if err := json.Unmarshal(out.Bytes(), &m); err != nil {
			t.Fatalf("%q printed %q: %v", args, out.String(), err)
		}
		return m
	}
	status := jsonOf("status", "--json")
	check := func(what string, v any, want ...string) {
		t.Helper()
		if got := svKeys(v); !slices.Equal(got, want) {
			t.Errorf("%s keys = %q, want %q", what, got, want)
		}
	}
	check("status", status, "consent", "events", "harness_mismatches", "last_stop", "session", "sessions_before_markers", "sessions_with_calls", "store", "unknown")
	if rows, _ := status["events"].([]any); len(rows) != len(install.Events()) {
		t.Errorf("status events has %d rows, want %d", len(rows), len(install.Events()))
	} else {
		check("events[0]", rows[0], "event", "installed", "newest_session", "not_firing", "of", "sessions")
	}
	if rows, _ := status["harness_mismatches"].([]any); len(rows) != 1 {
		t.Errorf("status harness_mismatches = %v, want one row", status["harness_mismatches"])
	} else {
		check("harness_mismatches[0]", rows[0], "key", "seen", "stood_down")
	}
	check("last_stop", status["last_stop"], "checked", "notes", "point_seq", "recording", "session")
	check("consent", status["consent"], "agreed_at_unix_ms", "build_scope_version", "build_steer_disclosure", "current", "scope_version", "steer_disclosure")

	bodiesHere(t)
	file := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(file, []byte(sharedLessons), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := learnRun(t, "import", file); code != 0 {
		t.Fatalf("import: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	lessons := jsonOf("lessons", "--json")
	check("lessons", lessons, "lessons", "project", "quarantined", "quarantined_at_unix_ms")
	if rows, _ := lessons["lessons"].([]any); len(rows) != 2 {
		t.Errorf("lessons has %v, want two rows", lessons["lessons"])
	} else {
		check("lessons[0]", rows[0], "confirmed", "fires", "id", "refuted", "since_unix_ms", "text", "tier", "unresolved")
	}
}

// TestSV1g_NothingRecordedIsSaidPlainly: on a machine with no store, each
// new status block and lessons say there is nothing yet, in text and in
// JSON, and asking creates no store. Break: print an empty block, and "no
// events" reads the same as "status could not look".
func TestSV1g_NothingRecordedIsSaidPlainly(t *testing.T) {
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	t.Chdir(t.TempDir())
	out := svStatus(t)
	for _, want := range []string{"events: nothing recorded yet\n", "harness mismatches: none\n",
		"last stop: nothing recorded yet\n", "consent: none on file\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("status with no store does not say %q:\n%s", want, out)
		}
	}
	var buf bytes.Buffer
	if err := cmdStatusVerb([]string{"--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("status --json printed %q: %v", buf.String(), err)
	}
	if m["session"] != nil || m["last_stop"] != nil || m["consent"] != nil || m["sessions_with_calls"] != float64(0) {
		t.Errorf("status --json with no store = %v", m)
	}
	if rows, ok := m["events"].([]any); !ok || len(rows) != 0 {
		t.Errorf("status --json events = %v, want []", m["events"])
	}
	buf.Reset()
	if err := cmdLessons(nil, &buf); err != nil || buf.String() != "no lessons: nothing has been recorded here\n" {
		t.Errorf("lessons with no store = %q (%v)", buf.String(), err)
	}
	buf.Reset()
	if err := cmdLessons([]string{"--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	m = nil
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("lessons --json printed %q: %v", buf.String(), err)
	}
	if rows, ok := m["lessons"].([]any); !ok || len(rows) != 0 || m["project"] != nil || m["quarantined_at_unix_ms"] != nil {
		t.Errorf("lessons --json with no store = %v, want no project, no quarantine and []", m)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Errorf("asking left %d entries under the store root (%v)", len(entries), err)
	}
}
