package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The converse of spec.UnenforcedRuleKinds. The list is defined on one
// source, the spec file: it names the kinds whose rule written there
// changes no verdict, ledger row or finding. These tests show, through the
// real entries (cmdRecap pending for a prompt, cmdPost for a call, cmdRecap
// for a Stop) over cfProject, whose spec the store binds as trusted, that a
// spec rule of every other kind changes one; that a decision compiled from
// a knowledge row, which is outside the list, changes one too; and they pin
// the known limits the list's words name.
//
// A read is what a row's act calls give after its prompt: their stdout, and
// what they append to each ndjson file of the session's run directory. The
// prompt is setup, outside the read: the prompt reader names any rule whose
// Paths cover the prompt's file, of any kind, and a name is not a finding.
// Each store draws its own key and each record its own clock, so two
// sessions never hold equal bytes. kindMask replaces each distinct UUID and
// hex digest with its first-seen ordinal in its class, so which records
// share a key is still compared, and each RFC 3339 time and epoch-ms number
// with one constant, since which records share a millisecond is chance. The
// session's own temporary directories are replaced by fixed names. What the
// mask cannot see: a rule that changes only a clock, or only the value of a
// key or digest while keeping its pattern of repeats.

// kindBaseRule is in the spec of every read, with the row's rule or not: a
// protect_path rule over a directory no call touches, so a v5 store keeps a
// conflict base in both reads and only the row's rule differs.
const kindBaseRule = `{"id": "p0", "kind": "protect_path", "paths": ["none/*.go"], "why": "w", "hard_control": "h"}`

func kindSpec(rules ...string) string {
	all := kindBaseRule
	for _, r := range rules {
		if r != "" {
			all += ", " + r
		}
	}
	return `{"rashomon_spec": 1, "rules": [` + all + `]}`
}

// kindRow is one case: a spec rule, or a knowledge row the decision rule
// compiles from, the record scope it is read at, the prompt that sets the
// session up ("" for none) and the real entry calls read after it, their
// stdout joined. control, on a limit row, is a spec rule the same calls
// must answer to, so the read can see a change at that scope; witness,
// where set, shows the row's rule reached the reader whose silence the row
// pins. finding, on an enforced or knowledge row, is a token the rule's
// added lines must hold, and controlFinding, on a limit row, the token the
// control's must hold: a post writes a conflict_check record, possibly
// empty, for any edit a rule scopes (card D4), so a read that only differs
// shows no finding. opens, where set, is a spec rule in every read of the
// row, so the check whose silence the row pins runs in each.
type kindRow struct {
	name           string
	kind           spec.RuleKind
	scope          string
	rule           string
	row            *knowledge.Row
	finding        string
	control        string
	controlFinding string
	opens          string
	prompt         string
	act            func(s *planSession) string
	witness        func(t *testing.T, s *planSession)
}

type kindRead struct{ out, records string }

func (r kindRead) equal(o kindRead) bool { return r.out == o.out && r.records == o.records }

// kindReadOf runs r in a new session at r's scope with the session content
// marker set, over cfProject, under the base rule, r.opens and rule, with row in the
// project's knowledge where it is not nil.
func kindReadOf(t *testing.T, r kindRow, rule string, row *knowledge.Row) (kindRead, *planSession) {
	t.Helper()
	s := ubPostSession(t, r.scope)
	s.project = cfProject(t)
	cfBindSpecText(t, s.root, s.project, kindSpec(r.opens, rule))
	if row != nil {
		if _, err := knowledge.WriteFile(s.project, "d", []knowledge.Row{*row}); err != nil {
			t.Fatal(err)
		}
	}
	if r.prompt != "" {
		s.prompt(r.prompt)
	}
	_, from := kindRecords(t, s, nil)
	out := r.act(s)
	records, _ := kindRecords(t, s, from)
	return kindRead{out: kindPlaces(s, out), records: kindMask(kindPlaces(s, records))}, s
}

// kindRecords is what each ndjson file of the session's run directory holds
// past from (each file's size at an earlier read), in name order, each
// under its name, and each file's size now.
func kindRecords(t *testing.T, s *planSession, from map[string]int) (string, map[string]int) {
	t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob(filepath.Join(st.RunDir(benchSessionID), "*.ndjson"))
	if err != nil || len(names) == 0 {
		t.Fatalf("run directory files %v, %v; want at least the coverage record", names, err)
	}
	slices.Sort(names)
	var b strings.Builder
	sizes := map[string]int{}
	for _, n := range names {
		data, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(n)
		sizes[name] = len(data)
		fmt.Fprintf(&b, "== %s\n%s", name, data[min(from[name], len(data)):])
	}
	return b.String(), sizes
}

// kindPlaces replaces the session's own temporary directories, as given and
// as resolved, with fixed names.
func kindPlaces(s *planSession, text string) string {
	for _, p := range []struct{ dir, name string }{{s.project, "<project>"}, {s.root, "<store>"}} {
		if real, err := filepath.EvalSymlinks(p.dir); err == nil && real != p.dir {
			text = strings.ReplaceAll(text, real, p.name)
		}
		text = strings.ReplaceAll(text, p.dir, p.name)
	}
	return text
}

var kindMaskClasses = []struct {
	class   string
	ordinal bool
	re      *regexp.Regexp
}{
	{"U", true, regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)},
	{"W", false, regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)},
	{"D", true, regexp.MustCompile(`[0-9a-f]{16,}`)},
	{"T", false, regexp.MustCompile(`\b1\d{12}\b`)},
}

// kindMask replaces each distinct token of an ordinal class with its
// first-seen ordinal in that class (<D1>, <D2>, ...), and each token of a
// clock class with the class's one name (<T>).
func kindMask(text string) string {
	for _, c := range kindMaskClasses {
		seen := map[string]string{}
		text = c.re.ReplaceAllStringFunc(text, func(tok string) string {
			if !c.ordinal {
				return "<" + c.class + ">"
			}
			o, ok := seen[tok]
			if !ok {
				o = fmt.Sprintf("<%s%d>", c.class, len(seen)+1)
				seen[tok] = o
			}
			return o
		})
	}
	return text
}

// stop runs the real Stop entry (recap) for the session.
func (s *planSession) stop() string {
	s.t.Helper()
	head := newBenchHead("Stop")
	head.CWD = s.project
	body := benchJSON(s.t, struct {
		benchHead
		StopHookActive       bool   `json:"stop_hook_active"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}{head, false, "Done: the change is in and the tests pass."})
	var out, errOut bytes.Buffer
	if code := cmdRecap([]string{install.Marker, s.id}, bytes.NewReader(body), &out, &errOut); code != exitOK || errOut.Len() != 0 {
		s.t.Fatalf("stop exited %d, stderr %q", code, errOut.String())
	}
	return out.String()
}

func (s *planSession) bash(useID, line string) string {
	return s.call(true, "Bash", useID, map[string]any{"command": line})
}

// declareLand is land after the call's PreToolUse, as the harness runs a
// Write: a post with no declaration leaves the record incomplete, and a
// Stop judges no rule over an incomplete record.
func (s *planSession) declareLand(useID, rel, body string) string {
	s.t.Helper()
	path := filepath.Join(s.project, filepath.FromSlash(rel))
	return s.call(false, "Write", useID, map[string]any{"file_path": path, "content": body}) + s.land(useID, rel, body)
}

// kindDecisionRow is an approved, declared knowledge decision with one
// predicate, the row knowledge.Rules compiles to a decision rule.
func kindDecisionRow(pred, arg string, appliesTo ...string) *knowledge.Row {
	return &knowledge.Row{Schema: knowledge.Schema, ID: "D1", Kind: knowledge.KindDecision, Title: "leave it", Clause: "it stays as it is",
		Status: knowledge.StatusApproved, Tier: knowledge.TierDeclared, AppliesTo: appliesTo,
		Predicates: []knowledge.Predicate{{Kind: pred, Arg: arg}}}
}

// kindAdded is the lines of with that base does not hold, as many times as
// with holds them more often.
func kindAdded(base, with kindRead) string {
	have := map[string]int{}
	for _, l := range strings.Split(base.out+"\n"+base.records, "\n") {
		have[l]++
	}
	var out []string
	for _, l := range strings.Split(with.out+"\n"+with.records, "\n") {
		if have[l] > 0 {
			have[l]--
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

const (
	kindKeepEdit  = "package keep\n\nfunc K() { panic(\"k\") }\n"
	kindGraphFunc = "package graph\n\nfunc G() {}\n"

	kindBoundaryRule  = `{"id": "k1", "kind": "boundary", "from": ["unit:go:internal/graph"], "to": ["unit:go:internal/hookio"], "why": "w", "hard_control": "h"}`
	kindUntouchedRule = `{"id": "k1", "kind": "decision", "predicate": "paths_untouched", "paths": ["keep/*.go"], "why": "w", "hard_control": "h"}`
	kindProtectKeep   = `{"id": "c1", "kind": "protect_path", "paths": ["keep/*.go"], "why": "w", "hard_control": "h"}`
	kindProtectGraph  = `{"id": "c1", "kind": "protect_path", "paths": ["internal/graph/*.go"], "why": "w", "hard_control": "h"}`

	// kindInertBoundary scopes an edit in internal/graph for the conflict
	// check and is crossed by no edit here: G imports and calls nothing.
	kindInertBoundary = `{"id": "k0", "kind": "boundary", "from": ["unit:go:internal/graph"], "to": ["unit:go:keep"], "why": "w", "hard_control": "h"}`

	kindProtectedRow = `"kind":"protected_path"`
	kindFrozenPath   = `"kind":"frozen-path"`
)

var (
	kindFixKeep  = "fix " + cfKeep
	kindFixGraph = "fix " + cfGraph
)

func kindLandKeep(s *planSession) string { return s.land("toolu_k1", cfKeep, kindKeepEdit) }

func kindLandKeepStop(s *planSession) string {
	return s.declareLand("toolu_k1", cfKeep, kindKeepEdit) + s.stop()
}

func kindCrossGraph(s *planSession) string { return s.land("toolu_k1", cfGraph, cfCross) }

func kindLandGraphFunc(s *planSession) string { return s.land("toolu_k1", cfGraph, kindGraphFunc) }

// kindSymbolPresentRow is a symbol_present decision over internal/graph
// naming a symbol G does not reach.
func kindSymbolPresentRow() *knowledge.Row {
	return kindDecisionRow(spec.PredicateSymbolPresent, "", "sym:go:internal/hookio:func:H", "unit:go:internal/graph")
}

// kindBaseHoldsSymbolPresent shows the conflict base holds the
// symbol_present rule, so the row's silence is not the rule's absence.
func kindBaseHoldsSymbolPresent(t *testing.T, s *planSession) {
	b, err := os.ReadFile(s.basePath())
	if err != nil || !bytes.Contains(b, []byte(`"symbol_present"`)) {
		t.Errorf("the conflict base does not hold the symbol_present rule (read %v), so the row's silence proves nothing", err)
	}
}

// kindSpecRow is the one predicate the list's membership is read on: r is
// driven by a rule in the project's spec file, which the store binds as
// trusted, and by no knowledge row.
func kindSpecRow(r kindRow) bool { return r.row == nil && r.rule != "" }

// kindRows are the enforced kinds' cases, one per kind outside
// spec.UnenforcedRuleKinds, each a spec file's rule at record scope v5.
var kindRows = []kindRow{
	{name: "spec forbid_command", kind: spec.ForbidCommand, scope: store.RecordScopeV5,
		rule:    `{"id": "k1", "kind": "forbid_command", "command": {"argv": ["git", "push"], "flags": ["--force"]}, "why": "w", "hard_control": "h"}`,
		finding: `"reason":"rule_hit"`,
		act:     func(s *planSession) string { return s.bash("toolu_k1", "git push --force origin main") }},
	{name: "spec protect_path", kind: spec.ProtectPath, scope: store.RecordScopeV5,
		rule: kindProtectKeep, finding: kindProtectedRow, prompt: kindFixKeep, act: kindLandKeep},
	{name: "spec forbid_code", kind: spec.ForbidCode, scope: store.RecordScopeV5,
		rule:    `{"id": "k1", "kind": "forbid_code", "pattern": "panic", "paths": ["keep/*.go"], "why": "w", "hard_control": "h"}`,
		finding: `"kind":"forbid_code","verdict":"`, prompt: kindFixKeep, act: kindLandKeepStop},
	{name: "spec require_evidence", kind: spec.RequireEvidence, scope: store.RecordScopeV5,
		rule:    `{"id": "k1", "kind": "require_evidence", "at": ["done"], "evidence": ["kind:test"], "why": "w", "hard_control": "h"}`,
		finding: `"kind":"require_evidence","verdict":"violation"`, prompt: kindFixKeep, act: kindLandKeepStop},
	{name: "spec order", kind: spec.Order, scope: store.RecordScopeV5,
		rule:    `{"id": "k1", "kind": "order", "then": {"argv": ["git", "push"]}, "after": {"argv": ["go", "test"]}, "invalidated_by": ["**/*.go"], "why": "w", "hard_control": "h"}`,
		finding: `"type":"after_hit"`,
		act: func(s *planSession) string {
			return s.bash("toolu_k1", "go test ./...") + s.bash("toolu_k2", "git push origin main")
		}},
	{name: "spec boundary", kind: spec.Boundary, scope: store.RecordScopeV5,
		rule: kindBoundaryRule, finding: `"kind":"boundary_cross"`, prompt: kindFixGraph, act: kindCrossGraph},
}

// kindKnowledgeRows are the decisions knowledge.Rules compiles from a
// knowledge row, which engine.RulesFor admits where it admits no spec
// file's decision rule.
var kindKnowledgeRows = []kindRow{
	{name: "knowledge row paths_untouched at v5 on a landed edit", kind: spec.Decision, scope: store.RecordScopeV5,
		row:     kindDecisionRow(spec.PredicatePathsUntouched, "keep/*.go"),
		finding: `"kind":"decision_contradicted"`, prompt: kindFixKeep, act: kindLandKeep},
	{name: "knowledge row paths_untouched at v4 on a plan", kind: spec.Decision, scope: store.RecordScopeV4,
		row:     kindDecisionRow(spec.PredicatePathsUntouched, "keep/*.go"),
		finding: `"rule_ids":["k-d1-0"]`, act: kindPlanKeep},
}

// kindPlanKeep is an accepted plan that names the keep file, declared and
// then posted.
func kindPlanKeep(s *planSession) string {
	text := "# Plan\nChange " + cfKeep + " so K returns early."
	return s.plan(false, "toolu_p1", text) + s.plan(true, "toolu_p1", text)
}

// TestRuleKinds_EnforcedChangeAFinding is the converse the known limit
// spec.UnenforcedRuleKinds owes, closed both ways: a kind off the list has
// a row here, whose rule adds the row's finding to the read of its calls,
// not merely a change (a post records an empty conflict_check for any edit
// a rule scopes); a kind on the list
// has a spec rule's row in kindLimits at record scope v5, whose rule leaves
// it equal. The read without the rule is the same twice. Break: list an
// enforced kind, or drop a kind from the list with no row; or let a row's
// rule stop reaching what its calls give (a detector, a reader, the
// base's predicate), and its row's finding is missing.
func TestRuleKinds_EnforcedChangeAFinding(t *testing.T) {
	has, inert := map[spec.RuleKind]bool{}, map[spec.RuleKind]bool{}
	for _, r := range kindRows {
		if !kindSpecRow(r) {
			t.Errorf("row %q is not a spec file's rule: the list is read on spec rows alone, and a knowledge row goes in kindKnowledgeRows", r.name)
			continue
		}
		has[r.kind] = true
	}
	for _, r := range kindLimits {
		if kindSpecRow(r) && r.scope == store.RecordScopeV5 {
			inert[r.kind] = true
		}
	}
	for _, k := range spec.RuleKinds {
		switch listed := slices.Contains(spec.UnenforcedRuleKinds, k); {
		case listed && has[k]:
			t.Errorf("%s has a row showing it changes a finding, and spec.UnenforcedRuleKinds still lists it", k)
		case listed && !inert[k]:
			t.Errorf("%s is in spec.UnenforcedRuleKinds and has no v5 spec row in kindLimits showing it changes nothing", k)
		case !listed && !has[k]:
			t.Errorf("%s is not in spec.UnenforcedRuleKinds and has no row: show it changes a finding through a real hook call, or list it", k)
		}
	}
	kindChange(t, kindRows)
}

// TestRuleKinds_KnowledgeDecisionChangesAFinding: a paths_untouched
// decision compiled from an approved knowledge row changes the read of a
// landed Go edit at record scope v5 (the conflict check) and of an accepted
// plan at v4 (the plan check), where the same rule in a spec file changes
// nothing (kindLimits). These rows decide no membership of the list. Break:
// drop the compiled rules from engine.RulesFor, or the decision case from
// engine.Conflicts, and a row's finding is missing.
func TestRuleKinds_KnowledgeDecisionChangesAFinding(t *testing.T) {
	kindChange(t, kindKnowledgeRows)
}

// kindChange runs each row three times: without its rule twice, which must
// read equal, and with it, whose added lines must hold the row's finding.
// It logs what the rule added.
func kindChange(t *testing.T, rows []kindRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			if r.finding == "" {
				t.Fatal("the row names no finding: a read that only differs shows none")
			}
			base, _ := kindReadOf(t, r, "", nil)
			if again, _ := kindReadOf(t, r, "", nil); !again.equal(base) {
				t.Fatalf("the base read twice differs:\nout %q\n    %q\nrecords\n%s\n----\n%s", base.out, again.out, base.records, again.records)
			}
			with, _ := kindReadOf(t, r, r.rule, r.row)
			added := kindAdded(base, with)
			if !strings.Contains(added, r.finding) {
				t.Fatalf("rule %s%v added no %s:\n%s", r.rule, r.row, r.finding, added)
			}
			t.Logf("the rule adds:\n%s", added)
		})
	}
}

// kindLimits are the known limits, each pinned as a read the rule leaves
// equal, with a control rule the same calls answer to at the same scope
// and the control's finding. A row with a spec rule at v5 stands for its
// kind in the list. symbol_present is pinned twice: on an edit no boundary
// or path rule scopes, where the post checks nothing (ConflictBase.InScope),
// and on one kindInertBoundary scopes, where the check runs and the
// predicate raises nothing at the edit's overlay revision.
var kindLimits = []kindRow{
	{name: "spec boundary below v5", kind: spec.Boundary, scope: store.RecordScopeV4,
		rule: kindBoundaryRule, control: kindProtectGraph, controlFinding: kindFrozenPath, prompt: kindFixGraph, act: kindCrossGraph},
	{name: "spec paths_untouched decision at v5", kind: spec.Decision, scope: store.RecordScopeV5,
		rule: kindUntouchedRule, control: kindProtectKeep, controlFinding: kindProtectedRow, prompt: kindFixKeep, act: kindLandKeep},
	{name: "spec reuse with paths at v5", kind: spec.Reuse, scope: store.RecordScopeV5,
		rule:    `{"id": "k1", "kind": "reuse", "paths": ["keep/*.go"], "why": "w", "hard_control": "h"}`,
		control: kindProtectKeep, controlFinding: kindProtectedRow, prompt: kindFixKeep, act: kindLandKeep},
	{name: "knowledge row paths_untouched below v5 on a landed edit", kind: spec.Decision, scope: store.RecordScopeV4,
		row:     kindDecisionRow(spec.PredicatePathsUntouched, "keep/*.go"),
		control: kindProtectKeep, controlFinding: kindFrozenPath, prompt: kindFixKeep, act: kindLandKeep},
	{name: "knowledge row symbol_present at v5 on a landed edit", kind: spec.Decision, scope: store.RecordScopeV5,
		row:     kindSymbolPresentRow(),
		control: kindProtectGraph, controlFinding: kindProtectedRow, prompt: kindFixGraph,
		act: kindLandGraphFunc, witness: kindBaseHoldsSymbolPresent},
	{name: "knowledge row symbol_present at v5 on a landed edit a boundary rule scopes", kind: spec.Decision, scope: store.RecordScopeV5,
		row: kindSymbolPresentRow(), opens: kindInertBoundary,
		control: kindProtectGraph, controlFinding: kindProtectedRow, prompt: kindFixGraph,
		act: kindLandGraphFunc, witness: kindBaseHoldsSymbolPresent},
}

// TestRuleKinds_LimitsChangeNothing pins the limits the list's words name:
// with the rule and without, each call's output is byte-equal and the
// records are equal under the mask. The base read twice is equal, the
// control rule adds its finding, and a row that opens the conflict check
// shows the check ran in the base read, so equal is not equal by
// construction. Break: keep a conflict base below v5, admit a decision to
// the post's scope, or let symbol_present fire at an overlay revision, and
// the rule's read moves off the base.
func TestRuleKinds_LimitsChangeNothing(t *testing.T) {
	for _, r := range kindLimits {
		t.Run(r.name, func(t *testing.T) {
			base, _ := kindReadOf(t, r, "", nil)
			if again, _ := kindReadOf(t, r, "", nil); !again.equal(base) {
				t.Fatalf("the base read twice differs:\nout %q\n    %q\nrecords\n%s\n----\n%s", base.out, again.out, base.records, again.records)
			}
			if r.opens != "" && !strings.Contains(base.records, `"type":"conflict_check"`) {
				t.Fatalf("rule %s opened no conflict check, so the row's silence is the check's absence:\n%s", r.opens, base.records)
			}
			ctl, _ := kindReadOf(t, r, r.control, nil)
			added := kindAdded(base, ctl)
			if r.controlFinding == "" || !strings.Contains(added, r.controlFinding) {
				t.Fatalf("control rule %s added no %q: the read cannot see a finding here:\n%s", r.control, r.controlFinding, added)
			}
			t.Logf("control rule %s adds:\n%s", r.control, added)
			with, s := kindReadOf(t, r, r.rule, r.row)
			if with.out != base.out {
				t.Errorf("rule %s%v changed the output:\n got %q\nbase %q", r.rule, r.row, with.out, base.out)
			}
			if with.records != base.records {
				t.Errorf("rule %s%v changed the records:\n got\n%s\nbase\n%s", r.rule, r.row, with.records, base.records)
			}
			if r.witness != nil {
				r.witness(t, s)
			}
		})
	}
}
