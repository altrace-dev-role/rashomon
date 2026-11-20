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

// Versioned is a Resolver that states the producer of the records it
// writes, the string index.Build names for its language among an index
// header's producers: its extractor and version, joined by a slash. A
// Versioned resolver registered indexed is unindexed under a header whose
// producers name any other string for its language, so the records an
// older version of the reader wrote count as none (Registry.Unindexed).
type Versioned interface {
	Producer() string
}

// Registry is an ordered set of resolvers; the first that claims a file
// resolves it. It is built by the caller that wires languages (the index
// command, index.Wire for a hook), never at init. Its entries are values,
// so Register allocates nothing past the growth of the list: a Go Stop
// builds a registry from its producers (index.UnitOf), and the Stop's
// allocation budget counts every allocation it makes. A configured entry's
// step is held by pointer, so a Registry copied by value shares each
// configured resolver and the lock that guards it.
type Registry struct{ es []entry }

// entry is one registered resolver and its configuration step, nil for a
// resolver with none. indexed is a resolver registered by RegisterIndexed.
type entry struct {
	r       Resolver
	st      *step
	indexed bool
}

// step is an entry's configuration step and what it returned for each root
// it ran under.
type step struct {
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
func (g *Registry) Register(r Resolver) { g.es = append(g.es, entry{r: r}) }

// RegisterConfigured appends r with its configuration step. r answers
// Claims and Lang; configure(root) returns the resolver that reads files
// under root, or the error that refused root's configuration. It runs at
// most once per root, on the first Claim of a path r claims under it.
func (g *Registry) RegisterConfigured(r Resolver, configure func(root string) (Resolver, error)) {
	g.es = append(g.es, entry{r: r, st: &step{configure: configure}})
}

// RegisterIndexed is RegisterConfigured for a resolver whose files a hook
// reads only where the index holds records of its language written by this
// version of it: where an index header's producers name none, or, for a
// Versioned resolver, a string other than its own, Unindexed reports the
// language, and the hook builds no overlay of its files there and runs no
// configuration step.
func (g *Registry) RegisterIndexed(r Resolver, configure func(root string) (Resolver, error)) {
	g.es = append(g.es, entry{r: r, st: &step{configure: configure}, indexed: true})
}

// Unindexed is the language of the resolver that claims rel, true where it
// was registered by RegisterIndexed and producers, an index header's, names
// no producer of that language or, for a Versioned resolver, one other than
// the string it states, compared for equality alone, never by prefix or
// order. Any other claimant, and a path none claims, is "" and false. It
// runs no configuration step: the string compared is the registered
// resolver's.
func (g *Registry) Unindexed(root, rel string, producers map[string]string) (string, bool) {
	e, ok := g.claimant(root, rel)
	if !ok || !e.indexed {
		return "", false
	}
	l := e.r.Lang()
	if _, has := producers[l]; !has {
		return l, true
	}
	if v, ok := e.r.(Versioned); ok && producers[l] != v.Producer() {
		return l, true
	}
	return "", false
}

// For is the resolver that claims rel, false when none does. It runs no
// configuration step: the resolver is the one registered.
func (g *Registry) For(root, rel string) (Resolver, bool) {
	if e, ok := g.claimant(root, rel); ok {
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
	e, ok := g.claimant(root, rel)
	if !ok {
		return nil, false, nil
	}
	if e.st == nil {
		return e.r, true, nil
	}
	c := e.st.at(root)
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

// claimant is the first entry whose resolver claims rel, false for none.
func (g *Registry) claimant(root, rel string) (entry, bool) {
	for _, e := range g.es {
		if e.r.Claims(root, rel) {
			return e, true
		}
	}
	return entry{}, false
}

// at is the step's result under root, running the step the first time
// root is asked for and keeping its result, error included, for every
// later ask. The lock is held across the step, so two Claims under one
// root cannot both run it.
func (s *step) at(root string) configured {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.by[root]; ok {
		return c
	}
	r, err := s.configure(root)
	c := configured{r: r, err: err}
	if s.by == nil {
		s.by = map[string]configured{}
	}
	s.by[root] = c
	return c
}
