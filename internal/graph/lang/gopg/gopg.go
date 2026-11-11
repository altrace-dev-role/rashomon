// Package gopg is the Go producer behind lang.Resolver (W-M1.3): it maps
// one pgraph sync, read whole by platform.ReadSnapshot, to graph records.
// It parses no Go itself; pgraph is the extractor and runs out of band, so
// the hook binary links no toolchain (AD3, AD12). A Go unit is the package
// directory (AD2); the engine reads the records and never this package.
package gopg

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// Lang is the language namespace gopg writes under.
const Lang = "go"

// mayCallConf is the confidence of a may_call edge: a class-hierarchy
// possibility, never a certainty (the store never marks MayCall certain).
const mayCallConf = 0.5

// prov names pgraph's v0 sidecars as the source of every record.
var prov = graph.Prov{Extractor: "pgraph", Version: "v0"}

// Resolver is the language's lang.Resolver. It has no state: the sync under
// the root is the state.
type Resolver struct{}

// Lang is "go".
func (Resolver) Lang() string { return Lang }

// Claims is every .go file.
func (Resolver) Claims(_, rel string) bool { return strings.HasSuffix(rel, ".go") }

// Unit is unit:go:<dir>, the package directory, "." at the root.
func (Resolver) Unit(_, rel string) (graph.ID, bool) {
	return graph.UnitID(Lang, path.Dir(rel)), true
}

// Overlay is nil, nil: Go is admitted without overlays until W-M2.5 reads a
// file's declarations with go/parser.
func (Resolver) Overlay(string, []byte, graph.Rev, int64) ([]graph.Record, error) {
	return nil, nil
}

// Extract reads the pgraph sync under root (platform.ReadSnapshot) and
// returns its records at rev. With files non-empty, only those files, the
// symbols placed in them and the edges between kept symbols are returned.
// Without a sync the error wraps platform.ErrNotSynced and names the
// remedy, pgraph sync.
func (Resolver) Extract(root string, files []string, rev graph.Rev) ([]graph.Record, error) {
	s, err := platform.ReadSnapshot(context.Background(), root)
	if err != nil {
		return nil, fmt.Errorf("gopg: %w", err)
	}
	return newMapper(s, files, rev).records(), nil
}

// sym is one Go symbol as the graph names it.
type sym struct {
	id        graph.ID
	kind      string // func, method, type, const, var, object
	pkg       string // import path
	unit      string // package directory
	qualified string // the id's spelling after the package path
}

// mapper holds one Snapshot and the maps the mapping needs.
type mapper struct {
	s      platform.Snapshot
	rev    graph.Rev
	units  map[string]string // import path -> package directory
	byLen  []string          // import paths, longest first
	placed map[string]platform.Place
	want   map[string]bool // files to keep; nil keeps all
	syms   map[string]sym  // pgraph id -> symbol, in root
}

func newMapper(s platform.Snapshot, files []string, rev graph.Rev) *mapper {
	m := &mapper{s: s, rev: rev, units: map[string]string{}, placed: map[string]platform.Place{}, syms: map[string]sym{}}
	for pkg, fs := range s.Files {
		if len(fs) > 0 {
			m.units[pkg] = path.Dir(fs[0])
			m.byLen = append(m.byLen, pkg)
		}
	}
	sort.Slice(m.byLen, func(i, j int) bool {
		return len(m.byLen[i]) > len(m.byLen[j]) || (len(m.byLen[i]) == len(m.byLen[j]) && m.byLen[i] < m.byLen[j])
	})
	for _, p := range s.Places {
		m.placed[p.ID] = p
	}
	if len(files) > 0 {
		m.want = map[string]bool{}
		for _, f := range files {
			m.want[f] = true
		}
	}
	return m
}

// keepFile reports whether rel is in the wanted set (all, with no set).
func (m *mapper) keepFile(rel string) bool { return m.want == nil || m.want[rel] }

// keepPkg reports whether any file of pkg is kept.
func (m *mapper) keepPkg(pkg string) bool {
	for _, f := range m.s.Files[pkg] {
		if m.keepFile(f) {
			return true
		}
	}
	return false
}

// symOf resolves a pgraph id (kind:<import path>.<qualified>) to a symbol
// of a synced package, by its place when it has one, else by the longest
// synced import path that prefixes it. A pkg: id and an id of a package
// the sync does not hold (the standard library, another module) are false.
func (m *mapper) symOf(id string) (sym, bool) {
	if s, ok := m.syms[id]; ok {
		return s, true
	}
	kind, rest, ok := strings.Cut(id, ":")
	if !ok || kind == "pkg" {
		return sym{}, false
	}
	pkg := ""
	if p, ok := m.placed[id]; ok {
		pkg = p.Pkg
	} else {
		for _, cand := range m.byLen {
			if strings.HasPrefix(rest, cand+".") {
				pkg = cand
				break
			}
		}
	}
	unit, ok := m.units[pkg]
	if !ok || !strings.HasPrefix(rest, pkg+".") {
		return sym{}, false
	}
	q := rest[len(pkg)+1:]
	s := sym{id: graph.SymID(Lang, unit, kind, q), kind: kind, pkg: pkg, unit: unit, qualified: q}
	m.syms[id] = s
	return s, true
}

// records is every record of the sync, in a stable order: units, files,
// symbols, then edges by (src, pred, dst).
func (m *mapper) records() []graph.Record {
	var out []graph.Record
	pkgs := make([]string, 0, len(m.units))
	for pkg := range m.units {
		if m.keepPkg(pkg) {
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Strings(pkgs)
	seenUnit := map[string]bool{}
	for _, pkg := range pkgs {
		unit := m.units[pkg]
		if !seenUnit[unit] {
			seenUnit[unit] = true
			out = append(out, graph.Record{Node: &graph.Node{ID: graph.UnitID(Lang, unit), Kind: "unit", Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov,
				Attrs: map[string]string{"path": unit, "pkg": pkg, "lang": Lang}}})
		}
		for _, rel := range m.s.Files[pkg] {
			if !m.keepFile(rel) {
				continue
			}
			out = append(out,
				graph.Record{Node: &graph.Node{ID: graph.FileID(rel), Kind: "file", Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov,
					Attrs: map[string]string{"digest": m.s.Digests[rel], "pkg": pkg, "lang": Lang}}},
				graph.Record{Edge: &graph.Edge{Src: graph.UnitID(Lang, unit), Pred: graph.Contains, Dst: graph.FileID(rel), Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov}})
		}
	}
	out = append(out, m.symbols()...)
	return append(out, m.edges()...)
}

// symbols is a sym node per placed symbol in a kept file (or unplaced in a
// kept package), with a declares edge from its file (its unit when it has
// none), then a node and a unit declares edge for every symbol of a kept
// package the facts name and the places do not (an interface's methods).
func (m *mapper) symbols() []graph.Record {
	var out []graph.Record
	done := map[string]bool{}
	for _, p := range m.s.Places {
		if p.File != "" && !m.keepFile(p.File) || p.File == "" && !m.keepPkg(p.Pkg) {
			continue
		}
		s, ok := m.symOf(p.ID)
		if !ok {
			continue
		}
		done[p.ID] = true
		attrs := map[string]string{"name": nameOf(s.qualified), "symkind": s.kind, "pkg": s.pkg}
		if p.Line > 0 {
			attrs["line"] = strconv.Itoa(p.Line)
		}
		if p.Doc != "" {
			attrs["doc"] = p.Doc
		}
		from := graph.UnitID(Lang, s.unit)
		if p.File != "" {
			from = graph.FileID(p.File)
		}
		out = append(out,
			graph.Record{Node: &graph.Node{ID: s.id, Kind: "sym", Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov, Attrs: attrs}},
			graph.Record{Edge: &graph.Edge{Src: from, Pred: graph.Declares, Dst: s.id, Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov}})
	}
	var extra []string
	for _, f := range m.s.Facts {
		for _, id := range [2]string{f.Subject, f.Object} {
			if done[id] {
				continue
			}
			if s, ok := m.symOf(id); ok && m.keepPkg(s.pkg) {
				done[id] = true
				extra = append(extra, id)
			}
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		s := m.syms[id]
		out = append(out,
			graph.Record{Node: &graph.Node{ID: s.id, Kind: "sym", Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov,
				Attrs: map[string]string{"name": nameOf(s.qualified), "symkind": s.kind, "pkg": s.pkg}}},
			graph.Record{Edge: &graph.Edge{Src: graph.UnitID(Lang, s.unit), Pred: graph.Declares, Dst: s.id, Tier: graph.Proven, Conf: 1.0, Rev: m.rev, Prov: prov}})
	}
	return out
}

// edges is one edge per distinct (subject, predicate, object) of the facts
// whose two ends are symbols of kept packages: calls to Calls, may-call to
// MayCall, implements and implements-via-pointer to ResolvesTo, embeds to
// References, and a symbol's declares (a type's methods) to Declares. A
// pkg: subject's declares is the symbols' edge already. Two build configs
// of one fact are one edge.
func (m *mapper) edges() []graph.Record {
	type key struct{ src, pred, dst string }
	seen := map[key]bool{}
	var out []graph.Record
	for _, f := range m.s.Facts {
		pred, via, ok := predOf(f.Predicate)
		if !ok {
			continue
		}
		src, ok := m.symOf(f.Subject)
		if !ok || !m.keepPkg(src.pkg) {
			continue
		}
		dst, ok := m.symOf(f.Object)
		if !ok || !m.keepPkg(dst.pkg) {
			continue
		}
		tier, conf, ok := tierOf(f.Tier, pred)
		if !ok {
			continue
		}
		k := key{string(src.id), string(pred), string(dst.id)}
		if seen[k] {
			continue
		}
		seen[k] = true
		e := &graph.Edge{Src: src.id, Pred: pred, Dst: dst.id, Tier: tier, Conf: conf, Rev: m.rev, Prov: prov}
		if via != "" {
			e.Attrs = map[string]string{"via": via}
		}
		out = append(out, graph.Record{Edge: e})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Edge.Key() < out[j].Edge.Key() })
	return out
}

// predOf is the graph predicate of a pgraph predicate, with the via
// attribute an implements-via-pointer edge carries; false for a predicate
// this mapping does not carry.
func predOf(p string) (graph.Pred, string, bool) {
	switch p {
	case platform.PredDeclares:
		return graph.Declares, "", true
	case platform.PredCalls:
		return graph.Calls, "", true
	case platform.PredMayCall:
		return graph.MayCall, "", true
	case platform.PredImplements:
		return graph.ResolvesTo, "", true
	case platform.PredImplPtr:
		return graph.ResolvesTo, "pointer", true
	case platform.PredEmbeds:
		return graph.References, "", true
	}
	return "", "", false
}

// tierOf is the graph tier and confidence of a pgraph tier: PROVEN at 1.0,
// ALGORITHMIC at mayCallConf for a may_call and 1.0 otherwise, a model's
// proposal as inferred at mayCallConf; false for a tier this mapping does
// not know.
func tierOf(t string, pred graph.Pred) (graph.Tier, float64, bool) {
	switch t {
	case platform.TierProven:
		return graph.Proven, 1.0, true
	case platform.TierAlgorithmic:
		if pred == graph.MayCall {
			return graph.Algorithmic, mayCallConf, true
		}
		return graph.Algorithmic, 1.0, true
	case platform.TierProposed:
		return graph.Inferred, mayCallConf, true
	}
	return "", 0, false
}

// nameOf is the bare identifier of a qualified spelling: Area of
// Square.Area, T of Owner$T.
func nameOf(q string) string {
	if i := strings.LastIndexAny(q, ".$"); i >= 0 {
		return q[i+1:]
	}
	return q
}
