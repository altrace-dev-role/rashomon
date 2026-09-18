package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// runWithDeclaredHosts builds the declaration side of the comparison.
func runWithDeclaredHosts(hosts ...string) *store.Run {
	return &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "toolu_1",
			ToolName:  "Bash",
			Hosts:     hosts,
		}},
	}
}

func observed(hosts ...wire.Destination) wire.Observation {
	obs := wire.Observation{Observed: true, WindowApplied: true, Hosts: hosts}
	for _, h := range hosts {
		obs.Attempts += h.Attempts
		// Inherited attempts are carried too. The helper used to drop them, so a
		// fixture with an inherited host produced a Destinations whose Inherited
		// was zero and whose inherited line therefore never rendered -- which is
		// why the first version of the client-plane test failed with the flag set
		// correctly and no line to show it on.
		obs.Inherited += h.InheritedAttempts
		if h.Attempts > 0 {
			obs.DistinctHosts++
		}
	}
	return obs
}

func has(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestWireOnly_FiresOnAHostNoDeclarationNamed is the product's central line and
// the reason this package reconciles two records instead of printing one.
//
// The fixture is the real case: the agent runs `pip download requests`, which
// names pypi.org, and the wire also sees files.pythonhosted.org, which the
// index redirected to and which no tool call ever mentioned. No transcript
// reader and no hook log can produce that second host.
func TestWireOnly_FiresOnAHostNoDeclarationNamed(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		observed(
			wire.Destination{Host: "pypi.org", Attempts: 1},
			wire.Destination{Host: "files.pythonhosted.org", Attempts: 1},
		), "", nil)

	if !has(d.WireOnly, "files.pythonhosted.org") {
		t.Errorf("wire_only = %v, want files.pythonhosted.org", d.WireOnly)
	}
	if has(d.WireOnly, "pypi.org") {
		t.Errorf("wire_only = %v; pypi.org WAS declared and must not be a finding", d.WireOnly)
	}
	if d.ProxyOnPath != ProxyOnPathTrue {
		t.Errorf("proxy_on_path = %q, want true", d.ProxyOnPath)
	}
}

// TestWireOnly_DoesNotFireWhenEveryHostWasNamed is the negative fixture. A line
// that fired on a well-behaved session would be noise, and a reader who saw it
// once when it was wrong would discount it when it was right.
func TestWireOnly_DoesNotFireWhenEveryHostWasNamed(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org", "files.pythonhosted.org"),
		observed(
			wire.Destination{Host: "pypi.org", Attempts: 1},
			wire.Destination{Host: "files.pythonhosted.org", Attempts: 2},
		), "", nil)

	if len(d.WireOnly) != 0 {
		t.Errorf("wire_only = %v, want empty: every observed host was declared", d.WireOnly)
	}
	if d.ProxyOnPath != ProxyOnPathTrue {
		t.Errorf("proxy_on_path = %q, want true", d.ProxyOnPath)
	}
}

// TestWireOnly_LoopbackIsNeverAFinding keeps a local dev server out of the
// accusation. An agent talking to 127.0.0.1 is not going somewhere nobody
// wrote down.
func TestWireOnly_LoopbackIsNeverAFinding(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts(),
		observed(
			wire.Destination{Host: "127.0.0.1", Attempts: 3},
			wire.Destination{Host: "localhost", Attempts: 1},
			wire.Destination{Host: "[::1]", Attempts: 1},
		), "", nil)
	if len(d.WireOnly) != 0 {
		t.Errorf("wire_only = %v, want empty for loopback only", d.WireOnly)
	}
}

// TestClientPlane_RendersSeparatelyAndIsNotAFinding covers the host that would
// otherwise produce a false accusation on every single session. Claude Code's
// own model traffic transits the same proxy, so api.anthropic.com appears with
// no declaring tool call in every run — and an agent's own request to that host
// is indistinguishable from the client's.
func TestClientPlane_RendersSeparatelyAndIsNotAFinding(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		observed(
			wire.Destination{Host: "api.anthropic.com", Attempts: 4},
			wire.Destination{Host: "pypi.org", Attempts: 1},
		), "", nil)

	if has(d.WireOnly, "api.anthropic.com") {
		t.Error("api.anthropic.com is in wire_only. It appears with no declaring tool " +
			"call in every session, so treating it as a finding would make the " +
			"central line fire falsely every time and be discounted when real.")
	}
	if !has(d.ClientPlane, "api.anthropic.com") {
		t.Errorf("client_plane = %v, want api.anthropic.com rendered separately", d.ClientPlane)
	}
}

// TestClientPlane_MCPProxyIsClientPlaneWithoutAnMCPCall is the first branch of
// the mcp-proxy ruling.
//
// mcp-proxy.anthropic.com is the client's own transport. A session that made no
// mcp__* call did not reach it through any tool call, so reporting it as
// "reached but never named" would be true of the bytes and false about the
// agent -- the client contacted it whatever the agent did. Six attempts of it
// showed up in the first real demo session and were reported as a finding,
// which is what prompted the ruling.
func TestClientPlane_MCPProxyIsClientPlaneWithoutAnMCPCall(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		observed(
			wire.Destination{Host: "mcp-proxy.anthropic.com", Attempts: 6},
			wire.Destination{Host: "pypi.org", Attempts: 1},
		), "", nil)

	if has(d.WireOnly, "mcp-proxy.anthropic.com") {
		t.Error("mcp-proxy.anthropic.com is reported as a finding on a session with no " +
			"mcp__* call. The client contacts it on its own behalf, so the line would " +
			"be true of the bytes and false about the agent.")
	}
	if !has(d.ClientPlane, "mcp-proxy.anthropic.com") {
		t.Errorf("client_plane = %v, want mcp-proxy.anthropic.com", d.ClientPlane)
	}
}

// TestClientPlane_MCPProxyIsAttributedWhenTheSessionMadeAnMCPCall is the second
// branch, and the reason the ruling is not simply "add it to the list".
//
// When the session DID make an mcp__* call, that host is the transport those
// calls travelled over. Attributing it to them is more accurate than filing it
// under the client plane: the traffic genuinely belongs to the agent's work, and
// burying it would hide the one destination an MCP call can be observed at.
//
// It is still not a wire-only finding. The declaration exists, so the host was
// accounted for -- what changes is which section it is accounted for IN.
func TestClientPlane_MCPProxyIsAttributedWhenTheSessionMadeAnMCPCall(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		{ToolUseID: "t1", ToolName: "mcp__deploy__status"},
		{ToolUseID: "t2", ToolName: "Bash", Hosts: []string{"pypi.org"}},
	}}

	d := buildDestinations(run, observed(
		wire.Destination{Host: "mcp-proxy.anthropic.com", Attempts: 6},
		wire.Destination{Host: "pypi.org", Attempts: 1},
	), "", nil)

	if has(d.ClientPlane, "mcp-proxy.anthropic.com") {
		t.Error("mcp-proxy.anthropic.com is under client_plane although this session made " +
			"an mcp__* call. It is the transport those calls used, so it belongs to the " +
			"agent's work rather than to the client's own traffic.")
	}
	if has(d.WireOnly, "mcp-proxy.anthropic.com") {
		t.Error("mcp-proxy.anthropic.com is reported as reached-but-never-named; the " +
			"mcp__* declaration accounts for it")
	}
	if d.ProxyOnPath != ProxyOnPathTrue {
		t.Errorf("proxy_on_path = %q, want true", d.ProxyOnPath)
	}
	// It must still appear in the per-host list either way: the section it is
	// filed under changes, the record of it does not.
	var listed bool
	for _, h := range d.Hosts {
		if h.Host == "mcp-proxy.anthropic.com" {
			listed = true
		}
	}
	if !listed {
		t.Error("mcp-proxy.anthropic.com vanished from the host list; attribution changes " +
			"which section reports a host, never whether it is reported")
	}
}

// TestProxyOnPath_UnknownWhenTheStoreCouldNotBeRead is the degradation case.
// Printing false would say "the proxy was not observing", which is a different
// and unsupported claim.
func TestProxyOnPath_UnknownWhenTheStoreCouldNotBeRead(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		wire.Observation{Observed: false, Reason: wire.NotObservedNoStore}, "", nil)

	if d.Observed {
		t.Error("observed = true for an unreadable store")
	}
	if d.ProxyOnPath != ProxyOnPathUnknown {
		t.Errorf("proxy_on_path = %q, want unknown: \"false\" would assert the proxy was "+
			"not observing, which is not what an unreadable store shows", d.ProxyOnPath)
	}
	if d.Reason != wire.NotObservedNoStore {
		t.Errorf("reason = %q, want %q", d.Reason, wire.NotObservedNoStore)
	}
	if len(d.WireOnly) != 0 {
		t.Errorf("wire_only = %v, want empty; a finding cannot be derived from records "+
			"that were never read", d.WireOnly)
	}
}

// TestProxyOnPath_UnknownWhenTheWindowIsEmpty separates "the store was readable
// and this session reached nothing" from "we could not tell". Both print
// unknown, and the notes differ, because neither supports the claim that the
// proxy was observing this session.
func TestProxyOnPath_UnknownWhenTheWindowIsEmpty(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		wire.Observation{Observed: true, WindowApplied: true}, "", nil)

	if d.ProxyOnPath != ProxyOnPathUnknown {
		t.Errorf("proxy_on_path = %q, want unknown for a readable store with no rows in "+
			"the window", d.ProxyOnPath)
	}
	if d.ProxyNote == "" {
		t.Error("no note explaining why it is unknown")
	}
}

// TestProxyOnPath_TrueWhenNothingMatchedButRowsExist is the most interesting
// version of "on the path": the proxy was observing, and not one destination it
// saw was named by a tool call.
func TestProxyOnPath_TrueWhenNothingMatchedButRowsExist(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("declared-but-never-reached.example"),
		observed(wire.Destination{Host: "surprise.example", Attempts: 1}), "", nil)

	if d.ProxyOnPath != ProxyOnPathTrue {
		t.Errorf("proxy_on_path = %q, want true: rows inside the window prove the proxy "+
			"was observing, whether or not anything matched", d.ProxyOnPath)
	}
	if !has(d.WireOnly, "surprise.example") {
		t.Errorf("wire_only = %v, want surprise.example", d.WireOnly)
	}
}

// TestDeclaredSSHHostsDoNotSuppressAFinding pins the reason hosts and ssh_hosts
// are separate lists. An ssh host is unobservable by this proxy, so letting it
// satisfy the comparison would silence a real https finding for the same name.
func TestDeclaredSSHHostsDoNotSuppressAFinding(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{{
		ToolUseID: "toolu_1",
		ToolName:  "Bash",
		SSHHosts:  []string{"github.com"},
	}}}

	d := buildDestinations(run, observed(wire.Destination{Host: "github.com", Attempts: 1}), "", nil)

	if !has(d.WireOnly, "github.com") {
		t.Errorf("wire_only = %v, want github.com. It was declared only as an ssh host, "+
			"which this proxy cannot observe, so an https connection to the same "+
			"name is still a destination no tool call named.", d.WireOnly)
	}
}

// TestDeclaredNotObserved_FiresOnADeclaredHostWithNoWireRow is the other
// direction of the comparison, and it is a weaker claim than wire_only by
// nature: a declared host with no wire row may have been a call the user
// denied, a call that failed before connecting, a cached response, or a host
// the proxy simply did not see. The report says which hosts, not why.
func TestDeclaredNotObserved_FiresOnADeclaredHostWithNoWireRow(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org", "never-reached.example"),
		observed(wire.Destination{Host: "pypi.org", Attempts: 1}), "", nil)

	if !has(d.DeclaredNotObserved, "never-reached.example") {
		t.Errorf("declared_not_observed = %v, want never-reached.example", d.DeclaredNotObserved)
	}
	if has(d.DeclaredNotObserved, "pypi.org") {
		t.Errorf("declared_not_observed = %v; pypi.org WAS observed", d.DeclaredNotObserved)
	}
}

// TestDeclaredNotObserved_IsEmptyWhenEverythingWasObserved is the negative
// fixture.
func TestDeclaredNotObserved_IsEmptyWhenEverythingWasObserved(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		observed(wire.Destination{Host: "pypi.org", Attempts: 1}), "", nil)
	if len(d.DeclaredNotObserved) != 0 {
		t.Errorf("declared_not_observed = %v, want empty", d.DeclaredNotObserved)
	}
}

// TestDeclaredNotObserved_IsEmptyWhenTheStoreCouldNotBeRead is the one that
// matters most, and it is the silence-as-zero rule at its sharpest.
//
// With no proxy store, EVERY declared host has no wire row. Listing them all as
// "declared but not observed" would turn "we were not watching" into a finding
// against the agent for every host it honestly named -- the most damaging
// possible inversion, because it fires hardest on the sessions where the tool
// was doing the least.
func TestDeclaredNotObserved_IsEmptyWhenTheStoreCouldNotBeRead(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org", "api.github.com"),
		wire.Observation{Observed: false, Reason: wire.NotObservedNoStore}, "", nil)

	if len(d.DeclaredNotObserved) != 0 {
		t.Errorf("declared_not_observed = %v with no store read. Every declared host "+
			"trivially has no wire row, so this would accuse the agent for every host "+
			"it honestly named, hardest on the sessions where we observed least.",
			d.DeclaredNotObserved)
	}
}

// TestDeclaredNotObserved_ExcludesInheritedRows: a host whose only wire rows
// belong to an earlier session was not observed for THIS session, so it belongs
// in this list.
func TestDeclaredNotObserved_ExcludesInheritedRows(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		wire.Observation{
			Observed:      true,
			WindowApplied: true,
			Inherited:     2,
			Hosts: []wire.Destination{
				{Host: "pypi.org", InheritedAttempts: 2, Inherited: true},
			},
		}, "", nil)

	if !has(d.DeclaredNotObserved, "pypi.org") {
		t.Errorf("declared_not_observed = %v, want pypi.org: its only rows are another "+
			"session's, so this session did not observe it", d.DeclaredNotObserved)
	}
}

// TestSSHHostsRenderAsNotObservableNeverAsAFinding pins the rule the schema's
// two separate lists exist to serve.
//
// The proxy sees CONNECT, and ssh is not CONNECT. An ssh host with no wire row
// is not a gap in the record; it is outside what this product can observe at
// all, and reporting it as either finding would be describing a limitation of
// the tool as a property of the session.
func TestSSHHostsRenderAsNotObservableNeverAsAFinding(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{{
		ToolUseID: "t1",
		ToolName:  "Bash",
		Hosts:     []string{"pypi.org"},
		SSHHosts:  []string{"github.com", "deploy.example.com"},
	}}}

	d := buildDestinations(run, observed(wire.Destination{Host: "pypi.org", Attempts: 1}), "", nil)

	for _, h := range []string{"github.com", "deploy.example.com"} {
		if has(d.DeclaredNotObserved, h) {
			t.Errorf("%s is under declared_not_observed; ssh is outside what a CONNECT "+
				"proxy can see, so that would report a limitation of the tool as a "+
				"fact about the session", h)
		}
		if has(d.WireOnly, h) {
			t.Errorf("%s is under wire_only", h)
		}
		if !has(d.NotObservable, h) {
			t.Errorf("not_observable = %v, want %s", d.NotObservable, h)
		}
	}
}

// TestNotObservableIsEmptyWithoutSSHHosts is the negative fixture: the line
// must not appear on a session that used no ssh host.
func TestNotObservableIsEmptyWithoutSSHHosts(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		observed(wire.Destination{Host: "pypi.org", Attempts: 1}), "", nil)
	if len(d.NotObservable) != 0 {
		t.Errorf("not_observable = %v, want empty", d.NotObservable)
	}
}

// TestExecutedNotAsDeclared_IsCountedWithoutAProxyStore is the case the first
// placement of the rendered line got wrong.
//
// The count compares two records the recorder wrote itself; the proxy has
// nothing to do with it. Computing or printing it only when the wire was
// readable would hide a rewritten call precisely on the sessions where the
// proxy was not running -- which are the sessions where the hooks are the only
// evidence there is.
func TestExecutedNotAsDeclared_IsCountedWithoutAProxyStore(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash",
			Shape: shape.Shape{Digest: "aaaa"},
		}},
		Executions: []store.Execution{{ToolUseID: "t1", ExecutedDigest: "bbbb"}},
	}

	d := buildDestinations(run, wire.Observation{Observed: false, Reason: wire.NotObservedNoStore}, "", nil)

	if d.ExecutedNotAsDeclared != 1 {
		t.Errorf("executed_not_as_declared = %d, want 1 even with no proxy store: the "+
			"comparison is between two hook records", d.ExecutedNotAsDeclared)
	}
}

// TestExecutedNotAsDeclared_UnknownDigestIsNotADifference covers the three
// non-comparable shapes. Each would inflate the count on ordinary sessions.
func TestExecutedNotAsDeclared_UnknownDigestIsNotADifference(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  *store.Run
	}{
		{
			name: "execution carried no tool_input",
			run: &store.Run{
				Declarations: []store.Declaration{{ToolUseID: "t1", Shape: shape.Shape{Digest: "aaaa"}}},
				Executions:   []store.Execution{{ToolUseID: "t1"}},
			},
		},
		{
			name: "execution has no declaration in this run",
			run: &store.Run{
				Executions: []store.Execution{{ToolUseID: "orphan", ExecutedDigest: "bbbb"}},
			},
		},
		{
			name: "declaration carries no digest",
			run: &store.Run{
				Declarations: []store.Declaration{{ToolUseID: "t1"}},
				Executions:   []store.Execution{{ToolUseID: "t1", ExecutedDigest: "bbbb"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := buildDestinations(tc.run, observed(), "", nil)
			if d.ExecutedNotAsDeclared != 0 {
				t.Errorf("executed_not_as_declared = %d, want 0: an incomparable pair is "+
					"not a difference", d.ExecutedNotAsDeclared)
			}
		})
	}
}

// TestInheritedHostIsNotAFinding keeps another session's traffic out of this
// session's accusation.
func TestInheritedHostIsNotAFinding(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		wire.Observation{
			Observed:      true,
			WindowApplied: true,
			Attempts:      1,
			DistinctHosts: 1,
			Inherited:     2,
			Hosts: []wire.Destination{
				{Host: "pypi.org", Attempts: 1},
				{Host: "someone-elses.example", InheritedAttempts: 2, Inherited: true},
			},
		}, "", nil)

	if has(d.WireOnly, "someone-elses.example") {
		t.Errorf("wire_only = %v; an inherited host was not reached by this session and "+
			"must not be reported as something it hid", d.WireOnly)
	}
	if d.Inherited != 2 {
		t.Errorf("inherited = %d, want 2 carried through for the coverage line", d.Inherited)
	}
}

// TestNovelty_UnavailableWithoutARecordedProject pins the degradation. A run
// whose coverage records carry no cwd has no project, and novelty must say
// unknown rather than render "none" — which would read as "this session
// reached nothing new" on a session where nothing was checked.
func TestNovelty_UnavailableWithoutARecordedProject(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		observed(wire.Destination{Host: "new.example", Attempts: 1}),
		t.TempDir(), nil)

	if d.Novelty.Available {
		t.Error("novelty available with no recorded cwd; the project is unknown")
	}
	if d.Novelty.Reason == "" {
		t.Error("no reason given for unavailable novelty")
	}
	if len(d.Novelty.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty", d.Novelty.Hosts)
	}
}

// TestNovelty_UsesTheRunsOwnCWDNotTheReadersCWD. A report run from a different
// directory must not key a past session to the reader's project — that would
// make novelty depend on where somebody happened to stand when they read it.
func TestNovelty_UsesTheRunsOwnCWDNotTheReadersCWD(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	run := &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1"}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: project, SessionID: "sess-1",
		}},
	}

	first := buildDestinations(run, observed(wire.Destination{Host: "a.example", Attempts: 1}), root, nil)
	if !first.Novelty.Available {
		t.Fatalf("novelty unavailable: %s", first.Novelty.Reason)
	}
	if !first.Novelty.Established {
		t.Error("the first session in a project should establish the baseline")
	}

	// A different session in the SAME project, later, reaching a new host.
	run2 := &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "t2", ToolName: "Bash", SessionID: "sess-2"}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms + 3600000, CWD: project, SessionID: "sess-2",
		}},
	}
	second := buildDestinations(run2, observed(
		wire.Destination{Host: "a.example", Attempts: 1},
		wire.Destination{Host: "b.example", Attempts: 1},
	), root, nil)

	if second.Novelty.Established {
		t.Error("the second session established the baseline again")
	}
	if len(second.Novelty.Hosts) != 1 || second.Novelty.Hosts[0] != "b.example" {
		t.Errorf("novelty hosts = %v, want [b.example]", second.Novelty.Hosts)
	}
}

// TestNovelty_ClientPlaneIsNeverNovel. Folding the client's own control plane
// into a project baseline would report api.anthropic.com as a new destination
// on the first session of every project — the novelty line's most obvious way
// to make itself worthless.
func TestNovelty_ClientPlaneIsNeverNovel(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	mk := func(id string, at int64) *store.Run {
		return &store.Run{
			Declarations: []store.Declaration{{ToolUseID: "t", ToolName: "Bash", SessionID: id}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
			}},
		}
	}
	// Establish, then a later session whose only new host is client plane.
	buildDestinations(mk("sess-1", t0ms), observed(wire.Destination{Host: "a.example", Attempts: 1}), root, nil)
	d := buildDestinations(mk("sess-2", t0ms+3600000), observed(
		wire.Destination{Host: "a.example", Attempts: 1},
		wire.Destination{Host: "api.anthropic.com", Attempts: 4},
		wire.Destination{Host: "127.0.0.1", Attempts: 2},
	), root, nil)

	if len(d.Novelty.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: the client plane and loopback are not "+
			"new destinations for a project", d.Novelty.Hosts)
	}
}

const t0ms = int64(1789000000000)

// TestProxyOnPath_ATrueVerdictCitesWhatWasMeasured pins the contract the note
// carries rather than its prose: whenever the verdict is true, the note says
// what was measured to make it true.
//
// Both true branches are checked because only one of them was wrong. The
// no-declared-match branch led with the negative and rendered as
// "true -- no declared host matched", which reads as a contradiction and
// invites a reader to distrust a verdict that is correct. Seen in the first
// full end-to-end run, where every destination was genuinely reached without
// a tool call naming it -- the most interesting thing this report can say,
// phrased as though it were a failure.
func TestProxyOnPath_ATrueVerdictCitesWhatWasMeasured(t *testing.T) {
	cases := []struct {
		name string
		d    Destinations
	}{
		{
			name: "a declared host was observed",
			d: buildDestinations(runWithDeclaredHosts("pypi.org"),
				observed(wire.Destination{Host: "pypi.org", Attempts: 2}), "", nil),
		},
		{
			name: "rows in the window, none of them declared",
			d: buildDestinations(runWithDeclaredHosts(),
				observed(wire.Destination{Host: "pypi.org", Attempts: 3}), "", nil),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.d.ProxyOnPath != ProxyOnPathTrue {
				t.Fatalf("premise: proxy_on_path = %q, want true", tc.d.ProxyOnPath)
			}
			if !strings.Contains(tc.d.ProxyNote, "measured:") {
				t.Errorf("note for a true verdict does not cite a measurement: %q",
					tc.d.ProxyNote)
			}
			if strings.HasPrefix(tc.d.ProxyNote, "no ") {
				t.Errorf("note for a true verdict opens with a negation, which reads as a "+
					"contradiction of the verdict: %q", tc.d.ProxyNote)
			}
		})
	}
}

// renderDestinations renders just the destinations section, for the two lines
// below whose wording is the whole point of them.
func renderDestinations(d Destinations) string {
	var b bytes.Buffer
	writeDestinations(&b, d)
	return b.String()
}

// TestInherited_SaysSoWhenEveryInheritedAttemptIsClientPlane.
//
// One inherited attempt appears on a COMPLETELY FRESH proxy store, every time:
// the client opens its own api.anthropic.com tunnel before the SessionStart
// hook has written the window's left edge. The exclusion is correct and the
// count is honest, but a reader who sees "1 attempt from outside this session's
// window" on a store that has never held anything else is being invited to look
// for a previous session that does not exist.
//
// The clause is added only when EVERY inherited attempt is client-plane. A
// genuinely inherited agent destination must keep the bare line, because that
// one does mean another session ran.
func TestInherited_SaysSoWhenEveryInheritedAttemptIsClientPlane(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts(),
		observed(
			wire.Destination{Host: "api.anthropic.com", InheritedAttempts: 1, Inherited: true},
			wire.Destination{Host: "pypi.org", Attempts: 2},
		), "", nil)

	if !d.InheritedAllClientPlane {
		t.Fatal("inherited_all_client_plane = false although the only inherited host is the client's own API")
	}
	out := renderDestinations(d)
	if !strings.Contains(out, "client-plane traffic before the session's first hook") {
		t.Errorf("the inherited line does not say the traffic is the client's own:\n%s", out)
	}
}

// TestInherited_StaysBareWhenAnAgentDestinationIsInherited is the other half.
// A tool-call destination carried in from another session is exactly the case
// where a reader SHOULD go looking, so the reassuring clause must not appear.
func TestInherited_StaysBareWhenAnAgentDestinationIsInherited(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts(),
		observed(
			wire.Destination{Host: "api.anthropic.com", InheritedAttempts: 1, Inherited: true},
			wire.Destination{Host: "internal.example", InheritedAttempts: 3, Inherited: true},
		), "", nil)

	if d.InheritedAllClientPlane {
		t.Error("inherited_all_client_plane = true although an agent destination was inherited")
	}
	out := renderDestinations(d)
	if strings.Contains(out, "client-plane traffic before the session's first hook") {
		t.Errorf("the reassuring clause appears although another session's agent traffic "+
			"was inherited:\n%s", out)
	}
}

// TestNovelty_EstablishedLineSaysTheClientPlaneIsExcluded.
//
// The line reports the baseline size and the destinations are listed beneath
// it, so "established (2 hosts)" above four destinations reads as a
// disagreement. The client plane is excluded from novelty on purpose -- a
// session should not be told the client's own control plane is a new
// destination for its project -- and the line now says so rather than leaving
// the reader to work out which two.
func TestNovelty_EstablishedLineSaysTheClientPlaneIsExcluded(t *testing.T) {
	var b bytes.Buffer
	writeNovelty(&b, Novelty{Available: true, Established: true, KnownHosts: 2, Hosts: []string{}})

	out := b.String()
	if !strings.Contains(out, "client plane excluded") {
		t.Errorf("the established line does not say the client plane is excluded from the "+
			"count:\n%s", out)
	}
}

// TestForget_ForgottenHostIsGoneFromEverySectionOfTheReport is the defect an
// Opus review of this branch found, and it undid the product's one privacy
// action.
//
// suppress() builds a new slice and assigns it to d.Hosts. Everything after it
// then read obs.Hosts -- the unsuppressed original -- so a host the user had
// asked to forget came back in two places. Worse than coming back: because
// forget --host also deletes the DECLARATION, the host was no longer in
// declaredHosts(run), so it reappeared under "reached but never named", which
// is this product's central accusation, aimed at a destination the user had
// explicitly asked it to drop. And novelHostCandidates fed it to
// baseline.Update, re-adding the entry forget had just cleared.
//
// The comment above suppress() says "dropped from the whole view BEFORE
// anything else looks at them", so the intent was right and only the reads
// were wrong. No test in the package passed anything but nil for the forgotten
// predicate, so nothing exercised it.
func TestForget_ForgottenHostIsGoneFromEverySectionOfTheReport(t *testing.T) {
	const gone = "acme-secret.internal"
	forgotten := func(h string) bool { return h == gone }

	// forget --host removes the declaration too, so the run no longer names it:
	// this is the state that made it read as a finding rather than as a host
	// that was merely accounted for.
	run := runWithDeclaredHosts("pypi.org")
	d := buildDestinations(run, observed(
		// NOT inherited. Every existing forget test wrote its proxy row after
		// the session window closed, so the row was inherited and skipped
		// before the code under test was reached.
		wire.Destination{Host: gone, Attempts: 3},
		wire.Destination{Host: "pypi.org", Attempts: 1},
	), t.TempDir(), forgotten)

	if d.Suppressed != 1 {
		t.Errorf("suppressed = %d, want 1", d.Suppressed)
	}
	if has(d.WireOnly, gone) {
		t.Error("a forgotten host is reported as \"reached but never named\" -- the " +
			"product's central accusation, aimed at the one destination the user asked " +
			"it to forget")
	}
	if has(d.ClientPlane, gone) {
		t.Error("a forgotten host is listed under the client plane")
	}
	for _, h := range d.Hosts {
		if h.Host == gone {
			t.Error("a forgotten host is still in the per-host list")
		}
	}
	// And it must not be handed to the baseline, which would re-add the entry
	// that forget --host just cleared.
	for _, h := range novelHostCandidates(d.Hosts, false) {
		if h == gone {
			t.Error("a forgotten host is offered to the novelty baseline, which re-adds " +
				"the entry forget --host removed; the next report then reports it as new")
		}
	}
	// The honest half: the host that was NOT forgotten still reports normally.
	if !has(d.WireOnly, "pypi.org") && len(d.WireOnly) == 0 {
		t.Log("pypi.org was declared, so its absence from wire_only is correct")
	}
}

// TestNovelty_UnreadableStoreSaysWhyRatherThanVanishing.
//
// buildDestinations returns early when the proxy store could not be read, and
// that early return sits ABOVE the line that assigns Novelty -- so the section
// came back as the zero value: Available false, Reason empty, Hosts nil. A
// degraded state with no reason at all, and in the text form the line did not
// print, which this package's own rule forbids: a section that vanishes when it
// has nothing to say cannot be told apart from one that was never built.
//
// This is the MOST COMMON case, not an edge: no --proxy-store and no
// ~/.altrace/observe/causal.db is what a user without the proxy running has.
// Families handles the same case correctly, which is what makes this an
// oversight rather than a decision.
func TestNovelty_UnreadableStoreSaysWhyRatherThanVanishing(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"),
		wire.Observation{Observed: false, Reason: wire.NotObservedNoStore},
		t.TempDir(), nil)

	if d.Novelty.Available {
		t.Error("novelty reports itself available with no store to compare against")
	}
	if d.Novelty.Reason == "" {
		t.Error("novelty is unavailable with no reason. \"no new hosts\" and \"we could " +
			"not tell\" are different answers, and an empty reason renders as neither.")
	}
	if d.Novelty.Hosts == nil {
		t.Error("novelty.hosts is nil rather than empty; every comparable list in this " +
			"package is an empty slice so a consumer never meets null")
	}

	// And the line must actually print.
	out := renderDestinations(d)
	if !strings.Contains(out, "new for this project") {
		t.Errorf("the novelty line does not appear at all when the store could not be "+
			"read, so a reader cannot tell it from a section that was never built:\n%s",
			out)
	}
}
