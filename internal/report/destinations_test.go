package report

import (
	"bytes"
	"slices"
	"strings"
	"testing"

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

	d := buildDestinations(run, Novelty{}, nil)

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
			d := buildDestinations(tc.run, Novelty{}, nil)
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
	_, n := NoveltyQueryOf(runWithDeclaredHosts("pypi.org"), t.TempDir(), nil)
	if n == nil {
		t.Fatal("a run with no recorded cwd produced a baseline query; the project is unknown")
	}
	if n.Available {
		t.Error("novelty available with no recorded cwd; the project is unknown")
	}
	if n.Reason == "" {
		t.Error("no reason given for unavailable novelty")
	}
	if n.Hosts == nil || len(n.Hosts) != 0 {
		t.Errorf("novelty hosts = %#v, want empty", n.Hosts)
	}
}

// TestNovelty_TheQueryIsTheRunsOwnProjectAndHosts is the pure half of the
// two baseline tests in internal/report/gather: the question asked of the
// baseline is keyed by the cwd the RUN recorded, owned by its session from its
// start, and names its hosts less loopback and the client's own plane.
func TestNovelty_TheQueryIsTheRunsOwnProjectAndHosts(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t", ToolName: "Bash", SessionID: "sess-1",
			Hosts: []string{"a.example", "api.anthropic.com", "127.0.0.1"},
		}},
		Coverage: []store.Coverage{
			{Phase: store.PhaseEnd, RecordedAtMS: t0ms + 5, CWD: "/elsewhere", SessionID: "sess-1"},
			{Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: "/project", SessionID: "sess-1"},
		},
	}
	q, n := NoveltyQueryOf(run, "/root", nil)
	if n != nil {
		t.Fatalf("no query: %s", n.Reason)
	}
	if q.CWD != "/project" || q.SessionID != "sess-1" || q.Start.UnixMilli() != t0ms {
		t.Errorf("query = %+v, want the start record's cwd and instant and the run's session", q)
	}
	if strings.Join(q.Hosts, ",") != "a.example" {
		t.Errorf("query hosts = %v, want [a.example]: loopback and the client plane are not asked about", q.Hosts)
	}
}

// TestNovelty_AFailureIsUnknownWithTheReadStepsReason: a baseline that
// could not answer is unavailable novelty carrying the reason the read step
// named for it (gather's baselineErrorReason keeps the error's text out), and
// a clean answer is available with its hosts never null.
func TestNovelty_AFailureIsUnknownWithTheReadStepsReason(t *testing.T) {
	const reason = "the project baseline could not be read or written"
	n := AnsweredNovelty(true, 3, 0, []string{"x.example"}, reason)
	if n.Available || n.Reason != reason || n.Hosts == nil || len(n.Hosts) != 0 {
		t.Errorf("novelty = %+v, want unavailable, the read step's reason, and empty hosts", n)
	}
	n = AnsweredNovelty(false, 4, 0, nil, "")
	if !n.Available || n.Established || n.KnownHosts != 4 || n.Hosts == nil || len(n.Hosts) != 0 {
		t.Errorf("novelty = %+v, want available, not established, 4 known and empty hosts", n)
	}
}

// TestNovelty_TheMCPTransportIsTheAgentsOnAnMCPSession: the MCP transport is
// the one client-plane host that can be the agent's, and clientPlaneHost
// decides it from the session's own mcp__* declarations. NoveltyQueryOf has
// to hand that answer to the novelty candidates it asks the baseline about,
// or a session that made MCP calls and named the transport is told nothing
// about it, while the sandbox section of the same session counts it as the
// agent's.
func TestNovelty_TheMCPTransportIsTheAgentsOnAnMCPSession(t *testing.T) {
	for _, mcp := range []bool{false, true} {
		run := &store.Run{
			Declarations: []store.Declaration{{
				ToolUseID: "t", ToolName: "Bash", SessionID: "sess-1",
				Hosts: []string{"a.example", mcpProxyHost},
			}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: "/project", SessionID: "sess-1",
			}},
		}
		if mcp {
			run.Declarations = append(run.Declarations, store.Declaration{
				ToolUseID: "m", ToolName: "mcp__docs__search", SessionID: "sess-1",
			})
		}
		q, unavailable := NoveltyQueryOf(run, "/root", nil)
		if unavailable != nil {
			t.Fatalf("mcp session %v: no query: %s", mcp, unavailable.Reason)
		}
		if got := slices.Contains(q.Hosts, mcpProxyHost); got != mcp {
			t.Errorf("mcp session %v: %s asked of the baseline = %v, want %v (query hosts %v)",
				mcp, mcpProxyHost, got, mcp, q.Hosts)
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
// NoveltyQueryOf returns early -- Available false, a Reason, Hosts the empty
// slice -- for any of several reasons a baseline cannot be consulted, and the
// text form must print the line in every one of them: a section that
// vanishes when it has nothing to say cannot be told apart from one that was
// never built. This fixture reaches it through an empty store root (no
// baseline directory to read or write), which is distinct from the
// no-recorded-cwd case above and exercises NoveltyQueryOf's other guard.
func TestNovelty_UnreadableStoreSaysWhyRatherThanVanishing(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1", Hosts: []string{"pypi.org"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: t.TempDir(), SessionID: "sess-1",
		}},
	}
	_, n := NoveltyQueryOf(run, "", nil)
	if n == nil {
		t.Fatal("a query was produced with no store root to locate a baseline in")
	}
	d := buildDestinations(run, *n, nil)

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
// NoveltyQueryOf chooses the candidates, so an ssh host must be among the
// hosts it asks the baseline about.
func TestNovelty_SSHHostsAreCandidatesToo(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t", ToolName: "Bash", SessionID: "sess-1",
			SSHHosts: []string{"github.com", "deploy.example"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: "/project", SessionID: "sess-1",
		}},
	}
	q, unavailable := NoveltyQueryOf(run, "/root", nil)
	if unavailable != nil {
		t.Fatalf("no query: %s", unavailable.Reason)
	}
	if !slices.Contains(q.Hosts, "deploy.example") {
		t.Errorf("query hosts = %v, want deploy.example: an ssh host named in this "+
			"project must be asked of the baseline as a novelty candidate", q.Hosts)
	}
}
