package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// Report is what an import kept and dropped, one entry per source.
type Report struct {
	Sources []SourceReport
}

// SourceReport counts one source's rows: those with a clause under the
// approved-text rules, those title only (with a note naming the refusal),
// the lines that started as a row and did not parse, the applies_to ids
// resolved against the index and the path-like words it did not hold.
type SourceReport struct {
	File, Written                                         string
	Rows, Clauses, TitleOnly, Unparsed, Resolved, Dropped int
	Notes                                                 []string
}

// String is the report as the verb prints it.
func (r Report) String() string {
	var b strings.Builder
	for _, s := range r.Sources {
		fmt.Fprintf(&b, "%s: %d row(s), %d with a clause, %d title only, %d unparsed; applies_to %d resolved, %d path(s) not in the index -> %s\n",
			s.File, s.Rows, s.Clauses, s.TitleOnly, s.Unparsed, s.Resolved, s.Dropped, s.Written)
		for _, n := range s.Notes {
			fmt.Fprintf(&b, "  %s\n", n)
		}
	}
	return b.String()
}

// Import reads each source (a ledger file, a notes file with decision
// lines, or a directory of decision records), writes its rows under
// root/Dir as <name>.json and reports what it kept: each row's clause
// under the approved-text rules or title only, and the paths the row
// names that the index g holds, as global ids under rid, or does not. The
// bytes written depend on the sources and the index alone, so a second
// import changes nothing.
func Import(root string, sources []string, g *graph.Store, rid string, rev graph.Rev) (Report, error) {
	idx := newResolver(g, rev)
	var rep Report
	for _, src := range sources {
		sr, err := importOne(root, src, idx, rid)
		if err != nil {
			return Report{}, err
		}
		rep.Sources = append(rep.Sources, sr)
	}
	return rep, nil
}

func importOne(root, src string, idx *resolver, rid string) (SourceReport, error) {
	rows, unparsed, err := parse(src)
	if err != nil {
		return SourceReport{}, err
	}
	sr := SourceReport{File: src, Rows: len(rows), Unparsed: unparsed}
	for i := range rows {
		r := &rows[i]
		c, err := clause(r.Clause)
		if err != nil {
			sr.TitleOnly++
			sr.Notes = append(sr.Notes, fmt.Sprintf("%s: clause refused, title only: %v", r.ID, err))
			c = ""
		} else {
			sr.Clauses++
		}
		r.Clause = c
		var dropped int
		r.AppliesTo, dropped = idx.resolve(r.Title+" "+r.Text, rid)
		refs, missed := idx.resolveRefs(r.Evidence, rid)
		r.AppliesTo = union(r.AppliesTo, refs)
		sr.Resolved += len(r.AppliesTo)
		sr.Dropped += dropped + missed
	}
	sr.Written, err = WriteFile(root, fileName(src), rows)
	return sr, err
}

// parse reads src by its shape: a directory is decision records; a JSON
// object with a rows list is a truth table; a file with at least one
// ledger row is a ledger; any other file holds decision lines (a prose
// line that starts with a row id does not make a ledger).
func parse(src string) ([]Row, int, error) {
	info, err := os.Stat(src)
	if err != nil {
		return nil, 0, err
	}
	if info.IsDir() {
		return ParseADR(src)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return nil, 0, err
	}
	sum := sha256.Sum256(b)
	file := filepath.Base(src)
	rows, unparsed := ParseLedger(file, b)
	if len(rows) == 0 {
		if tt, ok := ParseTruthTable(file, b); ok {
			rows = tt
		} else {
			rows, unparsed = ParseDecisions(file, b)
		}
	}
	for i := range rows {
		rows[i].Source.Digest = hex.EncodeToString(sum[:])
	}
	return rows, unparsed, nil
}

// fileName is the source's base name without its extension, lower case,
// any run of other characters one hyphen.
func fileName(src string) string {
	base := filepath.Base(src)
	if !isDir(src) {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	var b strings.Builder
	dash := false
	for _, c := range strings.ToLower(base) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// clauseCharset is the approved-text alphabet (slot.Approved).
const clauseCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 ,.;:()_/+='-"

// Clause is raw as a brief may carry it: bytes outside the approved-text
// alphabet become spaces, double quotes, backticks and non-ASCII bytes
// are dropped, a quote that is no apostrophe is dropped, spaces collapse,
// and the result is cut at a sentence or word boundary within 160 bytes;
// "" when the rules still refuse it (a banned word, a URL scheme, a
// secret-shaped word).
func Clause(raw string) string {
	c, err := clause(raw)
	if err != nil {
		return ""
	}
	return c
}

func clause(raw string) (string, error) {
	b := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case strings.IndexByte(clauseCharset, c) >= 0:
			b = append(b, c)
		case c < 0x80 && c != '"' && c != '`':
			b = append(b, ' ')
		}
	}
	keep := make([]byte, 0, len(b))
	for i, c := range b {
		if c == '\'' && (i == 0 || i == len(b)-1 || !alnum(b[i-1]) || !alnum(b[i+1])) {
			continue
		}
		keep = append(keep, c)
	}
	s := cut(strings.Join(strings.Fields(string(keep)), " "), 160)
	if s == "" {
		return "", errors.New("no text")
	}
	return s, slot.Approved(s).Validate()
}

func alnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// cut is s within max bytes: at its last sentence end past 40 bytes, else
// its last space, with trailing punctuation dropped.
func cut(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if i := strings.LastIndex(s[:max], ". "); i >= 40 {
		return s[:i+1]
	}
	if j := strings.LastIndexByte(s[:max], ' '); j > 0 {
		s = s[:j]
	} else {
		s = s[:max]
	}
	return strings.TrimRight(s, " ,;:-(")
}

// resolver is the index's files by relative path and by base name, its
// units by path, and its symbols by declaring file and short name
// ("internal/platform/read.go#Read"): the targets a row's words and a
// truth table's evidence can name.
type resolver struct {
	files map[string]graph.ID
	bases map[string][]graph.ID
	units map[string]graph.ID
	syms  map[string][]graph.ID
}

func newResolver(g *graph.Store, rev graph.Rev) *resolver {
	r := &resolver{files: map[string]graph.ID{}, bases: map[string][]graph.ID{}, units: map[string]graph.ID{}, syms: map[string][]graph.ID{}}
	for _, n := range g.Nodes(rev) {
		switch n.Kind {
		case "file":
			rel := strings.TrimPrefix(string(n.ID), "file:")
			r.files[rel] = n.ID
			r.bases[path.Base(rel)] = append(r.bases[path.Base(rel)], n.ID)
		case "unit":
			if _, p, ok := strings.Cut(strings.TrimPrefix(string(n.ID), "unit:"), ":"); ok {
				r.units[p] = n.ID
			}
		case "sym":
			for _, e := range g.In(n.ID, rev, graph.Declares) {
				key := strings.TrimPrefix(string(e.Src), "file:") + "#" + shortName(n.ID)
				r.syms[key] = append(r.syms[key], n.ID)
			}
		}
	}
	return r
}

// shortName is a symbol's own name: the last dotted segment of its id's
// last field (auth.sessions.is_expired is is_expired; Read is Read).
func shortName(id graph.ID) string {
	s := string(id)
	s = s[strings.LastIndexByte(s, ':')+1:]
	return s[strings.LastIndexByte(s, '.')+1:]
}

// resolveRefs is the global ids of evidence references, "file" or
// "file#symbol", that the index holds, sorted and unique, and the count
// it does not: a symbol reference applies to the symbol and to its file,
// so a reader keyed by either finds the row.
func (r *resolver) resolveRefs(refs []string, rid string) ([]string, int) {
	ids := map[graph.ID]bool{}
	missed := 0
	for _, ref := range refs {
		file, sym, hasSym := strings.Cut(ref, "#")
		found := false
		if id, ok := r.files[file]; ok {
			ids[id], found = true, true
		}
		for _, id := range r.syms[file+"#"+sym] {
			ids[id], found = true, true
		}
		if !found || hasSym && len(r.syms[file+"#"+sym]) == 0 {
			missed++
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, string(graph.GlobalID(rid, id)))
	}
	sort.Strings(out)
	return out, missed
}

// union is a and b sorted and unique.
func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// resolve is the global ids, sorted and unique, of the paths text names
// that the index holds: a file by its relative path, a unit by its path,
// or a file by a base name that only one file carries; and the count of
// distinct path-like words the index does not hold.
func (r *resolver) resolve(text, rid string) ([]string, int) {
	ids := map[graph.ID]bool{}
	dropped := map[string]bool{}
	for _, w := range strings.FieldsFunc(text, isDelim) {
		p, ok := pathWord(w)
		if !ok {
			continue
		}
		if id, ok := r.lookup(p); ok {
			ids[id] = true
		} else {
			dropped[p] = true
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, string(graph.GlobalID(rid, id)))
	}
	sort.Strings(out)
	return out, len(dropped)
}

func (r *resolver) lookup(p string) (graph.ID, bool) {
	if id, ok := r.files[p]; ok {
		return id, true
	}
	if id, ok := r.units[p]; ok {
		return id, true
	}
	if ids := r.bases[p]; len(ids) == 1 && !strings.Contains(p, "/") {
		return ids[0], true
	}
	return "", false
}

func isDelim(c rune) bool { return unicode.IsSpace(c) || strings.ContainsRune(",;()[]{}<>\"'`|", c) }

// pathWord is w as a path: a :line suffix and trailing punctuation
// dropped, a leading ./ dropped; it holds a letter and either a slash or
// an extension of one to five letters.
func pathWord(w string) (string, bool) {
	w = strings.TrimRight(w, ".,;:")
	if i := strings.LastIndexByte(w, ':'); i > 0 && strings.Trim(w[i+1:], "0123456789-") == "" {
		w = w[:i]
	}
	w = strings.TrimPrefix(w, "./")
	letter := false
	for i := 0; i < len(w); i++ {
		if unicode.IsLetter(rune(w[i])) {
			letter = true
			break
		}
	}
	if !letter {
		return "", false
	}
	if strings.Contains(w, "/") {
		return w, true
	}
	ext := path.Ext(w)
	if len(ext) < 2 || len(ext) > 6 || strings.Trim(ext[1:], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return "", false
	}
	return w, true
}
