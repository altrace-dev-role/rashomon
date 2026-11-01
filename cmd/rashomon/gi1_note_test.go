package main

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestGI1n_TheStopOwesTheNoteForADeclaredOrLearnedCommandLeftUnrun: a Stop
// whose inferred row is absent or stale after a relevant write of this turn
// owes the next prompt a note (card GATE-INFER1, W5), at the Stop point's
// seq, under the runner's name and the file, when the project declares the
// command or the index has seen a test run. Controls: a convention alone, a
// green or red row, writes the gate ignores, a turn that wrote nothing, no
// inferred gate and no runner's name owe nothing. Break: drop any condition.
func TestGI1n_TheStopOwesTheNoteForADeclaredOrLearnedCommandLeftUnrun(t *testing.T) {
	declared := &verify.InferredGate{Argv: []string{"pytest", "-q"}, From: ".github/workflows/ci.yml", Declared: true}
	learned := &verify.InferredGate{Argv: []string{"go", "test", "./..."}, From: "go.mod", Learned: true}
	convention := &verify.InferredGate{Argv: []string{"go", "test", "./..."}, From: "go.mod"}
	view := func(state verify.State, since, turnWrites int, program string) verify.StopView {
		v := verify.StopView{Programs: map[string]string{verify.InferredGateID: program}}
		v.Snap.Point.Seq = 12
		v.Snap.Gates = []verify.GateState{{Gate: "learned:go-build", State: verify.Absent, WritesSinceGreen: 1},
			{Gate: verify.InferredGateID, State: state, WritesSinceGreen: since}}
		v.Turn.Writes = turnWrites
		return v
	}
	for _, c := range []struct {
		name string
		v    verify.StopView
		inf  *verify.InferredGate
		want recap.Owed
	}{
		{"declared, absent", view(verify.Absent, 2, 2, "pytest"), declared, recap.Owed{Seq: 12, Gate: "pytest", From: ".github/workflows/ci.yml"}},
		{"declared, stale", view(verify.Stale, 1, 1, "pytest"), declared, recap.Owed{Seq: 12, Stale: true, Gate: "pytest", From: ".github/workflows/ci.yml"}},
		{"learned convention", view(verify.Absent, 1, 1, "go test"), learned, recap.Owed{Seq: 12, Gate: "go test", From: "go.mod"}},
		{"control: a convention alone", view(verify.Absent, 1, 1, "go test"), convention, recap.Owed{}},
		{"control: green", view(verify.Green, 0, 1, "pytest"), declared, recap.Owed{}},
		{"control: red after the edit", view(verify.Red, 1, 1, "pytest"), declared, recap.Owed{}},
		{"control: only writes the gate ignores", view(verify.Absent, 0, 1, "pytest"), declared, recap.Owed{}},
		{"control: this turn wrote nothing", view(verify.Absent, 1, 0, "pytest"), declared, recap.Owed{}},
		{"control: no inferred gate", view(verify.Absent, 1, 1, "pytest"), nil, recap.Owed{}},
		{"control: no runner's name", view(verify.Absent, 1, 1, ""), declared, recap.Owed{}},
	} {
		if got := owedNote(c.v, c.inf); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestGI1n_TheNextPromptGetsItOncePerCheckpoint: the note is a
// claim-vs-record finding at nudge, absent or stale, keyed by the Stop
// point's seq, its text a statement naming the runner and the file; a steer
// line that gave that key, delivered or withheld by arm off, ends it, and
// one for an earlier checkpoint or a superseded one does not. A file the
// path slot refuses leaves the runner's name. Controls: no seq, a seq past
// the decision's and no runner's name give none. Break: key it by gate
// alone, or count a superseded line as given.
func TestGI1n_TheNextPromptGetsItOncePerCheckpoint(t *testing.T) {
	o := recap.Owed{Seq: 12, Gate: "pytest", From: ".github/workflows/ci.yml"}
	const want = "rashomon record: gate 'pytest' (from '.github/workflows/ci.yml') was not run after the last edit."
	line := func(key, state, by string) []byte {
		return []byte(`{"pattern":"claim-vs-record","state":"` + state + `","suppressed_by":"` + by + `","key":"` + key + `"}`)
	}
	f, ok := owedFinding(o, 14, nil)
	if !ok {
		t.Fatal("no finding for an owed note")
	}
	if f.Pattern != pattern.ClaimVsRecord || f.Kind != "absent" || f.Key != "inferred:test@12" || f.MaxLevel != steer.LevelNudge ||
		f.AfterSeq != 14 || f.Message.Text() != want {
		t.Errorf("finding %+v text %q, want claim-vs-record/absent at nudge keyed inferred:test@12, %q", f, f.Message.Text(), want)
	}
	if err := f.CheckOn(steer.EventUserPromptSubmit); err != nil {
		t.Errorf("CheckOn: %v", err)
	}
	stale := o
	stale.Stale = true
	if f, ok := owedFinding(stale, 14, nil); !ok || f.Kind != "stale" {
		t.Errorf("stale: kind %q, ok %v, want stale", f.Kind, ok)
	}
	for _, c := range []struct {
		name  string
		lines [][]byte
		want  bool
	}{
		{"given", [][]byte{line("inferred:test@12", "delivered", "")}, false},
		{"withheld by arm off", [][]byte{line("inferred:test@12", "suppressed", "arm_off")}, false},
		{"an earlier checkpoint's", [][]byte{line("inferred:test@9", "delivered", "")}, true},
		{"superseded", [][]byte{line("inferred:test@12", "superseded", "budget_batch")}, true},
		{"another pattern's", [][]byte{[]byte(`{"pattern":"task-preflight","state":"delivered","key":"inferred:test@12"}`)}, true},
	} {
		if _, ok := owedFinding(o, 14, c.lines); ok != c.want {
			t.Errorf("%s: ok %v, want %v", c.name, ok, c.want)
		}
	}
	refused := o
	refused.From = "/etc/ci.yml"
	if f, ok := owedFinding(refused, 14, nil); !ok || f.Message.Text() != "rashomon record: gate 'pytest' was not run after the last edit." {
		t.Errorf("a refused file: %q, ok %v, want the runner's name alone", f.Message.Text(), ok)
	}
	for name, c := range map[string]recap.Owed{"no seq": {Gate: "pytest", From: "Makefile"}, "no runner's name": {Seq: 12, From: "Makefile"}} {
		if _, ok := owedFinding(c, 14, nil); ok {
			t.Errorf("control, %s: a finding, want none", name)
		}
	}
	if _, ok := owedFinding(o, 11, nil); ok {
		t.Error("control, a seq past the decision's: a finding, want none")
	}
}
