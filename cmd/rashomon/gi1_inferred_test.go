package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestGI1c_TheStopInfersWhileNoSpecDeclaresATestGate: the Stop's inferred
// gate (card GATE-INFER1, W3') is built from the project's files while no
// spec declares a test gate, at any trust; the index's test family marks it
// learned and never ends it; the replay of a Stop builds it only when that
// Stop's checkpoint named it. Break: build it beside a spec test gate, stop
// at the index's first test family, or replay one the checkpoint never had.
func TestGI1c_TheStopInfersWhileNoSpecDeclaresATestGate(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module m\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	yes := true
	withGate := func(kind string, tr spec.Trust) *spec.Compiled {
		return &spec.Compiled{Sources: []spec.Source{{Path: filepath.Join(root, ".claude", "rashomon", "spec.json"), Trust: tr}}, Trust: tr,
			Gates: []spec.Declared{{ID: "spec:g", Gate: spec.Gate{ID: "g", Kind: kind, Argv: []string{"make"}, Match: "prefix", Required: &yes}}}}
	}
	tested := &verify.ProjectIndex{ProjectKey: root, Entries: []verify.IndexEntry{{Family: "r@l", Kind: "test", Sessions: 1}}}
	g := inferredGate(root, nil, nil)
	if g == nil || !slices.Equal(g.Argv, []string{"go", "test", "./..."}) || g.Learned {
		t.Fatalf("no spec, no index: %+v, want go test ./..., not learned", g)
	}
	if g = inferredGate(root, withGate("build", spec.Trusted), tested); g == nil || !g.Learned {
		t.Errorf("a build gate and an index with a test family: %+v, want built and learned", g)
	}
	if g = inferredGate(root, withGate("test", spec.Untrusted), nil); g != nil {
		t.Errorf("an untrusted spec test gate: %+v, want none (a spec wins at any trust)", g)
	}
	if g = inferredGate(t.TempDir(), nil, nil); g != nil {
		t.Errorf("nothing to infer: %+v, want none", g)
	}
	if g = replayInferred(root, nil, nil, map[string]bool{"r:go@l": true}); g != nil {
		t.Errorf("replay of a checkpoint that named no inferred gate: %+v, want none", g)
	}
	if g = replayInferred(root, nil, nil, map[string]bool{verify.InferredGateID: true}); g == nil {
		t.Error("replay of a checkpoint that named the inferred gate: none, want it")
	}
	if g = replayInferred(root, nil, nil, nil); g == nil {
		t.Error("replay with no checkpoint: none, want the Stop's rule")
	}
}

// TestGI1l_TheReplayJudgesTheInferredGateTheStopJudged: a session in a
// go.mod project edits and runs no test; the Stop judges the inferred gate
// absent and required, and the replay of that Stop (the ledger's and the
// report's) judges it too, as the checkpoint names it. Break: leave the
// replay unwired, and the report reads a turn the Stop never saw.
func TestGI1l_TheReplayJudgesTheInferredGateTheStopJudged(t *testing.T) {
	f := newLW(t)
	f.file("go.mod", "module m\n")
	f.land("t1", "Edit", lwEdit(f.file("a.go", "a")), nil)
	v := f.stop()
	if g, ok := gi1Row(v.Snap.Gates); !ok || g.State != verify.Absent || !g.Required {
		t.Fatalf("Stop rows %+v, want %s absent and required", v.Snap.Gates, verify.InferredGateID)
	}
	r, err := replayLastStop(f.st, "s")
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := gi1Row(r.View.Snap.Gates); !ok || g.State != verify.Absent || !g.Required {
		t.Errorf("replay rows %+v, want %s absent and required, as the Stop judged it", r.View.Snap.Gates, verify.InferredGateID)
	}
}

// gi1Row is the inferred gate's row among gates.
func gi1Row(gates []verify.GateState) (verify.GateState, bool) {
	for _, g := range gates {
		if g.Gate == verify.InferredGateID {
			return g, true
		}
	}
	return verify.GateState{}, false
}
