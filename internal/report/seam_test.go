package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// THE SEAM TESTS.
//
// A unit test on buildNono proves the function right given an Observation; it
// cannot show that Build actually constructs one and wires it in. That wire
// broke silently more than once: once between Build and buildNono, and again
// between main and WithNonoTrail, at a level this package's own tests could
// not see from the function alone -- a guard one level below the actual break
// is not a guard.
//
// These go through report.Build against a real recorded trail, which is the
// only path that proves the pieces are actually joined.

// TestSeam_TheNonoTrailReachesTheReport.
//
// Written because the adapter was, for a while, exactly what it warns about:
// a package that compiled, had nine green tests against a real capture, and
// was never called. buildNono existed, WithNonoTrail existed, the renderer
// existed, and Build never invoked any of it -- so the report printed
// "sandbox (nono): not observed (unknown)" on a session with a perfectly good
// trail, which reads as an honest answer.
//
// This goes red the moment that wire is cut again.
func TestSeam_TheNonoTrailReachesTheReport(t *testing.T) {
	// THE WINDOW MUST CONTAIN THE FIXTURE'S INSTANTS. They are real wall-clock
	// times from the machine that recorded the capture, not relative offsets,
	// so a window built around time.Now() excludes all four events -- which
	// reads as "the adapter is not wired" and is not.
	at := firstNonoEvent(t)
	st, sessionID, now := seamSessionAt(t, at)

	rep, err := Build(st, sessionID, now, WithNonoTrail(nonoFixture))
	if err != nil {
		t.Fatal(err)
	}
	n := rep.Sessions[0].Nono

	if !n.Observed {
		t.Fatalf("the trail was not read: %q. The adapter is not wired into Build.", n.Reason)
	}
	if len(n.Allowed) == 0 {
		t.Error("no allowed hosts reached the report from a trail that has two")
	}
	if len(n.Denied) == 0 {
		t.Error("no denied hosts reached the report from a trail that has two")
	}

	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "sandbox (nono):") {
		t.Errorf("the sandbox line is absent from a rendered report:\n%s", b.String())
	}
	if strings.Contains(b.String(), "sandbox (nono): not observed") {
		t.Errorf("a readable trail rendered as not observed:\n%s", b.String())
	}
}

// TestSeam_NoTrailConfiguredIsSilent.
//
// This asserted the OPPOSITE when it was written -- that the section always
// prints, on the rule that a section which vanishes cannot be told apart from
// one that was never built. H-28 caught it: that rule is right for a source
// the session was USING and wrong for an optional one it was not. Printing
// "not observed" for a sandbox nobody asked for puts a degradation marker on
// every healthy run, and a reader who meets the word there stops believing it.
func TestSeam_NoTrailConfiguredIsSilent(t *testing.T) {
	st, sessionID, now := seamSession(t)
	rep, err := Build(st, sessionID, now)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	// SILENT, and that is the corrected rule. H-28 asserts a healthy run shows
	// no degradation marker, and most sessions run no sandbox at all -- so
	// "not observed" here would print that marker on every clean session and
	// teach readers to discount the word.
	if strings.Contains(b.String(), "sandbox (nono)") {
		t.Errorf("a session with no sandbox configured rendered a sandbox line:\n%s",
			b.String())
	}
}

// seamSession builds a minimal recorded session whose window contains now.
func seamSession(t *testing.T) (*store.Store, string, time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	const id = "seam-session"
	// The window must CONTAIN the rows. The first version of this helper put
	// both the start and the end a minute in the past, so the window closed
	// before the fixture rows existed and every one of them read as inherited
	// -- four failing seam tests that looked like a broken join and were a
	// broken fixture. Asserted below rather than left to be inferred.
	for phase, at := range map[string]time.Time{
		store.PhaseStart: now.Add(-time.Minute),
		store.PhaseEnd:   now.Add(time.Minute),
	} {
		if err := st.AppendCoverage(store.Coverage{
			Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: phase,
			RecordedAtMS: at.UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, id, now
}

const nonoFixture = "../../test/fixtures/nono/audit-events.ndjson"

// firstNonoEvent reads the fixture's earliest instant, so a window can be
// built around data rather than around the clock.
func firstNonoEvent(t *testing.T) time.Time {
	t.Helper()
	obs := nono.Read(nonoFixture, nono.Window{Start: time.Unix(0, 0)})
	if !obs.Observed || len(obs.Events) == 0 {
		t.Fatalf("premise: the nono fixture yields no events (%s)", obs.Reason)
	}
	return obs.Events[0].At
}

// seamSessionAt is seamSession with the window centred on a given instant.
func seamSessionAt(t *testing.T, at time.Time) (*store.Store, string, time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "seam-session"
	for phase, when := range map[string]time.Time{
		store.PhaseStart: at.Add(-time.Minute),
		store.PhaseEnd:   at.Add(time.Hour),
	} {
		if err := st.AppendCoverage(store.Coverage{
			Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: phase,
			RecordedAtMS: when.UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, id, at.Add(time.Hour)
}

// TestSeam_AForgottenHostDoesNotReturnUnderTheSandboxHeading.
//
// buildNono used to read the Destinations view for the stated reason that it
// had "already suppressed forgotten hosts" -- true of the wire side only.
// The trail is a second place the name lives, and leaving it unfiltered did
// worse than republish: suppression removed the host from the wire side, so
// the forgotten host became the one thing the section called out. Forgetting
// PROMOTED it. There is no wire side any more, but the trail is still a
// second place a forgotten name can live, and nothing in internal/report's
// own tests reaches buildNono's forgotten parameter with a non-nil predicate
// except this one.
func TestSeam_AForgottenHostDoesNotReturnUnderTheSandboxHeading(t *testing.T) {
	at := firstNonoEvent(t)
	st, id, now := seamSessionAt(t, at)

	// A DECLARATION NAMING THE HOST, because forget --host writes a gap only
	// for runs where it evicted something -- and the predicate that drives
	// suppression is built from those gaps. Without this the forget is a
	// silent no-op and the test passes for the wrong reason.
	//
	// That hole -- a host only the sandbox's trail held could not be forgotten
	// at all -- was found here and is closed by the store-level forget record
	// (#45); TestSeam_AHostOnlyTheSandboxSawCanBeForgotten pins it.
	if err := st.AppendDeclaration(store.Declaration{
		Type: "declaration", SchemaVersion: 2, SessionID: id,
		ToolUseID: "toolu_forget", ToolName: "Bash",
		RecordedAtMS: at.UnixMilli(), Hosts: []string{"pypi.org"},
	}); err != nil {
		t.Fatal(err)
	}

	// PREMISE: pypi.org is really in the real capture's trail, allowed, inside
	// this window -- not a host that was simply never there to begin with.
	before, err := Build(st, id, now, WithNonoTrail(nonoFixture))
	if err != nil {
		t.Fatal(err)
	}
	var sawPypi bool
	for _, h := range before.Sessions[0].Nono.Allowed {
		if h == "pypi.org" {
			sawPypi = true
		}
	}
	if !sawPypi {
		t.Fatalf("premise: pypi.org is not among the trail's allowed hosts before the "+
			"forget, so its absence after one proves nothing: %v",
			before.Sessions[0].Nono.Allowed)
	}

	if _, err := st.ForgetHost("pypi.org", now); err != nil {
		t.Fatalf("forget --host: %v", err)
	}
	forgotten, err := st.ForgottenHost()
	if err != nil {
		t.Fatal(err)
	}
	if !forgotten("pypi.org") {
		t.Fatal("premise: the forget wrote no gap, so nothing is suppressed and this " +
			"test would pass for the wrong reason")
	}

	rep, err := Build(st, id, now, WithNonoTrail(nonoFixture))
	if err != nil {
		t.Fatal(err)
	}
	// THE LISTS, not only the text. The sandbox line renders allowed hosts as
	// a count, so a forgotten host left in Allowed never reached the page and
	// a text-only check passed with it there -- while --json carried it.
	n := rep.Sessions[0].Nono
	if !reflect.DeepEqual(n.Allowed, []string{"example.com"}) {
		t.Errorf("nono.allowed after the forget = %v, want [example.com]: the forgotten host "+
			"is still among what the sandbox allowed", n.Allowed)
	}
	if !reflect.DeepEqual(n.Denied, []string{"169.254.169.254", "github.com"}) {
		t.Errorf("nono.denied after the forget = %v, want the two refused hosts and nothing else", n.Denied)
	}
	body, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "pypi.org") {
		t.Errorf("a forgotten host is in the report's JSON:\n%s", locate(string(body), "pypi.org"))
	}

	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "pypi.org") {
		t.Errorf("a forgotten host returned under the sandbox heading:\n%s", b.String())
	}
	// The count is the text's only trace of the allowed list, so it is the
	// assertion the text can make: one allowed, not two.
	if !strings.Contains(b.String(), "sandbox (nono): 1 allowed, 2 denied") {
		t.Errorf("the sandbox line does not count the forgotten host out:\n%s", b.String())
	}
}

// TestSeam_AHostOnlyTheSandboxSawCanBeForgotten is #45, in both of the shapes
// the live nono run produced.
//
//	only-trail  no record of rashomon's ever named the host; only the
//	            sandbox's trail holds it.
//	same-call   the host WAS named, by a call an earlier forget of another
//	            host already removed -- so this forget removes nothing either.
//	            Measured on nono 0.79: forgetting github.com took the call that
//	            also named example.com, and example.com could then never leave
//	            the sandbox's allowed list.
func TestSeam_AHostOnlyTheSandboxSawCanBeForgotten(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, st *store.Store, id string, at, now time.Time)
	}{
		{"only-trail", func(*testing.T, *store.Store, string, time.Time, time.Time) {}},
		{"same-call", func(t *testing.T, st *store.Store, id string, at, now time.Time) {
			if err := st.AppendDeclaration(store.Declaration{
				Type: "declaration", SchemaVersion: 2, SessionID: id,
				ToolUseID: "toolu_both", ToolName: "Bash",
				RecordedAtMS: at.UnixMilli(), Hosts: []string{"github.com", "pypi.org"},
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := st.ForgetHost("github.com", now); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := firstNonoEvent(t)
			st, id, now := seamSessionAt(t, at)
			tc.setup(t, st, id, at, now)

			gaps, err := st.ForgetHost("pypi.org", now)
			if err != nil {
				t.Fatalf("forget --host: %v", err)
			}
			if len(gaps) != 0 {
				t.Fatalf("premise: this forget must remove nothing, or the gap path is "+
					"what suppresses the host; it wrote %d gap(s)", len(gaps))
			}

			rep, err := Build(st, id, now, WithNonoTrail(nonoFixture))
			if err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			if err := Text(&b, rep); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(b.String(), "sandbox (nono):") {
				t.Fatalf("premise: the sandbox section did not render:\n%s", b.String())
			}
			// THE COUNT, NOT THE NAME. The text names only refused hosts, and
			// pypi.org is an allowed one, so a search for its name passed
			// whether or not the forget worked. The fixture
			// allows example.com and pypi.org; with pypi.org forgotten the
			// count is 1.
			if !strings.Contains(b.String(), "sandbox (nono): 1 allowed,") {
				t.Errorf("a host forgotten with nothing to remove is still counted under the "+
					"sandbox heading (#45):\n%s", b.String())
			}
			if strings.Contains(b.String(), "pypi.org") {
				t.Errorf("a host forgotten with nothing to remove is back under the "+
					"sandbox heading (#45):\n%s", b.String())
			}
			for _, s := range rep.Sessions {
				for _, h := range append(append([]string{}, s.Nono.Allowed...), s.Nono.Denied...) {
					if h == "pypi.org" {
						t.Errorf("the JSON still lists the forgotten host: %v / %v",
							s.Nono.Allowed, s.Nono.Denied)
					}
				}
			}
		})
	}
}

// TestSeam_TheMCPTransportReachesTheSandboxSection: whether the session made
// mcp__* calls is decided from its declarations, and that answer has to reach
// the sandbox section through Build, or the MCP transport is filed as the
// client's own traffic on the one kind of session where it is the agent's.
func TestSeam_TheMCPTransportReachesTheSandboxSection(t *testing.T) {
	at := firstNonoEvent(t)
	st, id, now := seamSessionAt(t, at)
	if err := st.AppendDeclaration(store.Declaration{
		Type: "declaration", SchemaVersion: 2, SessionID: id,
		ToolUseID: "toolu_mcp", ToolName: "mcp__docs__search",
		RecordedAtMS: at.UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	trail := filepath.Join(t.TempDir(), "mcp.ndjson")
	body := `{"sequence":0,"event":{"type":"session_started"}}` + "\n" +
		ev(1, at.UnixMilli(), "connect", "allow", mcpProxyHost, 443)
	if err := os.WriteFile(trail, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(st, id, now, WithNonoTrail(trail))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sessions) != 1 || !rep.Sessions[0].Nono.Observed {
		t.Fatalf("premise: the sandbox section did not observe the trail: %+v", rep.Sessions)
	}
	if n := rep.Sessions[0].Nono; strings.Join(n.Allowed, ",") != mcpProxyHost {
		t.Errorf("on a session that made mcp__* calls the MCP transport is the agent's; "+
			"allowed %v, client_plane_allowed %v", n.Allowed, n.ClientPlaneAllowed)
	}
}

// ev builds one nono network record.
func ev(seq int, ms int64, mode, decision, target string, port int) string {
	return fmt.Sprintf(
		`{"sequence":%d,"event":{"type":"network","event":{"timestamp_unix_ms":%d,`+
			`"mode":%q,"decision":%q,"target":%q,"port":%d}}}`+"\n",
		seq, ms, mode, decision, target, port)
}

// TestSeam_AnySpellingOfAForgottenHostLeavesTheSandboxSection: the trail's
// hosts are canonical, so a forget has to digest the canonical spelling of
// whatever the user typed. github.com is the fixture's refused host, which
// the text names, so the text assertion can fail; pypi.org is an allowed
// host, which the text only counts, so it is checked in the JSON.
func TestSeam_AnySpellingOfAForgottenHostLeavesTheSandboxSection(t *testing.T) {
	for _, c := range []struct{ spelled, canonical string }{
		{"GitHub.com", "github.com"},
		{"github.com.", "github.com"},
		{"https://github.com/", "github.com"},
		{"github.com:443", "github.com"},
		{"PyPI.org", "pypi.org"},
		{"pypi.org:443", "pypi.org"},
	} {
		t.Run(c.spelled, func(t *testing.T) {
			at := firstNonoEvent(t)
			st, id, now := seamSessionAt(t, at)
			if _, err := st.ForgetHost(c.spelled, now); err != nil {
				t.Fatalf("forget --host %q: %v", c.spelled, err)
			}
			rep, err := Build(st, id, now, WithNonoTrail(nonoFixture))
			if err != nil {
				t.Fatal(err)
			}
			var b strings.Builder
			if err := Text(&b, rep); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(b.String(), "sandbox (nono):") {
				t.Fatalf("premise: the sandbox section did not render:\n%s", b.String())
			}
			if strings.Contains(b.String(), c.canonical) {
				t.Errorf("forget --host %q left %s in the report:\n%s", c.spelled, c.canonical, b.String())
			}
			n := rep.Sessions[0].Nono
			if slices.Contains(n.Allowed, c.canonical) || slices.Contains(n.Denied, c.canonical) {
				t.Errorf("forget --host %q: the sandbox still lists %s: allowed %v, denied %v",
					c.spelled, c.canonical, n.Allowed, n.Denied)
			}
		})
	}
}

// trailOf writes a nono trail holding one connect event per host and
// decision, all at one instant, and returns its path.
func trailOf(t *testing.T, at time.Time, events ...[2]string) string {
	t.Helper()
	body := `{"sequence":0,"event":{"type":"session_started"}}` + "\n"
	for i, e := range events {
		body += ev(i+1, at.UnixMilli(), "connect", e[1], e[0], 443)
	}
	path := filepath.Join(t.TempDir(), "trail.ndjson")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSeam_AClientPlaneHostAToolCallNamedIsTheAgents: the client-plane list
// says which hosts the CLIENT contacts on its own, and a tool call naming one
// is the agent's attempt, not the client's. With the list applied to the
// sandbox section alone, a refused `curl` to the Datadog log intake -- a
// shared endpoint that takes posts under any account's key, the kind of place
// data leaves through -- vanished from "refused by the sandbox" and was named
// as the client's own traffic, the agent's attempt relabelled as not its own.
func TestSeam_AClientPlaneHostAToolCallNamedIsTheAgents(t *testing.T) {
	for _, h := range []string{"http-intake.logs.us5.datadoghq.com", "sentry.io", "api.anthropic.com"} {
		for _, decision := range []string{"deny", "allow"} {
			t.Run(h+"/"+decision, func(t *testing.T) {
				at := firstNonoEvent(t)
				st, id, now := seamSessionAt(t, at)
				if err := st.AppendDeclaration(store.Declaration{
					Type: "declaration", SchemaVersion: 2, SessionID: id,
					ToolUseID: "toolu_curl", ToolName: "Bash",
					RecordedAtMS: at.UnixMilli(), Hosts: []string{h},
				}); err != nil {
					t.Fatal(err)
				}
				rep, err := Build(st, id, now, WithNonoTrail(trailOf(t, at, [2]string{h, decision})))
				if err != nil {
					t.Fatal(err)
				}
				n := rep.Sessions[0].Nono
				column, want := n.Allowed, "sandbox (nono): 1 allowed, 0 denied"
				if decision == "deny" {
					column, want = n.Denied, "    refused by the sandbox: "+h+"\n"
				}
				if strings.Join(column, ",") != h {
					t.Errorf("the sandbox's %s of %s, a host a tool call named, is not in the "+
						"agent's column: allowed %v, denied %v", decision, h, n.Allowed, n.Denied)
				}
				var b strings.Builder
				if err := Text(&b, rep); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(b.String(), want) {
					t.Errorf("want %q in:\n%s", want, b.String())
				}
				if strings.Contains(b.String(), "the client's own traffic") {
					t.Errorf("the agent's own attempt is named as the client's traffic:\n%s", b.String())
				}
			})
		}
	}
}

// TestSeam_TheClientsOwnTrafficSaysWhatTheSandboxDid: the client-traffic line
// is there so a reader comparing against nono's own log finds every host, and
// a host without its decision is half of that. One list fed from both the
// allowed and the refused hosts rendered a refused log upload -- the very
// case the line was added for -- under "1 allowed, 0 denied" with nothing to
// say it was refused. The JSON is split the same way.
func TestSeam_TheClientsOwnTrafficSaysWhatTheSandboxDid(t *testing.T) {
	const intake = "http-intake.logs.us5.datadoghq.com"
	at := firstNonoEvent(t)
	st, id, now := seamSessionAt(t, at)
	rep, err := Build(st, id, now, WithNonoTrail(trailOf(t, at,
		[2]string{"api.anthropic.com", "allow"}, [2]string{intake, "deny"},
		[2]string{"pypi.org", "allow"})))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"sandbox (nono): 1 allowed, 0 denied in this session's window\n",
		"    the client's own traffic, not counted above: allowed api.anthropic.com; refused " + intake + "\n",
	} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("want %q in:\n%s", want, b.String())
		}
	}
	got := nonoJSON(t, rep)
	if a, d := got["client_plane_allowed"], got["client_plane_denied"]; fmt.Sprint(a) != "[api.anthropic.com]" || fmt.Sprint(d) != "["+intake+"]" {
		t.Errorf("client_plane_allowed = %v, client_plane_denied = %v; want each decision in its own list", a, d)
	}
	if _, merged := got["client_plane"]; merged {
		t.Errorf("the JSON still carries one client_plane list with no decision: %v", got["client_plane"])
	}
}

// TestSeam_TheClientsOwnTrafficIsListedInOrder: every client host, met in
// reverse order in the trail, comes out sorted, run after run. The list used
// to be collected from a map, and only a sort made it deterministic; a test
// with two hosts caught that sort's removal by luck. Each event is a
// millisecond later than the one before, so the trail reader's own ordering
// (by time, then host) keeps them reversed, and only the sort can put them
// back.
func TestSeam_TheClientsOwnTrafficIsListedInOrder(t *testing.T) {
	var hosts []string
	for h := range clientPlaneHosts {
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	want := fmt.Sprint(hosts)
	slices.Reverse(hosts)
	at := firstNonoEvent(t)
	st, id, now := seamSessionAt(t, at)
	body := `{"sequence":0,"event":{"type":"session_started"}}` + "\n"
	for i, h := range hosts {
		body += ev(i+1, at.UnixMilli()+int64(i), "connect", "allow", h, 443)
	}
	trail := filepath.Join(t.TempDir(), "trail.ndjson")
	if err := os.WriteFile(trail, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		rep, err := Build(st, id, now, WithNonoTrail(trail))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(nonoJSON(t, rep)["client_plane_allowed"]); got != want {
			t.Fatalf("run %d: client_plane_allowed = %s, want %s", i, got, want)
		}
	}
}

// nonoJSON is the sandbox section of the first session as its JSON reader
// sees it.
func nonoJSON(t *testing.T, rep *Report) map[string]any {
	t.Helper()
	raw, err := json.Marshal(rep.Sessions[0].Nono)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
