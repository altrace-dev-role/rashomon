package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Family coverage answers "which of this session's tools actually went through
// the proxy", and it is DERIVED FROM THE JOIN rather than measured by probing.
//
// The obvious implementation is to make one request per program family at
// session start and see which appear on the wire. That was rejected, and not
// for cost: the probe would have to run inside the SessionStart hook, which is
// on the recorder's path, and the recorder must contain no networking package
// at all. A probe would also measure the wrong thing -- whether curl honours
// the variables in principle, not whether THIS session's curl calls were
// observed.
//
// Deriving it from records the session actually produced measures the real
// question and costs nothing. What it gives up is coverage of a family the
// session never used, which is honestly reported as "not exercised" rather
// than guessed at from a synthetic request.

// Family statuses. Four, not two, because each says something different and a
// reader who cannot tell them apart will draw the wrong conclusion from three
// of them.
const (
	// FamilyTransit: this family declared hosts and at least one was observed
	// on the wire. Proof, for this session, that its traffic is being seen.
	FamilyTransit = "transit measured"
	// FamilyNotObserved: it declared hosts and none appeared. Either the
	// program does not honour the proxy variables, or the calls did not run, or
	// the responses were cached. The line does not pick between them.
	FamilyNotObserved = "declared hosts not observed"
	// FamilyNoHosts: the family ran but named no host -- `go build`, `git
	// status`. Nothing to join on, and claiming either of the above would be
	// inventing a measurement.
	FamilyNoHosts = "exercised, no hosts declared"
	// FamilyNotExercised is reserved for a family the session never used. It is
	// not rendered per family; the absent families are counted instead, because
	// a report listing ten "not exercised" lines buries the ones that matter.
	FamilyNotExercised = "not exercised"
)

// families maps a program name to the family it belongs to.
//
// Grouped because the question is about the TOOL, not the binary: pip and
// python3 are one story, and so are npm, npx and node. A program not listed
// here is not reported as a family at all -- this is a named set, and an
// unrecognised program would otherwise create a family per command a session
// happened to run.
var families = map[string]string{
	"curl": "curl", "wget": "curl",

	"python3": "python", "python": "python", "pip": "python", "pip3": "python",
	"uv": "python", "poetry": "python",

	"node": "node", "npm": "node", "npx": "node", "yarn": "node",
	"pnpm": "node", "bun": "node",

	"go": "go",

	"git": "git",
	"gh":  "gh",

	"aws": "aws", "kubectl": "kubectl", "helm": "helm", "brew": "brew",
}

// notObservable is what this instrument cannot see, whatever the session did.
//
// Listed in every report, including a completely healthy one, because these are
// properties of the tool and not of the session. A reader who is told only what
// WAS observed will assume the rest is absence of traffic rather than absence
// of observation -- which is the same silence-as-zero error the coverage rules
// exist to prevent, one level up.
//
// Node's built-in fetch is measured, not assumed: on v22.19.0 it made a request
// with no CONNECT reaching the proxy, with and without NODE_USE_ENV_PROXY set.
var notObservable = []string{
	"Node built-in fetch (measured on v22.19.0: no CONNECT, with or without NODE_USE_ENV_PROXY)",
	"ssh and git over ssh (not CONNECT, so outside what this proxy sees)",
	"DNS resolution (a name is resolved before any proxy is consulted)",
	"raw sockets (no proxy variable applies)",
	"plain HTTP (not observed in this release; no HTTP_PROXY is exported)",
}

// Family is one program family's coverage for this session.
type Family struct {
	Name string `json:"name"`
	// Programs are the actual program names seen, so a reader can tell pip from
	// python3 inside one family.
	Programs []string `json:"programs"`
	Calls    int      `json:"calls"`
	Status   string   `json:"status"`
	// HostsDeclared and HostsObserved are the join, per family: the number it
	// named and the number of those the wire confirmed.
	HostsDeclared int `json:"hosts_declared"`
	HostsObserved int `json:"hosts_observed"`
}

// FamilyCoverage is the whole section.
type FamilyCoverage struct {
	// Available is false when the proxy store could not be read. Without it
	// every family would read as "declared hosts not observed", which is a
	// statement about the families rather than about the missing store -- the
	// worst available misreading.
	Available     bool     `json:"available"`
	Reason        string   `json:"reason,omitempty"`
	Families      []Family `json:"families"`
	NotExercised  []string `json:"not_exercised"`
	NotObservable []string `json:"not_observable"`
}

// buildFamilies groups the session's declarations by program family and joins
// each family's declared hosts against what the wire observed.
func buildFamilies(run *store.Run, observedHosts map[string]bool, observed bool, reason string) FamilyCoverage {
	fc := FamilyCoverage{
		Available:     observed,
		Families:      []Family{},
		NotExercised:  []string{},
		NotObservable: notObservable,
	}
	if !observed {
		fc.Reason = reason
	}

	type acc struct {
		programs map[string]bool
		hosts    map[string]bool
		calls    int
	}
	seen := map[string]*acc{}
	for _, d := range run.Declarations {
		if d.Shape.Program == nil {
			continue
		}
		name, ok := families[*d.Shape.Program]
		if !ok {
			continue
		}
		a := seen[name]
		if a == nil {
			a = &acc{programs: map[string]bool{}, hosts: map[string]bool{}}
			seen[name] = a
		}
		a.calls++
		a.programs[*d.Shape.Program] = true
		for _, h := range d.Hosts {
			a.hosts[h] = true
		}
	}

	// Every family name in the table, so "not exercised" is a complete list
	// rather than whatever happened to come to mind.
	all := map[string]bool{}
	for _, name := range families {
		all[name] = true
	}
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		a := seen[name]
		if a == nil {
			fc.NotExercised = append(fc.NotExercised, name)
			continue
		}
		f := Family{
			Name:          name,
			Programs:      sortedKeys(a.programs),
			Calls:         a.calls,
			HostsDeclared: len(a.hosts),
		}
		for h := range a.hosts {
			if observedHosts[h] {
				f.HostsObserved++
			}
		}
		switch {
		case !observed:
			// The store could not be read, so this family's status is unknown
			// rather than negative. Saying "declared hosts not observed" here
			// would blame the family for the reader's own blindness.
			f.Status = unknown
		case f.HostsDeclared == 0:
			f.Status = FamilyNoHosts
		case f.HostsObserved > 0:
			f.Status = FamilyTransit
		default:
			f.Status = FamilyNotObserved
		}
		fc.Families = append(fc.Families, f)
	}
	return fc
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// observedHostSet is the non-inherited hosts the wire confirmed, which is what
// a family's declared hosts are joined against.
//
// Inherited rows are excluded for the same reason they are excluded everywhere
// else: they are another session's traffic, and letting them satisfy this
// session's family join would report transit this session never had.
func observedHostSet(d Destinations) map[string]bool {
	out := map[string]bool{}
	for _, h := range d.Hosts {
		if !h.Inherited && h.Attempts > 0 {
			out[h.Host] = true
		}
	}
	return out
}
