package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

const rid = "github.com/x/y"

var rev = graph.RevCommit("c1")

// indexed is a store with three file nodes and one unit the sources name.
func indexed() *graph.Store {
	s := graph.NewStore()
	for _, id := range []graph.ID{"file:internal/verify/claims.go", "file:cmd/rashomon/graph.go", "file:internal/steer/decide.go"} {
		s.AssertNode(graph.Node{ID: id, Kind: "file", Tier: graph.Proven, Conf: 1, Rev: rev})
	}
	s.AssertNode(graph.Node{ID: "unit:go:internal/verify", Kind: "unit", Tier: graph.Declared, Conf: 1, Rev: rev})
	return s
}

func put(t *testing.T, p, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const ledgerText = `FAILURE LEDGER. Fields, separated by " | ":
id | when (UTC) | who | what happened | pattern / layer | evidence | catcher | today | gap / card

B227 | 11-01 09:53 | manager | closed ROWS-195 by reading the dispatch sha's rows, not the PR head's; QA-1 found an 18th row | stale-evidence / DETECT | MANAGER-NOTES.md:324; memory feedback_check_all_done_against_files | DONE + LRN | no | EVIDENCE-AT-HEAD1: a claim whose last supporting read names a sha other than the current head is unbacked
B231 | 11-01 11:02 | lane 2 | ran rashomon audit --keep through an unquoted heredoc | shell-dialect / PREVENT | MANAGER-NOTES.md:340 | DE | no | quote a heredoc that holds backticks
B233 | 11-01 12:10 | eng-2 | edited internal/verify/claims.go:12 without rerunning TestVER47 in cmd/rashomon/graph.go's package; see MANAGER-NOTES.md:350 | gate-subset / DETECT | merges.log | QA-2 | partial | GATE-ALL1: a changed package reruns every test that names it before "done" is said, under the whole internal/verify unit
B240 11-02 prose row with no separators, left as text
B354 11-03 00:15 PST manager: hand-retyped commit ids twice in 4 minutes. The second sentence stays in the text.
B355 11-03 00:29 PST manager: merged #229 (FALSE-FIRE1, 20f38b62) on CI green and the R67 pair without judging the mutant rows on the merged tree with ` + "`sweep.py --check`" + `; B351's class again, one hour after quoting B351. Caught by: QA-1. Intent missed: main is always sweep-clean, and a merge proves its rows, not only its tests. Fix: until #241 lands, the manager runs sweep.py --check on each merge-tree.
B440 11-04 10:05 PST eng-2: a test of a git read passed in a plain directory and failed in the harness. Lesson: a test of a git read runs in a worktree too, because every scratch tree the harness makes is one.
B250 | 11-01 12:30 | eng-1 | ran the gate on a stale base | stale-evidence / DETECT | merges.log | DONE | no | 
B347 | 11-02 22:1x | manager | cited a green from a gate whose log was never read | evidence / DETECT | gate3 log | QA-1 | no | fixed at 94c2451e, gate3 queued; rule: a green is cited only with its log's exit line
`

// TestImport_LedgerRow (W-M1.6): a nine-field row yields a lesson keyed by
// the ledger's own id, titled by what happened, with the gap field as its
// clause within 160 bytes, the row as its text, and the source's file,
// row and digest; a prose row (id, a date, a who, a colon, the text) yields
// one with its first sentence as title; a line that starts as a row and is
// neither is counted unparsed. What happened is never the clause (W-M1.26):
// a prose row's clause is the sentence after "Intent missed: ", "Rule: "
// or "Lesson: ", a gap field that writes the repair first gives the
// sentence after its "rule: " (B347), and a row with no rule written (a
// prose row with no mark, a nine-field row with an empty gap) is title
// only. The title passes the clause's rules: a narrative with '#' and
// backticks over 160 bytes is written plain and cut at a word, so a brief
// can carry the failure.
func TestImport_LedgerRow(t *testing.T) {
	root := t.TempDir()
	src := put(t, filepath.Join(root, "notes", "FAILURE-LEDGER.txt"), ledgerText)
	rep, err := Import(root, []string{src}, indexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 8 || len(rep.Sources) != 1 || rep.Sources[0].Rows != 8 || rep.Sources[0].Unparsed != 1 {
		t.Fatalf("rows %d, report %+v; want 8 rows and 1 unparsed", len(rows), rep.Sources)
	}
	if p := rows[3]; p.ID != "B354" || p.Kind != KindLesson || p.Title != "hand-retyped commit ids twice in 4 minutes." || p.Clause != "" || !strings.Contains(p.Text, "second sentence") || p.Source.Row != "B354" {
		t.Errorf("prose row = %+v; want its first sentence as title, no clause (no rule written) and the line as text", p)
	}
	if p := rows[4]; p.Clause != "main is always sweep-clean, and a merge proves its rows, not only its tests." || !strings.HasPrefix(p.Title, "merged 229 (FALSE-FIRE1, 20f38b62) on CI green") ||
		strings.ContainsAny(p.Title, "#`") || len(p.Title) > 160 || slot.Approved(p.Title).Validate() != nil || !strings.Contains(p.Text, "#229") {
		t.Errorf("B355 title %q (%d bytes), clause %q; want the failure plain within 160 bytes, the intent missed as the rule, the raw line as text", p.Title, len(p.Title), p.Clause)
	}
	if p := rows[5]; p.Clause != "a test of a git read runs in a worktree too, because every scratch tree the harness makes is one." || p.Title != "a test of a git read passed in a plain directory and failed in the harness." {
		t.Errorf("B440 title %q, clause %q; want the failure and the lesson", p.Title, p.Clause)
	}
	if p := rows[6]; p.ID != "B250" || p.Title != "ran the gate on a stale base" || p.Clause != "" {
		t.Errorf("B250 title %q, clause %q; want title only: an empty gap is no rule", p.Title, p.Clause)
	}
	// B231 names the tool and B233 a path under cmd/rashomon: a banned word
	// either way, so each title stays as written and no brief carries it.
	if p := rows[1]; p.ID != "B231" || p.Title != "ran rashomon audit --keep through an unquoted heredoc" || slot.Approved(p.Title).Validate() == nil {
		t.Errorf("B231 title %q; want it as written: it holds a banned word", p.Title)
	}
	if p := rows[2]; p.ID != "B233" || !strings.Contains(p.Title, "cmd/rashomon/graph.go's package; see MANAGER-NOTES.md:350") {
		t.Errorf("B233 title %q; want it as written: it holds a banned word", p.Title)
	}
	if p := rows[7]; p.ID != "B347" || p.Clause != "a green is cited only with its log's exit line" {
		t.Errorf("B347 clause %q; want the rule its author marked after the repair, not the repair", p.Clause)
	}
	if s := rep.Sources[0]; s.Clauses != 6 || s.TitleOnly != 2 || s.Titles != 6 || len(s.Notes) != 0 {
		t.Errorf("report %+v; want 6 with a clause, 2 title only with no refusal note, 6 titles a brief can carry (B231's and B233's hold a banned word)", s)
	}
	r := rows[0]
	if r.Schema != Schema || r.ID != "B227" || r.Kind != KindLesson || r.Status != StatusCandidate || r.Tier != TierInferred {
		t.Errorf("row = %+v; want schema 1, B227, lesson, candidate, INFERRED", r)
	}
	if !strings.HasPrefix(r.Title, "closed ROWS-195") || r.Text != strings.Split(ledgerText, "\n")[3] {
		t.Errorf("title %q, text %q; want what happened and the whole row", r.Title, r.Text)
	}
	want := "EVIDENCE-AT-HEAD1: a claim whose last supporting read names a sha other than the current head is unbacked"
	if r.Clause != want || len(r.Clause) > 160 || slot.Approved(r.Clause).Validate() != nil {
		t.Errorf("clause = %q; want the gap field, approved", r.Clause)
	}
	if r.Source.File != "FAILURE-LEDGER.txt" || r.Source.Row != "B227" || len(r.Source.Digest) != 64 {
		t.Errorf("source = %+v; want the file's base name, the row id and a sha256", r.Source)
	}
	if rep.Sources[0].Written != filepath.Join(root, Dir, "failure-ledger.json") {
		t.Errorf("written %q", rep.Sources[0].Written)
	}
}

const notesText = `# Approach notes

## D66 (23:41, card 2.3 R7, Gate A A4): the gate word is the shape's program base name

shape.Derive's Program is path.Base of argv[0], so go test ./... renders the base name; the key
is the gate id, so only the text is ambiguous. decide.go:209 holds the key.

## D64 amended (23:47, card 2.3 R10)

The pair rule holds for the amended card. A second sentence.

## Something else

B48 on failed runner calls only: the tail scan reads the last line, which is prose about a row.

- D161 (14:02, owner): the truth-table row travels with the change: every PR that moves a PRD row carries its
  row in test/prd/truth_table.json. The second sentence is not the clause.
- D161 amended (15:10): the row may also be moved by the carrier.
- B5 is not a decision line.
`

// TestImport_DecisionLine (W-M1.6): a "- D161 (...): ..." line and a
// "## D66 (...): ..." heading each yield a decision keyed by the id, the
// heading's text as its title and clause, the first sentence of a dash
// line as its clause and the whole entry as its text; a heading with no
// title takes its paragraph's first sentence; an amended id keys on its
// own; a line that is not a decision is skipped, and a prose line that
// starts with a row id does not make the file a ledger.
func TestImport_DecisionLine(t *testing.T) {
	root := t.TempDir()
	src := put(t, filepath.Join(root, "notes", "APPROACH-NOTES.md"), notesText)
	if _, err := Import(root, []string{src}, indexed(), rid, rev); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Row{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if len(rows) != 4 || byID["D66"].Kind != KindDecision || byID["D161"].Kind != KindDecision || byID["D161-amended"].Kind != KindDecision || byID["D64-amended"].Kind != KindDecision {
		t.Fatalf("rows = %+v; want D66, D64-amended, D161 and D161-amended decisions", rows)
	}
	if d := byID["D64-amended"]; d.Title != "The pair rule holds for the amended card." || d.Clause != d.Title || !strings.Contains(d.Text, "second sentence") || d.Source.Row != "D64-amended" {
		t.Errorf("D64-amended = %+v; want a heading with no title keyed on the id, titled by its paragraph's first sentence", d)
	}
	if d := byID["D66"]; d.Title != "the gate word is the shape's program base name" || d.Clause != d.Title || !strings.Contains(d.Text, "argv[0]") || !strings.Contains(d.Text, "ambiguous") {
		t.Errorf("D66 = %+v; want the heading's text as title and clause and the paragraph as text", d)
	}
	if d := byID["D161"]; d.Clause != "the truth-table row travels with the change: every PR that moves a PRD row carries its row in test/prd/truth_table.json." || !strings.Contains(d.Text, "second sentence") {
		t.Errorf("D161 = %+v; want the first sentence as clause and the wrapped entry as text", d)
	}
	if d := byID["D161-amended"]; d.Source.Row != "D161-amended" || d.Source.File != "APPROACH-NOTES.md" {
		t.Errorf("amended source = %+v", d.Source)
	}
}

// TestImport_ADRDirectory (W-M1.6): a directory of NNNN-title.md records
// yields a decision per file keyed ADR-NNNN, titled by its heading, with
// the Decision section's first sentence as its clause; an accepted record
// is approved and DECLARED, a proposed one a candidate.
func TestImport_ADRDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "adr")
	put(t, filepath.Join(dir, "0007-sidecars.md"), "# 7. Go facts enter through sidecars\n\n## Status\n\nAccepted\n\n## Context\n\nThe hook binary stays small.\n\n## Decision\n\nThe core reads pgraph's sidecars under internal/verify and never imports pgraph. A second sentence.\n\n## Consequences\n\nNone.\n")
	put(t, filepath.Join(dir, "0008-proposal.md"), "# 8. A proposal\n\n## Status\n\nProposed\n\n## Decision\n\nWe might do it.\n")
	put(t, filepath.Join(dir, "README.md"), "# not a record\n")
	rep, err := Import(root, []string{dir}, indexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rep.Sources[0].Written != filepath.Join(root, Dir, "adr.json") {
		t.Fatalf("rows = %+v, report %+v", rows, rep.Sources)
	}
	a := rows[0]
	if a.ID != "ADR-0007" || a.Kind != KindDecision || a.Title != "Go facts enter through sidecars" || a.Status != StatusApproved || a.Tier != TierDeclared ||
		a.Clause != "The core reads pgraph's sidecars under internal/verify and never imports pgraph." || a.Source.File != "adr/0007-sidecars.md" ||
		!reflect.DeepEqual(a.AppliesTo, []string{"g:" + rid + "#unit:go:internal/verify"}) {
		t.Errorf("ADR-0007 = %+v", a)
	}
	if b := rows[1]; b.ID != "ADR-0008" || b.Status != StatusCandidate || b.Tier != TierInferred || b.Clause != "We might do it." {
		t.Errorf("ADR-0008 = %+v", b)
	}
}

// TestImport_ClauseValidates (W-M1.6): every written clause passes the
// approved-text rules or is empty; a clause the rules refuse (a banned
// word, a quote that is no apostrophe) leaves the row title-only and the
// report says so; a long clause is cut at a sentence or word boundary.
func TestImport_ClauseValidates(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("a changed package reruns every test that names it ", 5)
	src := put(t, filepath.Join(root, "L.txt"), "id | a | b | c | d | e | f | g | h\n"+
		"B1 | t | w | banned word row | p | e | c | no | the rashomon binary must not be named\n"+
		"B2 | t | w | url row | p | e | c | no | see https://example.com/x for the rule\n"+
		"B3 | t | w | long row | p | e | c | no | "+long+"\n"+
		"B4 | t | w | unicode and symbols row | p | e | c | no | the `claims` comparator [sees] a done’s run — or none\n"+
		"B5 | t | w | quote row | p | e | c | no | a 'quoted' word\n")
	rep, err := Import(root, []string{src}, indexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Clause != "" && (slot.Approved(r.Clause).Validate() != nil || len(r.Clause) > 160) {
			t.Errorf("%s: clause %q written yet refused", r.ID, r.Clause)
		}
	}
	if rows[0].Clause != "" || rows[1].Clause != "" || rows[0].Title != "banned word row" {
		t.Errorf("B1 %q, B2 %q; want title-only (a banned word, a URL scheme)", rows[0].Clause, rows[1].Clause)
	}
	if c := rows[4].Clause; c != "a quoted word" {
		t.Errorf("B5 clause = %q; want the quotes that are no apostrophe dropped", c)
	}
	if c := rows[2].Clause; c == "" || len(c) > 160 || strings.HasSuffix(c, " ") || !strings.HasPrefix(c, "a changed package") {
		t.Errorf("B3 clause = %q (%d bytes); want a cut at a word boundary", c, len(c))
	}
	if c := rows[3].Clause; c != "the claims comparator sees a dones run or none" {
		t.Errorf("B4 clause = %q; want symbols and non-ASCII folded away", c)
	}
	if rep.Sources[0].Rows != 5 || rep.Sources[0].TitleOnly != 2 || rep.Sources[0].Clauses != 3 || len(rep.Sources[0].Notes) != 2 || !strings.Contains(rep.Sources[0].Notes[0], "B1") {
		t.Errorf("report = %+v; want 5 rows, 2 title-only with a note each, 3 with a clause", rep.Sources[0])
	}
}

// TestImport_AppliesToResolved (W-M1.6, AD14): a path the row names that
// the index holds as a file (by its relative path, or by a base name only
// one file carries, a :line suffix dropped) or as a unit becomes a global
// applies_to id under the repository's rid; a path the index does not
// hold is dropped and counted; Records turns the ids back into local
// edges and ignores another repository's.
func TestImport_AppliesToResolved(t *testing.T) {
	root := t.TempDir()
	src := put(t, filepath.Join(root, "FAILURE-LEDGER.txt"), ledgerText)
	rep, err := Import(root, []string{src}, indexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"g:" + rid + "#file:cmd/rashomon/graph.go", "g:" + rid + "#file:internal/verify/claims.go", "g:" + rid + "#unit:go:internal/verify"}
	if !reflect.DeepEqual(rows[2].AppliesTo, want) {
		t.Errorf("B233 applies_to = %v; want %v", rows[2].AppliesTo, want)
	}
	if len(rows[0].AppliesTo) != 0 || rep.Sources[0].Resolved != 3 || rep.Sources[0].Dropped < 3 {
		t.Errorf("B227 applies_to = %v, report %+v; want nothing resolved for B227 and the notes paths dropped", rows[0].AppliesTo, rep.Sources[0])
	}
	rows[2].AppliesTo = append(rows[2].AppliesTo, "g:github.com/other/repo#file:internal/verify/claims.go", "file:not-global")
	s := graph.NewStore()
	for _, r := range Records(rows, rid, rev) {
		switch {
		case r.Node != nil:
			s.AssertNode(*r.Node)
		case r.Edge != nil:
			s.AssertEdge(*r.Edge)
		}
	}
	n, ok := s.Node("les:B233", rev)
	if !ok || n.Kind != KindLesson || n.Tier != graph.Inferred || n.Attrs["clause"] == "" || n.Attrs["source"] != "FAILURE-LEDGER.txt" || n.Attrs["status"] != StatusCandidate {
		t.Errorf("les:B233 = %+v, %v", n, ok)
	}
	if out := s.Out("les:B233", rev, graph.AppliesTo); len(out) != 3 || out[0].Dst != "file:cmd/rashomon/graph.go" || out[0].Tier != graph.Inferred {
		t.Errorf("edges = %+v; want the three local targets only", out)
	}
}

// TestImport_Idempotent (W-M1.6): a second import of the same sources
// writes the same bytes and reports the same counts.
func TestImport_Idempotent(t *testing.T) {
	root := t.TempDir()
	srcs := []string{put(t, filepath.Join(root, "FAILURE-LEDGER.txt"), ledgerText), put(t, filepath.Join(root, "APPROACH-NOTES.md"), notesText)}
	rep1, err := Import(root, srcs, indexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	read := func() map[string][]byte {
		out := map[string][]byte{}
		es, err := os.ReadDir(filepath.Join(root, Dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			b, _ := os.ReadFile(filepath.Join(root, Dir, e.Name()))
			out[e.Name()] = b
		}
		return out
	}
	first := read()
	rep2, err := Import(root, srcs, indexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, read()) || !reflect.DeepEqual(rep1, rep2) || len(first) != 2 {
		t.Errorf("second import changed the files or the report")
	}
	var f File
	if err := json.Unmarshal(first["failure-ledger.json"], &f); err != nil || f.Schema != Schema || len(f.Rows) != 8 {
		t.Errorf("file = %+v, %v", f, err)
	}
}
