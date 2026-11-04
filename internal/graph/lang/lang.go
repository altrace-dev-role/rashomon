// Package lang is the one interface a language implements to join the
// platform graph (contracts-v2 section 6.2). The engine never names a
// language; it reads records. A language names itself here and nowhere
// else.
package lang

import "github.com/altrace-dev-role/rashomon/internal/graph"

// Resolver is what a language supplies. Extract is the full extractor, run
// by the index out of band, and may start a process. Overlay is the reduced
// extractor run inside a hook over one file's post-edit content, and may
// not: no process, no socket, no file read. A language whose Overlay
// returns nil, nil is admitted without overlays, and every line about its
// files says so.
type Resolver interface {
	Lang() string
	Claims(root, rel string) bool
	Unit(root, rel string) (graph.ID, bool)
	Extract(root string, files []string, rev graph.Rev) ([]graph.Record, error)
	Overlay(rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error)
}

// Registry is an ordered set of resolvers; the first that claims a file
// resolves it. It is built by the caller that wires languages (the index
// command, a hook's overlay writer), never at init.
type Registry struct{ rs []Resolver }

// Register appends r.
func (g *Registry) Register(r Resolver) { g.rs = append(g.rs, r) }

// For is the resolver that claims rel, false when none does.
func (g *Registry) For(root, rel string) (Resolver, bool) {
	for _, r := range g.rs {
		if r.Claims(root, rel) {
			return r, true
		}
	}
	return nil, false
}
