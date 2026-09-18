package report

import (
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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		observed(
			wire.Destination{Host: "pypi.org", Attempts: 1},
			wire.Destination{Host: "files.pythonhosted.org", Attempts: 1},
		),
	)

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org", "files.pythonhosted.org"),
		observed(
			wire.Destination{Host: "pypi.org", Attempts: 1},
			wire.Destination{Host: "files.pythonhosted.org", Attempts: 2},
		),
	)

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
	d := buildDestinations(
		runWithDeclaredHosts(),
		observed(
			wire.Destination{Host: "127.0.0.1", Attempts: 3},
			wire.Destination{Host: "localhost", Attempts: 1},
			wire.Destination{Host: "[::1]", Attempts: 1},
		),
	)
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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		observed(
			wire.Destination{Host: "api.anthropic.com", Attempts: 4},
			wire.Destination{Host: "pypi.org", Attempts: 1},
		),
	)

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		observed(
			wire.Destination{Host: "mcp-proxy.anthropic.com", Attempts: 6},
			wire.Destination{Host: "pypi.org", Attempts: 1},
		),
	)

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
	))

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		wire.Observation{Observed: false, Reason: wire.NotObservedNoStore},
	)

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		wire.Observation{Observed: true, WindowApplied: true},
	)

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
	d := buildDestinations(
		runWithDeclaredHosts("declared-but-never-reached.example"),
		observed(wire.Destination{Host: "surprise.example", Attempts: 1}),
	)

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

	d := buildDestinations(run, observed(wire.Destination{Host: "github.com", Attempts: 1}))

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org", "never-reached.example"),
		observed(wire.Destination{Host: "pypi.org", Attempts: 1}),
	)

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		observed(wire.Destination{Host: "pypi.org", Attempts: 1}),
	)
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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org", "api.github.com"),
		wire.Observation{Observed: false, Reason: wire.NotObservedNoStore},
	)

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		wire.Observation{
			Observed:      true,
			WindowApplied: true,
			Inherited:     2,
			Hosts: []wire.Destination{
				{Host: "pypi.org", InheritedAttempts: 2, Inherited: true},
			},
		},
	)

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

	d := buildDestinations(run, observed(wire.Destination{Host: "pypi.org", Attempts: 1}))

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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
		observed(wire.Destination{Host: "pypi.org", Attempts: 1}),
	)
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

	d := buildDestinations(run, wire.Observation{Observed: false, Reason: wire.NotObservedNoStore})

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
			d := buildDestinations(tc.run, observed())
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
	d := buildDestinations(
		runWithDeclaredHosts("pypi.org"),
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
		},
	)

	if has(d.WireOnly, "someone-elses.example") {
		t.Errorf("wire_only = %v; an inherited host was not reached by this session and "+
			"must not be reported as something it hid", d.WireOnly)
	}
	if d.Inherited != 2 {
		t.Errorf("inherited = %d, want 2 carried through for the coverage line", d.Inherited)
	}
}
