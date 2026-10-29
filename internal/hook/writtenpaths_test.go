package hook

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// capturedAt is captured with the store's record scope set to scope first
// ("" for none on file): the declaration one PreToolUse payload lands.
func capturedAt(t *testing.T, scope, tool string, input map[string]any) *store.Declaration {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if scope != "" {
		if err := store.WriteRecordScopeMarker(st.Root(), scope); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := json.Marshal(map[string]any{
		"session_id": "sess-wt11", "tool_use_id": "toolu_wt11", "transcript_path": "/tmp/t.jsonl",
		"tool_name": tool, "cwd": "/tmp/project", "hook_event_name": "PreToolUse", "tool_input": input,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, time.Now)
	h.Close(&Signals{}, h.Capture(strings.NewReader(string(payload))))
	run, err := st.ReadRunDir(st.DirName(h.sessionID))
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 1 {
		t.Fatalf("%s: %d declarations, want 1", tool, len(run.Declarations))
	}
	return &run.Declarations[0]
}

// TestWT11_HookRecordsWrittenPaths (D118): the hook records what
// shape.WrittenPaths names for a Bash call, and null and complete for a file
// tool, which names its path in file_path; a line WrittenPaths reads
// incomplete records the targets it found and false. Below record scope v5
// the list is content the user did not agree to record, withheld as
// no_consent and incomplete. Break: leave the fill out, so every declaration
// records null for both.
func TestWT11_HookRecordsWrittenPaths(t *testing.T) {
	for _, c := range []struct {
		scope, tool string
		input       map[string]any
		paths       []string
		complete    bool
		withheld    string
	}{
		{store.RecordScopeV5, "Bash", map[string]any{"command": "touch a.go"}, []string{"a.go"}, true, ""},
		{store.RecordScopeV5, "Bash", map[string]any{"command": "touch a.go && make"}, []string{"a.go"}, false, ""},
		{store.RecordScopeV5, "Bash", map[string]any{"command": "ls -la"}, nil, true, ""},
		{store.RecordScopeV5, "Edit", map[string]any{"file_path": "/tmp/project/a.go", "old_string": "x", "new_string": "y"}, nil, true, ""},
		{"", "Bash", map[string]any{"command": "touch a.go"}, nil, false, "no_consent"},
	} {
		d := capturedAt(t, c.scope, c.tool, c.input)
		withheld := ""
		if d.WrittenPathsWithheld != nil {
			withheld = *d.WrittenPathsWithheld
		}
		if !slices.Equal(d.WrittenPaths, c.paths) || (d.WrittenPaths == nil) != (c.paths == nil) ||
			d.WrittenPathsComplete == nil || *d.WrittenPathsComplete != c.complete || withheld != c.withheld {
			complete := "null"
			if d.WrittenPathsComplete != nil {
				complete = map[bool]string{true: "true", false: "false"}[*d.WrittenPathsComplete]
			}
			t.Errorf("%s %v at scope %q: written_paths %q complete %s withheld %q; want %q complete %t withheld %q",
				c.tool, c.input, c.scope, d.WrittenPaths, complete, withheld, c.paths, c.complete, c.withheld)
		}
	}
}
