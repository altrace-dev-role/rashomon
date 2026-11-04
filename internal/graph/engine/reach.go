package engine

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// ReachBonus is what a candidate gains when the agent cannot read it on
// demand: its file lies outside the directory the hook ran in, where no
// search from the prompt's words finds it. A table gains it when one of
// its writers or readers does, since the join is the only road from this
// directory to that code. This is the product statement's first clause
// (code the agent cannot read on demand) as a ranking term, equal to
// KnowledgeBonus, the second clause's term (card GRAPH-REACH1).
const ReachBonus = 0.5

// Outside is whether a node's code lies outside the hook's working
// directory, under rel, the index path as that directory sees it: a file
// by its path, a symbol by its declaring file, a table by any writer or
// reader. A path outside begins with "../" once made relative; with the
// index root as the working directory nothing is outside.
func Outside(g Reader, rev graph.Rev, rel func(string) string) func(graph.ID) bool {
	var outside func(id graph.ID) bool
	outside = func(id graph.ID) bool {
		s := string(id)
		switch {
		case strings.HasPrefix(s, "file:"):
			return strings.HasPrefix(rel(strings.TrimPrefix(s, "file:")), "../")
		case strings.HasPrefix(s, "sym:"):
			for _, e := range g.In(id, rev, graph.Declares) {
				return outside(e.Src)
			}
		case strings.HasPrefix(s, "tbl:"):
			for _, e := range g.In(id, rev, graph.Writes, graph.Reads) {
				if outside(e.Src) {
					return true
				}
			}
		}
		return false
	}
	return outside
}
