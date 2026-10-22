package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestEmpty_CountsNoCallsWithoutASessionID: the no-store report has the same
// shape as a built one (H-29), and a location where nothing was ever recorded
// received no sessionless call either.
func TestEmpty_CountsNoCallsWithoutASessionID(t *testing.T) {
	rep := Empty(time.Now())
	if rep.CallsWithoutSessionID == nil || *rep.CallsWithoutSessionID != 0 {
		t.Errorf("Empty: calls without a session id = %v, want 0", deref(rep.CallsWithoutSessionID))
	}
}

// TestText_CallsWithoutSessionID renders all three states, each distinct:
// the degraded line names the count and where the calls are, its healthy
// twin shares no words with it that a reader could mistake, and unknown is
// never rendered as none.
func TestText_CallsWithoutSessionID(t *testing.T) {
	one, three, zero := 1, 3, 0
	for _, tc := range []struct {
		name string
		n    *int
		want string
	}{
		{"one", &one, "1 call arrived without a session id (recorded under session unattributed)"},
		{"several", &three, "3 calls arrived without a session id (recorded under session unattributed)"},
		{"none", &zero, "calls without a session id: none"},
		{"unknown", nil, "calls without a session id: unknown (session unattributed could not be read)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := &Report{CallsWithoutSessionID: tc.n, Sessions: []Session{{SessionID: "sess-1"}}}
			var out bytes.Buffer
			if err := Text(&out, rep); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(out.String(), "\n")
			if len(lines) < 2 || lines[1] != tc.want {
				t.Errorf("the line after the header = %q, want %q\n%s", lines[min(1, len(lines)-1)], tc.want, out.String())
			}
		})
	}

	// With nothing recorded at all there is nothing to count against, and
	// "no sessions recorded" already says so.
	var out bytes.Buffer
	if err := Text(&out, Empty(time.Now())); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "session id") {
		t.Errorf("the empty report speaks of session ids:\n%s", out.String())
	}
}

// TestText_TheUnattributedRunHasNoFailureCount: no call that arrives without
// a session id records an outcome (hook.Post.Capture), so the unattributed
// block's "failed calls: 0" was a clean zero over outcomes never recorded. It
// renders unknown there, and a real session's zero stays a zero.
func TestText_TheUnattributedRunHasNoFailureCount(t *testing.T) {
	one := 1
	rep := &Report{CallsWithoutSessionID: &one, Sessions: []Session{
		{SessionID: store.UnattributedSession, SilentFailures: SilentFailures{AbsentWords: []string{}}},
		{SessionID: "sess-1", SilentFailures: SilentFailures{AbsentWords: []string{}}},
	}}
	var out bytes.Buffer
	if err := Text(&out, rep); err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(out.String(), "\nsession ")
	if len(blocks) != 3 {
		t.Fatalf("want a header and two session blocks, got %d:\n%s", len(blocks), out.String())
	}
	unattributed, sess := blocks[1], blocks[2]
	const unknownLine = "  failed calls: unknown (calls without a session id carry no outcome)\n"
	if !strings.Contains(unattributed, unknownLine) || strings.Contains(unattributed, "failed calls: 0") {
		t.Errorf("the unattributed block does not say its failure count is unknown:\n%s", unattributed)
	}
	if !strings.Contains(sess, "  failed calls: 0\n") || strings.Contains(sess, "failed calls: unknown") {
		t.Errorf("a real session's zero failures no longer render as 0:\n%s", sess)
	}
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
