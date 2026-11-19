//go:build t2conflicts

package judge

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// The Conflicts leg of J3 (j3_test.go names the inputs). Built with -tags
// t2conflicts: DiffOf and the rules-taking Conflicts are W-M2.2's
// (as/m2-w2.2 at 35a33a05 and at e3cb7417, diff.go 49 and conflicts.go
// 130), not yet on as/m2; the tag goes at the re-cut that brings them.

// j3Conflict is one conflict as the row file carries it.
type j3Conflict struct {
	Kind     string   `json:"kind"`
	Rule     string   `json:"rule"`
	Unit     string   `json:"unit"`
	Subject  string   `json:"subject"`
	Edge     string   `json:"edge,omitempty"`
	Evidence []string `json:"evidence"`
}

// TestJ3_ConflictsAtStop is J3's reading of one run: the seeded rules over
// the diff from the pinned tree to the arm's final tree, by the M2
// engine's Conflicts, written to RASHOMON_T2_J3_OUT. The base must raise
// nothing on the empty diff (ACM21's first read), or every arm would carry
// the base's conflicts. The overlay view (the changed Go files' overlays
// over the base, what the live path can see) is written beside it and
// claims nothing.
func TestJ3_ConflictsAtStop(t *testing.T) {
	seed, base, stop := os.Getenv("RASHOMON_T2_SEED"), os.Getenv("RASHOMON_T2_J3_BASE"), os.Getenv("RASHOMON_T2_J3_STOP")
	run, out := os.Getenv("RASHOMON_T2_J3_RUN"), os.Getenv("RASHOMON_T2_J3_OUT")
	if seed == "" || base == "" || stop == "" || run == "" || out == "" {
		t.Skip("RASHOMON_T2_SEED, RASHOMON_T2_J3_BASE, _STOP, _RUN and _OUT not all set")
	}
	s, baseRev, stopRev, rid := loadStop(t, base, stop)
	rules := seedRules(t, seed, os.Getenv("RASHOMON_T2_SPEC"), s, rid, baseRev)
	if got := engine.Conflicts(s, engine.Diff{Rev: baseRev}, rules); len(got) != 0 {
		t.Fatalf("the base at %s raises %d conflicts on the empty diff, want none", baseRev, len(got))
	}
	files := changedFiles(t, run)
	atStop := engine.Conflicts(s, engine.DiffOf(s, baseRev, stopRev, files), rules)
	ovRev := overlayView(t, s, baseRev, stop, files)
	view := engine.Conflicts(s, engine.DiffOf(s, baseRev, ovRev, files), rules)
	doc := map[string]any{"run": run, "base": baseRev.String(), "files": len(files),
		"at_stop": rows(atStop), "overlay_view": rows(view),
		"scored_kinds": j3Scored, "at_stop_scored": rows(scoredOnly(atStop)),
		"rules_by_kind": rulesByKind(rules)}
	b, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("J3 %s: %d files; at Stop %s; overlay view %s", run, len(files), byKind(atStop), byKind(view))
}

// overlayView applies the Go overlay of each changed Go file of the final
// tree over base, one overlay revision per file, and returns the last.
// Carried is Declares, exactly what the Go overlay emits (gopg Overlay);
// when gopg's Resolver gains OverlayCarried, as pylite's has, the judge
// calls it instead.
func overlayView(t *testing.T, s *graph.Store, base graph.Rev, stop string, files []graph.ID) graph.Rev {
	t.Helper()
	rev := graph.RevOverlay(base.Commit, 0)
	for _, f := range files {
		rel := strings.TrimPrefix(string(f), "file:")
		content, ok := goContent(t, stop, rel)
		if !ok {
			continue
		}
		next := graph.RevOverlay(base.Commit, rev.Seq+1)
		recs, err := gopg.Resolver{}.Overlay(rel, content, base, next.Seq)
		if err != nil {
			t.Fatalf("overlay %s: %v", rel, err)
		}
		s.ApplyOverlay(f, content, recs, next, graph.Carried{graph.Declares})
		rev = next
	}
	return rev
}

func rows(cs []engine.Conflict) []j3Conflict {
	out := make([]j3Conflict, 0, len(cs))
	for _, c := range cs {
		r := j3Conflict{Kind: string(c.Kind), Rule: c.RuleID, Unit: string(c.Unit), Subject: string(c.Subject)}
		if c.Edge.Src != "" {
			r.Edge = fmt.Sprintf("%s %s %s", c.Edge.Src, c.Edge.Pred, c.Edge.Dst)
		}
		for _, e := range c.Evidence {
			r.Evidence = append(r.Evidence, string(e))
		}
		out = append(out, r)
	}
	return out
}

// rulesByKind counts the seeded rules by engine.Rule.Kind, the spec.RuleKind
// strings ("boundary", "decision"; spec.go:42-43). A scored kind whose rule
// kind counts 0 was not run live, and row.py says so: an absent rule must not
// read as a kept one (amendment A5, accepted in addendum 1).
func rulesByKind(rules []engine.Rule) map[string]int {
	m := map[string]int{}
	for _, r := range rules {
		m[r.Kind]++
	}
	return m
}

// j3Scored is J3's scored set, ruled before any run (the director, 2026-11-11
// 16:23:31 PST, on the k replay's score.txt 0719b323: no k in 2..6 reaches zero
// wrong flags, so duplicate_abstraction ships silent in M2). arity,
// signature_mismatch and protected_path were never scored (no Go writer, no
// rule). Every kind the M2 binary's Conflicts raises stays in at_stop beside;
// "zero unresolved conflicts at Stop" reads at_stop_scored alone.
var j3Scored = []engine.ConflictKind{engine.ConflictBoundaryCross, engine.ConflictDecisionContradicted}

// scoredOnly keeps the conflicts whose kind is in j3Scored, in input order.
func scoredOnly(cs []engine.Conflict) []engine.Conflict {
	var out []engine.Conflict
	for _, c := range cs {
		for _, k := range j3Scored {
			if c.Kind == k {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// TestJ3_ScoredKindsPinned fails when the scored set moves without a ruling.
func TestJ3_ScoredKindsPinned(t *testing.T) {
	want := []engine.ConflictKind{"boundary_cross", "decision_contradicted"}
	if fmt.Sprint(j3Scored) != fmt.Sprint(want) {
		t.Fatalf("j3Scored = %v, want %v (a change needs a ruling before any run)", j3Scored, want)
	}
}

// byKind is the conflicts counted per kind, in ConflictKinds order.
func byKind(cs []engine.Conflict) string {
	var parts []string
	for _, k := range engine.ConflictKinds {
		n := 0
		for _, c := range cs {
			if c.Kind == k {
				n++
			}
		}
		parts = append(parts, fmt.Sprintf("%s %d", k, n))
	}
	return strings.Join(parts, ", ")
}
