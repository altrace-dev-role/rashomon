package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// pyprojectPytest is the gate QA-2's FHV inferred on ea6f100c (10:41 PST):
// pytest, declared by pyproject.toml's [tool.pytest] table.
var pyprojectPytest = &InferredGate{Argv: []string{"pytest"}, From: "pyproject.toml", Declared: true}

// fhvPytest is the line the FHV agent ran at calls 1, 11 and 18, word for
// word: the interpreter's -m form, on three test files.
const fhvPytest = "python3 -m pytest -q -p no:cacheprovider tests/test_itsdangerous/test_signer.py " +
	"tests/test_itsdangerous/test_serializer.py tests/test_itsdangerous/test_encoding.py"

// TestIF1v_ThePythonModuleFormRunsTheInferredPytest (card INFER-FALSE1,
// B302): the FHV's turns cut to their calls. Turn 1 runs the line and edits
// nothing; turn 3 edits encoding.py, runs it red, edits again and runs it
// green. The inferred `pytest` binds every run (the regression note already
// called them gate 'pytest'), green after the last edit, marked as other
// arguments for the three files; the learned row of the same runs is
// absorbed. Today the gate keys "pytest" and the runs "python3 -m pytest",
// so the row is absent, never_run_after_write, and the next prompt is told
// pytest "was not run after the last edit" after it ran.
func TestIF1v_ThePythonModuleFormRunsTheInferredPytest(t *testing.T) {
	r := runOf(t, gateCall(1, "pytest", store.ExecOK))
	named(t, r, 1, fhvPytest)
	ranIn(r, "/repo", "")
	if g, ok := gateNamed(giStop(r, nil, pyprojectPytest).Snap.Gates, InferredGateID); !ok || g.State != Green || g.LastRun == nil || g.LastRun.Seq != declSeq(r, 0) {
		t.Errorf("turn 1: row %+v, want %s green from call 1", g, InferredGateID)
	}

	r = runOf(t, edit(1, "src/itsdangerous/encoding.py"), gateCall(2, "pytest", store.ExecFailed),
		edit(3, "src/itsdangerous/encoding.py"), gateCall(4, "pytest", store.ExecOK))
	named(t, r, 2, fhvPytest)
	named(t, r, 4, fhvPytest)
	ranIn(r, "/repo", "")
	v := giStop(r, nil, pyprojectPytest)
	if len(v.Snap.Gates) != 1 {
		t.Fatalf("turn 3: rows %+v, want the inferred row alone: the learned row of its runs is absorbed", v.Snap.Gates)
	}
	g := v.Snap.Gates[0]
	if g.Gate != InferredGateID || g.State != Green || g.LastRun == nil || g.LastRun.Seq != declSeq(r, 3) || !g.LastRun.OtherArgs {
		t.Errorf("turn 3: row %+v, want %s green from call 4, after the last edit, its words other than `pytest`", g, InferredGateID)
	}
	if v.Summary.Row != RowVerified {
		t.Errorf("turn 3: row %s, want verified", v.Summary.Row)
	}
}

// TestIF1v_EveryFormOfOneToolRunsItsGate: each runner row that runs the same
// tool binds a gate written as another (shape.sameWork), either way round.
// Controls: another tool, or the same launcher's other script, binds nothing.
// Break: drop a pair, or map by the program word alone.
func TestIF1v_EveryFormOfOneToolRunsItsGate(t *testing.T) {
	for _, c := range []struct {
		gate []string
		line string
		want bool
	}{
		{[]string{"pytest"}, "python -m pytest", true},
		{[]string{"pytest"}, "uv run pytest -x", true},
		{[]string{"pytest"}, "poetry run pytest", true},
		{[]string{"python3", "-m", "pytest"}, "pytest tests/a.py", true},
		{[]string{"python3", "-m", "unittest"}, "python -m unittest", true},
		{[]string{"jest"}, "npx jest", true},
		{[]string{"vitest"}, "npx vitest", true},
		{[]string{"bundle", "exec", "rspec"}, "rspec spec/a_spec.rb", true},
		{[]string{"npm", "test"}, "npm t", true},
		{[]string{"npm", "run", "test"}, "npm test", true},
		{[]string{"./mvnw", "test"}, "mvn test", true},
		{[]string{"gradle", "test"}, "./gradlew test", true},
		{[]string{"pytest"}, "tox", false},
		{[]string{"pytest"}, "python3 -m unittest", false},
		{[]string{"jest"}, "npx vitest", false},
		{[]string{"npm", "test"}, "yarn test", false},
		{[]string{"mvn", "test"}, "gradle test", false},
	} {
		r := runOf(t, gateCall(1, "r", store.ExecOK))
		named(t, r, 1, c.line)
		ranIn(r, "/repo", "")
		g, ok := gateNamed(giStop(r, nil, &InferredGate{Argv: c.gate}).Snap.Gates, InferredGateID)
		if got := ok && g.State == Green; got != c.want {
			t.Errorf("gate %q, run %q: bound %v (row %+v), want %v", c.gate, c.line, got, g, c.want)
		}
	}
}
