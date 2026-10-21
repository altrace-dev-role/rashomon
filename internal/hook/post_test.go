package hook

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// present decodes a key of tool_response to one bit, and the value is never
// a Go string: no copy of a task id, however long, is made in this process.
// Break: trim a string(b) copy, as the first version did, and the decode
// allocates the value.
func TestPresent_DecodesABitAndBuildsNoString(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`"bash_1"`, true},
		{` "x" `, true},
		{`{"id":1}`, true},
		{`true`, true},
		{`0`, true},
		{`null`, false},
		{` null `, false},
		{`false`, false},
		{"\tfalse\n", false},
	} {
		var p present
		if err := p.UnmarshalJSON([]byte(tc.raw)); err != nil || bool(p) != tc.want {
			t.Errorf("%q: present %v, %v; want %v", tc.raw, p, err, tc.want)
		}
	}

	id := append(append([]byte(`"`), bytes.Repeat([]byte("a"), 4096)...), '"')
	var p present
	if n := testing.AllocsPerRun(20, func() { _ = p.UnmarshalJSON(id) }); n != 0 {
		t.Errorf("decoding a task id allocated %v times, want 0: the value became a string", n)
	}
}

// A Codex execution is recorded with no outcome, whatever its payload says:
// a plain PostToolUse for a call that failed, and a failure event carrying
// Claude Code's exit-code shape, both land as outcome unobserved with no exit
// code. Without --harness the same failure event reads as failed, exit 1.
// Break: drop the Codex branch in Post.Capture and the first two read ok and
// failed.
func TestPost_CodexOutcomeIsUnobserved(t *testing.T) {
	for _, tc := range []struct {
		name    string
		harness Harness
		payload string
		outcome string
		exit    *int
	}{
		{"codex ok-shaped", Codex,
			`{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"t","tool_input":{"command":"python3 missing.py"},"tool_response":"can't open file\n"}`,
			"", nil},
		{"codex failure-shaped", Codex,
			`{"session_id":"s","hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_use_id":"t","tool_input":{"command":"x"},"error":"Exit code 1","is_interrupt":false}`,
			"", nil},
		{"claude code failure", ClaudeCode,
			`{"session_id":"s","hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_use_id":"t","tool_input":{"command":"x"},"error":"Exit code 1","is_interrupt":false}`,
			store.ExecFailed, intPtr(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			p := NewPost(st, time.Now)
			p.Harness = tc.harness
			if err := p.Capture(strings.NewReader(tc.payload)); err != nil {
				t.Fatal(err)
			}
			run, err := st.ReadRun("s")
			if err != nil || len(run.Executions) != 1 {
				t.Fatalf("run %+v, %v; want one execution", run, err)
			}
			x := run.Executions[0]
			if x.Outcome != tc.outcome || !reflect.DeepEqual(x.ExitCode, tc.exit) || (tc.harness == Codex && x.IsInterrupt != nil) {
				t.Errorf("outcome %q, exit %v, interrupt %v; want %q, %v", x.Outcome, x.ExitCode, x.IsInterrupt, tc.outcome, tc.exit)
			}
			if x.ExecutedDigest == "" {
				t.Error("no executed digest: the record dropped what it does know")
			}
		})
	}
}

func intPtr(n int) *int { return &n }
