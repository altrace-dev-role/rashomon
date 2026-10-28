package verify

import (
	"cmp"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// timeoutFired is the exit status timeout(1) gives when the duration ran out
// and the command was stopped: timeout's own status, not the runner's.
const timeoutFired = 124

// noWrite is Claude Code's tools that track or stop tasks, ask the user,
// enter or leave plan mode, read or stop a background shell, load a skill,
// search for tools, message another agent, schedule or list cron jobs, or
// list and read MCP resources: none writes a source or test file. The copy of
// report's list (testbending.go:281), which is the source and must stay equal
// to this (INT14 holds one row per name). Owed: one copy, lifted into shape
// with mayEdit, which both read (a later card, beside outranks's).
var noWrite = map[string]bool{
	"TodoWrite": true, "TaskCreate": true, "TaskUpdate": true, "TaskList": true, "TaskGet": true,
	"TaskOutput": true, "TaskStop": true, "AskUserQuestion": true, "EnterPlanMode": true,
	"ExitPlanMode": true, "BashOutput": true, "KillShell": true, "KillBash": true, "Skill": true,
	"ToolSearch": true, "SendMessage": true, "CronCreate": true, "CronDelete": true, "CronList": true,
	"ListMcpResourcesTool": true, "ReadMcpResourceTool": true,
}

// testsOnlyThenGreen is the tests_only_then_green pairs whose passing run is
// in the window (from, at], each (the failed run's Seq, the passing run's
// Seq), in Seq order of the passing run (2.2.6; spec 20.2.6: "the existing
// pair (testbending.go:173-174), emitted at the passing run"). It is report's
// detector read over classify's pick, and the definition is report's (Gate A,
// card 1.8r): INT14 holds it equal to report.DetectTestBending over a
// MaskingAsRan copy on any record.
//
// Over every declaration in Seq order, whatever agent made it. A call's
// outcome is its picked execution's (outranks, report.outcomeRecord's rule):
// failed when that failed, unless the line's program is timeout and it exited
// 124, which is no result; otherwise its outcome, with a backgrounded one no
// result; a call with no execution has none. Masking is read as the line ran
// (asRan; D44), as report reads it after MaskingAsRan.
//
//   - A run is a declaration of verb class test that hid no runner's status
//     and ended ok or failed. Two runs are the same command when their shape
//     digests and their CWDDigests are equal; a run at no known directory
//     (CWDDigest "") pairs with none.
//   - An edit is a call that may change files (mayEdit). It is a test edit when
//     it is a file tool's write (class write) to a path labelled test-file that
//     ended ok; a test-labelled write that failed or has no execution moves
//     neither count, since whether it changed the file is what the record
//     cannot say; every other edit is an other edit.
//   - A run pairs with the last run of its command before it, when that one
//     failed, this one passed, no other edit fell between them and at least
//     one test edit did. A run is no edit between itself and the run it pairs
//     with, and every run with a result becomes its command's last run, so
//     two reds then a green pair the second red alone.
//
// A record holding a call by the point whose declaration was lost (an
// execution or terminal naming a tool_use_id no declaration names, other
// than a flush_failed terminal with no id) has no pairs: that call may have
// been an edit anywhere. The check is for the detector on its own, and runs
// only once a pair is found. Within Evaluate it can never fire, since
// recording's outcome_without_declaration marks such a record incomplete
// first, so integrity reads testPairs, the walk without it (1.8r2).
//
// Cost: testPairs, then the declared-id set only when a pair is found.
func testsOnlyThenGreen(run *store.Run, picked map[string]*store.Execution, from, at int64) [][2]int64 {
	pairs := testPairs(run, picked, from, at)
	if len(pairs) > 0 && undeclared(run, at) {
		return nil
	}
	return pairs
}

// testPairs is testsOnlyThenGreen's walk without its guard: the pairs as
// though no call by the point had lost its declaration. It is for a caller
// that has established recording reads the record Complete at at, whose
// outcome_without_declaration has then refused every record the guard
// would void (integrity, by its precondition).
//
// Cost: one walk over the declarations, a map lookup in picked for each, and
// a map of command to its last run; an index slice and a sort only when the
// declarations are not already in Seq order, as the store writes them. No
// copy of the declarations and no executions index.
func testPairs(run *store.Run, picked map[string]*store.Execution, from, at int64) [][2]int64 {
	if run == nil {
		return nil
	}
	type command struct{ digest, cwd string }
	type mark struct {
		seq           int64
		failed        bool
		tests, others int
	}
	var pairs [][2]int64
	var tests, others int
	last := make(map[command]mark)
	visit := func(d *store.Declaration) {
		e := picked[d.ToolUseID]
		hidden := maskedAsRan(d, e)
		ok, failed := result(d, e)
		isRun := d.Shape.VerbClass == shape.VerbTest && !hidden && (ok || failed)
		key := command{d.Shape.Digest, d.CWDDigest}
		if prev, seen := last[key]; seen && isRun && prev.failed && ok && others == prev.others && tests > prev.tests && from < d.Seq && d.Seq <= at {
			pairs = append(pairs, [2]int64{prev.seq, d.Seq})
		}
		if mayEdit(d, hidden) {
			switch {
			case d.Shape.VerbClass != shape.VerbWrite || d.FileLabel == nil || *d.FileLabel != shape.LabelTestFile:
				others++
			case ok:
				tests++
			}
		}
		if isRun && d.CWDDigest != "" {
			last[key] = mark{seq: d.Seq, failed: failed, tests: tests, others: others}
		}
	}
	bySeq := func(a, b store.Declaration) int { return cmp.Compare(a.Seq, b.Seq) }
	if slices.IsSortedFunc(run.Declarations, bySeq) {
		for i := range run.Declarations {
			visit(&run.Declarations[i])
		}
	} else {
		order := make([]int, len(run.Declarations))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(i, j int) int { return bySeq(run.Declarations[i], run.Declarations[j]) })
		for _, i := range order {
			visit(&run.Declarations[i])
		}
	}
	return pairs
}

// result is whether d ended ok and whether it failed, by e, its picked
// execution: report's testOutcome over the same record. A backgrounded call
// and one with no execution did neither, nor did a timeout-prefixed line
// stopped by timeout (exit 124).
func result(d *store.Declaration, e *store.Execution) (ok, failed bool) {
	switch {
	case e == nil:
		return false, false
	case e.Outcome == store.ExecFailed:
		stopped := d.Shape.Program != nil && *d.Shape.Program == "timeout" && e.ExitCode != nil && *e.ExitCode == timeoutFired
		return false, !stopped
	}
	return e.Outcome == store.ExecOK && !e.Backgrounded, false
}

// maskedAsRan reports whether d's line, as it ran (asRan, with e its picked
// execution), hid a test or build runner's status.
func maskedAsRan(d *store.Declaration, e *store.Execution) bool {
	m := d.Shape.StatusMasked
	if e != nil {
		m, _, _, _ = asRan(d, e)
	}
	return m != nil && (*m == shape.MaskedTest || *m == shape.MaskedBuild)
}

// mayEdit reports a call that may change files: report's mayEdit, whose
// comment says why each class is in or out. Every verb class but read,
// network and agent, and a shell line of any class that may write or hid a
// runner's status; never a noWrite tool.
func mayEdit(d *store.Declaration, hidden bool) bool {
	if noWrite[d.ToolName] {
		return false
	}
	if d.Shape.MayWrite || hidden {
		return true
	}
	switch d.Shape.VerbClass {
	case shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:
		return false
	}
	return true
}

// undeclared reports a call in run by the point at whose declaration was
// lost: an execution or terminal by the point (byPoint) naming a tool_use_id
// no declaration names, report's undeclaredCalls cut at the point so that
// integrity at a point equals integrity over the prefix (INV-11). A
// flush_failed terminal with no id names no call and is skipped.
func undeclared(run *store.Run, at int64) bool {
	declared := make(map[string]struct{}, len(run.Declarations))
	for i := range run.Declarations {
		declared[run.Declarations[i].ToolUseID] = struct{}{}
	}
	for i := range run.Executions {
		e := &run.Executions[i]
		if _, ok := declared[e.ToolUseID]; !ok && byPoint(e.Seq, at) {
			return true
		}
	}
	for i := range run.Terminals {
		t := &run.Terminals[i]
		if t.FlushFailed() && t.ToolUseID == "" {
			continue
		}
		if _, ok := declared[t.ToolUseID]; !ok && byPoint(t.Seq, at) {
			return true
		}
	}
	return false
}
