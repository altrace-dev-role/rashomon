package hookio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"

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

// opaqueReader hides a reader's concrete type behind io.Reader, at full
// speed (unlike iotest.OneByteReader, which throttles to one byte per call
// and is too slow for this file's multi-megabyte sizes). Read's *bytes.
// Reader fast path type-switches on the concrete type it is handed; this
// defeats that switch without changing anything about how the bytes
// themselves arrive, so a test wrapping one in opaqueReader exercises the
// general growth loop exactly as a real stdin pipe (never a *bytes.Reader)
// does in production.
type opaqueReader struct{ io.Reader }

// TestRES4_ReadAllocBound: Read allocates at most 2.2 times the payload from
// 40 KB up and at most 8 KiB at 400 B. io.ReadAll's doubling cost about five
// times. Break: Read through io.ReadAll and every size from 40 KB fails.
//
// Run twice per size, once through the *bytes.Reader fast path and once
// through opaqueReader so neither wrapping can make the other's allocation
// path a blind spot: TestRead_BytesReaderFastPath above already pins the
// fast path's own boundaries, but this is the test that pins its allocation
// BUDGET, and a bound checked only through one path is not checked at all
// for the other once the two diverge in cost.
func TestRES4_ReadAllocBound(t *testing.T) {
	for _, n := range []int{400, 40_000, 1 << 20, 4 << 20, 8 << 20} {
		p := payloadOf(n)
		for _, wrap := range []struct {
			name string
			mk   func([]byte) io.Reader
		}{
			{"bytes.Reader", func(b []byte) io.Reader { return bytes.NewReader(b) }},
			{"opaque", func(b []byte) io.Reader { return opaqueReader{bytes.NewReader(b)} }},
		} {
			var got []byte
			allocs := testing.AllocsPerRun(3, func() {
				var err error
				if got, err = Read(wrap.mk(p)); err != nil {
					t.Fatal(err)
				}
			})
			before := totalAlloc()
			out, err := Read(wrap.mk(p))
			bytesAlloc := totalAlloc() - before
			if err != nil || !bytes.Equal(out, p) || !bytes.Equal(got, p) {
				t.Fatalf("%s %d B: read %d bytes, %v", wrap.name, n, len(out), err)
			}
			limit := uint64(float64(n) * 2.2)
			if n < 40_000 {
				limit = 8 << 10
			}
			t.Logf("%s %d B: %d B allocated (%.2fx), %.0f allocations", wrap.name, n, bytesAlloc, float64(bytesAlloc)/float64(n), allocs)
			if bytesAlloc > limit {
				t.Errorf("%s %d B payload: Read allocated %d B, over %d", wrap.name, n, bytesAlloc, limit)
			}
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

// TestReadToleratesOneByteReads: Read's growth loop reads through
// io.ReadFull, which retries on a short read, so a reader that never returns
// more than one byte at a time -- the extreme a chunked body or a slow pipe
// can present -- still yields the same payload Read gets from one that
// returns it whole.
func TestReadToleratesOneByteReads(t *testing.T) {
	p := payloadOf(40_000)
	got, err := Read(iotest.OneByteReader(bytes.NewReader(p)))
	if err != nil || !bytes.Equal(got, p) {
		t.Fatalf("got %d bytes, err %v; want the same %d-byte payload", len(got), err, len(p))
	}
}

// TestReadReturnsAMidStreamError: a read error partway through the payload,
// after some of it has already arrived, is returned as-is rather than
// folded into io.EOF or swallowed by the chunk loop.
func TestReadReturnsAMidStreamError(t *testing.T) {
	boom := errors.New("boom")
	_, err := Read(io.MultiReader(strings.NewReader(`{"session_id":`), iotest.ErrReader(boom)))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// TestRead_BytesReaderFastPath: the *bytes.Reader branch in Read is checked
// against the general growth loop at every boundary that matters to it --
// empty, mid-size, exactly MaxPayloadBytes, and one byte over -- rather than
// relying on TestRES4_ReadAllocBound and BenchmarkRead to exercise it only
// incidentally through sizes chosen for a different assertion. Break: have
// the fast path return br's own backing array instead of a copy, and a
// caller mutating one payload's bytes (recap's own byte-identity replay,
// cmd/rashomon's claudeHostGuard) would corrupt the other.
func TestRead_BytesReaderFastPath(t *testing.T) {
	for _, n := range []int{0, 1, 4096, 40_000, MaxPayloadBytes} {
		p := bytes.Repeat([]byte("x"), n)
		got, err := Read(bytes.NewReader(p))
		if err != nil || !bytes.Equal(got, p) {
			t.Fatalf("n=%d: got %d bytes, err %v", n, len(got), err)
		}
		if n > 0 {
			got[0] = 'y'
			if p[0] == got[0] {
				t.Fatalf("n=%d: Read aliased the *bytes.Reader's backing array", n)
			}
		}
	}

	over := bytes.Repeat([]byte("x"), MaxPayloadBytes+1)
	if _, err := Read(bytes.NewReader(over)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("MaxPayloadBytes+1 over *bytes.Reader: err = %v, want ErrTooLarge", err)
	}
}

// BenchmarkRead keeps its original rows (c.name, the *bytes.Reader fast
// path a *bytes.Reader argument actually takes in production: internal/
// hostguard's own replay) and adds a "_opaque" row per size that forces the
// general growth loop through opaqueReader -- the path every real hook,
// post or probe invocation's FIRST read of stdin actually runs, since
// stdin itself is never a *bytes.Reader. Both are production paths now;
// a single set of rows measuring only one would silently stop pricing the
// other.
func BenchmarkRead(b *testing.B) {
	for _, c := range []struct {
		name string
		n    int
	}{{"400B", 400}, {"40KB", 40_000}, {"1MiB", 1 << 20}, {"4MiB", 4 << 20}, {"8MiB", 8 << 20}} {
		p := payloadOf(c.n)
		for _, wrap := range []struct {
			suffix string
			mk     func([]byte) io.Reader
		}{
			{"", func(b []byte) io.Reader { return bytes.NewReader(b) }},
			{"_opaque", func(b []byte) io.Reader { return opaqueReader{bytes.NewReader(b)} }},
		} {
			b.Run(c.name+wrap.suffix, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(c.n))
				for i := 0; i < b.N; i++ {
					out, err := Read(wrap.mk(p))
					if err != nil || len(out) != c.n {
						b.Fatalf("read %d of %d: %v", len(out), c.n, err)
					}
				}
			})
		}
	}
}
