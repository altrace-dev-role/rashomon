package recap

import (
	"encoding/json"
	"strings"
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
	line := [][]byte{keyed(t, "a", "regression-location", "go", "go:test", 37)}
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

// keyed is finding with the line's dedupe key, the gate id a gate pattern's
// line carries (verify.keyFor).
func keyed(t *testing.T, id, pat, gate, key string, seq int64) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(finding(t, id, pat, gate, seq, "x"), &m); err != nil {
		t.Fatal(err)
	}
	m["key"] = key
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// goView is a view whose two gates both run go: learned:go-test and
// learned:go-build, as the programs map names them.
func goView(test, build verify.GateState) verify.StopView {
	v := viewOf(verify.RowVerified, wrote, test, build)
	v.Programs = map[string]string{"learned:go-test": "go", "learned:go-build": "go"}
	return v
}

// TestRP1i_AGreenBuildDoesNotResolveATestFinding: a regression on go test is
// still open while go test is red, though go build, which shares its
// program, went green after it (the review's probe); the line's key names
// the gate, and the slot alone, shared by two gates, settles nothing.
func TestRP1i_AGreenBuildDoesNotResolveATestFinding(t *testing.T) {
	v := goView(gateAt("learned:go-test", "test", verify.Red, verify.FailedExit, 10, exitCode(1)),
		gateAt("learned:go-build", "build", verify.Green, verify.OK, 20, nil))
	for name, line := range map[string][]byte{
		"keyed":   keyed(t, "a", "regression-location", "go", "learned:go-test", 10),
		"unkeyed": finding(t, "a", "regression-location", "go", 10, "x"),
	} {
		im := ImpactOf(v, [][]byte{line}, nil)
		if len(im.Findings) != 1 || im.Findings[0].State != FindingOpen || im.Findings[0].ResolvedBy != 0 || len(im.Resolved()) != 0 {
			t.Errorf("%s: findings %+v, want one open", name, im.Findings)
		}
		if got := clause(im); strings.Contains(got, "resolved at") {
			t.Errorf("%s: clause %q claims a resolution", name, got)
		}
	}
}

// TestRP1i_TheSameGateGoingGreenResolves: the gate the line's key names has a
// green after the finding, so it is resolved at that green's seq, not at
// the other gate's earlier green.
func TestRP1i_TheSameGateGoingGreenResolves(t *testing.T) {
	test := gateAt("learned:go-test", "test", verify.Green, verify.OK, 30, nil)
	v := goView(test, gateAt("learned:go-build", "build", verify.Green, verify.OK, 20, nil))
	im := ImpactOf(v, [][]byte{keyed(t, "a", "regression-location", "go", "learned:go-test", 10)}, nil)
	if len(im.Findings) != 1 || im.Findings[0].State != FindingResolved || im.Findings[0].ResolvedBy != 30 {
		t.Errorf("findings %+v, want resolved by 30", im.Findings)
	}
}

// TestRP1i_ASlotTwoGatesShareReadsOpen: a line with no gate id whose slot two
// gates share cannot say which it means, so it reads open though one of the
// two is green after it; with one gate on the slot it still resolves.
func TestRP1i_ASlotTwoGatesShareReadsOpen(t *testing.T) {
	line := [][]byte{finding(t, "a", "stale-evidence", "go", 10, "x")}
	v := goView(gateAt("learned:go-test", "test", verify.Green, verify.OK, 30, nil),
		gateAt("learned:go-build", "build", verify.Green, verify.OK, 20, nil))
	if im := ImpactOf(v, line, nil); im.Findings[0].State != FindingOpen {
		t.Errorf("two gates share the slot: %+v, want open", im.Findings)
	}
	one := viewOf(verify.RowVerified, wrote, gateAt("go:test", "test", verify.Green, verify.OK, 30, nil))
	if im := ImpactOf(one, line, nil); im.Findings[0].State != FindingResolved || im.Findings[0].ResolvedBy != 30 {
		t.Errorf("one gate on the slot: %+v, want resolved by 30", im.Findings)
	}
}

// runnerView is goView with the programs as the Stop view names them once a
// gate carries its runner's whole name: "go test" and "go build".
func runnerView(test, build verify.GateState) verify.StopView {
	v := goView(test, build)
	v.Programs = map[string]string{"learned:go-test": "go test", "learned:go-build": "go build"}
	return v
}

// TestRP1i_ARunnerNameIsComparedByItsProgramWord: a line with no gate id whose
// slot is the program word ("go") finds the gate whose program is the runner's
// whole name ("go test"), so a keyless finding resolves, and with go test and
// go build both present the first word is shared and the finding reads open.
func TestRP1i_ARunnerNameIsComparedByItsProgramWord(t *testing.T) {
	line := [][]byte{finding(t, "a", "regression-location", "go", 10, "x")}
	test := gateAt("learned:go-test", "test", verify.Green, verify.OK, 30, nil)
	one := viewOf(verify.RowVerified, wrote, test)
	one.Programs = map[string]string{"learned:go-test": "go test"}
	if im := ImpactOf(one, line, nil); im.Findings[0].State != FindingResolved || im.Findings[0].ResolvedBy != 30 {
		t.Errorf("one gate named go test: %+v, want resolved by 30", im.Findings)
	}
	both := runnerView(test, gateAt("learned:go-build", "build", verify.Green, verify.OK, 20, nil))
	if im := ImpactOf(both, line, nil); im.Findings[0].State != FindingOpen || im.Findings[0].ResolvedBy != 0 {
		t.Errorf("go test and go build share the word go: %+v, want open", im.Findings)
	}
	// The line's own gate id still settles it with both present.
	if im := ImpactOf(both, [][]byte{keyed(t, "a", "regression-location", "go", "learned:go-test", 10)}, nil); im.Findings[0].State != FindingResolved {
		t.Errorf("keyed line: %+v, want resolved", im.Findings)
	}
}

// TestRP1j_ARunnerWordAnywhereInItsNameSettlesAKeylessFinding: a keyless line
// whose slot is the runner's name word ("pytest", made by gateout.ProgramFor)
// finds the gate whose program is the whole line ("python3 -m pytest"), where
// the first word is "python3". The one-match rule stands: go test and go build
// still share "go" and read open. Break: compare the slot to the first word
// only, and the pytest finding reads open.
func TestRP1j_ARunnerWordAnywhereInItsNameSettlesAKeylessFinding(t *testing.T) {
	line := [][]byte{finding(t, "a", "regression-location", "pytest", 10, "x")}
	py := gateAt("learned:pytest", "test", verify.Green, verify.OK, 30, nil)
	one := viewOf(verify.RowVerified, wrote, py)
	one.Programs = map[string]string{"learned:pytest": "python3 -m pytest"}
	if im := ImpactOf(one, line, nil); im.Findings[0].State != FindingResolved || im.Findings[0].ResolvedBy != 30 {
		t.Errorf("pytest under python3 -m pytest: %+v, want resolved by 30", im.Findings)
	}
	goLine := [][]byte{finding(t, "a", "regression-location", "go", 10, "x")}
	both := runnerView(gateAt("learned:go-test", "test", verify.Green, verify.OK, 30, nil), gateAt("learned:go-build", "build", verify.Green, verify.OK, 20, nil))
	if im := ImpactOf(both, goLine, nil); im.Findings[0].State != FindingOpen {
		t.Errorf("go test and go build both hold the word go: %+v, want open", im.Findings)
	}
}
