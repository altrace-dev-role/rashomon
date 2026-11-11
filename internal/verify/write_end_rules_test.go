package verify

import (
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestWEND6_ARuleReadsNoWriteThatDidNotApply: a protect_path rule's verdict
// names no failed file tool as a write (the Stop's line would say the call
// wrote the path), and cannot be decided while a call that may have written
// the path stands: a shell line that failed, a call interrupted, one with no
// ending. An order rule's invalidated_by reads the same way. The twin, the
// Edit that applied, is the violation and the invalidation. Break: drop held
// writes from the rules, or read a failed file tool as one that may have
// written.
func TestWEND6_ARuleReadsNoWriteThatDidNotApply(t *testing.T) {
	interrupted := edit(1, "db/migrations/001.sql")
	interrupted.outcome = store.ExecInterrupted
	failedSed := bashWrite(1, "db/migrations/003.sql")
	failedSed.outcome, failedSed.exit = store.ExecFailed, 2
	open := call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"db/migrations/001.sql"}, term: store.OutcomeOK}
	for _, c := range []struct {
		name string
		run  *store.Run
		want RuleVerdictValue
	}{
		{"an Edit that applied", runOf(t, edit(1, "db/migrations/001.sql")), Violation},
		{"a failed Edit", runOf(t, failedEdit(1, "db/migrations/001.sql")), NoRecordedViolation},
		{"an interrupted Edit", runOf(t, interrupted), RuleUnknown},
		{"a failed sed naming the path", runOf(t, failedSed), RuleUnknown},
		{"an Edit with no ending", unended(runOf(t, open), 0), RuleUnknown},
		{"a failed Edit elsewhere", runOf(t, failedEdit(1, "db/schema.sql")), NoRecordedViolation},
	} {
		snap := Evaluate(c.run, nil, ruleSpec(spec.Trusted, frozen(allModes...)), stopPoint(c.run), time.Time{})
		if v := oneVerdict(t, snap); v.Verdict != c.want {
			t.Errorf("protect_path, %s: verdict %+v, want %s", c.name, v, c.want)
		}
	}

	interruptedGo := edit(2, "pkg/a.go")
	interruptedGo.outcome = store.ExecInterrupted
	sp := ruleSpec(spec.Trusted, testFirst())
	for _, c := range []struct {
		name    string
		between call
		want    RuleVerdictValue
	}{
		{"an Edit that applied invalidates the run", edit(2, "pkg/a.go"), Violation},
		{"a failed Edit does not", failedEdit(2, "pkg/a.go"), NoRecordedViolation},
		{"an interrupted Edit may have", interruptedGo, RuleUnknown},
	} {
		r := ordered(t, shc(1, "go test ./..."), c.between, shc(3, "git push origin main"))
		snap, _ := specLedgerAt(r, sp, stopPoint(r))
		if v := oneVerdict(t, snap); v.Verdict != c.want {
			t.Errorf("order, %s: verdict %+v, want %s", c.name, v, c.want)
		}
	}
}

// TestWEND7_StoreTouchedStaysOutcomeBlind: a call that targets a path under
// .claude/rashomon/ is a store_touched entry whatever its ending (D135): a
// failed Write and one with no ending as much as one that applied, since a
// failed shaped write may still have landed and the entry says the call
// targeted the path, never that it wrote it. Break: read store_touched over
// the edits alone.
func TestWEND7_StoreTouchedStaysOutcomeBlind(t *testing.T) {
	failed := writeCall(1, ".claude/rashomon/spec.json")
	failed.outcome = store.ExecFailed
	open := call{seq: 1, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{".claude/rashomon/spec.json"}, term: store.OutcomeOK}
	for name, r := range map[string]*store.Run{
		"applied":   runOf(t, writeCall(1, ".claude/rashomon/spec.json")),
		"failed":    runOf(t, failed),
		"no ending": unended(runOf(t, open), 0),
	} {
		got := integrityOf(r, 0, lastSeqOf(r)).Unresolved
		if len(got) != 1 || got[0] != (IntegrityEntry{Check: StoreTouched, Seq: declSeq(r, 0), Path: ".claude/rashomon/spec.json"}) {
			t.Errorf("%s: entries %+v, want one store_touched at the call", name, got)
		}
	}
}

// TestWEND8_TheRegressionNoteNamesOnlyEdits: between a green run and a red
// one, the note names the files the edits that applied wrote and counts the
// unnamed writes of calls that ended ok; a failed Edit and a failed sed with
// no target are not "files written between the two runs". Before the rule
// the note named the failed Edit's file and counted the sed. Break: count a
// held write between the runs, in the paths or in "and k more".
func TestWEND8_TheRegressionNoteNamesOnlyEdits(t *testing.T) {
	sed := call{seq: 4, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecFailed, exit: 4, mayWrite: true}
	r := runOf(t,
		gateCall(1, "go", store.ExecOK),
		failedEdit(2, "internal/a.go"),
		edit(3, "internal/b.go"),
		sed,
		gateCall(5, "go", store.ExecFailed),
	)
	minuteClock(t, r)
	for i := range r.Declarations {
		r.Declarations[i].Project = strp("/repo")
	}
	view := viewAt(t, r, 5)
	if len(view.Findings) != 1 {
		t.Fatalf("SteerAt = %+v, want one regression finding", view.Findings)
	}
	const want = "rashomon record: gate 'go' exited 0 earlier in this session (4 min ago) and exited 1 on this run. " +
		"Files written between the two runs: 'internal/b.go'."
	if got := view.Findings[0].Message.Text(); got != want {
		t.Errorf("note %q, want %q", got, want)
	}
}
