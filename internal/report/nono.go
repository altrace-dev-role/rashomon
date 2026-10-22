package report

import (
	"github.com/altrace-dev-role/rashomon/internal/nono"
)

// Nono is what the sandbox saw: the hosts it allowed and refused inside this
// session's window, and the reader's own drop counts, so that they reach a
// human rather than stopping one layer down.
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
	//
	// Neither holds the client's own traffic: see ClientPlaneAllowed.
	Allowed []string `json:"allowed"`
	Denied  []string `json:"denied"`
	// ClientPlaneAllowed and ClientPlaneDenied are the client's own traffic in
	// the trail -- Claude Code's model API and telemetry -- kept out of the two
	// lists above. Measured inside nono 0.79: api.anthropic.com was counted as
	// an allowed host of the session, and the client's log upload was listed
	// as "refused by the sandbox", an attempt the agent never made. Named
	// rather than dropped, so a reader can still see what the sandbox did with
	// them -- and split by decision for that reason: one list of both said a
	// host was there and not whether the sandbox let it through.
	ClientPlaneAllowed []string `json:"client_plane_allowed"`
	ClientPlaneDenied  []string `json:"client_plane_denied"`
	// UnknownDecisions counts trail events whose decision was neither allow nor
	// deny. Silence there would make the Decision field's own comment false.
	UnknownDecisions int `json:"unknown_decisions"`
	// UnknownModes counts events whose transport this reader has not learned,
	// as UnknownDecisions is.
	UnknownModes int `json:"unknown_modes"`
	// Skipped and UnparseableTargets are the reader's own drop counts, carried
	// through so they reach a human. They existed one layer down and stopped
	// there, which made the fix they represent invisible where anybody reads.
	Skipped            int `json:"skipped"`
	UnparseableTargets int `json:"unparseable_targets"`
	// Inherited counts trail events outside this session's window.
	Inherited int `json:"inherited"`
	Sessions  int `json:"sessions"`
	// NetworkEventsInTrail is the trail's whole-file count of "network"
	// records, in or out of the window. A trail can be Observed (it holds a
	// session_started and session_ended record) and still carry zero of
	// them, which means the sandbox never recorded a network decision at
	// all -- not that it filtered the session and saw nothing. The text
	// renders the two differently; see writeNono.
	//
	// NOT JSON: TRU-57 freezes this section's key set. New information goes
	// into an existing key or into the text report, never a new JSON key --
	// so this is `json:"-"` and the distinction reaches a reader through the
	// sandbox line's wording alone.
	NetworkEventsInTrail int `json:"-"`

	// WindowOpen is true when the session has no end record, so the window
	// runs to the end of the trail and a later session's events on a shared
	// trail count as this one's. The text says so on the sandbox line.
	WindowOpen bool `json:"window_open"`
}

// buildNono reads the sandbox's trail for this session's window. mcpAttributed
// and named are the session's own: whether it made an mcp__* call, and the
// hosts its tool calls named. They decide which of the trail's hosts are the
// client's own traffic.
func buildNono(obs nono.Observation, configured, mcpAttributed bool, named []string, forgotten func(string) bool) Nono {
	n := Nono{
		Configured:           configured,
		Observed:             obs.Observed,
		Reason:               obs.Reason,
		Allowed:              []string{},
		Denied:               []string{},
		ClientPlaneAllowed:   []string{},
		ClientPlaneDenied:    []string{},
		Inherited:            obs.Inherited,
		WindowOpen:           obs.WindowOpen,
		Sessions:             obs.Sessions,
		Skipped:              obs.Skipped,
		UnparseableTargets:   obs.UnparseableTargets,
		NetworkEventsInTrail: obs.NetworkEventsInTrail,
	}
	if !obs.Observed {
		return n
	}
	// Forgotten hosts are dropped from the trail before anything reads it. The
	// trail is not ours to delete from, so without this a host whose records
	// were forgotten reappears in the next report as something the sandbox
	// saw -- the forget reading as though it had been undone.
	obs = suppressTrail(obs, forgotten)

	// THE CLIENT'S OWN TRAFFIC, set apart before anything counts it. Without
	// this, api.anthropic.com was an allowed host of every session and the
	// client's log upload was "refused by the sandbox". The rule is
	// clientPlaneHost, the one the novelty candidates apply, so the MCP
	// transport stays the agent's on a session that made mcp__* calls.
	//
	// EXCEPT A HOST A TOOL CALL NAMED. The list says which hosts the client
	// contacts on its own; it cannot say a session's attempt on one is the
	// client's. A trail event carries no caller, so when the session's own
	// call named the host the attempt stays in the agent's columns. Filed as
	// the client's, a refused `curl` to the Datadog log intake -- a shared
	// endpoint that takes posts under any account's key -- left "refused by
	// the sandbox" and was named as traffic the agent did not make.
	declared := make(map[string]bool, len(named))
	for _, h := range named {
		declared[h] = true
	}
	clientPlane := func(h string) bool { return clientPlaneHost(h, mcpAttributed) }
	clientsOwn := func(h string) bool { return clientPlane(h) && !declared[h] }
	// Split per decision, from lists the trail already sorted, so both halves
	// keep that order without a map between them.
	split := func(hosts []string) (agent, client []string) {
		agent, client = []string{}, []string{}
		for _, h := range hosts {
			if clientsOwn(h) {
				client = append(client, h)
				continue
			}
			agent = append(agent, h)
		}
		return agent, client
	}
	n.Allowed, n.ClientPlaneAllowed = split(obs.Hosts())
	n.Denied, n.ClientPlaneDenied = split(obs.Denied())

	// Every event is counted, whatever its decision or host: an unknown
	// decision or transport absorbed silently would make the Decision field's
	// own comment false.
	for _, e := range obs.Events {
		if e.Decision != nono.DecisionAllow && e.Decision != nono.DecisionDeny {
			n.UnknownDecisions++
		}
		if e.Mode != "reverse" && e.Mode != "connect" {
			n.UnknownModes++
		}
	}

	return n
}

// suppressTrail drops forgotten hosts from the sandbox's events, before
// anything reads them.
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
