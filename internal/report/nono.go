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
	// PlainHTTPOnly counts how many of these are explained by exactly that.
	SawWhatTheProxyDidNot []string `json:"saw_what_the_proxy_did_not"`
	PlainHTTPOnly         int      `json:"plain_http_only"`
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
func buildNono(obs nono.Observation, dests Destinations, configured bool) Nono {
	n := Nono{
		Configured:            configured,
		Observed:              obs.Observed,
		Reason:                obs.Reason,
		Allowed:               []string{},
		Denied:                []string{},
		SawWhatTheProxyDidNot: []string{},
		ProxySawWhatItDidNot:  []string{},
		Inherited:             obs.Inherited,
		Sessions:              obs.Sessions,
	}
	if !obs.Observed {
		return n
	}
	n.Allowed = obs.Hosts()
	n.Denied = obs.Denied()

	// The comparison is only meaningful when BOTH sides observed something.
	// Against an unread proxy store every sandbox host would read as "the
	// proxy missed it", which blames the wire for not being there.
	if !dests.Observed {
		return n
	}

	onWire := map[string]bool{}
	for _, h := range dests.Hosts {
		onWire[h.Host] = true
	}
	plainHTTP := map[string]bool{}
	inTrail := map[string]bool{}
	for _, e := range obs.Events {
		if e.Decision != nono.DecisionAllow {
			// A denied host never reached the network, so the wire's silence
			// about it is agreement, not a gap.
			continue
		}
		inTrail[e.Host] = true
		if e.Mode == "reverse" || e.Port == 80 {
			plainHTTP[e.Host] = true
		}
	}

	for h := range inTrail {
		if !onWire[h] {
			n.SawWhatTheProxyDidNot = append(n.SawWhatTheProxyDidNot, h)
			if plainHTTP[h] {
				n.PlainHTTPOnly++
			}
		}
	}
	for h := range onWire {
		if !inTrail[h] {
			n.ProxySawWhatItDidNot = append(n.ProxySawWhatItDidNot, h)
		}
	}
	sort.Strings(n.SawWhatTheProxyDidNot)
	sort.Strings(n.ProxySawWhatItDidNot)
	return n
}
