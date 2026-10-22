package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Family coverage answers "which of this session's network-capable tools ran,
// and how many hosts did each name", DERIVED FROM THE RECORD rather than
// measured by probing.
//
// It used to join each family's declared hosts against what the wire observed;
// the proxy that observed the wire never shipped, and its code is gone. What
// stays is the grouping, and the rule behind it: nothing here probes the
// network, because a probe would have to run inside the SessionStart hook,
// which is on the recorder's path, and the recorder must contain no networking
// package at all. What the grouping gives up is a family the session never
// used, which is reported as "not exercised" rather than guessed at.

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

// Family is one program family's coverage for this session.
type Family struct {
	Name string `json:"name"`
	// Programs are the actual program names seen, so a reader can tell pip from
	// python3 inside one family.
	Programs []string `json:"programs"`
	Calls    int      `json:"calls"`
	// HostsDeclared is the number of distinct hosts the family's calls named.
	HostsDeclared int `json:"hosts_declared"`
}

// FamilyCoverage is the whole section.
type FamilyCoverage struct {
	Families     []Family `json:"families"`
	NotExercised []string `json:"not_exercised"`
}

// buildFamilies groups the session's declarations by program family and counts
// each family's calls and declared hosts.
//
// forgotten excludes a host from a family's count for the declaration that
// named it at or before the latest forget --host of that name -- the same
// per-declaration rule buildDestinations applies, and for the same reason: a
// later declaration of a name a project once forgot is real, current usage
// and must still count.
func buildFamilies(run *store.Run, forgotten func(host string, declaredAtMS int64) bool) FamilyCoverage {
	fc := FamilyCoverage{
		Families:     []Family{},
		NotExercised: []string{},
	}
	if forgotten == nil {
		forgotten = func(string, int64) bool { return false }
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
			if forgotten(h, d.RecordedAtMS) {
				continue
			}
			a.hosts[h] = true
		}
		for _, h := range d.SSHHosts {
			if forgotten(h, d.RecordedAtMS) {
				continue
			}
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
