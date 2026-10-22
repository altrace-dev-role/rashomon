package nono

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// openers are the os functions that open a file by its path.
var openers = map[string]bool{
	"Open": true, "ReadFile": true, "OpenFile": true,
	"OpenInRoot": true, "OpenRoot": true, "DirFS": true,
}

// declNode is one function, method or package-level var: what an importer
// can be handed. body is what it refers to -- a function's body, a var's
// initializers.
type declNode struct {
	file, kind, name string
	exported         bool
	body             []ast.Node
}

// refs is what n refers to: whether it names an opener itself, the plain
// identifiers in it, and the names it selects (x.Name). os is matched by its
// import name in the file, so an alias or a dot import is still os.
func refs(n ast.Node, osNames map[string]bool, dotOS bool) (direct bool, idents, sels []string) {
	sel := map[*ast.Ident]bool{}
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			// Pre-order: the selector is seen before its Sel, so Sel is
			// marked here and not read again below as a plain identifier.
			sel[x.Sel] = true
			if pkg, ok := x.X.(*ast.Ident); ok && osNames[pkg.Name] && openers[x.Sel.Name] {
				direct = true
			}
			sels = append(sels, x.Sel.Name)
		case *ast.Ident:
			if sel[x] {
				return true
			}
			if dotOS && openers[x.Name] {
				direct = true
			}
			idents = append(idents, x.Name)
		}
		return true
	})
	return direct, idents, sels
}

// exportsReachingAnOpener parses files (name to source) as one package and
// lists each exported function, method or package-level var that can reach
// an os opener, directly or through anything else the package declares, and
// how many exported declarations it checked.
//
// A reference reaches, not only a call: `var ReadTrail = read` and a func
// that returns read each hand an importer the opener without calling it. A
// method is matched by its name whatever its receiver, since resolving
// x.Load to a type needs go/types, which this check does not carry; a
// method named like one that reaches is taken to reach. That can fail a
// clean package, never pass one that leaks.
func exportsReachingAnOpener(t *testing.T, files map[string]string) (leaks []string, checked int) {
	t.Helper()
	fset := token.NewFileSet()
	var nodes []*declNode
	direct := map[*declNode]bool{}
	edges := map[*declNode][]string{} // "id:name" or "sel:name"
	byIdent := map[string][]*declNode{}
	bySel := map[string][]*declNode{}
	add := func(n *declNode, osNames map[string]bool, dotOS bool) {
		nodes = append(nodes, n)
		for _, b := range n.body {
			d, ids, sels := refs(b, osNames, dotOS)
			direct[n] = direct[n] || d
			for _, id := range ids {
				edges[n] = append(edges[n], "id:"+id)
			}
			for _, s := range sels {
				edges[n] = append(edges[n], "sel:"+s)
			}
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, files[name], 0)
		if err != nil {
			t.Fatal(err)
		}
		osNames, dotOS := map[string]bool{}, false
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "os" {
				switch {
				case imp.Name == nil:
					osNames["os"] = true
				case imp.Name.Name == ".":
					dotOS = true
				default:
					osNames[imp.Name.Name] = true
				}
			}
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Body == nil {
					continue
				}
				n := &declNode{file: name, kind: "func", name: d.Name.Name, exported: d.Name.IsExported(), body: []ast.Node{d.Body}}
				if d.Recv != nil {
					n.kind = "method"
					bySel[d.Name.Name] = append(bySel[d.Name.Name], n)
				} else {
					byIdent[d.Name.Name] = append(byIdent[d.Name.Name], n)
				}
				add(n, osNames, dotOS)
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, spec := range d.Specs {
					vs := spec.(*ast.ValueSpec)
					var body []ast.Node
					for _, v := range vs.Values {
						body = append(body, v)
					}
					for _, id := range vs.Names {
						n := &declNode{file: name, kind: "var", name: id.Name, exported: id.IsExported(), body: body}
						byIdent[id.Name] = append(byIdent[id.Name], n)
						add(n, osNames, dotOS)
					}
				}
			}
		}
	}

	// reaches walks the references depth first. A node already on the path
	// reaches nothing further from there, which is sound: a cycle through
	// an opener is found where the cycle first meets it.
	memo := map[*declNode]bool{}
	visiting := map[*declNode]bool{}
	var reaches func(n *declNode) bool
	reaches = func(n *declNode) bool {
		if v, ok := memo[n]; ok {
			return v
		}
		if visiting[n] {
			return false
		}
		visiting[n] = true
		defer delete(visiting, n)
		result := direct[n]
		for _, e := range edges[n] {
			if result {
				break
			}
			kind, name, _ := strings.Cut(e, ":")
			targets := byIdent[name]
			if kind == "sel" {
				targets = bySel[name]
			}
			for _, m := range targets {
				if reaches(m) {
					result = true
					break
				}
			}
		}
		memo[n] = result
		return result
	}
	for _, n := range nodes {
		if !n.exported {
			continue
		}
		checked++
		if reaches(n) {
			leaks = append(leaks, n.file+": "+n.kind+" "+n.name)
		}
	}
	return leaks, checked
}

// TestRubricHLD4_NoExportedNonoFunctionOpensAFile holds this package's split
// between a parser and an opener: Observe takes an already-open reader, and
// the function that calls os.Open (read) is unexported, so a caller that
// links this package only for its types -- report does, for Observation,
// and digest transitively through report -- has no exported path from that
// import to a sandbox trail's bytes. Opening the trail is the read step's
// own (internal/report/gather), which is why read has no exported twin.
//
// No exported function, method or package-level var can reach an opener,
// through any chain of references to what this package declares: see
// exportsReachingAnOpener.
func TestRubricHLD4_NoExportedNonoFunctionOpensAFile(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(b)
	}
	leaks, checked := exportsReachingAnOpener(t, files)
	if checked == 0 {
		t.Fatal("premise: no exported declaration was checked")
	}
	for _, l := range leaks {
		t.Errorf("%s can reach an os opener, directly or through this package; the file opener must stay unreachable from outside", l)
	}
}

// TestRubricHLD4_TheOpenerGuardSeesEveryWrapper: the guard above passed with
// `var ReadTrail = read` and an Observation method calling read added to
// this package, since it followed calls between free functions only. Each
// shape of wrapper below is a leak it must name, and the opener alone is
// not.
func TestRubricHLD4_TheOpenerGuardSeesEveryWrapper(t *testing.T) {
	const base = "package nono\n\nimport \"os\"\n\ntype Observation struct{}\n\ntype Window struct{}\n\n" +
		"func read(p string, w Window) Observation { f, _ := os.Open(p); _ = f; return Observation{} }\n\n" +
		"func Observe(p string) Observation { return Observation{} }\n"
	for _, tc := range []struct {
		name, src string
		leaks     bool
	}{
		{"the opener alone", base, false},
		{"a call wrapper", base + "func ReadPath(p string) Observation { return read(p, Window{}) }\n", true},
		{"a var naming the opener", base + "var ReadTrail = read\n", true},
		{"a method calling the opener", base + "func (o Observation) Load(p string) Observation { return read(p, Window{}) }\n", true},
		{"a method through an unexported one", base +
			"func (o Observation) load(p string) Observation { return read(p, Window{}) }\n" +
			"func (o Observation) Load(p string) Observation { return o.load(p) }\n", true},
		{"a func returning the opener", base + "func Opener() func(string, Window) Observation { return read }\n", true},
		{"a var holding a literal that calls it", base + "var Load = func(p string) Observation { return read(p, Window{}) }\n", true},
		{"an exported func through an unexported var", base + "var r = read\n\nfunc Get(p string) Observation { return r(p, Window{}) }\n", true},
		{"os under another name", "package nono\n\nimport o \"os\"\n\nfunc Slurp(p string) { _, _ = o.ReadFile(p) }\n", true},
		{"os dot-imported", "package nono\n\nimport . \"os\"\n\nfunc Slurp(p string) { _, _ = ReadFile(p) }\n", true},
	} {
		leaks, checked := exportsReachingAnOpener(t, map[string]string{"fixture.go": tc.src})
		if checked == 0 {
			t.Fatalf("%s: premise: no exported declaration was checked", tc.name)
		}
		if got := len(leaks) > 0; got != tc.leaks {
			t.Errorf("%s: leaks %v, want leaking %v", tc.name, leaks, tc.leaks)
		}
	}
}
