package report

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// runWithDeclaredHosts builds a run whose one declaration names the given
// hosts.
func runWithDeclaredHosts(hosts ...string) *store.Run {
	return &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "toolu_1",
			ToolName:  "Bash",
			Hosts:     hosts,
		}},
	}
}

// TestExecutedNotAsDeclared_IsCountedWithoutAProxyStore pins that the count is
// a comparison between two hook records and needs nothing else to produce it.
// There is no longer a second way to compute it -- the kept name says what
// was once worth distinguishing, back when a proxy store existed to vary.
func TestExecutedNotAsDeclared_IsCountedWithoutAProxyStore(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash",
			Shape: shape.Shape{Digest: "aaaa"},
		}},
		Executions: []store.Execution{{ToolUseID: "t1", ExecutedDigest: "bbbb"}},
	}

	d := buildDestinations(run, "", nil)

	if d.ExecutedNotAsDeclared != 1 {
		t.Errorf("executed_not_as_declared = %d, want 1: the comparison is between two "+
			"hook records, and nothing else can suppress or produce it", d.ExecutedNotAsDeclared)
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
			d := buildDestinations(tc.run, "", nil)
			if d.ExecutedNotAsDeclared != 0 {
				t.Errorf("executed_not_as_declared = %d, want 0: an incomparable pair is "+
					"not a difference", d.ExecutedNotAsDeclared)
			}
		})
	}
}

// renderDestinations renders just the destinations section.
func renderDestinations(d Destinations) string {
	var b bytes.Buffer
	writeDestinations(&b, d)
	return b.String()
}

const t0ms = int64(1789000000000)

// TestNovelty_UnavailableWithoutARecordedProject pins the degradation. A run
// whose coverage records carry no cwd has no project, and novelty must say
// unknown rather than render "none" — which would read as "this session
// reached nothing new" on a session where nothing was checked.
func TestNovelty_UnavailableWithoutARecordedProject(t *testing.T) {
	d := buildDestinations(runWithDeclaredHosts("pypi.org"), t.TempDir(), nil)

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
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1", Hosts: []string{"a.example"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: project, SessionID: "sess-1",
		}},
	}

	first := buildDestinations(run, root, nil)
	if !first.Novelty.Available {
		t.Fatalf("novelty unavailable: %s", first.Novelty.Reason)
	}
	if !first.Novelty.Established {
		t.Error("the first session in a project should establish the baseline")
	}

	// A different session in the SAME project, later, naming a new host.
	run2 := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t2", ToolName: "Bash", SessionID: "sess-2",
			Hosts: []string{"a.example", "b.example"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms + 3600000, CWD: project, SessionID: "sess-2",
		}},
	}
	second := buildDestinations(run2, root, nil)

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
	mk := func(id string, at int64, hosts ...string) *store.Run {
		return &store.Run{
			Declarations: []store.Declaration{{
				ToolUseID: "t", ToolName: "Bash", SessionID: id, Hosts: hosts,
			}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
			}},
		}
	}
	// Establish, then a later session whose only new host is client plane.
	buildDestinations(mk("sess-1", t0ms, "a.example"), root, nil)
	d := buildDestinations(mk("sess-2", t0ms+3600000, "a.example", "api.anthropic.com", "127.0.0.1"), root, nil)

	if len(d.Novelty.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: the client plane and loopback are not "+
			"new destinations for a project", d.Novelty.Hosts)
	}
}

// TestNovelty_TheMCPTransportIsTheAgentsOnAnMCPSession: the MCP transport is
// the one client-plane host that can be the agent's, and clientPlaneHost
// decides it from the session's own mcp__* declarations. buildDestinations has
// to hand that answer to the novelty candidates, or a session that made MCP
// calls and named the transport is told nothing about it, while the sandbox
// section of the same session counts it as the agent's.
func TestNovelty_TheMCPTransportIsTheAgentsOnAnMCPSession(t *testing.T) {
	for _, mcp := range []bool{false, true} {
		project, root := t.TempDir(), t.TempDir()
		mk := func(id string, at int64, hosts ...string) *store.Run {
			run := &store.Run{
				Declarations: []store.Declaration{{
					ToolUseID: "t", ToolName: "Bash", SessionID: id, Hosts: hosts,
				}},
				Coverage: []store.Coverage{{
					Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
				}},
			}
			if mcp {
				run.Declarations = append(run.Declarations, store.Declaration{
					ToolUseID: "m", ToolName: "mcp__docs__search", SessionID: id,
				})
			}
			return run
		}
		buildDestinations(mk("sess-1", t0ms, "a.example"), root, nil)
		d := buildDestinations(mk("sess-2", t0ms+3600000, "a.example", mcpProxyHost), root, nil)
		if got := slices.Contains(d.Novelty.Hosts, mcpProxyHost); got != mcp {
			t.Errorf("mcp session %v: %s novel = %v, want %v (novelty hosts %v)",
				mcp, mcpProxyHost, got, mcp, d.Novelty.Hosts)
		}
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

// TestNovelty_UnreadableStoreSaysWhyRatherThanVanishing.
//
// buildNovelty returns early -- Available false, a Reason, Hosts the empty
// slice -- for any of several reasons a baseline cannot be consulted, and the
// text form must print the line in every one of them: a section that
// vanishes when it has nothing to say cannot be told apart from one that was
// never built. This fixture reaches it through an empty store root (no
// baseline directory to read or write), which is distinct from the
// no-recorded-cwd case above and exercises buildNovelty's other guard.
func TestNovelty_UnreadableStoreSaysWhyRatherThanVanishing(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1", Hosts: []string{"pypi.org"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: t.TempDir(), SessionID: "sess-1",
		}},
	}
	d := buildDestinations(run, "", nil)

	if d.Novelty.Available {
		t.Error("novelty reports itself available with no store root to locate a baseline in")
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
	if !strings.Contains(out, "not yet in this project's baseline") {
		t.Errorf("the novelty line does not appear at all when the baseline cannot be "+
			"located, so a reader cannot tell it from a section that was never built:\n%s",
			out)
	}
}

// TestBaselineErrorReason_DistinctCausesDistinctMessages: a locked baseline,
// a session naming more hosts than a baseline can hold, and a corrupt or
// unreadable file are three different situations, and a reader who sees "the
// project baseline could not be read or written" for all three cannot tell a
// transient contention from a reason to go look at the file. The first two
// have their own named errors (baseline.ErrTooManyHosts, store.ErrLockTimeout)
// precisely so a caller can tell them apart; only the generic message is
// deliberately silent about which file.
func TestBaselineErrorReason_DistinctCausesDistinctMessages(t *testing.T) {
	tooMany := baselineErrorReason(baseline.ErrTooManyHosts)
	locked := baselineErrorReason(store.ErrLockTimeout)
	generic := baselineErrorReason(errors.New("baseline: unrecognised file version"))
	for _, pair := range [][2]string{{tooMany, locked}, {tooMany, generic}, {locked, generic}} {
		if pair[0] == pair[1] {
			t.Errorf("two distinct baseline errors produced the same reason %q", pair[0])
		}
	}
	if generic == "" || strings.Contains(generic, "unrecognised file version") {
		t.Errorf("generic reason %q: must say something, and must not repeat the error text, "+
			"which can carry the baseline's path", generic)
	}
}

// TestNovelty_TooManyHostsSaysSoNotTheGenericMessage: the integration path,
// not just the mapping function above -- a session that names more hosts
// than a baseline holds must reach the reader as something more specific
// than the generic "could not be read or written", which a lock timeout and
// a corrupt file also produce.
func TestNovelty_TooManyHostsSaysSoNotTheGenericMessage(t *testing.T) {
	hosts := make([]string, 5000)
	for i := range hosts {
		hosts[i] = fmt.Sprintf("h%d.example", i)
	}
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1", Hosts: hosts,
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: t.TempDir(), SessionID: "sess-1",
		}},
	}
	d := buildDestinations(run, t.TempDir(), nil)
	if d.Novelty.Available {
		t.Fatal("novelty reports itself available for a session past the baseline's host cap")
	}
	if d.Novelty.Reason == "the project baseline could not be read or written" {
		t.Errorf("reason %q: a session naming too many hosts reads identically to a locked "+
			"or corrupt baseline", d.Novelty.Reason)
	}
}

// TestClientPlane_EveryViewAppliesTheOneRule: whether a host is the client's
// own traffic is asked by two readers -- the novelty candidates and the
// sandbox section -- and one of them used to restate the rule instead of
// calling it. They agreed, but a change to clientPlaneHost reached the
// sandbox section alone, and nothing compared the views. Every listed host
// and one that is not, on a session with mcp__* calls and one without, must
// be filed the same way by both.
func TestClientPlane_EveryViewAppliesTheOneRule(t *testing.T) {
	hosts := []string{"pypi.org"}
	for h := range clientPlaneHosts {
		hosts = append(hosts, h)
	}
	for _, mcp := range []bool{false, true} {
		for _, h := range hosts {
			want := clientPlaneHost(h, mcp)
			wire, _ := novelHostCandidates([]string{h}, nil, mcp)
			if got := !slices.Contains(wire, h); got != want {
				t.Errorf("mcp session %v, %s: left out of the novelty candidates %v, the rule says %v", mcp, h, got, want)
			}
			trail := trailWith(nono.Event{Host: h, Port: 443, Decision: nono.DecisionAllow, Mode: "connect"})
			if got := slices.Contains(buildNono(trail, true, mcp, nil, nil).ClientPlaneAllowed, h); got != want {
				t.Errorf("mcp session %v, %s: sandbox client_plane_allowed %v, the rule says %v", mcp, h, got, want)
			}
		}
	}
}

// TestNovelty_SSHHostsAreCandidatesToo: a session that only reaches a new
// host over ssh -- a git deploy target, say -- must be told about it the same
// way an HTTPS one would be. The proxy-era exclusion of ssh from novelty
// existed because the proxy could not see ssh at all; nothing here still
// observes anything, so there is no longer a reason to leave ssh out.
func TestNovelty_SSHHostsAreCandidatesToo(t *testing.T) {
	project, root := t.TempDir(), t.TempDir()
	mk := func(id string, at int64, sshHosts ...string) *store.Run {
		return &store.Run{
			Declarations: []store.Declaration{{
				ToolUseID: "t", ToolName: "Bash", SessionID: id, SSHHosts: sshHosts,
			}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
			}},
		}
	}
	buildDestinations(mk("sess-1", t0ms, "github.com"), root, nil) // ubiquitous; establishes
	d := buildDestinations(mk("sess-2", t0ms+3600000, "github.com", "deploy.example"), root, nil)

	if !slices.Contains(d.Novelty.Hosts, "deploy.example") {
		t.Errorf("novelty hosts = %v, want deploy.example: an ssh host named for the "+
			"first time in this project must count as a novelty candidate", d.Novelty.Hosts)
	}
}
