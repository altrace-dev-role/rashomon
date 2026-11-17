package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// The Stop's coverage notice: what the claim checks do while the session's
// record is not complete, said once per session, and the two run-directory
// markers that remember it was said, by the names the inventories list.
const (
	rv5Off         = "claim checks are off until rashomon's record of this session is complete"
	rv5Sticky      = "claim checks are off for the rest of this session"
	rv5On          = "claim checks are on again from this turn"
	rv5Any         = "claim checks are"
	rv5Old         = "coverage unverified ("
	rv5Marker      = "notice-coverage"
	rv5MarkerClear = "notice-coverage-clear"
)

// rv5Env is a generated session (ten calls whose final message claims a pass
// over failed calls) whose post-phase coverage records each test sets.
type rv5Env struct {
	t         *testing.T
	root      string
	installID string
	st        *store.Store
}

func newRV5Env(t *testing.T) *rv5Env {
	t.Helper()
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	installID := benchInstall(t, root, cfg)
	if _, err := payload.WriteSession(root, 10, payload.Prompts); err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	return &rv5Env{t: t, root: root, installID: installID, st: st}
}

// cov is one post-phase coverage record of the session, with reason "" for
// none.
func (e *rv5Env) cov(state, reason, entry, probe string) store.Coverage {
	c := store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: time.Now().UnixMilli(), SessionID: payload.Session, InstallID: e.st.InstallID(),
		Phase: store.PhasePost, State: state, HookEntry: entry, Probe: probe, CWD: payload.CWD}
	if reason != "" {
		c.Reason = &reason
	}
	return c
}

// setCoverage replaces the session's coverage records with recs; none leaves
// the run with no coverage record at all.
func (e *rv5Env) setCoverage(recs ...store.Coverage) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.st.RunDir(payload.Session), store.FileCoverage), nil, 0o600); err != nil {
		e.t.Fatal(err)
	}
	e.add(recs...)
}

func (e *rv5Env) add(recs ...store.Coverage) {
	e.t.Helper()
	for _, c := range recs {
		if err := e.st.AppendCoverage(c); err != nil {
			e.t.Fatal(err)
		}
	}
}

// stop runs the session's Stop through the recap entry as its turn's first
// Stop (recap.json removed, as benchRecap does), so nothing but the notice
// markers can keep a line from repeating, and returns the systemMessage, ""
// when it printed none.
func (e *rv5Env) stop() string {
	e.t.Helper()
	if err := os.Remove(filepath.Join(e.root, "recap.json")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	args, in := benchRecapInput(e.t, e.installID, false)
	var stdout, stderr bytes.Buffer
	if code := cmdRecap(args, bytes.NewReader(in), &stdout, &stderr); code != exitOK {
		e.t.Fatalf("recap: exit %d, stderr %q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		return ""
	}
	return systemMessage(e.t, stdout.String())
}

// gate is the claim gate's reading of the record now, through the
// catch-up's view, which writes nothing: the fixture's control.
func (e *rv5Env) gate() (verify.Recording, []string) {
	e.t.Helper()
	run, err := digest.ReadRun(e.st, payload.Session)
	if err != nil {
		e.t.Fatal(err)
	}
	now := time.Now()
	d, err := digest.BuildFrom(run, e.st, payload.Session, "", payload.FinalMessage, now)
	if err != nil {
		e.t.Fatal(err)
	}
	p := recapPayload{SessionID: payload.Session, LastAssistantMessage: payload.FinalMessage,
		Harness: harnessSignal{HookEventName: "UserPromptSubmit"}, OK: true, CWD: payload.CWD}
	s, err := stopView(e.st, run, d, p, true, now)
	if err != nil {
		e.t.Fatal(err)
	}
	return s.view.Snap.Recording, s.view.Snap.Notes
}

func (e *rv5Env) wantOff(note string) {
	e.t.Helper()
	if rec, notes := e.gate(); rec == verify.Complete || !slices.Contains(notes, note) {
		e.t.Fatalf("fixture: recording %v, notes %q; want claim checks off with %s", rec, notes, note)
	}
}

func (e *rv5Env) wantOn() {
	e.t.Helper()
	if rec, notes := e.gate(); rec != verify.Complete {
		e.t.Fatalf("fixture: recording %v, notes %q; want complete", rec, notes)
	}
}

func (e *rv5Env) marked(name string) bool {
	_, err := os.Lstat(filepath.Join(e.st.RunDir(payload.Session), name))
	return err == nil
}

// TestRV1_5a_CoverageNoticeOncePerSession: a session whose record leaves the
// claim checks off is told so once, at the first Stop that finds it, and the
// session's later Stops say nothing of it while the cause stays, with the
// coverage records naming 0, 1 and 2 reasons at the three Stops. No Stop
// says the old "coverage unverified (...)" sentence. Control: at each Stop
// the claim gate reads the record incomplete with coverage_unverified.
// Break: print the notice on every Stop whatever the marker says, or keep
// the old sentence.
func TestRV1_5a_CoverageNoticeOncePerSession(t *testing.T) {
	e := newRV5Env(t)
	e.setCoverage(e.cov(store.StateUnverified, "", store.EntryPresentSettings, store.ProbeFresh))
	said := 0
	for i, reason := range []string{"", store.ReasonHookEntryAbsent, store.ReasonProbeAbsent} {
		if reason != "" {
			e.add(e.cov(store.StateUnverified, reason, store.EntryPresentSettings, store.ProbeFresh))
		}
		e.wantOff("coverage_unverified")
		msg := e.stop()
		said += strings.Count(msg, rv5Off)
		switch {
		case i == 0 && !strings.Contains(msg, rv5Off):
			t.Errorf("Stop 1 (0 reasons): %q, want the notice %q", msg, rv5Off)
		case i > 0 && strings.Contains(msg, rv5Any):
			t.Errorf("Stop %d (%d reasons): %q, want no notice after the first", i+1, i, msg)
		}
		if strings.Contains(msg, rv5Sticky) || strings.Contains(msg, rv5On) {
			t.Errorf("Stop %d: %q says the sticky or the cleared line on a fault that can clear and has not", i+1, msg)
		}
		if strings.Contains(msg, rv5Old) {
			t.Errorf("Stop %d (%d reasons): %q still says %q", i+1, i, msg, rv5Old)
		}
	}
	if said != 1 {
		t.Errorf("the notice was said %d times over three Stops, want 1", said)
	}
	if !e.marked(rv5Marker) {
		t.Errorf("no %s marker in the run directory after the notice", rv5Marker)
	}
	if e.marked(rv5MarkerClear) {
		t.Errorf("a %s marker with the record never complete", rv5MarkerClear)
	}
}

// TestRV1_5b_AStickyFaultSaysTheRestOfTheSession: a coverage record whose
// probe, or whose hook entry, resolved to unknown keeps the claim checks off
// whatever later records say, so the notice says "for the rest of this
// session" and never "until ... complete"; a verified record after it
// changes nothing: no second notice and no cleared line. Control: the claim
// gate reads coverage_unverified before and after the verified record.
// Break: word every notice as one that can clear, or read a verified record
// after an unknown one as clearing.
func TestRV1_5b_AStickyFaultSaysTheRestOfTheSession(t *testing.T) {
	for _, tc := range []struct{ name, entry, probe string }{
		{"probe unknown", store.EntryPresentSettings, store.ProbeUnknown},
		{"hook entry unknown", store.EntryUnknown, store.ProbeFresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newRV5Env(t)
			e.setCoverage(e.cov(store.StateVerified, "", tc.entry, tc.probe))
			e.wantOff("coverage_unverified")
			if msg := e.stop(); !strings.Contains(msg, rv5Sticky) || strings.Contains(msg, rv5Off) {
				t.Errorf("first Stop: %q, want %q and not %q", msg, rv5Sticky, rv5Off)
			}
			e.add(e.cov(store.StateVerified, "", store.EntryPresentSettings, store.ProbeFresh))
			e.wantOff("coverage_unverified")
			if msg := e.stop(); strings.Contains(msg, rv5Any) {
				t.Errorf("Stop after a verified record: %q, want no notice and no cleared line", msg)
			}
			if !e.marked(rv5Marker) || e.marked(rv5MarkerClear) {
				t.Errorf("markers: %s %v, %s %v; want the notice's alone", rv5Marker, e.marked(rv5Marker),
					rv5MarkerClear, e.marked(rv5MarkerClear))
			}
		})
	}
}

// TestRV1_5c_ClearingIsSaidOnce: a session with no coverage record is told
// the checks are off until its record is complete; the first Stop after a
// verified record completes it says once that they are on again, and the
// Stop after that says neither. A session complete from its first Stop says
// neither and leaves no marker: the cleared line follows a notice, never a
// session that was never off. Control: the claim gate reads coverage_absent,
// then complete. Break: write no clear marker, so the cleared line prints at
// every complete Stop, or print it with no notice before it.
func TestRV1_5c_ClearingIsSaidOnce(t *testing.T) {
	e := newRV5Env(t)
	e.setCoverage()
	e.wantOff("coverage_absent")
	if msg := e.stop(); !strings.Contains(msg, rv5Off) {
		t.Errorf("Stop with no coverage record: %q, want %q", msg, rv5Off)
	}
	e.add(e.cov(store.StateVerified, "", store.EntryPresentSettings, store.ProbeFresh))
	e.wantOn()
	if msg := e.stop(); strings.Count(msg, rv5On) != 1 || strings.Contains(msg, "claim checks are off") {
		t.Errorf("first complete Stop: %q, want %q once and no notice", msg, rv5On)
	}
	if !e.marked(rv5MarkerClear) {
		t.Errorf("no %s marker after the cleared line", rv5MarkerClear)
	}
	if msg := e.stop(); strings.Contains(msg, rv5Any) {
		t.Errorf("second complete Stop: %q, want neither line", msg)
	}

	never := newRV5Env(t)
	never.wantOn()
	if msg := never.stop(); strings.Contains(msg, rv5Any) {
		t.Errorf("a session complete from its first Stop: %q, want neither line", msg)
	}
	if never.marked(rv5Marker) || never.marked(rv5MarkerClear) {
		t.Errorf("a session never off holds a notice marker")
	}
}

// TestRV1_5d_WatchRefusalCarriesTheClause: watch run inside an agent session
// is refused, and the refusal the agent reads as its tool result says what
// that leaves of the session: the claim checks are off until rashomon's
// record of it is complete. consent's refusal, through the same function,
// does not say it: consent changes no recording. Control: both refuse.
// Break: drop the clause, or add it to every refusal.
func TestRV1_5d_WatchRefusalCarriesTheClause(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	for _, tc := range []struct {
		verb string
		want bool
	}{{"watch", true}, {"consent", false}} {
		_, err := askConsent(tc.verb, []byte("screen\n"), false, false, workTreeQuestion{}, io.Discard)
		if err == nil {
			t.Fatalf("%s inside an agent session: no refusal", tc.verb)
		}
		if got := strings.Contains(strings.ToLower(err.Error()), rv5Off); got != tc.want {
			t.Errorf("%s refusal %q: holds the clause %v, want %v", tc.verb, err, got, tc.want)
		}
		// The whole ruled text, typed here and built from no source
		// constant, so a change of any word in the refusal is seen.
		if tc.verb == "watch" && err.Error() != rv5WatchRefusal {
			t.Errorf("watch refusal:\n got %q\nwant %q", err.Error(), rv5WatchRefusal)
		}
	}
}

// rv5WatchRefusal is watch's refusal inside an agent session, as ruled.
const rv5WatchRefusal = "the screen above is yours to agree to, so only you can run it, in a terminal outside Claude Code: " +
	"`rashomon watch`. Nothing was written. If rashomon is not recording this session, claim checks are off until " +
	"rashomon's record of this session is complete. (CLAUDECODE is set here: Claude Code sets it for every command " +
	"it runs, a `!` line included.)"

// replay runs the session's Stop again for the prompt its last Stop claimed
// (recap.json kept), so ClaimTurn refuses the line, and returns the
// systemMessage, "" when it printed none.
func (e *rv5Env) replay() string {
	e.t.Helper()
	args, in := benchRecapInput(e.t, e.installID, false)
	var stdout, stderr bytes.Buffer
	if code := cmdRecap(args, bytes.NewReader(in), &stdout, &stderr); code != exitOK {
		e.t.Fatalf("recap: exit %d, stderr %q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		return ""
	}
	return systemMessage(e.t, stdout.String())
}

// TestRV1_5e_ANoticeIsTakenOnlyWithItsLine: the notice's marker is taken
// only when the turn's line is claimed, so a Stop whose prompt already
// produced a line takes nothing, and the next turn's Stop says the notice.
// Control: the first Stop, record complete, prints a line without the
// notice, and the replayed Stop prints nothing. Break: take the marker
// whether or not the line was claimed.
func TestRV1_5e_ANoticeIsTakenOnlyWithItsLine(t *testing.T) {
	e := newRV5Env(t)
	e.wantOn()
	if msg := e.stop(); msg == "" || strings.Contains(msg, rv5Any) {
		t.Fatalf("fixture: first Stop, record complete: %q, want a line without the notice", msg)
	}
	e.setCoverage()
	e.wantOff("coverage_absent")
	if msg := e.replay(); msg != "" {
		t.Fatalf("fixture: the same prompt's second Stop printed %q, want nothing", msg)
	}
	if e.marked(rv5Marker) {
		t.Errorf("a %s marker after a Stop that printed nothing", rv5Marker)
	}
	if msg := e.stop(); !strings.Contains(msg, rv5Off) {
		t.Errorf("the next turn's Stop: %q, want the notice %q", msg, rv5Off)
	}
}
