package nono

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The fixture is a REAL capture: nono v0.78.0, a real session, its real
// audit-events.ndjson, redacted only of the local home path and checked in.
// Hand-built JSON would have encoded what I believed the format to be -- and
// the documentation described event types the program does not emit, so a
// hand-built fixture would have been wrong in exactly the way a test cannot
// notice.
const fixture = "../../test/fixtures/nono/audit-events.ndjson"

// fixtureWindow spans the captured session. The events carry real wall-clock
// instants from the machine that recorded them, so the window is derived from
// the file rather than from time.Now().
func fixtureWindow(t *testing.T) Window {
	t.Helper()
	obs := Read(fixture, Window{Start: time.Unix(0, 0)})
	if !obs.Observed || len(obs.Events) == 0 {
		t.Fatalf("premise: the fixture yields no events (%s)", obs.Reason)
	}
	first := obs.Events[0].At
	last := obs.Events[len(obs.Events)-1].At
	return Window{Start: first.Add(-time.Second), End: last.Add(time.Second)}
}

// TestRead_TheRealCapture is the shape, asserted against the bytes nono wrote.
func TestRead_TheRealCapture(t *testing.T) {
	obs := Read(fixture, fixtureWindow(t))

	if !obs.Observed {
		t.Fatalf("not observed: %s", obs.Reason)
	}
	if obs.Sessions != 1 {
		t.Errorf("sessions = %d, want 1", obs.Sessions)
	}
	if len(obs.Events) != 4 {
		t.Fatalf("events = %d, want 4 (two allowed, two denied): %+v",
			len(obs.Events), obs.Events)
	}

	if got, want := obs.Hosts(), []string{"example.com", "pypi.org"}; !eq(got, want) {
		t.Errorf("allowed hosts = %v, want %v", got, want)
	}
	if got, want := obs.Denied(), []string{"169.254.169.254", "github.com"}; !eq(got, want) {
		t.Errorf("denied hosts = %v, want %v", got, want)
	}
}

// TestRead_DeniedHostsAreNotDestinations. A denied host is one the agent TRIED
// to reach and did not. Folding it in with the allowed ones would report an
// attempt as a destination -- the same distinction the wire reader's Unreached
// exists to make, and the same direction of error.
func TestRead_DeniedHostsAreNotDestinations(t *testing.T) {
	obs := Read(fixture, fixtureWindow(t))
	for _, h := range obs.Hosts() {
		if h == "github.com" || h == "169.254.169.254" {
			t.Errorf("%s was denied and appears as a reached destination", h)
		}
	}
}

// TestRead_TheReverseModeEventIsKept is the one that explains a DISAGREEMENT
// with the wire, and it is why Mode is carried at all.
//
// rashomon exports no HTTP_PROXY -- plain HTTP is outside what its proxy can
// see. nono's reverse-proxy path does see it: the captured session's attempt on
// the cloud metadata endpoint is port 80, mode "reverse". Without this field a
// reader comparing the two sources finds a host nono saw and rashomon did not,
// and reads it as a recording failure rather than as a known boundary.
func TestRead_TheReverseModeEventIsKept(t *testing.T) {
	obs := Read(fixture, fixtureWindow(t))

	var reverse []Event
	for _, e := range obs.Events {
		if e.Mode == "reverse" {
			reverse = append(reverse, e)
		}
	}
	if len(reverse) != 1 {
		t.Fatalf("reverse-mode events = %d, want 1: %+v", len(reverse), obs.Events)
	}
	if reverse[0].Port != 80 {
		t.Errorf("port = %d, want 80: the point of this event is that it is plain HTTP",
			reverse[0].Port)
	}
	if reverse[0].Host != "169.254.169.254" {
		t.Errorf("host = %q, want the metadata endpoint", reverse[0].Host)
	}
}

// TestRead_DenialCategoriesSurvive. nono distinguishes an allowlist miss from a
// deny-list hit by its reason string; both carry denial_category host_denied.
// The category is kept and the reason is NOT -- it is free text embedding the
// host, and this program does not carry free text from another program's log
// into its own output.
func TestRead_DenialCategoriesSurvive(t *testing.T) {
	obs := Read(fixture, fixtureWindow(t))
	for _, e := range obs.Events {
		switch e.Decision {
		case DecisionDeny:
			if e.Category == "" {
				t.Errorf("%s denied with no category: %+v", e.Host, e)
			}
		case DecisionAllow:
			if e.Category != "" {
				t.Errorf("%s allowed and carries a denial category %q", e.Host, e.Category)
			}
		default:
			t.Errorf("unknown decision %q", e.Decision)
		}
	}
}

// TestRead_NoCommandLineReachesAnEvent is the invariant, and it is the reason
// this package can exist at all.
//
// nono's session_started record carries the FULL COMMAND LINE of the sandboxed
// process. rashomon stores no content. A fourth evidence source that quietly
// became the first one to record a command would undo the property the whole
// codebase is arranged around, and would do it while looking like an
// integration.
func TestRead_NoCommandLineReachesAnEvent(t *testing.T) {
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	// A fragment that appears ONLY inside the captured command line.
	const canary = "dev/null"
	if !strings.Contains(string(raw), canary) {
		t.Fatalf("premise: the fixture no longer carries a command line, so this test " +
			"proves nothing -- recapture it from a session that runs one")
	}

	obs := Read(fixture, fixtureWindow(t))
	for _, e := range obs.Events {
		for _, got := range []string{e.Host, e.Decision, e.Mode, e.Category} {
			if strings.Contains(got, canary) || strings.Contains(got, "curl") {
				t.Errorf("a command-line fragment reached an event: %+v", e)
			}
		}
	}

	// And the type has nowhere to put one. Counted rather than named: a new
	// field is a new place for content to land, and this goes red until
	// somebody adds it deliberately and says so here.
	if n := reflect.TypeOf(Event{}).NumField(); n != 6 {
		t.Errorf("Event has %d fields, not 6. Adding one widens what this package can "+
			"carry out of another program's log; do it deliberately.", n)
	}
}

// TestRead_AbsentTrailIsCoverageNotAnError.
func TestRead_AbsentTrailIsCoverageNotAnError(t *testing.T) {
	obs := Read(filepath.Join(t.TempDir(), "missing.ndjson"), Window{Start: time.Now()})
	if obs.Observed {
		t.Error("an absent trail was reported as observed")
	}
	if obs.Reason != NotObservedNoTrail {
		t.Errorf("reason = %q, want %q", obs.Reason, NotObservedNoTrail)
	}
	if obs.Events == nil {
		t.Error("Events is nil; it must always marshal as an array, because null and [] " +
			"are the same absence to a reader and different values to a consumer")
	}
}

// TestRead_NoWindowObservesNothing. With no interval nothing can be attributed
// -- the wire reader learned this by attributing an entire database to an
// evicted run.
func TestRead_NoWindowObservesNothing(t *testing.T) {
	obs := Read(fixture, Window{})
	if obs.Observed {
		t.Error("a trail was read with no window to read it against")
	}
	if obs.Reason != NotObservedNoWindow {
		t.Errorf("reason = %q, want %q", obs.Reason, NotObservedNoWindow)
	}
}

// TestRead_EventsOutsideTheWindowAreCountedNotDropped.
func TestRead_EventsOutsideTheWindowAreCountedNotDropped(t *testing.T) {
	obs := Read(fixture, Window{Start: time.Now().Add(24 * time.Hour)})
	if len(obs.Events) != 0 {
		t.Errorf("events = %d, want 0 for a window after the capture", len(obs.Events))
	}
	if obs.Inherited != 4 {
		t.Errorf("inherited = %d, want 4. Dropping them silently would make a shared "+
			"audit directory look quiet.", obs.Inherited)
	}
}

// TestRead_AMalformedLineDoesNotDiscardTheRest. A trail is append-only and a
// torn final write is the ordinary case, not an exception.
func TestRead_AMalformedLineDoesNotDiscardTheRest(t *testing.T) {
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	torn := filepath.Join(t.TempDir(), "torn.ndjson")
	if err := os.WriteFile(torn, append(raw, []byte(`{"sequence": 9, "eve`)...), 0o600); err != nil {
		t.Fatal(err)
	}

	full := Read(fixture, fixtureWindow(t))
	obs := Read(torn, fixtureWindow(t))
	if len(obs.Events) != len(full.Events) {
		t.Errorf("a torn last line cost %d events", len(full.Events)-len(obs.Events))
	}
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRead_AnEmptyOrWrongShapedFileIsNotAQuietSession.
//
// Observed used to be set on a successful open, so three different broken
// inputs all reported "the sandbox watched and saw nothing" -- silence read as
// zero, which is the failure this package's own doc says it exists to prevent.
func TestRead_AnEmptyOrWrongShapedFileIsNotAQuietSession(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{"empty", ""},
		{"blank lines", "\n   \n\t\n"},
		{"not ndjson", "hello\nworld\n"},
		{"valid json, wrong shape", `{"foo":1}` + "\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "audit-events.ndjson")
			if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
				t.Fatal(err)
			}
			obs := Read(path, Window{Start: time.Now().Add(-time.Hour)})
			if obs.Observed {
				t.Errorf("reported as observed with zero records; a reader cannot tell this "+
					"from a session where the sandbox genuinely saw nothing (reason=%q)",
					obs.Reason)
			}
			if obs.Reason != NotObservedNoRecords {
				t.Errorf("reason = %q, want %q", obs.Reason, NotObservedNoRecords)
			}
		})
	}
}

// TestRead_DroppedRecordsAreCounted. Four paths used to drop a record with no
// trace, so a trail that was three-quarters unreadable rendered identically to
// a quiet one.
func TestRead_DroppedRecordsAreCounted(t *testing.T) {
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mixed.ndjson")
	body := string(raw) +
		"{\"sequence\": 90, \"eve\n" + // torn
		`{"sequence": 91}` + "\n" + // no event
		`{"sequence": 92, "event": {"type": "network", "event": {"target": "", "decision": "allow"}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	obs := Read(path, fixtureWindow(t))
	if obs.Skipped != 2 {
		t.Errorf("skipped = %d, want 2 (a torn line and one with no event)", obs.Skipped)
	}
	if obs.UnparseableTargets != 1 {
		t.Errorf("unparseable_targets = %d, want 1. A DENY on a credential-bearing target "+
			"is the most interesting record a sandbox can write, and it used to vanish here.",
			obs.UnparseableTargets)
	}
}

// TestRead_ACredentialBearingTargetIsRefusedAndCounted.
func TestRead_ACredentialBearingTargetIsRefusedAndCounted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.ndjson")
	body := `{"sequence":0,"event":{"type":"session_started"}}` + "\n" +
		`{"sequence":1,"event":{"type":"network","event":{"timestamp_unix_ms":1,` +
		`"mode":"connect","decision":"deny","denial_category":"host_denied",` +
		`"target":"user:hunter2@internal.example","port":443}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	obs := Read(path, Window{Start: time.UnixMilli(0)})
	for _, e := range obs.Events {
		if strings.Contains(e.Host, "@") || strings.Contains(e.Host, "hunter2") {
			t.Errorf("a credential reached an event: %+v", e)
		}
	}
	if obs.UnparseableTargets != 1 {
		t.Errorf("unparseable_targets = %d, want 1", obs.UnparseableTargets)
	}
}

// TestRead_AFileOfUnlearnedTypesIsNotAQuietSession is the drift case, and it
// is the one the first version of the Observed guard missed.
//
// `parsed++` ran above the type switch, so a well-formed record of a type this
// reader has not learned counted as parsed. log_allowed and log_denied are not
// hypothetical names: this package's doc records them as what nono's own
// documentation claims the program emits, and it does not. A future version
// that does emit them would have rendered as a quiet sandbox.
func TestRead_AFileOfUnlearnedTypesIsNotAQuietSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drift.ndjson")
	body := `{"sequence":0,"event":{"type":"log_allowed","target":"x.example"}}` + "\n" +
		`{"sequence":1,"event":{"type":"log_denied","target":"y.example"}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	obs := Read(path, Window{Start: time.UnixMilli(0)})
	if obs.Observed {
		t.Errorf("a file of only unlearned record types reported as observed; it is "+
			"indistinguishable from a sandbox that genuinely saw nothing (reason=%q, "+
			"skipped=%d)", obs.Reason, obs.Skipped)
	}
	if obs.Skipped != 2 {
		t.Errorf("skipped = %d, want 2", obs.Skipped)
	}
}
