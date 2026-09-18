package report

import (
	"fmt"
	"sort"
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

// clientPlaneHosts are the client's own control-plane destinations.
//
// A fixed list, and deliberately short. Guessing at this set is worse than
// under-covering it: a host wrongly placed here is a real finding suppressed,
// which is the one error this report must not make.
var clientPlaneHosts = map[string]bool{
	"api.anthropic.com":     true,
	"statsig.anthropic.com": true,
	"statsig.com":           true,
	"sentry.io":             true,
}

// buildDestinations reconciles the proxy's observation against the session's
// declarations.
func buildDestinations(run *store.Run, obs wire.Observation) Destinations {
	d := Destinations{
		Observed:      obs.Observed,
		Reason:        obs.Reason,
		Hosts:         obs.Hosts,
		Attempts:      obs.Attempts,
		DistinctHosts: obs.DistinctHosts,
		Inherited:     obs.Inherited,
		WindowApplied: obs.WindowApplied,
		WireOnly:      []string{},
		ClientPlane:   []string{},
		ProxyOnPath:   ProxyOnPathUnknown,
	}
	if d.Hosts == nil {
		d.Hosts = []wire.Destination{}
	}
	if !obs.Observed {
		d.ProxyNote = "the proxy's store could not be read, so whether it was on the path is unknown"
		return d
	}

	declared := declaredHosts(run)

	var matched int
	for _, h := range obs.Hosts {
		if h.Inherited {
			continue
		}
		switch {
		case loopbackHosts[h.Host]:
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
	sort.Strings(d.WireOnly)
	sort.Strings(d.ClientPlane)

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
