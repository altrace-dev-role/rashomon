package verify

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// Whether a go test run tested a package (card PL-WIRE1). The record does not
// hold a run's words: the hook records the line's shape (a keyed digest of
// the whole line), its runner's keyed digest and where it ran, never the
// command (hook/handle.go fills cwd, project, file_path and written_paths).
// The runner's digest leaves out what stands ahead of the runner, so
// `GOFLAGS=-run=X go test ./...`, `export GOFLAGS=...; go test ./...` and
// `timeout 60 go test ./...` share it with `go test ./...` (M3). The whole
// line's digest does not: coverOf derives, as the hook derived the run's, the
// whole-line digest of each line that names the package from the directory
// the run's location digest places it in (coverLines), and only an exact
// match is a cover. Any other go test run may have tested the package with
// words the record cannot show, so it reads coverMaybe: never a cover, never
// a miss. The one certain miss is a run in a directory above the package's
// module, which no pattern of its own reaches into.

// cover is how a go test run stands to one package.
type cover uint8

const (
	coverNo    cover = iota // ran where no pattern of its own reaches the package
	coverMaybe              // its words are none the record can match
	coverMany               // its line names the package among others: ./..., ./d/...
	coverAlone              // its line names the package alone: go test, ., ./pkg
)

// coverFlags are the flags coverLines puts before a pattern, one at most:
// go test's own (TestPLW1n_ holds each to `go help`), the ones a test run
// commonly carries, none of which runs fewer tests: not -run, -skip, -list,
// -short (testing.Short lets a test skip itself), -c or -n. A slice of
// constants, laid out with no code run at init (RES-2).
var coverFlags = []string{"", "-v", "-race", "-count=1", "-cover", "-failfast", "-json"}

// coverOf is how run r, a go test run, stands to f: placed at a directory of
// f's own or an ancestor's within f's module, with a whole line one of
// coverLines' for that directory.
func (p *platformJudge) coverOf(r *gateRun, f PlatformFamily) cover {
	dir, placed := p.place(r.Location, f)
	switch {
	case !placed:
		return coverMaybe
	case !within(dir, f.ModuleDir):
		return coverNo
	}
	d := p.declaration(r.Seq)
	if d == nil {
		return coverMaybe
	}
	key := dir + "\x00" + f.Dir
	if p.lines == nil {
		p.lines = map[string]map[string]cover{}
	}
	ds, ok := p.lines[key]
	if !ok {
		ds = map[string]cover{}
		for _, l := range coverLines(dir, f.Dir) {
			ds[lineShape(l.argv, p.key).Digest] = l.cover
		}
		p.lines[key] = ds
	}
	if c, ok := ds[d.Shape.Digest]; ok {
		return c
	}
	return coverMaybe
}

// coverLine is one go test line, as argv, and how it names the package.
type coverLine struct {
	argv  []string
	cover cover
}

// coverLines is every go test line that, run at dir (pkgDir or an ancestor of
// it, both project-relative), names the package at pkgDir: alone, as no
// pattern or "." at pkgDir itself and ./pkgDir from above; or among others,
// as ./... and as ./d/... for each directory from dir's child down to pkgDir.
// Each is bare or after one of coverFlags.
func coverLines(dir, pkgDir string) []coverLine {
	type pat struct {
		word  string
		cover cover
	}
	pats := []pat{{"./...", coverMany}}
	if dir == pkgDir {
		pats = append(pats, pat{"", coverAlone}, pat{".", coverAlone})
	} else {
		rel := pkgDir
		if dir != "." {
			rel = strings.TrimPrefix(pkgDir, dir+"/")
		}
		parts := strings.Split(rel, "/")
		for i := range parts {
			pats = append(pats, pat{"./" + strings.Join(parts[:i+1], "/") + "/...", coverMany})
		}
		pats = append(pats, pat{"./" + rel, coverAlone})
	}
	out := make([]coverLine, 0, len(coverFlags)*len(pats))
	for _, fl := range coverFlags {
		for _, pt := range pats {
			argv := []string{"go", "test"}
			if fl != "" {
				argv = append(argv, fl)
			}
			if pt.word != "" {
				argv = append(argv, pt.word)
			}
			out = append(out, coverLine{argv, pt.cover})
		}
	}
	return out
}

// place is the directory, among f's and its ancestors' up to the project
// root, whose location digest (shape.CWDDigest under the install key, as the
// hook digested the run's) is loc; ok false where none is, or loc or the
// root is not known. Each directory's digest is derived once per judge.
func (p *platformJudge) place(loc string, f PlatformFamily) (string, bool) {
	root := p.snap.ProjectRoot
	if loc == "" || root == "" || len(p.key) == 0 {
		return "", false
	}
	if p.locs == nil {
		p.locs = map[string]string{}
	}
	for d := f.Dir; ; d = path.Dir(d) {
		dg, ok := p.locs[d]
		if !ok {
			dg = shape.CWDDigest(p.key, filepath.Join(root, filepath.FromSlash(d)))
			p.locs[d] = dg
		}
		if dg == loc {
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
