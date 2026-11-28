package verify

import (
	"bytes"
	"encoding/json"
	stdflag "flag" // the package's own flag is a ceiling (tosteer_card.go)
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// updateReqNil rewrites reqNilGolden from the code under test. It is run at
// main 9a6e359c only, before any requirement row exists; a head that adds
// requirement rows must match that file, never rewrite it.
var updateReqNil = stdflag.Bool("update-req-nil", false, "rewrite "+reqNilGolden)

// reqNilGolden is the ledger of each Stop of reqNilStops as main 9a6e359c
// wrote it, with Reqs nil.
const reqNilGolden = "testdata/req_nil_ledger.json"

// reqNilStops is a table of Stops built from these tests' own runs, each with
// Reqs nil: a verified turn, a red one, an edit with no gate, a Bash write, a
// declared gate green and never run, a strict done claim over a verified
// turn, and a task's expected evidence over a declared gate green with a
// strict done claim and never run. Every row kind the ledger writes for a
// turn is in one of them, and the Expected rows (no task, no rows:
// evidence.go expected) are in the two task Stops.
func reqNilStops(t *testing.T) map[string]StopView {
	t.Helper()
	declared := func(sp *spec.Compiled, in StopInput, calls ...call) StopView {
		r := runOf(t, calls...)
		for _, c := range calls {
			if c.tool == "Bash" && c.runner != "" {
				recorded(t, r, c.seq, "go test ./...")
			}
		}
		ranIn(r, "/repo", "")
		in.Key = dgKey
		return EvaluateStop(r, nil, sp, dgPoint(10), time.Time{}, in)
	}
	plain := func() *spec.Compiled { return dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./...")) }
	// tasked's gate supplies real_data_run; its task expects that kind, one
	// no gate supplies, and both kinds read from the message and the task.
	tasked := func() *spec.Compiled {
		g := dgGate("unit", "go", "test", "./...")
		g.Gate.Supplies = []string{"real_data_run"}
		sp := dgSpec(spec.Trusted, g)
		sp.Task = &spec.Task{ID: "t1", Goal: "g", Decision: "d", Mechanism: "profile",
			ExpectedEvidence: []string{"real_data_run", "cross_check", "judgment_disclosed", "mechanism_stated"}}
		return sp
	}
	claim := StopInput{Head: []byte("All 42 tests pass.")}
	return map[string]StopView{
		"verified":           stopOf(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), nil, "sess-1", StopInput{}),
		"red":                stopOf(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed)), nil, "sess-1", StopInput{}),
		"edit, no gate":      stopOf(runOf(t, edit(1, "pkg/a.go")), nil, "sess-1", StopInput{}),
		"bash write":         stopOf(runOf(t, bashWrite(1, "pkg/b.go")), nil, "sess-1", StopInput{}),
		"declared green":     declared(plain(), StopInput{}, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)),
		"declared never run": declared(plain(), StopInput{}, edit(1, "pkg/a.go")),
		"done claim":         stopOf(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), nil, "sess-1", StopInput{Key: dgKey, Head: []byte("All 42 tests pass.")}),
		"task, green, claim": declared(tasked(), claim, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)),
		"task, never run":    declared(tasked(), StopInput{}, edit(1, "pkg/a.go")),
	}
}

// TestReq_ReqsNilIsByteEqual: a Stop with Reqs nil writes the ledger it
// wrote before W-M3.4. Over each Stop of reqNilStops, the JSON
// of its Expected and Obligations equals reqNilGolden byte for byte, a file
// cut at main 9a6e359c; and no row marshals a req key.
func TestReq_ReqsNilIsByteEqual(t *testing.T) {
	type ledger struct {
		Expected    []Expected   `json:"expected"`
		Obligations []Obligation `json:"obligations"`
	}
	got := map[string]ledger{}
	for name, v := range reqNilStops(t) {
		got[name] = ledger{v.Expected, v.Obligations}
	}
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if *updateReqNil {
		if err := os.MkdirAll(filepath.Dir(reqNilGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(reqNilGolden, b, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s, %d bytes", reqNilGolden, len(b))
		return
	}
	want, err := os.ReadFile(reqNilGolden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, want) {
		i := 0
		for i < len(b) && i < len(want) && b[i] == want[i] {
			i++
		}
		lo := max(0, i-80)
		t.Errorf("the ledger with Reqs nil differs from %s at byte %d:\ngot  ...%s\nwant ...%s", reqNilGolden, i, b[lo:min(len(b), i+80)], want[lo:min(len(want), i+80)])
	}
	if bytes.Contains(b, []byte(`"req"`)) {
		t.Errorf("a row with Reqs nil marshals a req key")
	}
}
