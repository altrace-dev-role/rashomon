package recap

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

func conflictRow(kind, rule, path string) verify.ConflictRow {
	return verify.ConflictRow{Kind: kind, Unit: "g:r#unit:go:" + path, UnitPath: path, RuleID: rule, Subject: "sym:go:" + path}
}

// conflictView is a view the value line speaks for (one edit, no gate known)
// holding rows as its unresolved conflicts.
func conflictView(rows ...verify.ConflictRow) verify.StopView {
	v := viewOf(verify.RowNoGate, verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5})
	v.Conflicts = rows
	return v
}

const rl = "no-ui-db"

// TestStop_NamesUnresolvedConflictByRuleID (PLAN): EvaluateStop's view of a
// conflict the session was shown and did not resolve; the Stop's text names
// the rule id and the unit, and a rule-less kind names the unit alone.
func TestStop_NamesUnresolvedConflictByRuleID(t *testing.T) {
	v := verify.EvaluateStop(&store.Run{}, nil, nil, verify.Point{Kind: verify.PointStop, SessionID: "s1"}, time.Unix(1, 0), verify.StopInput{
		Conflicts: []verify.ConflictSeen{
			{Row: conflictRow("boundary_cross", "spec:"+rl, "internal/ui"), Seqs: []int64{3}},
			{Row: conflictRow("signature_mismatch", "", "internal/db"), Seqs: []int64{4}},
		}})
	v.Summary.Row, v.Turn = verify.RowNoGate, verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5}
	got := ValueLine(v, Impact{}, "sess-1", false)
	for _, want := range []string{
		indent + "spec rule " + rl + ":\n" + indent + "  unit 'internal/ui' adds a call, import or reference across a boundary a rule sets\n",
		indent + "unit 'internal/db': changes a signature that a caller no longer fits\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("line %q does not hold %q", got, want)
		}
	}
	if !strings.HasSuffix(got, indent+"→ rashomon report --session sess-1") {
		t.Errorf("line %q lost its pointer", got)
	}
}

// TestStop_ResolvedConflictIsMetWithEvidence (PLAN), the text half: a group
// all resolved has no view entry, so no line.
func TestStop_ResolvedConflictIsMetWithEvidence(t *testing.T) {
	v := verify.EvaluateStop(&store.Run{}, nil, nil, verify.Point{Kind: verify.PointStop, SessionID: "s1"}, time.Unix(1, 0), verify.StopInput{
		Conflicts: []verify.ConflictSeen{{Row: conflictRow("boundary_cross", rl, "internal/ui"), Seqs: []int64{3}, ResolvedSeq: 6}}})
	if len(v.Obligations) != 1 || v.Obligations[0].State != verify.ObligationMet || !slices.Equal(v.Obligations[0].Evidence, []int64{6}) {
		t.Fatalf("obligations %+v, want one met row citing [6]", v.Obligations)
	}
	v.Summary.Row, v.Turn = verify.RowNoGate, verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5}
	if got := ValueLine(v, Impact{}, "sess-1", false); strings.Contains(got, "conflict") || strings.Contains(got, "unit ") {
		t.Errorf("line %q speaks of a resolved conflict", got)
	}
	if got := RuleLine(v, "sess-1", false); got != "" {
		t.Errorf("RuleLine %q for a resolved conflict, want none", got)
	}
}

// TestStop_ConflictPhrasesAreTheRowKinds: the phrase table holds exactly the
// kinds that owe a row, in their order, each phrase non-empty.
func TestStop_ConflictPhrasesAreTheRowKinds(t *testing.T) {
	if got := ConflictPhraseKinds(); !slices.Equal(got, verify.ConflictRowKinds) {
		t.Errorf("phrase kinds %v, want %v", got, verify.ConflictRowKinds)
	}
	for _, k := range verify.ConflictRowKinds {
		if p, ok := conflictPhrase(k); !ok || p == "" {
			t.Errorf("kind %s has no phrase", k)
		}
	}
	for _, k := range verify.WithheldConflictKinds {
		if _, ok := conflictPhrase(k); ok {
			t.Errorf("withheld kind %s has a phrase", k)
		}
	}
}

// TestStop_ConflictLinesCapAndWidth: two lines then a counting line, in the
// plugin form without the command, a unit the path slot refuses reads "a
// unit", and a line past the columns puts its detail on its own line.
func TestStop_ConflictLinesCapAndWidth(t *testing.T) {
	rows := []verify.ConflictRow{conflictRow("protected_path", "", "a"), conflictRow("protected_path", "", "b"), conflictRow("protected_path", "", "c")}
	got := conflictLines(conflictView(rows...), false, "")
	want := indent + "unit 'a': edits a file a rule protects\n" + indent + "unit 'b': edits a file a rule protects\n" +
		indent + "and 1 more unresolved conflict: rashomon report --ledger\n"
	if got != want {
		t.Errorf("lines %q, want %q", got, want)
	}
	if got := conflictLines(conflictView(append(rows, rows[0])...), true, ""); !strings.HasSuffix(got, indent+"and 2 more unresolved conflicts\n") {
		t.Errorf("plugin lines %q, want the counting line without the command", got)
	}
	if got := conflictLines(conflictView(conflictRow("protected_path", "", "")), false, ""); got != indent+"a unit: edits a file a rule protects\n" {
		t.Errorf("unplaced unit %q, want \"a unit\"", got)
	}
	long := conflictLines(conflictView(conflictRow("boundary_cross", strings.Repeat("r", 48), "internal/some/package")), false, "")
	if !strings.Contains(long, ":\n"+indent+"  unit 'internal/some/package' adds") {
		t.Errorf("long line %q, want its detail on a line of its own", long)
	}
	if got := conflictLines(conflictView(conflictRow("boundary_cross", "Bad Rule", "u")), false, ""); !strings.HasPrefix(got, indent+"a spec rule:\n"+indent+"  unit 'u' ") {
		t.Errorf("refused id %q, want \"a spec rule\"", got)
	}
}

// maxView is the maximum of every capped kind: ids and paths at their
// longest legal lengths.
func maxView() verify.StopView {
	long := strings.Repeat("r", 48)
	path := strings.Repeat("d/", 100) + "f"
	v := viewOf(verify.RowNoGate, verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5})
	for i := range 3 {
		id := string(rune('a'+i)) + long[1:]
		v.Violations = append(v.Violations, matched(id, spec.ProtectPath, 1, 2, 3, 4))
		v.Obligations = append(v.Obligations, verify.Obligation{Kind: verify.PlatformImpact, State: verify.ObligationOpen})
		v.Platform.Families = append(v.Platform.Families, verify.PlatformFamily{Package: "p", Dir: strings.Repeat("e/", 60) + string(rune('a'+i)), ModuleDir: "."})
		v.Conflicts = append(v.Conflicts, conflictRow("boundary_cross", id, path+string(rune('a'+i))))
	}
	v.Unbound = []string{path, path + "2"}
	return v
}

var maxImpact = Impact{Findings: []FindingRef{{State: FindingResolved, ResolvedBy: 41}}}

// TestStop_MaxOfEveryLineKindKeepsTail (B2): the maximum of every capped
// kind, each composer: the clause and the pointer end the message and the
// total is within stopMessageChars.
func TestStop_MaxOfEveryLineKindKeepsTail(t *testing.T) {
	v := maxView()
	got := ValueLine(v, maxImpact, "sess-1", false)
	for _, want := range []string{"and 1 more unresolved conflict: rashomon report --ledger\n", "and 1 more spec rule matched\n", "and 1 more: rashomon report --ledger\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("max line lacks %q:\n%s", want, got)
		}
	}
	if c := clause(maxImpact); c == "" || !strings.HasSuffix(got, indent+c+"\n"+indent+"→ rashomon report --session sess-1") {
		t.Errorf("max line does not end with its clause and pointer:\n%s", got)
	}
	if n := columns(PreviousTurn(got)); n > stopMessageChars {
		t.Errorf("max line is %d characters, over %d", n, stopMessageChars)
	}
}

// TestStop_OverBudgetConflictCountLineYieldsFirst (B2): with the other lines
// leaving room for less than the lines and their counting line, the
// counting line goes first, then the lines give way to a line that counts
// them, then nothing; the tail is intact at each step.
func TestStop_OverBudgetConflictCountLineYieldsFirst(t *testing.T) {
	rows := []verify.ConflictRow{conflictRow("protected_path", "", "a"), conflictRow("protected_path", "", "b"), conflictRow("protected_path", "", "c")}
	v := conflictView(rows...)
	full := conflictLines(v, false, "")
	lines, _, _ := strings.Cut(full, indent+"and ")
	alone := indent + "3 unresolved conflicts: rashomon report --ledger\n"
	// rest sized so that exactly `left` characters remain for the conflict lines.
	restFor := func(left int) string { return strings.Repeat("x", stopMessageChars-left) }
	for name, c := range map[string]struct {
		left int
		want string
	}{
		"all fit":        {wideLines(full), full},
		"count goes":     {wideLines(full) - 1, lines},
		"lines give way": {wideLines(lines) - 1, alone},
		"nothing":        {wideLines(alone) - 1, ""},
	} {
		if got := conflictLines(v, false, restFor(c.left)); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	// Through a composer: unboundLines has no count cap, so enough of them
	// fill the message to any point; sweep the fill a character at a time and
	// see every stage, the tail intact and the cap kept at each.
	stages := map[string]bool{}
	build := func(n, extra int) verify.StopView {
		w := conflictView(rows...)
		for i := range n {
			p := strings.Repeat("d/", 40) + strings.Repeat("f", i%7)
			if i == 0 {
				p += strings.Repeat("g", extra)
			}
			w.Unbound = append(w.Unbound, p)
		}
		return w
	}
	size := func(w verify.StopView) int {
		w.Conflicts = nil
		return wide(ValueLine(w, Impact{}, "sess-1", false))
	}
	n := 1
	for size(build(n, 0)) < stopMessageChars-400 {
		n++
	}
	for _, k := range []int{n, n + 1, n + 2, n + 3, n + 4} {
		for extra := 0; extra < 200; extra++ {
			w := build(k, extra)
			got := ValueLine(w, Impact{}, "sess-1", false)
			if !strings.HasSuffix(got, indent+"→ rashomon report --session sess-1") {
				t.Fatalf("n=%d extra=%d: the pointer is cut", k, extra)
			}
			if base := size(w); wide(got) > max(stopMessageChars, base) {
				t.Fatalf("n=%d extra=%d: %d characters, over the cap and the message without conflicts (%d)", k, extra, wide(got), base)
			}
			switch {
			case strings.Contains(got, "and 1 more unresolved conflict"):
				stages["all"] = true
			case strings.Contains(got, "unit 'a':"):
				stages["lines"] = true
			case strings.Contains(got, "3 unresolved conflicts"):
				stages["alone"] = true
			default:
				stages["nothing"] = true
			}
		}
	}
	if len(stages) != 4 {
		t.Errorf("stages seen %v, want all four", stages)
	}
}

// TestStop_ConflictLineSpeaksWithSummaryOff (B5): RuleLine speaks for an
// unresolved conflict with no rule verdict, its head counting what it names;
// a verdict-only view prints RuleLine's bytes as before.
func TestStop_ConflictLineSpeaksWithSummaryOff(t *testing.T) {
	v := conflictView(conflictRow("boundary_cross", rl, "internal/ui"))
	want := prefix + "a conflict is unresolved.\n" +
		indent + "spec rule " + rl + ":\n" + indent + "  unit 'internal/ui' adds a call, import or reference across a boundary a rule sets\n" +
		indent + "→ rashomon report --session sess-1"
	if got := RuleLine(v, "sess-1", false); got != want {
		t.Errorf("RuleLine %q, want %q", got, want)
	}
	v.Conflicts = append(v.Conflicts, conflictRow("protected_path", "", "x"), conflictRow("protected_path", "", "y"))
	if got := RuleLine(v, "sess-1", false); !strings.HasPrefix(got, prefix+"3 conflicts are unresolved.\n") {
		t.Errorf("RuleLine %q, want the head to count three", got)
	}
	verdict := conflictView()
	verdict.Violations = []verify.RuleVerdict{matched("no-rm-glob", spec.ForbidCommand, 3), matched("frozen-api", spec.ProtectPath, 4)}
	wantV := prefix + "2 spec rules matched in this turn.\n" + indent + "spec rule no-rm-glob matched call 3\n" +
		indent + "spec rule frozen-api protects a path call 4 wrote\n" + indent + "→ rashomon report --session sess-1"
	if got := RuleLine(verdict, "sess-1", false); got != wantV {
		t.Errorf("verdict-only RuleLine %q, want %q", got, wantV)
	}
	verdict.Violations = verdict.Violations[:1]
	if got := RuleLine(verdict, "sess-1", false); !strings.HasPrefix(got, prefix+"a spec rule matched in this turn.\n") {
		t.Errorf("one-verdict RuleLine %q changed its head", got)
	}
	if got := RuleLine(conflictView(), "sess-1", false); got != "" {
		t.Errorf("RuleLine %q for nothing to name", got)
	}
}

// wide is a text's length in characters as the catch-up prints it, through
// PreviousTurn, which only lengthens it.
func wide(s string) int { return columns(PreviousTurn(s)) }

// wideLines is wide for lines that sit after a newline in a message.
func wideLines(s string) int { return columns(PreviousTurn("\n"+s)) - 1 }
