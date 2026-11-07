package engine

import (
	"errors"
	"math"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Field weights of a token match (contracts-v2 section 6): a hit on a
// node's own name counts three, on its summary or its path one. Each
// prompt token counts once per field.
const (
	WeightName    = 3.0
	WeightSummary = 1.0
	WeightPath    = 1.0
	// SeedFloor is the least score a seed needs: one name hit on a short
	// name clears it, path-only hits do not.
	SeedFloor = 1.0
	// MaxSeeds caps the seeds a prompt yields.
	MaxSeeds = 8
)

// Seed is a node the prompt's words selected, with its score and the
// tokens that hit.
type Seed struct {
	ID    graph.ID
	Score float64
	Hits  []string
}

// Tokens is the prompt's and the node's shared vocabulary: lower case,
// split on punctuation, underscores and case changes, stemmed, with stop
// words and single characters dropped. An initialism (HTTPServer, HMACKey)
// is a word of its own: the last capital of a run of two or more starts
// the next word, except a lower-case s that ends the run (URLs), which is
// its plural (W-M1.5).
func Tokens(s string) []string {
	var out []string
	var cur []byte
	flush := func() {
		if len(cur) > 0 {
			if t := stem(strings.ToLower(string(cur))); t != "" {
				out = append(out, t)
			}
			cur = cur[:0]
		}
	}
	prevLower, upperRun := false, 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
			if upperRun > 1 && !pluralS(s, i) {
				last := cur[len(cur)-1]
				cur = cur[:len(cur)-1]
				flush()
				cur = append(cur, last)
			}
			cur = append(cur, c)
			prevLower, upperRun = true, 0
		case c >= '0' && c <= '9':
			cur = append(cur, c)
			prevLower, upperRun = false, 0
		case c >= 'A' && c <= 'Z':
			if prevLower {
				flush()
			}
			cur = append(cur, c)
			prevLower, upperRun = false, upperRun+1
		default:
			flush()
			prevLower, upperRun = false, 0
		}
	}
	flush()
	return out
}

// pluralS is s[i] as the lower-case s that ends an initialism (URLs, IDs):
// no lower-case letter follows it.
func pluralS(s string, i int) bool {
	return s[i] == 's' && (i+1 == len(s) || s[i+1] < 'a' || s[i+1] > 'z')
}

var stopWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"only": true, "same": true, "own": true, "just": true, "also": true, "please": true, "still": true, "yet": true,
		"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true, "in": true, "on": true, "for": true,
		"it": true, "its": true, "is": true, "be": true, "by": true, "with": true, "when": true, "once": true, "has": true, "have": true,
		"make": true, "add": true, "fix": true, "respect": true, "so": true, "that": true, "this": true, "src": true,
		"too": true, "many": true, "new": true, "level": true, "should": true, "must": true, "we": true, "our": true, "at": true,
		"from": true, "into": true, "if": true, "not": true, "no": true, "as": true, "all": true, "any": true, "each": true, "every": true,
	}
}}

// stem is a small suffix stripper, the same on both sides of a match:
// sessions and session, expiry, expires and expired, settings and setting
// agree.
func stem(w string) string {
	if len(w) < 2 || stopWords.get()[w] {
		return ""
	}
	if _, err := parseDigits(w); err == nil {
		return ""
	}
	for range 2 {
		for _, suf := range []string{"ing", "ed", "es", "s"} {
			if len(w) > len(suf)+3 && strings.HasSuffix(w, suf) && !strings.HasSuffix(w, "ss") {
				w = w[:len(w)-len(suf)]
				break
			}
		}
	}
	if len(w) > 4 && (strings.HasSuffix(w, "y") || strings.HasSuffix(w, "e")) {
		w = w[:len(w)-1]
	}
	if stopWords.get()[w] {
		return ""
	}
	return w
}

func parseDigits(w string) (int, error) {
	n := 0
	for i := 0; i < len(w); i++ {
		if w[i] < '0' || w[i] > '9' {
			return 0, errNotDigits
		}
		n = n*10 + int(w[i]-'0')
	}
	return n, nil
}

var errNotDigits = errors.New("not digits")

// Fields are a node's name, summary and path words.
type Fields struct {
	Name, Summary, Path []string
}

// FieldsOf reads a node's fields from its id and attributes. The id
// grammar is graph's (ids.go); the engine reads it without naming a
// language. Summary is the sig, value, text, cols and doc attributes, so a
// symbol's doc sentence meets the prompt at the summary's weight (W-M1.5).
func FieldsOf(n graph.Node) Fields {
	var f Fields
	id := string(n.ID)
	kind, rest, _ := strings.Cut(id, ":")
	switch kind {
	case "sym":
		parts := strings.Split(rest, ":")
		qual := parts[len(parts)-1]
		if i := strings.LastIndex(qual, "."); i >= 0 {
			f.Path = Tokens(qual[:i])
			qual = qual[i+1:]
		}
		f.Name = Tokens(qual)
		if len(parts) > 2 {
			f.Path = append(f.Path, Tokens(parts[1])...)
		}
	case "file":
		base := rest
		if i := strings.LastIndex(rest, "/"); i >= 0 {
			f.Path = Tokens(rest[:i])
			base = rest[i+1:]
		}
		f.Name = Tokens(strings.TrimSuffix(base, path.Ext(base)))
	case "tbl":
		f.Name = Tokens(rest)
	default:
		f.Name = Tokens(n.Attrs["title"])
		f.Path = Tokens(rest)
	}
	for _, k := range []string{"sig", "value", "text", "cols", "doc"} {
		f.Summary = append(f.Summary, Tokens(n.Attrs[k])...)
	}
	return f
}

// Score is the prompt tokens' score against fields: the weighted count of
// distinct prompt tokens hitting each field, over 1+ln(1+|node tokens|),
// so a long summary does not win by volume.
func Score(prompt []string, f Fields, weight func(string) float64) (float64, []string) {
	hits := map[string]bool{}
	score := 0.0
	count := func(field []string, w float64) {
		seen := map[string]bool{}
		for _, t := range field {
			seen[t] = true
		}
		for _, p := range prompt {
			if seen[p] {
				score += w * weight(p)
				hits[p] = true
			}
		}
	}
	count(f.Name, WeightName)
	count(f.Summary, WeightSummary)
	count(f.Path, WeightPath)
	n := len(f.Name) + len(f.Summary) + len(f.Path)
	if n == 0 || score == 0 {
		return 0, nil
	}
	out := make([]string, 0, len(hits))
	for h := range hits {
		out = append(out, h)
	}
	sort.Strings(out)
	return score / (1 + math.Log(1+float64(n))), out
}

// ScopeRow is one scored node, kept for the explanation whether or not it
// seeded.
type ScopeRow struct {
	ID    graph.ID
	Kind  string
	Score float64
	Hits  []string
	Seed  bool
}

// corpus is every code node at a revision with its fields, the unit each
// belongs to, and the document frequency of each token over the nodes
// (df, any field) and over their names and paths alone (nameDF): one pass
// over the store serves Scope and Units.
type corpus struct {
	rev    graph.Rev
	idf    bool
	nodes  []graph.Node
	fields []Fields
	units  []string
	index  map[graph.ID]int
	unitID map[string]graph.ID
	df     map[string]int
	nameDF map[string]int
	// files, bases and syms are the exact-name indexes R0 reads (W-M1.13):
	// a file by its path, files by their base name, symbols by their name.
	files map[string]graph.ID
	bases map[string][]graph.ID
	syms  map[string][]graph.ID
	exts  map[string]bool
}

// scan reads the corpus at rev. A lesson or decision is not in it: it
// reaches the brief only by applying to code the request's words selected,
// so its own words cannot vote for it (the B' confound, GRAPH-SLOT1); a
// unit is not scored but is recorded, since Units resolves to one.
func scan(g *graph.Store, rev graph.Rev) corpus {
	c := corpus{rev: rev, index: map[graph.ID]int{}, unitID: map[string]graph.ID{}, df: map[string]int{}, nameDF: map[string]int{},
		files: map[string]graph.ID{}, bases: map[string][]graph.ID{}, syms: map[string][]graph.ID{}, exts: map[string]bool{}}
	for _, n := range g.Nodes(rev) {
		if n.Kind == "unit" {
			c.unitID[unitPath(n.ID)] = n.ID
			continue
		}
		if n.Kind == "mod" || IsKnowledge(n.Kind) {
			continue
		}
		f := FieldsOf(n)
		c.index[n.ID] = len(c.nodes)
		u, uid := unitOf(g, rev, n)
		if u != "" {
			c.unitID[u] = uid
		}
		c.nodes, c.fields, c.units = append(c.nodes, n), append(c.fields, f), append(c.units, u)
		c.exact(n)
		seen := map[string]bool{}
		for _, t := range slices.Concat(f.Name, f.Path) {
			if !seen[t] {
				seen[t] = true
				c.nameDF[t]++
				c.df[t]++
			}
		}
		for _, t := range f.Summary {
			if !seen[t] {
				seen[t] = true
				c.df[t]++
			}
		}
	}
	return c
}

// exact enters n in the exact-name indexes: a file by its path, its base
// name (less the extension) and its extension, a symbol by the last
// segment of its qualified name.
func (c *corpus) exact(n graph.Node) {
	s := string(n.ID)
	switch {
	case strings.HasPrefix(s, "file:"):
		p := strings.TrimPrefix(s, "file:")
		c.files[p] = n.ID
		base := strings.TrimSuffix(path.Base(p), path.Ext(p))
		c.bases[base] = append(c.bases[base], n.ID)
		if ext := path.Ext(p); ext != "" {
			c.exts[ext] = true
		}
	case strings.HasPrefix(s, "sym:"):
		name := short(n.ID)
		c.syms[name] = append(c.syms[name], n.ID)
	}
}

// unitPath is the path a unit id names, after its language.
func unitPath(id graph.ID) string {
	_, p, _ := strings.Cut(strings.TrimPrefix(string(id), "unit:"), ":")
	return p
}

// unitOf is the path and id of the unit holding n: a symbol's from its
// id (the grammar is graph's, ids.go), a file's through the containing
// edge, none for the rest. The unit need not be a node of its own: an
// index that asserts the containing edges alone still names it.
func unitOf(g Reader, rev graph.Rev, n graph.Node) (string, graph.ID) {
	s := string(n.ID)
	switch {
	case strings.HasPrefix(s, "sym:"):
		if parts := strings.Split(s, ":"); len(parts) > 2 {
			return parts[2], graph.UnitID(parts[1], parts[2])
		}
	case strings.HasPrefix(s, "file:"):
		for _, e := range g.In(n.ID, rev, graph.Contains) {
			return unitPath(e.Src), e.Src
		}
	}
	return "", ""
}

// weights is each prompt token's weight: ln((N+1)/df) under idf, 1 otherwise.
func (c corpus) weights(toks []string, idf bool) map[string]float64 {
	w := map[string]float64{}
	for _, t := range toks {
		w[t] = 1
		if idf {
			w[t] = 0
			if c.df[t] > 0 {
				w[t] = math.Log(float64(len(c.nodes)+1) / float64(c.df[t]))
			}
		}
	}
	return w
}

// Scope scores every code node at rev against the prompt and returns the seeds
// (at most MaxSeeds, each at or over SeedFloor), every row that scored,
// and the weight each prompt token carried.
func Scope(g *graph.Store, rev graph.Rev, prompt string, opt Options) ([]Seed, []ScopeRow, map[string]float64) {
	return scan(g, rev).scope(prompt, opt)
}

func (c corpus) scope(prompt string, opt Options) ([]Seed, []ScopeRow, map[string]float64) {
	toks := uniq(Tokens(prompt))
	weights := c.weights(toks, opt.IDF)
	weight := func(t string) float64 { return weights[t] }
	var rows []ScopeRow
	for i, n := range c.nodes {
		s, hits := Score(toks, c.fields[i], weight)
		if s > 0 {
			rows = append(rows, ScopeRow{ID: n.ID, Kind: n.Kind, Score: s, Hits: hits})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		return rows[i].ID < rows[j].ID
	})
	var seeds []Seed
	for i := range rows {
		if len(seeds) == MaxSeeds || rows[i].Score < SeedFloor {
			break
		}
		rows[i].Seed = true
		seeds = append(seeds, Seed{ID: rows[i].ID, Score: rows[i].Score, Hits: rows[i].Hits})
	}
	return seeds, rows, weights
}

func uniq(ts []string) []string {
	seen := map[string]bool{}
	out := ts[:0]
	for _, t := range ts {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
