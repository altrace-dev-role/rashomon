package pylite_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// carriedSrc is a file of the specimen's jobs unit whose overlay emits every
// predicate pylite carries: an import and a from-import of known units, a
// call between two defs, and a read and a write SQL literal.
const carriedSrc = `import auth
from orgs.members import add_member


def load(n):
    return "SELECT id FROM accounts WHERE id = 1"


def save(n):
    load(n)
    return "INSERT INTO accounts (id) VALUES (1)"
`

// TestOverlayCarried_EqualsEmittedPredicates: the predicates of the edges
// Overlay emits from the file node and from the symbols the file declares
// are exactly OverlayCarried. A carried predicate the overlay never emits
// would retract the index's edges of it; an emitted one it does not carry
// would leave stale index edges standing.
func TestOverlayCarried_EqualsEmittedPredicates(t *testing.T) {
	res := graphtest.Resolver()
	rel := "jobs/carried.py"
	recs, err := res.Overlay(rel, []byte(carriedSrc), graph.RevCommit(graphtest.Commit), 1)
	if err != nil {
		t.Fatal(err)
	}
	got, want := strings.Join(emittedPreds(recs, graph.FileID(rel)), ","), res.OverlayCarried().String()
	if got != want {
		t.Errorf("Overlay emits {%s} from the file and its symbols, OverlayCarried is {%s}", got, want)
	}
}

// emittedPreds is the sorted set of predicates of the edges in recs whose
// source is file or a symbol file declares.
func emittedPreds(recs []graph.Record, file graph.ID) []string {
	from := map[graph.ID]bool{file: true}
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Src == file && r.Edge.Pred == graph.Declares {
			from[r.Edge.Dst] = true
		}
	}
	set := map[string]bool{}
	for _, r := range recs {
		if r.Edge != nil && from[r.Edge.Src] {
			set[string(r.Edge.Pred)] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}
