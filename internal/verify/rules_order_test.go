package verify

import (
	"slices"
	"strings"
	"testing"
	"time"

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

// ordered is a run of cs with each Bash call's line recorded (call.line).
func ordered(t *testing.T, cs ...call) *store.Run {
	t.Helper()
	m := map[int64]string{}
	for _, c := range cs {
		if c.tool == "Bash" && c.line != "" {
			m[c.seq] = c.line
		}
	}
	return withLines(runOf(t, cs...), m)
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
	c.line = line
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
	for _, seq := range []int64{1, 3} {
		if v := RulesAtCall(sp, orderCall(t, r, seq, ""), atPost(landedAt(t, r, seq)), postEvent); len(v.Findings) != 0 {
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
	here.loc = "d:repo"
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
		"then in bash -c":     ordered(t, shc(1, "go build ./..."), shc(2, "bash -c 'git push'")),
		"then expands":        ordered(t, shc(1, "go build ./..."), shc(2, "git $VERB origin")),
		"then in a heredoc":   ordered(t, shc(1, "go build ./..."), shc(2, "bash <<EOF\ngit push\nEOF")),
		"after in bash -c":    ordered(t, shc(1, "bash -c 'go test ./...'"), shc(2, "git push origin main")),
		"after expands":       ordered(t, shc(1, "go $SUB ./..."), shc(2, "git push origin main")),
		"both in one line":    ordered(t, shc(1, "go test ./... && git push origin main")),
		"glob write between":  glob,
		"unnamed write":       blind,
		"after line withheld": withheld,
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
