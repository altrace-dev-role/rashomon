package claudecode

import (
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

func contentSteps(t *testing.T, transcript string) []trace.Step {
	t.Helper()
	var out []trace.Step
	if _, err := ReadContent(strings.NewReader(transcript), func(s trace.Step) { out = append(out, s) }); err != nil {
		t.Fatal(err)
	}
	return out
}

func lastCall(ss []trace.Step) *trace.CallStep {
	for i := len(ss) - 1; i >= 0; i-- {
		if ss[i].Call != nil {
			return ss[i].Call
		}
	}
	return nil
}

// TestReadContent_Calls: each tool's command, path and changes are read from
// its input, and Read leaves them empty.
func TestReadContent_Calls(t *testing.T) {
	use := func(name, input string) string {
		return `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c","name":"` + name + `","input":` + input + `}]}}` + "\n"
	}
	for _, c := range []struct {
		name, input string
		want        trace.CallStep
	}{
		{"Bash", `{"command":"npm test"}`, trace.CallStep{Command: "npm test"}},
		{"Read", `{"file_path":"/w/a.py"}`, trace.CallStep{Paths: []string{"/w/a.py"}}},
		{"Grep", `{"pattern":"x","path":"/w/src"}`, trace.CallStep{Paths: []string{"/w/src"}}},
		{"Edit", `{"file_path":"/w/a.py","old_string":"a","new_string":"b"}`,
			trace.CallStep{Paths: []string{"/w/a.py"}, Changes: []trace.Change{{Path: "/w/a.py", Old: "a", New: "b"}}}},
		{"MultiEdit", `{"file_path":"/w/a.ts","edits":[{"old_string":"a","new_string":"b"},{"old_string":"c","new_string":"d"}]}`,
			trace.CallStep{Paths: []string{"/w/a.ts"}, Changes: []trace.Change{{Path: "/w/a.ts", Old: "a", New: "b"}, {Path: "/w/a.ts", Old: "c", New: "d"}}}},
		{"Write", `{"file_path":"/w/main.tf","content":"x"}`,
			trace.CallStep{Paths: []string{"/w/main.tf"}, Changes: []trace.Change{{Path: "/w/main.tf", New: "x"}}}},
		{"NotebookEdit", `{"notebook_path":"/w/n.ipynb","new_source":"y"}`,
			trace.CallStep{Paths: []string{"/w/n.ipynb"}, Changes: []trace.Change{{Path: "/w/n.ipynb", New: "y"}}}},
	} {
		got := lastCall(contentSteps(t, use(c.name, c.input)))
		if got.Command != c.want.Command || !reflect.DeepEqual(got.Paths, c.want.Paths) || !reflect.DeepEqual(got.Changes, c.want.Changes) {
			t.Errorf("%s: command %q paths %q changes %+v, want %+v", c.name, got.Command, got.Paths, got.Changes, c.want)
		}
		if lean := lastCall(steps(t, use(c.name, c.input))); lean.Command != "" || lean.Paths != nil || lean.Changes != nil {
			t.Errorf("%s: Read filled content: %+v", c.name, lean)
		}
	}
}

// TestReadContent_Outputs: every result carries its output, a failed one's
// as well as a passing one's; Read keeps a failed result's text only.
func TestReadContent_Outputs(t *testing.T) {
	const rec = `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"a","is_error":false,"content":"3 passed"},{"type":"tool_result","tool_use_id":"b","is_error":true,"content":[{"type":"text","text":"Exit code 1\n1 failed"}]}]}}` + "\n"
	got := contentSteps(t, rec)
	if got[1].Result.Output != "3 passed" || got[2].Result.Output != "Exit code 1\n1 failed" {
		t.Errorf("outputs %q, %q", got[1].Result.Output, got[2].Result.Output)
	}
	lean := steps(t, rec)
	if lean[1].Result.Output != "" || lean[2].Result.Output != "" || lean[2].Result.Text == "" {
		t.Errorf("Read: outputs %q %q text %q", lean[1].Result.Output, lean[2].Result.Output, lean[2].Result.Text)
	}
}
