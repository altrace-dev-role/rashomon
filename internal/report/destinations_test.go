package report

import (
	"testing"

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
