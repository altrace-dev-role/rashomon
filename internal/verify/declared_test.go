package verify

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// dgKey is the install key the declared-gate tests digest lines under.
var dgKey = []byte("declared-gates-test-key-0123456789")

// recorded sets the Bash call t<seq> of r to have run line: its shape as
// shape derives it under dgKey on the declaration, and the runner fields on
// the execution, so a declared gate's digests can match it. The call fixture
// writes literal runner names ("r:go-test"), which no digest equals.
func recorded(t *testing.T, r *store.Run, seq int64, line string) {
	t.Helper()
	input, err := json.Marshal(map[string]string{"command": line})
	if err != nil {
		t.Fatal(err)
	}
	s := shape.Derive("Bash", input, dgKey)
	id, n := callID(seq), 0
	for i := range r.Declarations {
		if d := &r.Declarations[i]; d.ToolUseID == id && d.ToolName == "Bash" {
			d.Shape = s
			n++
		}
	}
	for i := range r.Executions {
		if e := &r.Executions[i]; e.ToolUseID == id {
			e.StatusMasked, e.RunnerDigest, e.RunnerKind = s.StatusMasked, s.RunnerDigest, s.RunnerKind
			n++
		}
	}
	if n != 2 {
		t.Fatalf("recorded: %d records for Bash call %s, want its declaration and its execution", n, id)
	}
}

// ranIn sets the declarations of r to have run in dir of project root:
// Project root, and the CWDDigest of root/dir as hook.Locate keys it.
func ranIn(r *store.Run, root, dir string) {
	loc := shape.CWDDigest(dgKey, filepath.Join(root, dir))
	for i := range r.Declarations {
		r.Declarations[i].Project = &root
		if r.Declarations[i].ToolName == "Bash" {
			r.Declarations[i].CWDDigest = loc
		}
	}
}

// dgSpec is a compiled spec whose one source has trust tr, declaring gates.
func dgSpec(tr spec.Trust, gates ...spec.Declared) *spec.Compiled {
	return &spec.Compiled{Sources: []spec.Source{{Path: "/repo/.claude/rashomon/spec.json", Digest: "sha256:0", Trust: tr}},
		Trust: tr, Gates: gates}
}

// dgGate is the declared gate spec:<id> of kind test, required, matching
// argv by prefix.
func dgGate(id string, argv ...string) spec.Declared {
	yes := true
	return spec.Declared{ID: "spec:" + id, Gate: spec.Gate{ID: id, Kind: "test", Argv: argv, Match: "prefix", Required: &yes}}
}

func dgPoint(seq int64) Point { return Point{Kind: PointStop, Seq: seq, SessionID: "s1"} }

// dgLedger is the Stop view's snapshot and ledger at pt over sp, its lines
// digested under key, with no index (card LEDGER-WIRE1: the ledger's one
// producer is EvaluateStop's).
func dgLedger(r *store.Run, sp *spec.Compiled, key []byte, pt Point) (Snapshot, []Obligation) {
	v := EvaluateStop(r, nil, sp, pt, time.Time{}, StopInput{Key: key})
	return v.Snap, v.Obligations
}

// gateNamed is the row of snap whose Gate is id, and whether there is one.
func gateNamed(gates []GateState, id string) (GateState, bool) {
	for _, g := range gates {
		if g.Gate == id {
			return g, true
		}
	}
	return GateState{}, false
}

// TestDG1_DeclaredGateReadsItsRuns: a trusted spec's gate "unit", argv
// [go test ./...] by prefix, and one green `go test ./...` after an edit: the
// snapshot holds spec:unit Green, Source declared, Family declared, Trust
// trusted, Required, its last green the run; the learned family the run made
// is absorbed, so there is no second row; and the ledger owes
// gate_after_write for it with source spec, met, under an id of its own.
func TestDG1_DeclaredGateReadsItsRuns(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	recorded(t, r, 2, "go test ./...")
	ranIn(r, "/repo", "")
	snap, obs := dgLedger(r, dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./...")), dgKey, dgPoint(10))
	if len(snap.Gates) != 1 {
		t.Fatalf("rows %+v, want spec:unit alone: the learned family it absorbed has no row", snap.Gates)
	}
	g := snap.Gates[0]
	if g.Gate != "spec:unit" || g.State != Green || g.Reason != OK || g.Source != "declared" || g.Family != "declared" ||
		g.Trust != "trusted" || !g.Required || g.Kind != "test" || seqOr0(g.LastGreen) != declOf(r, 2).Seq {
		t.Fatalf("row %+v, want spec:unit green from the run, declared, trusted, required, kind test", g)
	}
	want := ObligationID(ScopeSession, GateAfterWrite, "spec:unit")
	found := false
	for _, o := range obs {
		if o.Kind == GateAfterWrite {
			found = true
			if o.ID != want || o.Source != "spec" || o.State != ObligationMet {
				t.Errorf("gate_after_write %+v, want id %s, source spec, met", o, want)
			}
		}
	}
	if !found {
		t.Errorf("obligations %+v hold no gate_after_write row", obs)
	}
}

// TestDG1b_UntrustedSpecDeclaresNoGate: the same spec untrusted declares
// nothing (INV-2; the consent screen's sentence): no spec: row, and the
// learned row stands.
func TestDG1b_UntrustedSpecDeclaresNoGate(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	recorded(t, r, 2, "go test ./...")
	snap, _ := dgLedger(r, dgSpec(spec.Untrusted, dgGate("unit", "go", "test", "./...")), dgKey, dgPoint(10))
	if len(snap.Gates) != 1 || snap.Gates[0].Source != sourceLearned || snap.Gates[0].State != Green {
		t.Fatalf("rows %+v, want the learned row alone, green", snap.Gates)
	}
}

// TestDG1c_ExactMatchNeedsTheWholeLine: by prefix a gate matches a run of its
// runner's words whatever surrounds them on the line, and nothing with other
// words; by exact only the whole line matches. A run the gate does not match
// leaves its family's learned row.
func TestDG1c_ExactMatchNeedsTheWholeLine(t *testing.T) {
	exact := dgGate("unit", "go", "test", "./...")
	exact.Gate.Match = "exact"
	for _, c := range []struct {
		name    string
		gate    spec.Declared
		line    string
		state   State
		learned bool // the run's learned row stands
	}{
		{"prefix, the runner on a longer line", dgGate("unit", "go", "test", "./..."), "go test ./... && echo ok", Green, false},
		{"prefix, other runner words", dgGate("unit", "go", "test", "./..."), "go test ./... -run TestX", Absent, true},
		{"exact, the whole line", exact, "go test ./...", Green, false},
		{"exact, a longer line", exact, "go test ./... && echo ok", Absent, true},
	} {
		r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
		recorded(t, r, 2, c.line)
		ranIn(r, "/repo", "")
		snap, _ := dgLedger(r, dgSpec(spec.Trusted, c.gate), dgKey, dgPoint(10))
		g, ok := gateNamed(snap.Gates, "spec:unit")
		run := declOf(r, 2)
		_, learned := gateNamed(snap.Gates, learnedGateID("test", familyKey{runner: *run.Shape.RunnerDigest, location: run.CWDDigest}))
		if !ok || g.State != c.state || learned != c.learned {
			t.Errorf("%s: spec:unit %+v (present %v), learned row %v; want %s, learned %v", c.name, g, ok, learned, c.state, c.learned)
		}
	}
}

// TestDG1c2_ARewrittenLineMatchesNoGate (D211): a line a hook rewrote ran
// at no location the record can vouch for, so lineRuns clears its location
// and no gate matches it, not even an exact gate whose line it ran as; the
// same line, not rewritten, reads Green. The clearing is what makes the
// digest's execution arm unobservable (B218).
func TestDG1c2_ARewrittenLineMatchesNoGate(t *testing.T) {
	exact := dgGate("unit", "go", "test", "./...")
	exact.Gate.Match = "exact"
	declared, err := json.Marshal(map[string]string{"command": "go test ./pkg/..."})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name      string
		rewritten bool
		state     State
	}{
		{"the gate's line as declared", false, Green},
		{"another line a hook rewrote to the gate's", true, Absent},
	} {
		r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
		recorded(t, r, 2, "go test ./...")
		if c.rewritten {
			ran := declOf(r, 2).Shape.Digest
			declOf(r, 2).Shape = shape.Derive("Bash", declared, dgKey)
			execOf(r, 2).ExecutedDigest = ran
		}
		ranIn(r, "/repo", "")
		snap, _ := dgLedger(r, dgSpec(spec.Trusted, exact), dgKey, dgPoint(10))
		if g, ok := gateNamed(snap.Gates, "spec:unit"); !ok || g.State != c.state {
			t.Errorf("%s: spec:unit %+v (present %v), want %s", c.name, g, ok, c.state)
		}
	}
}

// TestDG1d_DirBindsTheLocation: a gate with dir matches a run only in that
// directory of the project root the record names; a run at the root does not
// match, and with no root known the gate matches nothing.
func TestDG1d_DirBindsTheLocation(t *testing.T) {
	gate := dgGate("web", "go", "test", "./...")
	gate.Gate.Dir = "web"
	for _, c := range []struct {
		name      string
		root, dir string
		state     State
	}{
		{"in the directory", "/repo", "web", Green},
		{"at the root", "/repo", "", Absent},
		{"no root known", "", "web", Absent},
	} {
		r := runOf(t, edit(1, "web/a.go"), gateCall(2, "go-test", store.ExecOK))
		recorded(t, r, 2, "go test ./...")
		ranIn(r, c.root, c.dir)
		if c.root == "" {
			for i := range r.Declarations {
				r.Declarations[i].Project = nil
			}
			r.Declarations[1].CWDDigest = shape.CWDDigest(dgKey, "/repo/web")
		}
		snap, _ := dgLedger(r, dgSpec(spec.Trusted, gate), dgKey, dgPoint(10))
		if g, ok := gateNamed(snap.Gates, "spec:web"); !ok || g.State != c.state {
			t.Errorf("%s: spec:web %+v (present %v), want %s", c.name, g, ok, c.state)
		}
	}
}

// TestDG1d2_EmptyDirBindsTheProjectRoot: a gate with no dir is the project
// root's (D158), never any location's: a run at the root matches; a run in a
// subdirectory, or in another checkout under the same project, does not, and
// its learned family keeps its row; with no root known nothing matches.
func TestDG1d2_EmptyDirBindsTheProjectRoot(t *testing.T) {
	for _, c := range []struct {
		name      string
		root, dir string
		at        string // the run's directory where it is not root/dir
		state     State
		learned   bool
	}{
		{"at the root", "/repo", "", "", Green, false},
		{"in a subdirectory", "/repo", "web", "", Absent, true},
		{"in another checkout", "/repo", "", "/other", Absent, true},
		{"no root known", "", "", "/repo", Absent, true},
	} {
		r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
		recorded(t, r, 2, "go test ./...")
		ranIn(r, c.root, c.dir)
		if c.root == "" {
			for i := range r.Declarations {
				r.Declarations[i].Project = nil
			}
		}
		if c.at != "" {
			declOf(r, 2).CWDDigest = shape.CWDDigest(dgKey, c.at)
		}
		snap, _ := dgLedger(r, dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./...")), dgKey, dgPoint(10))
		g, ok := gateNamed(snap.Gates, "spec:unit")
		run := declOf(r, 2)
		_, learned := gateNamed(snap.Gates, learnedGateID("test", familyKey{runner: *run.Shape.RunnerDigest, location: run.CWDDigest}))
		if !ok || g.State != c.state || learned != c.learned {
			t.Errorf("%s: spec:unit %+v (present %v), learned row %v; want %s, learned %v", c.name, g, ok, learned, c.state, c.learned)
		}
	}
}

// TestDG1e_AbsentDeclaredGateReadsNeverRun: a declared gate no run matched is
// Absent, required: never_run_after_write with the writes it owes counted
// and the last one cited, or empty_run with none. A gate its spec marks not
// required reads advisory: its Required is the spec's.
func TestDG1e_AbsentDeclaredGateReadsNeverRun(t *testing.T) {
	sp := dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./..."))
	snap, _ := dgLedger(runOf(t, edit(1, "pkg/a.go")), sp, dgKey, dgPoint(10))
	g, ok := gateNamed(snap.Gates, "spec:unit")
	if !ok || g.State != Absent || g.Reason != NeverRunAfterWrite || !g.Required || g.LastRun != nil || g.LastGreen != nil ||
		g.WritesSinceGreen != 1 || g.LastWrite == nil || str(g.LastWrite.Path) != "pkg/a.go" || g.WritersSinceGreen == nil {
		t.Fatalf("after an edit: %+v (present %v), want absent never_run_after_write, required, one write pkg/a.go", g, ok)
	}
	snap, _ = dgLedger(runOf(t, gateCall(1, "go-vet", store.ExecOK)), sp, dgKey, dgPoint(10))
	g, ok = gateNamed(snap.Gates, "spec:unit")
	if !ok || g.State != Absent || g.Reason != EmptyRun || g.WritesSinceGreen != 0 || g.LastWrite != nil {
		t.Fatalf("with no write: %+v (present %v), want absent empty_run, no write", g, ok)
	}
	no := false
	advisory := dgGate("unit", "go", "test", "./...")
	advisory.Gate.Required = &no
	snap, _ = dgLedger(runOf(t, edit(1, "pkg/a.go")), dgSpec(spec.Trusted, advisory), dgKey, dgPoint(10))
	if g, ok = gateNamed(snap.Gates, "spec:unit"); !ok || g.Required {
		t.Fatalf("required false: %+v (present %v), want the row advisory", g, ok)
	}
}

// TestDG1f_ExpectedRowReadsTheDeclaredGate: the task names real_data_run, the
// declared gate supplies it, and its run is green after the edit: at Stop the
// row is met, read from spec:unit. Before the adapter the snapshot held no
// spec: row, and the row read unmet, "no green run of spec:unit since the
// last edit".
func TestDG1f_ExpectedRowReadsTheDeclaredGate(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	recorded(t, r, 2, "go test ./...")
	ranIn(r, "/repo", "")
	gate := dgGate("unit", "go", "test", "./...")
	gate.Gate.Supplies = []string{"real_data_run"}
	sp := dgSpec(spec.Trusted, gate)
	sp.Task = &spec.Task{ID: "t1", Goal: "g", Decision: "d", ExpectedEvidence: []string{"real_data_run"}}
	v := EvaluateStop(r, nil, sp, dgPoint(10), time.Time{}, StopInput{Key: dgKey})
	want := Expected{Kind: "real_data_run", State: ObligationMet, Gate: "spec:unit"}
	if len(v.Expected) != 1 || v.Expected[0] != want {
		t.Fatalf("expected rows %+v, want %+v", v.Expected, want)
	}
}

// TestDG1g_CIGateRowCarriesItsSource: a ci_commands entry's row is ci:<n>,
// Source ci_commands, kind custom; its argv here names no runner shape
// knows, so it matches its whole line. A managed spec's entry carries trust
// managed.
func TestDG1g_CIGateRowCarriesItsSource(t *testing.T) {
	yes := true
	for _, c := range []struct {
		id    string
		trust spec.Trust
	}{{"ci:0", spec.Trusted}, {"ci:managed:0", spec.Managed}} {
		lint := call{seq: 2, tool: "Bash", prog: "golangci-lint", class: shape.VerbExecute, outcome: store.ExecOK, loc: "d:repo"}
		r := runOf(t, edit(1, "pkg/a.go"), lint)
		recorded(t, r, 2, "golangci-lint run")
		ranIn(r, "/repo", "")
		ci := spec.Declared{ID: c.id, Gate: spec.Gate{Kind: "custom", Argv: []string{"golangci-lint", "run"}, Match: "prefix", Required: &yes}}
		snap, _ := dgLedger(r, dgSpec(c.trust, ci), dgKey, dgPoint(10))
		g, ok := gateNamed(snap.Gates, c.id)
		if !ok || g.Source != "ci_commands" || g.Kind != "custom" || g.Trust != string(c.trust) || g.State != Green {
			t.Errorf("%s: %+v (present %v), want source ci_commands, kind custom, trust %s, green", c.id, g, ok, c.trust)
		}
	}
}

// TestDG1h_EvaluateWithoutKeyDeclaresNothing: with no install key no
// declared line can be digested as the hook digested the run, so Evaluate,
// whose signature is frozen, and a nil key elsewhere declare no gate; the
// learned row stands.
func TestDG1h_EvaluateWithoutKeyDeclaresNothing(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	recorded(t, r, 2, "go test ./...")
	sp := dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./..."))
	for name, snap := range map[string]Snapshot{
		"Evaluate":              Evaluate(r, nil, sp, dgPoint(10), time.Time{}),
		"EvaluateStop, nil key": EvaluateStop(r, nil, sp, dgPoint(10), time.Time{}, StopInput{}).Snap,
	} {
		if len(snap.Gates) != 1 || snap.Gates[0].Source != sourceLearned {
			t.Errorf("%s: rows %+v, want the learned row alone", name, snap.Gates)
		}
	}
}
