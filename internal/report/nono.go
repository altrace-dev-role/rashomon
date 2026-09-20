package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/nono"
)

// Nono is what the sandbox saw, beside what the proxy saw and what the agent
// declared.
//
// A FOURTH EVIDENCE SOURCE, and the reason to carry it is that it DISAGREES.
// Three sources that always agree add confidence and nothing else; a fourth
// that sees a different slice of the same session is the one that can show
// what the others structurally cannot.
type Nono struct {
	// Configured says a trail was ASKED FOR. It is separate from Observed, and
	// the separation is the whole of what H-28 caught: most sessions run no
	// sandbox at all, so rendering "not observed" for an optional integration
	// nobody requested prints a degradation marker on a perfectly healthy run.
	// A reader who meets that word on a clean session learns to ignore it, and
	// then misses it when it is true.
	//
	// Not configured -> the section is silent. Configured and unreadable -> the
	// section says so, loudly, because then something that was asked for did
	// not happen.
	Configured bool `json:"configured"`
	// Observed is false when no trail was configured or it could not be read,
	// with Reason saying which. Never an empty list standing in for that: an
	// empty list is the answer to a different question.
	Observed bool   `json:"observed"`
	Reason   string `json:"reason,omitempty"`
	// Allowed and Denied are the distinct hosts, kept apart because a denied
	// host is one the agent TRIED to reach and did not.
	Allowed []string `json:"allowed"`
	Denied  []string `json:"denied"`
	// SawWhatTheProxyDidNot is the interesting column. A host here reached the
	// network with the sandbox watching and left no row on the proxy's wire.
	//
	// It is NOT automatically a recording failure, and the report must not
	// present it as one: rashomon exports no HTTP_PROXY, so plain HTTP is
	// outside what its proxy can see, while nono's reverse-proxy path sees it.
	// PlainHTTP names the subset explained by exactly that.
	SawWhatTheProxyDidNot []string `json:"saw_what_the_proxy_did_not"`
	// PlainHTTP NAMES the subset explained by plain HTTP rather than counting
	// it. A count beside a list lets a reader conclude the whole list is benign
	// when the numbers happen to match, and gives no way to identify the
	// unexplained host -- the only one that mattered.
	PlainHTTP []string `json:"plain_http"`
	// UnknownDecisions counts trail events whose decision was neither allow nor
	// deny. Silence there would make the Decision field's own comment false.
	UnknownDecisions int `json:"unknown_decisions"`
	// UnknownModes counts events whose transport this reader has not learned.
	UnknownModes int `json:"unknown_modes"`
	// DeniedButReached: the sandbox refused it and the wire recorded reaching
	// it anyway. Traffic that escaped the sandbox -- the strongest finding a
	// fourth evidence source can produce, and the first version cancelled it.
	DeniedButReached []string `json:"denied_but_reached"`
	// Skipped and UnparseableTargets are the reader's own drop counts, carried
	// through so they reach a human. They existed one layer down and stopped
	// there, which made the fix they represent invisible where anybody reads.
	Skipped            int `json:"skipped"`
	UnparseableTargets int `json:"unparseable_targets"`
	// ProxySawWhatItDidNot is the other direction: on the wire, absent from
	// the sandbox's trail. Expected when the two cover different windows, or
	// when traffic left a process the sandbox was not supervising.
	ProxySawWhatItDidNot []string `json:"proxy_saw_what_it_did_not"`
	// Inherited counts trail events outside this session's window.
	Inherited int `json:"inherited"`
	Sessions  int `json:"sessions"`
}

// buildNono reconciles the sandbox's trail against the wire's observation.
//
// The proxy side is read from the DESTINATIONS VIEW rather than the raw
// observation, for the same reason the chain links are: that view has already
// had forgotten hosts suppressed, and a second consumer reading around it is
// how a suppressed host returns in a different section under a different name.
func buildNono(obs nono.Observation, dests Destinations, configured bool, forgotten func(string) bool) Nono {
	n := Nono{
		Configured:            configured,
		Observed:              obs.Observed,
		Reason:                obs.Reason,
		Allowed:               []string{},
		Denied:                []string{},
		SawWhatTheProxyDidNot: []string{},
		PlainHTTP:             []string{},
		ProxySawWhatItDidNot:  []string{},
		Inherited:             obs.Inherited,
		Sessions:              obs.Sessions,
		Skipped:               obs.Skipped,
		UnparseableTargets:    obs.UnparseableTargets,
		DeniedButReached:      []string{},
	}
	if !obs.Observed {
		return n
	}
	// SUPPRESSED ON THE TRAIL SIDE TOO. The comment above claimed reading the
	// Destinations view was enough to keep a forgotten host from returning
	// "under a different heading" -- true of the wire side only. The trail is
	// a second place the name lives, and leaving it unfiltered did something
	// worse than republish: because suppression removes the host from the wire
	// side, the forgotten host became the one thing the section calls out, as
	// "seen by the sandbox and not on the wire". Forgetting PROMOTED it.
	obs = suppressTrail(obs, forgotten)

	n.Allowed = obs.Hosts()
	n.Denied = obs.Denied()

	// COUNTED ABOVE THE WIRE GUARD. This sat below it, so on the default path
	// -- no proxy store, which is most users -- an unknown decision was
	// absorbed silently, making the Decision field's own comment false in the
	// one configuration it mattered.
	for _, e := range obs.Events {
		if e.Decision != nono.DecisionAllow && e.Decision != nono.DecisionDeny {
			n.UnknownDecisions++
		}
	}

	// The comparison is only meaningful when BOTH sides observed something.
	// Against an unread proxy store every sandbox host would read as "the
	// proxy missed it", which blames the wire for not being there.
	if !dests.Observed || !dests.WindowApplied {
		// WINDOW APPLIED TOO, not just observed. With unparseable proxy
		// timestamps the destinations section prints "rows from other sessions
		// may be included" -- and this section then listed every historical
		// wire host as traffic the sandbox missed, with no caveat. chains.go
		// gates hostState on this same flag for this same reason.
		return n
	}

	// The wire side, filtered the way the rest of this package filters it.
	// Unfiltered, this column reported loopback, the client's own plane and
	// another session's inherited rows as traffic the sandbox missed -- three
	// categories the codebase argues at length must never read as findings,
	// and on a real session mostly Claude Code's own model traffic.
	// ONE PREDICATE, BOTH SIDES. Filtering the wire and not the trail moved the
	// false finding from the quiet column into the loud one: a loopback or
	// client-plane host present on BOTH sides satisfied inTrail && !onWire and
	// was reported as traffic the proxy missed -- which on a real session fires
	// every time, because api.anthropic.com is the client's own model traffic.
	//
	// The client-plane test reads the VIEW'S OWN LIST, not clientPlaneHosts. That
	// is a rule chains.go states by name: the two differ for
	// mcp-proxy.anthropic.com, which belongs to the agent on a session that made
	// mcp__* calls. I wrote that rule and broke it two files later.
	clientPlane := map[string]bool{}
	for _, h := range dests.ClientPlane {
		clientPlane[h] = true
	}
	excluded := func(h string) bool { return loopbackHosts[h] || clientPlane[h] }

	onWire := map[string]bool{}
	wireReached := map[string]bool{}
	for _, h := range dests.Hosts {
		if h.Inherited || excluded(h.Host) {
			continue
		}
		onWire[h.Host] = true
		// Whether the WIRE saw a connection, as against a refused attempt.
		if !h.Unreached {
			wireReached[h.Host] = true
		}
	}

	// Two transports per host, tracked apart. The first version ORed
	// mode=="reverse" with port==80 and set a single flag, which was wrong
	// twice: a CONNECT tunnel to port 80 IS visible to the proxy, and a host
	// with one plain-HTTP leg had its missing HTTPS leg excused along with it.
	// The rendered line states the excuse as fact, so both errors produced
	// false comfort about a real gap.
	plainOnly := map[string]bool{}
	observable := map[string]bool{}
	inTrail := map[string]bool{}
	denied := map[string]bool{}
	for _, e := range obs.Events {
		if e.Decision == nono.DecisionDeny {
			denied[e.Host] = true
			continue
		}
		if e.Decision != nono.DecisionAllow {
			// Already counted above the wire guard; skipped here so the
			// number does not double when a proxy store is present.
			continue
		}
		if excluded(e.Host) {
			// The same filter as the wire side. Counted as nothing: loopback and
			// the client's own plane are findings on neither.
			continue
		}
		inTrail[e.Host] = true
		switch e.Mode {
		case "reverse":
			plainOnly[e.Host] = true
		case "connect":
			observable[e.Host] = true
		default:
			// A transport this reader has not learned. Counted rather than
			// defaulted silently into observable: schema drift is a measured
			// property of this dependency.
			n.UnknownModes++
			observable[e.Host] = true
		}
	}

	for h := range inTrail {
		if !onWire[h] {
			n.SawWhatTheProxyDidNot = append(n.SawWhatTheProxyDidNot, h)
			// "Only" means only. A host with any proxy-observable leg has a
			// real gap whatever else it did.
			if plainOnly[h] && !observable[h] {
				n.PlainHTTP = append(n.PlainHTTP, h)
			}
		}
	}
	for h := range onWire {
		// A host the sandbox REFUSED is not one it failed to see. The proxy
		// records the attempt regardless of nono's verdict, so without this the
		// same section said both "refused by the sandbox: github.com" and "on
		// the wire and not in the sandbox's trail: github.com" -- from the
		// fixture's own data.
		if inTrail[h] {
			continue
		}
		if denied[h] {
			// THE STRONGEST FINDING A FOURTH SOURCE CAN PRODUCE, and the first
			// version cancelled it. A denial agrees with the wire only when the
			// wire also shows the host was never reached; if the proxy recorded a
			// connection, the traffic ESCAPED the sandbox.
			if wireReached[h] {
				n.DeniedButReached = append(n.DeniedButReached, h)
			}
			continue
		}
		n.ProxySawWhatItDidNot = append(n.ProxySawWhatItDidNot, h)
	}
	sort.Strings(n.PlainHTTP)
	sort.Strings(n.DeniedButReached)
	sort.Strings(n.SawWhatTheProxyDidNot)
	sort.Strings(n.ProxySawWhatItDidNot)
	return n
}

// suppressTrail drops forgotten hosts from the sandbox's events.
//
// Applied before anything reads them, which is the same ordering
// buildDestinations uses for the wire: "Forgotten destinations are dropped
// from the whole view before anything else looks at them."
func suppressTrail(obs nono.Observation, forgotten func(string) bool) nono.Observation {
	if forgotten == nil || len(obs.Events) == 0 {
		return obs
	}
	kept := make([]nono.Event, 0, len(obs.Events))
	for _, e := range obs.Events {
		if forgotten(e.Host) {
			continue
		}
		kept = append(kept, e)
	}
	obs.Events = kept
	return obs
}
