package gopg

import (
	"bytes"
	"go/ast"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// MaxOverlayImports bounds the import specs one file's mapping reads, on the
// index and in an overlay alike (RES-29): past it a file's imports say less,
// never wrong.
const MaxOverlayImports = 64

// MaxGoModBytes bounds ReadModulePath's read of the root's go.mod. A module
// directive past the bound, or cut by it, is no module path.
const MaxGoModBytes = 16 << 10

// ImportMap is what a file's imports are mapped under: the root module's
// path and whether a directory under the root exists. Overlay reads nothing
// itself, so its caller supplies both (ReadModulePath, DirUnder), as Extract
// does for the index's edges. A nil map, or one with no module or no IsDir,
// maps no import.
type ImportMap struct {
	Module string
	IsDir  func(dir string) bool
}

// ImportUnit is the one mapping from an import path to a Go unit, shared by
// the index (Extract) and the overlay. The module path itself is the root's
// package directory "."; a path under it by a slash is the directory the
// rest names. Anything else maps to nothing: the standard library, another
// module, and a nested module whose path does not follow its directory. A
// nested module whose path does follow it (pgraph, the module path plus
// "/pgraph") maps by the same rule. The rest must be a clean relative path,
// and isDir must report it a directory; isDir is asked about such a
// candidate only. go.mod's requires are never read: the edge is the import
// line's, so it stands before the build reports a missing require.
func ImportUnit(module, importPath string, isDir func(dir string) bool) (graph.ID, bool) {
	dir, ok := importDir(module, importPath)
	if !ok || !isDir(dir) {
		return "", false
	}
	return graph.UnitID(Lang, dir), true
}

// importDir is importPath's candidate directory under module, before the
// existence check.
func importDir(module, importPath string) (string, bool) {
	if module == "" || importPath == "" {
		return "", false
	}
	if importPath == module {
		return ".", true
	}
	rest, ok := strings.CutPrefix(importPath, module+"/")
	if !ok || rest == "" || rest == "." || rest == ".." || strings.HasPrefix(rest, "/") ||
		strings.HasPrefix(rest, "../") || path.Clean(rest) != rest {
		return "", false
	}
	return rest, true
}

// fileImports is one parsed file's Imports records under m: one per distinct
// mapped unit other than the file's own, over at most MaxOverlayImports
// specs, with isDir asked once per directory. A _test.go file gives none: the
// index holds no test file, since pgraph loads ./... without tests.
func fileImports(f *ast.File, rel string, m ImportMap, edge func(dst graph.ID) graph.Record) []graph.Record {
	if m.Module == "" || m.IsDir == nil || strings.HasSuffix(rel, "_test.go") {
		return nil
	}
	own := graph.UnitID(Lang, path.Dir(rel))
	known := map[string]bool{}
	isDir := func(dir string) bool {
		v, ok := known[dir]
		if !ok {
			v = m.IsDir(dir)
			known[dir] = v
		}
		return v
	}
	seen := map[graph.ID]bool{}
	var out []graph.Record
	for i, spec := range f.Imports {
		if i == MaxOverlayImports {
			break
		}
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		u, ok := ImportUnit(m.Module, p, isDir)
		if !ok || u == own || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, edge(u))
	}
	return out
}

// ModulePath is the path of the module directive in a go.mod's bytes: the
// first line, comment stripped, whose first word is module, with the path
// bare, quoted or back-quoted; "" with none or a malformed one.
func ModulePath(gomod []byte) string {
	for line := range bytes.Lines(gomod) {
		s := string(line)
		if i := strings.Index(s, "//"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
		rest, ok := strings.CutPrefix(s, "module")
		if !ok || rest == "" {
			if s == "module" {
				return ""
			}
			continue
		}
		switch rest[0] {
		case ' ', '\t', '"', '`':
		default:
			continue
		}
		rest = strings.TrimSpace(rest)
		var p string
		if rest[0] == '"' || rest[0] == '`' {
			u, err := strconv.Unquote(rest)
			if err != nil {
				return ""
			}
			p = u
		} else {
			fs := strings.Fields(rest)
			if len(fs) != 1 {
				return ""
			}
			p = fs[0]
		}
		if p == "" || strings.ContainsAny(p, " \t\"`()") {
			return ""
		}
		return p
	}
	return ""
}

// ReadModulePath is the module path of root's go.mod, read through
// MaxGoModBytes, complete lines only; "" without one. The hook calls it on
// the overlay path and Extract on the index: one read of one known file,
// never a walk.
func ReadModulePath(root string) string {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, MaxGoModBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return ""
	}
	b := buf[:n]
	if n == MaxGoModBytes {
		i := bytes.LastIndexByte(b, '\n')
		if i < 0 {
			return ""
		}
		b = b[:i+1]
	}
	return ModulePath(b)
}

// DirUnder is the existence check both sides give ImportUnit: dir, a slash
// path under root, is a directory, and its last element is not a link.
func DirUnder(root string) func(dir string) bool {
	return func(dir string) bool {
		fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(dir)))
		return err == nil && fi.IsDir()
	}
}
