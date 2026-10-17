package hook

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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
