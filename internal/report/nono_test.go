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
//
// NetworkEventsInTrail is set to len(events): every caller here builds a
// trail that DID write network records, which is the ordinary case this
// package's other tests check. A test of the zero-network-events case
// (TestNono_ANoNetworkEventsTrailSaysSoInsteadOfZero) sets it explicitly on
// a trail it builds itself, because that is the one fact trailWith's
// default would otherwise hide.
func trailWith(events ...nono.Event) nono.Observation {
	at := time.UnixMilli(1790725093166)
	for i := range events {
		events[i].At = at
	}
	return nono.Observation{Observed: true, Events: events, Sessions: 1, NetworkEventsInTrail: len(events)}
}

// renderNono is the sandbox section as the text report prints it.
func renderNono(n Nono) string {
	var b bytes.Buffer
	writeNono(&b, n)
	return b.String()
}

// TestNono_AnUnknownModeIsCountedWithoutAProxyStore: the mode counter sat
// below a guard that returned early in one configuration, so a transport
// nono added after this reader was written went uncounted there. The decision
// counter had already been moved above that guard for the same reason; this
// pins the mode counter the same way.
func TestNono_AnUnknownModeIsCountedWithoutAProxyStore(t *testing.T) {
	obs := trailWith(
		nono.Event{Host: "pypi.org", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"},
		nono.Event{Host: "tunnel.example", Port: 443, Decision: nono.DecisionAllow, Mode: "tunnel2"},
	)
	n := buildNono(obs, true, false, nil, nil)
	if n.UnknownModes != 1 {
		t.Errorf("unknown_modes = %d, want 1", n.UnknownModes)
	}
	if out := renderNono(n); !strings.Contains(out, "    1 event carried a transport this reader does not know\n") {
		t.Errorf("the text does not report the unknown transport:\n%s", out)
	}
}

// openWindowCaveat is what the sandbox line says when the session has no end.
const openWindowCaveat = " (no end record, so events up to the end of the trail are counted)"

// TestNono_AWindowWithNoEndIsSaidOnTheSandboxLine: with no end record the
// window runs to the end of the trail, so a later session's traffic on a
// shared trail is counted as this one's.
func TestNono_AWindowWithNoEndIsSaidOnTheSandboxLine(t *testing.T) {
	obs := trailWith(nono.Event{Host: "pypi.org", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"})
	obs.WindowOpen = true
	n := buildNono(obs, true, false, nil, nil)
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
	closed := buildNono(obs, true, false, nil, nil)
	if out := renderNono(closed); strings.Contains(out, "no end record") {
		t.Errorf("a window with an end carries the open-window caveat:\n%s", out)
	}
	raw, err = json.Marshal(closed)
	if err != nil {
		t.Fatal(err)
	}
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
	n := buildNono(obs, true, false, nil, nil)
	want := "  sandbox (nono): not observed (nono_audit_no_records; 2 trail records could not be read)\n"
	if out := renderNono(n); out != want {
		t.Errorf("an all-torn trail rendered\n%q\nwant\n%q", out, want)
	}

	one := buildNono(nono.Observation{Reason: nono.NotObservedNoRecords, Skipped: 1}, true, false, nil, nil)
	want = "  sandbox (nono): not observed (nono_audit_no_records; 1 trail record could not be read)\n"
	if out := renderNono(one); out != want {
		t.Errorf("one unreadable record rendered\n%q\nwant\n%q", out, want)
	}

	// Nothing skipped: the line is as it was.
	none := buildNono(nono.Observation{Reason: nono.NotObservedNoRecords}, true, false, nil, nil)
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
		n := buildNono(trailWith(events...), true, false, nil, nil)
		if n.UnknownDecisions != c.events {
			t.Fatalf("premise: unknown_decisions = %d, want %d", n.UnknownDecisions, c.events)
		}
		if out := renderNono(n); !strings.Contains(out, c.want) {
			t.Errorf("want %q in:\n%s", c.want, out)
		}
	}
}

// TestNono_TheClientsOwnTrafficIsNotTheSessions is the live finding of
// 2026-10-09 (Claude Code 2.1.285 inside nono 0.79.0): the
// sandbox line counted api.anthropic.com as an allowed host of the session and
// listed the client's log upload as "refused by the sandbox" -- an attempt the
// agent never made. The events below are that trail's hosts.
func TestNono_TheClientsOwnTrafficIsNotTheSessions(t *testing.T) {
	const intake = "http-intake.logs.us5.datadoghq.com"
	obs := trailWith(
		nono.Event{Host: "api.anthropic.com", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"},
		nono.Event{Host: "pypi.org", Port: 443, Decision: nono.DecisionAllow, Mode: "connect"},
		nono.Event{Host: "github.com", Port: 443, Decision: nono.DecisionDeny, Mode: "connect"},
		nono.Event{Host: intake, Port: 443, Decision: nono.DecisionDeny, Mode: "connect"},
	)
	n := buildNono(obs, true, false, nil, nil)
	if got, want := strings.Join(n.Allowed, ","), "pypi.org"; got != want {
		t.Errorf("allowed = %q, want %q: the client's model traffic counted as the agent's", got, want)
	}
	if got, want := strings.Join(n.Denied, ","), "github.com"; got != want {
		t.Errorf("denied = %q, want %q: the client's telemetry read as an attempt the agent made", got, want)
	}
	if got, want := strings.Join(n.ClientPlaneAllowed, ","), "api.anthropic.com"; got != want {
		t.Errorf("client_plane_allowed = %q, want %q: named apart, not dropped", got, want)
	}
	if got, want := strings.Join(n.ClientPlaneDenied, ","), intake; got != want {
		t.Errorf("client_plane_denied = %q, want %q: named apart, not dropped", got, want)
	}
	out := renderNono(n)
	for _, want := range []string{
		"sandbox (nono): 1 allowed, 1 denied in this session's window\n",
		"    refused by the sandbox: github.com\n",
		"    the client's own traffic, not counted above: allowed api.anthropic.com; refused " + intake + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestNono_AReadThatStopsPartwaySaysHowMuchItCouldNotRead: on
// nono_audit_unreadable the scan stopped at the line that broke it, so
// Skipped counts only the torn lines BEFORE that point. writeNono printed
// the count on every not-observed reason, so two torn lines followed by a
// line over the scanner's cap read as "2 trail records could not be read",
// however much of the trail follows the point where the read gave up. The
// count is only complete when the reader read the whole file, which is the
// nono_audit_no_records case alone.
func TestNono_AReadThatStopsPartwaySaysHowMuchItCouldNotRead(t *testing.T) {
	trail := filepath.Join(t.TempDir(), "audit-events.ndjson")
	var body []byte
	body = append(body, []byte(`{"sequence": 0, "eve`+"\n")...)
	body = append(body, []byte(`{"sequence": 1, "ev`+"\n")...)
	body = append(body, bytes.Repeat([]byte("a"), 9*1024*1024)...)
	body = append(body, '\n')
	if err := os.WriteFile(trail, body, 0o600); err != nil {
		t.Fatal(err)
	}
	obs := nono.Read(trail, nono.Window{Start: time.Unix(0, 0)})
	if obs.Observed || obs.Reason != nono.NotObservedUnreadable {
		t.Fatalf("premise: observed=%v reason=%q, want false, %q", obs.Observed, obs.Reason, nono.NotObservedUnreadable)
	}
	n := buildNono(obs, true, false, nil, nil)
	want := "  sandbox (nono): not observed (nono_audit_unreadable)\n"
	if out := renderNono(n); out != want {
		t.Errorf("a read that stopped partway rendered\n%q\nwant\n%q (no skipped count: it is an undercount)",
			out, want)
	}
}

// TestNono_ANoNetworkEventsTrailSaysSoInsteadOfZero.
//
// A trail holding only session_started and session_ended records is
// Observed (both are known record types), but it is a different fact from a
// session the sandbox filtered and saw nothing on: the sandbox never wrote a
// network decision at all, which happens on nono 0.78.0/0.79 whenever the
// active profile does not filter the network. Rendering it as "0 allowed, 0
// denied in this session's window" reads as a quiet, filtered session --
// silence read as zero, the exact failure this package exists to prevent --
// when no filtering happened. Noted as a follow-up in #53 and not fixed
// there: "the reader could say 'the trail holds no network events' instead
// of rendering 0/0 as observed."
func TestNono_ANoNetworkEventsTrailSaysSoInsteadOfZero(t *testing.T) {
	obs := nono.Observation{Observed: true, Events: []nono.Event{}, Sessions: 1, NetworkEventsInTrail: 0}
	n := buildNono(obs, true, false, nil, nil)
	want := "  sandbox (nono): the trail holds no network events (the active profile may not " +
		"filter the network; this is not the same as a filtered session that saw nothing)\n"
	if out := renderNono(n); !strings.Contains(out, want) {
		t.Errorf("want %q in:\n%s", want, out)
	}
	if strings.Contains(renderNono(n), "0 allowed, 0 denied") {
		t.Errorf("a trail with no network events at all rendered as a filtered, quiet session:\n%s", renderNono(n))
	}

	// A session that genuinely saw traffic and allowed or denied none of it
	// in-window still renders the ordinary 0/0 line: NetworkEventsInTrail > 0
	// says the sandbox did filter, and found nothing for this window.
	filtered := trailWith()
	filtered.Observed = true
	filtered.NetworkEventsInTrail = 3
	fn := buildNono(filtered, true, false, nil, nil)
	if out := renderNono(fn); !strings.Contains(out, "0 allowed, 0 denied in this session's window") {
		t.Errorf("a filtered session with no in-window events lost the ordinary 0/0 line:\n%s", out)
	}
}

// TestNono_TheMCPTransportIsTheAgentsOnAnMCPSession: the one client-plane host
// that can belong to the agent stays in the agent's columns when the session
// made mcp__* calls, as it does in the novelty candidates.
func TestNono_TheMCPTransportIsTheAgentsOnAnMCPSession(t *testing.T) {
	obs := trailWith(nono.Event{Host: mcpProxyHost, Port: 443, Decision: nono.DecisionAllow, Mode: "connect"})
	if n := buildNono(obs, true, true, nil, nil); strings.Join(n.Allowed, ",") != mcpProxyHost || len(n.ClientPlaneAllowed) != 0 {
		t.Errorf("mcp session: allowed %v, client_plane_allowed %v; want the transport as the agent's", n.Allowed, n.ClientPlaneAllowed)
	}
	if n := buildNono(obs, true, false, nil, nil); len(n.Allowed) != 0 || strings.Join(n.ClientPlaneAllowed, ",") != mcpProxyHost {
		t.Errorf("no mcp calls: allowed %v, client_plane_allowed %v; want the transport as the client's", n.Allowed, n.ClientPlaneAllowed)
	}
}
