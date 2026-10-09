package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestAccounting_AReadableTranscriptRendersEveryEmptyListAsNone: a transcript
// list is null only when the transcript could not be read. A readable one with
// nothing to list is a comparison that found nothing, and rendering it as
// "unknown" is the unknown-never-zero mistake pointed the other way. Nothing
// else pins it: each list's empty initialiser can be deleted with every other
// test green.
func TestAccounting_AReadableTranscriptRendersEveryEmptyListAsNone(t *testing.T) {
	path := writeTranscriptBlocks(t, []blk{
		{Type: "tool_result", ToolUseID: "toolu_ran", Content: "ok"},
	})
	sess := build(&store.Run{
		Declarations: []store.Declaration{
			{ToolUseID: "toolu_ran", ToolName: "Bash", SessionID: "s", TranscriptPath: path},
		},
		Executions: []store.Execution{{ToolUseID: "toolu_ran", SessionID: "s"}},
	})
	tr := groupFor(t, sess, path)
	if !tr.Readable {
		t.Fatal("premise: the transcript was not read")
	}
	lists := map[string][]string{
		"missing from store":      tr.MissingFromStore,
		"missing from transcript": tr.MissingFromTranscript,
		"refused before any hook": tr.RefusedBeforeHooks,
		"executed but unrecorded": tr.ExecutedButUnrecorded,
		"denied before running":   tr.DeniedByUser,
		"declared without result": tr.DeclaredWithoutResult,
	}
	var b bytes.Buffer
	if err := Text(&b, &Report{Sessions: []Session{sess}}); err != nil {
		t.Fatal(err)
	}
	for label, l := range lists {
		if l == nil || len(l) != 0 {
			t.Errorf("%s = %#v, want an empty list: the transcript was read and found nothing", label, l)
		}
		if line := "    " + label + ": " + none + "\n"; !strings.Contains(b.String(), line) {
			t.Errorf("the render does not carry %q:\n%s", strings.TrimSpace(line), b.String())
		}
	}
}
