package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// trailWith is an observed trail holding the given events, at one instant.
func trailWith(events ...nono.Event) nono.Observation {
	at := time.UnixMilli(1790725093166)
	for i := range events {
		events[i].At = at
	}
	return nono.Observation{Observed: true, Events: events, Sessions: 1}
}

// renderNono is the sandbox section as the text report prints it.
func renderNono(n Nono) string {
	var b bytes.Buffer
	writeNono(&b, n)
	return b.String()
}

// TestNono_AnUnknownModeIsCountedWithoutAProxyStore: the default path is no
// proxy store, and the mode counter sat below the guard that returns when
// there is none, so a transport nono added after this reader was written
// went uncounted for most users. The decision counter had already been moved
// above that guard for the same reason.
func TestNono_AnUnknownModeIsCountedWithoutAProxyStore(t *testing.T) {
	obs := trailWith(
		nono.Event{Host: "pypi.org", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"},
		nono.Event{Host: "tunnel.example", Port: 443, Decision: nono.DecisionAllow, Mode: "tunnel2"},
	)
	n := buildNono(obs, Destinations{}, true, nil)
	if n.UnknownModes != 1 {
		t.Errorf("no proxy store: unknown_modes = %d, want 1", n.UnknownModes)
	}
	if out := renderNono(n); !strings.Contains(out, "    1 event carried a transport this reader does not know\n") {
		t.Errorf("the text does not report the unknown transport:\n%s", out)
	}
}

// TestNono_AnUnknownModeIsCountedOnceWithAProxyStore: with a proxy store the
// reconciliation loop runs as well, and must not count the same event again.
func TestNono_AnUnknownModeIsCountedOnceWithAProxyStore(t *testing.T) {
	obs := trailWith(
		nono.Event{Host: "tunnel.example", Port: 443, Decision: nono.DecisionAllow, Mode: "tunnel2"},
	)
	dests := Destinations{Observed: true, WindowApplied: true}
	n := buildNono(obs, dests, true, nil)
	if n.UnknownModes != 1 {
		t.Errorf("with a proxy store: unknown_modes = %d, want 1", n.UnknownModes)
	}
	// Still read as observable by the proxy, so the gap it leaves is not
	// excused as plain HTTP.
	if len(n.SawWhatTheProxyDidNot) != 1 || len(n.PlainHTTP) != 0 {
		t.Errorf("saw_what_the_proxy_did_not = %v, plain_http = %v; want the host listed and not excused",
			n.SawWhatTheProxyDidNot, n.PlainHTTP)
	}
}

// openWindowCaveat is what the sandbox line says when the session has no end.
const openWindowCaveat = " (no end record, so events up to the end of the trail are counted)"

// TestNono_AWindowWithNoEndIsSaidOnTheSandboxLine: with no end record the
// window runs to the end of the trail, so a later session's traffic on a
// shared trail is counted as this one's. The proxy side says "window not
// applied" in the same case; the sandbox line said nothing.
func TestNono_AWindowWithNoEndIsSaidOnTheSandboxLine(t *testing.T) {
	obs := trailWith(nono.Event{Host: "pypi.org", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"})
	obs.WindowOpen = true
	n := buildNono(obs, Destinations{}, true, nil)
	if !n.WindowOpen {
		t.Fatal("window_open was not carried from the trail into the report")
	}
	want := "  sandbox (nono): 1 allowed, 0 denied in this session's window" + openWindowCaveat + "\n"
	if out := renderNono(n); !strings.Contains(out, want) {
		t.Errorf("the sandbox line does not say the window has no end; want %q in:\n%s", want, out)
	}
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"window_open":true`) {
		t.Errorf("the JSON does not carry window_open: %s", raw)
	}

	obs.WindowOpen = false
	closed := buildNono(obs, Destinations{}, true, nil)
	if out := renderNono(closed); strings.Contains(out, "no end record") {
		t.Errorf("a window with an end carries the open-window caveat:\n%s", out)
	}
	raw, _ = json.Marshal(closed)
	if !strings.Contains(string(raw), `"window_open":false`) {
		t.Errorf("the JSON does not carry window_open:false: %s", raw)
	}
}

// TestNono_ASessionWithNoEndRecordReportsAnOpenWindow drives Build: the
// window comes from the run's coverage records, and a run with a start and
// no end must reach the sandbox line as open.
func TestNono_ASessionWithNoEndRecordReportsAnOpenWindow(t *testing.T) {
	at := firstNonoEvent(t)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "no-end-session"
	if err := st.AppendCoverage(store.Coverage{
		Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: store.PhaseStart,
		RecordedAtMS: at.Add(-time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(st, id, at.Add(time.Hour), WithNonoTrail(nonoFixture))
	if err != nil {
		t.Fatal(err)
	}
	n := rep.Sessions[0].Nono
	if !n.Observed || !n.WindowOpen {
		t.Errorf("observed=%v window_open=%v, want true, true (%s)", n.Observed, n.WindowOpen, n.Reason)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), openWindowCaveat) {
		t.Errorf("the rendered report does not say the window has no end:\n%s", b.String())
	}
}

// TestNono_AnUnreadableTrailSaysHowMuchItCouldNotRead: a trail of only torn
// lines printed "not observed (nono_audit_no_records)", which reads as "nono
// wrote nothing". The JSON carried the skipped count and the text did not.
func TestNono_AnUnreadableTrailSaysHowMuchItCouldNotRead(t *testing.T) {
	trail := filepath.Join(t.TempDir(), "audit-events.ndjson")
	if err := os.WriteFile(trail, []byte(`{"sequence": 0, "eve`+"\n"+`{"sequence": 1, "ev`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	obs := nono.Read(trail, nono.Window{Start: time.Unix(0, 0)})
	n := buildNono(obs, Destinations{}, true, nil)
	want := "  sandbox (nono): not observed (nono_audit_no_records; 2 trail records could not be read)\n"
	if out := renderNono(n); out != want {
		t.Errorf("an all-torn trail rendered\n%q\nwant\n%q", out, want)
	}

	one := buildNono(nono.Observation{Reason: nono.NotObservedNoRecords, Skipped: 1}, Destinations{}, true, nil)
	want = "  sandbox (nono): not observed (nono_audit_no_records; 1 trail record could not be read)\n"
	if out := renderNono(one); out != want {
		t.Errorf("one unreadable record rendered\n%q\nwant\n%q", out, want)
	}

	// Nothing skipped: the line is as it was.
	none := buildNono(nono.Observation{Reason: nono.NotObservedNoRecords}, Destinations{}, true, nil)
	want = "  sandbox (nono): not observed (nono_audit_no_records)\n"
	if out := renderNono(none); out != want {
		t.Errorf("an empty trail rendered\n%q\nwant\n%q", out, want)
	}
}

// TestNono_TheUnknownDecisionLineAgreesInNumber: "1 sandbox event ... and
// were counted" did not agree with its subject.
func TestNono_TheUnknownDecisionLineAgreesInNumber(t *testing.T) {
	for _, c := range []struct {
		events int
		want   string
	}{
		{1, "    1 sandbox event carried a decision this reader does not know, and was counted in neither column\n"},
		{2, "    2 sandbox events carried a decision this reader does not know, and were counted in neither column\n"},
	} {
		var events []nono.Event
		for i := 0; i < c.events; i++ {
			events = append(events, nono.Event{Host: "x.example", Port: 443, Decision: "challenge", Mode: "connect"})
		}
		n := buildNono(trailWith(events...), Destinations{}, true, nil)
		if n.UnknownDecisions != c.events {
			t.Fatalf("premise: unknown_decisions = %d, want %d", n.UnknownDecisions, c.events)
		}
		if out := renderNono(n); !strings.Contains(out, c.want) {
			t.Errorf("want %q in:\n%s", c.want, out)
		}
	}
}
