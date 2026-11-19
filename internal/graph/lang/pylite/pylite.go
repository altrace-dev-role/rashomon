// Package pylite is the reduced Python extractor: one file, read in
// process, no interpreter. It sees definitions and their signatures,
// module constants, imports, and calls by name with their argument count
// and literal keyword arguments. It resolves a callee to a symbol only
// when the name is defined in the same module or imported from a module in
// a known unit; anything else is left out rather than guessed. It is the
// Overlay half of lang.Resolver and, in this slice, also the Extract half.
package pylite

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Lang is the language namespace pylite writes under.
const Lang = "py"

// ErrParse is the refusal for content the reduced extractor cannot read:
// a bracket that does not balance, or a def with no colon. No records are
// written for the file. Overlay returns the refusal as an error that is
// ErrParse to errors.Is and carries the line it stopped at; Extract goes
// on past the file and names it among the graph.Refusals it returns.
var ErrParse = errors.New("pylite: content not parsed")

// parseError is one parse refusal: the 1-based line the reader stopped at
// and why, in words a person can act on. It is ErrParse to errors.Is.
type parseError struct {
	line int
	why  string
}

func (e parseError) Error() string { return fmt.Sprintf("%v: line %d: %s", ErrParse, e.line, e.why) }

// Is makes a parse refusal match ErrParse.
func (e parseError) Is(target error) bool { return target == ErrParse }

// Unit is one Python unit: the directory that holds it, and the prefix
// stripped from a file's path before it is read as a dotted module name.
type Unit struct {
	Path    string
	ModRoot string
}

// Resolver is the language's lang.Resolver. Modules maps a top-level
// module name to the unit path that holds it, so an import can be followed
// across units.
type Resolver struct {
	Units   []Unit
	Modules map[string]string
}

// Lang is "py".
func (Resolver) Lang() string { return Lang }

// Claims is every .py file.
func (Resolver) Claims(_, rel string) bool { return strings.HasSuffix(rel, ".py") }

// Unit is the id of the unit whose Path is rel's longest prefix.
func (r Resolver) Unit(_, rel string) (graph.ID, bool) {
	u, ok := r.unit(rel)
	if !ok {
		return "", false
	}
	return graph.UnitID(Lang, u.Path), true
}

// unit is rel's unit by longest Path prefix.
func (r Resolver) unit(rel string) (Unit, bool) {
	var best Unit
	found := false
	for _, u := range r.Units {
		if (rel == u.Path || strings.HasPrefix(rel, u.Path+"/")) && (!found || len(u.Path) > len(best.Path)) {
			best, found = u, true
		}
	}
	return best, found
}

// Extract reads each file under root and parses it at rev with the
// committed confidences: definitions and imports 1.0, calls 0.9. A file it
// cannot read, or whose content it refuses, is a gap and not the end of the
// index: Extract goes on, and returns the records of every other file with
// graph.Refusals as its error, one per such file in the order given (a read
// refusal has no line, and its reason is the read error's words without the
// path; a parse refusal has the line and the reason).
func (r Resolver) Extract(root string, files []string, rev graph.Rev) ([]graph.Record, error) {
	var out []graph.Record
	var refused graph.Refusals
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			refused = append(refused, graph.Refusal{Path: rel, Reason: readReason(err)})
			continue
		}
		recs, err := r.parse(rel, b, rev, 1.0, 0.9, true)
		var pe parseError
		switch {
		case errors.As(err, &pe):
			refused = append(refused, graph.Refusal{Path: rel, Line: pe.line, Reason: pe.why})
			continue
		case err != nil:
			return nil, err
		}
		out = append(out, recs...)
	}
	if len(refused) > 0 {
		return out, refused
	}
	return out, nil
}

// readReason is a read error's words without the path, which the refusal
// already names.
func readReason(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}

// Overlay parses one file's post-edit content at the overlay revision with
// the reduced confidences: definitions and imports 0.9, calls 0.7 (no
// cross-file resolution happened).
func (r Resolver) Overlay(rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
	return r.parse(rel, content, graph.RevOverlay(base.Commit, seq), 0.9, 0.7, false)
}

// OverlayCarried is exactly the predicates Overlay emits: declarations,
// imports, calls, the references of from-imports, and the reads and writes of
// SQL literals. Contains is index-time structure and is not among them.
func (Resolver) OverlayCarried() graph.Carried {
	return graph.Carried{graph.Declares, graph.Imports, graph.Calls, graph.References, graph.Reads, graph.Writes}
}

// module is rel's dotted module name within its unit.
func module(u Unit, rel string) string {
	m := strings.TrimSuffix(rel, ".py")
	if u.ModRoot != "" {
		m = strings.TrimPrefix(m, u.ModRoot+"/")
	}
	m = strings.TrimSuffix(m, "/__init__")
	return strings.ReplaceAll(m, "/", ".")
}

// parse is the one parser behind Extract and Overlay. Unit membership
// (the contains edge) is index-time structure an edit cannot change, so
// only a committed parse writes it; an overlay that re-wrote it would turn
// a certain edge uncertain.
func (r Resolver) parse(rel string, content []byte, rev graph.Rev, defConf, callConf float64, committed bool) ([]graph.Record, error) {
	u, ok := r.unit(rel)
	if !ok {
		return nil, nil
	}
	src := string(content)
	if err := balanced(src); err != nil {
		return nil, err
	}
	mod := module(u, rel)
	f, err := scan(src)
	if err != nil {
		return nil, err
	}
	file := graph.FileID(rel)
	algo := graph.Prov{Extractor: "pylite", Version: "1"}
	recs := []graph.Record{{Node: &graph.Node{ID: file, Kind: "file", Tier: graph.Algorithmic, Conf: 1.0, Rev: rev, Prov: algo,
		Attrs: map[string]string{"digest": graph.ContentDigest(content), "module": mod}}}}
	if committed {
		recs = append(recs, graph.Record{Edge: &graph.Edge{Src: graph.UnitID(Lang, u.Path), Pred: graph.Contains, Dst: file,
			Tier: graph.Algorithmic, Conf: 1.0, Rev: rev, Prov: algo}})
	}
	local := map[string]graph.ID{}
	for _, d := range f.defs {
		local[d.name] = graph.SymID(Lang, u.Path, "func", mod+"."+d.name)
	}
	for _, d := range f.defs {
		id := local[d.name]
		recs = append(recs,
			graph.Record{Node: &graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: defConf, Rev: rev, Prov: algo,
				Attrs: map[string]string{"symkind": "func", "sig": d.sig, "arity_min": strconv.Itoa(d.min), "arity_max": d.max, "line": strconv.Itoa(d.line)}}},
			graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: defConf, Rev: rev, Prov: algo}})
	}
	for _, c := range f.consts {
		id := graph.SymID(Lang, u.Path, "const", mod+"."+c.name)
		recs = append(recs,
			graph.Record{Node: &graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: defConf, Rev: rev, Prov: algo,
				Attrs: map[string]string{"symkind": "const", "value": c.value, "line": strconv.Itoa(c.line)}}},
			graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: defConf, Rev: rev, Prov: algo}})
	}
	for _, m := range f.importedModules() {
		if up, known := r.Modules[strings.SplitN(m, ".", 2)[0]]; known {
			recs = append(recs, graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Imports, Dst: graph.SymID(Lang, up, "mod", m),
				Tier: graph.Algorithmic, Conf: defConf, Rev: rev, Prov: algo}})
		}
	}
	recs = append(recs, r.extras(f, src, file, local, rev, defConf, algo)...)
	for _, c := range f.calls {
		callee, ok := r.callee(c.name, local, f.imports)
		if !ok {
			continue
		}
		attrs := map[string]string{"argc": strconv.Itoa(c.argc), "line": strconv.Itoa(c.line)}
		for k, v := range c.kw {
			attrs["kw."+k] = v
		}
		recs = append(recs, graph.Record{Edge: &graph.Edge{Src: local[c.caller], Pred: graph.Calls, Dst: callee,
			Tier: graph.Algorithmic, Conf: callConf, Rev: rev, Prov: algo, Attrs: attrs}})
	}
	return recs, nil
}

// callee is the symbol a call name resolves to: a def of this module, an
// imported name, or an attribute of an imported module in a known unit.
func (r Resolver) callee(name string, local map[string]graph.ID, imports map[string]importRef) (graph.ID, bool) {
	if id, ok := local[name]; ok {
		return id, true
	}
	if im, ok := imports[name]; ok && im.attr != "" {
		return r.symIn(im.module, im.attr)
	}
	head, attr, dotted := strings.Cut(name, ".")
	if !dotted {
		return "", false
	}
	if im, ok := imports[head]; ok && im.attr == "" && !strings.Contains(attr, ".") {
		return r.symIn(im.module, attr)
	}
	return "", false
}

// symIn is the func symbol attr of module mod, when mod's unit is known.
func (r Resolver) symIn(mod, attr string) (graph.ID, bool) {
	up, known := r.Modules[strings.SplitN(mod, ".", 2)[0]]
	if !known {
		return "", false
	}
	return graph.SymID(Lang, up, "func", mod+"."+attr), true
}

// balanced is nil when brackets balance over src outside string literals,
// and otherwise the parse refusal: a closer with no opener at the line it
// is on, or, at the end of the file, the outermost opener still open at
// the line it opened (the last line depth rose from zero).
func balanced(src string) error {
	depth, line, opened := 0, 1, 0
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c == '\n' {
			line++
		}
		switch {
		case quote != 0:
			if c == '\\' {
				i++
				if i < len(src) && src[i] == '\n' {
					line++
				}
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '#':
			for i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		case c == '(' || c == '[' || c == '{':
			if depth == 0 {
				opened = line
			}
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
			if depth < 0 {
				return parseError{line: line, why: "a bracket closed at this line was not opened"}
			}
		}
	}
	if depth > 0 {
		return parseError{line: opened, why: "a bracket opened at this line is not closed"}
	}
	return nil
}

// extras are the records beyond defs, consts, imports and calls: a
// references edge from the file to every name a from-import brings in
// from a known unit (a const when the name is upper case, a function
// otherwise: the lite extractor's one guess), and the table nodes with
// the reads and writes edges of the SQL literals in the file.
func (r Resolver) extras(f fileScan, src string, file graph.ID, local map[string]graph.ID, rev graph.Rev, conf float64, algo graph.Prov) []graph.Record {
	var recs []graph.Record
	for _, im := range f.imports {
		if im.attr == "" {
			continue
		}
		up, known := r.Modules[strings.SplitN(im.module, ".", 2)[0]]
		if !known {
			continue
		}
		kind := "func"
		if strings.ToUpper(im.attr) == im.attr {
			kind = "const"
		}
		recs = append(recs, graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.References, Dst: graph.SymID(Lang, up, kind, im.module+"."+im.attr),
			Tier: graph.Algorithmic, Conf: conf, Rev: rev, Prov: algo}})
	}
	for _, q := range scanSQL(src, f.defs) {
		tbl := graph.TableID(q.table)
		src := file
		if q.caller != "" {
			src = local[q.caller]
		}
		pred := graph.Reads
		if q.write {
			pred = graph.Writes
		}
		recs = append(recs,
			graph.Record{Node: &graph.Node{ID: tbl, Kind: "table", Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: algo,
				Attrs: map[string]string{"name": q.table}}},
			graph.Record{Edge: &graph.Edge{Src: src, Pred: pred, Dst: tbl, Tier: graph.Algorithmic, Conf: 0.9, Rev: rev, Prov: algo,
				Attrs: map[string]string{"line": strconv.Itoa(q.line), "cols": strings.Join(q.cols, ",")}}})
	}
	return recs
}
