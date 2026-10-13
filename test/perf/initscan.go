package perf

import (
	"go/ast"
	"go/token"
	"path"
	"sort"
	"strings"
)

// RES-2's static half: no package linked into the binary compiles a regexp
// while the process initialises. The scan starts at every package-level var
// initialiser and every init function and follows what they call, because a
// compile is as eager behind a helper as it is written inline (the spec's own
// mutant is `var rules = compileRules()`). Calls are resolved by name, without
// type information: a function of the same package (its name, or under a dot
// import of a scanned package), a function of another scanned package (under
// whatever name the file imports it as), and any method of that name declared
// in a scanned package, which over-approximates and so errs towards a finding.
// A function literal is walked wherever initialisation meets it, called or not,
// for the same reason.
//
// A function handed to sync.OnceFunc, OnceValue or OnceValues runs on first
// use, not at init, so it is followed only where initialisation calls the Once
// result. A call through a function value is followed in exactly these cases:
// the Once result called inline (`sync.OnceValue(f)()`), and a call whose
// target names a package-level var (of this package, a dot import, or an
// imported scanned package) initialised with a Once call, a function name or a
// function literal. Any other function value (a local var, a field, a
// parameter, a var assigned in init) is not followed.

// SrcPackage is one scanned package: its import path and its parsed files.
type SrcPackage struct {
	Path  string
	Files []*ast.File
}

// InitCompile is one regexp compile initialisation reaches; Via names the
// path to it from the var or init that starts it.
type InitCompile struct {
	Pos token.Pos
	Via string
}

// regexpCompiles are the regexp functions that compile a pattern.
var regexpCompiles = map[string]bool{
	"Compile": true, "MustCompile": true, "CompilePOSIX": true, "MustCompilePOSIX": true,
	"Match": true, "MatchString": true, "MatchReader": true,
}

// syncDefers are the sync functions that run their argument on first use.
var syncDefers = map[string]bool{"OnceFunc": true, "OnceValue": true, "OnceValues": true}

type scanFunc struct {
	decl *ast.FuncDecl
	file *fileScope
	pkg  *scanPkg
}

// scanVar is a package-level var and the value it is initialised with.
type scanVar struct {
	name  *ast.Ident
	value ast.Expr
	file  *fileScope
	pkg   *scanPkg
}

type scanPkg struct {
	funcs   map[string][]*scanFunc
	methods map[string][]*scanFunc
	vars    map[string][]*scanVar
	files   []*fileScope
}

// fileScope is a file's imports: local name to path, and its dot imports.
type fileScope struct {
	file    *ast.File
	imports map[string]string
	dots    []string
}

type initScan struct {
	byPath map[string]*scanPkg
	seen   map[ast.Node]bool
	at     map[token.Pos]bool
	found  []InitCompile
}

// RegexpCompilesAtInit finds every regexp compile that initialising pkgs
// reaches, sorted by position.
func RegexpCompilesAtInit(pkgs []SrcPackage) []InitCompile {
	s := &initScan{byPath: map[string]*scanPkg{}, seen: map[ast.Node]bool{}, at: map[token.Pos]bool{}}
	names := map[string]string{}
	for _, p := range pkgs {
		if len(p.Files) > 0 {
			names[p.Path] = p.Files[0].Name.Name
		}
	}
	for _, p := range pkgs {
		s.byPath[p.Path] = s.index(p, names)
	}
	for _, p := range pkgs {
		for _, fs := range s.byPath[p.Path].files {
			s.roots(fs, s.byPath[p.Path])
		}
	}
	sort.Slice(s.found, func(i, j int) bool { return s.found[i].Pos < s.found[j].Pos })
	return s.found
}

func (s *initScan) index(p SrcPackage, names map[string]string) *scanPkg {
	sp := &scanPkg{funcs: map[string][]*scanFunc{}, methods: map[string][]*scanFunc{}, vars: map[string][]*scanVar{}}
	for _, f := range p.Files {
		fs := &fileScope{file: f, imports: map[string]string{}}
		for _, is := range f.Imports {
			ip := strings.Trim(is.Path.Value, "\"`")
			name, ok := names[ip]
			if !ok {
				name = path.Base(ip)
			}
			switch {
			case is.Name == nil:
				fs.imports[name] = ip
			case is.Name.Name == ".":
				fs.dots = append(fs.dots, ip)
			case is.Name.Name != "_":
				fs.imports[is.Name.Name] = ip
			}
		}
		sp.files = append(sp.files, fs)
		for _, d := range f.Decls {
			sp.add(d, fs)
		}
	}
	return sp
}

// add records a file's function, method or package-level var declaration.
func (sp *scanPkg) add(d ast.Decl, fs *fileScope) {
	switch d := d.(type) {
	case *ast.FuncDecl:
		fn := &scanFunc{decl: d, file: fs, pkg: sp}
		if d.Recv == nil {
			sp.funcs[d.Name.Name] = append(sp.funcs[d.Name.Name], fn)
		} else {
			sp.methods[d.Name.Name] = append(sp.methods[d.Name.Name], fn)
		}
	case *ast.GenDecl:
		if d.Tok != token.VAR {
			return
		}
		for _, spec := range d.Specs {
			vs := spec.(*ast.ValueSpec)
			if len(vs.Values) != len(vs.Names) {
				continue
			}
			for i, n := range vs.Names {
				sp.vars[n.Name] = append(sp.vars[n.Name], &scanVar{name: n, value: vs.Values[i], file: fs, pkg: sp})
			}
		}
	}
}

// roots walks a file's package-level var initialisers and init functions.
func (s *initScan) roots(fs *fileScope, pkg *scanPkg) {
	for _, d := range fs.file.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				continue
			}
			for _, spec := range d.Specs {
				vs := spec.(*ast.ValueSpec)
				var names []string
				for _, n := range vs.Names {
					names = append(names, n.Name)
				}
				for _, v := range vs.Values {
					s.walk(v, fs, pkg, "var "+strings.Join(names, ", "))
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.Name == "init" && d.Body != nil {
				s.walk(d.Body, fs, pkg, "init")
			}
		}
	}
}

func (s *initScan) walk(n ast.Node, fs *fileScope, pkg *scanPkg, via string) {
	ast.Inspect(n, func(x ast.Node) bool {
		call, ok := x.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fs.calls(call.Fun, "sync", syncDefers) {
			for _, a := range call.Args {
				if _, lit := a.(*ast.FuncLit); !lit {
					s.walk(a, fs, pkg, via)
				}
			}
			return false
		}
		if fs.calls(call.Fun, "regexp", regexpCompiles) {
			if !s.at[call.Pos()] {
				s.at[call.Pos()] = true
				s.found = append(s.found, InitCompile{Pos: call.Pos(), Via: via})
			}
			return true
		}
		s.invoke(call.Fun, fs, pkg, via)
		return true
	})
}

// invoke walks what calling fun runs: a function literal's body, a function
// a sync.Once* call defers (the Once result is being called), the functions
// and methods fun names, and the value of a package var fun names.
func (s *initScan) invoke(fun ast.Expr, fs *fileScope, pkg *scanPkg, via string) {
	switch f := unwrap(fun).(type) {
	case *ast.FuncLit:
		if !s.seen[f] {
			s.seen[f] = true
			s.walk(f.Body, fs, pkg, via+" -> func literal")
		}
		return
	case *ast.CallExpr:
		if fs.calls(f.Fun, "sync", syncDefers) {
			for _, a := range f.Args {
				s.invoke(a, fs, pkg, via)
			}
		}
		return
	}
	fns, vars := s.resolve(fun, fs, pkg)
	for _, fn := range fns {
		if s.seen[fn.decl] || fn.decl.Body == nil {
			continue
		}
		s.seen[fn.decl] = true
		s.walk(fn.decl.Body, fn.file, fn.pkg, via+" -> "+fn.decl.Name.Name)
	}
	for _, v := range vars {
		if !s.seen[v.name] {
			s.seen[v.name] = true
			s.invoke(v.value, v.file, v.pkg, via+" -> "+v.name.Name)
		}
	}
}

// calls reports whether fun names one of names from the package at path,
// through this file's name for it or a dot import.
func (fs *fileScope) calls(fun ast.Expr, pkgPath string, names map[string]bool) bool {
	switch f := unwrap(fun).(type) {
	case *ast.SelectorExpr:
		id, ok := f.X.(*ast.Ident)
		return ok && fs.imports[id.Name] == pkgPath && names[f.Sel.Name]
	case *ast.Ident:
		if !names[f.Name] {
			return false
		}
		for _, d := range fs.dots {
			if d == pkgPath {
				return true
			}
		}
	}
	return false
}

// resolve lists what a call's target may name: the scanned functions and
// methods it may run, and the package-level vars whose value it may call.
func (s *initScan) resolve(fun ast.Expr, fs *fileScope, pkg *scanPkg) (fns []*scanFunc, vars []*scanVar) {
	switch f := unwrap(fun).(type) {
	case *ast.Ident:
		fns = append(fns, pkg.funcs[f.Name]...)
		vars = append(vars, pkg.vars[f.Name]...)
		for _, d := range fs.dots {
			if p := s.byPath[d]; p != nil {
				fns = append(fns, p.funcs[f.Name]...)
				vars = append(vars, p.vars[f.Name]...)
			}
		}
	case *ast.SelectorExpr:
		if id, ok := f.X.(*ast.Ident); ok {
			if ip, imported := fs.imports[id.Name]; imported {
				if p := s.byPath[ip]; p != nil {
					return p.funcs[f.Sel.Name], p.vars[f.Sel.Name]
				}
				return nil, nil
			}
		}
		for _, p := range s.byPath {
			fns = append(fns, p.methods[f.Sel.Name]...)
		}
	}
	return fns, vars
}

// unwrap strips parentheses and generic instantiation from a call's target.
func unwrap(e ast.Expr) ast.Expr {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		default:
			return e
		}
	}
}
