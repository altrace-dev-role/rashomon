package lang

import "slices"

// Inferrer is the optional half of a language that lets `rashomon graph
// index` write a configuration for a repository that has none: the names of
// the manifest files that mark one of its projects, and the rule that reads
// its units from the repository's files. Infer reads no file: files is every
// regular file the index's walk found under the root, as slash paths
// relative to it and sorted, with the walk's skips already applied, so the
// rule and Build skip the same directories.
type Inferrer interface {
	Manifests() []string
	Infer(files []string) Inferred
}

// Inferred is what one language's rule found under a root: its units, and
// each path it left out with why.
type Inferred struct {
	Units []Unit
	Left  []Left
}

// Unit is one inferred unit: its path, the directory its modules start at,
// and the top-level module names it owns, as a configuration declares a
// unit; and the manifest and the words of the rule it came from, for the
// line that names it.
type Unit struct {
	Path, ModRoot string
	Modules       []string
	From, Rule    string
}

// Left is a path a rule left out, and why, for a person to read and then
// add by hand if it is a unit.
type Left struct{ Path, Why string }

// The kinds of reason a rule leaves a path out for, in words that name no
// language, so a person reads one set of reasons whatever the language: a
// rule's Why is one of them, then, where it adds something, what the rule
// checked in its own language's terms, in parentheses.
const (
	// LeftNested is a project whose manifest lies inside another project's
	// directory; a workspace file's list of members is not read.
	LeftNested = "a project inside another project"
	// LeftLooseModule is a source file directly at the modules' root, in no
	// package.
	LeftLooseModule = "a module file at the modules' root, not in a package"
	// LeftNotPackage is a directory at the modules' root that holds source
	// but not what makes a directory a package in its language; the rule
	// reads no manifest, so it does not say why.
	LeftNotPackage = "a directory of source that is not a package"
	// LeftOutsideRoot is source in a project but outside the directory its
	// modules start at.
	LeftOutsideRoot = "source outside the modules' root"
)

// Inferrers is the registered resolvers that are Inferrers, in the order
// they were registered, each language once.
func (g *Registry) Inferrers() []Inferrer {
	var out []Inferrer
	var seen []string
	for _, e := range g.es {
		i, ok := e.r.(Inferrer)
		if l := e.r.Lang(); ok && !slices.Contains(seen, l) {
			out = append(out, i)
			seen = append(seen, l)
		}
	}
	return out
}
