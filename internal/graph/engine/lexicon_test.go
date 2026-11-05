package engine

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestOptions_LexiconIsReserved (W-M1.7, AD16): Options carries the
// lexicon the proposer layer will write, and Build reads none of it until
// AC-P1 adopts it: a lexicon that would expand the prompt's one word to
// the purge job's names changes nothing.
func TestOptions_LexiconIsReserved(t *testing.T) {
	sp := graphtest.New(t)
	rev := graph.RevCommit(graphtest.Commit)
	plain := Build(sp.Store, rev, "fan-in of the daily report", Options{IDF: true})
	lex := Build(sp.Store, rev, "fan-in of the daily report", Options{IDF: true,
		Lexicon: map[string][]string{"fan-in": {"purge", "sessions", "callers"}}})
	if plain.Text() != lex.Text() || len(plain.Tokens) != len(lex.Tokens) {
		t.Errorf("the lexicon changed the brief:\n%s\n--\n%s", plain.Text(), lex.Text())
	}
}
