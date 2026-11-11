package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// symPlaces is the symbols file a sync of symSources writes for symLog's
// thirteen declared symbols: chained to symDigests, one row per id in id
// order, each placed in its package's file at a line, F and G with a doc.
func symPlaces(digestsSum string) string {
	var ids []string
	for _, l := range strings.Split(strings.TrimSuffix(symLog(), "\n"), "\n")[1:] {
		if f := strings.Split(l, "\t"); f[0] == "+" && f[2] == "declares" {
			ids = append(ids, f[3])
		}
	}
	sort.Strings(ids)
	s := SymbolsHeader + "\ndigests\t" + digestsSum + "\n"
	for i, id := range ids {
		pkg := id[strings.IndexByte(id, ':')+1 : strings.LastIndexByte(id, '.')]
		if strings.HasPrefix(id, "method:") {
			pkg = pkg[:strings.LastIndexByte(pkg, '.')]
		}
		doc := ""
		switch id {
		case "func:example.com/m/a.F":
			doc = "F is f."
		case "func:example.com/m/b.G":
			doc = "G calls F."
		}
		dir := strings.TrimPrefix(pkg, "example.com/m/")
		s += id + "\t" + pkg + "\t" + dir + "/" + dir[:1] + ".go\t" + strconv.Itoa(3+i) + "\t" + doc + "\n"
	}
	return s
}

// putWalkAll writes root's walk file naming log, digests and symbols.
func putWalkAll(t *testing.T, root, log, digests, symbols string) {
	t.Helper()
	put(t, filepath.Join(root, stateDir, walkFile), []byte(walkHeader+"\nwalk\t"+sumOf("walk")+"\nlog\t"+log+"\ndigests\t"+digests+"\nsymbols\t"+symbols+"\n"))
}

// snapSynced is symSynced plus the symbols file and the walk file that
// vouches for all three.
func snapSynced(t *testing.T) string {
	t.Helper()
	root := symSynced(t, false)
	symbols := symPlaces(sumOf(symDigests()))
	put(t, symbolsPath(root), []byte(symbols))
	putWalkAll(t, root, sumOf(symLog()), sumOf(symDigests()), sumOf(symbols))
	return root
}

// TestReadSnapshot_Vouched (W-M1.2): the sync read whole. Facts are the
// log's live set (an assert retracted later is not in it; two configs of
// one call are two), sorted; Places are the symbols rows in id order with
// their file, line and doc; Files are the digests' packages and paths.
func TestReadSnapshot_Vouched(t *testing.T) {
	root := snapSynced(t)
	s, err := ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Facts) != 20 || !slices.IsSortedFunc(s.Facts, func(a, b Fact) int {
		return strings.Compare(a.Subject+a.Predicate+a.Object+a.Tier+a.Config, b.Subject+b.Predicate+b.Object+b.Tier+b.Config)
	}) {
		t.Errorf("%d facts, sorted %v; want the log's 20 live facts in order", len(s.Facts), slices.IsSortedFunc(s.Facts, func(a, b Fact) int { return strings.Compare(a.Subject, b.Subject) }))
	}
	call := Fact{"func:example.com/m/b.G", PredCalls, "func:example.com/m/a.F", TierProven, cfg}
	old := Fact{"func:example.com/m/b.Old", PredCalls, "func:example.com/m/a.F", TierProven, cfg}
	if !slices.Contains(s.Facts, call) || slices.Contains(s.Facts, old) {
		t.Errorf("facts hold b.G calls a.F %v and the retracted b.Old call %v; want true, false", slices.Contains(s.Facts, call), slices.Contains(s.Facts, old))
	}
	if len(s.Places) != 13 || s.Places[0] != (Place{"func:example.com/m/a.F", "example.com/m/a", "a/a.go", 3, "F is f."}) ||
		s.Places[12].ID != "type:example.com/m/a.T" || s.Places[2].Doc != "G calls F." {
		t.Errorf("places = %+v; want 13 in id order, a.F first at a/a.go:3 with its doc", s.Places)
	}
	want := map[string][]string{"example.com/m/a": {"a/a.go"}, "example.com/m/b": {"b/b.go"}, "example.com/m/c": {"c/c.go"}, "example.com/m/yaml.v3": {"yaml.v3/y.go"}}
	if !reflect.DeepEqual(s.Files, want) {
		t.Errorf("files = %v; want %v", s.Files, want)
	}
	digests := map[string]string{}
	for rel, src := range symSources {
		digests[rel] = sumOf(src)
	}
	if !reflect.DeepEqual(s.Digests, digests) {
		t.Errorf("digests = %v; want the digests rows %v", s.Digests, digests)
	}
}

// TestReadSnapshot_RefusesStaleSymbols (W-M1.2): one byte of the symbols
// file flipped after the sync, or the log or the digests moved aside, and
// the whole read is ErrUnvouched with an empty Snapshot; so is a symbols
// file the walk names whose digests line is another sync's.
func TestReadSnapshot_RefusesStaleSymbols(t *testing.T) {
	for name, edit := range map[string]func(root string){
		"a symbols byte": func(root string) {
			b, _ := os.ReadFile(symbolsPath(root))
			put(t, symbolsPath(root), []byte(strings.Replace(string(b), "F is f.", "F is g.", 1)))
		},
		"the log": func(root string) { put(t, LogPath(root), []byte(LogHeader+"\n")) },
		"the digests": func(root string) {
			put(t, digestsPath(root), []byte(DigestsHeader+"\nexample.com/m/a\ta/a.go\t"+sumOf("package a\n")+"\n"))
		},
		"another sync's chain": func(root string) {
			symbols := symPlaces(sumOf("other digests"))
			put(t, symbolsPath(root), []byte(symbols))
			putWalkAll(t, root, sumOf(symLog()), sumOf(symDigests()), sumOf(symbols))
		},
	} {
		root := snapSynced(t)
		edit(root)
		s, err := ReadSnapshot(context.Background(), root)
		if !errors.Is(err, ErrUnvouched) || !reflect.DeepEqual(s, Snapshot{}) {
			t.Errorf("%s: got %d facts, %d places, %d files, %v; want ErrUnvouched and nothing", name, len(s.Facts), len(s.Places), len(s.Files), err)
		}
	}
}

// TestReadSnapshot_NeedsTheSidecar (W-M1.2): no root, no walk file, or a
// walk file from a pgraph that wrote no symbols file, is ErrNotSynced: the
// remedy is a sync, not a repair.
func TestReadSnapshot_NeedsTheSidecar(t *testing.T) {
	old := symSynced(t, true)
	for name, root := range map[string]string{"no root": "", "no walk file": symSynced(t, false), "four-line walk": old} {
		if _, err := ReadSnapshot(context.Background(), root); !errors.Is(err, ErrNotSynced) {
			t.Errorf("%s: %v; want ErrNotSynced", name, err)
		}
	}
}

// TestReadSnapshot_RefusesWhatItCannotReadWhole (W-M1.2): a symbols file
// under another header, with a row short a field, a row out of id order or
// a line number that is no count is its own error, not ErrUnvouched and
// not ErrNotSynced, even when the walk file names it.
func TestReadSnapshot_RefusesWhatItCannotReadWhole(t *testing.T) {
	good := symPlaces(sumOf(symDigests()))
	for name, symbols := range map[string]string{
		"header":       strings.Replace(good, "v0", "v1", 1),
		"short row":    strings.Replace(good, "\tF is f.\n", "\n", 1),
		"out of order": good + "func:example.com/m/a.A\texample.com/m/a\ta/a.go\t9\t\n",
		"no count":     strings.Replace(good, "\t3\tF is f.", "\tthree\tF is f.", 1),
	} {
		root := symSynced(t, false)
		put(t, symbolsPath(root), []byte(symbols))
		putWalkAll(t, root, sumOf(symLog()), sumOf(symDigests()), sumOf(symbols))
		_, err := ReadSnapshot(context.Background(), root)
		if err == nil || errors.Is(err, ErrUnvouched) || errors.Is(err, ErrNotSynced) {
			t.Errorf("%s: %v; want a read error", name, err)
		}
	}
}

// TestReadSnapshot_ReadsWhatPgraphWrote (W-M1.2, R14): the sidecar set the
// real pgraph wrote for testdata/snapshot (two packages, b calling a, doc
// comments on both functions) reads whole: a Place per declares object with
// the file, line and doc sentence pgraph placed, the PROVEN call from G to F
// among the facts, and both files under their packages.
func TestReadSnapshot_ReadsWhatPgraphWrote(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("testdata", "snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := ReadSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, f := range s.Facts {
		if f.Predicate == PredDeclares {
			declared[f.Object] = true
		}
	}
	placed := map[string]Place{}
	for _, p := range s.Places {
		placed[p.ID] = p
	}
	if len(placed) != len(declared) || len(placed) == 0 {
		t.Errorf("%d places for %d declared symbols", len(placed), len(declared))
	}
	for id := range declared {
		if _, ok := placed[id]; !ok {
			t.Errorf("declared %s has no place", id)
		}
	}
	if g := placed["func:example.com/snap/b.G"]; g.File != "b/b.go" || g.Line != 6 || g.Doc != "G calls F." {
		t.Errorf("G placed at %+v; want b/b.go:6, doc \"G calls F.\"", g)
	}
	if f := placed["func:example.com/snap/a.F"]; f.Doc != "F is one." {
		t.Errorf("F's doc = %q; want the first sentence only", f.Doc)
	}
	if !slices.ContainsFunc(s.Facts, func(f Fact) bool {
		return f.Subject == "func:example.com/snap/b.G" && f.Predicate == PredCalls && f.Object == "func:example.com/snap/a.F" && f.Tier == TierProven
	}) {
		t.Errorf("no PROVEN call from G to F among %d facts", len(s.Facts))
	}
	if !reflect.DeepEqual(s.Files, map[string][]string{"example.com/snap/a": {"a/a.go"}, "example.com/snap/b": {"b/b.go"}}) {
		t.Errorf("files = %v", s.Files)
	}
	for _, rel := range []string{"a/a.go", "b/b.go"} {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || s.Digests[rel] != sumOf(string(b)) {
			t.Errorf("digest of %s = %q; want the file's sha256 (%v)", rel, s.Digests[rel], err)
		}
	}
}
