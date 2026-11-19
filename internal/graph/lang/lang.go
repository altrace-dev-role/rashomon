// Package lang is the one interface a language implements to join the
// platform graph (contracts-v2 section 6.2). The engine never names a
// language; it reads records. A language names itself here and nowhere
// else. A program kind with no Resolver is named here too (ProgramKind),
// as a file kind a request can name, not as a language the graph reads;
// and a file kind no language reads at all is named in the kinds table
// (NonSource).
package lang

import (
	"slices"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Resolver is what a language supplies, in three phases. Extract is the
// full extractor, run by the index out of band, and may start a process.
// Configuration is the named step a Registry runs once per root when it
// returns the resolver for a path the resolver claims (RegisterConfigured,
// Claim): one capped read of that root's configuration, the only read the
// hook-side phases make, its error returned to the caller. Overlay is the
// reduced extractor run inside a hook over one file's post-edit content,
// on the resolver the configuration returned, and may not start a process,
// open a socket or read a file; Claims and Unit read nothing either.
// OverlayCarried is the set of predicates Overlay emits, so an overlay
// replaces the index's edges of exactly those predicates and leaves the
// rest standing. A language whose Overlay returns nil, nil is admitted
// without overlays: its OverlayCarried returns nil, its overlays replace no
// index edge, and every line about its files says so.
type Resolver interface {
	Lang() string
	Claims(root, rel string) bool
	Unit(root, rel string) (graph.ID, bool)
	Extract(root string, files []string, rev graph.Rev) ([]graph.Record, error)
	Overlay(rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error)
	OverlayCarried() graph.Carried
}

// Registry is an ordered set of resolvers; the first that claims a file
// resolves it. It is built by the caller that wires languages (the index
// command, index.Wire for a hook), never at init. Its entries are held by
// pointer, so a Registry copied by value shares each entry's configured
// resolvers and the lock that guards them.
type Registry struct{ es []*entry }

// entry is one registered resolver, its configuration step (nil for none),
// and what that step returned for each root it ran under.
type entry struct {
	r         Resolver
	configure func(root string) (Resolver, error)
	mu        sync.Mutex
	by        map[string]configured
}

// configured is one run of an entry's configuration step.
type configured struct {
	r   Resolver
	err error
}

// Register appends r, which needs no configuration: Claim returns r itself.
func (g *Registry) Register(r Resolver) { g.es = append(g.es, &entry{r: r}) }

// RegisterConfigured appends r with its configuration step. r answers
// Claims and Lang; configure(root) returns the resolver that reads files
// under root, or the error that refused root's configuration. It runs at
// most once per root, on the first Claim of a path r claims under it.
func (g *Registry) RegisterConfigured(r Resolver, configure func(root string) (Resolver, error)) {
	g.es = append(g.es, &entry{r: r, configure: configure})
}

// For is the resolver that claims rel, false when none does. It runs no
// configuration step: the resolver is the one registered.
func (g *Registry) For(root, rel string) (Resolver, bool) {
	if e := g.claimant(root, rel); e != nil {
		return e.r, true
	}
	return nil, false
}

// Claim is the resolver that reads rel under root: the first registered
// resolver that claims rel, configured for root. A resolver with no
// configuration step is returned as registered. With a step, the step's
// resolver for root is returned, or, where the step refused, nil and its
// error, the same on every Claim under that root. false, with no error and
// no step run, when no resolver claims rel.
func (g *Registry) Claim(root, rel string) (Resolver, bool, error) {
	e := g.claimant(root, rel)
	if e == nil {
		return nil, false, nil
	}
	if e.configure == nil {
		return e.r, true, nil
	}
	c := e.at(root)
	if c.err != nil {
		return nil, true, c.err
	}
	return c.r, true, nil
}

// Langs is the registered languages in the order they were registered,
// each once.
func (g *Registry) Langs() []string {
	out := make([]string, 0, len(g.es))
	for _, e := range g.es {
		if l := e.r.Lang(); !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// claimant is the first entry whose resolver claims rel, nil for none.
func (g *Registry) claimant(root, rel string) *entry {
	for _, e := range g.es {
		if e.r.Claims(root, rel) {
			return e
		}
	}
	return nil
}

// at is the entry's configuration under root, running the step the first
// time root is asked for and keeping its result, error included, for every
// later ask. The lock is held across the step, so two Claims under one root
// cannot both run it.
func (e *entry) at(root string) configured {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.by[root]; ok {
		return c
	}
	r, err := e.configure(root)
	c := configured{r: r, err: err}
	if e.by == nil {
		e.by = map[string]configured{}
	}
	e.by[root] = c
	return c
}
