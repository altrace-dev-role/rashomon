package main

import (
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// stopUnitOf is StopInput.UnitOf for the Stop and for its replay under
// root, the project the Stop judged under (W-M3.3). A written path, placed
// under root, maps to the global id of the unit the graph index at or above
// root files it in, by the index's own claim rule over its configuration
// (index.UnitOf), in a conflict row's form: the index root's repository id
// (baseline.RepoID, as the overlay writer keys a conflict row), "#", the
// local id. "" for a path the index would not hold, and for every path where
// no index is found at or above root or its configuration does not read: an
// index below root is not found, so a write under it makes no unit row. It
// loads no graph: the first path it maps reads the configuration and the
// repository id, so a Stop that maps none reads nothing. nil for root "".
// One Stop calls it from one goroutine.
func stopUnitOf(root string) verify.UnitOf {
	if root == "" {
		return nil
	}
	var (
		read    bool
		at, rid string
		unit    func(root, rel string) (graph.ID, bool)
	)
	return func(rel string) string {
		if !read {
			read = true
			if at = graph.FindIndex(root); at != "" {
				if c, err := index.ReadConfig(at); err == nil {
					unit, _ = index.UnitOf(c)
					rid, _ = baseline.RepoID(at)
				}
			}
		}
		if unit == nil || rid == "" {
			return ""
		}
		under, err := filepath.Rel(at, filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !filepath.IsLocal(under) {
			return ""
		}
		u, ok := unit(at, filepath.ToSlash(under))
		if !ok {
			return ""
		}
		return string(graph.GlobalID(rid, u))
	}
}
