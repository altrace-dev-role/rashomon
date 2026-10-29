package platform

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// goMod is what one directory's go.mod says for packageOf: whether the
// directory holds one, and the module path its first module directive names
// ("" for none).
type goMod struct {
	found bool
	path  string
}

// packageOf is the import path of the Go package a written file belongs to,
// by Go's own rule (D132): one directory, one package, its import path the
// module path of the nearest go.mod at or above the file's directory joined
// with the directory relative to that go.mod. The walk up stops at root. A
// file under a directory the go tool ignores (testdata, or a name starting
// with . or _) is in no package; a _test.go file counts for its directory's
// package, the conservative reading. It is "" when the path leaves root, when
// no go.mod is found at or under root, or when the nearest go.mod names no
// module: that directory is in a module the go tool cannot build, not in the
// one above it. mods caches each directory's go.mod for one call.
func packageOf(root, file string, mods map[string]goMod) string {
	rel := file
	if filepath.IsAbs(file) {
		r, err := filepath.Rel(root, file)
		if err != nil {
			return ""
		}
		rel = r
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return ""
	}
	dir := path.Dir(rel)
	for _, e := range strings.Split(dir, "/") {
		if e == "testdata" || e != "." && (strings.HasPrefix(e, ".") || strings.HasPrefix(e, "_")) {
			return ""
		}
	}
	for d := dir; ; d = path.Dir(d) {
		if m := moduleAt(root, d, mods); m.found {
			switch {
			case m.path == "":
				return ""
			case d == dir:
				return m.path
			case d == ".":
				return m.path + "/" + dir
			}
			return m.path + "/" + strings.TrimPrefix(dir, d+"/")
		}
		if d == "." {
			return ""
		}
	}
}

// moduleAt is what the go.mod in dir (relative to root) says, cached in mods.
func moduleAt(root, dir string, mods map[string]goMod) goMod {
	if m, ok := mods[dir]; ok {
		return m
	}
	m := readModule(filepath.Join(root, filepath.FromSlash(dir), "go.mod"))
	mods[dir] = m
	return m
}

// readModule reads the go.mod at p for its first module directive: `module
// PATH` or `module "PATH"`, comments and blank lines skipped. replace, vendor
// and the block form are not read (D132). A go.mod that exists but cannot be
// read names no module.
func readModule(p string) goMod {
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return goMod{}
	}
	if err != nil {
		return goMod{found: true}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		mod := strings.TrimSpace(rest)
		if u, err := strconv.Unquote(mod); err == nil {
			mod = u
		}
		if mod == "(" {
			return goMod{found: true}
		}
		return goMod{found: true, path: mod}
	}
	return goMod{found: true}
}
