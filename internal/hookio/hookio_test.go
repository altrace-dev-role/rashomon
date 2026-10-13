package hookio

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// The key order is a contract later PRs index into: they append, never
// reorder. Break: swap two names, and every consumer reads the wrong span.
func TestPayloadKeyOrderIsFrozen(t *testing.T) {
	want := []string{
		"session_id", "prompt_id", "agent_id", "agent_type", "hook_event_name",
		"tool_name", "tool_use_id", "cwd", "permission_mode", "transcript_path",
		"tool_input", "tool_response", "error", "is_interrupt", "duration_ms",
		"cursor_version", "conversation_id", "generation_id", "turn_id", "model",
		"user_email", "mcp_server",
	}
	got := KeyNames()
	if strings.Join(got, ",") != strings.Join(want, ",") || NumKeys != len(want) {
		t.Fatalf("key order\n got %q\nwant %q (NumKeys %d)", got, want, NumKeys)
	}
	idx := []int{SessionID, PromptID, AgentID, AgentType, HookEventName, ToolName, ToolUseID, CWD,
		PermissionMode, TranscriptPath, ToolInput, ToolResponse, Error, IsInterrupt, DurationMS,
		CursorVersion, ConversationID, GenerationID, TurnID, Model, UserEmail, MCPServer}
	for i, k := range idx {
		if k != i {
			t.Fatalf("constant %d is %d", i, k)
		}
	}
}

func TestIntakeFillsSpansAndDecodesFields(t *testing.T) {
	p, err := Intake(strings.NewReader(`{"session_id":"s1","tool_input":{"command":"ls"},"is_interrupt":true,"duration_ms":12,"user_email":"x@y","error":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if s, err := p.Str(SessionID); s != "s1" || err != nil {
		t.Errorf("session_id = %q %v", s, err)
	}
	if string(p.Value(ToolInput)) != `{"command":"ls"}` || !p.Has(UserEmail) || p.Has(Error) {
		t.Errorf("spans: tool_input %q user_email %v error %v", p.Value(ToolInput), p.Has(UserEmail), p.Has(Error))
	}
	if b, err := p.Bool(IsInterrupt); err != nil || b == nil || !*b {
		t.Errorf("is_interrupt = %v %v", b, err)
	}
	if n, err := p.Int(DurationMS); err != nil || n == nil || *n != 12 {
		t.Errorf("duration_ms = %v %v", n, err)
	}
	for _, bad := range []string{`{"session_id":5}`, `{"duration_ms":1.5}`, `{"is_interrupt":"yes"}`} {
		p, err := Intake(strings.NewReader(bad))
		if err != nil {
			t.Fatal(err)
		}
		_, e1 := p.Str(SessionID)
		_, e2 := p.Int(DurationMS)
		_, e3 := p.Bool(IsInterrupt)
		if !errors.Is(e1, ErrType) && !errors.Is(e2, ErrType) && !errors.Is(e3, ErrType) {
			t.Errorf("%s: no type error", bad)
		}
	}
	if _, err := Intake(strings.NewReader(`{"session_id":`)); !errors.Is(err, jsonscan.ErrInvalid) {
		t.Errorf("truncated payload: %v", err)
	}
}

func payloadOf(n int) []byte {
	head := []byte(`{"session_id":"s","tool_name":"Write","tool_input":{"content":"`)
	tail := []byte(`"}}`)
	return append(append(head, bytes.Repeat([]byte("x"), n-len(head)-len(tail))...), tail...)
}

func totalAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

// TestRES4_ReadAllocBound: Read allocates at most 2.2 times the payload from
// 40 KB up and at most 8 KiB at 400 B. io.ReadAll's doubling cost about five
// times. Break: Read through io.ReadAll and every size from 40 KB fails.
func TestRES4_ReadAllocBound(t *testing.T) {
	for _, n := range []int{400, 40_000, 1 << 20, 4 << 20, 8 << 20} {
		p := payloadOf(n)
		var got []byte
		allocs := testing.AllocsPerRun(3, func() {
			var err error
			if got, err = Read(bytes.NewReader(p)); err != nil {
				t.Fatal(err)
			}
		})
		before := totalAlloc()
		out, err := Read(bytes.NewReader(p))
		bytesAlloc := totalAlloc() - before
		if err != nil || !bytes.Equal(out, p) || !bytes.Equal(got, p) {
			t.Fatalf("%d B: read %d bytes, %v", n, len(out), err)
		}
		limit := uint64(float64(n) * 2.2)
		if n < 40_000 {
			limit = 8 << 10
		}
		t.Logf("%d B: %d B allocated (%.2fx), %.0f allocations", n, bytesAlloc, float64(bytesAlloc)/float64(n), allocs)
		if bytesAlloc > limit {
			t.Errorf("%d B payload: Read allocated %d B, over %d", n, bytesAlloc, limit)
		}
	}
}

// TestRES6_LargeFieldScanIsConstantMemory: a 4 MiB stdout read through Lines
// costs one 64 KiB work buffer, not a copy of the field: the pass summary on
// its last line is found with at most 256 KiB of heap growth. Break: unescape
// the whole span in lines.go, and the delta is the field's size.
func TestRES6_LargeFieldScanIsConstantMemory(t *testing.T) {
	var out bytes.Buffer
	out.WriteString(`{"session_id":"s","tool_name":"Bash","tool_response":{"stdout":"`)
	for i := 0; out.Len() < 4<<20; i++ {
		fmt.Fprintf(&out, "=== RUN   TestCase%d\\n--- PASS: TestCase%d (0.00s)\\n", i, i)
	}
	out.WriteString(`PASS\nok  \texample.com/pkg\t0.412s","stderr":""}}`)
	p, err := Intake(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var resp [1]jsonscan.Span
	if err := jsonscan.Field(p.Raw, p.Spans[ToolResponse], jsonscan.NewKeySet("stdout"), resp[:]); err != nil {
		t.Fatal(err)
	}
	if resp[0].Len < 4<<20-1024 {
		t.Fatalf("stdout span is %d bytes", resp[0].Len)
	}
	work := make([]byte, 64<<10)
	kind, lines := "", 0
	before := totalAlloc()
	err = jsonscan.Lines(p.Raw, resp[0], work, func(line []byte, more bool) bool {
		lines++
		kind = "no_summary"
		if !more && bytes.HasPrefix(line, []byte("ok  \t")) {
			kind = "summary_pass"
		}
		return true
	})
	delta := totalAlloc() - before
	if err != nil || kind != "summary_pass" || lines < 1000 {
		t.Fatalf("kind %q after %d lines: %v", kind, lines, err)
	}
	t.Logf("4 MiB stdout, %d lines: %d B allocated", lines, delta)
	if delta > 256<<10 {
		t.Fatalf("scanning a 4 MiB field allocated %d B, over 256 KiB", delta)
	}
}

func BenchmarkRead(b *testing.B) {
	for _, c := range []struct {
		name string
		n    int
	}{{"400B", 400}, {"40KB", 40_000}, {"1MiB", 1 << 20}, {"4MiB", 4 << 20}, {"8MiB", 8 << 20}} {
		p := payloadOf(c.n)
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(c.n))
			for i := 0; i < b.N; i++ {
				out, err := Read(bytes.NewReader(p))
				if err != nil || len(out) != c.n {
					b.Fatalf("read %d of %d: %v", len(out), c.n, err)
				}
			}
		})
	}
}
