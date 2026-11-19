package gopg

import (
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// Manifests is the file that marks a Go module.
func (Resolver) Manifests() []string { return []string{"go.mod"} }

// Infer is Go's units under a root with no configuration: each go.mod not
// under a testdata directory or a directory whose name starts with _ (the
// go tool's own rule; the index's walk already skips dot directories) is a
// unit at its directory, which is also where its modules start; it names
// no module, as a hand-written Go unit names none. A nested module is a unit
// of its own, as the go tool treats it as a module of its own.
func (Resolver) Infer(files []string) lang.Inferred {
	var got lang.Inferred
	for _, f := range files {
		dir := path.Dir(f)
		if path.Base(f) != "go.mod" || skippedByGo(dir) {
			continue
		}
		got.Units = append(got.Units, lang.Unit{Path: dir, ModRoot: dir, Modules: []string{}, From: f, Rule: modRule})
	}
	slices.SortFunc(got.Units, func(a, b lang.Unit) int { return strings.Compare(a.Path, b.Path) })
	return got
}

// modRule is the words of Go's rule, for the line that names a unit.
const modRule = "a go.mod is a module, a unit at its directory"

// skippedByGo reports whether the go tool leaves a module at dir, a slash
// path, out of ./...: a component named testdata or starting with _.
func skippedByGo(dir string) bool {
	for _, c := range strings.Split(dir, "/") {
		if c == "testdata" || strings.HasPrefix(c, "_") {
			return true
		}
	}
	return false
}
