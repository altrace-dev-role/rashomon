package hook

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// backgroundedCall reads two presence bits off tool_response's spans, and the
// value is never a Go string: no copy of a task id, however long, is made.
// Break: decode the response to read the id, and the allocation grows with it.
func TestBackgroundedCall_ReadsTwoBitsAndBuildsNoString(t *testing.T) {
	for _, tc := range []struct {
		response string
		want     bool
	}{
		{`{"backgroundTaskId":"bash_1"}`, true},
		{`{"backgroundTaskId": "x" }`, true},
		{`{"backgroundTaskId":{"id":1}}`, true},
		{`{"backgroundedByUser":true}`, true},
		{`{"backgroundTaskId":0}`, true},
		{`{"backgroundTaskId":null}`, false},
		{`{"backgroundTaskId":false,"backgroundedByUser":null}`, false},
		{`{"stdout":"backgroundTaskId"}`, false},
		{`"backgroundTaskId"`, false},
		{`null`, false},
	} {
		in, err := hookio.Intake(strings.NewReader(`{"tool_name":"Bash","tool_response":` + tc.response + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if got := backgroundedCall(in); got != tc.want {
			t.Errorf("%s: backgrounded %v, want %v", tc.response, got, tc.want)
		}
	}

	allocs := func(idBytes int) float64 {
		in, err := hookio.Intake(strings.NewReader(`{"tool_response":{"backgroundTaskId":"` + strings.Repeat("a", idBytes) + `"}}`))
		if err != nil {
			t.Fatal(err)
		}
		return testing.AllocsPerRun(20, func() {
			if !backgroundedCall(in) {
				t.Fatal("not backgrounded")
			}
		})
	}
	if small, big := allocs(8), allocs(64<<10); big != small {
		t.Errorf("reading a 64 KiB task id allocated %v times, an 8-byte one %v: the value became a string", big, small)
	}
}

// TestPG1c_TheExecutionCarriesTheRunnerKind: the execution records the
// runner kind of the line as it ran, beside its runner digest and from the
// same Derive, so the two are set together (D9). Break: drop the copy, and
// the execution of `go test ./... && echo ok` no longer says a test runner
// ran.
func TestPG1c_TheExecutionCarriesTheRunnerKind(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct{ cmd, kind string }{
		{"go test ./... && echo ok", shape.RunnerTest},
		{"make release; ls dist", shape.RunnerBuild},
		{"ls -la", ""},
	} {
		t.Run(tc.cmd, func(t *testing.T) {
			st := mustOpen(t)
			payload, err := json.Marshal(map[string]any{
				"session_id": "s", "tool_use_id": "toolu_x", "tool_name": "Bash", "hook_event_name": "PostToolUse",
				"tool_input": map[string]string{"command": tc.cmd},
			})
			if err != nil {
				t.Fatal(err)
			}
			p := NewPost(st, time.Now)
			p.Close(&Signals{}, p.Capture(strings.NewReader(string(payload))))
			run, err := st.ReadRun("s")
			if err != nil {
				t.Fatal(err)
			}
			if len(run.Executions) != 1 {
				t.Fatalf("read %d executions, want 1", len(run.Executions))
			}
			x := run.Executions[0]
			got := ""
			if x.RunnerKind != nil {
				got = *x.RunnerKind
			}
			if got != tc.kind {
				t.Errorf("runner_kind %q, want %q", got, tc.kind)
			}
			if (x.RunnerDigest == nil) != (tc.kind == "") {
				t.Errorf("runner_digest set %t, want %t", x.RunnerDigest != nil, tc.kind != "")
			}
		})
	}
}
