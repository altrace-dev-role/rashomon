package engine

import (
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// buildLessons is the ablation arm (card GRAPH-ABL1): a token match over
// the knowledge nodes alone against the prompt, every token at weight 1,
// no traversal of the platform graph and no rank beyond the match score;
// the matched records, best first, become knowledge lines through the same
// slot, template and budget as Build. A record's path is the file its own
// applies_to names, which is part of the record, not a traversal. What
// this arm cannot do is name code: a file, a table or a symbol reaches the
// brief only through Build.
func buildLessons(g *graph.Store, rev graph.Rev, prompt string, opt Options) Brief {
	start := time.Now()
	b := Brief{Rev: rev, Prompt: prompt, Tokens: uniq(Tokens(prompt)), Scorer: "lessons", Weights: map[string]float64{}}
	for _, t := range b.Tokens {
		b.Weights[t] = 1
	}
	weight := func(t string) float64 { return b.Weights[t] }
	for _, n := range g.Nodes(rev) {
		if !IsKnowledge(n.Kind) {
			continue
		}
		if s, hits := Score(b.Tokens, FieldsOf(n), weight); s > 0 {
			b.Rows = append(b.Rows, ScopeRow{ID: n.ID, Kind: n.Kind, Score: s, Hits: hits})
		}
	}
	sort.SliceStable(b.Rows, func(i, j int) bool {
		if b.Rows[i].Score != b.Rows[j].Score {
			return b.Rows[i].Score > b.Rows[j].Score
		}
		return b.Rows[i].ID < b.Rows[j].ID
	})
	rel := opt.rel()
	for i := range b.Rows {
		if i >= MaxLines {
			b.Decisions = append(b.Decisions, Decision{ID: b.Rows[i].ID, Why: "cut: budget"})
			continue
		}
		b.Rows[i].Seed = true
		b.Seeds = append(b.Seeds, Seed{ID: b.Rows[i].ID, Score: b.Rows[i].Score, Hits: b.Rows[i].Hits})
		n, _ := g.Node(b.Rows[i].ID, rev)
		b.Lines = append(b.Lines, Line{Key: n.ID, Score: b.Rows[i].Score, Members: []graph.ID{n.ID},
			Slot: slot.NewKnowledgeLine(knowledgeFact(g, rev, n, "", rel))})
		b.Decisions = append(b.Decisions, Decision{ID: n.ID, Line: len(b.Lines), Why: "selected"})
	}
	b.Neighborhood = len(b.Seeds)
	b.bind()
	b.deliver()
	b.Elapsed = time.Since(start)
	return b
}
