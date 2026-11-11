package engine

import "github.com/altrace-dev-role/rashomon/internal/graph"

// Rule is a compiled rule as the conflict detectors read it: a spec.Rule of
// the knowledge graph's kinds with AppliesTo resolved to graph ids. Kind is
// a spec rule kind; From and To are unit-id prefixes; Predicate is a
// decision's check (paths_untouched, symbol_present); Source is the
// knowledge row id the rule was compiled from.
type Rule struct {
	ID        string
	Kind      string
	From      []string
	To        []string
	Predicate string
	AppliesTo []graph.ID
	Source    string
}

// Diff is what changed for a set of files between a base revision and Rev:
// Files are the files' ids and Touched the units that contain them; Decls
// are the records present at Rev and absent at the base, Removed the
// reverse.
type Diff struct {
	Rev     graph.Rev
	Touched []graph.ID
	Files   []graph.ID
	Decls   []graph.Record
	Removed []graph.Record
}
