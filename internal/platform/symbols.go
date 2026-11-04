package platform

import (
	"bytes"
	"context"
	"path"
	"slices"
	"strings"
)

// The bounds of one Symbols read: the names it looks up, and the packages
// one name may be declared in and still be given (past it, which one the
// request meant is a guess).
const (
	maxNames = 4
	maxDecls = 2
)

// Symbol is what the log proves of one name a request named (PW1b-ON-230
// step 3, WED-SLICE row 1(c)): the symbol's spelling after its package (F,
// T, or T.M for a method), the directory of the package that declares it,
// relative to the root, and its direct callers, the distinct PROVEN calls
// records whose object it is, with the packages they are in.
type Symbol struct {
	Name           string
	Dir            string
	Callers        int
	CallerPackages int
}

// Symbols is what the log at root proves of the first maxNames of names, in
// their order, a name declared in two packages once for each, by
// directory. It reads only the pair the walk file vouches for: with no walk
// file, or one that names another log or other digests, it is nil, since a
// line built from it goes to the model (the Stop's Read, whose lines go to
// the person, takes a pair with no walk file). A name is matched after its
// package, bare or with the package's last element (F, a.F, T.M), and only
// PROVEN facts count: a calls record of another tier, a may-call or a
// retracted call is no caller. A symbol is given only when the digests
// vouch for every package its answer rests on, the declaring one and each
// caller's, as fresh does for the Stop: a caller count from a package
// edited since the sync is a guess. Callers in a package that had none at
// the sync are not seen; that is the read's one blind spot. Any error, ctx
// done included, is nil: unknown, never a partial answer.
func Symbols(ctx context.Context, root string, names []string) []Symbol {
	if root == "" || len(names) == 0 {
		return nil
	}
	names = names[:min(len(names), maxNames)]
	vouched, err := readVouch(root)
	if err != nil || !vouched.on {
		return nil
	}
	hook := recordHook
	live, err := inWorker(ctx, func() (map[fact]struct{}, error) {
		return readLog(ctx, LogPath(root), MaxLogBytes, vouched.log, hook, keepNamed(names))
	})
	if err != nil {
		return nil
	}
	dg, err := readDigests(digestsPath(root))
	if err != nil || dg.sum != vouched.digests {
		return nil
	}
	decls := map[string][]string{}
	callers := map[string][]string{}
	for f := range live {
		switch {
		case f.tier != tierProven:
		case f.predicate == predDeclares:
			for _, n := range names {
				if named([]byte(f.object), n) && !slices.Contains(decls[n], f.object) {
					decls[n] = append(decls[n], f.object)
				}
			}
		case f.predicate == predCalls:
			if !slices.Contains(callers[f.object], f.subject) {
				callers[f.object] = append(callers[f.object], f.subject)
			}
		}
	}
	fr := freshness{root: root, dg: dg, known: map[string]bool{}}
	var out []Symbol
	for _, n := range names {
		ids := decls[n]
		if len(ids) == 0 || len(ids) > maxDecls {
			continue
		}
		var given []Symbol
		for _, id := range ids {
			if s, ok := fr.symbol(id, callers[id]); ok {
				given = append(given, s)
			}
		}
		slices.SortFunc(given, func(a, b Symbol) int { return strings.Compare(a.Dir, b.Dir) })
		out = append(out, given...)
	}
	return out
}

// freshness is one Symbols read's fresh answers, each package hashed once
// and every package under the one maxHashed count.
type freshness struct {
	root   string
	dg     *digests
	known  map[string]bool
	hashed int
}

func (fr *freshness) fresh(pkg string) bool {
	if ok, seen := fr.known[pkg]; seen {
		return ok
	}
	ok := fresh(fr.root, fr.dg, pkg, nil, &fr.hashed)
	fr.known[pkg] = ok
	return ok
}

// symbol is id's Symbol from its callers' subjects, or false when its
// package, or a caller's, is not one the digests list, or not fresh.
func (fr *freshness) symbol(id string, subjects []string) (Symbol, bool) {
	pkg := packageIn(fr.dg, id)
	if pkg == "" || !fr.fresh(pkg) {
		return Symbol{}, false
	}
	pkgs := map[string]bool{}
	for _, s := range subjects {
		p := packageIn(fr.dg, s)
		if p == "" || !fr.fresh(p) {
			return Symbol{}, false
		}
		pkgs[p] = true
	}
	_, rest, _ := strings.Cut(id, ":")
	return Symbol{Name: rest[len(pkg)+1:], Dir: path.Dir(fr.dg.byPkg[pkg][0]), Callers: len(subjects), CallerPackages: len(pkgs)}, true
}

// packageIn is the package of a pgraph id (kind, ':', the package's path,
// '.', the rest): the one package the digests list that the id's path
// starts with, then a dot, whose rest has its kind's shape (fitsKind). ""
// for none, and for two: a guess is never a name. pgraph's spelling cannot
// be cut at a fixed dot, since a package path's last element may hold one
// (gopkg.in/yaml.v3), and the longest prefix is wrong where a/b declares a
// type c and a/b.c is a package too (method:a/b.c.X is a/b's).
func packageIn(dg *digests, id string) string {
	kind, rest, ok := strings.Cut(id, ":")
	if !ok {
		return ""
	}
	found := ""
	for i := len(rest) - 1; i > 0; i-- {
		if rest[i] != '.' {
			continue
		}
		if _, listed := dg.byPkg[rest[:i]]; !listed || !fitsKind(kind, rest[i+1:]) {
			continue
		}
		if found != "" {
			return ""
		}
		found = rest[:i]
	}
	return found
}

// fitsKind reports whether rest, an id's spelling after its package, has
// kind's shape in pgraph's ids (pgraph/identity.go): a function, variable
// or constant is one name, with a fingerprint ("#", "~") that holds no
// dot; a type is one name, or Owner$Name for a type declared in a function,
// whose owner may be a method (T.M$Name); a method is its receiver's
// spelling, then '.', then its name.
func fitsKind(kind, rest string) bool {
	switch kind {
	case "func", "var", "const":
		return !strings.Contains(rest, ".")
	case "type":
		owner, _, local := strings.Cut(rest, "$")
		return !strings.Contains(rest, ".") || local && strings.Count(owner, ".") == 1
	case "method":
		return strings.Contains(rest, ".")
	}
	return false
}

// symbolKinds are the id kinds a request can name: pgraph's spelling of a
// function, a method, a type, a variable and a constant (pgraph/identity.go
// id), never a package or the object: fallback.
var symbolKinds = [...]string{"func", "method", "type", "var", "const"}

// named reports whether object, a pgraph id, is a symbol spelled name after
// its package: its path ends in name after a '.' (F, T.M) or a '/' (a.F,
// the package's last element and the symbol). It allocates nothing, as
// decode's keep must not.
func named(object []byte, name string) bool {
	i := bytes.IndexByte(object, ':')
	if i < 0 {
		return false
	}
	kind, rest := object[:i], object[i+1:]
	symbol := false
	for _, k := range symbolKinds {
		symbol = symbol || string(kind) == k
	}
	if !symbol {
		return false
	}
	j := len(rest) - len(name)
	return j > 0 && string(rest[j:]) == name && (rest[j-1] == '.' || rest[j-1] == '/')
}

// keepNamed is decode's keep for names: a record whose object is a symbol a
// name spells, which holds both the record that declares it and each that
// calls it. The subject is not read.
func keepNamed(names []string) func(subject, object []byte) bool {
	return func(_, object []byte) bool {
		for _, n := range names {
			if named(object, n) {
				return true
			}
		}
		return false
	}
}
