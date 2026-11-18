package engine

import (
	"bufio"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// TestACM_CandidateDump is what the engine computed for each labelled
// request before the band decision (Part F, item 2 of the 2026-11-11
// handoff to the ML engineer): the judged tests print only the unit the
// brief named and the line shown, so a request the brief got wrong cannot be
// told from one the roles stage never saw right. This writes, per label row
// and in label order, one JSON object per line: the truth and the row kind,
// the claim the brief made, the roles stage's reading (why, basis, every
// named word, every pick with every feature UnitPick carries), the rank of
// the first pick that is a truth unit, the scope seeds and the Rank stage's
// candidates with their scores. Each list is capped at candCap entries and
// the uncapped length is written beside it, so nothing is dropped silently.
//
// It is skipped unless three variables are set: RASHOMON_ACM11_DIR (the
// indexed root, as the other ACM tests read it), RASHOMON_ACM_CANDIDATES_LABELS
// (a nine-column label file such as test/live/autosteer/acm11/classes.tsv)
// and RASHOMON_ACM_CANDIDATES_OUT (the file written). Run it with
//
//	RASHOMON_ACM11_DIR=<root> RASHOMON_ACM_CANDIDATES_LABELS=<tsv> \
//	RASHOMON_ACM_CANDIDATES_OUT=<out.jsonl> \
//	go test -count=1 -run TestACM_CandidateDump ./internal/graph/engine
//
// The brief is built exactly as judgeClassRow builds it (the same prompt
// decoding, Options{IDF: true}), and the truth is read as it reads it: the
// third column split on ';', a row's unit being right by unitOKOf (shape 2,
// judgeClassRow's own verdict, field unit_ok) or by unitRight3 (shape 3,
// field unit_right3). So the count of rows with unit_ok true is the judged
// table's unit-right count (the "unit" column of TestACM15_ClassesTable) on
// the same index, row for row. The count of rows whose brief_unit is in
// truth is smaller, and only by the rows where '-' is an alternative of the
// truth (a no-unit row, or one like '-;test/perf'): the judge counts those
// right when the brief holds no unit line, and brief_unit is in no truth
// there. On the four development sets the judge's unit-right count is 96,
// 60, 58 and 65; the rows whose brief_unit is in truth are 69, 35, 35 and
// 40, and no row is in truth and not right, so the gap is those '-' rows,
// 27, 25, 23 and 25, and not a difference in how the truth is read.
//
// The output holds no full prompt and no machine path, but it is not free of
// prompt words: the request's own words appear in target (the phrase the
// roles stage resolved), named_words and named_unknown (word, as the request
// spelled it, and why, which quotes it), roles_why and picks[].refused
// (both quote request words), and picks[].tokens (the request tokens a unit
// matched). Do not point it at labels cut from real sessions and keep the
// output. Everything else is the graph's own, relative to the index root.
// Struct fields, not maps, so the same index and label file give the same
// bytes. The dump reads the Brief the product already builds and changes no
// product behaviour.
func TestACM_CandidateDump(t *testing.T) {
	root := os.Getenv("RASHOMON_ACM11_DIR")
	labels := os.Getenv("RASHOMON_ACM_CANDIDATES_LABELS")
	out := os.Getenv("RASHOMON_ACM_CANDIDATES_OUT")
	if root == "" || labels == "" || out == "" {
		t.Skip("RASHOMON_ACM11_DIR, RASHOMON_ACM_CANDIDATES_LABELS and RASHOMON_ACM_CANDIDATES_OUT not set")
	}
	g, rev := loadACM11(t, root)
	rows := readTSV(t, labels, 9)
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(candidateRow(g, rev, r, candCap)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("candidate dump: %d rows written", len(rows))
}

// candCap is how many entries of each list a dump row carries.
const candCap = 20

// subCap is how many nodes of a pick, or knowledge ids of a candidate, a row
// carries; the length they had is written beside them.
const subCap = 5

// capped is a list cut to at most limit entries, and the length it had.
type capped[T any] struct {
	Len     int `json:"len"`
	Entries []T `json:"entries"`
}

func capList[T any](all []T, limit int) capped[T] {
	n := len(all)
	if n > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []T{}
	}
	return capped[T]{Len: n, Entries: all}
}

type dumpWord struct {
	Word    string `json:"word"`
	Kind    string `json:"kind"`
	Unit    string `json:"unit"`
	Node    string `json:"node"`
	Why     string `json:"why"`
	Refuses bool   `json:"refuses"`
}

type dumpPick struct {
	ID        string         `json:"id"`
	Path      string         `json:"path"`
	Class     string         `json:"class"`
	Covered   float64        `json:"covered"`
	Of        float64        `json:"of"`
	Precision float64        `json:"precision"`
	Mass      float64        `json:"mass"`
	Callable  bool           `json:"callable"`
	Tied      int            `json:"tied"`
	Refused   string         `json:"refused"`
	Tokens    []string       `json:"tokens"`
	Nodes     capped[string] `json:"nodes"`
	IsTruth   bool           `json:"is_truth"`
}

type dumpSeed struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

type dumpCand struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Distance  int            `json:"distance"`
	Tier      string         `json:"tier"`
	Conf      float64        `json:"conf"`
	Base      float64        `json:"base"`
	Lex       float64        `json:"lex"`
	Knowledge capped[string] `json:"knowledge"`
	Reach     bool           `json:"reach"`
	Score     float64        `json:"score"`
	Via       string         `json:"via"`
}

// dumpRow is one request's candidate dump, in the order the fields are
// written.
type dumpRow struct {
	ID    string   `json:"id"`
	Class string   `json:"class"`
	Truth []string `json:"truth"`
	Kind  string   `json:"row_kind"`

	BriefUnit      string `json:"brief_unit"`
	UnitOK         bool   `json:"unit_ok"`
	UnitRight3     bool   `json:"unit_right3"`
	ClaimLine      bool   `json:"claim_line"`
	ClaimCandidate bool   `json:"claim_candidate"`
	ClaimRunner    string `json:"claim_runner_up"`
	Basis          string `json:"basis"`

	Why           string           `json:"roles_why"`
	RolesBasis    string           `json:"roles_basis"`
	Fallback      bool             `json:"fallback"`
	Unnamed       bool             `json:"unnamed"`
	Target        []string         `json:"target"`
	NamedUnit     string           `json:"named_unit"`
	NamedWords    capped[dumpWord] `json:"named_words"`
	NamedUnknown  capped[dumpWord] `json:"named_unknown"`
	Picks         capped[dumpPick] `json:"picks"`
	TruthRank     int              `json:"truth_rank"`
	Seeds         capped[dumpSeed] `json:"seeds"`
	Candidates    capped[dumpCand] `json:"candidates"`
	DecisionCount int              `json:"decisions"`
}

func dumpIDs(in []graph.ID) []string {
	out := make([]string, 0, len(in))
	for _, id := range in {
		out = append(out, string(id))
	}
	return out
}

func dumpWords(in []NamedWord) []dumpWord {
	out := make([]dumpWord, 0, len(in))
	for _, w := range in {
		out = append(out, dumpWord{w.Word, w.Kind, w.Unit, string(w.Node), w.Why, w.Refuses})
	}
	return out
}

// candidateRow builds the brief of one label row as judgeClassRow does and
// reads what the engine computed before the band decision out of it, each
// list cut to limit entries. The env-gated dump and its unit test both run
// this, so the test reads the code that writes the file.
func candidateRow(g *graph.Store, rev graph.Rev, r []string, limit int) dumpRow {
	want := strings.Split(r[2], ";")
	prompt := strings.ReplaceAll(r[8], "\\n", "\n")
	b := Build(g, rev, prompt, Options{IDF: true})
	got := briefUnit(g, rev, b)
	basis, _, has := unitDisclosure(b)
	claim := claimOf(b, got)
	unitOK := unitOKOf(want, got, has)
	ro := b.Roles
	d := dumpRow{
		ID: r[0], Class: r[1], Truth: want, Kind: rowKind(r),
		BriefUnit: got, UnitOK: unitOK, ClaimLine: claim.Line, ClaimCandidate: claim.Candidate, ClaimRunner: claim.Runner, Basis: basis,
		Why: ro.Why, RolesBasis: ro.Basis, Fallback: ro.Fallback, Unnamed: ro.Unnamed,
		Target: append([]string{}, ro.Target...), NamedUnit: ro.Named.Unit,
		NamedWords:    capList(dumpWords(ro.Named.Words), limit),
		NamedUnknown:  capList(dumpWords(ro.Named.Unknown), limit),
		DecisionCount: len(b.Decisions),
	}
	d.UnitRight3 = unitRight3(d.Kind, want, claim, unitOK)
	picks := make([]dumpPick, 0, len(ro.Picks))
	for i, p := range ro.Picks {
		isTruth := slices.Contains(want, p.Path)
		if isTruth && d.TruthRank == 0 {
			d.TruthRank = i + 1
		}
		nodes := capList(dumpIDs(p.Nodes), subCap)
		picks = append(picks, dumpPick{string(p.ID), p.Path, p.Class, p.Covered, p.Of, p.Precision, p.Mass,
			p.Callable, p.Tied, p.Refused, append([]string{}, p.Tokens...), nodes, isTruth})
	}
	d.Picks = capList(picks, limit)
	seeds := make([]dumpSeed, 0, len(b.Seeds))
	for _, s := range b.Seeds {
		seeds = append(seeds, dumpSeed{string(s.ID), s.Score})
	}
	d.Seeds = capList(seeds, limit)
	cands := make([]dumpCand, 0, len(b.Candidates))
	for _, c := range b.Candidates {
		k := capList(dumpIDs(c.Knowledge), subCap)
		cands = append(cands, dumpCand{string(c.ID), c.Kind, c.Distance, string(c.Tier), c.Conf, c.Base, c.Lex, k, c.Reach, c.Score, c.Via})
	}
	d.Candidates = capList(cands, limit)
	return d
}

// A candidate's knowledge ids are cut like every other list, and the length
// they had stays beside them.
func TestACM_CandidateRowKeepsTheSubListLengths(t *testing.T) {
	names := []string{"continuity_state", "continuity_load", "continuity_save", "continuity_drop", "continuity_walk", "continuity_mark", "continuity_open"}
	var syms []synSym
	for _, n := range names {
		syms = append(syms, fn(n))
	}
	g := synth(t, []synFile{{"cmd", "cmd/main.py", syms}}, nil, nil)
	for _, id := range []string{"dec:K1", "dec:K2", "dec:K3", "dec:K4", "dec:K5", "dec:K6"} {
		record(t, g, id, "decision", "cmd/main.py#continuity_state")
	}
	rev := graph.RevCommit(graphtest.Commit)
	r := []string{"R3", "prose", "cmd", "-", "-", "-", "-", "-", "the continuity state should name the open requirements"}
	d := candidateRow(g, rev, r, candCap)

	if len(d.Picks.Entries) == 0 {
		t.Fatal("no picks")
	}
	if n := d.Picks.Entries[0].Nodes; n.Len < subCap+1 || len(n.Entries) != subCap {
		t.Errorf("pick nodes %d/%d, want more than %d held and %d kept", len(n.Entries), n.Len, subCap, subCap)
	}
	found := false
	for _, c := range d.Candidates.Entries {
		if c.Knowledge.Len > subCap {
			found = true
			if len(c.Knowledge.Entries) != subCap || c.Knowledge.Len != 6 {
				t.Errorf("%s knowledge %d/%d, want 5/6", c.ID, len(c.Knowledge.Entries), c.Knowledge.Len)
			}
		}
	}
	if !found {
		t.Error("no candidate holds more than five knowledge ids; the fixture no longer exercises the cut")
	}
}

// A dump row's unit verdicts are the judge's: unit_ok is judgeClassRow's
// UnitOK on the same row, and unit_right3 is unitRight3 over it, so a count
// over the dump is a count of the judged table.
func TestACM_CandidateRowUnitVerdictsAreTheJudges(t *testing.T) {
	g := synth(t, []synFile{
		{"cmd", "cmd/main.py", []synSym{fn("continuity_state"), fn("resume_briefing")}},
		{"pattern", "pattern/pattern.py", []synSym{{kind: "const", name: "CONTINUITY_BRIEFING"}}},
	}, nil, nil)
	rev := graph.RevCommit(graphtest.Commit)
	const p = "the continuity briefing should name the open requirements"
	rows := [][]string{
		{"A", "prose", "cmd", "-", "-", "-", "-", "-", p},
		{"B", "prose", "pattern", "-", "-", "-", "-", "-", p},
		{"C", "prose", "cmd;pattern", "-", "-", "-", "-", "-", p},
		{"D", "prose", "-", "-", "-", "-", "-", "-", p},
		{"E", "named-path", "cmd", "-", "-", "-", "-", "-", "change cmd/main.py to log the continuity state"},
		{"F", "no-unit", "-", "-", "-", "-", "-", "-", "what time is it"},
	}
	for _, r := range rows {
		v := judgeClassRow(t, g, rev, r, "test")
		d := candidateRow(g, rev, r, candCap)
		if d.UnitOK != v.UnitOK {
			t.Errorf("%s: dump unit_ok %v, judge %v (brief unit %q, truth %q)", r[0], d.UnitOK, v.UnitOK, d.BriefUnit, r[2])
		}
		if want := unitRight3(rowKind(r), strings.Split(r[2], ";"), v.Claim, v.UnitOK); d.UnitRight3 != want {
			t.Errorf("%s: dump unit_right3 %v, judge's %v", r[0], d.UnitRight3, want)
		}
	}
}

// A dump row holds the rank of the first truth pick (0 when none is), the
// picks with their class, the named words, and the length each cut list had.
func TestACM_CandidateRowReadsTheRolesStage(t *testing.T) {
	g := synth(t, []synFile{
		{"cmd", "cmd/main.py", []synSym{fn("continuity_state"), fn("resume_briefing")}},
		{"pattern", "pattern/pattern.py", []synSym{{kind: "const", name: "CONTINUITY_BRIEFING"}}},
	}, nil, nil)
	rev := graph.RevCommit(graphtest.Commit)
	prompt := "the continuity briefing should name the open requirements"
	row := func(truth string) []string {
		return []string{"R1", "prose", truth, "-", "-", "-", "-", "-", prompt}
	}

	first := candidateRow(g, rev, row("cmd"), candCap)
	if first.TruthRank != 1 || first.Picks.Len != 2 || len(first.Picks.Entries) != 2 {
		t.Fatalf("truth cmd: rank %d, picks %d/%d; want 1, 2/2", first.TruthRank, first.Picks.Len, len(first.Picks.Entries))
	}
	if p := first.Picks.Entries[0]; p.Path != "cmd" || p.Class != ClassFull || !p.Callable || !p.IsTruth || p.Tokens == nil || p.Of == 0 {
		t.Errorf("first pick %+v, want the cmd unit's full, callable claim marked as truth", p)
	}
	if first.Kind != kindProse || first.Truth[0] != "cmd" || first.Class != "prose" || first.ID != "R1" {
		t.Errorf("row identity %q %q %q %v", first.ID, first.Class, first.Kind, first.Truth)
	}

	if second := candidateRow(g, rev, row("pattern"), candCap); second.TruthRank != 2 || second.Picks.Entries[1].Path != "pattern" || second.Picks.Entries[0].IsTruth {
		t.Errorf("truth pattern: rank %d, want 2 with only the second pick marked", second.TruthRank)
	}
	if none := candidateRow(g, rev, row("elsewhere"), candCap); none.TruthRank != 0 {
		t.Errorf("truth elsewhere: rank %d, want 0 (absent from the picks)", none.TruthRank)
	}

	// A path the prompt names is recorded as a named word with its unit.
	named := candidateRow(g, rev, []string{"R2", "named-path", "cmd", "-", "-", "-", "-", "-", "change cmd/main.py to log the continuity state"}, candCap)
	if named.NamedWords.Len == 0 || named.NamedUnit != "cmd" {
		t.Errorf("named words %d, named unit %q; want a word and cmd", named.NamedWords.Len, named.NamedUnit)
	}

	// The cap cuts each list and keeps the length it had.
	cut := candidateRow(g, rev, row("cmd"), 1)
	if cut.Picks.Len != 2 || len(cut.Picks.Entries) != 1 {
		t.Errorf("cap 1: picks %d/%d, want 2/1", cut.Picks.Len, len(cut.Picks.Entries))
	}
	if cut.TruthRank != 1 || cut.Candidates.Len != first.Candidates.Len || len(cut.Candidates.Entries) > 1 || len(cut.Seeds.Entries) > 1 {
		t.Errorf("cap 1: rank %d, candidates %d (want %d), entries %d, seeds %d", cut.TruthRank, cut.Candidates.Len, first.Candidates.Len, len(cut.Candidates.Entries), len(cut.Seeds.Entries))
	}
	if first.Seeds.Len == 0 || first.Candidates.Len == 0 {
		t.Errorf("seeds %d, candidates %d; want both read from the brief", first.Seeds.Len, first.Candidates.Len)
	}

	// Same input, same bytes.
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(candidateRow(g, rev, row("cmd"), candCap))
	if string(a) != string(b) {
		t.Errorf("two builds of one row differ:\n%s\n%s", a, b)
	}
}
