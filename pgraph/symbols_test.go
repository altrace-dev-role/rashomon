package pgraph

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

// docModule is a module whose declarations carry the doc comment shapes
// Symbols reads: two sentences (the first is the synopsis), a method, a bare
// declaration, a const group with a comment on one spec, a type whose comment
// sits on the keyword, a comment with non-ASCII runes, and one past the cap.
func docModule(t *testing.T) string {
	t.Helper()
	return writeTree(t, map[string]string{
		"go.mod": "module example.com/d\n\ngo 1.25.0\n",
		"d.go": "package d\n\n" +
			"// Add adds a and b. It never subtracts, whatever b is.\n" +
			"func Add(a, b int) int { return a + b }\n\n" +
			"// Counter counts.\n" +
			"type Counter struct{ n int }\n\n" +
			"// Inc increments the count. The second sentence is not a synopsis.\n" +
			"func (c *Counter) Inc() { c.n++ }\n\n" +
			"func Bare() {}\n\n" +
			"const (\n\t// One is one.\n\tOne = 1\n\tTwo = 2\n)\n\n" +
			"// Group is a single-spec declaration whose comment sits on the keyword.\n" +
			"type Group struct{}\n\n" +
			"// Naïve résumés are folded. Second.\n" +
			"func Folded() {}\n\n" +
			"// Long is long " + strings.Repeat("word ", 60) + "and ends here. Second.\n" +
			"func Long() {}\n",
	})
}

func symbolByID(syms []Symbol, id string) (Symbol, bool) {
	for _, s := range syms {
		if s.ID == id {
			return s, true
		}
	}
	return Symbol{}, false
}

// TestSymbols_PlacesEveryDeclared (W-M1.1): Symbols is one entry per object
// Extract declares, no more and no fewer, each placed in a slash path under
// the module at a line, in ascending id order, with its package path.
func TestSymbols_PlacesEveryDeclared(t *testing.T) {
	p := loadT(t, testModule)
	declared := map[string]bool{}
	for _, tr := range byPredicate(Extract(p), Declares) {
		declared[tr.Object] = true
	}
	syms := Symbols(p)
	seen := map[string]bool{}
	for i, s := range syms {
		if i > 0 && syms[i-1].ID >= s.ID {
			t.Errorf("not ascending at %d: %q after %q", i, s.ID, syms[i-1].ID)
		}
		if seen[s.ID] {
			t.Errorf("duplicate symbol %q", s.ID)
		}
		seen[s.ID] = true
		if !declared[s.ID] {
			t.Errorf("symbol %q is no object Extract declares", s.ID)
		}
		if s.File == "" || !strings.HasSuffix(s.File, ".go") || strings.HasPrefix(s.File, "/") || strings.Contains(s.File, "\\") || s.Line <= 0 {
			t.Errorf("symbol %q placed at %q:%d; want a slash path under the module and a line", s.ID, s.File, s.Line)
		}
		if s.Pkg == "" || !strings.Contains(s.ID, s.Pkg+".") {
			t.Errorf("symbol %q carries package %q; want the package path its id spells", s.ID, s.Pkg)
		}
	}
	for id := range declared {
		if !seen[id] {
			t.Errorf("declared %q has no symbol", id)
		}
	}
	if len(syms) == 0 {
		t.Fatal("no symbols")
	}
}

// TestSymbols_DocFirstSentence (W-M1.1): Doc is the first sentence of the
// declaration's doc comment (go/doc's synopsis rule), from the spec's own
// comment or the keyword's for a single-spec declaration, empty with none,
// with non-ASCII runes dropped, and cut at a word boundary within MaxDocBytes.
func TestSymbols_DocFirstSentence(t *testing.T) {
	syms := Symbols(loadT(t, docModule(t)))
	want := map[string]string{
		"func:example.com/d.Add":           "Add adds a and b.",
		"type:example.com/d.Counter":       "Counter counts.",
		"method:example.com/d.Counter.Inc": "Inc increments the count.",
		"func:example.com/d.Bare":          "",
		"const:example.com/d.One":          "One is one.",
		"const:example.com/d.Two":          "",
		"type:example.com/d.Group":         "Group is a single-spec declaration whose comment sits on the keyword.",
		"func:example.com/d.Folded":        "Nave rsums are folded.",
	}
	for id, doc := range want {
		s, ok := symbolByID(syms, id)
		if !ok {
			t.Errorf("no symbol %q", id)
			continue
		}
		if s.Doc != doc {
			t.Errorf("%s: Doc = %q; want %q", id, s.Doc, doc)
		}
	}
	long, ok := symbolByID(syms, "func:example.com/d.Long")
	if !ok {
		t.Fatal("no symbol for Long")
	}
	if len(long.Doc) > MaxDocBytes || len(long.Doc) < MaxDocBytes-8 || strings.HasSuffix(long.Doc, " ") || !strings.HasSuffix(long.Doc, "word") {
		t.Errorf("Long: Doc = %q (%d bytes); want a cut at a word boundary within %d bytes", long.Doc, len(long.Doc), MaxDocBytes)
	}
	add, _ := symbolByID(syms, "func:example.com/d.Add")
	if add.File != "d.go" || add.Line != 4 {
		t.Errorf("Add placed at %s:%d; want d.go:4", add.File, add.Line)
	}
}

// TestSymbols_EncodeChainsToDigests (W-M1.1): the symbols file is its header,
// then "digests\t" and the sha256 of the digests bytes it was written beside
// (the vouch chain), then one tab-separated row per symbol in id order, with
// files re-rooted at the sync root so a nested module's symbol names its
// path under the root, as the digests file does.
func TestSymbols_EncodeChainsToDigests(t *testing.T) {
	root := twoModules(t)
	mods, err := ScanRoots(root)
	if err != nil {
		t.Fatal(err)
	}
	var progs []*Program
	for _, m := range mods {
		progs = append(progs, loadT(t, m))
	}
	digests, err := EncodeDigests(root, progs)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncodeSymbols(root, progs, digests)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) < 3 || lines[0] != SymbolsHeader {
		t.Fatalf("symbols file = %q; want %s, the digests line and rows", string(b), SymbolsHeader)
	}
	sum := sha256.Sum256(digests)
	if lines[1] != "digests\t"+hex.EncodeToString(sum[:]) {
		t.Errorf("line 2 = %q; want digests\\t%s", lines[1], hex.EncodeToString(sum[:]))
	}
	rows := map[string][]string{}
	prev := ""
	for _, l := range lines[2:] {
		f := strings.Split(l, "\t")
		if len(f) != 5 {
			t.Fatalf("row %q has %d fields; want id, pkg, file, line, doc", l, len(f))
		}
		if f[0] <= prev {
			t.Errorf("row %q out of id order after %q", f[0], prev)
		}
		prev = f[0]
		if _, err := strconv.Atoi(f[3]); err != nil {
			t.Errorf("row %q: line %q is no integer", l, f[3])
		}
		rows[f[0]] = f
	}
	for id, want := range map[string][]string{
		"func:example.com/r/p.Call": {"example.com/r/p", "p/p.go"},
		"func:example.com/s/q.Call": {"example.com/s/q", "sub/q/q.go"},
	} {
		f, ok := rows[id]
		if !ok {
			t.Errorf("no row for %s", id)
			continue
		}
		if f[1] != want[0] || f[2] != want[1] {
			t.Errorf("%s: pkg, file = %q, %q; want %q, %q", id, f[1], f[2], want[0], want[1])
		}
	}
	if _, err := EncodeSymbols(root, progs, nil); err != nil {
		t.Errorf("EncodeSymbols with empty digests: %v; want the chain to the empty file's sha256, not an error", err)
	}
}
