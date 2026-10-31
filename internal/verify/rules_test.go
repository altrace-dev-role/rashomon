package verify

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ruleSpec is a compiled spec of one repository source at trust tr holding
// rules, each under its declared id "spec:<id>", as Discover merges them.
func ruleSpec(tr spec.Trust, rules ...spec.Rule) *spec.Compiled {
	sp := &spec.Compiled{
		Sources: []spec.Source{{Path: ".claude/rashomon/spec.json", Digest: spec.DigestPrefix + strings.Repeat("11", 32), Trust: tr}},
		Digest:  spec.DigestPrefix + strings.Repeat("ab", 32),
		Trust:   tr,
	}
	for _, r := range rules {
		sp.Rules = append(sp.Rules, spec.DeclaredRule{ID: "spec:" + r.ID, Source: 0, Rule: r})
	}
	return sp
}

var allModes = []string{"modify", "delete", "create"}

// noForce forbids git push with --force or -f.
func noForce() spec.Rule {
	return spec.Rule{ID: "no-force", Kind: string(spec.ForbidCommand), Why: "history is shared", HardControl: "branch protection",
		Command: &spec.CommandMatch{Argv: []string{"git", "push"}, Flags: []string{"--force", "-f"}}}
}

// frozen protects db/migrations in modes.
func frozen(modes ...string) spec.Rule {
	return spec.Rule{ID: "frozen-migrations", Kind: string(spec.ProtectPath), Why: "applied migrations are immutable",
		HardControl: "CODEOWNERS", Paths: []string{"db/migrations/**"}, Modes: modes}
}

// needs requires evidence at points.
func needs(evidence []string, at ...string) spec.Rule {
	return spec.Rule{ID: "evidence-first", Kind: string(spec.RequireEvidence), Why: "w", HardControl: "CI",
		At: at, Evidence: evidence}
}

// sh is a Bash call that runs line; the line itself is set by withLines.
func sh(seq int64, line string) call {
	prog, _, _ := strings.Cut(line, " ")
	return call{seq: seq, tool: "Bash", prog: prog, class: shape.VerbExecute, outcome: store.ExecOK}
}

// withLines records each call's command line on its declaration, keyed by
// the call's seq (its ToolUseID "t<seq>"), as record scope v5 keeps it.
func withLines(r *store.Run, lines map[int64]string) *store.Run {
	for i := range r.Declarations {
		d := &r.Declarations[i]
		k, err := strconv.ParseInt(strings.TrimPrefix(d.ToolUseID, "t"), 10, 64)
		if l, ok := lines[k]; ok && err == nil {
			d.Command = &l
		}
	}
	return r
}

// shells is a run of one Bash call per line, each line recorded.
func shells(t *testing.T, lines ...string) *store.Run {
	t.Helper()
	calls := make([]call, len(lines))
	m := make(map[int64]string, len(lines))
	for i, l := range lines {
		calls[i], m[int64(i+1)] = sh(int64(i+1), l), l
	}
	return withLines(runOf(t, calls...), m)
}

func stopPoint(r *store.Run) Point { return Point{Kind: PointStop, Seq: r.LastSeq(), SessionID: "s1"} }

// specLedgerAt is the snapshot at pt and its ledger, built as EvaluateStop
// builds them: evaluate, then ledgerOf, the one producer of the rows.
func specLedgerAt(r *store.Run, sp *spec.Compiled, pt Point) (Snapshot, []Obligation) {
	var c classified
	snap := evaluate(r, nil, sp, nil, pt, time.Time{}, &c)
	return snap, ledgerOf(snap, &c, expected(snap, evidenceIn{sp: sp, wrote: WroteRelevant(r, pt.Seq)}))
}

// specRows is the spec_rule rows of obs.
func specRows(obs []Obligation) []Obligation {
	var out []Obligation
	for _, o := range obs {
		if o.Kind == SpecRule {
			out = append(out, o)
		}
	}
	return out
}

func oneVerdict(t *testing.T, snap Snapshot) RuleVerdict {
	t.Helper()
	if len(snap.SpecVerdicts) != 1 {
		t.Fatalf("spec verdicts %+v, want one", snap.SpecVerdicts)
	}
	return snap.SpecVerdicts[0]
}

// atPost is a post point for a call that landed at seq.
func atPost(seq int64) Point { return Point{Seq: seq, SessionID: "s1"} }

const postEvent = string(steer.EventPostToolUse)

// TestSR1_ForbidCommandViolatedByTheCallRaisesAndRowsUnmet: a trusted
// forbid_command rule matched at command position is a violation citing the
// call's declaration; the ledger's spec_rule row is unmet citing it, sourced
// "spec"; a clean run is no_recorded_violation with a met row citing nothing;
// the post raises platform-rule/command at nudge from the call alone, with
// the rule id and the program and no rule text, at PostToolUse only.
func TestSR1_ForbidCommandViolatedByTheCallRaisesAndRowsUnmet(t *testing.T) {
	sp := ruleSpec(spec.Trusted, noForce())
	r := shells(t, "git status", "git push --force origin main", "git push origin main")
	snap, obs := specLedgerAt(r, sp, stopPoint(r))
	bad := declOf(r, 2).Seq
	want := RuleVerdict{Rule: "spec:no-force", Kind: spec.ForbidCommand, Verdict: Violation, Seqs: []int64{bad}, HardControl: "branch protection"}
	if v := oneVerdict(t, snap); v.Rule != want.Rule || v.Kind != want.Kind || v.Verdict != want.Verdict ||
		!slices.Equal(v.Seqs, want.Seqs) || v.HardControl != want.HardControl {
		t.Fatalf("verdict %+v, want %+v", v, want)
	}
	rows := specRows(obs)
	if len(rows) != 1 {
		t.Fatalf("spec rows %+v, want one", rows)
	}
	if o := rows[0]; o.ID != ObligationID(ScopeSession, SpecRule, "spec:no-force") || o.Scope != ScopeSession ||
		o.Source != "spec" || o.State != ObligationUnmet || !slices.Equal(o.Evidence, []int64{bad}) {
		t.Fatalf("row %+v, want unmet citing %d", o, bad)
	}

	clean := shells(t, "git status", "git push origin main")
	snap, obs = specLedgerAt(clean, sp, stopPoint(clean))
	if v := oneVerdict(t, snap); v.Verdict != NoRecordedViolation || v.Seqs == nil || len(v.Seqs) != 0 {
		t.Fatalf("clean verdict %+v, want no_recorded_violation with no seqs", v)
	}
	if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationMet || rows[0].Evidence == nil || len(rows[0].Evidence) != 0 {
		t.Fatalf("clean rows %+v, want one met citing nothing", rows)
	}

	view := RulesAtCall(sp, RuleCall{Seq: 9, ToolName: "Bash", Command: "git push -f origin main"}, atPost(9), postEvent)
	if len(view.Findings) != 1 || view.Dropped != 0 {
		t.Fatalf("findings %+v dropped %d, want one", view.Findings, view.Dropped)
	}
	f := view.Findings[0]
	if f.Pattern != pattern.PlatformRule || f.Kind != "command" || f.MaxLevel != steer.LevelNudge ||
		!slices.Equal(f.Evidence, []int64{9}) || f.AfterSeq != 9 {
		t.Fatalf("finding %+v, want platform-rule/command at nudge citing 9", f)
	}
	if got := f.Message.Text(); got != "rashomon record: this 'git' call matched spec rule no-force." {
		t.Fatalf("note %q", got)
	}
	if v := RulesAtCall(sp, RuleCall{Seq: 9, ToolName: "Bash", Command: "git push origin main"}, atPost(9), postEvent); len(v.Findings) != 0 {
		t.Fatalf("a clean call raised %+v", v.Findings)
	}
	// The card raises platform-rule at PostToolUse alone: a forced push that
	// failed is the record's, and the post neither raises it nor hands the
	// mapper a finding to refuse.
	failed := string(steer.EventPostToolUseFailure)
	if v := RulesAtCall(sp, RuleCall{Seq: 9, ToolName: "Bash", Command: "git push -f origin main"}, atPost(9), failed); len(v.Findings) != 0 || v.Dropped != 0 {
		t.Fatalf("at a failure: findings %+v dropped %d, want neither", v.Findings, v.Dropped)
	}
}

// TestSR1b_ProtectPathWriteInMode: a write to a path the rule's paths match
// and its exclusions do not, in a listed mode. Edit, MultiEdit and
// NotebookEdit modify; a Write creates or modifies, which the record does
// not tell apart, and never deletes: it violates a rule listing both, is
// undecided under one of them and is none under delete alone. A Bash target
// may be any of the three. A pattern naming a directory covers what is under
// it (the matcher's parent rule), not a prefix of its name. A path outside
// the project matches nothing; an MCP call is no write a path rule reads.
// The post raises integrity/frozen-path, a user flag, naming the path, at
// either post event.
func TestSR1b_ProtectPathWriteInMode(t *testing.T) {
	excl := frozen(allModes...)
	excl.ExcludePaths = []string{"db/migrations/README.md"}
	dir, prefix := frozen(allModes...), frozen(allModes...)
	dir.Paths, prefix.Paths = []string{"db/migrations"}, []string{"db/migr"}
	write := func(seq int64, p string) call {
		return call{seq: seq, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{p}}
	}
	for _, c := range []struct {
		name string
		rule spec.Rule
		call call
		want RuleVerdictValue
	}{
		{"edit in modify", frozen("modify", "delete"), edit(1, "db/migrations/001.sql"), Violation},
		{"edit elsewhere", frozen("modify", "delete"), edit(1, "db/schema.sql"), NoRecordedViolation},
		{"edit under create only", frozen("create"), edit(1, "db/migrations/001.sql"), NoRecordedViolation},
		{"write, mode untold", frozen("modify", "delete"), write(1, "db/migrations/002.sql"), RuleUnknown},
		{"write, every mode", frozen(allModes...), write(1, "db/migrations/002.sql"), Violation},
		{"write, create and modify", frozen("create", "modify"), write(1, "db/migrations/002.sql"), Violation},
		{"write, delete only", frozen("delete"), write(1, "db/migrations/002.sql"), NoRecordedViolation},
		{"edit under a directory pattern", dir, edit(1, "db/migrations/001.sql"), Violation},
		{"edit under a name prefix", prefix, edit(1, "db/migrations/001.sql"), NoRecordedViolation},
		{"mcp call", frozen(allModes...), call{seq: 1, tool: "mcp__fs__write", class: shape.VerbMCP, outcome: store.ExecOK}, NoRecordedViolation},
		{"bash target, mode untold", frozen("modify"), bashWrite(1, "db/migrations/003.sql"), RuleUnknown},
		{"bash target, every mode", frozen(allModes...), bashWrite(1, "db/migrations/003.sql"), Violation},
		{"excluded", excl, edit(1, "db/migrations/README.md"), NoRecordedViolation},
		{"outside the project", frozen(allModes...), edit(1, "/tmp/db/migrations/001.sql"), NoRecordedViolation},
	} {
		r := runOf(t, c.call)
		snap := Evaluate(r, nil, ruleSpec(spec.Trusted, c.rule), stopPoint(r), time.Time{})
		v := oneVerdict(t, snap)
		if v.Verdict != c.want || v.Kind != spec.ProtectPath {
			t.Errorf("%s: verdict %+v, want %s", c.name, v, c.want)
		}
		if c.want == Violation && !slices.Equal(v.Seqs, []int64{declOf(r, 1).Seq}) {
			t.Errorf("%s: seqs %v, want the write's declaration", c.name, v.Seqs)
		}
	}

	sp := ruleSpec(spec.Trusted, frozen("modify", "delete"))
	view := RulesAtCall(sp, RuleCall{Seq: 7, ToolName: "Edit", FilePath: "/repo/db/migrations/001.sql", Project: "/repo"}, atPost(7), postEvent)
	if len(view.Findings) != 1 {
		t.Fatalf("findings %+v, want one", view.Findings)
	}
	f := view.Findings[0]
	if f.Pattern != pattern.Integrity || f.Kind != "frozen-path" || f.MaxLevel != steer.LevelFlag || !slices.Equal(f.Evidence, []int64{7}) {
		t.Fatalf("finding %+v, want integrity/frozen-path, a flag, citing 7", f)
	}
	if got := f.Message.Text(); got != "rashomon record: this call wrote 'db/migrations/001.sql', which spec rule frozen-migrations protects." {
		t.Fatalf("line %q", got)
	}
	for _, rc := range []RuleCall{
		{Seq: 7, ToolName: "Edit", FilePath: "/repo/db/schema.sql", Project: "/repo"},
		{Seq: 7, ToolName: "Write", FilePath: "/repo/db/migrations/002.sql", Project: "/repo"},
		{Seq: 7, ToolName: "Read", FilePath: "/repo/db/migrations/001.sql", Project: "/repo"},
	} {
		if v := RulesAtCall(sp, rc, atPost(7), postEvent); len(v.Findings) != 0 {
			t.Errorf("%+v raised %+v", rc, v.Findings)
		}
	}
	failed := RuleCall{Seq: 7, ToolName: "Edit", FilePath: "/repo/db/migrations/001.sql", Project: "/repo"}
	if v := RulesAtCall(sp, failed, atPost(7), string(steer.EventPostToolUseFailure)); len(v.Findings) != 1 {
		t.Errorf("an edit that failed raised %+v, want the flag: integrity is raised at either post event", v.Findings)
	}
}

// requireAt is ruleVerdicts over a snapshot of gates at a point of kind kind,
// the window having a relevant write or not: the declared-gate rows the
// snapshot holds once DECLARED-GATES1 lands are given here by hand.
func requireAt(sp *spec.Compiled, kind string, wrote bool, gates ...GateState) []RuleVerdict {
	snap := Snapshot{Recording: Complete, Point: Point{Kind: kind, Seq: 20, SessionID: "s1"}, Gates: gates}
	return ruleVerdicts(sp, &store.Run{}, nil, &classified{}, &snap, wrote)
}

func declaredIn(id, kind string, s State) GateState {
	return GateState{Gate: "spec:" + id, Kind: kind, Source: "declared", Required: true, State: s}
}

// TestSR1c_RequireEvidenceAtStop: done is read at the Stop point, and only
// when the window holds a relevant write; commit at a commit point. Every
// named gate Green is no_recorded_violation; one red, stale or absent is a
// violation citing the point; an unknown state is unknown; kind:<k> is met
// by any gate of that kind Green. A point the rule does not name gives no
// verdict.
func TestSR1c_RequireEvidenceAtStop(t *testing.T) {
	unit := []string{"unit"}
	for _, c := range []struct {
		name  string
		rule  spec.Rule
		kind  string
		wrote bool
		gates []GateState
		want  RuleVerdictValue // "" is no verdict
	}{
		{"green at done", needs(unit, "done"), PointStop, true, []GateState{declaredIn("unit", "test", Green)}, NoRecordedViolation},
		{"red at done", needs(unit, "done"), PointStop, true, []GateState{declaredIn("unit", "test", Red)}, Violation},
		{"stale at done", needs(unit, "done"), PointStop, true, []GateState{declaredIn("unit", "test", Stale)}, Violation},
		{"absent at done", needs(unit, "done"), PointStop, true, []GateState{declaredIn("unit", "test", Absent)}, Violation},
		{"unknown at done", needs(unit, "done"), PointStop, true, []GateState{declaredIn("unit", "test", Unknown)}, RuleUnknown},
		{"no write at done", needs(unit, "done"), PointStop, false, []GateState{declaredIn("unit", "test", Red)}, ""},
		{"done at a commit", needs(unit, "done"), PointCommit, true, []GateState{declaredIn("unit", "test", Red)}, ""},
		{"commit at a commit", needs(unit, "commit"), PointCommit, false, []GateState{declaredIn("unit", "test", Red)}, Violation},
		{"push at Stop", needs(unit, "push"), PointStop, true, []GateState{declaredIn("unit", "test", Red)}, ""},
		{"one of two red", needs([]string{"unit", "lint"}, "done"), PointStop, true,
			[]GateState{declaredIn("unit", "test", Green), declaredIn("lint", "lint", Red)}, Violation},
		{"kind met by a learned gate", needs([]string{"kind:test"}, "done"), PointStop, true,
			[]GateState{{Gate: "learned:test:r:go-test@d:repo", Kind: "test", Source: "learned", Required: true, State: Green}}, NoRecordedViolation},
		{"kind, one of two green", needs([]string{"kind:test"}, "done"), PointStop, true,
			[]GateState{declaredIn("a", "test", Red), declaredIn("b", "test", Green)}, NoRecordedViolation},
		{"kind, none green", needs([]string{"kind:test"}, "done"), PointStop, true,
			[]GateState{declaredIn("a", "test", Red)}, Violation},
		{"no evidence named", needs(nil, "done"), PointStop, true, nil, RuleUnknown},
	} {
		got := requireAt(ruleSpec(spec.Trusted, c.rule), c.kind, c.wrote, c.gates...)
		switch {
		case c.want == "" && len(got) != 0:
			t.Errorf("%s: %+v, want no verdict", c.name, got)
		case c.want != "" && (len(got) != 1 || got[0].Verdict != c.want || got[0].Kind != spec.RequireEvidence):
			t.Errorf("%s: %+v, want %s", c.name, got, c.want)
		case c.want == Violation && !slices.Equal(got[0].Seqs, []int64{20}):
			t.Errorf("%s: seqs %v, want the point's", c.name, got[0].Seqs)
		}
	}
}

// TestSR1i_RequireEvidenceNamingAGateTheSnapshotLacksIsUnknown: a gate id or
// kind no snapshot gate carries reads unknown, never a violation, so a
// session on a tree where declared gates are not yet snapshot rows raises
// nothing false; through Evaluate, its row is unknown, and record-green is
// untouched.
func TestSR1i_RequireEvidenceNamingAGateTheSnapshotLacksIsUnknown(t *testing.T) {
	for _, ev := range [][]string{{"unit"}, {"kind:scan"}, {"unit", "kind:scan"}} {
		got := requireAt(ruleSpec(spec.Trusted, needs(ev, "done")), PointStop, true,
			GateState{Gate: "learned:test:r:go-test@d:repo", Kind: "test", Source: "learned", Required: true, State: Green})
		if len(got) != 1 || got[0].Verdict != RuleUnknown {
			t.Errorf("%v: %+v, want unknown", ev, got)
		}
	}
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	snap, obs := specLedgerAt(r, ruleSpec(spec.Trusted, needs([]string{"unit"}, "done")), stopPoint(r))
	if v := oneVerdict(t, snap); v.Verdict != RuleUnknown {
		t.Fatalf("verdict %+v, want unknown", v)
	}
	if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnknown || len(rows[0].Evidence) != 0 {
		t.Fatalf("rows %+v, want one unknown citing nothing", rows)
	}
	if !snap.RecordGreen {
		t.Fatal("an unknown rule cleared record-green")
	}
}

// TestSR1d_UntrustedSpecFlagsTheUserOnly: an untrusted rule's verdict is
// kept, but it owes no ledger row, never clears record-green, and its post
// finding is a user flag. Trust is per rule: a managed rule beside an
// untrusted repository spec keeps its row although the Compiled's trust is
// the lowest.
func TestSR1d_UntrustedSpecFlagsTheUserOnly(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), sh(3, "git push --force"))
	r = withLines(r, map[int64]string{3: "git push --force"})
	sp := ruleSpec(spec.Untrusted, noForce())
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
	view := RulesAtCall(sp, RuleCall{Seq: 9, ToolName: "Bash", Command: "git push --force"}, atPost(9), postEvent)
	if len(view.Findings) != 1 || view.Findings[0].MaxLevel != steer.LevelFlag {
		t.Fatalf("findings %+v, want one flag", view.Findings)
	}

	mixed := &spec.Compiled{
		Sources: []spec.Source{
			{Path: "/managed/rashomon/spec.json", Digest: spec.DigestPrefix + strings.Repeat("22", 32), Trust: spec.Managed},
			{Path: ".claude/rashomon/spec.json", Digest: spec.DigestPrefix + strings.Repeat("11", 32), Trust: spec.Untrusted},
		},
		Trust: spec.Untrusted,
		Rules: []spec.DeclaredRule{{ID: "spec:no-force", Source: 0, Rule: noForce()}},
	}
	snap, obs = specLedgerAt(r, mixed, stopPoint(r))
	if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnmet {
		t.Fatalf("managed rows %+v, want one unmet", rows)
	}
	if snap.RecordGreen {
		t.Fatal("a managed violation left record-green")
	}
	view = RulesAtCall(mixed, RuleCall{Seq: 9, ToolName: "Bash", Command: "git push --force"}, atPost(9), postEvent)
	if len(view.Findings) != 1 || view.Findings[0].MaxLevel != steer.LevelNudge {
		t.Fatalf("managed findings %+v, want one nudge", view.Findings)
	}
}

// TestSR1e_UndecidedNeverFires: what the record or the line cannot decide is
// undecided: unknown verdict, unknown row, no finding. A command withheld (no
// record scope v5) or cut; a masked line, short of what it still shows; a
// shell's -c string, eval, a here-document or here-string into a shell, a
// wrapper before the rule's program, an expansion where the rule's words or
// flags stand; a line the shape cannot read. A write the record cannot name,
// and a file tool whose path was withheld. A violation elsewhere in the
// window still beats every undecided call.
func TestSR1e_UndecidedNeverFires(t *testing.T) {
	sp := ruleSpec(spec.Trusted, noForce())
	for _, line := range []string{
		"bash -c 'git push --force'",
		"sh -c \"git push -f\"",
		"eval git push --force",
		"bash <<<'git push --force'",
		"sudo git push --force",
		"env GIT_TRACE=1 git push --force",
		"git push $FLAGS",
		"git $SUB --force",
		"$(echo git) push --force",
	} {
		r := shells(t, line)
		snap, obs := specLedgerAt(r, sp, stopPoint(r))
		if v := oneVerdict(t, snap); v.Verdict != RuleUnknown || len(v.Seqs) != 0 {
			t.Errorf("%q: verdict %+v, want unknown", line, v)
		}
		if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnknown {
			t.Errorf("%q: rows %+v, want one unknown", line, rows)
		}
		if v := RulesAtCall(sp, RuleCall{Seq: 5, ToolName: "Bash", Command: line}, atPost(5), postEvent); len(v.Findings) != 0 {
			t.Errorf("%q raised %+v", line, v.Findings)
		}
	}

	withheld := shells(t, "git push --force")
	withheld.Declarations[0].Command = nil
	cut := shells(t, "git push --force")
	cut.Declarations[0].CommandCut = &store.Cut{Offset: 4, RemovedBytes: 100}
	// A marker stands where a value was: a line holding one decides nothing
	// it does not show, and what it shows still decides.
	masked := shells(t, "git push origin TOKEN")
	masked.Declarations[0].Masked = []store.MaskSpan{{Rule: "token", Start: 16, End: 21}}
	for name, r := range map[string]*store.Run{"withheld": withheld, "cut": cut, "masked": masked} {
		if v := oneVerdict(t, Evaluate(r, nil, sp, stopPoint(r), time.Time{})); v.Verdict != RuleUnknown {
			t.Errorf("%s: verdict %+v, want unknown", name, v)
		}
	}
	shown := shells(t, "git push --force origin TOKEN")
	shown.Declarations[0].Masked = []store.MaskSpan{{Rule: "token", Start: 24, End: 29}}
	if v := oneVerdict(t, Evaluate(shown, nil, sp, stopPoint(shown), time.Time{})); v.Verdict != Violation {
		t.Errorf("a masked line's shown flag: verdict %+v, want violation", v)
	}

	both := shells(t, "bash -c 'git push --force'", "git push --force")
	if v := oneVerdict(t, Evaluate(both, nil, sp, stopPoint(both), time.Time{})); v.Verdict != Violation || !slices.Equal(v.Seqs, []int64{declOf(both, 2).Seq}) {
		t.Errorf("a violation beside an undecided call: %+v, want the violation", v)
	}

	fp := ruleSpec(spec.Trusted, frozen(allModes...))
	blind := runOf(t, unnamedWrite(1))
	noPath := runOf(t, edit(1, "db/migrations/001.sql"))
	noPath.Declarations[0].FilePath = nil
	for name, r := range map[string]*store.Run{"unnamed bash write": blind, "path withheld": noPath} {
		if v := oneVerdict(t, Evaluate(r, nil, fp, stopPoint(r), time.Time{})); v.Verdict != RuleUnknown {
			t.Errorf("%s: verdict %+v, want unknown", name, v)
		}
	}
}

// TestSR1g_VerdictsReachTheCheckpoint: the checkpoint keeps rule, kind,
// verdict and seqs, never hard_control.
func TestSR1g_VerdictsReachTheCheckpoint(t *testing.T) {
	r := shells(t, "git push --force")
	snap := Evaluate(r, nil, ruleSpec(spec.Trusted, noForce()), stopPoint(r), time.Time{})
	cp := Cut(snap, time.UnixMilli(1))
	want := []CheckpointRuleVerdict{{Rule: "spec:no-force", Kind: spec.ForbidCommand, Verdict: Violation, Seqs: []int64{declOf(r, 1).Seq}}}
	if len(cp.SpecVerdicts) != 1 || cp.SpecVerdictsOmitted != 0 || cp.SpecVerdicts[0].Rule != want[0].Rule ||
		cp.SpecVerdicts[0].Verdict != want[0].Verdict || !slices.Equal(cp.SpecVerdicts[0].Seqs, want[0].Seqs) {
		t.Fatalf("checkpoint spec verdicts %+v omitted %d, want %+v", cp.SpecVerdicts, cp.SpecVerdictsOmitted, want)
	}
	b, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("hard_control")) || bytes.Contains(b, []byte("branch protection")) {
		t.Fatalf("the checkpoint carries the rule's text: %s", b)
	}
}

// TestSR1h_TrustedViolationClearsRecordGreen: PF-PG8's record-green point
// holds no spec violation. A trusted violation clears it, and the value line
// counts it as a record finding; an unknown verdict does not.
func TestSR1h_TrustedViolationClearsRecordGreen(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), sh(3, "git push --force"))
	r = withLines(r, map[int64]string{3: "git push --force"})
	if snap := Evaluate(r, nil, nil, stopPoint(r), time.Time{}); !snap.RecordGreen {
		t.Fatal("the fixture is not record-green without a spec")
	}
	snap := Evaluate(r, nil, ruleSpec(spec.Trusted, noForce()), stopPoint(r), time.Time{})
	wantNoGreen(t, snap)
	s := TurnSummary(snap, TurnCounts{Writes: 1, GateRuns: 1})
	if s.Row != RowFindings || s.Sentence != findingsSentence(1) {
		t.Fatalf("summary %+v, want the findings row counting 1", s)
	}

	r.Declarations[len(r.Declarations)-1].Command = nil
	if snap := Evaluate(r, nil, ruleSpec(spec.Trusted, noForce()), stopPoint(r), time.Time{}); !snap.RecordGreen {
		t.Fatal("an unknown verdict cleared record-green")
	}
}

// TestSR1j_ImpliedTaskRuleIsStoreTouchedsNotARow: the protect_path rule a
// task file implies (Source SourceImplied, trust builtin) keeps its verdict
// and owes no row and raises no finding: its writes are store-touched's,
// raised at the call already.
func TestSR1j_ImpliedTaskRuleIsStoreTouchedsNotARow(t *testing.T) {
	sp := ruleSpec(spec.Trusted)
	sp.Task = &spec.Task{ID: "t1", Goal: "g", Decision: "d", ExpectedEvidence: []string{"mechanism_stated"}}
	sp.Rules = append(sp.Rules, spec.DeclaredRule{ID: "builtin:task-file", Source: spec.SourceImplied, Rule: spec.Rule{
		ID: "task-file", Kind: string(spec.ProtectPath), Why: "w", HardControl: "c",
		Paths: []string{".claude/rashomon/task/**"}, Modes: allModes}})
	r := runOf(t, edit(1, ".claude/rashomon/task/current.json"))
	snap, obs := specLedgerAt(r, sp, stopPoint(r))
	if v := oneVerdict(t, snap); v.Rule != "builtin:task-file" || v.Verdict != Violation {
		t.Fatalf("verdict %+v, want builtin:task-file violated", v)
	}
	if rows := specRows(obs); len(rows) != 0 {
		t.Fatalf("rows %+v, want none", rows)
	}
	if !slices.ContainsFunc(snap.Integrity.Unresolved, func(e IntegrityEntry) bool { return e.Check == StoreTouched }) {
		t.Fatalf("no store_touched entry: %+v", snap.Integrity.Unresolved)
	}
	view := RulesAtCall(sp, RuleCall{Seq: 3, ToolName: "Edit", FilePath: "/repo/.claude/rashomon/task/current.json", Project: "/repo"}, atPost(3), postEvent)
	if len(view.Findings) != 0 {
		t.Fatalf("findings %+v, want none", view.Findings)
	}
}

// TestSR1k_VerdictsAndRowsUnderShuffledRecords: VER18's order independence
// with rules: the verdicts and the spec rows are byte-identical over 100
// shuffles of the run's records.
func TestSR1k_VerdictsAndRowsUnderShuffledRecords(t *testing.T) {
	r := runOf(t, sh(1, "git push --force"), edit(2, "db/migrations/001.sql"), bashWrite(3, "db/migrations/002.sql"),
		sh(4, "git push -f origin"), unnamedWrite(5), edit(6, "pkg/a.go"))
	r = withLines(r, map[int64]string{1: "git push --force", 4: "git push -f origin"})
	sp := ruleSpec(spec.Trusted, noForce(), frozen("modify"), frozen(allModes...))
	sp.Rules[2].ID, sp.Rules[2].Rule.ID = "spec:frozen-all", "frozen-all"
	encodeAt := func(r *store.Run) []byte {
		snap, obs := specLedgerAt(r, sp, stopPoint(r))
		b, err := json.Marshal(struct {
			V []RuleVerdict
			O []Obligation
		}{snap.SpecVerdicts, specRows(obs)})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	want := encodeAt(r)
	if len(want) == 0 || !bytes.Contains(want, []byte(`"violation"`)) {
		t.Fatalf("fixture %s has no violation", want)
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(r.Declarations), func(i, j int) { r.Declarations[i], r.Declarations[j] = r.Declarations[j], r.Declarations[i] })
		rng.Shuffle(len(r.Executions), func(i, j int) { r.Executions[i], r.Executions[j] = r.Executions[j], r.Executions[i] })
		if got := encodeAt(r); !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d: %s, want %s", i, got, want)
		}
	}
}

// TestSR1l_ForbidCommandConditions: archive 20.2.8's forbid_command row, read
// the same at Stop and at the post (one matcher): argv at command position,
// the program by its base name; then any flags token as an exact word or its
// flag= form, a flag_values value case-insensitively in either form, or
// every require_flags token absent; no condition at all is the argv alone.
// An expansion that could be the missing word is undecided; SQL is
// undecided in 1a (ACCEPTED GAP: no SQL reader yet). ACCEPTED GAP: options
// between the argv words are not stepped over, so `git -C repo push
// --force` does not match argv [git push].
func TestSR1l_ForbidCommandConditions(t *testing.T) {
	values := spec.Rule{ID: "no-prod", Kind: string(spec.ForbidCommand), Why: "w", HardControl: "c",
		Command: &spec.CommandMatch{Argv: []string{"deploy"}, FlagValues: map[string][]string{"--target": {"Prod"}}}}
	dry := spec.Rule{ID: "dry-run-only", Kind: string(spec.ForbidCommand), Why: "w", HardControl: "c",
		Command: &spec.CommandMatch{Argv: []string{"terraform", "apply"}, RequireFlags: []string{"--dry-run", "-n"}}}
	bare := spec.Rule{ID: "no-destroy", Kind: string(spec.ForbidCommand), Why: "w", HardControl: "c",
		Command: &spec.CommandMatch{Argv: []string{"terraform", "destroy"}}}
	sql := spec.Rule{ID: "no-truncate", Kind: string(spec.ForbidCommand), Why: "w", HardControl: "c",
		Command: &spec.CommandMatch{Argv: []string{"cqlsh"}, SQL: []string{"TRUNCATE"}}}
	for _, c := range []struct {
		rule spec.Rule
		line string
		want RuleVerdictValue
	}{
		{noForce(), "git push --force", Violation},
		{noForce(), "git push origin main --force", Violation},
		{noForce(), "git push --force=yes", Violation},
		{noForce(), "/usr/bin/git push -f", Violation},
		{noForce(), "git push --force-with-lease", NoRecordedViolation},
		{noForce(), "git push origin '--force'", Violation},
		{noForce(), "git pull --force", NoRecordedViolation},
		{noForce(), "echo git push --force", NoRecordedViolation},
		{noForce(), "git -C repo push --force", NoRecordedViolation}, // ACCEPTED GAP, above
		{noForce(), "git push origin $BRANCH", RuleUnknown},
		{noForce(), "git push --force origin $BRANCH", Violation},
		{values, "deploy --target prod", Violation},
		{values, "deploy --target=PROD", Violation},
		{values, "deploy --target dev", NoRecordedViolation},
		{values, "deploy --target $ENV", RuleUnknown},
		{dry, "terraform apply", Violation},
		{dry, "terraform apply --dry-run", NoRecordedViolation},
		{dry, "terraform apply -n", NoRecordedViolation},
		{dry, "terraform apply $ARGS", RuleUnknown},
		{dry, "terraform plan", NoRecordedViolation},
		{bare, "terraform destroy -auto-approve", Violation},
		{bare, "terraform plan", NoRecordedViolation},
		{sql, "cqlsh -e 'TRUNCATE ks.t'", RuleUnknown}, // ACCEPTED GAP, above
		{sql, "psql -c 'TRUNCATE t'", NoRecordedViolation},
	} {
		sp := ruleSpec(spec.Trusted, c.rule)
		r := shells(t, c.line)
		v := oneVerdict(t, Evaluate(r, nil, sp, stopPoint(r), time.Time{}))
		if v.Verdict != c.want {
			t.Errorf("%s over %q: %s, want %s", c.rule.ID, c.line, v.Verdict, c.want)
		}
		raised := len(RulesAtCall(sp, RuleCall{Seq: 3, ToolName: "Bash", Command: c.line}, atPost(3), postEvent).Findings) == 1
		if raised != (c.want == Violation) {
			t.Errorf("%s over %q: the post raised %t, the record reads %s", c.rule.ID, c.line, raised, c.want)
		}
	}
}

// TestSR1m_EachCommandIsReadOncePerEvaluation: a command line is read once
// per evaluation, not once per rule: sixteen rules over forty lines cost
// little more than one rule does, where reading per rule would cost every
// line fifteen times over.
func TestSR1m_EachCommandIsReadOncePerEvaluation(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "git commit -m 'step " + strconv.Itoa(i) + "' && go test ./internal/... | tail -20"
	}
	r := shells(t, lines...)
	one := ruleSpec(spec.Trusted, noForce())
	many := ruleSpec(spec.Trusted)
	for i := range 16 {
		rl := noForce()
		rl.ID = "no-force-" + strconv.Itoa(i)
		many.Rules = append(many.Rules, spec.DeclaredRule{ID: "spec:" + rl.ID, Rule: rl})
	}
	pt := stopPoint(r)
	a1 := testing.AllocsPerRun(5, func() { Evaluate(r, nil, one, pt, time.Time{}) })
	a16 := testing.AllocsPerRun(5, func() { Evaluate(r, nil, many, pt, time.Time{}) })
	if a16-a1 > 200 {
		t.Fatalf("16 rules cost %.0f allocations over one rule's %.0f: the lines are read per rule", a16, a1)
	}
}
