package verify

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// Whether a run tested a package (card PL-WIRE1). A run of the gate's runner
// meets a platform row only where its own words show it covered the package:
// `go test ./internal/b` after an edit under internal/a is a run of the
// runner and no test of a. The record does not hold a run's words: the hook
// records the line's shape, its runner's keyed digest and where it ran
// (hook/handle.go fills cwd, project, file_path and written_paths, never the
// command). So a run's words are known only by that digest: coverOf derives,
// as the hook derived the run's, the runner digest of each line that names
// the package from the directory the run's location names (candidateLines),
// and a match is a cover. A run whose words are none of those may still have
// covered the package with words the list does not hold (`go test -race
// -count=1 ./...`), so it is triMaybe: never a cover, never a miss. The one
// certain miss is a run in a directory above the package's module, which no
// pattern of its own reaches into.

// coverFlags are the flags candidateLines puts before a pattern, one at
// most: go test's own (TestPLW1n_ holds each to `go help`), the ones a test
// run commonly carries, each of which leaves the package's tests run whole.
// A slice of constants, laid out with no code run at init (RES-2).
var coverFlags = []string{"", "-v", "-race", "-count=1", "-short", "-cover", "-failfast", "-json"}

// coverOf is whether run r covers f: placed at a directory of f's own or an
// ancestor's within f's module, with runner words one of candidateLines'.
// The digests are derived once per directory and family in a judge.
func (p *platformJudge) coverOf(r *gateRun, f PlatformFamily) tri {
	dir, placed := p.place(r.Location, f)
	switch {
	case !placed || r.Runner == nil:
		return triMaybe
	case !within(dir, f.ModuleDir):
		return triNo
	}
	key := dir + "\x00" + f.Dir
	if p.digests == nil {
		p.digests = map[string]map[string]bool{}
	}
	ds, ok := p.digests[key]
	if !ok {
		ds = map[string]bool{}
		for _, argv := range candidateLines(dir, f.Dir) {
			if s := lineShape(argv, p.key); s.RunnerDigest != nil {
				ds[*s.RunnerDigest] = true
			}
		}
		p.digests[key] = ds
	}
	if ds[*r.Runner] {
		return triYes
	}
	return triMaybe
}

// candidateLines is every go test line, as argv, that run at dir names the
// package at pkgDir (dir is pkgDir or an ancestor of it, both
// project-relative): ./... ; at pkgDir itself no pattern and "."; below it,
// ./d/... for each directory from dir's child down to pkgDir, and ./pkgDir
// relative to dir; each bare or after one of coverFlags.
func candidateLines(dir, pkgDir string) [][]string {
	pats := []string{"./..."}
	if dir == pkgDir {
		pats = append(pats, "", ".")
	} else {
		rel := pkgDir
		if dir != "." {
			rel = strings.TrimPrefix(pkgDir, dir+"/")
		}
		parts := strings.Split(rel, "/")
		for i := range parts {
			pats = append(pats, "./"+strings.Join(parts[:i+1], "/")+"/...")
		}
		pats = append(pats, "./"+rel)
	}
	out := make([][]string, 0, len(coverFlags)*len(pats))
	for _, fl := range coverFlags {
		for _, pat := range pats {
			argv := []string{"go", "test"}
			if fl != "" {
				argv = append(argv, fl)
			}
			if pat != "" {
				argv = append(argv, pat)
			}
			out = append(out, argv)
		}
	}
	return out
}

// place is the directory, among f's and its ancestors' up to the project
// root, whose location digest (shape.CWDDigest under the install key, as the
// hook digested the run's) is loc; ok false where none is, or loc or the
// root is not known.
func (p *platformJudge) place(loc string, f PlatformFamily) (string, bool) {
	root := p.snap.ProjectRoot
	if loc == "" || root == "" || len(p.key) == 0 {
		return "", false
	}
	for d := f.Dir; ; d = path.Dir(d) {
		if shape.CWDDigest(p.key, filepath.Join(root, filepath.FromSlash(d))) == loc {
			return d, true
		}
		if d == "." || d == "/" {
			return "", false
		}
	}
}

// within reports whether d is base or below it, both project-relative and
// clean, "." being the root.
func within(d, base string) bool {
	return base == "." || d == base || strings.HasPrefix(d, base+"/")
}
