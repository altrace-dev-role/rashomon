package judge

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// J3 of slice T2 (PLAN W-M2.8): conflicts at Stop, one instrument for every
// arm. The engine source this package compiles against (the M2 tree at
// T1_G_BIN_SHA) is the judge; no arm's binary judges. Exported functions
// only. The tests read their inputs from the environment and skip without
// them, as ACM15 and ACM21 do:
//
//	RASHOMON_T2_SEED     the seeded rows: one knowledge File, schema 1
//	                     (pins.txt T1_KNOWLEDGE_SEED_SHA256)
//	RASHOMON_T2_SPEC     arm G's managed spec, rashomon/spec.json under
//	                     the directory of RASHOMON_MANAGED_SETTINGS_PATH
//	                     (pins.txt T1_MANAGED_SPEC_SHA256); optional, unset
//	                     when the smoke drops S1
//	RASHOMON_T2_J3_BASE  a root at T1_TREE_SHA, synced with the judge's
//	                     pgraph and indexed with the M2 binary
//	RASHOMON_T2_J3_STOP  the arm's final tree built the same way:
//	                     T1_TREE_SHA, the run's diff.patch applied and its
//	                     new files copied, so its index names the base's
//	                     commit
//	RASHOMON_T2_J3_RUN   the run directory (status.txt, new-files.txt)
//	RASHOMON_T2_J3_OUT   the file TestJ3_ConflictsAtStop writes
//
// The Stop graph is the final tree indexed in full and re-asserted under
// stopCommit, not overlays over the base: the Go overlay carries
// declarations alone (gopg Overlay: "Calls are not resolved"), and a
// boundary_cross needs an added Calls, MayCall, Imports or References
// edge, so an overlay-only judge raises none by construction. The overlay
// view is computed beside it, as what the live path can see, and claims
// nothing. The product's own Stop read may be weaker than the judge's, so a
// boundary_cross the judge finds that G never named is an unresolved
// conflict against G. That is the measurement, not a judge defect.
//
// The rules: engine.RulesFor, the product's one rules function (the plan
// check, the W-M2.4 steer and this judge take their rules from it), over
// the seed's rows and the managed spec as spec.Discover reads it, against
// the base index's repository (its header) at the base commit. RulesFor
// drops a refused row, a global applies_to id of another repository and
// one the graph does not hold, without a word; seedRules restates each
// drop as a failure, so no rule is thinner than its row. A spec Discover
// refuses (an Errors line: a managed spec not admin-owned, or one Parse
// refuses) fails too, since the arms' binaries ignore it.
// TestJ3_RuleFieldsPinned fails when engine.Rule gains or loses a field.

// stopCommit names the arm's final tree in the judge's store, apart from
// every real commit.
const stopCommit = "t2-stop"

// TestJ3_SeedRulesCompile is the seed's check before pre-registration:
// every seeded row compiles (knowledge.Rules refuses none) and its clause
// is one the importer would write unchanged (knowledge.Clause); the
// managed spec, when set, is read with no Errors line; the base index
// names its repository; RulesFor keeps every rule and every applies_to id
// of each row; at least one rule results; every id a rule applies to is a
// node the base graph holds at its commit; and every From and To prefix
// covers at least one unit node there, so no rule is silent by a mistyped
// prefix.
func TestJ3_SeedRulesCompile(t *testing.T) {
	seed, base := os.Getenv("RASHOMON_T2_SEED"), os.Getenv("RASHOMON_T2_J3_BASE")
	if seed == "" || base == "" {
		t.Skip("RASHOMON_T2_SEED and RASHOMON_T2_J3_BASE not set")
	}
	for _, row := range readSeed(t, seed).Rows {
		if c := knowledge.Clause(row.Clause); c != row.Clause {
			t.Errorf("seed row %s: clause %q reads %q under the approved-text rules", row.ID, row.Clause, c)
		}
	}
	s, h, err := graph.LoadFile(base)
	if err != nil {
		t.Fatalf("index under %s: %v", base, err)
	}
	if h.Repo == "" {
		t.Fatalf("index under %s names no repository; RulesFor would localise no global id", base)
	}
	t.Logf("base index repository %s commit %s", h.Repo, h.Rev.Commit)
	rev := graph.RevCommit(h.Rev.Commit)
	rules := seedRules(t, seed, os.Getenv("RASHOMON_T2_SPEC"), s, h.Repo, rev)
	var units []graph.ID
	for _, n := range s.Nodes(rev) {
		if strings.HasPrefix(string(n.ID), "unit:") {
			units = append(units, n.ID)
		}
	}
	for _, r := range rules {
		for _, id := range r.AppliesTo {
			if _, ok := s.Node(id, rev); !ok {
				t.Errorf("rule %s (row %s) applies to %s, which the graph at %s does not hold", r.ID, r.Source, id, rev)
			}
		}
		for _, p := range slices.Concat(r.From, r.To) {
			n := 0
			for _, u := range units {
				if string(u) == p || strings.HasPrefix(string(u), p+"/") {
					n++
				}
			}
			if n == 0 {
				t.Errorf("rule %s names %s, which covers no unit node of the graph at %s", r.ID, p, rev)
			}
			t.Logf("rule %s prefix %s covers %d unit nodes", r.ID, p, n)
		}
		t.Logf("rule %s kind %s predicate %s applies_to %v from %v to %v source %s", r.ID, r.Kind, r.Predicate, r.AppliesTo, r.From, r.To, r.Source)
	}
}

// TestJ3_RuleFieldsPinned pins engine.Rule, the rule as J3 reads it from
// engine.RulesFor: it fails when a field is added or removed, so a new
// field is read before J3 claims anything. Paths carries protect_path
// rules; J3's scored kinds (j3Scored) do not read it.
func TestJ3_RuleFieldsPinned(t *testing.T) {
	want := []string{"ID", "Kind", "From", "To", "Predicate", "Paths", "AppliesTo", "Source"}
	typ := reflect.TypeFor[engine.Rule]()
	var got []string
	for i := range typ.NumField() {
		got = append(got, typ.Field(i).Name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("engine.Rule fields %v, pinned %v; read the change before J3 claims anything", got, want)
	}
}

// TestJ3_OverlayCarriesDeclaresOnly pins the fact the Stop graph rests on:
// over every changed Go file of the run, the Go overlay emits nodes and
// Declares edges and nothing else. If it ever emits another edge this
// fails, and the overlay view is read again before J3 claims anything from
// it.
func TestJ3_OverlayCarriesDeclaresOnly(t *testing.T) {
	stop, run := os.Getenv("RASHOMON_T2_J3_STOP"), os.Getenv("RASHOMON_T2_J3_RUN")
	if stop == "" || run == "" {
		t.Skip("RASHOMON_T2_J3_STOP and RASHOMON_T2_J3_RUN not set")
	}
	files, recs := 0, 0
	for i, f := range changedFiles(t, run) {
		rel := strings.TrimPrefix(string(f), "file:")
		content, ok := goContent(t, stop, rel)
		if !ok {
			continue
		}
		out, err := gopg.Resolver{}.Overlay(rel, content, graph.RevCommit(stopCommit), int64(i+1))
		if err != nil {
			t.Fatalf("overlay %s: %v", rel, err)
		}
		for _, r := range out {
			if r.Edge != nil && r.Edge.Pred != graph.Declares {
				t.Errorf("overlay of %s emits %s %s %s; the overlay view must be read again", rel, r.Edge.Src, r.Edge.Pred, r.Edge.Dst)
			}
		}
		files, recs = files+1, recs+len(out)
	}
	t.Logf("overlay over %d changed Go files: %d records, Declares the only edge", files, recs)
}

// readSeed is the seed file as one knowledge File under the product's
// schema.
func readSeed(t *testing.T, path string) knowledge.File {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	var f knowledge.File
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("seed %s: %v", path, err)
	}
	if f.Schema != knowledge.Schema {
		t.Fatalf("seed %s: schema %d, want %d", path, f.Schema, knowledge.Schema)
	}
	return f
}

// seedRules is engine.RulesFor over the seed file's rows and the managed
// spec at specPath ("" for none) as spec.Discover reads it, the managed
// layer alone (what hook.DiscoverSpec reads from the managed directory),
// against repository rid at rev of g. RulesFor is silent where the binary
// is, so each loss is restated here as a failure: a refused row
// (knowledge.Rules' refusals, which RulesFor discards), a spec Errors line
// (the binary ignores that spec), a seed rule RulesFor does not return,
// and a row whose applies_to ids RulesFor thinned (a global id of another
// repository, or one g does not hold at rev).
func seedRules(t *testing.T, path, specPath string, g engine.Reader, rid string, rev graph.Rev) []engine.Rule {
	t.Helper()
	rows := readSeed(t, path).Rows
	compiled, refused := knowledge.Rules(rows)
	for _, r := range refused {
		t.Errorf("seed row %s predicate %s refused: %s", r.Row, r.Predicate, r.Reason)
	}
	if len(compiled) == 0 {
		t.Fatalf("seed %s compiles to no rule", path)
	}
	var sp *spec.Compiled
	nSpec := 0
	if specPath != "" {
		c, err := spec.Discover(spec.OSFS{}, nil, specPath)
		if err != nil {
			t.Fatalf("spec %s: %v", specPath, err)
		}
		if c == nil {
			t.Fatalf("spec %s: no file applies", specPath)
		}
		for _, e := range c.Errors {
			t.Errorf("spec %s: %s; the arms' binaries ignore it", specPath, e)
		}
		for _, d := range c.Rules {
			if k := spec.RuleKind(d.Rule.Kind); k == spec.Boundary || k == spec.ProtectPath {
				nSpec++
			}
		}
		if nSpec == 0 {
			t.Fatalf("spec %s holds no boundary or protect_path rule", specPath)
		}
		t.Logf("spec %s digest %s trust %s: %d rules, %d boundary or protect_path", specPath, c.Digest, c.Trust, len(c.Rules), nSpec)
		sp = c
	}
	out := engine.RulesFor(sp, rows, g, rid, rev)
	held := map[string]int{}
	for _, r := range out {
		if _, dup := held[r.ID]; dup {
			t.Errorf("RulesFor returned rule id %s twice; the per-rule checks below would read one of them", r.ID)
		}
		held[r.ID] = len(r.AppliesTo)
	}
	for _, r := range compiled {
		n, ok := held[r.ID]
		switch {
		case !ok:
			t.Errorf("seed rule %s is not among RulesFor's rules", r.ID)
		case n != len(r.AppliesTo):
			t.Errorf("seed rule %s: %d applies_to ids in its row, %d after RulesFor against %s at %s: an id of another repository or one the graph does not hold was dropped", r.ID, len(r.AppliesTo), n, rid, rev)
		}
	}
	if want := len(compiled) + nSpec; len(out) != want {
		t.Errorf("RulesFor returned %d rules, want %d (%d from the seed, %d from the spec)", len(out), want, len(compiled), nSpec)
	}
	return out
}

// changedFiles is every path the run changed, as file ids, sorted and
// unique: status.txt (git status --porcelain less .rashomon and .pgraph; a
// rename names both sides; an untracked directory's collapsed line is
// skipped) and new-files.txt (every untracked file).
func changedFiles(t *testing.T, run string) []graph.ID {
	t.Helper()
	var out []graph.ID
	for _, name := range []string{"status.txt", "new-files.txt"} {
		b, err := os.ReadFile(filepath.Join(run, name))
		if err != nil {
			t.Fatalf("run %s: %v", run, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if line == "" {
				continue
			}
			if name == "status.txt" {
				if len(line) < 4 {
					t.Fatalf("%s: status line %q", run, line)
				}
				line = line[3:]
			}
			if strings.HasPrefix(line, `"`) {
				t.Fatalf("%s: quoted path %s; the judge reads plain paths only", run, line)
			}
			for _, p := range strings.Split(line, " -> ") {
				if !strings.HasSuffix(p, "/") {
					out = append(out, graph.FileID(p))
				}
			}
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(out)))
}

// goContent is rel's bytes under root when rel is a Go file that exists
// there; a deleted file has no content.
func goContent(t *testing.T, root, rel string) ([]byte, bool) {
	t.Helper()
	if !strings.HasSuffix(rel, ".go") {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return b, true
}

// loadStop is the base index with the final tree's records re-asserted
// under stopCommit: one store holding both revisions (Rev.Admits matches
// commits exactly), the base at its index's commit, and the repository
// the base index names. Both indexes must name one commit and one
// repository, or the stop root was not built from the pinned tree.
//
//lint:ignore U1000 the only caller is TestJ3_ConflictsAtStop in j3_conflicts_test.go, built under the t2conflicts tag, which no CI run builds.
func loadStop(t *testing.T, base, stop string) (*graph.Store, graph.Rev, graph.Rev, string) {
	t.Helper()
	s, h, err := graph.LoadFile(base)
	if err != nil {
		t.Fatalf("index under %s: %v", base, err)
	}
	st, sh, err := graph.LoadFile(stop)
	if err != nil {
		t.Fatalf("index under %s: %v", stop, err)
	}
	if sh.Rev.Commit != h.Rev.Commit {
		t.Fatalf("stop index names %s, base %s; the stop root is not the pinned tree", sh.Rev.Commit, h.Rev.Commit)
	}
	if h.Repo == "" || sh.Repo != h.Repo {
		t.Fatalf("stop index names repository %q, base %q; RulesFor needs one named repository", sh.Repo, h.Repo)
	}
	from, to := graph.RevCommit(sh.Rev.Commit), graph.RevCommit(stopCommit)
	for _, n := range st.Nodes(from) {
		n.Rev = to
		s.AssertNode(n)
	}
	for _, e := range st.Edges(from) {
		e.Rev = to
		s.AssertEdge(e)
	}
	return s, graph.RevCommit(h.Rev.Commit), to, h.Repo
}
