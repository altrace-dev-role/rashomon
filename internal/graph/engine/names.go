package engine

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// R0 exact names (W-M1.13): a prompt word in name form claims the top of
// the ladder before any phrase is weighed. A path the index holds names
// its file and unit; a path it lacks names the unit of its directory,
// the gap disclosed; a qualified name (unit.symbol) resolves inside the
// unit it names; an identifier (an underscore, a case change, or a
// capitalised word mid-sentence that is no English word of the lists)
// names its symbol, and one that several units declare resolves by the
// rest of the prompt's words against each or stands disclosed; a
// knowledge id (req:, les:, dec:) names its record; a scope tag ("the
// hook package") names the unit whose last path segment it is. Words are
// read in prompt order and the first that resolves to one unit is the
// claim; the rest are disclosed. A word inside a constraint clause ("do
// not touch X") names what to leave alone and claims nothing. A path
// form the index has no unit for, when nothing resolves, refuses the
// ladder: the graph cannot own a file it has not indexed, and a phrase
// claim over the leftover words would be a false positive (readiness B:
// no false-positive guidance). Nothing is matched by similarity.

// ClassNamed is the class of a claim a name made.
const ClassNamed = "named"

// The kinds of a named word.
const (
	NamedPath      = "path"
	NamedQualified = "qualified"
	NamedIdent     = "identifier"
	NamedKnowledge = "knowledge"
	NamedTag       = "tag"
)

// NamedWord is one prompt word in name form: the word as written, its
// kind, the unit it resolves to and the node (a file, a symbol or a
// knowledge record) when the index holds one, else why it resolved to
// nothing.
type NamedWord struct {
	Word string
	Kind string
	Unit string
	Node graph.ID
	Why  string
	// Refuses is a word that names where the change goes and that the
	// index has no node for: the ladder must not claim another unit for
	// it (a place in go.mod, the README's section).
	Refuses bool
}

// Named is R0's reading of the prompt: every word in name form in prompt
// order; the first unit a path, name or tag resolved to, with its node;
// the knowledge ids named, with the one unit their evidence lies in when
// there is one; and the path forms that resolved to nothing.
type Named struct {
	Words         []NamedWord
	Unit          string
	Node          graph.ID
	Knowledge     []graph.ID
	KnowledgeUnit string
	Unknown       []NamedWord
}

// Names reads the prompt's named words against the corpus. toks and
// weights are the prompt's tokens and weights, for an identifier several
// units declare.
func Names(g Reader, c corpus, prompt string, toks []string, weights map[string]float64) Named {
	var n Named
	seen := map[string]bool{}
	for _, sentence := range sentencesOf(prompt) {
		quoted := quotedWords(sentence)
		for ci, cl := range clausesOf(sentence) {
			if excludedClause(cl) {
				continue
			}
			for i := 0; i < len(cl); i++ {
				w := cleanWord(cl[i])
				if w == "" {
					continue
				}
				prev := ""
				if i > 0 {
					prev = low(cl[i-1])
				}
				next := ""
				if i+1 < len(cl) {
					next = low(cl[i+1])
				}
				nw, ok := c.nameOf(g, w, wordAt{prev: prev, next: next, initial: ci == 0 && i == 0, quoted: quoted[w]}, toks, weights)
				if !ok || seen[nw.Kind+" "+nw.Word] {
					continue
				}
				seen[nw.Kind+" "+nw.Word] = true
				n.add(g, c, nw)
			}
		}
	}
	return n
}

// wordAt is a word's place in its clause: the words beside it, whether
// it opens the sentence, and whether the prompt set it in code font.
type wordAt struct {
	prev, next      string
	initial, quoted bool
}

// quotedWords is the words the sentence wraps in backticks: the code
// font a prompt uses to name a thing in the code.
func quotedWords(sentence string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range strings.Fields(sentence) {
		raw = strings.TrimRight(raw, ".:,;")
		if len(raw) > 2 && raw[0] == '`' && raw[len(raw)-1] == '`' {
			out[cleanWord(raw)] = true
		}
	}
	return out
}

// cleanWord is the word without the punctuation that wraps or ends it.
func cleanWord(w string) string {
	w = strings.TrimLeft(w, "(\"'`[{<")
	return strings.TrimRight(w, ".:,;)\"'`]}>")
}

func (n *Named) add(g Reader, c corpus, w NamedWord) {
	n.Words = append(n.Words, w)
	switch {
	case w.Kind == NamedKnowledge:
		n.Knowledge = append(n.Knowledge, w.Node)
		if u := c.evidenceUnit(g, w.Node); n.KnowledgeUnit == "" || n.KnowledgeUnit == u {
			n.KnowledgeUnit = u
		} else {
			n.KnowledgeUnit = "?"
		}
	case w.Unit != "" && n.Unit == "":
		n.Unit, n.Node = w.Unit, w.Node
	case w.Unit == "" && (w.Kind == NamedPath || w.Refuses):
		n.Unknown = append(n.Unknown, w)
	}
}

// excludedClause is a clause that names what to leave alone.
func excludedClause(cl []string) bool {
	if len(cl) == 0 {
		return false
	}
	head := low(cl[0])
	return verbClass.get()[head] == "keep" || excludeWords.get()[head] || (head == "do" && len(cl) > 1 && low(cl[1]) == "not")
}

var excludeWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"don't": true, "dont": true, "never": true, "without": true, "not": true, "avoid": true, "except": true, "excluding": true, "skip": true, "ignore": true}
}}

var tagWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"package": true, "pkg": true, "module": true, "unit": true, "directory": true, "dir": true, "folder": true, "subsystem": true}
}}

// nameOf reads the word as a name, in the order path, qualified,
// knowledge id, scope tag, identifier, test name, owner.
func (c corpus) nameOf(g Reader, w string, at wordAt, toks []string, weights map[string]float64) (NamedWord, bool) {
	if placeWords.get()[at.prev] && fileLike(w) {
		nw := c.resolvePath(w)
		nw.Refuses = nw.Unit == "" && nw.Node == ""
		return nw, true
	}
	if c.pathForm(w) {
		return c.resolvePath(w), true
	}
	if l, r, ok := qualifiedForm(w); ok {
		if nw, ok := c.resolveQualified(w, l, r); ok {
			return nw, true
		}
	}
	if knowledgeForm(w) {
		for _, kind := range []string{"req:", "les:", "dec:"} {
			if _, ok := g.Node(graph.ID(kind+w), c.rev); ok {
				return NamedWord{Word: w, Kind: NamedKnowledge, Node: graph.ID(kind + w)}, true
			}
		}
	}
	if tagWords.get()[at.next] || tagWords.get()[at.prev] {
		return c.resolveTag(w), true
	}
	if identForm(w, at) {
		if nw, ok := c.resolveIdent(w, at, toks, weights); ok {
			return nw, true
		}
	}
	if nw, ok := c.resolveTest(w); ok {
		return nw, true
	}
	return c.resolveOwner(w)
}

// fileLike is a bare file name of a kind the index need not hold: a
// lower-case base and a short extension (go.mod, package.json), after a
// place word ("in go.mod").
func fileLike(w string) bool {
	base, ext, ok := strings.Cut(w, ".")
	if !ok || len(base) < 2 || len(ext) < 2 || len(ext) > 4 || strings.Contains(ext, ".") {
		return false
	}
	for i := 0; i < len(w); i++ {
		c := w[i]
		if !(c == '.' || c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return true
}

// resolveTest reads a test's name as the unit it tests: TestInstall_Events
// is a test of install.
func (c corpus) resolveTest(w string) (NamedWord, bool) {
	toks := Tokens(w)
	if !identChars(w) || len(toks) < 2 || toks[0] != "test" {
		return NamedWord{}, false
	}
	unit, ok := c.unitBySegment(toks[1])
	if !ok {
		return NamedWord{}, false
	}
	return NamedWord{Word: w, Kind: NamedIdent, Unit: unit, Node: c.unitID[unit], Why: "a test of " + unit}, true
}

// resolveOwner reads a possessive in capitals (the README's section, the
// CLI's flags) as the thing it names: a symbol, a file or a unit, else
// a place the index lacks, which refuses the ladder.
func (c corpus) resolveOwner(w string) (NamedWord, bool) {
	base, ok := possessive(w)
	if !ok || len(base) < 2 {
		return NamedWord{}, false
	}
	for i := 0; i < len(base); i++ {
		if c := base[i]; !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return NamedWord{}, false
		}
	}
	nw := NamedWord{Word: w, Kind: NamedIdent}
	switch {
	case len(c.syms[base]) > 0:
		nw.Node = c.syms[base][0]
		nw.Unit = c.units[c.index[nw.Node]]
	case len(c.bases[base]) == 1:
		nw.Node = c.bases[base][0]
		nw.Unit = c.units[c.index[nw.Node]]
	default:
		if unit, ok := c.unitBySegment(base); ok {
			nw.Unit, nw.Node = unit, c.unitID[unit]
		} else {
			nw.Why, nw.Refuses = base+" is not in the index", true
		}
	}
	return nw, true
}

// pathForm is a word with a path separator or an extension the index
// holds files of: the corpus, not a list, says what a source file is.
func (c corpus) pathForm(w string) bool {
	if strings.Contains(w, "/") {
		return !strings.Contains(w, "://")
	}
	return c.exts[path.Ext(lineless(w))]
}

// lineless is the word without a trailing line (and column) number:
// stop.go:214 is stop.go.
func lineless(w string) string {
	for {
		i := strings.LastIndex(w, ":")
		if i < 0 {
			return w
		}
		if _, err := strconv.Atoi(w[i+1:]); err != nil {
			return w
		}
		w = w[:i]
	}
}

// resolvePath is the word as a path: the file the index holds, or a
// unit it names, read at every suffix (a module prefix or a pasted
// absolute path drops away), else the unit of its directory, else for a
// bare file name the one file or unit of that base name.
func (c corpus) resolvePath(word string) NamedWord {
	nw := NamedWord{Word: word, Kind: NamedPath}
	w := strings.TrimPrefix(word, "./")
	if i := strings.Index(w, "("); i >= 0 {
		w = w[:i]
	}
	w = lineless(w)
	if strings.Contains(w, "/") {
		if id, unit, ok := c.fileAt(w); ok {
			nw.Node, nw.Unit = id, unit
			if unit == "" {
				nw.Why = word + " is in no unit"
			}
			return nw
		}
		if unit, ok := c.unitAt(w); ok {
			nw.Node, nw.Unit = c.unitID[unit], unit
			return nw
		}
		if base := path.Base(w); strings.Contains(base, ".") && !c.exts[path.Ext(base)] {
			dot := strings.LastIndex(base, ".")
			if unit, ok := c.unitAt(path.Join(path.Dir(w), base[:dot])); ok {
				nw.Kind, nw.Unit = NamedQualified, unit
				if sym, ok := c.symIn(base[dot+1:], unit); ok {
					nw.Node = sym
				} else {
					nw.Why = base[dot+1:] + " is not declared in " + unit
				}
				return nw
			}
		}
		if unit, ok := c.unitAt(path.Dir(w)); ok {
			nw.Unit, nw.Why = unit, word+" is not in the index; its directory is the unit"
			return nw
		}
		nw.Why = word + " is not in the index"
		return nw
	}
	base := strings.TrimSuffix(strings.TrimSuffix(w, path.Ext(w)), "_test")
	if ids := c.bases[base]; len(ids) == 1 {
		nw.Node, nw.Unit = ids[0], c.units[c.index[ids[0]]]
		if nw.Unit == "" {
			nw.Why = word + " is in no unit"
		}
		return nw
	}
	if unit, ok := c.unitBySegment(base); ok {
		nw.Unit, nw.Why = unit, word+" is not in the index; its name is the unit's"
		return nw
	}
	nw.Why = word + " is not in the index"
	return nw
}

// fileAt is the file node at p or at any suffix of p, with its unit.
func (c corpus) fileAt(p string) (graph.ID, string, bool) {
	segs := strings.Split(p, "/")
	for i := range segs {
		sub := strings.Join(segs[i:], "/")
		if id, ok := c.files[sub]; ok {
			return id, c.units[c.index[id]], true
		}
	}
	return "", "", false
}

// unitAt is the unit whose path is p or any suffix of p.
func (c corpus) unitAt(p string) (string, bool) {
	segs := strings.Split(p, "/")
	for i := range segs {
		sub := strings.Join(segs[i:], "/")
		if _, ok := c.unitID[sub]; ok && sub != "" {
			return sub, true
		}
	}
	return "", false
}

// unitBySegment is the one unit whose last path segment is seg, else the
// one unit with seg anywhere in its path; a segment matches as written
// or as tokens, so a plural or a case meets its segment.
func (c corpus) unitBySegment(seg string) (string, bool) {
	seg, _ = possessive(seg)
	seg = low(seg)
	var last, any []string
	for u := range c.unitID {
		parts := strings.Split(u, "/")
		if segmentIs(parts[len(parts)-1], seg) {
			last = append(last, u)
		} else if hasSegment(parts, seg) {
			any = append(any, u)
		}
	}
	if len(last) == 1 {
		return last[0], true
	}
	if len(last) == 0 && len(any) == 1 {
		return any[0], true
	}
	return "", false
}

func hasSegment(parts []string, s string) bool {
	for _, p := range parts {
		if segmentIs(p, s) {
			return true
		}
	}
	return false
}

// segmentIs is whether a path segment is s, as written or by tokens.
func segmentIs(part, s string) bool {
	if low(part) == s {
		return true
	}
	a, b := Tokens(part), Tokens(s)
	return len(a) > 0 && strings.Join(a, " ") == strings.Join(b, " ")
}

// resolveTag is the word before or after a tag word as a unit.
func (c corpus) resolveTag(word string) NamedWord {
	nw := NamedWord{Word: word, Kind: NamedTag}
	if unit, ok := c.unitBySegment(word); ok {
		nw.Unit = unit
	} else {
		nw.Why = "no one unit has the segment " + low(word)
	}
	return nw
}

// qualifiedForm is a word of two identifier parts joined by one dot.
func qualifiedForm(w string) (string, string, bool) {
	l, r, ok := strings.Cut(w, ".")
	if !ok || l == "" || r == "" || strings.Contains(r, ".") || !identChars(l) || !identChars(r) {
		return "", "", false
	}
	return l, r, true
}

// resolveQualified reads unit.symbol or Type.method: the left part a
// unit's last segment or a declared type, the right a name inside it.
func (c corpus) resolveQualified(word, l, r string) (NamedWord, bool) {
	nw := NamedWord{Word: word, Kind: NamedQualified}
	unit, ok := c.unitBySegment(l)
	if !ok {
		if syms := c.syms[l]; len(syms) > 0 {
			unit, ok = c.units[c.index[syms[0]]], true
		}
	}
	if !ok {
		return nw, false
	}
	nw.Unit = unit
	if sym, ok := c.symIn(r, unit); ok {
		nw.Node = sym
	} else {
		nw.Why = r + " is not declared in " + unit
	}
	return nw, true
}

// symIn is the first symbol named name inside unit.
func (c corpus) symIn(name, unit string) (graph.ID, bool) {
	for _, id := range c.syms[name] {
		if c.units[c.index[id]] == unit {
			return id, true
		}
	}
	return "", false
}

// knowledgeForm is a word shaped as a record id: an upper-case opening,
// a digit, and only letters, digits and dashes.
func knowledgeForm(w string) bool {
	if len(w) < 2 || w[0] < 'A' || w[0] > 'Z' {
		return false
	}
	digit := false
	for i := 0; i < len(w); i++ {
		c := w[i]
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '-':
		default:
			return false
		}
	}
	return digit
}

// pathHits is the weight of the prompt tokens found in the fields' path
// and summary.
func pathHits(prompt []string, f Fields, weights map[string]float64) float64 {
	seen := map[string]bool{}
	for _, t := range f.Path {
		seen[t] = true
	}
	for _, t := range f.Summary {
		seen[t] = true
	}
	s := 0.0
	for _, p := range prompt {
		if seen[p] {
			s += weights[p]
		}
	}
	return s
}

// evidenceUnit is the one unit the record's evidence lies in, "" when
// none or more than one.
func (c corpus) evidenceUnit(g Reader, id graph.ID) string {
	unit := ""
	for _, e := range g.Out(id, c.rev, graph.AppliesTo) {
		switch u := c.unitOf(e.Dst); {
		case u == "":
		case unit == "" || unit == u:
			unit = u
		default:
			return ""
		}
	}
	return unit
}

// unitOf is the unit a node is in: a scanned node's, a unit node's own
// path, "" for the rest.
func (c corpus) unitOf(id graph.ID) string {
	if i, ok := c.index[id]; ok {
		return c.units[i]
	}
	if strings.HasPrefix(string(id), "unit:") {
		return unitPath(id)
	}
	return ""
}

// evidenceIn is the record's evidence nodes inside unit, sorted.
func (c corpus) evidenceIn(g Reader, id graph.ID, unit string) []graph.ID {
	var out []graph.ID
	for _, e := range g.Out(id, c.rev, graph.AppliesTo) {
		if i, ok := c.index[e.Dst]; ok && c.units[i] == unit {
			out = append(out, e.Dst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
