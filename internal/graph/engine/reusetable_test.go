package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The reuse claims' table (W-M1.28): for every row of the development set
// with a reuse truth, the frame's phrases, each reuse claim with its
// grounds, the basis word its line carries and whether it is a truth, and
// for a truth file no claim holds, what dropped its hits. Each hit token
// is marked by the phrase class that holds it (T target, A addition, U
// use, C constraint, - none). RASHOMON_ACM11_REUSE=1 runs it over
// RASHOMON_ACM15_CLASSES (both halves, the acceptance half spent); the
// fresh set is refused by name. A reading, never a gate: it fails only
// when its replay of the claim walk disagrees with the engine's claims.
func TestReuse_ClaimTable(t *testing.T) {
	root, path := os.Getenv("RASHOMON_ACM11_DIR"), os.Getenv("RASHOMON_ACM15_CLASSES")
	if root == "" || path == "" || os.Getenv("RASHOMON_ACM11_REUSE") == "" {
		t.Skip("RASHOMON_ACM11_DIR, RASHOMON_ACM15_CLASSES and RASHOMON_ACM11_REUSE not set")
	}
	if filepath.Base(path) == "classes2.tsv" {
		t.Fatalf("%s: the fresh set is read once, at the frozen engine (TestACM15_FreshTable), and is never development data", path)
	}
	g, rev := loadACM11(t, root)
	c := scan(g, rev)
	c.idf = true
	tally := map[string]int{}
	seen := map[string]int{}
	var sb strings.Builder
	for _, row := range readTSV(t, path, 9) {
		half := halfOf(seen[row[1]])[:3]
		seen[row[1]]++
		if row[4] == "*" {
			continue
		}
		b := Build(g, rev, strings.ReplaceAll(row[8], "\\n", "\n"), Options{IDF: true})
		r := b.Roles
		got := briefUnit(g, rev, b)
		unitOK := slices.Contains(strings.Split(row[2], ";"), got) && got != ""
		lines := rolePaths(b, RoleReuse)
		v := judgeAny(lines, row[4])
		truths := strings.Split(row[4], ";")
		fmt.Fprintf(&sb, "%s %s %s: unit %s %s; reuse truth %s; lines %v %s\n  frame: targets %s additions %s uses %s constraints %s verbs %v\n",
			row[0], row[1], half, got, rightMark(unitOK), row[4], lines, rightMark(v.Right), phrasesText(r.Frame.Targets), phrasesText(r.Frame.Additions), phrasesText(r.Frame.Uses), phrasesText(r.Frame.Constraints), r.Frame.Verbs)
		if !r.Found() {
			tally["row: no unit"]++
			continue
		}
		w := walkReuse(g, c, b.Rows, r, b.Weights)
		if msg := w.check(r.ReuseClaims); msg != "" {
			t.Errorf("%s: the table's replay of the claim walk disagrees with the engine: %s", row[0], msg)
		}
		for i, f := range w.order {
			cl := w.files[f]
			kept := slices.ContainsFunc(r.Reuse, func(rf RoleFile) bool { return rf.File == f })
			truth := slices.Contains(truths, filePath(f))
			fmt.Fprintf(&sb, "  #%d %s in %v use %.2f add %.2f via %v out %v called %v basis %q line %v%s; best %s\n",
				i, filePath(f), cl.in, cl.use, cl.add, cl.viaAdd, cl.reused, cl.called, cl.basis(), kept, truthMark(truth), w.best(f))
			if cl.basis() == "" && cl.called {
				key := "bare claim elsewhere (the unit calls it)"
				if cl.in {
					key = "bare claim in the unit"
				}
				tally[key+fmt.Sprintf(", truth %v, row truth '-' %v", truth, slices.Contains(truths, "-"))]++
			}
		}
		for _, p := range truths {
			if p == "-" {
				continue
			}
			in := strings.HasPrefix(p, r.Path+"/") && !strings.Contains(p[len(r.Path)+1:], "/")
			where := "elsewhere"
			if in {
				where = "in the unit"
			}
			id := graph.ID("file:" + p)
			status := w.status(id, r)
			fmt.Fprintf(&sb, "  truth %s (%s, unit %s): %s\n", p, where, rightMark(unitOK), status)
			tally[fmt.Sprintf("truth %s, unit %s: %s", where, rightMark(unitOK), strings.SplitN(status, ";", 2)[0])]++
		}
	}
	var keys []string
	for k := range tally {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "tally %4d  %s\n", tally[k], k)
	}
	t.Logf("reuse claim table (W-M1.28):\n%s", sb.String())
}

// reuseWalk is the table's replay of reuse's walk: every hit, its file and
// what dropped it ("" when it folds into its file's claim), and the claims
// in the engine's rank order.
type reuseWalk struct {
	hits  map[graph.ID][]walkHit
	files map[graph.ID]*reuseClaim
	order []graph.ID
	cls   map[string]string
}

// walkHit is one scope row's hit on the reuse role.
type walkHit struct {
	id   graph.ID
	h    reuseHit
	drop string
}

// walkReuse replays reuse over the rows, as reuse.go reads them.
func walkReuse(g Reader, c corpus, rows []ScopeRow, r Roles, weights map[string]float64) reuseWalk {
	w := reuseWalk{hits: map[graph.ID][]walkHit{}, files: map[graph.ID]*reuseClaim{}, cls: map[string]string{}}
	for _, k := range []struct {
		mark string
		ps   []Phrase
	}{{"C", r.Frame.Constraints}, {"U", r.Frame.Uses}, {"A", r.Frame.Additions}, {"T", r.Frame.Targets}} {
		for t := range phraseTokens(k.ps) {
			w.cls[t] = k.mark
		}
	}
	skip := map[graph.ID]string{r.File: "the target file"}
	for _, rf := range r.Consumers {
		skip[rf.File] = "a consumer"
	}
	spent := map[string]bool{}
	for _, t := range append(Tokens(r.Path), r.Frame.Verbs...) {
		spent[t] = true
	}
	T := c.named(r.Target)
	for _, id := range r.Targets {
		if i, ok := c.index[id]; ok {
			for _, t := range c.fields[i].Name {
				spent[t] = spent[t] || T[t]
			}
		}
	}
	uses, adds := phraseTokens(r.Frame.Uses), phraseTokens(r.Frame.Additions)
	named := map[string]bool{}
	for t := range weights {
		named[t] = c.nameDF[t] > 0
	}
	var viaAdd []graph.ID
	for _, row := range rows {
		if row.Score < SeedFloor {
			break
		}
		i, ok := c.index[row.ID]
		if !ok {
			continue
		}
		h := reuseHits(c.fields[i], spent, uses, adds, named, weights)
		claims := h.name && h.claims(c, r.Frame.Additions, weights)
		if claims {
			viaAdd = append(viaAdd, row.ID)
		}
		file := declaring(g, c.rev, row.ID)
		in := c.units[i] == r.Path
		drop := ""
		switch {
		case h.mass == 0:
			drop = "every word spent"
		case file == "":
			drop = "no file"
		case skip[file] != "":
			drop = skip[file]
		case !in && !usedByUnit(g, c, row.ID, r.Path) && !(h.name && (h.use > 0 || claims)):
			drop = "elsewhere, no name hit on a use or claimed addition"
		}
		w.hits[file] = append(w.hits[file], walkHit{id: row.ID, h: h, drop: drop})
		if drop != "" {
			continue
		}
		if !claims {
			h.add = 0
		}
		cl := w.files[file]
		if cl == nil {
			cl = &reuseClaim{file: file, in: in}
			w.files[file] = cl
			w.order = append(w.order, file)
		}
		cl.take(row.ID, h, calledOutside(g, c, row.ID, r.Path))
	}
	for _, id := range viaAdd {
		for _, e := range g.Out(id, c.rev, graph.Calls, graph.MayCall, graph.References) {
			if cl := w.files[declaring(g, c.rev, e.Dst)]; cl != nil && contains(cl.nodes, e.Dst) {
				cl.viaAdd = true
			}
		}
	}
	sort.SliceStable(w.order, func(i, j int) bool { return w.files[w.order[i]].before(*w.files[w.order[j]]) })
	return w
}

// check is "" when the replay's claims are the engine's, in its order and
// with its grounds, else the first difference.
func (w reuseWalk) check(claims []ReuseClaim) string {
	if len(claims) != len(w.order) {
		return fmt.Sprintf("%d claims, the replay %d", len(claims), len(w.order))
	}
	for i, cl := range claims {
		o := w.files[w.order[i]]
		if cl.File != o.file || cl.Use != o.use || cl.Add != o.add || cl.ViaAdd != o.viaAdd || cl.In != o.in || cl.Reused != o.reused {
			return fmt.Sprintf("claim #%d %s, the replay %s", i, cl.File, o.file)
		}
	}
	return ""
}

// best is the file's claimed symbols, callables first, each with its hit
// tokens marked by phrase class and its precision.
func (w reuseWalk) best(f graph.ID) string {
	var out []string
	for _, wh := range w.hits[f] {
		if wh.drop == "" && len(out) < 3 {
			out = append(out, fmt.Sprintf("%s [%s] prec %.2f", shortSym(wh.id), w.marked(wh.h), wh.h.precision))
		}
	}
	return strings.Join(out, "; ")
}

// status is what became of a truth file: its claim's state, or what
// dropped each of its hits, or no scope row over SeedFloor.
func (w reuseWalk) status(f graph.ID, r Roles) string {
	if cl := w.files[f]; cl != nil {
		kept := slices.ContainsFunc(r.Reuse, func(rf RoleFile) bool { return rf.File == f })
		switch {
		case kept:
			return "a line; " + cl.basis()
		case !cl.called:
			return "claimed, no callable; " + w.best(f)
		case cl.basis() == "":
			return "claimed, no basis; " + w.best(f)
		}
		return "claimed with a basis, past the cap; " + w.best(f)
	}
	hs := w.hits[f]
	if len(hs) == 0 {
		return "no scope row over the seed floor"
	}
	drops := map[string]int{}
	var ex []string
	for _, wh := range hs {
		drops[wh.drop]++
		if len(ex) < 2 {
			ex = append(ex, fmt.Sprintf("%s [%s]", shortSym(wh.id), w.marked(wh.h)))
		}
	}
	var parts []string
	for _, k := range keys(boolKeys(drops)) {
		parts = append(parts, fmt.Sprintf("%s x%d", k, drops[k]))
	}
	return "dropped: " + strings.Join(parts, ", ") + "; " + strings.Join(ex, "; ")
}

// marked is the hit's tokens, each with its phrase class.
func (w reuseWalk) marked(h reuseHit) string {
	var out []string
	for _, t := range keys(h.named) {
		m := w.cls[t]
		if m == "" {
			m = "-"
		}
		out = append(out, t+"/"+m)
	}
	return strings.Join(out, " ")
}

func boolKeys(m map[string]int) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

func phrasesText(ps []Phrase) string {
	var out []string
	for _, p := range ps {
		out = append(out, strings.Join(p.Tokens, " "))
	}
	return "[" + strings.Join(out, " | ") + "]"
}

func filePath(id graph.ID) string { return strings.TrimPrefix(string(id), "file:") }

func shortSym(id graph.ID) string {
	parts := strings.Split(string(id), ":")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return string(id)
}

func rightMark(ok bool) string {
	if ok {
		return "R"
	}
	return "W"
}

func truthMark(ok bool) string {
	if ok {
		return " TRUTH"
	}
	return ""
}
