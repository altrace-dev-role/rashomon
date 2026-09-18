package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// Destinations is what the wire saw, set against what the session declared.
//
// The comparison is the product, not the inventory. A list of hosts is
// something several tools already print; the line that cannot be produced from
// a transcript or a hook log is "this host was reached and no tool call ever
// named it", and that line exists only because two independent records are
// being reconciled here.
type Destinations struct {
	// Observed is false when the proxy's store could not be read, and Reason
	// says which of the fixed reasons applied. The report then prints
	// "destinations: not observed (<reason>)" rather than an empty list: an
	// empty list is the answer to "where did it go" and this is the answer to
	// "were we looking".
	Observed bool   `json:"observed"`
	Reason   string `json:"reason,omitempty"`

	Hosts         []wire.Destination `json:"hosts"`
	Attempts      int                `json:"attempts"`
	DistinctHosts int                `json:"distinct_hosts"`
	Inherited     int                `json:"inherited"`

	// WindowApplied is false when the proxy's timestamps could not be parsed,
	// in which case rows from other sessions may be included and the report has
	// to say so rather than imply a window it did not enforce.
	WindowApplied bool `json:"window_applied"`

	// WireOnly is the finding: hosts the proxy saw that no declaration named.
	// Loopback and the client's own control-plane hosts are excluded, because
	// neither is the agent going somewhere nobody wrote down.
	WireOnly []string `json:"wire_only"`

	// ClientPlane is rendered separately and never as a finding. Claude Code's
	// own model traffic transits the same proxy, and an agent request to
	// api.anthropic.com is indistinguishable from the client's, so calling
	// either one a hidden destination would be a false accusation in one
	// direction and a blind spot in the other.
	ClientPlane []string `json:"client_plane"`

	// DeclaredNotObserved is the comparison's other direction: hosts a tool
	// call named that no wire row inside this window shows.
	//
	// It is a WEAKER claim than WireOnly by nature, and the rendering says so.
	// A declared host with no row may have been a call the user denied, a call
	// that failed before it connected, a response served from a cache, or a
	// host the proxy did not see. The report names the hosts and not the cause.
	//
	// It is EMPTY whenever the store could not be read. Without a store every
	// declared host trivially has no row, so listing them would turn "we were
	// not watching" into a finding against the agent for every host it honestly
	// named -- firing hardest on exactly the sessions where the tool observed
	// least.
	DeclaredNotObserved []string `json:"declared_not_observed"`

	// NotObservable are declared destinations outside what a CONNECT proxy can
	// see at all: ssh and git+ssh hosts.
	//
	// They are neither finding: an ssh host with no wire row is not a gap in
	// the record, it is a limitation of the instrument, and reporting a
	// limitation of the tool as a property of the session is the error this
	// field exists to prevent.
	NotObservable []string `json:"not_observable"`

	// ExecutedNotAsDeclared counts calls whose executed input digest differs
	// from the digest of the input they were declared with.
	//
	// A COUNT and not a diff. Neither input is stored, so the report can say
	// that a call ran differently from how it was asked for and cannot say how
	// -- which is the honest limit of a digest, and the reason the number is
	// rendered plainly as "0" on the overwhelming majority of sessions rather
	// than being hidden when it is zero. A line that only ever appeared when
	// non-zero would give a reader no way to know it was being checked.
	//
	// Only comparable pairs count: an execution with no digest (a payload that
	// carried no tool_input) is unknown, and unknown is not a difference.
	ExecutedNotAsDeclared int `json:"executed_not_as_declared"`

	// ProxyOnPath answers "was the proxy actually observing this session":
	// true, false, or unknown. Derived from the join rather than from a probe,
	// so it is a statement about records that exist rather than about
	// configuration that was read.
	ProxyOnPath string `json:"proxy_on_path"`
	ProxyNote   string `json:"proxy_note"`
}

// Proxy-on-path verdicts. Three values, not two: "we could not tell" is a real
// answer and collapsing it into false would print "the proxy was not on the
// path" for a session where the store simply could not be read.
const (
	ProxyOnPathTrue    = "true"
	ProxyOnPathFalse   = "false"
	ProxyOnPathUnknown = "unknown"
)

// loopbackHosts never count as a destination the agent reached. They are
// excluded at report time rather than in the canonicaliser, so the wire layer
// can still see them and this layer excludes them knowingly.
var loopbackHosts = map[string]bool{
	"localhost": true, "127.0.0.1": true, "0.0.0.0": true, "[::1]": true, "::1": true,
}

// clientPlaneHosts are the hosts the client contacts on its own behalf,
// regardless of the agent's tool calls.
//
// A fixed list, and it grows only by a RECORDED DECISION, never by an agent's
// judgement in the moment. That rule is the important part: a host wrongly
// placed here is a real finding permanently suppressed, which is the one error
// this report must not make, and the cost of leaving a client host out is only
// that it renders as a finding until somebody rules on it.
//
// mcp-proxy.anthropic.com was added after the first real session reported six
// attempts of it as "reached but never named" -- true of the bytes, false about
// the agent. See mcpProxyHost for the case where it belongs to the agent after
// all.
var clientPlaneHosts = map[string]bool{
	"api.anthropic.com":       true,
	"statsig.anthropic.com":   true,
	"statsig.com":             true,
	"sentry.io":               true,
	"mcp-proxy.anthropic.com": true,
}

// mcpProxyHost is the one client-plane host that can belong to the agent.
//
// It is the transport an mcp__* tool call travels over. On a session that made
// such a call, filing its traffic under the client plane would bury the only
// destination an MCP call can be observed at; on a session that made none, the
// client contacted it anyway and calling it a finding would accuse the agent of
// traffic it did not cause. Which of the two is true is decided by the
// declarations, not by the host.
const mcpProxyHost = "mcp-proxy.anthropic.com"

// mcpTool is the prefix every MCP tool name carries.
const mcpTool = "mcp__"

// buildDestinations reconciles the proxy's observation against the session's
// declarations.
func buildDestinations(run *store.Run, obs wire.Observation) Destinations {
	d := Destinations{
		Observed:            obs.Observed,
		Reason:              obs.Reason,
		Hosts:               obs.Hosts,
		Attempts:            obs.Attempts,
		DistinctHosts:       obs.DistinctHosts,
		Inherited:           obs.Inherited,
		WindowApplied:       obs.WindowApplied,
		WireOnly:            []string{},
		ClientPlane:         []string{},
		DeclaredNotObserved: []string{},
		NotObservable:       sshDeclared(run),
		// Computed whether or not the proxy store was read: it compares two
		// records the recorder wrote itself and has nothing to do with the
		// wire.
		ExecutedNotAsDeclared: executedNotAsDeclared(run),
		ProxyOnPath:           ProxyOnPathUnknown,
	}
	if d.Hosts == nil {
		d.Hosts = []wire.Destination{}
	}
	if !obs.Observed {
		// DeclaredNotObserved stays empty. See its field comment: without a
		// store every declared host trivially has no row, and listing them
		// would accuse the agent for every host it honestly named. The
		// NotObservable list is still correct, because it is a property of the
		// declarations rather than of the observation.
		d.ProxyNote = "the proxy's store could not be read, so whether it was on the path is unknown"
		return d
	}

	declared := declaredHosts(run)
	// An mcp__* declaration attributes the MCP transport to the agent's work.
	// Computed once: it is a property of the session, not of a host.
	mcpAttributed := madeMCPCall(run)

	var matched int
	for _, h := range obs.Hosts {
		if h.Inherited {
			continue
		}
		switch {
		case loopbackHosts[h.Host]:
			continue
		case h.Host == mcpProxyHost && mcpAttributed:
			// Accounted for by the mcp__* declarations, so neither a finding
			// nor client-plane traffic. It stays in the per-host list; only
			// the section it is filed under changes.
			matched++
			continue
		case clientPlaneHosts[h.Host]:
			d.ClientPlane = append(d.ClientPlane, h.Host)
			// A client-plane host that WAS declared still counts as evidence
			// the proxy was on the path: the agent named it and the wire saw
			// it, whatever else also reaches that host.
			if declared[h.Host] {
				matched++
			}
			continue
		}
		if declared[h.Host] {
			matched++
			continue
		}
		d.WireOnly = append(d.WireOnly, h.Host)
	}
	// The other direction, computed only now that the observation is known to
	// be real. A host counts as observed only through a NON-inherited row: its
	// only rows belonging to an earlier session means this session did not
	// observe it.
	seen := map[string]bool{}
	for _, h := range obs.Hosts {
		if !h.Inherited && h.Attempts > 0 {
			seen[h.Host] = true
		}
	}
	for h := range declared {
		if !seen[h] {
			d.DeclaredNotObserved = append(d.DeclaredNotObserved, h)
		}
	}

	sort.Strings(d.WireOnly)
	sort.Strings(d.ClientPlane)
	sort.Strings(d.DeclaredNotObserved)

	// Derived from the join, in three cases, because each says something
	// different and one boolean cannot carry them.
	switch {
	case matched > 0:
		d.ProxyOnPath = ProxyOnPathTrue
		d.ProxyNote = pluralHosts("measured: %d declared host%s observed on the wire", matched)
	case obs.Attempts > 0:
		// Rows exist in the window, so the proxy was observing something, but
		// nothing it saw was named by a tool call. That is still "on the path"
		// -- and it is the most interesting version of it.
		d.ProxyOnPath = ProxyOnPathTrue
		d.ProxyNote = "no declared host matched; every observed destination was reached without a tool call naming it"
	default:
		d.ProxyOnPath = ProxyOnPathUnknown
		d.ProxyNote = "the store was readable and held no rows inside this session's window, so whether the proxy was on the path is unknown"
	}
	return d
}

// declaredHosts is the set of wire-observable hostnames the session named.
//
// ssh_hosts are deliberately NOT included. They are unobservable by this
// proxy, so folding them in would let an ssh host suppress a wire-only finding
// for the same name reached over https -- and they belong under coverage as
// "not observable", never in this comparison.
// executedNotAsDeclared counts calls that ran with a different input from the
// one they were declared with, joined on tool_use_id.
//
// Three cases are deliberately NOT counted, and each would inflate the number
// on ordinary sessions:
//
//   - an execution with no digest, because the payload carried no tool_input.
//     Unknown is not a difference.
//   - an execution whose declaration is absent from this run. There is nothing
//     to compare it against, and comparing it against nothing would make every
//     spilled or evicted declaration look like a rewrite.
//   - a declaration with no shape digest at all, which a schema 1 record can
//     be.
func executedNotAsDeclared(run *store.Run) int {
	// Pre-sized from the run's own record count, which the store's size cap
	// bounds; the keys are tool_use_ids read back from disk, not values a
	// caller supplies, so there is no unbounded axis here.
	declared := make(map[string]string, len(run.Declarations))
	for _, d := range run.Declarations {
		if d.Shape.Digest != "" {
			declared[d.ToolUseID] = d.Shape.Digest
		}
	}
	var n int
	for _, x := range run.Executions {
		if x.ExecutedDigest == "" {
			continue
		}
		want, ok := declared[x.ToolUseID]
		if !ok {
			continue
		}
		if x.ExecutedDigest != want {
			n++
		}
	}
	return n
}

// sshDeclared is the sorted, de-duplicated set of ssh hosts the session named.
//
// Always returns a non-nil slice so the JSON carries an empty list rather than
// null: "no ssh host was named" is a fact, and null would read as "we did not
// look".
func sshDeclared(run *store.Run) []string {
	set := map[string]bool{}
	for _, d := range run.Declarations {
		for _, h := range d.SSHHosts {
			set[h] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// madeMCPCall reports whether the session declared any MCP tool call.
//
// The declaration is the evidence, not the host: an MCP call names no hostname
// of its own, so the transport is the only place it can be observed, and this
// is what connects the two.
func madeMCPCall(run *store.Run) bool {
	for _, d := range run.Declarations {
		if strings.HasPrefix(d.ToolName, mcpTool) {
			return true
		}
	}
	return false
}

func declaredHosts(run *store.Run) map[string]bool {
	out := map[string]bool{}
	for _, decl := range run.Declarations {
		for _, h := range decl.Hosts {
			out[h] = true
		}
	}
	return out
}

func pluralHosts(format string, n int) string {
	s := "s"
	if n == 1 {
		s = ""
	}
	return fmt.Sprintf(format, n, s)
}

// window derives the watched interval from the run's own coverage records.
//
// The run's account of itself is the only honest source: report must never
// re-read today's configuration to judge a past run, because that would let a
// later detach retroactively invalidate every session already captured. An
// absent start record leaves the window open at that end, which the wire layer
// reports as "window not applied" rather than silently treating everything as
// in-window.
//
// RunID is left empty on purpose. The proxy's run_id is the agent's own
// identifier and shares no namespace with a Claude Code session id, so
// matching on it would exclude every row. The window is the join.
func window(run *store.Run) wire.Window {
	var w wire.Window
	for _, c := range run.Coverage {
		at := time.UnixMilli(c.RecordedAtMS)
		switch c.Phase {
		case store.PhaseStart:
			if w.Start.IsZero() || at.Before(w.Start) {
				w.Start = at
			}
		case store.PhaseEnd:
			if at.After(w.End) {
				w.End = at
			}
		}
	}
	// A session with no start record still has records; anchor on the earliest
	// one rather than leaving the window unbounded, which would pull in every
	// row the store has ever held.
	if w.Start.IsZero() {
		for _, c := range run.Coverage {
			at := time.UnixMilli(c.RecordedAtMS)
			if w.Start.IsZero() || at.Before(w.Start) {
				w.Start = at
			}
		}
	}
	return w
}
