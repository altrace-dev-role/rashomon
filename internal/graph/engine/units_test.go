package engine

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// units resolves prompt's first target phrase over the specimen under
// weight 1 per token, so the arithmetic in each case is a count.
func units(t *testing.T, g *graph.Store, prompt string) ([]UnitPick, corpus) {
	t.Helper()
	c := scan(g, graph.RevCommit(graphtest.Commit))
	f := Parse(prompt)
	all := uniq(Tokens(prompt))
	toks := all
	if len(f.Targets) > 0 {
		toks = f.Targets[0].Tokens
	}
	picks := Units(g, c, toks, except(all, toks), c.weights(all, false))
	for _, p := range picks {
		t.Logf("%-10s %4.2f of %4.2f  %s  callable %v prec %.2f mass %.2f  %v  %s", p.Class, p.Covered, p.Of, p.Path, p.Callable, p.Precision, p.Mass, p.Nodes, p.Refused)
	}
	return picks, c
}

// Task A's target is "session expiry" (what "respect" opens is the use
// phrase, not the target): services/auth covers both tokens across two
// names, a full claim, entered by is_expired in sessions.py; the other
// units' single "session" names sit at the floor and rank under it; the
// consumers and the reuse follow from there.
func TestUnits_AFullClaimOwnsTaskA(t *testing.T) {
	sp := graphtest.New(t)
	picks, _ := units(t, sp.Store, "make session expiry respect the org-level timeout override.")
	if len(picks) == 0 || picks[0].Path != "services/auth" || picks[0].Class != ClassFull || picks[0].Refused != "" {
		t.Fatalf("picks %+v", picks)
	}
	if p := picks[0]; p.Covered != 2 || p.Of != 2 || len(p.Nodes) == 0 || p.Nodes[0] != graphtest.Sym("services/auth", "auth.sessions.is_expired") {
		t.Errorf("claim %+v", p)
	}
}

// A prompt whose words one name covers whole (purge_sessions) is a single
// claim: jobs owns it over every unit a session name touches.
func TestUnits_ASingleNameCoveringThePhraseOwnsIt(t *testing.T) {
	sp := graphtest.New(t)
	picks, _ := units(t, sp.Store, "purging the sessions")
	if len(picks) == 0 || picks[0].Path != "jobs" || picks[0].Class != ClassSingle || picks[0].Refused != "" {
		t.Fatalf("picks %+v", picks)
	}
}

// "the session" is a name in five units: the tie is settled by the call
// direction, the unit the others reference and that references none of
// them (services/auth) owns it.
func TestUnits_ATieGoesToTheUnitTheOthersCall(t *testing.T) {
	sp := graphtest.New(t)
	picks, _ := units(t, sp.Store, "fix the session")
	if len(picks) < 2 || picks[0].Path != "services/auth" || picks[0].Refused != "" {
		t.Fatalf("picks %+v", picks)
	}
	if picks[1].Class != picks[0].Class || picks[1].Covered != picks[0].Covered {
		t.Errorf("no tie to settle: %+v", picks[:2])
	}
}

// A claim under half the phrase is refused, and so is a tie no call edge
// settles; the refusal is on the pick, and nothing else claims.
func TestUnits_RefusesUnderTheFloorAndAnUnsettledTie(t *testing.T) {
	sp := graphtest.New(t)
	picks, _ := units(t, sp.Store, "fix org expiry daily cache purge")
	if len(picks) == 0 || picks[0].Refused != "below the floor" {
		t.Fatalf("floor: picks %+v", picks)
	}
	picks, _ = units(t, sp.Store, "fix expiry org")
	if len(picks) < 2 || picks[0].Refused != "ambiguous: 2 units tie" {
		t.Fatalf("tie: picks %+v", picks)
	}
	if picks, _ := units(t, sp.Store, "fix the timeout override"); picks != nil {
		t.Fatalf("a phrase no name holds claimed: %+v", picks)
	}
}
