package hook

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestREC31_OversizeDegrades: a call whose record would be over
// MaxRecordBytes degrades, and only a record that is over it after that is
// refused.
//
// At schema 4 the degrade step is the capture caps (REC-52), not a second
// marshal in marshalLine: the spec's <field>_withheld keys are schema 5, and
// a v4 record says a value was cut with record_oversize on the call's
// coverage. So a payload whose values alone are past the record limit still
// lands a declaration, with those values withheld, closed by its terminal and
// covered as record_oversize. The refusal is the store's: a declaration that
// skipped the caps is refused whole with ErrRecordOversize, nothing is
// written, and the hook names the loss record_oversize too. With the caps in
// place no payload reaches the refusal (TestREC31_WorstCaseRecordFits).
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
