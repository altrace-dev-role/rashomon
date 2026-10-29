package prd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
)

// The PRD implementation truth table (card TRUTH-TABLE1, D148): what each of
// the PRD's 44 rows has on this tree, where it is, and through which installed
// hook event or verb a session reaches it. The rows are data,
// truth_table.json; this file is the checks that make a stale row fail the
// build instead of misleading a reader. A row changes in the same PR as the
// code it describes.
//
// Every check is a plain read: one stat and one text scan per cited file, no
// go/parser and no subprocess, so the table costs well under a second.

// prdName is the PRD the rows are read against.
const prdName = "autosteer-prd-2026-10-21"

// table is truth_table.json. Field order is key order and the keys are
// sorted, so the file must be exactly what the encoder writes for its own
// decoding: another field, another order or another indent fails.
type table struct {
	PRD  string `json:"prd"`
	Rows []row  `json:"rows"`
	Tree string `json:"tree"`
}

// row is one PRD id. Status is built, partial or none, and built and partial
// cite evidence. SessionProof is a path under the record store that shows the
// row exercised in a real Claude Code session; E2EVerified is true only with
// one.
type row struct {
	E2EVerified    bool          `json:"e2e_verified"`
	Evidence       []evidence    `json:"evidence"`
	ID             string        `json:"id"`
	Implementation string        `json:"implementation"`
	Integration    []integration `json:"integration"`
	Missing        []string      `json:"missing"`
	PlannedPR      string        `json:"planned_pr"`
	SessionProof   string        `json:"session_proof"`
	Status         string        `json:"status"`
	Title          string        `json:"title"`
}

// evidence is a module-relative file and, optionally, a func, method or type
// it declares.
type evidence struct {
	File   string `json:"file"`
	Symbol string `json:"symbol,omitempty"`
}

// integration is how a session reaches the row: an installed hook event, a
// verb, or none, and the file that handles it ("" for none).
type integration struct {
	Event   string `json:"event"`
	Handler string `json:"handler"`
}

// prdIDs is the 44 ids in the PRD's order: PG 1-13, PL 1-14, HD 1-17.
func prdIDs() []string {
	var ids []string
	for _, fam := range []struct {
		prefix string
		n      int
	}{{"PF-PG", 13}, {"PF-PL", 14}, {"PF-HD", 17}} {
		for i := 1; i <= fam.n; i++ {
			ids = append(ids, fam.prefix+strconv.Itoa(i))
		}
	}
	return ids
}

// events is what an integration may name: the events the installer writes,
// read from install.Events so the set follows the installer, and "verb" and
// "none". SubagentStop and PreCompact are not installed, so no row reaches
// a session through them.
func events() map[string]bool {
	ev := map[string]bool{"verb": true, "none": true}
	for _, e := range install.Events() {
		ev[e] = true
	}
	return ev
}

// checkTable is every check over the table's bytes. Each problem opens with
// its row and check, "<id>: <check>: ", or with "table: " for the file.
func checkTable(root string, b []byte) []string {
	var tab table
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tab); err != nil {
		return []string{"table: does not decode as the table: " + err.Error()}
	}
	var probs []string
	if canon, err := encode(tab); err != nil || !bytes.Equal(canon, b) {
		probs = append(probs, "table: not in canonical form (sorted keys, two-space indent, trailing newline)"+firstDiff(canon, b))
	}
	if tab.PRD != prdName {
		probs = append(probs, fmt.Sprintf("table: prd %q, want %q", tab.PRD, prdName))
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(tab.Tree) {
		probs = append(probs, fmt.Sprintf("table: tree %q is not a full commit id", tab.Tree))
	}
	probs = append(probs, checkIDs(tab.Rows)...)
	f := &files{root: root, text: map[string]string{}}
	ev := events()
	for i, r := range tab.Rows {
		probs = append(probs, checkRow(name(i, r), r, f, ev)...)
	}
	return probs
}

// name is how a problem names a row: its id, or its index when it has none.
func name(i int, r row) string {
	if r.ID == "" {
		return "row " + strconv.Itoa(i)
	}
	return r.ID
}

// checkIDs is R2 (a): every PRD id exactly once, in the PRD's order, and no
// other.
func checkIDs(rows []row) []string {
	want := prdIDs()
	at := make(map[string]int, len(want))
	for i, id := range want {
		at[id] = i
	}
	var probs []string
	seen := map[string]bool{}
	last := -1
	for i, r := range rows {
		pos, known := at[r.ID]
		switch {
		case !known:
			probs = append(probs, name(i, r)+": ids: not one of the PRD's 44")
		case seen[r.ID]:
			probs = append(probs, r.ID+": ids: duplicated")
		case pos < last:
			probs = append(probs, r.ID+": ids: out of the PRD's order")
		default:
			last = pos
		}
		seen[r.ID] = true
	}
	for _, id := range want {
		if !seen[id] {
			probs = append(probs, id+": ids: missing")
		}
	}
	return probs
}

// checkRow is R2 (b) to (g) for one row: the status word, evidence behind
// built and partial, each cited file and symbol, each integration's event and
// handler, and a proof behind a verified row.
func checkRow(id string, r row, f *files, ev map[string]bool) []string {
	var probs []string
	fail := func(check, format string, a ...any) {
		probs = append(probs, id+": "+check+": "+fmt.Sprintf(format, a...))
	}
	switch r.Status {
	case "built", "partial":
		if len(r.Evidence) == 0 {
			fail("evidence", "%s cites no evidence", r.Status)
		}
	case "none":
	default:
		fail("status", "%q is not built, partial or none", r.Status)
	}
	for _, e := range r.Evidence {
		text, err := f.read(e.File)
		if err != nil {
			fail("evidence", "%s: %v", e.File, err)
			continue
		}
		if e.Symbol != "" && !declares(text, e.Symbol) {
			fail("symbol", "%s declares no func, method or type %s", e.File, e.Symbol)
		}
	}
	for _, in := range r.Integration {
		if !ev[in.Event] {
			fail("integration", "event %q is not an installed event, verb or none", in.Event)
		}
		if in.Handler != "" {
			if _, err := f.read(in.Handler); err != nil {
				fail("integration", "handler %s: %v", in.Handler, err)
			}
		}
	}
	if r.E2EVerified && r.SessionProof == "" {
		fail("e2e", "verified with no session_proof")
	}
	return probs
}

// files reads module-relative files, each once.
type files struct {
	root string
	text map[string]string
}

// read is the file's text. Its error says the path is not module-relative or
// the file is not there (R2 d).
func (f *files) read(rel string) (string, error) {
	if t, ok := f.text[rel]; ok {
		return t, nil
	}
	if strings.Contains(rel, `\`) || !filepath.IsLocal(filepath.FromSlash(rel)) {
		return "", errors.New("not a module-relative path")
	}
	b, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", errors.New("no such file under the module root")
	}
	if err != nil {
		return "", err
	}
	f.text[rel] = string(b)
	return string(b), nil
}

// declares is R2 (e)'s text scan: a line that opens `func sym(`, a method's
// `func (recv) sym(`, or `type sym `, type parameters allowed after either
// name. A var, a const or a name inside a grouped type block is not found;
// cite the file alone for those.
func declares(text, sym string) bool {
	q := regexp.QuoteMeta(sym)
	return regexp.MustCompile(`(?m)^(func (\([^)]*\) )?` + q + `[(\[]|type ` + q + `[ \[])`).MatchString(text)
}

// encode is the table's canonical bytes: the encoder's, two-space indent, no
// HTML escaping, and the trailing newline Encode writes.
func encode(tab table) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	err := enc.Encode(tab)
	return buf.Bytes(), err
}

// firstDiff names the line of b where it first parts from want.
func firstDiff(want, b []byte) string {
	i := 0
	for i < len(want) && i < len(b) && want[i] == b[i] {
		i++
	}
	return fmt.Sprintf("; first difference at line %d", bytes.Count(b[:i], []byte("\n"))+1)
}

// moduleRoot is the directory of the nearest go.mod above the test's own.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		up := filepath.Dir(dir)
		if up == dir {
			t.Fatal("no go.mod above the test's directory")
		}
		dir = up
	}
}

// TestPRD1_TheTableHoldsOnThisTree: truth_table.json passes every check on
// this tree. A row whose file moved, whose symbol was renamed, or whose event
// the installer no longer writes fails here by name.
func TestPRD1_TheTableHoldsOnThisTree(t *testing.T) {
	b, err := os.ReadFile("truth_table.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range checkTable(moduleRoot(t), b) {
		t.Error(p)
	}
}

// valid is a table every check passes: the 44 ids in order, the first built,
// citing this file and go.mod and reached at Stop, the rest none and not
// reachable.
func valid() table {
	rows := make([]row, 0, 44)
	for _, id := range prdIDs() {
		rows = append(rows, row{ID: id, Title: id, Status: "none", Implementation: "Not built.",
			Evidence: []evidence{}, Integration: []integration{{Event: "none"}}, Missing: []string{}})
	}
	rows[0].Status = "built"
	rows[0].Evidence = []evidence{{File: "test/prd/truth_table_test.go", Symbol: "checkTable"}, {File: "go.mod"}}
	rows[0].Integration = []integration{{Event: install.EventStop, Handler: "test/prd/truth_table_test.go"}}
	return table{PRD: prdName, Rows: rows, Tree: strings.Repeat("0", 40)}
}

func mustEncode(t *testing.T, tab table) []byte {
	t.Helper()
	b, err := encode(tab)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func hasPrefix(probs []string, prefix string) bool {
	for _, p := range probs {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// TestPRD1b_EachCheckNamesItsRow: each defect R2 names, put into a table that
// otherwise passes, is reported against its own row and check: an absent or
// non-local file, an absent symbol or a const cited as one, a missing,
// duplicated, foreign or misordered id, an unknown status, a partial row
// with no evidence, an event the installer does not write, an absent
// handler, and a verified row with no proof.
func TestPRD1b_EachCheckNamesItsRow(t *testing.T) {
	root := moduleRoot(t)
	if p := checkTable(root, mustEncode(t, valid())); len(p) != 0 {
		t.Fatalf("the valid table fails: %q", p)
	}
	for _, c := range []struct {
		name, want string
		edit       func(*table)
	}{
		{"an absent file", "PF-PG1: evidence: ", func(tb *table) { tb.Rows[0].Evidence[1].File = "no/such/file.go" }},
		{"a path outside the module", "PF-PG1: evidence: ", func(tb *table) { tb.Rows[0].Evidence[1].File = "../go.mod" }},
		{"an absent symbol", "PF-PG1: symbol: ", func(tb *table) { tb.Rows[0].Evidence[0].Symbol = "noSuchSymbol" }},
		{"a const cited as a symbol", "PF-PG1: symbol: ", func(tb *table) { tb.Rows[0].Evidence[0].Symbol = "prdName" }},
		{"a missing id", "PF-HD17: ids: ", func(tb *table) { tb.Rows = tb.Rows[:43] }},
		{"a duplicated id", "PF-PG2: ids: ", func(tb *table) { tb.Rows[2].ID = "PF-PG2" }},
		{"an id outside the 44", "PF-PG14: ids: ", func(tb *table) { tb.Rows[1].ID = "PF-PG14" }},
		{"an id out of order", "PF-PG1: ids: ", func(tb *table) { tb.Rows[0], tb.Rows[1] = tb.Rows[1], tb.Rows[0] }},
		{"an unknown status", "PF-PG2: status: ", func(tb *table) { tb.Rows[1].Status = "done" }},
		{"a partial row with no evidence", "PF-PG2: evidence: ", func(tb *table) { tb.Rows[1].Status = "partial" }},
		{"an event the installer does not write", "PF-PG1: integration: ",
			func(tb *table) { tb.Rows[0].Integration[0].Event = "SubagentStop" }},
		{"an absent handler", "PF-PG1: integration: ",
			func(tb *table) { tb.Rows[0].Integration[0].Handler = "no/such/handler.go" }},
		{"verified with no proof", "PF-PG1: e2e: ", func(tb *table) { tb.Rows[0].E2EVerified = true }},
	} {
		tb := valid()
		c.edit(&tb)
		if got := checkTable(root, mustEncode(t, tb)); !hasPrefix(got, c.want) {
			t.Errorf("%s: problems %q, want one opening %q", c.name, got, c.want)
		}
	}
}

// TestPRD1c_OnlyTheCanonicalBytesPass: a field outside the schema does not
// decode, and the same table with keys out of order, tab indents or no
// trailing newline is not canonical; each is a table problem, never a pass.
func TestPRD1c_OnlyTheCanonicalBytesPass(t *testing.T) {
	root := moduleRoot(t)
	b := mustEncode(t, valid())
	for _, c := range []struct {
		name, want string
		in         []byte
	}{
		{"a field outside the schema", "table: does not decode",
			bytes.Replace(b, []byte(`"id": "PF-PG1",`), []byte(`"id": "PF-PG1", "verified_by": "x",`), 1)},
		{"keys out of order", "table: not in canonical form",
			bytes.Replace(b, []byte("\"planned_pr\": \"\",\n      \"session_proof\": \"\","),
				[]byte("\"session_proof\": \"\",\n      \"planned_pr\": \"\","), 1)},
		{"tab indents", "table: not in canonical form", bytes.ReplaceAll(b, []byte("  "), []byte("\t"))},
		{"no trailing newline", "table: not in canonical form", bytes.TrimSuffix(b, []byte("\n"))},
	} {
		if bytes.Equal(c.in, b) {
			t.Fatalf("%s: the edit changed nothing", c.name)
		}
		if got := checkTable(root, c.in); !hasPrefix(got, c.want) {
			t.Errorf("%s: problems %q, want one opening %q", c.name, got, c.want)
		}
	}
}
