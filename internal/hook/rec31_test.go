package hook

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestREC31_OversizeDegrades: a call whose record would be over
// MaxRecordBytes degrades, and only a record that is over it after that is
// refused.
//
// The first degrade step is the capture caps (REC-52): a payload whose values
// alone are past the record limit still lands a declaration, with those
// values withheld, closed by its terminal and covered as record_oversize.
// The store's own degrade (TestREC31c_) withholds the capped fields of a
// record that skipped the caps; one still over the limit after that -- here
// an over-long tool_name, which neither step drops -- is refused whole with
// ErrRecordOversize, nothing is written, and the hook names the loss
// record_oversize too. With the caps in place no payload reaches the refusal
// (TestREC31_WorstCaseRecordFits).
func TestREC31_OversizeDegrades(t *testing.T) {
	big := strings.Repeat("p", store.MaxRecordBytes)
	payload := capPayload(t, "transcript_path", "/"+big)
	payload = strings.Replace(payload, `"tool_name":"Bash"`, `"tool_name":"`+big+`"`, 1)

	decl, reason, _ := captured(t, payload)
	if decl == nil {
		t.Fatal("an oversize call was refused whole; it should degrade to a declaration with the values withheld")
	}
	if decl.TranscriptPath != "" || decl.ToolName != "" {
		t.Errorf("the oversize values were kept: transcript_path %d bytes, tool_name %d bytes",
			len(decl.TranscriptPath), len(decl.ToolName))
	}
	if reason != store.ReasonRecordOversize {
		t.Errorf("a degraded call's coverage reason is %q, want %q", reason, store.ReasonRecordOversize)
	}

	// Then refuses: the same values with no cap applied are not written.
	st := mustOpen(t)
	d := *decl
	d.TranscriptPath, d.ToolName = "/"+big, big
	err := st.AppendDeclaration(d)
	if !errors.Is(err, store.ErrRecordOversize) {
		t.Fatalf("an uncapped oversize declaration: %v, want ErrRecordOversize", err)
	}
	run, rerr := st.ReadRun(d.SessionID)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if len(run.Declarations)+run.Skipped != 0 {
		t.Errorf("a refused declaration left %d declarations and %d skipped lines", len(run.Declarations), run.Skipped)
	}
	if got := closeReason(err); got != store.ReasonRecordOversize {
		t.Errorf("the refusal reads as %q, want %q", got, store.ReasonRecordOversize)
	}
}

// TestREC31d_ADegradedRecordLandsWithheld: an append the store reports as
// degraded wrote the record, with its capped fields withheld, so the hook
// treats it as written -- the declaration is open and gets its terminal --
// and marks the call withheld, which its coverage record turns into
// record_oversize. Any other error is still the append's failure.
func TestREC31d_ADegradedRecordLandsWithheld(t *testing.T) {
	other := errors.New("disk full")
	for _, c := range []struct {
		in, out  error
		withheld bool
	}{
		{nil, nil, false},
		{fmt.Errorf("append: %w", store.ErrRecordDegraded), nil, true},
		{other, other, false},
		{store.ErrRecordOversize, store.ErrRecordOversize, false},
	} {
		withheld := false
		if got := landed(c.in, &withheld); !errors.Is(got, c.out) || (got == nil) != (c.out == nil) || withheld != c.withheld {
			t.Errorf("landed(%v) = %v with withheld %v; want %v and %v", c.in, got, withheld, c.out, c.withheld)
		}
	}
}

// TestREC31e_AHookAppendTheStoreDegradedLands: the hook's own appends take a
// degraded record as written, on both call paths. The caps at capture keep a
// payload from reaching the store's degrade, so the test stands in for a
// writer that skipped them (capDecl, capExec) and hands the store a record
// over MaxRecordBytes in a capped field. The declaration must land and be
// closed by its terminal, the execution must land, and each call's coverage
// record must say record_oversize; an append taken as failed leaves the
// declaration without its terminal and the call reading as an internal
// error.
func TestREC31e_AHookAppendTheStoreDegradedLands(t *testing.T) {
	huge := strings.Repeat("<", store.MaxRecordBytes)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	t.Run("declaration", func(t *testing.T) {
		st := mustOpen(t)
		h := New(st, time.Now)
		h.capDecl = func(d *store.Declaration) bool { d.CWD = &huge; return false }
		h.Close(&Signals{}, h.Capture(strings.NewReader(capPayload(t, "cwd", "/tmp/project"))))
		run, err := st.ReadRun("sess-caps")
		if err != nil {
			t.Fatal(err)
		}
		if len(run.Declarations) != 1 || len(run.Terminals) != 1 || len(run.Coverage) != 1 {
			t.Fatalf("read %d declarations, %d terminals, %d coverage records; want 1 each",
				len(run.Declarations), len(run.Terminals), len(run.Coverage))
		}
		if d := run.Declarations[0]; d.CWD != nil || withheldReason(d.CWDWithheld) != "oversize" {
			t.Errorf("degraded declaration kept cwd %v (reason %q)", d.CWD != nil, withheldReason(d.CWDWithheld))
		}
		if term := run.Terminals[0]; term.Outcome != store.OutcomeOK || term.ToolUseID != run.Declarations[0].ToolUseID {
			t.Errorf("terminal %+v does not close the degraded declaration as ok", term)
		}
		if r := run.Coverage[0].Reason; r == nil || *r != store.ReasonRecordOversize {
			t.Errorf("coverage reason %v, want %q", r, store.ReasonRecordOversize)
		}
	})

	t.Run("execution", func(t *testing.T) {
		st := mustOpen(t)
		p := NewPost(st, time.Now)
		p.capExec = func(x *store.Execution) bool { x.ExecutedCommand = &huge; return false }
		payload := `{"session_id":"s","tool_use_id":"toolu_x","tool_name":"Bash","hook_event_name":"PostToolUse"}`
		p.Close(&Signals{}, p.Capture(strings.NewReader(payload)))
		run, err := st.ReadRun("s")
		if err != nil {
			t.Fatal(err)
		}
		if len(run.Executions) != 1 || len(run.Coverage) != 1 {
			t.Fatalf("read %d executions, %d coverage records; want 1 each", len(run.Executions), len(run.Coverage))
		}
		if x := run.Executions[0]; x.ExecutedCommand != nil || withheldReason(x.ExecutedCommandWithheld) != "oversize" {
			t.Errorf("degraded execution kept executed_command %v (reason %q)",
				x.ExecutedCommand != nil, withheldReason(x.ExecutedCommandWithheld))
		}
		if r := run.Coverage[0].Reason; r == nil || *r != store.ReasonRecordOversize {
			t.Errorf("coverage reason %v, want %q", r, store.ReasonRecordOversize)
		}
	})
}

func withheldReason(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
