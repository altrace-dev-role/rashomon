package verify

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// testFirst forbids git push unless go test ran green before it and no go
// file was written since.
func testFirst() spec.Rule {
	return spec.Rule{ID: "test-first", Kind: string(spec.Order), Why: "push only tested code", HardControl: "CI",
		Then:          &spec.CommandMatch{Argv: []string{"git", "push"}},
		After:         &spec.CommandMatch{Argv: []string{"go", "test"}},
		InvalidatedBy: []string{"**/*.go"}}
}

// noTodo forbids adding a TODO under src.
func noTodo() spec.Rule {
	return spec.Rule{ID: "no-todo", Kind: string(spec.ForbidCode), Why: "w", HardControl: "lint",
		Pattern: `TODO`, Paths: []string{"src/**"}}
}

// ordered is a run of cs with each Bash call's line recorded (call.line) and,
// as the post records them, an after_hit for each call the testFirst rule's
// after may run (hitsFor).
func ordered(t *testing.T, cs ...call) *store.Run {
	t.Helper()
	m := map[int64]string{}
	for _, c := range cs {
		if c.tool == "Bash" && c.line != "" {
			m[c.seq] = c.line
		}
	}
	r := withLines(runOf(t, cs...), m)
	hitsFor(t, r, cs)
	return r
}

// testFirstDigest is the digest ruleSpec gives the source of its rules.
const testFirstDigest = spec.DigestPrefix + "1111111111111111111111111111111111111111111111111111111111111111"

// hitsFor appends to r the after_hit the post would record for each call of
// cs whose line the testFirst rule's after matches or may match: its exit
// status (nil for a failure with none), foreground flag and directory digest
// as the call's records say, maybe where the line only may.
func hitsFor(t *testing.T, r *store.Run, cs []call) {
	t.Helper()
	rule := testFirst()
	for _, c := range cs {
		if c.tool != "Bash" || c.line == "" {
			continue
		}
		cmds, ok := shape.Commands(c.line)
		tr := lineMatch(rule.After, &ruleLine{cmds: cmds, whole: ok})
		if tr == triNo {
			continue
		}
		var exit *int
		if c.outcome == store.ExecOK {
			zero := 0
			exit = &zero
		} else if c.exit != 0 {
			exit = &c.exit
		}
		r.AfterHits = append(r.AfterHits, *store.NewAfterHit("s1", postEvent, "spec:test-first", testFirstDigest,
			landedAt(t, r, c.seq), exit, !c.bg, c.loc, tr == triMaybe, 0))
	}
}

// sedc is a Bash write of paths, its line recorded.
func sedc(seq int64, paths ...string) call {
	c := bashWrite(seq, paths...)
	c.line = "sed -i x " + strings.Join(paths, " ")
	return c
}

// shc is a Bash call of line that exits 0.
func shc(seq int64, line string) call {
	c := sh(seq, line)
	c.line, c.loc = line, "d:repo"
	return c
}

// landedAt is the seq the call's execution landed at.
func landedAt(t *testing.T, r *store.Run, seq int64) int64 {
	t.Helper()
	for i := range r.Executions {
		if r.Executions[i].ToolUseID == callID(seq) && r.Executions[i].Seq != nil {
			return *r.Executions[i].Seq
		}
	}
	t.Fatalf("no execution for call %d", seq)
	return 0
}

func orderCall(t *testing.T, r *store.Run, seq int64, line string) RuleCall {
	return RuleCall{Seq: landedAt(t, r, seq), ToolName: "Bash", Command: line, ToolUseID: callID(seq), Run: r}
}

// TestSR1o_OrderSatisfiedByAnEarlierGreenAfter: a Then run with a foreground
// exit-0 After run before it, in the same root and with no invalidating
// write between, is no violation; the row is met and the post raises nothing.
// A write the rule does not name between them does not invalidate.
func TestSR1o_OrderSatisfiedByAnEarlierGreenAfter(t *testing.T) {
	sp := ruleSpec(spec.Trusted, testFirst())
	for name, r := range map[string]*store.Run{
		"plain":            ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main")),
		"unrelated write":  ordered(t, shc(1, "go test ./..."), edit(2, "docs/readme.md"), shc(3, "git push origin main")),
		"write then after": ordered(t, edit(1, "pkg/a.go"), shc(2, "go test ./..."), shc(3, "git push origin main")),
	} {
		snap, obs := specLedgerAt(r, sp, stopPoint(r))
		if v := oneVerdict(t, snap); v.Verdict != NoRecordedViolation || v.Kind != spec.Order || len(v.Seqs) != 0 {
			t.Errorf("%s: verdict %+v, want no_recorded_violation", name, v)
		}
		if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationMet {
			t.Errorf("%s: rows %+v, want one met", name, rows)
		}
		last := int64(len(r.Declarations))
		if v := RulesAtCall(sp, orderCall(t, r, last, "git push origin main"), atPost(landedAt(t, r, last)), postEvent); len(v.Findings) != 0 {
			t.Errorf("%s: raised %+v", name, v.Findings)
		}
	}
	// No Then run at all: the rule applies and is met.
	r := ordered(t, shc(1, "go build ./..."))
	if v := oneVerdict(t, Evaluate(r, nil, sp, stopPoint(r), time.Time{})); v.Verdict != NoRecordedViolation {
		t.Errorf("no then run: %+v", v)
	}
}

// TestSR1p_OrderViolatedRaisesAtTheThenCallAndRowsUnmet: a Then run with no
// qualifying earlier After run is a violation citing the Then call's
// declaration; the row is unmet; the post raises ordering-rule at nudge at
// that call, naming the program and no rule text, and not at the calls that
// are not the Then.
func TestSR1p_OrderViolatedRaisesAtTheThenCallAndRowsUnmet(t *testing.T) {
	sp := ruleSpec(spec.Trusted, testFirst())
	r := ordered(t, shc(1, "go build ./..."), shc(2, "git push origin main"), shc(3, "go vet ./..."))
	snap, obs := specLedgerAt(r, sp, stopPoint(r))
	bad := declOf(r, 2).Seq
	if v := oneVerdict(t, snap); v.Verdict != Violation || !slices.Equal(v.Seqs, []int64{bad}) || v.HardControl != "CI" {
		t.Fatalf("verdict %+v, want violation citing %d", v, bad)
	}
	if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnmet || !slices.Equal(rows[0].Evidence, []int64{bad}) ||
		rows[0].ID != ObligationID(ScopeSession, SpecRule, "spec:test-first") || rows[0].Source != "spec" {
		t.Fatalf("rows %+v, want one unmet citing %d", rows, bad)
	}
	if snap.RecordGreen {
		t.Fatal("a trusted order violation left record-green")
	}
	pt := atPost(landedAt(t, r, 2))
	view := RulesAtCall(sp, orderCall(t, r, 2, "git push origin main"), pt, postEvent)
	if len(view.Findings) != 1 || view.Dropped != 0 {
		t.Fatalf("findings %+v dropped %d, want one", view.Findings, view.Dropped)
	}
	f := view.Findings[0]
	if f.Pattern != pattern.OrderingRule || f.MaxLevel != steer.LevelNudge || !slices.Equal(f.Evidence, []int64{pt.Seq}) {
		t.Fatalf("finding %+v, want ordering-rule at nudge citing %d", f, pt.Seq)
	}
	if got := f.Message.Text(); got != "rashomon record: this 'git' call ran before the earlier step spec rule test-first requires." {
		t.Fatalf("note %q", got)
	}
	// Neither the call before the then nor the one after it, which sees the
	// violation in the record, is the call that violated.
	for seq, line := range map[int64]string{1: "go build ./...", 3: "go vet ./..."} {
		if v := RulesAtCall(sp, orderCall(t, r, seq, line), atPost(landedAt(t, r, seq)), postEvent); len(v.Findings) != 0 {
			t.Fatalf("call %d, not the Then, raised %+v", seq, v.Findings)
		}
	}
	// No run read at the post: nothing is raised, the record judges at Stop.
	c := orderCall(t, r, 2, "git push origin main")
	c.Run = nil
	if v := RulesAtCall(sp, c, pt, postEvent); len(v.Findings) != 0 {
		t.Fatalf("with no run: %+v", v.Findings)
	}
}

// TestSR1q_OrderAfterMustQualify: an After run qualifies only with exit 0,
// in the foreground, in the Then's root, and with no write the rule names
// between them.
func TestSR1q_OrderAfterMustQualify(t *testing.T) {
	sp := ruleSpec(spec.Trusted, testFirst())
	failed := shc(1, "go test ./...")
	failed.outcome, failed.exit = store.ExecFailed, 1
	bg := shc(1, "go test ./...")
	bg.bg = true
	elsewhere := shc(1, "go test ./...")
	elsewhere.loc = "d:other"
	here := shc(2, "git push origin main")
	for name, r := range map[string]*store.Run{
		"nonzero exit":     ordered(t, failed, shc(2, "git push origin main")),
		"background":       ordered(t, bg, shc(2, "git push origin main")),
		"another root":     ordered(t, elsewhere, here),
		"invalidating":     ordered(t, shc(1, "go test ./..."), edit(2, "pkg/a.go"), shc(3, "git push origin main")),
		"bash write":       ordered(t, shc(1, "go test ./..."), sedc(2, "cmd/x/main.go"), shc(3, "git push origin main")),
		"after is later":   ordered(t, shc(1, "git push origin main"), shc(2, "go test ./...")),
		"after not landed": ordered(t, shc(1, "go vet ./..."), shc(2, "git push origin main")),
	} {
		v := oneVerdict(t, Evaluate(r, nil, sp, stopPoint(r), time.Time{}))
		if v.Verdict != Violation || len(v.Seqs) != 1 {
			t.Errorf("%s: verdict %+v, want one violation", name, v)
		}
	}
}

// TestSR1r_OrderIndirectionIsUndecidedAndNeverFires: a Then or After the
// line does not show, both in one line, a glob or unnamed write between, a
// line the record withheld, a rule missing a field and one naming a
// same_run_key: unknown verdict, unknown row, no finding. A violation of
// another Then still beats an undecided one.
func TestSR1r_OrderIndirectionIsUndecidedAndNeverFires(t *testing.T) {
	sp := ruleSpec(spec.Trusted, testFirst())
	glob := ordered(t, shc(1, "go test ./..."), sedc(2, "pkg/*.go"), shc(3, "git push origin main"))
	blind := ordered(t, shc(1, "go test ./..."), unnamedWrite(2), shc(3, "git push origin main"))
	withheld := ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main"))
	withheld.Declarations[0].Command = nil
	cases := map[string]*store.Run{
		"then in bash -c":    ordered(t, shc(1, "go build ./..."), shc(2, "bash -c 'git push'")),
		"then expands":       ordered(t, shc(1, "go build ./..."), shc(2, "git $VERB origin")),
		"then in a heredoc":  ordered(t, shc(1, "go build ./..."), shc(2, "bash <<EOF\ngit push\nEOF")),
		"after in bash -c":   ordered(t, shc(1, "bash -c 'go test ./...'"), shc(2, "git push origin main")),
		"after expands":      ordered(t, shc(1, "go $SUB ./..."), shc(2, "git push origin main")),
		"both in one line":   ordered(t, shc(1, "go test ./... && git push origin main")),
		"glob write between": glob,
		"unnamed write":      blind,
		"line withheld":      withheld,
	}
	for name, r := range cases {
		snap, obs := specLedgerAt(r, sp, stopPoint(r))
		if v := oneVerdict(t, snap); v.Verdict != RuleUnknown || len(v.Seqs) != 0 {
			t.Errorf("%s: verdict %+v, want unknown", name, v)
		}
		if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnknown {
			t.Errorf("%s: rows %+v, want one unknown", name, rows)
		}
		for _, c := range r.Declarations {
			if c.Command == nil {
				continue
			}
			seq := landedBy(r, c.ToolUseID)
			if v := RulesAtCall(sp, RuleCall{Seq: seq, ToolName: "Bash", Command: *c.Command, ToolUseID: c.ToolUseID, Run: r}, atPost(seq), postEvent); len(v.Findings) != 0 {
				t.Errorf("%s: raised %+v", name, v.Findings)
			}
		}
	}
	for name, mut := range map[string]func(*spec.Rule){
		"no after":   func(r *spec.Rule) { r.After = nil },
		"no then":    func(r *spec.Rule) { r.Then = nil },
		"after_gate": func(r *spec.Rule) { r.After, r.AfterGate = nil, "spec:go-test" },
		"run key":    func(r *spec.Rule) { r.SameRunKey = []string{"branch"} },
		"empty argv": func(r *spec.Rule) { r.Then = &spec.CommandMatch{} },
	} {
		rule := testFirst()
		mut(&rule)
		r := ordered(t, shc(1, "git push origin main"))
		if v := oneVerdict(t, Evaluate(r, nil, ruleSpec(spec.Trusted, rule), stopPoint(r), time.Time{})); v.Verdict != RuleUnknown {
			t.Errorf("%s: verdict %+v, want unknown", name, v)
		}
	}
	both := ordered(t, shc(1, "bash -c 'git push'"), shc(2, "git push origin main"))
	if v := oneVerdict(t, Evaluate(both, nil, sp, stopPoint(both), time.Time{})); v.Verdict != Violation || !slices.Equal(v.Seqs, []int64{declOf(both, 2).Seq}) {
		t.Errorf("a violation beside an undecided call: %+v", v)
	}
}

func landedBy(r *store.Run, id string) int64 {
	for i := range r.Executions {
		if r.Executions[i].ToolUseID == id && r.Executions[i].Seq != nil {
			return *r.Executions[i].Seq
		}
	}
	return 0
}

// TestSR1s_UntrustedOrderFlagsTheUserOnly: an untrusted spec's order
// violation is a user flag at the Then call, owes no ledger row and does not
// clear record-green.
func TestSR1s_UntrustedOrderFlagsTheUserOnly(t *testing.T) {
	sp := ruleSpec(spec.Untrusted, testFirst())
	r := ordered(t, edit(1, "docs/a.md"), gateCall(2, "go-test", store.ExecOK), shc(3, "git push origin main"))
	snap, obs := specLedgerAt(r, sp, stopPoint(r))
	if v := oneVerdict(t, snap); v.Verdict != Violation {
		t.Fatalf("verdict %+v, want violation", v)
	}
	if rows := specRows(obs); len(rows) != 0 {
		t.Fatalf("untrusted rows %+v, want none", rows)
	}
	if !snap.RecordGreen {
		t.Fatal("an untrusted violation cleared record-green")
	}
	view := RulesAtCall(sp, orderCall(t, r, 3, "git push origin main"), atPost(landedAt(t, r, 3)), postEvent)
	if len(view.Findings) != 1 || view.Findings[0].MaxLevel != steer.LevelFlag || view.Findings[0].Pattern != pattern.OrderingRule {
		t.Fatalf("findings %+v, want one ordering-rule flag", view.Findings)
	}
}

// TestSR1t_ForbidCodeIsUndecidedUntilContentIsRead: a write to a path the
// rule's paths match is undecided with reason "content not read" for every
// applicable write, whatever it wrote: never a violation, an unknown row,
// no finding, record-green untouched. With no applicable write the rule is
// met. A rule without a pattern or paths is undecided too.
func TestSR1t_ForbidCodeIsUndecidedUntilContentIsRead(t *testing.T) {
	sp := ruleSpec(spec.Trusted, noTodo())
	marked := edit(1, "src/a.go").marks(map[string]int{"TODO": 1}, nil)
	for name, r := range map[string]*store.Run{
		"edit":            runOf(t, edit(1, "src/a.go")),
		"edit with marks": runOf(t, marked),
		"bash write":      runOf(t, bashWrite(1, "src/b.go")),
	} {
		snap, obs := specLedgerAt(r, sp, stopPoint(r))
		v := oneVerdict(t, snap)
		if v.Verdict != RuleUnknown || v.Kind != spec.ForbidCode || len(v.Seqs) != 0 || v.reason != ReasonContentNotRead {
			t.Errorf("%s: verdict %+v, want unknown, %q", name, v, ReasonContentNotRead)
		}
		if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnknown {
			t.Errorf("%s: rows %+v, want one unknown", name, rows)
		}
		if view := RulesAtCall(sp, RuleCall{Seq: 3, ToolName: "Edit", FilePath: "src/a.go"}, atPost(3), postEvent); len(view.Findings) != 0 {
			t.Errorf("%s: raised %+v", name, view.Findings)
		}
	}
	clean := runOf(t, edit(1, "docs/a.md"))
	snap, obs := specLedgerAt(clean, sp, stopPoint(clean))
	if v := oneVerdict(t, snap); v.Verdict != NoRecordedViolation || v.reason != "" {
		t.Errorf("no applicable write: %+v, want met", v)
	}
	if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationMet {
		t.Errorf("no applicable write rows %+v", rows)
	}
	for name, mut := range map[string]func(*spec.Rule){
		"no pattern": func(r *spec.Rule) { r.Pattern = "" },
		"no paths":   func(r *spec.Rule) { r.Paths = nil },
	} {
		rule := noTodo()
		mut(&rule)
		r := runOf(t, edit(1, "docs/a.md"))
		if v := oneVerdict(t, Evaluate(r, nil, ruleSpec(spec.Trusted, rule), stopPoint(r), time.Time{})); v.Verdict != RuleUnknown {
			t.Errorf("%s: %+v, want unknown", name, v)
		}
	}
}

// TestSR1u_AnAfterCallLeavesAHitFromTheCallAlone: RulesAtCall returns an
// after_hit for a call whose line the rule's after matches, from the payload
// in memory: the rule's id and its file's digest, the call's exit status and
// foreground flag as the post read them, and its declaration's directory
// digest; Maybe where the line only may run the after; nothing for a line
// that does not, for a rule with no after, or for a call that is not Bash.
// An untrusted rule's hit is the same (the user flag judges from it), and a
// call with no run read carries no directory, which no Then compares equal
// to.
func TestSR1u_AnAfterCallLeavesAHitFromTheCallAlone(t *testing.T) {
	zero, one := 0, 1
	first := sh(1, "go test ./...")
	first.loc = "d:repo"
	r := runOf(t, first)
	call1 := func(line string, exit *int, fg bool, run *store.Run) RuleCall {
		return RuleCall{Seq: 2, ToolName: "Bash", Command: line, ToolUseID: callID(1), Run: run, Exit: exit, Foreground: fg}
	}
	for _, tr := range []spec.Trust{spec.Trusted, spec.Untrusted} {
		sp := ruleSpec(tr, testFirst())
		v := RulesAtCall(sp, call1("go test ./...", &zero, true, r), atPost(2), postEvent)
		want := AfterHit{RuleID: "spec:test-first", SourceDigest: testFirstDigest, CWDDigest: "d:repo", Exit: &zero, Foreground: true}
		if len(v.AfterHits) != 1 || !afterHitEqual(v.AfterHits[0], want) {
			t.Errorf("%v: after hits %+v, want %+v", tr, v.AfterHits, want)
		}
	}
	sp := ruleSpec(spec.Trusted, testFirst())
	if v := RulesAtCall(sp, call1("go test ./...", &one, true, r), atPost(2), postEvent); len(v.AfterHits) != 1 || v.AfterHits[0].Exit == nil || *v.AfterHits[0].Exit != 1 {
		t.Errorf("a failed after: %+v, want a hit with exit 1", v.AfterHits)
	}
	if v := RulesAtCall(sp, call1("go test ./...", nil, true, r), atPost(2), postEvent); len(v.AfterHits) != 1 || v.AfterHits[0].Exit != nil {
		t.Errorf("an after with no stated status: %+v, want a hit with no exit", v.AfterHits)
	}
	if v := RulesAtCall(sp, call1("go test ./...", &zero, false, r), atPost(2), postEvent); len(v.AfterHits) != 1 || v.AfterHits[0].Foreground {
		t.Errorf("a background after: %+v, want a hit that is not foreground", v.AfterHits)
	}
	if v := RulesAtCall(sp, call1("bash -c 'go test ./...'", &zero, true, r), atPost(2), postEvent); len(v.AfterHits) != 1 || !v.AfterHits[0].Maybe {
		t.Errorf("an after in bash -c: %+v, want a maybe hit", v.AfterHits)
	}
	if v := RulesAtCall(sp, call1("go test ./...", &zero, true, nil), atPost(2), postEvent); len(v.AfterHits) != 1 || v.AfterHits[0].CWDDigest != "" {
		t.Errorf("with no run: %+v, want a hit with no directory", v.AfterHits)
	}
	for name, c := range map[string]RuleCall{
		"another command": call1("go build ./...", &zero, true, r),
		"no line":         call1("", &zero, true, r),
		"not Bash":        {Seq: 2, ToolName: "Edit", Command: "go test ./...", Run: r},
	} {
		if v := RulesAtCall(sp, c, atPost(2), postEvent); len(v.AfterHits) != 0 {
			t.Errorf("%s: hit %+v, want none", name, v.AfterHits)
		}
	}
	noAfter := testFirst()
	noAfter.After = nil
	if v := RulesAtCall(ruleSpec(spec.Trusted, noAfter), call1("go test ./...", &zero, true, r), atPost(2), postEvent); len(v.AfterHits) != 0 {
		t.Errorf("a rule with no after: hit %+v, want none", v.AfterHits)
	}
}

func afterHitEqual(a, b AfterHit) bool {
	return a.RuleID == b.RuleID && a.SourceDigest == b.SourceDigest && a.CWDDigest == b.CWDDigest && a.Foreground == b.Foreground &&
		a.Maybe == b.Maybe && (a.Exit == nil) == (b.Exit == nil) && (a.Exit == nil || *a.Exit == *b.Exit)
}

// TestSR1v_TheJudgeReadsHitsNotAfterLines: the After's record is its hit. With
// every line withheld (record scope v4) but the hit, the Then call is met;
// with no hit it is a violation at the post (the record shows no After). Where
// the After's line is recorded and left no hit, or only a hit of an earlier
// version of the rule's file, the Then is undecided, never violated: a lost
// record is a gap, not a verdict. A hit past the Then does not qualify it.
func TestSR1v_TheJudgeReadsHitsNotAfterLines(t *testing.T) {
	sp := ruleSpec(spec.Trusted, testFirst())
	thenAt := func(r *store.Run) (RuleVerdictValue, int) {
		v := oneVerdict(t, Evaluate(r, nil, sp, stopPoint(r), time.Time{}))
		return v.Verdict, len(RulesAtCall(sp, orderCall(t, r, 2, "git push origin main"), atPost(landedAt(t, r, 2)), postEvent).Findings)
	}
	v4 := func(hit bool) *store.Run {
		r := ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main"))
		for i := range r.Declarations {
			r.Declarations[i].Command = nil
		}
		if !hit {
			r.AfterHits = nil
		}
		return r
	}
	// The Stop cannot see the Then line at v4 (undecided); the post can.
	if _, n := thenAt(v4(true)); n != 0 {
		t.Errorf("a hit and withheld lines: %d findings at the post, want 0", n)
	}
	if _, n := thenAt(v4(false)); n != 1 {
		t.Errorf("no hit and withheld lines: %d findings at the post, want 1", n)
	}
	lost := ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main"))
	lost.AfterHits = nil
	if v, n := thenAt(lost); v != RuleUnknown || n != 0 {
		t.Errorf("an after line with no hit: verdict %v, %d findings, want unknown and none", v, n)
	}
	edited := ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main"))
	edited.AfterHits[0].SourceDigest = spec.DigestPrefix + "22"
	if v, n := thenAt(edited); v != RuleUnknown || n != 0 {
		t.Errorf("a hit of an earlier file: verdict %v, %d findings, want unknown and none", v, n)
	}
	other := ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main"))
	other.AfterHits[0].RuleID = "spec:another"
	if v, _ := thenAt(other); v != RuleUnknown {
		t.Errorf("a hit of another rule: verdict %v, want unknown", v)
	}
	later := ordered(t, shc(1, "git push origin main"), shc(2, "go test ./..."))
	if v := oneVerdict(t, Evaluate(later, nil, sp, stopPoint(later), time.Time{})); v.Verdict != Violation {
		t.Errorf("a hit after the Then: %+v, want violation", v)
	}
	maybe := ordered(t, shc(1, "bash -c 'go test ./...'"), shc(2, "git push origin main"))
	if v, n := thenAt(maybe); v != RuleUnknown || n != 0 {
		t.Errorf("a maybe hit: verdict %v, %d findings, want unknown and none", v, n)
	}
	unknownDir := ordered(t, shc(1, "go test ./..."), shc(2, "git push origin main"))
	unknownDir.AfterHits[0].CWDDigest = ""
	if v, n := thenAt(unknownDir); v != RuleUnknown || n != 0 {
		t.Errorf("a hit with no directory: verdict %v, %d findings, want unknown and none", v, n)
	}
}
