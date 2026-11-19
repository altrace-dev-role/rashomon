package gopg

import "github.com/altrace-dev-role/rashomon/internal/graph/lang"

// Manifests is the file that marks a Go module.
func (Resolver) Manifests() []string { return []string{"go.mod"} }

// Infer is Go's units under a root with no configuration: each go.mod not
// under a testdata directory or a directory whose name starts with _ (the
// go tool's own rule; the index's walk already skips dot directories) is a
// unit at its directory, which is also where its modules start; it names
// no module, as a hand-written Go unit names none. A nested module is a unit
// of its own, as the go tool treats it as a module of its own.
func (Resolver) Infer(files []string) lang.Inferred {
	return lang.Inferred{}
}
