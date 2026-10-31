package recap

import (
	"encoding/json"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// finding is a delivered steer line of pattern naming gate, cited at seq.
func finding(t *testing.T, id, pat, gate string, seq int64, model string) []byte {
	t.Helper()
	m := map[string]any{"id": id, "state": "delivered", "prompt_id": "p1", "event": "PostToolUseFailure", "pattern": pat,
		"model_text": model, "after_seq": seq, "evidence_seqs": []int64{seq}}
	if gate != "" {
		m["slots"] = map[string]any{"gate": gate}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRP1_ResolvedOnlyByALaterGreen: a finding is resolved by a green run of
// the gate its line names that came after it, and by nothing else: not a
// green at or before it, not a gate it does not name, not a green that has
// since gone stale. The resolving seq is the green run's.
func TestRP1_ResolvedOnlyByALaterGreen(t *testing.T) {
	line := [][]byte{finding(t, "a", "regression-location", "go", 37, "x")}
	for _, c := range []struct {
		name  string
		gate  verify.GateState
		state FindingState
		by    int64
	}{
		{"a later green", gateAt("go:test", "test", verify.Green, verify.OK, 41, nil), FindingResolved, 41},
		{"a green before it", gateAt("go:test", "test", verify.Green, verify.OK, 30, nil), FindingOpen, 0},
		{"a green at its own seq", gateAt("go:test", "test", verify.Green, verify.OK, 37, nil), FindingOpen, 0},
		{"another gate", gateAt("npm:test", "test", verify.Green, verify.OK, 41, nil), FindingOpen, 0},
		{"a red later run", gateAt("go:test", "test", verify.Red, verify.FailedExit, 41, exitCode(1)), FindingOpen, 0},
	} {
		im := ImpactOf(viewOf(verify.RowVerified, wrote, c.gate), line, nil)
		if len(im.Findings) != 1 || im.Findings[0].State != c.state || im.Findings[0].ResolvedBy != c.by || im.Findings[0].Seq != 37 {
			t.Errorf("%s: findings %+v, want one at 37, %s by %d", c.name, im.Findings, c.state, c.by)
		}
	}
	// A line that names no gate is counted and settled by no run.
	im := ImpactOf(viewOf(verify.RowVerified, wrote, gateAt("go:test", "test", verify.Green, verify.OK, 41, nil)),
		[][]byte{finding(t, "b", "claim-vs-record", "", 12, "")}, nil)
	if len(im.Findings) != 1 || im.Findings[0].State != FindingUnchecked || len(im.Resolved()) != 0 {
		t.Errorf("a finding naming no gate: %+v, want unchecked and not resolved", im.Findings)
	}
}

// TestRP1_CountsAreRecordsWithSeqs: notes and findings are lines counted
// once by id, a flag (no model text) is a finding and no note, a suppressed
// line and a line that does not read count for nothing, and a briefing is a
// note and no finding.
func TestRP1_CountsAreRecordsWithSeqs(t *testing.T) {
	suppressed, _ := json.Marshal(map[string]any{"id": "s", "state": "suppressed", "pattern": "regression-location", "model_text": "x"})
	lines := [][]byte{
		finding(t, "a", "regression-location", "go", 7, "note"),
		finding(t, "a", "regression-location", "go", 7, "note"), // the same line twice
		finding(t, "f", "claim-vs-record", "", 9, ""),           // a flag: no model text
		finding(t, "k", "continuity-briefing", "", 0, "briefing"),
		suppressed,
		[]byte(`{"id":"broken"`),
	}
	im := ImpactOf(viewOf(verify.RowVerified, wrote), lines, nil)
	if len(im.Delivered) != 2 || im.Delivered[0].ID != "a" || im.Delivered[0].Seq != 7 || im.Delivered[1].ID != "k" {
		t.Errorf("delivered %+v, want the regression note a at 7 and the briefing k", im.Delivered)
	}
	if len(im.Findings) != 2 || im.Findings[0].ID != "a" || im.Findings[1].ID != "f" || im.Findings[1].Seq != 9 {
		t.Errorf("findings %+v, want a and the flag f at 9", im.Findings)
	}
}

// TestRP1g_UnknownIsNotMet: an expected row with no readable state is
// unknown, never met: it counts in the clause's total and in none of its met,
// and it is listed among the unknowns.
func TestRP1g_UnknownIsNotMet(t *testing.T) {
	v := viewOf(verify.RowVerified, wrote, gateAt("go:test", "test", verify.Green, verify.OK, 41, nil))
	v.Expected = []verify.Expected{
		{Kind: "real_data_run", State: verify.ObligationMet, Gate: "go:test"},
		{Kind: "judgment_disclosed", State: verify.ObligationUnknown},
	}
	im := ImpactOf(v, nil, nil)
	if met, unknown := im.Met(); met != 1 || unknown != 1 || len(im.Checks) != 2 || im.Checks[0].Seq != 41 {
		t.Fatalf("met %d, unknown %d, checks %+v, want 1 met at 41 of 2 with 1 unknown", met, unknown, im.Checks)
	}
	if len(im.Unknown) != 1 || im.Unknown[0].Kind != UnknownCheck {
		t.Errorf("unknown %+v, want the one check", im.Unknown)
	}
	if got, want := clause(im), "AutoSteer: checks 1 of 2 met, 1 unknown."; got != want {
		t.Errorf("clause %q, want %q", got, want)
	}
	v.Expected[0].State = verify.ObligationUnknown
	if met, _ := ImpactOf(v, nil, nil).Met(); met != 0 {
		t.Errorf("met %d with no met row, want 0", met)
	}
}

// TestRP1_ClauseFitsAndDropsDetailNotCounts: the full clause is the card's
// example, a clause over eighty columns loses its titles and seqs and keeps
// every count, and an impact with nothing to count has no clause.
func TestRP1_ClauseFitsAndDropsDetailNotCounts(t *testing.T) {
	note := func(pat string) NoteRef { return NoteRef{Pattern: pat, Event: "UserPromptSubmit"} }
	im := Impact{
		Delivered: []NoteRef{note("subtask-gate"), note("subtask-gate"), note("declared-fact")},
		Findings:  []FindingRef{{State: FindingResolved, ResolvedBy: 41}},
		Checks:    []CheckRef{{State: verify.ObligationMet}, {State: verify.ObligationMet}},
	}
	if got, want := ClauseBody(im), "3 notes given with your prompt; 1 finding, resolved at #41; checks 2 of 2 met"; got != want {
		t.Errorf("body %q, want %q", got, want)
	}
	if got, want := clause(im), "AutoSteer: 3 notes; 1 finding, resolved at #41; checks 2 of 2 met."; got != want {
		t.Errorf("clause %q, want %q", got, want)
	}
	if got := clause(Impact{}); got != "" {
		t.Errorf("an empty impact has clause %q", got)
	}
	if a, b := im.Fingerprint(), (Impact{Delivered: im.Delivered}).Fingerprint(); a == b {
		t.Errorf("fingerprint %q ignores findings and checks", a)
	}
}
