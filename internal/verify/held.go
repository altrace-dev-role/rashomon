package verify

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// A write is an edit only where the record holds its call's execution and it
// ended ok. The words below are how classify reads every other ending of a
// call that may write, and what it keeps of such a call (held) for the
// readers that must still see it.

// WriteEnd is how the record holds the ending of a call that may write: the
// execution classify picks for it (pickExecutions), read by its outcome.
type WriteEnd uint8

const (
	// EndOK: the execution ended ok. Only such a call's write is an edit.
	EndOK WriteEnd = iota
	// EndFailedEdit: a file tool's execution failed. The tool reported that
	// it did not apply, so the call wrote nothing.
	EndFailedEdit
	// EndFailedCall: a shell line's or an MCP call's execution failed.
	// Whether it wrote before it ended is not in the record.
	EndFailedCall
	// EndNone: no execution is in the record. The call may still be running,
	// may have been refused, or its ending was lost; whether it wrote, and
	// when, is not in the record.
	EndNone
	// EndInterrupted: any call's execution was interrupted. It reads as
	// EndFailedCall does everywhere but in the line's words, which say the
	// call was interrupted: the person stopped it, and nothing failed.
	EndInterrupted
)

// mayHaveWritten reports an ending under which the call may have written:
// every one but EndOK, whose write is an edit, and EndFailedEdit, a file
// tool that reported it did not apply.
func (e WriteEnd) mayHaveWritten() bool { return e != EndOK && e != EndFailedEdit }

// heldWrite is a write of a call whose ending is not EndOK, which classify
// holds out of the writes: no gate, count or note reads it as an edit. The
// readers that must still see it read it here: the row's guard
// (TurnCounts.unconfirmed), a rule that cannot be decided while a call
// may have written (heldBlocks), and store_touched, outcome-blind (D135).
type heldWrite struct {
	write
	End WriteEnd
}

// endOf is d's ending as e records it; e is pickExecutions' for d, nil for
// none. An interrupt arrives on the failure event, and a file tool stopped
// part way may have written, so only a failed file tool reads as not applied.
func endOf(d *store.Declaration, e *store.Execution) WriteEnd {
	switch {
	case e == nil:
		return EndNone
	case e.Outcome == store.ExecOK:
		return EndOK
	case e.Outcome == store.ExecFailed && fileWrite(d):
		return EndFailedEdit
	case e.Outcome == store.ExecInterrupted:
		return EndInterrupted
	}
	return EndFailedCall
}

// fileWrite reports a file tool's write (Write, Edit, MultiEdit,
// NotebookEdit): appendWrites' file branch.
func fileWrite(d *store.Declaration) bool {
	return d.ToolName != "Bash" && d.Shape.VerbClass == shape.VerbWrite
}

// appendHeld adds ws, d's writes as appendWrites read them, to held under
// end, with one entry with no path where d may have written a path the
// record cannot name (unnamed); a call that may write nothing adds nothing.
func appendHeld(held []heldWrite, ws []write, unnamed bool, d *store.Declaration, end WriteEnd) []heldWrite {
	named := false
	for _, w := range ws {
		held = append(held, heldWrite{write: w, End: end})
		named = named || w.Path != nil
	}
	// A shell line whose list was not read whole is unnamed beside its
	// named paths; a blind write already holds its entry with no path.
	if named && unnamed {
		held = append(held, heldWrite{write: write{Seq: d.Seq, decl: d, Tool: d.ToolName}, End: end})
	}
	return held
}

// heldBlocks reports a held write declared strictly between from and to
// that may have written a path paths match and excludes do not, in one of
// modes (nil reads every mode): a call that failed and may have written or
// has no recorded ending, never a failed file tool, which did not apply, nor
// an MCP call, which no path rule reads. Its path is matched as a write's
// would be (projectRels); one the record cannot name, or a Bash target
// holding a glob, may match. A rule that reads writes cannot say a path was
// not written while such a call stands, and cannot say it was. The cost: a
// refused, unended, interrupted or failed shell write naming a protected
// path leaves protect_path undecided where the call did write and the base
// read a violation, forbid_code undecided where it held, and an order rule's
// invalidation a maybe. The verdict says it could not decide, never that
// the path was kept, but only the ledger prints it (`rashomon report
// --ledger`, a spec_rule row unknown): the Stop names violations alone
// (recap's ruleLines), so where the base printed "spec rule <id> protects a
// path call N wrote" and counted a record finding, the Stop now prints no
// rule line and no finding for the rule. The turn's line, or the report's
// where a detector's sentence takes the Stop, names the call only where it
// bears on the row as an edit of its path would (countIn), and then without
// the rule.
func heldBlocks(held []heldWrite, paths, excludes, modes []string, from, to int64) bool {
	for i := range held {
		w := &held[i]
		if w.Seq <= from || w.Seq >= to || w.End == EndFailedEdit || strings.HasPrefix(w.Tool, "mcp__") {
			continue
		}
		if w.Path == nil {
			return true
		}
		rel, inside := projectPath(*w.Path)
		if inside && modeTri(w.Tool, modes) != triNo &&
			(w.Tool == "Bash" && strings.ContainsAny(rel, "*?[{") || anyPath(paths, rel) && !anyPath(excludes, rel)) {
			return true
		}
	}
	return false
}
