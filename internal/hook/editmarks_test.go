package hook

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// postedEdit runs one PostToolUse (or its failure event) for tool over input
// against st, and returns the execution it wrote.
func postedEdit(t *testing.T, st *store.Store, event, tool string, input any) store.Execution {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id": "s-em", "tool_use_id": "toolu_em", "tool_name": tool, "hook_event_name": event,
		"tool_input": input,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := NewPost(st, time.Now)
	p.Close(&Signals{}, p.Capture(strings.NewReader(string(payload))))
	run, err := st.ReadRun("s-em")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Executions) != 1 {
		t.Fatalf("read %d executions, want 1", len(run.Executions))
	}
	return run.Executions[0]
}

func skipEdit() map[string]string {
	return map[string]string{"file_path": "pkg/calc_test.go", "old_string": "\tbody()\n", "new_string": "\tt.Skip(\"later\")\n\tbody()\n"}
}

func setSwitch(t *testing.T, st *store.Store, v string) {
	t.Helper()
	if _, err := store.UpdateConfig(st.Root(), func(c *store.Config) { c.EditMarks = v }); err != nil {
		t.Fatal(err)
	}
}

// TestEM_PostWritesTheEditMarksOfAnEdit: the execution of an Edit that skips a
// test carries the marks of the producer, on the edit_delta basis, and a
// MultiEdit carries its edits' sum.
func TestEM_PostWritesTheEditMarksOfAnEdit(t *testing.T) {
	x := postedEdit(t, mustOpen(t), "PostToolUse", "Edit", skipEdit())
	if x.EditMarks == nil || !maps.Equal(x.EditMarks.Added, map[string]int{"go.t_skip": 1}) || len(x.EditMarks.Removed) != 0 ||
		x.EditMarks.Basis != "edit_delta" || x.EditMarks.Version != 1 || x.EditMarksWithheld != nil {
		t.Errorf("Edit: marks %+v, withheld %v; want go.t_skip added once on edit_delta, version 1", x.EditMarks, x.EditMarksWithheld)
	}
	x = postedEdit(t, mustOpen(t), "PostToolUse", "MultiEdit", map[string]any{"file_path": "pkg/calc_test.go", "edits": []map[string]string{
		{"old_string": "\tt.Errorf(\"a\")\n", "new_string": ""},
		{"old_string": "func TestA(t *testing.T) {\n", "new_string": ""},
	}})
	if x.EditMarks == nil || !maps.Equal(x.EditMarks.Removed, map[string]int{"go.assert": 1, "go.test_def": 1}) {
		t.Errorf("MultiEdit: marks %+v, want go.assert and go.test_def removed once", x.EditMarks)
	}
}

// TestEM_PostWritesNoMarksWhereThereAreNone: a failed edit lands nothing and
// so has no marks (1.8p computes marks at PostToolUse only), and nor does an
// edit that moves no id, another tool, or a path the table does not read.
func TestEM_PostWritesNoMarksWhereThereAreNone(t *testing.T) {
	for name, c := range map[string]struct {
		event, tool string
		input       any
	}{
		"a failure event":   {FailureEvent, "Edit", skipEdit()},
		"an edit with none": {"PostToolUse", "Edit", map[string]string{"file_path": "pkg/calc_test.go", "old_string": "a\n", "new_string": "b\n"}},
		"a source file":     {"PostToolUse", "Edit", map[string]string{"file_path": "pkg/calc.go", "old_string": "a\n", "new_string": "\tt.Skip()\n"}},
		"a Write":           {"PostToolUse", "Write", map[string]string{"file_path": "pkg/calc_test.go", "content": "\tt.Skip()\n"}},
		"a Bash call":       {"PostToolUse", "Bash", map[string]string{"command": "echo t.Skip()"}},
	} {
		x := postedEdit(t, mustOpen(t), c.event, c.tool, c.input)
		if x.EditMarks != nil || x.EditMarksWithheld != nil {
			t.Errorf("%s: marks %+v, withheld %v; want neither", name, x.EditMarks, x.EditMarksWithheld)
		}
	}
}

// TestEM_TheSwitchTurnsTheMarksOff: with config.json's edit_marks at off, or
// at any value but on, the execution carries no marks and says switched_off;
// a config that does not read fails closed the same way.
func TestEM_TheSwitchTurnsTheMarksOff(t *testing.T) {
	for _, v := range []string{"off", "Off", "disabled"} {
		st := mustOpen(t)
		setSwitch(t, st, v)
		x := postedEdit(t, st, "PostToolUse", "Edit", skipEdit())
		if x.EditMarks != nil || x.EditMarksWithheld == nil || *x.EditMarksWithheld != store.WithheldSwitchedOff {
			t.Errorf("edit_marks %q: marks %+v, withheld %v; want none and switched_off", v, x.EditMarks, x.EditMarksWithheld)
		}
	}
	st := mustOpen(t)
	setSwitch(t, st, "on")
	if x := postedEdit(t, st, "PostToolUse", "Edit", skipEdit()); x.EditMarks == nil {
		t.Error("edit_marks on: no marks")
	}
	st = mustOpen(t)
	if err := os.WriteFile(filepath.Join(st.Root(), store.FileConfig), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	x := postedEdit(t, st, "PostToolUse", "Edit", skipEdit())
	if x.EditMarks != nil || x.EditMarksWithheld == nil || *x.EditMarksWithheld != store.WithheldSwitchedOff {
		t.Errorf("an unreadable config: marks %+v, withheld %v; want none and switched_off", x.EditMarks, x.EditMarksWithheld)
	}
}

// TestEM_NoByteOfTheEditReachesTheStore: end to end, the old and new text of
// an edit that moves a mark, with a canary in each, is in no file the store
// holds: the mark is ids and counts.
func TestEM_NoByteOfTheEditReachesTheStore(t *testing.T) {
	st := mustOpen(t)
	const canaryOld, canaryNew = "CANARY-OLD-5f3a91", "CANARY-NEW-c07d42"
	x := postedEdit(t, st, "PostToolUse", "Edit", map[string]string{"file_path": "pkg/calc_test.go",
		"old_string": "\tt.Errorf(\"" + canaryOld + "\")\n\tleft := \"" + canaryOld + "\"\n",
		"new_string": "\tt.Skip(\"" + canaryNew + "\")\n\tright := \"" + canaryNew + "\"\n"})
	if x.EditMarks == nil {
		t.Fatal("no marks for an edit that skips a test and drops an assertion")
	}
	found := 0
	err := filepath.WalkDir(st.Root(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		found++
		for _, c := range []string{"CANARY", canaryOld, canaryNew} {
			if strings.Contains(string(b), c) {
				t.Errorf("%s holds %q from the edit", p, c)
			}
		}
		return nil
	})
	if err != nil || found == 0 {
		t.Fatalf("walked %d store files: %v", found, err)
	}
}
