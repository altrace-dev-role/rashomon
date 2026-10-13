package hook

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// capPayload is a PreToolUse payload with one field set to v, the rest
// ordinary.
func capPayload(t *testing.T, field string, v any) string {
	t.Helper()
	p := map[string]any{
		"session_id": "sess-caps", "tool_use_id": "toolu_caps", "prompt_id": "p1",
		"agent_id": "a1", "agent_type": "general", "transcript_path": "/tmp/t.jsonl",
		"permission_mode": "default", "tool_name": "Bash", "cwd": "/tmp/project",
		"hook_event_name": "PreToolUse", "tool_input": map[string]any{"command": "ls"},
	}
	p[field] = v
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// hostCommand names n distinct hosts under scheme.
func hostCommand(scheme string, n int) map[string]any {
	urls := make([]string, n)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s://h%d.example.com/x", scheme, i)
	}
	return map[string]any{"command": "curl " + strings.Join(urls, " ")}
}

// captured runs one PreToolUse invocation through Capture and Close against a
// fresh store, and returns its declaration (nil when none landed) and the
// reason on its coverage record.
func captured(t *testing.T, payload string) (*store.Declaration, string, string) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := New(st, time.Now)
	h.Close(&Signals{}, h.Capture(strings.NewReader(payload)))
	run, err := st.ReadRunDir(st.DirName(h.sessionID))
	if err != nil {
		t.Fatal(err)
	}
	reason := ""
	if len(run.Coverage) == 1 && run.Coverage[0].Reason != nil {
		reason = *run.Coverage[0].Reason
	}
	if len(run.Declarations) != 1 {
		return nil, reason, h.sessionID
	}
	// The terminal closes the declaration under the id the declaration
	// carries, capped or not, or the call reads as never closed.
	if len(run.Terminals) != 1 || run.Terminals[0].ToolUseID != run.Declarations[0].ToolUseID {
		t.Errorf("the terminal does not close the declaration: %+v", run.Terminals)
	}
	return &run.Declarations[0], reason, h.sessionID
}

// TestREC52_FieldCapsAtNAndNPlusOne: every field a payload hands the record
// verbatim has a cap, so that no payload inside the intake limit can make a
// record that is over MaxRecordBytes (REC-31's derivation reads the same
// constants). At its cap a value is stored; one byte or one entry over, it is
// withheld -- emptied, or null where the field is nullable -- and the call's
// coverage record carries record_oversize, which is how a v4 record says a
// field was cut, so the session never reads as complete. A session id over
// its cap buckets the call under unattributed instead.
func TestREC52_FieldCapsAtNAndNPlusOne(t *testing.T) {
	id := func(n int) string { return strings.Repeat("i", n) }
	path := func(n int) string { return "/" + strings.Repeat("p", n-1) }
	for _, row := range []struct {
		field  string
		at     func(int) any
		cap    int
		stored func(*store.Declaration) bool
	}{
		{"tool_use_id", func(n int) any { return id(n) }, store.MaxIDBytes, func(d *store.Declaration) bool { return d.ToolUseID != "" }},
		{"prompt_id", func(n int) any { return id(n) }, store.MaxIDBytes, func(d *store.Declaration) bool { return d.PromptID != nil }},
		{"agent_id", func(n int) any { return id(n) }, store.MaxIDBytes, func(d *store.Declaration) bool { return d.AgentID != nil }},
		{"agent_type", func(n int) any { return id(n) }, store.MaxIDBytes, func(d *store.Declaration) bool { return d.AgentType != nil }},
		{"permission_mode", func(n int) any { return id(n) }, store.MaxIDBytes, func(d *store.Declaration) bool { return d.PermissionMode != "" }},
		{"tool_name", func(n int) any { return id(n) }, store.MaxIDBytes, func(d *store.Declaration) bool { return d.ToolName != "" }},
		{"transcript_path", func(n int) any { return path(n) }, store.MaxPathBytes, func(d *store.Declaration) bool { return d.TranscriptPath != "" }},
		{"tool_input", func(n int) any { return map[string]any{"command": id(n) + " arg"} }, store.MaxProgramBytes,
			func(d *store.Declaration) bool { return d.Shape.Program != nil }},
		{"tool_input", func(n int) any { return hostCommand("https", n) }, store.MaxHosts, func(d *store.Declaration) bool { return d.Hosts != nil }},
		{"tool_input", func(n int) any { return hostCommand("ssh", n) }, store.MaxHosts, func(d *store.Declaration) bool { return d.SSHHosts != nil }},
	} {
		for _, n := range []int{row.cap, row.cap + 1} {
			decl, reason, _ := captured(t, capPayload(t, row.field, row.at(n)))
			if decl == nil {
				t.Fatalf("%s at %d: no declaration landed", row.field, n)
			}
			over := n > row.cap
			if got := row.stored(decl); got == over {
				t.Errorf("%s at %d (cap %d): stored %v, want %v", row.field, n, row.cap, got, !over)
			}
			if got := reason == store.ReasonRecordOversize; got != over {
				t.Errorf("%s at %d (cap %d): coverage reason %q", row.field, n, row.cap, reason)
			}
		}
	}

	t.Run("session_id", func(t *testing.T) {
		for _, n := range []int{store.MaxIDBytes, store.MaxIDBytes + 1} {
			decl, reason, session := captured(t, capPayload(t, "session_id", id(n)))
			want := id(n)
			if n > store.MaxIDBytes {
				want = UnattributedSession
			}
			if decl == nil || decl.SessionID != want || session != want {
				t.Errorf("session_id at %d: recorded under %q, want %q", n, session, want)
			}
			if got := reason == store.ReasonRecordOversize; got != (n > store.MaxIDBytes) {
				t.Errorf("session_id at %d: coverage reason %q", n, reason)
			}
		}
	})

	t.Run("cwd", func(t *testing.T) {
		for _, n := range []int{store.MaxPathBytes, store.MaxPathBytes + 1} {
			cov := BuildCoverage(mustOpen(t), "s", store.PhaseStart, "", path(n), time.Now())
			if got := cov.CWD != ""; got == (n > store.MaxPathBytes) {
				t.Errorf("cwd at %d: stored %v", n, got)
			}
			got := cov.Reason != nil && *cov.Reason == store.ReasonRecordOversize
			if got != (n > store.MaxPathBytes) {
				t.Errorf("cwd at %d: coverage reason %v", n, cov.Reason)
			}
		}
	})

	t.Run("execution", func(t *testing.T) {
		for _, n := range []int{store.MaxIDBytes, store.MaxIDBytes + 1} {
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			st := mustOpen(t)
			p := NewPost(st, time.Now)
			payload := fmt.Sprintf(`{"session_id":"s","tool_use_id":%q,"tool_name":%q,"hook_event_name":"PostToolUse"}`, id(n), id(n))
			p.Close(&Signals{}, p.Capture(strings.NewReader(payload)))
			run, err := st.ReadRun("s")
			if err != nil || len(run.Executions) != 1 || len(run.Coverage) != 1 {
				t.Fatalf("execution at %d: %v, %+v", n, err, run)
			}
			x, over := run.Executions[0], n > store.MaxIDBytes
			if (x.ToolUseID == "") != over || (x.ToolName == "") != over {
				t.Errorf("execution at %d: tool_use_id %d bytes, tool_name %d bytes", n, len(x.ToolUseID), len(x.ToolName))
			}
			r := run.Coverage[0].Reason
			if (r != nil && *r == store.ReasonRecordOversize) != over {
				t.Errorf("execution at %d: coverage reason %v", n, r)
			}
		}
	})
}

func mustOpen(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestREC31b_ARefusedRecordIsRecordOversize: a record the store refuses for
// its size leaves record_oversize on the call's coverage, not internal_error:
// nothing broke, the call went unrecorded for a reason that can be named.
func TestREC31b_ARefusedRecordIsRecordOversize(t *testing.T) {
	if got := closeReason(fmt.Errorf("append: %w", store.ErrRecordOversize)); got != store.ReasonRecordOversize {
		t.Errorf("closeReason(ErrRecordOversize) = %q, want %q", got, store.ReasonRecordOversize)
	}
}
