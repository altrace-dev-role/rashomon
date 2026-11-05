package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestEvidence_HoldsNoWordOfThePrompt (W-M1.9, D160): the form the hook
// path writes under the store carries the selection and nothing of the
// prompt: not the prompt line, not a token with its weight, not the words
// a scope row matched. Explain, printed by the verb for the person who
// typed the prompt, carries all three.
func TestEvidence_HoldsNoWordOfThePrompt(t *testing.T) {
	sp := graphtest.New(t)
	b := Build(sp.Store, graph.RevCommit(graphtest.Commit), "purging the sessions", Options{IDF: true})
	if b.Empty() || len(b.Rows) == 0 || len(b.Tokens) == 0 {
		t.Fatalf("the specimen brief is empty: %d rows, %d tokens", len(b.Rows), len(b.Tokens))
	}
	ex, ev := b.Explain(), b.Evidence()
	if !strings.Contains(ex, `prompt: "purging the sessions"`) || !strings.Contains(ex, "purg(") {
		t.Errorf("Explain lacks the prompt or its tokens:\n%s", ex)
	}
	for _, word := range []string{"purging", "prompt:", "purg("} {
		if strings.Contains(ev, word) {
			t.Errorf("Evidence holds %q:\n%s", word, ev)
		}
	}
	for _, r := range b.Rows {
		if len(r.Hits) > 0 && strings.Contains(ev, "["+strings.Join(r.Hits, " ")+"]") {
			t.Errorf("Evidence names the words %s matched: %v", r.ID, r.Hits)
		}
	}
	for _, same := range []string{"scope (", "rank (", "select (", "render (", string(b.Lines[0].Key), "rev: "} {
		if !strings.Contains(ev, same) {
			t.Errorf("Evidence lacks %q:\n%s", same, ev)
		}
	}
}
