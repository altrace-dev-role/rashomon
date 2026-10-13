package jsonscan

import (
	"io"
	"strconv"
	"strings"
	"testing"
)

// repeatReader yields head, then body n times, then tail, without holding
// more than one copy of each: a payload of any size at no memory cost.
type repeatReader struct {
	parts [][]byte
	n     int
	i     int
	off   int
}

func newRepeatReader(head, body, tail string, n int) *repeatReader {
	return &repeatReader{parts: [][]byte{[]byte(head), []byte(body), []byte(tail)}, n: n}
}

func (r *repeatReader) Read(p []byte) (int, error) {
	w := 0
	for w < len(p) {
		part := r.i
		switch {
		case r.i == 0:
		case r.i <= r.n:
			part = 1
		case r.i == r.n+1:
			part = 2
		default:
			if w == 0 {
				return 0, io.EOF
			}
			return w, nil
		}
		k := copy(p[w:], r.parts[part][r.off:])
		w += k
		r.off += k
		if r.off == len(r.parts[part]) {
			r.i, r.off = r.i+1, 0
		}
	}
	return w, nil
}

func (r *repeatReader) size() int64 {
	return int64(len(r.parts[0]) + r.n*len(r.parts[1]) + len(r.parts[2]))
}

// BenchmarkTop measures one scan as an entry makes it: the KeySet built for
// the call and the walk. Top itself allocates nothing per value
// (TestTopAllocatesNothingPerValue); the KeySet is the per-call cost, and
// the warm-up call keeps json.Valid's pooled scanner, refilled only after a
// GC, out of the measured loop, so the figure does not move with GC timing.
func BenchmarkTop(b *testing.B) {
	names := []string{"session_id", "prompt_id", "agent_id", "agent_type", "hook_event_name",
		"tool_name", "tool_use_id", "cwd", "permission_mode", "transcript_path", "tool_input",
		"tool_response", "error", "is_interrupt", "duration_ms"}
	keys := NewKeySet(names...)
	p := []byte(`{"session_id":"5c7e2b1a-9f0d-4e8b-a1c2-3d4e5f60718a","transcript_path":"/home/u/.claude/projects/p/s.jsonl",` +
		`"cwd":"/home/u/src/p","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Edit",` +
		`"tool_input":{"file_path":"/home/u/src/p/main.go","old_string":"x","new_string":"` +
		strings.Repeat(`\tif err != nil {\n\t\treturn err\n\t}\n`, 1500) + `"},"tool_use_id":"toolu_01"}`)
	out := make([]Span, keys.Len())
	if err := Top(p, keys, out); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(p)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		keys := NewKeySet(names...)
		if err := Top(p, keys, out); err != nil || out[10].Kind != Object {
			b.Fatalf("Top: %v, tool_input %+v", err, out[10])
		}
	}
}

func BenchmarkLines(b *testing.B) {
	var s strings.Builder
	s.WriteString(`{"stdout":"`)
	for i := 0; s.Len() < 4<<20; i++ {
		s.WriteString("--- PASS: TestCase" + strconv.Itoa(i) + ` (0.00s)\n`)
	}
	s.WriteString(`ok  \tpkg\t0.4s"}`)
	p := []byte(s.String())
	keys := NewKeySet("stdout")
	out := make([]Span, 1)
	if err := Top(p, keys, out); err != nil {
		b.Fatal(err)
	}
	work := make([]byte, 64<<10)
	b.Run("4MiB", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(p)))
		for i := 0; i < b.N; i++ {
			n := 0
			if err := Lines(p, out[0], work, func([]byte, bool) bool { n++; return true }); err != nil || n < 100_000 {
				b.Fatalf("Lines: %d lines, %v", n, err)
			}
		}
	})
}

func BenchmarkStream(b *testing.B) {
	call := `{"tool_use_id":"toolu_0123456789","tool_name":"Bash","tool_input":{"command":"go test ./..."},` +
		`"tool_response":{"stdout":"` + strings.Repeat(`ok  \texample.com/pkg\t0.412s\n`, 2000) + `","stderr":""}},`
	for _, c := range []struct {
		name  string
		r     func() *repeatReader
		paths []Path
		want  int
	}{
		{"64MiB_batch", func() *repeatReader {
			return newRepeatReader(`{"session_id":"s","hook_event_name":"PostToolBatch","tool_calls":[`, call,
				`{"tool_use_id":"last","tool_name":"Bash"}]}`, (64<<20)/len(call))
		}, []Path{
			{Elems: []string{"session_id"}, Max: 256, Mode: CaptureHead},
			{Elems: []string{"tool_calls", "[]", "tool_use_id"}, Max: 256, Mode: CaptureHead},
			{Elems: []string{"tool_calls", "[]", "tool_name"}, Max: 256, Mode: CaptureHead},
		}, 1 + 2*((64<<20)/len(call)+1)},
		{"187MB_line", func() *repeatReader {
			return newRepeatReader(`{"type":"assistant","message":"`, `The change is in place; every test passes.\n`, `"}`, 187_000_000/44)
		}, []Path{
			{Elems: []string{"message"}, Max: 64 << 10, Mode: CaptureHeadTail},
		}, 2},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(c.r().size())
			for i := 0; i < b.N; i++ {
				r := c.r()
				n := 0
				read, err := Stream(r, c.paths, func(int, []byte, bool) bool { n++; return true })
				if err != nil || read != r.size() || n != c.want {
					b.Fatalf("Stream: read %d of %d, %d values (want %d), %v", read, r.size(), n, c.want, err)
				}
			}
		})
	}
}
