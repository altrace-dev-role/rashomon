package pylite

import "github.com/altrace-dev-role/rashomon/internal/graph/lang"

// Manifests is the files that mark a Python project.
func (Resolver) Manifests() []string { return []string{"pyproject.toml", "setup.cfg", "setup.py"} }

// Infer is Python's units under a root with no configuration, by the layout
// alone (no manifest is parsed): a directory holding a manifest is a
// project; a project inside another is left out, named. A project's modules
// start at its src directory when that holds a package (a directory with an
// __init__.py), else at the project's own directory; each package there is
// a unit owning the module of its name. A top-level module file and a
// namespace package (a directory of .py files with no __init__.py) there are
// left out, named, for a person to add by hand.
func (Resolver) Infer(files []string) lang.Inferred {
	return lang.Inferred{}
}
