package shape

import (
	"encoding/json"
	"testing"
)

// Codex's own tools are classed by name: apply_patch writes, and the spawn
// and wait of a subagent are agent calls. None of them is a shell, so none
// is tokenized, and apply_patch gets no file label -- what its input looks
// like is unmeasured, and a label read from a guessed field would be a
// guess. Break: drop a name from verbForTool and its call reads unknown.
func TestCodexToolClasses(t *testing.T) {
	key := []byte("k")
	for _, tc := range []struct {
		tool  string
		input string
		want  string
	}{
		{"apply_patch", `{"input":"*** Begin Patch\n*** Update File: .env\n*** End Patch"}`, VerbWrite},
		{"collaborationspawn_agent", `{"task_name":"list_files","fork_turns":"all","message":"gAAAA"}`, VerbAgent},
		{"collaborationwait_agent", `{"timeout_ms":30000}`, VerbAgent},
	} {
		s := Derive(tc.tool, json.RawMessage(tc.input), key)
		if s.VerbClass != tc.want || s.Program != nil || s.Argc != nil {
			t.Errorf("%s: class %q, program %v, argc %v; want %q and no command line", tc.tool, s.VerbClass, s.Program, s.Argc, tc.want)
		}
		if l := Label(tc.tool, json.RawMessage(tc.input)); l != "" {
			t.Errorf("%s: label %q; want none until a capture shows the input", tc.tool, l)
		}
	}
}
