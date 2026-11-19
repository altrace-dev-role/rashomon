package hook

import (
	"github.com/altrace-dev-role/rashomon/internal/editmarks"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// fillEditMarks sets rec's edit marks (W-M3.7; issue 207): for an Edit or a
// MultiEdit that ended ok, the marks editmarks.Of computes from the call's
// tool_input, on the edit_delta basis. A failure event lands nothing, so it
// has none, and nor has a call that moves no id, which is most calls.
//
// Marks are scope-independent: they hold ids from a closed table and counts
// and never text of the edit, so no record scope withholds them. The user's
// switch does. config.json's edit_marks, read only for a call that has marks
// to write, so an edit with none pays no read, turns them off: the execution
// then carries none and edit_marks_withheld says switched_off. A config that
// does not read is off for it, as for the steer notes, since the switch the
// person set cannot be told.
func fillEditMarks(rec *store.Execution, st *store.Store, event, tool string, input []byte) {
	if event == FailureEvent || len(input) == 0 {
		return
	}
	marks := editmarks.Of(tool, input)
	if marks == nil {
		return
	}
	if cfg, err := store.ReadConfig(st.Root()); err != nil || cfg.EditMarksOff() {
		reason := store.WithheldSwitchedOff
		rec.EditMarksWithheld = &reason
		return
	}
	rec.EditMarks = marks
}
