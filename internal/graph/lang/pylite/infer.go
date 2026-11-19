package pylite

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// Manifests is the files that mark a Python project.
func (Resolver) Manifests() []string { return []string{"pyproject.toml", "setup.cfg", "setup.py"} }

// Infer is Python's units under a root with no configuration, by the layout
// alone (no manifest is parsed): a directory holding a manifest is a
// project; a project inside another is left out, named. A project's modules
// start at its src directory when that holds a package (a directory with an
// __init__.py), else at the project's own directory; each package there is
// a unit owning the module of its name. A top-level module file and a
// directory of .py files with no __init__.py there are left out, named, for
// a person to add by hand, and so, when the modules start at src, is each
// .py file and each directory of them outside it. Each reason is one of
// package lang's kinds, then what this rule checked.
func (r Resolver) Infer(files []string) lang.Inferred {
	ms := r.Manifests()
	from := map[string]string{} // a project's directory: its first manifest
	for _, f := range files {
		d := path.Dir(f)
		if slices.Contains(ms, path.Base(f)) && (from[d] == "" || f < from[d]) {
			from[d] = f
		}
	}
	owned := map[string][]string{} // a project's directory: the files it owns
	for _, f := range files {
		if p, ok := projectAt(path.Dir(f), from); ok {
			owned[p] = append(owned[p], f)
		}
	}
	var got lang.Inferred
	for p, m := range from {
		if _, ok := projectAt(path.Dir(p), from); ok && p != "." {
			got.Left = append(got.Left, lang.Left{Path: p, Why: lang.LeftNested})
			continue
		}
		inferProject(p, m, owned[p], &got)
	}
	slices.SortFunc(got.Units, func(a, b lang.Unit) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(got.Left, func(a, b lang.Left) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Why, b.Why))
	})
	return got
}

// projectAt is the project whose directory is dir or the nearest one above
// it, false for none: the project a file in dir belongs to, so a file in a
// project inside another belongs to the inner one alone.
func projectAt(dir string, from map[string]string) (string, bool) {
	for {
		if _, ok := from[dir]; ok {
			return dir, true
		}
		if dir == "." || dir == "/" {
			return "", false
		}
		dir = path.Dir(dir)
	}
}

// What this rule checked, in Python's terms, for the reasons whose kind
// alone would not say it.
const (
	notPackageChecked  = " (.py files under it and no __init__.py directly in it)"
	outsideDirChecked  = " (.py files in it, and the project's modules start at src)"
	outsideFileChecked = " (a .py file, and the project's modules start at src)"
)

// inferProject adds to got the units of the project at p, whose first
// manifest is from, out of files, the files it owns: each package directly
// under its modules' root is a unit; a module file and a directory of .py
// files that is no package there are left out, and so, when the modules
// start at src, is each .py file and each directory of them outside it.
func inferProject(p, from string, files []string, got *lang.Inferred) {
	rels := make([]string, 0, len(files))
	src := false
	for _, f := range files {
		rel := f
		if p != "." {
			rel = strings.TrimPrefix(f, p+"/")
		}
		rels = append(rels, rel)
		parts := strings.Split(rel, "/")
		src = src || len(parts) == 3 && parts[0] == "src" && parts[2] == "__init__.py"
	}
	modRoot, rule := p, "a package (a directory holding __init__.py) at the project's directory"
	if src {
		modRoot, rule = path.Join(p, "src"), "a package (a directory holding __init__.py) in the project's src directory"
	}
	pkgs, pyDirs := map[string]bool{}, map[string]bool{}
	outside := map[string]bool{} // the first name of each .py path outside src: true for a directory
	for _, rel := range rels {
		py := strings.HasSuffix(rel, ".py")
		if src {
			var ok bool
			if rel, ok = strings.CutPrefix(rel, "src/"); !ok {
				if first, _, dir := strings.Cut(rel, "/"); py && rel != "setup.py" {
					outside[first] = dir
				}
				continue
			}
		}
		parts := strings.Split(rel, "/")
		switch {
		case len(parts) == 1 && py && (src || rel != "setup.py"):
			got.Left = append(got.Left, lang.Left{Path: path.Join(modRoot, rel), Why: lang.LeftLooseModule})
		case len(parts) == 2 && parts[1] == "__init__.py":
			pkgs[parts[0]] = true
		case len(parts) >= 2 && py:
			pyDirs[parts[0]] = true
		}
	}
	for d := range pkgs {
		got.Units = append(got.Units, lang.Unit{Path: path.Join(modRoot, d), ModRoot: modRoot, Modules: []string{d}, From: from, Rule: rule})
	}
	for d := range pyDirs {
		if !pkgs[d] {
			got.Left = append(got.Left, lang.Left{Path: path.Join(modRoot, d), Why: lang.LeftNotPackage + notPackageChecked})
		}
	}
	for name, dir := range outside {
		why := outsideFileChecked
		if dir {
			why = outsideDirChecked
		}
		got.Left = append(got.Left, lang.Left{Path: path.Join(p, name), Why: lang.LeftOutsideRoot + why})
	}
}
