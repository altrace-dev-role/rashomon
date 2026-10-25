package pgraph

import (
	"errors"
	"fmt"
	"go/types"
	"sort"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/ssa/ssautil"
)

// CHAVersion names the rules of Dispatch. A change to what it emits for the
// same program takes a new version: v1 added the facts whose caller is a
// dependency function (the bodies of dependencies are part of the program), so
// the next SyncCHA of a v0 log tombstones its facts and asserts the v1 ones.
const CHAVersion = "v1"

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
// published as precision against a labelled corpus, with the counts and the
// corpus's size (Accuracy.Describe): on dispatch-v1, 14 labelled edges,
// precision is 0.368 (14 of 38 CHA edges are true) and recall 1.000 (14 of 14).
//
// The algorithm is go/callgraph/cha over the SSA of the whole loaded program,
// dependencies included: Load type-checks them from source, so an interface
// call inside the standard library or a module the scan root imports (sort.Sort
// calling Less on a repo type) is a call site like any other. Facts are kept in these terms, so that each is a statement in the same
// vocabulary as the PROVEN ones:
//
//   - Callers are the Calls convention: the declared function, a closure
//     belonging to the function that contains it, and the package for a
//     package-level initialiser. A promoted-method wrapper that the compiler
//     writes has no source; its dispatch is credited to whoever calls the
//     wrapper. For a call site inside a dependency the caller is the
//     dependency's exported entry point that statically reaches it, in the
//     same id vocabulary (func:sort.Sort); see callersOf for the rule and why.
//   - Targets are concrete methods declared in the scanned packages. Abstract
//     methods (an interface's own) are not targets, and neither is any
//     implementer outside the scan root, which would drown the graph in every
//     standard-library type that satisfies error or io.Writer. Calls inside a
//     dependency that reach scanned code are facts; the dependency's own
//     methods are never targets, and a dependency calling another dependency
//     makes no fact.
//   - Only interface-method calls make facts. A call through a func value is
//     not dispatch in this sense, and CHA's answer for it (every function of
//     that signature) is not published. Generic bodies are whatever CHA
//     yields for them and are not claimed.
//
// Time is that of SSA construction plus CHA, O(call sites x implementers),
// then sorting the facts. The result is sorted and without duplicates.
//
// Cost of reading dependency bodies, measured on this repository (the main
// module at this commit, 191 packages in go list -deps, 4 cores, go1.27.3,
// three runs each; peak RSS is the largest process, ours or go list's):
//
//	scan root only, Load + Extract                0.35-0.38 s    ~62 MB
//	Dispatch before (export data, no bodies)      0.46-0.73 s    ~95 MB    74 edges
//	Load with dependency bodies, nothing built    0.94-1.07 s   ~275 MB
//	Dispatch now (bodies, SSA, CHA)               1.55-1.71 s   ~450 MB   736 edges
//
// That is about three times the wall time and five times the memory of the
// dispatch it replaces, and under two seconds and half a gigabyte in absolute
// terms: judged against a scan that runs once per commit, the whole
// dependency set is loaded and there is no measured remainder. Sync (the
// PROVEN tier) does not pay it, since LoadScanRoot is enough for it. The 662
// added edges all have a dependency function as caller; the 74 from the scan
// root are the same as before.
func Dispatch(p *Program) ([]Triple, error) {
	if !p.deps {
		return nil, errors.New("pgraph: Dispatch needs a Program loaded with dependency bodies (Load, not LoadScanRoot)")
	}
	prog, built := ssautil.AllPackages(p.pkgs, 0)
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
	c := &callAttributor{nm: nm, roots: roots, memo: map[*callgraph.Node][]string{}}
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
			for _, from := range c.callers(node) {
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

// callAttributor names the declared function a call graph node's calls belong
// to, memoised per node.
type callAttributor struct {
	nm    *namer
	roots map[string]bool
	memo  map[*callgraph.Node][]string
}

func (c *callAttributor) callers(n *callgraph.Node) []string {
	if r, ok := c.memo[n]; ok {
		return r
	}
	r := c.callersOf(n, map[*callgraph.Node]bool{}, false)
	c.memo[n] = r
	return r
}

// callersOf names the declared function a call graph node's calls belong to:
// the node itself unless it is a closure (its outermost parent) or compiler-
// written (a wrapper or thunk), which hands its calls to its own callers,
// transitively. seen guards against cycles through wrappers.
//
// Inside a dependency the same hand-over applies one step further. A call
// site in an unexported dependency function (sort's insertionSort calls Less,
// its pdqsort calls insertionSort, and sort.Sort calls pdqsort) is credited
// to the exported functions that reach it, so sort.Sort may-call byLen.Less:
// the edge sits where a reader of the graph can find it, on the function the
// scanned code can call, and the dependency's internal helpers, whose names
// change with every release of the dependency, are not facts. Exported means
// an exported name, and for a method an exported receiver type. A dependency
// function that no static call reaches (it is called through a func value or
// by linkname) is credited to its package. A scanned function is always its
// own caller, exported or not.
//
// Only static calls hand a dependency helper's (or a dependency wrapper's)
// dispatch upward (static says the walk is already inside a dependency): a
// call through an interface or a func value that CHA says may reach a
// dependency's unexported method is not a reason to credit the caller with
// what that method does in turn, or one interface call in scanned code would
// inherit the dispatch of every unexported implementer in the dependency
// graph.
func (c *callAttributor) callersOf(n *callgraph.Node, seen map[*callgraph.Node]bool, static bool) []string {
	fn := n.Func
	for fn.Parent() != nil {
		fn = fn.Parent()
	}
	switch {
	case fn.Synthetic == "package initializer" && fn.Pkg != nil:
		return []string{"pkg:" + fn.Pkg.Pkg.Path()}
	case fn.Synthetic == "":
		obj, ok := fn.Object().(*types.Func)
		if !ok {
			return nil
		}
		if obj.Pkg() == nil || c.roots[obj.Pkg().Path()] || exportedFunc(obj) {
			return []string{c.nm.id(obj)}
		}
		// An unexported function of a dependency.
		static = true
	default:
		// A wrapper or thunk the compiler wrote; one on a dependency's type
		// is the dependency's.
		if recv := fn.Signature.Recv(); recv != nil {
			t := types.Unalias(recv.Type())
			if ptr, ok := t.(*types.Pointer); ok {
				t = types.Unalias(ptr.Elem())
			}
			if nt, ok := t.(*types.Named); ok && nt.Obj().Pkg() != nil && !c.roots[nt.Obj().Pkg().Path()] {
				static = true
			}
		}
	}
	if seen[n] {
		return nil
	}
	seen[n] = true
	var out []string
	callers := 0
	for _, in := range n.In {
		if static && (in.Site == nil || in.Site.Common().StaticCallee() == nil) {
			continue
		}
		callers++
		out = append(out, c.callersOf(in.Caller, seen, static)...)
	}
	if callers == 0 && fn.Synthetic == "" {
		out = append(out, "pkg:"+fn.Object().Pkg().Path())
	}
	return out
}

// exportedFunc reports whether code outside f's package could name f: an
// exported name, on an exported receiver type for a method.
func exportedFunc(f *types.Func) bool {
	if !f.Exported() {
		return false
	}
	recv := f.Type().(*types.Signature).Recv()
	if recv == nil {
		return true
	}
	t := types.Unalias(recv.Type())
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	nt, ok := t.(*types.Named)
	return ok && nt.Obj().Exported()
}
