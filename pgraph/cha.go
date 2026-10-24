package pgraph

import (
	"fmt"
	"go/types"
	"sort"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/ssa/ssautil"
)

// CHAVersion names the rules of Dispatch. A change to what it emits for the
// same program takes a new version.
const CHAVersion = "v0"

func chaProducer() Producer {
	p, err := newProducer("cha", CHAVersion, Algorithmic)
	if err != nil {
		panic(err) // constants; unreachable
	}
	return p
}

// SyncCHA reconciles the ALGORITHMIC dispatch facts of p, under p's config at
// the given commit, into the log. It touches no PROVEN fact, and Sync touches
// no ALGORITHMIC one: the two tiers are two producers on two clocks.
func SyncCHA(l *Log, p *Program, commit string) (Delta, error) {
	ts, err := Dispatch(p)
	if err != nil {
		return Delta{}, err
	}
	return l.Reconcile(chaProducer(), p.cfg.String(), commit, ts)
}

// Dispatch returns the may-call facts of p (PF-PL3): for each call on an
// interface method, an edge from the function containing the call to every
// concrete method that class hierarchy analysis says it can reach. The
// facts are ALGORITHMIC, never PROVEN: CHA assumes every type that satisfies
// the interface can be put into it, so the set is sound (it contains every
// true target among the loaded types, on a partial program too) and may be
// larger than the true set. Spurious edges are expected; their share is
// published as precision against a labelled corpus.
//
// The algorithm is go/callgraph/cha over the SSA of the whole loaded program.
// Facts are kept in these terms, so that each is a statement in the same
// vocabulary as the PROVEN ones:
//
//   - Callers are the Calls convention: the declared function, a closure
//     belonging to the function that contains it, and the package for a
//     package-level initialiser. A promoted-method wrapper that the compiler
//     writes has no source; its dispatch is credited to whoever calls the
//     wrapper.
//   - Targets are concrete methods declared in the scanned packages. Abstract
//     methods (an interface's own) are not targets, and neither is any
//     implementer outside the scan root, which would drown the graph in every
//     standard-library type that satisfies error or io.Writer. Dependencies
//     are read from export data, so their own bodies are not part of the
//     program: calls inside a dependency that reach scanned code are not seen.
//   - Only interface-method calls make facts. A call through a func value is
//     not dispatch in this sense, and CHA's answer for it (every function of
//     that signature) is not published. Generic bodies are whatever CHA
//     yields for them and are not claimed.
//
// Time is that of SSA construction plus CHA, O(call sites x implementers),
// then sorting the facts. The result is sorted and without duplicates.
func Dispatch(p *Program) ([]Triple, error) {
	prog, built := ssautil.Packages(p.pkgs, 0)
	for i, b := range built {
		if b == nil {
			return nil, fmt.Errorf("pgraph: no SSA for %s", p.pkgs[i].PkgPath)
		}
	}
	prog.Build()
	cg := cha.CallGraph(prog)
	nm := newNamer(p)
	roots := make(map[string]bool, len(p.pkgs))
	for _, pkg := range p.pkgs {
		roots[pkg.PkgPath] = true
	}
	var out []Triple
	for fn, node := range cg.Nodes {
		if fn == nil {
			continue
		}
		for _, e := range node.Out {
			if e.Site == nil || !e.Site.Common().IsInvoke() {
				continue
			}
			obj, ok := e.Callee.Func.Object().(*types.Func)
			if !ok || obj.Pkg() == nil || !roots[obj.Pkg().Path()] || types.IsInterface(obj.Type().(*types.Signature).Recv().Type()) {
				continue
			}
			for _, from := range callersOf(nm, node, map[*callgraph.Node]bool{}) {
				out = append(out, Triple{from, MayCall, nm.id(obj)})
			}
		}
	}
	sort.Slice(out, func(a, b int) bool { return tripleLess(out[a], out[b]) })
	uniq := out[:0]
	for i, t := range out {
		if i == 0 || t != out[i-1] {
			uniq = append(uniq, t)
		}
	}
	return uniq, nil
}

// callersOf names the declared function a call graph node's calls belong to:
// the node itself unless it is a closure (its outermost parent) or compiler-
// written (a wrapper or thunk), which hands its calls to its own callers,
// transitively. seen guards against cycles through wrappers.
func callersOf(nm *namer, n *callgraph.Node, seen map[*callgraph.Node]bool) []string {
	fn := n.Func
	for fn.Parent() != nil {
		fn = fn.Parent()
	}
	switch {
	case fn.Synthetic == "package initializer" && fn.Pkg != nil:
		return []string{"pkg:" + fn.Pkg.Pkg.Path()}
	case fn.Synthetic == "":
		if obj, ok := fn.Object().(*types.Func); ok {
			return []string{nm.id(obj)}
		}
		return nil
	}
	if seen[n] {
		return nil
	}
	seen[n] = true
	var out []string
	for _, in := range n.In {
		out = append(out, callersOf(nm, in.Caller, seen)...)
	}
	return out
}
