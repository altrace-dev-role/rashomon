package graph

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// UnknownScope is the scope of a consumer-side contract node: a consumer
// names a route, never the service behind it, so its node is formed with
// this scope and status Unresolved until an evidence rule binds it
// (ResolveConsumers).
const UnknownScope = "?"

// FileID is file:<path>, the path relative to the repository root.
func FileID(rel string) ID { return ID("file:" + rel) }

// UnitID is unit:<lang>:<path>, the unit's root relative to the repository.
func UnitID(lang, path string) ID { return ID("unit:" + lang + ":" + path) }

// SymID is sym:<lang>:<unit path>:<symkind>:<qualified name>.
func SymID(lang, unit, symkind, qualified string) ID {
	return ID("sym:" + lang + ":" + unit + ":" + symkind + ":" + qualified)
}

// CompID is comp:<name>, a declared component.
func CompID(name string) ID { return ID("comp:" + name) }

// ContractID is contract:<ckind>:<scope>:<canonical> (contracts-v2 section
// 3.1). scope is the exposing component's id for http and rpc, a database
// or broker id for table and topic, the repo id for config, env and flag,
// and UnknownScope on the consumer side.
func ContractID(ckind, scope, canonical string) ID {
	return ID("contract:" + ckind + ":" + scope + ":" + canonical)
}

// HTTPCanonical is METHOD:/path with every path parameter reduced to {}:
// /sessions/{id}, /sessions/<int:id> and /sessions/:id are one route.
func HTTPCanonical(method, path string) string {
	p := pathParam().ReplaceAllString(path, "{}")
	return strings.ToUpper(method) + ":" + p
}

// pathParam matches one path parameter in the three common spellings.
func pathParam() *regexp.Regexp {
	pathParamOnce.Do(func() { pathParamOnce.v = regexp.MustCompile(`\{[^/}]*\}|<[^/>]*>|:[A-Za-z_][A-Za-z0-9_]*`) })
	return pathParamOnce.v
}

// pathParamOnce holds pathParam's value, built on first use and never at
// init (RES-2).
var pathParamOnce struct {
	sync.Once
	v *regexp.Regexp
}

// ServiceMap is the declared service map (evidence E1): the consumer-side
// base symbol, as the consumer's code names it, to the component that
// serves it. It is read from the project's declaration file under spec's
// trust rules; here it is handed in.
type ServiceMap map[string]ID

// ConsumerBaseAttr is the consumer-side node attribute that holds the base
// symbol the consumer built the request from (AUTH_URL), the join key for
// a ServiceMap.
const ConsumerBaseAttr = "base"

// ResolveConsumers binds every consumer-side contract node at rev to the
// exposer-scoped nodes with the same kind and canonical. With a service map
// entry for the consumer's base symbol and an exposer under that component,
// it writes resolves_to at tier declared, conf 1.0 (certain), and marks the
// consumer asserted. Otherwise it writes may_resolve_to at conf 0.3 per
// candidate (never certain) and leaves the consumer unresolved. Path
// equality alone binds nothing.
func (s *Store) ResolveConsumers(sm ServiceMap, rev Rev, evidence string) {
	for id := range s.nodes {
		n, ok := s.Node(id, rev)
		if !ok || n.Kind != "contract" {
			continue
		}
		ckind, scope, canonical, ok := splitContract(id)
		if !ok || scope != UnknownScope {
			continue
		}
		cands := s.exposers(ckind, canonical, rev)
		if comp, bound := sm[n.Attrs[ConsumerBaseAttr]]; bound {
			want := ContractID(ckind, string(comp), canonical)
			for _, c := range cands {
				if c != want {
					continue
				}
				s.AssertEdge(Edge{Src: id, Pred: ResolvesTo, Dst: c, Tier: Declared, Conf: 1.0, Rev: rev,
					Prov: Prov{Extractor: "service_map", Evidence: []string{evidence}}})
				n.Status = Asserted
				n.Rev = rev
				s.AssertNode(n)
			}
		}
		for _, c := range cands {
			if _, has := s.Edge(id, ResolvesTo, c, rev); has {
				continue
			}
			s.AssertEdge(Edge{Src: id, Pred: MayResolveTo, Dst: c, Tier: Algorithmic, Conf: 0.3, Rev: rev,
				Prov: Prov{Extractor: "path_equality"}})
		}
	}
}

// exposers is every exposer-scoped contract node at rev with ckind and
// canonical, in a stable order.
func (s *Store) exposers(ckind, canonical string, rev Rev) []ID {
	var out []ID
	for id := range s.nodes {
		k, scope, c, ok := splitContract(id)
		if !ok || k != ckind || c != canonical || scope == UnknownScope {
			continue
		}
		if _, live := s.Node(id, rev); live {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// splitContract is a contract id's parts, false for any other id. A scope
// is UnknownScope or an id of two segments (comp:auth, db:main), and a
// canonical may hold colons of its own (GET:/path), so the id is read by
// structure, not split on the separator.
func splitContract(id ID) (ckind, scope, canonical string, ok bool) {
	rest, found := strings.CutPrefix(string(id), "contract:")
	if !found {
		return "", "", "", false
	}
	ckind, rest, found = strings.Cut(rest, ":")
	if !found {
		return "", "", "", false
	}
	if after, unknown := strings.CutPrefix(rest, UnknownScope+":"); unknown {
		return ckind, UnknownScope, after, after != ""
	}
	skind, rest, found := strings.Cut(rest, ":")
	if !found {
		return "", "", "", false
	}
	sname, canonical, found := strings.Cut(rest, ":")
	if !found || canonical == "" {
		return "", "", "", false
	}
	return ckind, skind + ":" + sname, canonical, true
}

// TableID is a database table's node id: tbl:<schema.name> as the code
// spells it.
func TableID(name string) ID { return ID("tbl:" + name) }
