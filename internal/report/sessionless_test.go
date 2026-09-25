package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// sessionlessStore holds one Claude Code session and n declarations in the
// unattributed bucket -- the run directory every payload that named no
// session is recorded under.
//
// The session also holds a declaration with NO PROMPT ID. That is what
// chains.go's Unattributed collects, and it is a different fact: a real
// session's call made before its first prompt, or any v1 record. A count
// that read the chain view would report it as a call without a session id,
// so it is here to be not counted.
func sessionlessStore(t *testing.T, n int) (*store.Store, time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	prompt := "prompt-1"
	for _, d := range []store.Declaration{
		{SessionID: "sess-1", ToolUseID: "toolu_prompted", PromptID: &prompt},
		{SessionID: "sess-1", ToolUseID: "toolu_before_any_prompt"},
	} {
		d.Type, d.SchemaVersion, d.ToolName, d.RecordedAtMS = store.TypeDeclaration, store.SchemaVersion, "Bash", now.UnixMilli()
		if err := st.AppendDeclaration(d); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < n; i++ {
		if err := st.AppendDeclaration(store.Declaration{
			Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
			SessionID: store.UnattributedSession, ToolName: "Bash",
			RecordedAtMS: now.UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return st, now
}

// TestCallsWithoutSessionID_CountsTheUnattributedRun: the count comes from
// the store's unattributed run, store-wide, in a report scoped to one session
// and in the whole report alike -- and never from a session's own prompt-less
// declarations.
func TestCallsWithoutSessionID_CountsTheUnattributedRun(t *testing.T) {
	st, now := sessionlessStore(t, 2)

	for _, scope := range []string{"sess-1", ""} {
		rep, err := Build(st, scope, now)
		if err != nil {
			t.Fatalf("Build(%q): %v", scope, err)
		}
		if rep.CallsWithoutSessionID == nil || *rep.CallsWithoutSessionID != 2 {
			t.Errorf("Build(%q): calls without a session id = %v, want 2", scope, deref(rep.CallsWithoutSessionID))
		}
	}

	// Premise: the prompt-less declaration really is in the chain view's
	// unattributed group, so the count above ignoring it means something.
	rep, err := Build(st, "sess-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(rep.Sessions[0].Chains.Unattributed); got != 1 {
		t.Fatalf("premise broken: sess-1's chain view has %d prompt-less link(s), want 1", got)
	}
}

// TestCallsWithoutSessionID_IsZeroWithNothingUnattributed: a store that
// never received a sessionless payload counts zero, and zero is a real
// answer here -- it is a count of this store's own records.
func TestCallsWithoutSessionID_IsZeroWithNothingUnattributed(t *testing.T) {
	st, now := sessionlessStore(t, 0)
	rep, err := Build(st, "sess-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if rep.CallsWithoutSessionID == nil || *rep.CallsWithoutSessionID != 0 {
		t.Errorf("calls without a session id = %v, want 0", deref(rep.CallsWithoutSessionID))
	}
}

// TestCallsWithoutSessionID_IsUnknownWhenTheRunCannotBeRead: an unreadable
// unattributed run makes the count unknown -- null, never zero -- and costs
// only this line. A report scoped to another session still renders: failing
// it would make a store-wide aside the reason a session's own report cannot
// be read.
func TestCallsWithoutSessionID_IsUnknownWhenTheRunCannotBeRead(t *testing.T) {
	st, now := sessionlessStore(t, 1)
	// A directory where the records file should be: open succeeds, the read
	// fails, which is a real I/O error rather than a missing-file no-op.
	records := filepath.Join(st.RunDir(store.UnattributedSession), store.FileRecords)
	if err := os.Remove(records); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(records, 0o700); err != nil {
		t.Fatal(err)
	}

	rep, err := Build(st, "sess-1", now)
	if err != nil {
		t.Fatalf("a report scoped to sess-1 failed because the unattributed run is unreadable: %v", err)
	}
	if rep.CallsWithoutSessionID != nil {
		t.Errorf("calls without a session id = %d, want unknown (nil)", *rep.CallsWithoutSessionID)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"calls_without_session_id":null`)) {
		t.Errorf("the JSON does not carry null for an unknown count:\n%s", b)
	}
}

// TestCallsWithoutSessionID_IsUnknownOverLinesItCouldNotRead: a line in the
// unattributed run that did not parse may be one more call, so a count of the
// lines that did is a floor, not a number -- and a floor printed as a count
// is the zero-for-unknown this report exists to refuse. The line here says
// it is a declaration and then fails to be one: its seq is not a number.
func TestCallsWithoutSessionID_IsUnknownOverLinesItCouldNotRead(t *testing.T) {
	st, now := sessionlessStore(t, 1)
	records := filepath.Join(st.RunDir(store.UnattributedSession), store.FileRecords)
	f, err := os.OpenFile(records, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"declaration","schema_version":2,"seq":"not a number","session_id":"unattributed"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	rep, err := Build(st, "sess-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if rep.CallsWithoutSessionID != nil {
		t.Errorf("calls without a session id = %d over a run with an unreadable line, want unknown (nil)",
			*rep.CallsWithoutSessionID)
	}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"calls_without_session_id":null`)) {
		t.Errorf("the JSON does not carry null for a count over unreadable lines:\n%s", b)
	}
	var out bytes.Buffer
	if err := Text(&out, rep); err != nil {
		t.Fatal(err)
	}
	if want := "calls without a session id: unknown (session unattributed could not be read)"; !strings.Contains(out.String(), want) {
		t.Errorf("the text does not say %q:\n%s", want, out.String())
	}
}

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
