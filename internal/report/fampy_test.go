package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestFamPy1_APytestRunExercisesPython: a session whose only commands ran
// pytest has exercised Python's tooling, so the report must count those calls
// under the python family and must not print python as not exercised. Each
// declaration is derived from the command line as the hook derives it, so the
// program name is the one a real `pytest -q` call records.
func TestFamPy1_APytestRunExercisesPython(t *testing.T) {
	key := []byte("fam-py1")
	run := &store.Run{}
	for i, line := range []string{"pytest -q", "pytest -q tests/test_basic.py"} {
		raw, err := json.Marshal(map[string]string{"command": line, "description": "Run the tests"})
		if err != nil {
			t.Fatal(err)
		}
		run.Declarations = append(run.Declarations, store.Declaration{
			ToolUseID: fmt.Sprintf("t-pytest-%d", i),
			ToolName:  "Bash",
			SessionID: "sess-1",
			Shape:     shape.Derive("Bash", raw, key),
		})
	}
	if p := run.Declarations[0].Shape.Program; p == nil || *p != "pytest" {
		t.Fatalf("the shape of `pytest -q` names program %v, want pytest: the test no longer reaches the table", p)
	}

	fc := buildFamilies(run, nil)

	f, ok := familyByName(fc, "python")
	if !ok {
		t.Fatalf("no python family after two pytest calls; not_exercised = %v", fc.NotExercised)
	}
	if f.Calls != 2 || len(f.Programs) != 1 || f.Programs[0] != "pytest" {
		t.Errorf("python family = %+v, want 2 calls by program pytest", f)
	}
	var b bytes.Buffer
	writeFamilies(&b, fc)
	text := b.String()
	if !strings.Contains(text, "    python (pytest): 2 call(s), 0 declared host(s)\n") {
		t.Errorf("the report does not count the pytest calls under python:\n%s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "not exercised") && strings.Contains(line, "python") {
			t.Errorf("the report says python was not exercised after two pytest calls: %q", line)
		}
	}
}
