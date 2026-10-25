package pgraph

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func fp(id, body string, nodes int) Fingerprint {
	rep := strings.Contains(id, "#")
	declared, _, _ := strings.Cut(id, "#")
	if !rep {
		declared = id
	}
	return Fingerprint{ID: id, Repeatable: rep, Declared: declared, Shape: "func () -> ()", Body: body, Nodes: nodes}
}

func linkMap(t *testing.T, old, cur []Fingerprint) map[string]string {
	t.Helper()
	ps, err := Link(old, cur)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, p := range ps {
		m[p.From] = p.To
	}
	return m
}

// The linking rules, each with one input that links and one that must not.
func TestPL10_LinkRules(t *testing.T) {
	big := MinLinkNodes
	for _, c := range []struct {
		name     string
		old, cur []Fingerprint
		want     map[string]string
	}{
		{"same id is the same symbol, body edited or not",
			[]Fingerprint{fp("f:A", "x", 1), fp("f:B", "y", 1)}, []Fingerprint{fp("f:A", "z", 1), fp("f:B", "y", 1)},
			map[string]string{"f:A": "f:A", "f:B": "f:B"}},
		{"pure rename links",
			[]Fingerprint{fp("f:Old", "x", big)}, []Fingerprint{fp("f:New", "x", big)},
			map[string]string{"f:Old": "f:New"}},
		{"rename with an edited body does not link",
			[]Fingerprint{fp("f:Old", "x", big)}, []Fingerprint{fp("f:New", "y", big)},
			map[string]string{}},
		{"one node under the floor does not link",
			[]Fingerprint{fp("f:Old", "x", big-1)}, []Fingerprint{fp("f:New", "x", big-1)},
			map[string]string{}},
		{"the floor is inclusive",
			[]Fingerprint{fp("f:Old", "x", big)}, []Fingerprint{fp("f:New", "x", big)},
			map[string]string{"f:Old": "f:New"}},
		{"two removed with one body: ambiguous",
			[]Fingerprint{fp("f:L", "x", big), fp("f:R", "x", big)}, []Fingerprint{fp("f:L2", "x", big), fp("f:R2", "x", big)},
			map[string]string{}},
		{"two removed, one added: ambiguous",
			[]Fingerprint{fp("f:L", "x", big), fp("f:R", "x", big)}, []Fingerprint{fp("f:N", "x", big)},
			map[string]string{}},
		{"one removed, two added: ambiguous",
			[]Fingerprint{fp("f:O", "x", big)}, []Fingerprint{fp("f:N1", "x", big), fp("f:N2", "x", big)},
			map[string]string{}},
		{"a surviving twin makes the body ambiguous: a copy is not a rename",
			[]Fingerprint{fp("f:A", "x", big), fp("f:B", "x", big)}, []Fingerprint{fp("f:A", "x", big), fp("f:C", "x", big)},
			map[string]string{"f:A": "f:A"}},
		{"a name kept by an unrelated function is identity, not a rename target",
			[]Fingerprint{fp("f:A", "x", big)}, []Fingerprint{fp("f:A", "other", big), fp("f:B", "x", big)},
			map[string]string{"f:A": "f:A"}},
		{"different shape does not link",
			[]Fingerprint{fp("f:Old", "x", big)}, []Fingerprint{{ID: "f:New", Shape: "func (int) -> ()", Body: "x", Nodes: big}},
			map[string]string{}},
		{"a repeatable id kept with its body links even when trivial",
			[]Fingerprint{fp("f:p.init#1", "x", 1)}, []Fingerprint{fp("f:p.init#1", "x", 1)},
			map[string]string{"f:p.init#1": "f:p.init#1"}},
		{"a repeatable id kept with another body is not the same symbol",
			[]Fingerprint{fp("f:p.init#1", "x", big)}, []Fingerprint{fp("f:p.init#1", "y", big)},
			map[string]string{}},
		{"two repeatable ids exchanged: bodies, not ids, decide",
			[]Fingerprint{fp("f:p.init#1", "a", big), fp("f:p.init#2", "b", big)}, []Fingerprint{fp("f:p.init#1", "b", big), fp("f:p.init#2", "a", big)},
			map[string]string{"f:p.init#1": "f:p.init#2", "f:p.init#2": "f:p.init#1"}},
		{"nothing in common",
			[]Fingerprint{fp("f:A", "x", big)}, []Fingerprint{fp("f:B", "y", big)},
			map[string]string{}},
		{"both empty", nil, nil, map[string]string{}},
	} {
		if got := linkMap(t, c.old, c.cur); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %v\n want %v", c.name, got, c.want)
		}
	}
}

func TestPL10_LinkRefusesDuplicateIDs(t *testing.T) {
	d := []Fingerprint{fp("f:A", "x", 1), fp("f:A", "y", 1)}
	if _, err := Link(d, nil); err == nil {
		t.Error("duplicate id on the old side accepted")
	}
	if _, err := Link(nil, d); err == nil {
		t.Error("duplicate id on the new side accepted")
	}
}

// Properties of Link over random inputs: a partial injection from old to new,
// the same for any input order, and a link between different ids always has
// the same shape and bodies at or above the floor, equal or (rule 4) alike
// enough within one declared name.
func TestPL10_LinkProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	gen := func(prefix string, n int) []Fingerprint {
		var out []Fingerprint
		used := map[string]bool{}
		for len(out) < n {
			id := fmt.Sprintf("f:%s%d", []string{"", "q.init#"}[rng.Intn(2)], rng.Intn(12))
			if used[id] {
				continue
			}
			used[id] = true
			nodes := rng.Intn(2 * MinLinkNodes)
			var toks []uint32
			if strings.Contains(id, "#") {
				// One of three bodies, with up to four tokens replaced.
				base := uint32(100 * rng.Intn(3))
				for i := 0; i < 20; i++ {
					toks = append(toks, base+uint32(i))
				}
				for k := rng.Intn(5); k > 0; k-- {
					toks[rng.Intn(20)] = uint32(1000 + rng.Intn(1000))
				}
				sort.Slice(toks, func(a, b int) bool { return toks[a] < toks[b] })
			}
			out = append(out, Fingerprint{Tokens: toks, ID: id, Repeatable: strings.Contains(id, "#"), Declared: strings.SplitN(id, "#", 2)[0], Shape: fmt.Sprint("s", rng.Intn(2)), Body: fmt.Sprint("b", rng.Intn(6), "n", nodes%3), Nodes: nodes})
		}
		return out
	}
	byID := func(fs []Fingerprint) map[string]Fingerprint {
		m := map[string]Fingerprint{}
		for _, f := range fs {
			m[f.ID] = f
		}
		return m
	}
	rule4 := 0
	defer func() {
		if rule4 == 0 {
			t.Error("no input reached rule 4: the property is vacuous for it")
		}
	}()
	for i := 0; i < 400; i++ {
		old, cur := gen("o", 1+rng.Intn(10)), gen("n", 1+rng.Intn(10))
		a, err := Link(old, cur)
		if err != nil {
			t.Fatal(err)
		}
		oi, ci := byID(old), byID(cur)
		from, to := map[string]bool{}, map[string]bool{}
		for _, p := range a {
			if from[p.From] || to[p.To] {
				t.Fatalf("not injective: %v in %v", p, a)
			}
			from[p.From], to[p.To] = true, true
			o, c := oi[p.From], ci[p.To]
			if o.ID == "" || c.ID == "" {
				t.Fatalf("link %v names an id that is not in its snapshot", p)
			}
			if p.From != p.To && o.Nodes < MinLinkNodes || p.From != p.To && c.Nodes < MinLinkNodes {
				t.Fatalf("link %v under the floor: %+v %+v", p, o, c)
			}
			if p.From != p.To && o.Shape != c.Shape {
				t.Fatalf("link %v across shapes: %+v %+v", p, o, c)
			}
			if o.Body != c.Body && !(p.From == p.To && !o.Repeatable) {
				// Only rule 4 links two different bodies (rule 1 links a name
				// that kept its id): the same declared name, both repeatable,
				// and alike enough.
				num, den := similarity(o.Tokens, c.Tokens)
				rule4++
				if !o.Repeatable || !c.Repeatable || o.Declared != c.Declared || den == 0 || num*100 < LinkSimilarityPercent*den {
					t.Fatalf("link %v of different bodies below rule 4: %+v %+v", p, o, c)
				}
			}
		}
		for id, o := range oi {
			if c, ok := ci[id]; ok && !o.Repeatable && !from[id] {
				t.Fatalf("%s is in both snapshots and was not linked to itself (%+v %+v)", id, o, c)
			}
		}
		rng.Shuffle(len(old), func(x, y int) { old[x], old[y] = old[y], old[x] })
		rng.Shuffle(len(cur), func(x, y int) { cur[x], cur[y] = cur[y], cur[x] })
		b, _ := Link(old, cur)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("order of input changed the links:\n%v\n%v", a, b)
		}
		if !sort.SliceIsSorted(a, func(x, y int) bool { return a[x].From < a[y].From }) {
			t.Fatalf("links not sorted: %v", a)
		}
	}
}

// writeMod writes a one-package module whose functions have k statements of
// the form `_ = 1`: each statement is an assignment, the blank identifier and
// a literal (three nodes), and the body adds its block, so a body of k has
// 1 + 3k nodes.
func writeMod(t *testing.T, names map[string]int) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/floor\n\ngo 1.25.0\n"), 0o644)
	var b strings.Builder
	b.WriteString("package p\n")
	keys := make([]string, 0, len(names))
	for n := range names {
		keys = append(keys, n)
	}
	sort.Strings(keys)
	for _, n := range keys {
		fmt.Fprintf(&b, "\nfunc %s() {\n%s}\n", n, strings.Repeat("\t_ = 1\n", names[n]))
	}
	os.WriteFile(filepath.Join(dir, "p.go"), []byte(b.String()), 0o644)
	return dir
}

// The floor measured on real source: with a floor of 20 nodes, a body of 6
// statements (19 nodes) never links across a rename and one of 7 (22) does.
func TestPL10_FloorOnRealBodies(t *testing.T) {
	if MinLinkNodes != 20 {
		t.Fatalf("MinLinkNodes = %d; this test's arithmetic assumes 20", MinLinkNodes)
	}
	before := Fingerprints(loadT(t, writeMod(t, map[string]int{"Six": 6, "Seven": 7})))
	after := Fingerprints(loadT(t, writeMod(t, map[string]int{"SixRenamed": 6, "SevenRenamed": 7})))
	nodes := map[string]int{}
	for _, f := range before {
		nodes[f.ID] = f.Nodes
	}
	if nodes["func:example.test/floor.Six"] != 19 || nodes["func:example.test/floor.Seven"] != 22 {
		t.Fatalf("body sizes %v, want 19 and 22", nodes)
	}
	got := linkMap(t, before, after)
	want := map[string]string{"func:example.test/floor.Seven": "func:example.test/floor.SevenRenamed"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// The floor is the parameter of the measurement and nothing else: one node
	// lower and the body of 19 links too, one node higher and neither does.
	for floor, n := range map[int]int{19: 2, 22: 1, 23: 0} {
		ps, err := link(before, after, floor)
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != n {
			t.Errorf("floor %d: %d links, want %d: %v", floor, len(ps), n, ps)
		}
	}
}

// A pure rename is linked on real source, and what is not a pure rename is
// not: an edited body, a body shared by two functions, a stub under the
// floor. The two commits are real git commits (idRepo).
func TestPL10_LinkAcrossCommits(t *testing.T) {
	dir, commits := idRepo(t)
	f1, f2 := Fingerprints(loadAt(t, dir, commits[0])), Fingerprints(loadAt(t, dir, commits[1]))
	core, util := idm+"/core.", idm+"/util."
	fn := func(n string) string { return "func:" + core + n }
	me := func(typ, n string) string { return "method:" + core + typ + "." + n }

	// Preconditions of the fixture: the renamed bodies are over the floor and
	// the stubs and one-line methods are under it.
	by := map[string]Fingerprint{}
	for _, f := range append(append([]Fingerprint{}, f1...), f2...) {
		by[f.ID] = f
	}
	declaredIDs := func(fps []Fingerprint, declared string) []string {
		var out []string
		for _, f := range fps {
			if f.Declared == declared {
				out = append(out, f.ID)
			}
		}
		return out
	}
	inits1, inits2 := declaredIDs(f1, fn("init")), declaredIDs(f2, fn("init"))
	blanks1, blanks2 := declaredIDs(f1, fn("_")), declaredIDs(f2, fn("_"))
	if len(inits1) != 2 || len(inits2) != 3 || len(blanks1) != 1 || len(blanks2) != 2 {
		t.Fatalf("fixture: inits %v then %v, blank functions %v then %v", inits1, inits2, blanks1, blanks2)
	}
	for _, id := range append([]string{fn("Normalize"), fn("Canonicalize"), "func:" + util + "Checksum", me("Stack", "Pop"), me("Pile", "Pop"), fn("Left"), fn("LeftSquares")}, inits1...) {
		if by[id].Nodes < MinLinkNodes {
			t.Fatalf("%s has %d nodes; the fixture needs it over the floor", id, by[id].Nodes)
		}
	}
	for _, id := range []string{fn("Noop"), fn("Placeholder"), me("Stack", "Push"), me("Pile", "Push"), me("Stack", "Reset"), me("Pile", "Reset")} {
		if by[id].Nodes >= MinLinkNodes {
			t.Fatalf("%s has %d nodes; the fixture needs it under the floor", id, by[id].Nodes)
		}
	}

	got := linkMap(t, f1, f2)
	want := map[string]string{
		// Unchanged ids, one with its body edited.
		fn("Compute"): fn("Compute"), fn("New"): fn("New"), fn("Pairs"): fn("Pairs"), fn("Triples"): fn("Triples"),
		fn("Both"): fn("Both"), fn("Double"): fn("Double"), fn("CloseIt"): fn("CloseIt"), fn("Measure"): fn("Measure"),
		me("Queue", "Reset"): me("Queue", "Reset"), me("Small", "Size"): me("Small", "Size"),
		// Pure renames and a move.
		fn("Normalize"):             fn("Canonicalize"),
		"func:" + core + "Checksum": "func:" + util + "Checksum",
		// A type renamed: its method with a real body follows by its body.
		me("Stack", "Pop"): me("Pile", "Pop"),
	}
	// The two inits swapped places between files (b_init.go became 0_init.go,
	// ahead of a_init.go, and a third init follows) and the blank function
	// stayed: each keeps its id, by rule 1, and the new init and blank function
	// are new.
	for _, id := range append(append([]string{}, inits1...), blanks1...) {
		want[id] = id
	}
	if !reflect.DeepEqual(got, want) {
		for k, v := range want {
			if got[k] != v {
				t.Errorf("link %s: got %q, want %q", k, got[k], v)
			}
		}
		for k, v := range got {
			if _, ok := want[k]; !ok {
				t.Errorf("unexpected link %s -> %s", k, v)
			}
		}
	}
	// Left and Right: one body twice, both renamed. Unlinked, and so are the
	// stubs: the unlinked ids are named so the refusal is on purpose.
	for _, id := range []string{fn("Left"), fn("Right"), fn("Noop"), me("Large", "Size")} {
		if _, ok := got[id]; ok {
			t.Errorf("%s was linked", id)
		}
	}
	// ACCEPTED GAP: a renamed type takes its methods' ids with it and only the
	// methods over the floor are carried across by body. Stack.Push and
	// Stack.Reset, one-liners, are unlinked.
	for _, id := range []string{me("Stack", "Push"), me("Stack", "Reset")} {
		if _, ok := got[id]; ok {
			t.Errorf("%s was linked despite being under the floor", id)
		}
	}
}

// Fingerprints depend on the program's text and nothing else: not on
// comments, layout, position, or the function's own name.
func TestPL10_FingerprintIgnoresLayoutAndOwnName(t *testing.T) {
	mk := func(src string) Fingerprint {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/fp\n\ngo 1.25.0\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644)
		fs := Fingerprints(loadT(t, dir))
		if len(fs) != 1 {
			t.Fatalf("%d fingerprints", len(fs))
		}
		return fs[0]
	}
	base := mk("package p\n\nfunc Fact(n int) int {\n\tif n < 2 {\n\t\treturn 1\n\t}\n\treturn n * Fact(n-1)\n}\n")
	same := mk("package p\n\n// moved and commented\nvar _ = 0\n\nfunc Factorial( n int ) int { // trailing\n\tif n<2 { return 1 }\n\t/* inner */ return n*Factorial(n-1)\n}\n")
	if base.Body != same.Body || base.Shape != same.Shape || base.Nodes != same.Nodes {
		t.Errorf("layout, comments or the recursive name changed the fingerprint:\n%+v\n%+v", base, same)
	}
	for name, src := range map[string]string{
		"operator":        "package p\n\nfunc Fact(n int) int {\n\tif n > 2 {\n\t\treturn 1\n\t}\n\treturn n * Fact(n-1)\n}\n",
		"literal":         "package p\n\nfunc Fact(n int) int {\n\tif n < 3 {\n\t\treturn 1\n\t}\n\treturn n * Fact(n-1)\n}\n",
		"parameter name":  "package p\n\nfunc Fact(m int) int {\n\tif m < 2 {\n\t\treturn 1\n\t}\n\treturn m * Fact(m-1)\n}\n",
		"result type":     "package p\n\nfunc Fact(n int) int64 {\n\tif n < 2 {\n\t\treturn 1\n\t}\n\treturn int64(n) * Fact(n-1)\n}\n",
		"extra statement": "package p\n\nfunc Fact(n int) int {\n\tif n < 2 {\n\t\treturn 1\n\t}\n\tn--\n\treturn n * Fact(n-1)\n}\n",
	} {
		if o := mk(src); o.Body == base.Body && o.Shape == base.Shape {
			t.Errorf("%s: fingerprint unchanged", name)
		}
	}
}

// Similarity measured on real bodies: the numbers LinkSimilarityPercent's
// comment quotes, and the margins on either side of the threshold.
func TestPL10_SimilarityOnRealBodies(t *testing.T) {
	tok := func(src string) Fingerprint {
		fps := fingerprintsOfModule(t, map[string]string{"p.go": mergeBase + src})
		for _, f := range fps {
			if f.Repeatable {
				return f
			}
		}
		t.Fatal("no init")
		return Fingerprint{}
	}
	pct := func(a, b Fingerprint) int {
		num, den := similarity(a.Tokens, b.Tokens)
		return 100 * num / den
	}
	a, a2, b, c, x := tok(initA), tok(initA2), tok(initB), tok(initC), tok(initX)
	t.Logf("A: %d nodes, A2: %d, B: %d, C: %d, X: %d", a.Nodes, a2.Nodes, b.Nodes, c.Nodes, x.Nodes)
	t.Logf("similarity percent: A~A %d, A~A2 (one literal) %d, A~C (every literal) %d, A~B %d, B~X %d, A~X %d", pct(a, a), pct(a, a2), pct(a, c), pct(a, b), pct(b, x), pct(a, x))
	if pct(a, a) != 100 {
		t.Errorf("a body is not 100 percent like itself: %d", pct(a, a))
	}
	if got := pct(a, a2); got < LinkSimilarityPercent {
		t.Errorf("one literal edited: %d percent, under the threshold %d", got, LinkSimilarityPercent)
	}
	for name, got := range map[string]int{"A~C": pct(a, c), "A~B": pct(a, b), "B~X": pct(b, x), "A~X": pct(a, x)} {
		if got >= LinkSimilarityPercent {
			t.Errorf("%s: %d percent, at or over the threshold %d", name, got, LinkSimilarityPercent)
		}
	}
}

// Rule 4 on both sides of its threshold, and each of its conditions on its
// own. Bodies are synthetic token multisets of 20 tokens, so the Dice
// coefficient of two that share k tokens is 2k over 40: 18 shared is 90
// percent, 17 is 85.
func TestPL10_LinkBestMatch(t *testing.T) {
	if LinkSimilarityPercent != 90 {
		t.Fatalf("LinkSimilarityPercent = %d; this test's arithmetic assumes 90", LinkSimilarityPercent)
	}
	// tokens returns n distinct tokens, the first k shared with every body
	// built by tokens(_, k) and the rest private to seed.
	tokens := func(seed uint32, k int) []uint32 {
		out := make([]uint32, 0, 20)
		for i := 0; i < 20; i++ {
			if i < k {
				out = append(out, uint32(i))
			} else {
				out = append(out, 1000*seed+uint32(i))
			}
		}
		return out
	}
	rep := func(id, declared, shape, body string, nodes int, toks []uint32) Fingerprint {
		return Fingerprint{ID: id, Repeatable: true, Declared: declared, Shape: shape, Body: body, Nodes: nodes, Tokens: toks}
	}
	const sh = "func () -> ()"
	big := MinLinkNodes
	for _, c := range []struct {
		name     string
		old, cur []Fingerprint
		want     map[string]string
	}{
		{"at the threshold links",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20))}, []Fingerprint{rep("f:p.init#b", "f:p.init", sh, "b", big, tokens(2, 18))},
			map[string]string{"f:p.init#a": "f:p.init#b"}},
		{"just under the threshold does not",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20))}, []Fingerprint{rep("f:p.init#b", "f:p.init", sh, "b", big, tokens(2, 17))},
			map[string]string{}},
		{"nothing in common does not",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 0))}, []Fingerprint{rep("f:p.init#b", "f:p.init", sh, "b", big, tokens(2, 0))},
			map[string]string{}},
		{"another declared name does not",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20))}, []Fingerprint{rep("f:p._#b", "f:p._", sh, "b", big, tokens(2, 18))},
			map[string]string{}},
		{"another shape does not",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20))}, []Fingerprint{rep("f:p.init#b", "f:p.init", "func (int) -> ()", "b", big, tokens(2, 18))},
			map[string]string{}},
		{"a stub under the floor does not",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big-1, tokens(1, 20))}, []Fingerprint{rep("f:p.init#b", "f:p.init", sh, "b", big, tokens(2, 18))},
			map[string]string{}},
		{"two olds equally near one new: a tie, not guessed",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20)), rep("f:p.init#c", "f:p.init", sh, "c", big, tokens(3, 20))},
			[]Fingerprint{rep("f:p.init#b", "f:p.init", sh, "b", big, tokens(2, 19))},
			map[string]string{}},
		{"each is the other's best: both link, whatever the order",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 0)), rep("f:p.init#c", "f:p.init", sh, "c", big, append(tokens(3, 0)[:10:10], 7, 8, 9, 10, 11, 12, 13, 14, 15, 16))},
			[]Fingerprint{rep("f:p.init#d", "f:p.init", sh, "d", big, tokens(1, 0)[:19:19]), rep("f:p.init#b", "f:p.init", sh, "b", big, append(tokens(3, 0)[:10:10], 7, 8, 9, 10, 11, 12, 13, 14, 15, 17))},
			map[string]string{"f:p.init#a": "f:p.init#d", "f:p.init#c": "f:p.init#b"}},
		{"a repeatable symbol that kept its id is not a candidate",
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20)), rep("f:p.init#c", "f:p.init", sh, "c", big, tokens(3, 20))},
			[]Fingerprint{rep("f:p.init#a", "f:p.init", sh, "a", big, tokens(1, 20)), rep("f:p.init#b", "f:p.init", sh, "b", big, tokens(2, 18))},
			map[string]string{"f:p.init#a": "f:p.init#a", "f:p.init#c": "f:p.init#b"}},
		{"a name that is not repeatable is never matched by similarity",
			[]Fingerprint{{ID: "f:p.A", Declared: "f:p.A", Shape: sh, Body: "a", Nodes: big, Tokens: tokens(1, 20)}}, []Fingerprint{{ID: "f:p.B", Declared: "f:p.B", Shape: sh, Body: "b", Nodes: big, Tokens: tokens(2, 18)}},
			map[string]string{}},
	} {
		if got := linkMap(t, c.old, c.cur); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got  %v\n want %v", c.name, got, c.want)
		}
		// The same answer for any input order.
		rev := func(fs []Fingerprint) []Fingerprint {
			out := append([]Fingerprint(nil), fs...)
			for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
				out[i], out[j] = out[j], out[i]
			}
			return out
		}
		if got := linkMap(t, rev(c.old), rev(c.cur)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s, inputs reversed:\n got  %v\n want %v", c.name, got, c.want)
		}
	}
}
