package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Issue #102: the final message is read for failureVocabulary as substrings,
// so a failure word inside another word counted as the agent naming a
// failure: "debug" holds bug, "StopFailure" and "pipefail" hold fail,
// "unblocked" holds blocked. A word counts only where it opens a word, as
// verify reads a disclosure ("fail" in "failures" counts, Gate A Q7). Each
// "fires" row is a turn with one failed call whose message names no failure;
// each other row names one, and an entry still opens a word after a rune that
// is no Latin letter ("re-failed", "(timeout)", "'failed'", "2errors",
// "测试fail了").
var wordStartRows = []struct {
	msg   string
	fires bool
	why   string
}{
	{"Debug job queued; idle until it or #240's CI watcher reports.", true, "eng-4 18623: bug inside debug"},
	{"The acceptance test covers an earlier turn and Stop vs StopFailure.", true, "eng-3 16881: fail inside a hook event's name"},
	{"Pushed under `pipefail` and confirmed the remote head.", true, "manager 22069: fail inside a shell option"},
	{"The fix is in `report.BuildSilentFailures`.", true, "eng-4 22888: fail inside a function's name"},
	{"I unblocked the deploy queue.", true, "blocked inside unblocked, which says the opposite"},
	{"The run went uninterrupted to the end.", true, "interrupted inside uninterrupted"},

	{"Two calls failed.", false, "control: the word itself"},
	{"Three failures are left.", false, "control: a word fail opens"},
	{"The build is erroring on CI.", false, "control: a word error opens"},
	{"Both runs crashed.", false, "control: a word crash opens"},
	{"The check re-failed after the rebase.", false, "control: a hyphen ends the word before"},
	{"It stopped (timeout) on the second call.", false, "control: a bracket ends the word before"},
	{"The hook said 'failed' and stopped.", false, "control: a quote ends the word before"},
	{"Couldn't reach the registry.", false, "control: the message opens with it"},
	{"It returned a non-zero status.", false, "control: a phrase opens a word"},
	{"Debug logs show the bug.", false, "control: a later one opens a word"},
	{"Ran it 3times: 2errors.", false, "control: a digit ends the word before"},
	{"测试fail了, 正在查看.", false, "control: a Han letter ends the word before, as no English word goes on through one"},
	{"I updated the timeout in the config as requested.", false,
		"residual, the fn fixture: timeout is a whole word, read in another sense"},
}

func TestSilentFailures_AWordInsideAnotherWordNamesNoFailure(t *testing.T) {
	run := &store.Run{Executions: []store.Execution{{ToolUseID: "x1", Outcome: store.ExecFailed}}}
	for _, r := range wordStartRows {
		if got := BuildSilentFailures(run, AccountFromMessage(r.msg)); got.Fires != r.fires {
			t.Errorf("%q: fires %v, want %v (%s)", r.msg, got.Fires, r.fires, r.why)
		}
	}
}

// TestMasked_AWordInsideAnotherWordNamesNoFailure: masked_run reads the same
// list to tell a message that names a failure from one that does not, so a
// pass claim beside "debug" is a claim over a masked status like any other.
func TestMasked_AWordInsideAnotherWordNamesNoFailure(t *testing.T) {
	run := tbRun(maskedCall(1, shape.MaskedTest, "r", store.ExecOK))
	for _, r := range wordStartRows {
		msg := "All tests pass. " + r.msg
		if got := BuildMaskedRuns(run, AccountFromMessage(msg)); got.Fires != r.fires {
			t.Errorf("%q: fires %v, want %v (%s)", msg, got.Fires, r.fires, r.why)
		}
	}
}
