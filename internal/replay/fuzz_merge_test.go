package replay

import (
	"fmt"
	"strings"
	"testing"
)

// FuzzSessionMerge holds the merge of a session's files to the properties that
// do not depend on what the transcripts say, over two transcripts generated
// from ops (the generator of FuzzReplayTimeline) read as two files of one
// session, and over arbitrary bytes read as a second file:
//
//   - no panic, no error from memory, and the same merged timeline on a second
//     read;
//   - each call has at most one PreToolUse and one Post in the whole timeline,
//     the Post after the Pre in the order yielded, whichever file each is in;
//   - a Gap has a taint, and the taint of the merged timeline, once there,
//     never moves later or goes away;
//   - a file merged with a byte-different copy of itself (one more line of
//     bookkeeping) gives the calls it gives alone, once each, in the same order.
func FuzzSessionMerge(f *testing.F) {
	f.Add([]byte{0, 1, 1, 2, 2, 5}, []byte{0, 1, 1, 2, 2, 5, 0, 1, 1, 3, 2, 3}, []byte(`{"type":"user"}`))
	f.Add([]byte{0, 1, 1, 2, 7, 0, 1, 3}, []byte{1, 0, 2, 0, 6, 1}, []byte(""))
	f.Add([]byte{}, []byte{0, 1, 3, 1, 5, 0}, []byte("{\n"))
	f.Fuzz(func(t *testing.T, opsA, opsB, raw []byte) {
		a, b := generate(opsA), generate(opsB)
		checkMergeProps(t, a, b)
		checkMergeProps(t, a, string(raw))
		checkCopyIdempotent(t, a)
	})
}

func fuzzSession(t *testing.T, docs ...string) SessionTimeline {
	t.Helper()
	tl, err := BuildSession(synthSession, files(len(docs), docs...), mopt())
	if err != nil {
		t.Fatalf("a read from memory failed: %v", err)
	}
	return tl
}

func checkMergeProps(t *testing.T, a, b string) {
	t.Helper()
	tl := fuzzSession(t, a, b)
	if again := fuzzSession(t, a, b); fmt.Sprint(again.Events) != fmt.Sprint(tl.Events) || again.TaintFrom != tl.TaintFrom {
		t.Fatalf("a second merge differs")
	}
	pre, post := map[string]int{}, map[string]int{}
	var first Place
	for i, e := range tl.Events {
		switch e.Kind {
		case PreToolUse:
			if _, dup := pre[e.ToolUseID]; dup {
				t.Fatalf("two PreToolUse for %q", e.ToolUseID)
			}
			pre[e.ToolUseID] = i
		case PostToolUse, PostToolUseFailure:
			if p, ok := pre[e.ToolUseID]; !ok || p > i {
				t.Fatalf("Post for %q before its Pre (%v)", e.ToolUseID, ok)
			}
			if post[e.ToolUseID]++; post[e.ToolUseID] > 1 {
				t.Fatalf("two Post for %q", e.ToolUseID)
			}
		}
		if e.Gap != "" && e.Taint.IsZero() {
			t.Fatalf("%s %q is a gap with no taint", e.Kind, e.ToolUseID)
		}
		here := Place{File: e.TaintFile, Pos: e.Taint}
		if !first.IsZero() && here != first {
			t.Fatalf("taint moved: %+v then %+v", first, here)
		}
		if first.IsZero() {
			first = here
		}
	}
	if !first.IsZero() && first != tl.TaintFrom {
		t.Fatalf("the events' taint %+v is not the timeline's %+v", first, tl.TaintFrom)
	}
}

func checkCopyIdempotent(t *testing.T, a string) {
	t.Helper()
	calls := func(tl SessionTimeline) string {
		var s []string
		for _, e := range tl.Events {
			if e.Kind == PreToolUse || e.Kind == PostToolUse || e.Kind == PostToolUseFailure {
				s = append(s, fmt.Sprintf("%s:%s:%v", e.Kind, e.ToolUseID, e.Supported()))
			}
		}
		return strings.Join(s, " ")
	}
	alone := fuzzSession(t, a)
	both := fuzzSession(t, a, a+`{"type":"last-prompt","sessionId":"s1"}`+"\n")
	if calls(alone) != calls(both) {
		t.Fatalf("a copy changed the calls:\n alone %s\n  both %s", calls(alone), calls(both))
	}
}
