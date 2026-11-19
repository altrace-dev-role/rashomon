package lang

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

// Inferrers is the registered resolvers that are Inferrers, in the order
// they were registered, each language once.
func (g *Registry) Inferrers() []Inferrer {
	return nil
}
