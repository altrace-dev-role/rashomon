package verify

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// call is one tool call of a test run. The evaluator is pure over a
// store.Run, so its tests build the Run directly rather than reading NDJSON
// through the store (approach notes, Task 0.3). It holds no invariant of its
// own; runOf enforces the ones listed on it.
type call struct {
	seq      int64  // the author's ordering key and the ToolUseID source; a record's Seq is runOf's counter
	tool     string // Bash | Edit | Write, or any tool; only these three carry paths
	prog     string
	class    string
	runner   string // non-empty marks a recognised runner; becomes RunnerDigest
	kind     string // shape.RunnerTest | shape.RunnerBuild, with a runner only; becomes RunnerKind
	masked   string // shape.MaskedNone | shape.MaskedTest | shape.MaskedBuild; empty on a Bash call is none
	outcome  string // store.ExecOK | store.ExecFailed | store.ExecInterrupted
	exit     int
	bg       bool
	paths    []string
	mayWrite bool // Bash write with no lexical target, the unnamed-write case; paths empty
	// term is the call's terminal outcome, store.OutcomeOK | store.OutcomeError
	// | store.OutcomeSignal; empty builds no terminal, the signature of an
	// uncontrolled exit (record.go, Terminal).
	term string
	// incomplete marks a named Bash write whose extraction did not yield a
	// target for every write-capable segment: WrittenPathsComplete false
	// (record_v5.go). Only a Bash write with paths can carry it.
	incomplete bool
	// gateOutput is the execution's classification of a gate call's output
	// (record_v5.go GateOutput), copied onto the execution; nil writes none.
	// No writer fills gate_output yet.
	gateOutput *store.GateOutput
	// editMarks is the execution's edit marks (record_v5.go EditMarks), set by
	// marks and copied onto the execution; nil writes none. No writer fills
	// edit_marks yet (1.8p is the producer).
	editMarks *store.EditMarks
	// sha is the execution's content_sha256: the sha256 the post recorded
	// for the file an Edit or Write names, as the call left it (card P5A);
	// "" writes none.
	sha string
	// runnerName and maskedBy are the execution's runner_name and masked_by
	// (call schema 8, D229): the runner in its row of the binary's lists and
	// the stage of the pipe that hid its status; empty writes null. rules is
	// the execution's rules_version, 0 where the fixture sets none.
	runnerName, maskedBy string
	rules                int
	// schema is the version both records are stamped at: 0 is
	// store.SchemaVersion; 5 is a record written before runner_kind, whose
	// runner carries no kind.
	schema int
	// loc is the declaration's CWDDigest, a literal like the runner's: "d:" and
	// a name for a read location (shape.CWDDigest), "u:" and a name for an
	// unread one (shape.UnreadCWDDigest), the two domains no digest of one
	// equals in the other; "" is a location not known.
	loc string
	// agent is the subagent the call ran in: AgentID, with AgentType beside it,
	// both present only inside a subagent call (record.go, Declaration); ""
	// is the main agent, both nil.
	agent string
	// spilled marks a call whose execution and terminal the store could not
	// order under its lock and wrote to the spill file (Store.SpillExecution,
	// Store.SpillTerminal): both carry a nil Seq, take no Seq from the
	// counter and read back after every ordered record, as ReadRun reads the
	// spill file last. The declaration keeps its Seq: no SpillDeclaration
	// exists.
	spilled bool
	// lostDeclaration marks a call whose declaration was never written for
	// want of the lock while its terminal and execution were: no Declaration
	// and no Seq taken for one. Handler.Close then writes the terminal as the
	// only trace of the tool_use_id, straight to the spill file, with outcome
	// error and reason lock_timeout, so term must be store.OutcomeError and
	// the terminal reads back as spilled.
	lostDeclaration bool
}

func gateCall(seq int64, runner, outcome string) call {
	exit := 0
	if outcome == store.ExecFailed {
		exit = 1 // a plain failed run; a failure with an unknown code is a call literal with exit 0
	}
	return call{seq: seq, tool: "Bash", prog: "go", class: shape.VerbTest, runner: runner, kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: outcome, exit: exit, loc: "d:repo"}
}
func edit(seq int64, path string) call {
	return call{seq: seq, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{path}}
}
func bashWrite(seq int64, paths ...string) call {
	return call{seq: seq, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, paths: paths}
}

// marks is c with its execution carrying edit marks, the deltas per marker id
// (record_v5.go EditMarks), version 1, on the basis archive 10.2.8 gives the
// tool: an Edit's old_string against its new_string (edit_delta), a Write's
// content against the counts read before it (write_vs_before). runOf refuses
// marks on any other tool and copies them, so the record never shares the
// caller's maps.
func (c call) marks(added, removed map[string]int) call {
	basis := "edit_delta"
	if c.tool == "Write" {
		basis = "write_vs_before"
	}
	c.editMarks = &store.EditMarks{Added: added, Removed: removed, Basis: basis, Version: 1}
	return c
}

// runOf is runOfSession for the session "s1", the one most tests need.
func runOf(t testing.TB, calls ...call) *store.Run {
	t.Helper()
	return runOfSession(t, "s1", calls...)
}

// runOfSession writes one Declaration and one Execution per call, a
// Terminal for each call with a term and one Coverage record, and fails the
// test on a run the store could not read back as built, or whose records
// break an invariant the hook and shape keep; the two shapes no writer produces yet, a named Bash write and
// a gate_output, are marked below. A fixture that breaks one makes every
// evaluator test built on it prove nothing. It enforces these invariants and
// no others, each where it comes from:
//   - Declarations and executions pair by ToolUseID, and by index too in a run with no spilled call and no lost declaration; a terminal pairs by ToolUseID.
//   - Seq strictly increases, so it is at least 1 (store.appendOrdered allocates it). A spilled record has none and is read after every ordered one, as ReadRun reads the spill file last.
//   - A spilled call, or one whose declaration was lost, has a terminal: without one it is the uncontrolled exit's signature instead. A lost declaration's terminal is the one Handler.Close writes for it: error, lock_timeout, spilled.
//   - An Edit or Write names its path (the payload's file_path, which the hook records).
//   - A call that is not Bash names no runner, is not backgrounded and does not set mayWrite (deriveCut derives the runner and mayWrite only for a tool whose class is execute and whose input carries a command field, shape.go:158-163; of the three execute tools, verb.go:39, only Bash carries one; post.go reads Backgrounded for Bash alone).
//   - A call that is not Bash, Edit or Write names no paths. The builder models those three: the hook records a file_path for MultiEdit and NotebookEdit too, which no evaluator test builds yet.
//   - Every record of a call is stamped at schema 5, 6 or 7 (store.AcceptsKind reads 1 to 7; no evaluator test needs another yet). At 6 a runner's kind is set, at 5 it is not: runner_kind is a schema 6 key (store.CallSchemaVersion), so a record before it has a runner digest and no kind (record.go, Execution.RunnerKind).
//   - A runner's masked word is one of shape.MaskedKinds and, at schema 6, its kind one of shape.RunnerKinds; with a runner the word is none or the runner's own kind where it has one (statusMasked and runnerOf take both from the runner group); a runner-less Bash call carries its word, none unless the line hid two runners or a displaced one (masked.go:177-178; runnerDigest, masked_eval.go:34-39); a tool that is not Bash carries no word (deriveCut returns before statusMasked). A line statusMasked could not parse (StatusMasked nil on a shell call) is not built: no evaluator test needs it yet.
//   - A call classed test has masked none and, when it names a kind, a test kind: Derive reclasses a shell line test only through runsTests, which no route to a hidden status reaches (the checks say why).
//   - The execution's runner fields mirror the declaration's (post.go copies them from the shape it derives).
//   - The outcome is one of the Execution outcomes (record.go). A backgrounded call's outcome is ok: post.go reads Backgrounded on the success event only.
//   - ExitCode is set only on the failure event, whose outcome is failed or interrupted, with a known code (post.go parses it before the interrupt check); an exit on an ok call is refused.
//   - A Bash write that names paths is complete unless incomplete says otherwise, and one that names none carries MayWrite alone. This is the schema's written_paths_complete (record_v5.go): no writer fills written_paths yet, so a named Bash write is the schema's shape, not the hook's.
//   - A gate_output is what the call literal says, copied onto the execution. No writer fills gate_output yet (record_v5.go GateOutput is the schema), so a run carrying one is the schema's shape, not the hook's.
//   - A content sha is on an Edit or a Write only, copied onto the execution (post.go hashes the file a named write names; card P5A).
//   - Edit marks are on an Edit or a Write only (archive 10.2.8: file-writing tools; the builder models those two), copied onto the execution. No writer fills edit_marks yet (1.8p), so a run carrying them is the schema's shape, not the hook's; a test that needs marks where no writer would put them sets the execution's field itself.
//   - No field is over the cap the hook would withhold it at (store.CapDeclaration and store.CapExecution, which the hook applies before its write).
//   - A terminal's outcome is one of the Terminal outcomes (record.go).
//   - The session id is its own run directory name (the store's DirName method, over segment): the builder does not digest. DirName reads no Store field (store.go:453, segment), which is what lets a zero Store answer; no test pins that.
//   - A location is "", or "d:" or "u:" and a name: the two digest domains hook.Locate writes, on any tool, since it locates every call.
//   - The run carries one coverage record, PhasePost and verified, as a hook writes one when its entry and its probe both resolve (hook.BuildCoverage); every real run has one. A test that needs others sets r.Coverage.
//
// The cap functions guarantee caps and nothing else. The derived fields
// (VerbClass, RunnerDigest, RunnerKind, StatusMasked, MayWrite) are what the
// call literal says, not what shape.Derive would derive from any command;
// CWDDigest is the call's loc, a literal, not a digest; and Hosts, SSHHosts
// and FileLabel are never set, since the hook computes them before its cap and
// the builder does not.
func runOfSession(t testing.TB, sessionID string, calls ...call) *store.Run {
	t.Helper()
	var zero store.Store
	if zero.DirName(sessionID) != sessionID {
		t.Fatalf("runOf: session id %q is not its own run directory name: the store digests it (Store.DirName), and the builder does not digest", sessionID)
	}
	r := &store.Run{Dir: sessionID}
	var last int64
	// next numbers the records as the store does, one Seq per record from 1,
	// in the hook's order for one call (pinned end to end by
	// TestHookOrder_DeclarationTerminalThenExecution in internal/hook): its
	// PreToolUse records (the declaration, then the terminal Close writes)
	// before its PostToolUse record (the execution). Records of other calls
	// interleaving in a real run are not modelled.
	next := int64(1)
	var spillE []store.Execution
	var spillT []store.Terminal
	for i, c := range calls {
		if c.seq <= last {
			t.Fatalf("runOf: call %d seq %d does not increase past %d (store: Seq strictly increases)", i, c.seq, last)
		}
		last = c.seq
		if (c.tool == "Edit" || c.tool == "Write") && len(c.paths) == 0 {
			t.Fatalf("runOf: call %d is %s with no path", i, c.tool)
		}
		if c.tool != "Bash" {
			if c.runner != "" {
				t.Fatalf("runOf: call %d names runner %q on %s (store: runner fields are null on a tool that is not a shell)", i, c.runner, c.tool)
			}
			if c.bg {
				t.Fatalf("runOf: call %d is backgrounded on %s (store: Backgrounded is false on every tool but a shell)", i, c.tool)
			}
			if c.mayWrite {
				t.Fatalf("runOf: call %d has mayWrite on %s (may_write is a shell-line property)", i, c.tool)
			}
			if c.tool != "Edit" && c.tool != "Write" && len(c.paths) > 0 {
				t.Fatalf("runOf: call %d names paths on %s (only Edit, Write and a Bash write name paths; shape: MayWrite is false on a tool that is not a shell)", i, c.tool)
			}
			if c.masked != "" {
				t.Fatalf("runOf: call %d has masked word %q on %s (shape: deriveCut returns before statusMasked unless the tool's class is execute and its input carries a command field)", i, c.masked, c.tool)
			}
		}
		if (c.runner != "" || c.masked != "") && !slices.Contains(shape.MaskedKinds(), c.masked) {
			t.Fatalf("runOf: call %d masked %q is not one of %v", i, c.masked, shape.MaskedKinds())
		}
		schema := c.schema
		if schema == 0 {
			schema = store.SchemaVersion
		}
		if schema < 5 || schema > 8 {
			t.Fatalf("runOf: call %d schema %d is not 5, 6, 7 or 8 (the builder stamps those four; store.AcceptsKind reads 1 to 8)", i, c.schema)
		}
		if (c.runnerName != "" || c.maskedBy != "") && (schema < 8 || c.runner == "") {
			t.Fatalf("runOf: call %d names a runner or a masking stage at schema %d with runner %q (runner_name and masked_by are schema 8 keys, set with a runner digest)", i, schema, c.runner)
		}
		if c.runnerName != "" && !slices.Contains(shape.RunnerNames(), c.runnerName) {
			t.Fatalf("runOf: call %d runner name %q is not a row of the runner lists", i, c.runnerName)
		}
		if c.maskedBy != "" && !slices.Contains(shape.MaskedByWords(), c.maskedBy) {
			t.Fatalf("runOf: call %d masked_by %q is not one of %v", i, c.maskedBy, shape.MaskedByWords())
		}
		if c.runner != "" && schema >= 6 && !slices.Contains(shape.RunnerKinds(), c.kind) {
			t.Fatalf("runOf: call %d runner kind %q is not one of %v", i, c.kind, shape.RunnerKinds())
		}
		if schema < 6 && c.kind != "" {
			t.Fatalf("runOf: call %d has runner kind %q at schema %d (store: runner_kind is a schema 6 key)", i, c.kind, schema)
		}
		if c.loc != "" && !strings.HasPrefix(c.loc, "d:") && !strings.HasPrefix(c.loc, "u:") {
			t.Fatalf("runOf: call %d location %q is neither \"d:\" (read) nor \"u:\" (unread) and a name (hook.Locate: shape.CWDDigest or shape.UnreadCWDDigest)", i, c.loc)
		}
		if c.runner == "" && c.kind != "" {
			t.Fatalf("runOf: call %d has runner kind %q and no runner (shape: RunnerKind is set exactly when RunnerDigest is)", i, c.kind)
		}
		if c.runner != "" && c.kind != "" && c.masked != shape.MaskedNone &&
			!(c.masked == shape.MaskedTest && c.kind == shape.RunnerTest) && !(c.masked == shape.MaskedBuild && c.kind == shape.RunnerBuild) {
			t.Fatalf("runOf: call %d masked %q with runner kind %q: with a runner the masked word is none or the runner's own kind (shape: statusMasked and runnerOf take both from one runner group)", i, c.masked, c.kind)
		}
		// Both refusals below follow from one invariant of Derive, not from a
		// list of cases: the class is test only when the line, past the
		// leading cd steps the program search folds, is one whole simple
		// command (wholeCommand) whose program position holds a testCommands
		// row (runsTests, through runnerOn, the matcher the runner grouping
		// reads too). That command's status is the line's, so status_masked is
		// none or null, never a hidden kind; and a runner grouped there is
		// grouped as test. The program search takes a leading `!` or a
		// compound's keyword (`if`) as the program, so those lines are never
		// test. The converse is not claimed. The proof leans on wholeCommand
		// being conservative: commandEnd ends a command at `&` and at every
		// other separator, and wholeCommand refuses one with anything after
		// it, so a change to commandEnd walks this invariant again. Not tied
		// here: the class against bg (the input and the response are two
		// sources), and class test against a runner being recorded: a
		// test-class line records no runner kind past shape.MaxLeadingCd
		// leading cd steps, which the program search folds and the masked
		// parser does not (65 `cd a && ` steps before `go test ./...`,
		// RES-10), or where the masked parser cannot read the line at all (a
		// stray `)`).
		if c.class == shape.VerbTest && c.masked != "" && c.masked != shape.MaskedNone {
			t.Fatalf("runOf: call %d is class %q with masked %q: a hidden status never reaches the test class (shape: runsTests, wholeCommand and the program search)", i, c.class, c.masked)
		}
		if c.class == shape.VerbTest && c.kind != "" && c.kind != shape.RunnerTest {
			t.Fatalf("runOf: call %d is class %q with runner kind %q: the test class needs a test command at the program position, which the runner grouping kinds test (shape: runsTests over testCommands)", i, c.class, c.kind)
		}
		if c.outcome != store.ExecOK && c.outcome != store.ExecFailed && c.outcome != store.ExecInterrupted {
			t.Fatalf("runOf: call %d outcome %q is not one of %q, %q and %q (store: Execution outcomes)", i, c.outcome, store.ExecOK, store.ExecFailed, store.ExecInterrupted)
		}
		if c.bg && c.outcome != store.ExecOK {
			t.Fatalf("runOf: call %d is backgrounded with outcome %q (post.go: Backgrounded is read on the success event only, whose outcome is ok)", i, c.outcome)
		}
		if c.exit != 0 && c.outcome == store.ExecOK {
			t.Fatalf("runOf: call %d has exit code %d with outcome %q (post.go: ExitCode is parsed on the failure event only, whose outcome is failed or interrupted)", i, c.exit, c.outcome)
		}
		if c.term != "" && c.term != store.OutcomeOK && c.term != store.OutcomeError && c.term != store.OutcomeSignal {
			t.Fatalf("runOf: call %d terminal outcome %q is not one of %q, %q and %q (store: Terminal outcomes)", i, c.term, store.OutcomeOK, store.OutcomeError, store.OutcomeSignal)
		}
		if c.sha != "" && c.tool != "Edit" && c.tool != "Write" {
			t.Fatalf("runOf: call %d carries a content sha on %s (post.go: only a named write's post hashes the file it names; the builder models Edit and Write)", i, c.tool)
		}
		if c.editMarks != nil && c.tool != "Edit" && c.tool != "Write" {
			t.Fatalf("runOf: call %d carries edit marks on %s (archive 10.2.8: edit_marks are a file-writing tool's; the builder models Edit and Write)", i, c.tool)
		}
		if c.incomplete && (c.tool != "Bash" || len(c.paths) == 0) {
			t.Fatalf("runOf: call %d is incomplete but is not a named Bash write (written_paths_complete describes a Bash write's named targets; the unnamed write carries MayWrite alone)", i)
		}
		if (c.spilled || c.lostDeclaration) && c.term == "" {
			t.Fatalf("runOf: call %d is spilled or lost its declaration and has no terminal: a missing terminal is the uncontrolled exit's signature, a different one", i)
		}
		if c.lostDeclaration && c.term != store.OutcomeError {
			t.Fatalf("runOf: call %d lost its declaration with terminal outcome %q (Handler.Close writes that terminal as %q with reason %q only)", i, c.term, store.OutcomeError, store.ReasonLockTimeout)
		}
		id := "t" + strconv.FormatInt(c.seq, 10)
		dseq, tseq := next, int64(0)
		if !c.lostDeclaration {
			next++
		}
		if c.term != "" && !c.spilled && !c.lostDeclaration {
			tseq = next
			next++
		}
		eseq := next
		if !c.spilled {
			next++
		}
		d := store.Declaration{Type: store.TypeDeclaration, SchemaVersion: schema, Seq: dseq, ToolUseID: id, SessionID: sessionID, ToolName: c.tool, CWDDigest: c.loc}
		if c.agent != "" {
			a, at := c.agent, "fixture"
			d.AgentID, d.AgentType = &a, &at
		}
		d.Shape.VerbClass = c.class
		if c.prog != "" {
			p := c.prog
			d.Shape.Program = &p
		}
		if c.tool == "Bash" {
			// statusMasked gives a word on every line it parses, none when
			// no runner runs on it.
			m := c.masked
			if m == "" {
				m = shape.MaskedNone
			}
			d.Shape.StatusMasked = &m
		}
		if c.runner != "" {
			rd := "r:" + c.runner
			d.Shape.RunnerDigest = &rd
			if c.kind != "" {
				k := c.kind
				d.Shape.RunnerKind = &k
			}
		}
		if c.tool == "Edit" || c.tool == "Write" {
			p := c.paths[0]
			d.FilePath = &p
		} else if len(c.paths) > 0 {
			// Complete when every write-capable segment yielded a target
			// (record_v5.go WrittenPathsComplete).
			complete := !c.incomplete
			d.WrittenPaths, d.WrittenPathsComplete = append([]string(nil), c.paths...), &complete
			d.Shape.MayWrite = true
		} else if c.mayWrite {
			d.Shape.MayWrite = true // a Bash write with no known target: the unnamed-write case
		}
		probe := d
		if store.CapDeclaration(&probe) {
			t.Fatalf("runOf: call %d: the hook would withhold a field over its cap (store.CapDeclaration): %+v", i, c)
		}
		e := store.Execution{Type: store.TypeExecution, SchemaVersion: schema, Seq: &eseq, ToolUseID: id, SessionID: sessionID, ToolName: c.tool, Outcome: c.outcome, Backgrounded: c.bg}
		e.RunnerDigest, e.StatusMasked, e.RunnerKind = d.Shape.RunnerDigest, d.Shape.StatusMasked, d.Shape.RunnerKind
		e.RulesVersion = c.rules
		if c.runnerName != "" {
			name := c.runnerName
			e.RunnerName = &name
		}
		if c.maskedBy != "" {
			by := c.maskedBy
			e.MaskedBy = &by
		}
		if c.exit != 0 {
			// The refusal above keeps an exit code to the failure event's two
			// outcomes, failed and interrupted.
			ex := c.exit // store.Execution: null on success, null on a failure without a code; never 0
			e.ExitCode = &ex
		}
		if c.gateOutput != nil {
			g := *c.gateOutput
			if g.Reason != nil {
				reason := *g.Reason
				g.Reason = &reason
			}
			e.GateOutput = &g // a fresh copy, never the caller's
		}
		if c.sha != "" {
			sha := c.sha
			e.ContentSHA256 = &sha
		}
		if c.editMarks != nil {
			m := *c.editMarks
			m.Added, m.Removed = maps.Clone(m.Added), maps.Clone(m.Removed)
			e.EditMarks = &m // a fresh copy, never the caller's
		}
		// Every field CapExecution caps that the builder sets (ToolUseID,
		// ToolName) is capped the same on the declaration, probed first, so
		// this probe refuses nothing yet: it guards the execution-only fields
		// the builder does not set today (ExecutedCommand and its cut and
		// masked fields, HeadAfter.Root, HarnessToolName). Dropping it is an
		// equivalent mutant today; when one is added, add its twin rows and
		// this mutant dies.
		probeE := e
		if store.CapExecution(&probeE) {
			t.Fatalf("runOf: call %d: the hook would withhold a field over its cap (store.CapExecution): %+v", i, c)
		}
		if !c.lostDeclaration {
			r.Declarations = append(r.Declarations, d)
		}
		if c.spilled {
			e.Seq = nil
			spillE = append(spillE, e)
		} else {
			r.Executions = append(r.Executions, e)
		}
		if c.term != "" {
			term := store.Terminal{Type: store.TypeTerminal, SchemaVersion: schema, Seq: &tseq,
				ToolUseID: id, SessionID: sessionID, Outcome: c.term, TerminalV5: store.TerminalV5{Writer: "fixture"}}
			if c.lostDeclaration {
				reason := store.ReasonLockTimeout
				term.Reason = &reason
			}
			if c.spilled || c.lostDeclaration {
				term.Seq = nil
				spillT = append(spillT, term)
			} else {
				r.Terminals = append(r.Terminals, term)
			}
		}
	}
	// ReadRun reads records.ndjson before the spill file.
	r.Executions = append(r.Executions, spillE...)
	r.Terminals = append(r.Terminals, spillT...)
	cov := postCoverage(store.StateVerified, nil, store.EntryPresentSettings, store.ProbeFresh)
	cov.SessionID = sessionID
	r.Coverage = []store.Coverage{cov}
	return r
}

// postCoverage is a PhasePost coverage record of runOf's session, as
// hook.BuildCoverage writes one, with the four fields recording reads given by
// hand: no default, so no row reads verified by accident.
func postCoverage(state string, reason *string, entry, probe string) store.Coverage {
	return store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion, SessionID: "s1",
		Phase: store.PhasePost, State: state, Reason: reason, HookEntry: entry, Probe: probe}
}

func intOr0[T int | int64](p *T) T {
	if p == nil {
		return 0
	}
	return *p
}

// samePtr reports whether a and b are both nil or point at equal values.
func samePtr[T comparable](a, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// gateKind and gateReason read a GateOutput's Kind and Reason, empty for nil.
func gateKind(g *store.GateOutput) string {
	if g == nil {
		return ""
	}
	return g.Kind
}
func gateReason(g *store.GateOutput) string {
	if g == nil || g.Reason == nil {
		return ""
	}
	return *g.Reason
}

// sameGateOutput is samePtr for a GateOutput, whose Reason is itself a pointer.
func sameGateOutput(a, b *store.GateOutput) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Kind == b.Kind && a.Parser == b.Parser && samePtr(a.Reason, b.Reason))
}

func TestFixture_RunOfPairsEveryDeclarationWithItsExecution(t *testing.T) {
	failed := gateCall(3, "go-test", store.ExecFailed)
	failed.term = store.OutcomeError
	bgRun := gateCall(4, "go-test", store.ExecOK)
	bgRun.bg = true
	calls := []call{
		edit(1, "a.go"),
		gateCall(2, "go-test", store.ExecOK),
		failed,
		bgRun,
		call{seq: 5, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-test", kind: shape.RunnerTest, masked: shape.MaskedTest, outcome: store.ExecOK},
		call{seq: 6, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"w.go"}},
		call{seq: 7, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", kind: shape.RunnerBuild, masked: shape.MaskedBuild, outcome: store.ExecOK},
		bashWrite(8, "z.go"),
	}
	// Written out, not computed from the counter's rule: each call's
	// declaration, then its terminal when it has one, then its execution.
	// Call 3's records are 5, 6 and 7.
	wantDecl := []int64{1, 3, 5, 8, 10, 12, 14, 16}
	wantExec := []int64{2, 4, 7, 9, 11, 13, 15, 17}
	wantTerm := []int64{6}
	r := runOf(t, calls...)
	if len(r.Declarations) != len(calls) || len(r.Executions) != len(calls) {
		t.Fatalf("builder mispaired: %d declarations and %d executions, want %d of each", len(r.Declarations), len(r.Executions), len(calls))
	}
	for i, c := range calls {
		d, e := r.Declarations[i], r.Executions[i]
		if d.Seq != wantDecl[i] {
			t.Fatalf("call %d: declaration Seq %d, want %d", i, d.Seq, wantDecl[i])
		}
		if e.Seq == nil {
			t.Fatalf("call %d: execution has no Seq", i)
		} else if *e.Seq != wantExec[i] {
			t.Fatalf("call %d: execution Seq %d, want %d", i, *e.Seq, wantExec[i])
		}
		if d.ToolUseID != e.ToolUseID {
			t.Fatalf("call %d: ToolUseID %q on the declaration, %q on the execution", i, d.ToolUseID, e.ToolUseID)
		}
		if d.SessionID != e.SessionID || d.SessionID != r.Dir {
			t.Fatalf("call %d: SessionID %q on the declaration, %q on the execution, Dir %q", i, d.SessionID, e.SessionID, r.Dir)
		}
		if d.ToolName != c.tool || e.ToolName != c.tool {
			t.Fatalf("call %d: ToolName %q on the declaration, %q on the execution, want %q", i, d.ToolName, e.ToolName, c.tool)
		}
		if e.Outcome != c.outcome {
			t.Fatalf("call %d: Outcome %q, want %q", i, e.Outcome, c.outcome)
		}
		if e.Backgrounded != c.bg {
			t.Fatalf("call %d: Backgrounded %t, want %t", i, e.Backgrounded, c.bg)
		}
		if d.Shape.VerbClass != c.class {
			t.Fatalf("call %d: VerbClass %q, want %q", i, d.Shape.VerbClass, c.class)
		}
		// case order is load-bearing: the nil case runs before the dereference.
		switch p := d.Shape.Program; {
		case c.prog == "" && p != nil:
			t.Fatalf("call %d: Program %q, want none", i, *p)
		case c.prog != "" && p == nil:
			t.Fatalf("call %d: no Program, want %q", i, c.prog)
		case c.prog != "" && *p != c.prog:
			t.Fatalf("call %d: Program %q, want %q", i, *p, c.prog)
		}
		if c.tool == "Edit" || c.tool == "Write" {
			if d.FilePath == nil {
				t.Fatalf("call %d: %s has no FilePath", i, c.tool)
			} else if *d.FilePath != c.paths[0] {
				t.Fatalf("call %d: FilePath %q, want %q", i, *d.FilePath, c.paths[0])
			}
			if d.WrittenPaths != nil || d.Shape.MayWrite {
				t.Fatalf("call %d: %s carries WrittenPaths %v and MayWrite %t, want neither", i, c.tool, d.WrittenPaths, d.Shape.MayWrite)
			}
		} else if d.FilePath != nil {
			t.Fatalf("call %d: FilePath %q on %s, want none", i, *d.FilePath, c.tool)
		}
		dm, em := d.Shape.StatusMasked, e.StatusMasked
		wantMasked := c.masked
		if wantMasked == "" {
			wantMasked = shape.MaskedNone // a runner-less Bash call: statusMasked's word for a line with no runner
		}
		if c.tool != "Bash" {
			if dm != nil || em != nil {
				t.Fatalf("call %d: StatusMasked set %t on the declaration and %t on the execution of %s, want neither", i, dm != nil, em != nil, c.tool)
			}
		} else if dm == nil || em == nil {
			t.Fatalf("call %d: StatusMasked set %t on the declaration and %t on the execution, want both", i, dm != nil, em != nil)
		} else if *dm != wantMasked || *em != wantMasked {
			t.Fatalf("call %d: StatusMasked %q on the declaration, %q on the execution, want %q", i, *dm, *em, wantMasked)
		}
		dk, ek := d.Shape.RunnerKind, e.RunnerKind
		if c.runner == "" {
			if dk != nil || ek != nil {
				t.Fatalf("call %d: RunnerKind set %t on the declaration and %t on the execution of a non-runner, want neither", i, dk != nil, ek != nil)
			}
		} else if dk == nil || ek == nil {
			t.Fatalf("call %d: RunnerKind set %t on the declaration and %t on the execution, want both", i, dk != nil, ek != nil)
		} else if *dk != c.kind || *ek != c.kind {
			t.Fatalf("call %d: RunnerKind %q on the declaration, %q on the execution, want %q", i, *dk, *ek, c.kind)
		}
	}
	if len(r.Terminals) != len(wantTerm) {
		t.Fatalf("built %d terminals, want %d", len(r.Terminals), len(wantTerm))
	}
	for j, term := range r.Terminals {
		if term.Seq == nil {
			t.Fatalf("terminal %d has no Seq", j)
		} else if *term.Seq != wantTerm[j] {
			t.Fatalf("terminal %d: Seq %d, want %d", j, *term.Seq, wantTerm[j])
		}
	}
	if r.Executions[1].RunnerDigest == nil {
		t.Fatal("gate execution has no RunnerDigest")
	}
	// The rows compare a record against its own call's kind, so gateCall's
	// default is pinned here or nowhere.
	if k := r.Executions[1].RunnerKind; k == nil || *k != shape.RunnerTest {
		t.Fatalf("gateCall's execution: RunnerKind set %t, want %q", k != nil, shape.RunnerTest)
	}
	if e := r.Executions[0]; e.RunnerDigest != nil || e.StatusMasked != nil || e.RunnerKind != nil || e.ExitCode != nil {
		t.Fatalf("edit execution carries runner fields: RunnerDigest set %t, StatusMasked set %t, RunnerKind set %t, ExitCode set %t", e.RunnerDigest != nil, e.StatusMasked != nil, e.RunnerKind != nil, e.ExitCode != nil)
	}
}

// RunnerDigest is the gate family key (D2) that Task 1.3 keys families on: one
// runner word gives one digest and two words give two. This tests that
// property, not the "r:" spelling the builder happens to use.
func TestFixture_RunnerDigestIsOneFamilyPerRunnerWord(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), gateCall(2, "go-vet", store.ExecOK), gateCall(3, "go-test", store.ExecOK))
	digests := make([]string, len(r.Executions))
	for i, e := range r.Executions {
		if e.RunnerDigest == nil {
			t.Fatalf("call %d: no RunnerDigest", i)
		}
		digests[i] = *e.RunnerDigest
	}
	if len(digests) != 3 || digests[0] != digests[2] || digests[0] == digests[1] {
		t.Fatalf("digests %q: want the first and third equal and the second different", digests)
	}
}

func TestFixture_FailedGateRunCarriesItsCodeAndOKCarriesNone(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), gateCall(2, "go-test", store.ExecFailed), call{seq: 3, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecFailed}, call{seq: 4, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecFailed, exit: 2}, call{seq: 5, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecInterrupted, exit: 130})
	if r.Executions[0].ExitCode != nil {
		t.Fatalf("ok run carries an exit code: %d", *r.Executions[0].ExitCode)
	}
	if e := r.Executions[1].ExitCode; e == nil || *e != 1 {
		t.Fatalf("failed run should carry exit 1, got set %t value %d", e != nil, intOr0(e))
	}
	if r.Executions[2].ExitCode != nil {
		t.Fatalf("failure with an unknown code must carry nil, got %d", *r.Executions[2].ExitCode)
	}
	if e := r.Executions[3].ExitCode; e == nil || *e != 2 {
		t.Fatalf("failed run with exit 2 should carry 2, got set %t value %d", e != nil, intOr0(e))
	}
	// The interrupted outcome comes from the same failure event, with the code
	// parsed before the interrupt check (post.go), so it carries the code too.
	if e := r.Executions[4].ExitCode; e == nil || *e != 130 {
		t.Fatalf("interrupted run with exit 130 should carry 130, got set %t value %d", e != nil, intOr0(e))
	}
}

func TestFixture_BashWriteIsCompleteAndUnnamedWriteHasNoPaths(t *testing.T) {
	r := runOf(t, bashWrite(1, "a.go", "b.go"), call{seq: 2, tool: "Bash", prog: "sh", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true})
	if d := r.Declarations[0]; len(d.WrittenPaths) != 2 || d.WrittenPathsComplete == nil || !*d.WrittenPathsComplete || !d.Shape.MayWrite {
		t.Fatalf("named bash write: paths %v complete set %t value %t mayWrite %t", d.WrittenPaths, d.WrittenPathsComplete != nil, d.WrittenPathsComplete != nil && *d.WrittenPathsComplete, d.Shape.MayWrite)
	}
	if d := r.Declarations[1]; d.WrittenPaths != nil || d.WrittenPathsComplete != nil || !d.Shape.MayWrite {
		t.Fatalf("unnamed write: paths %v complete set %t value %t mayWrite %t", d.WrittenPaths, d.WrittenPathsComplete != nil, d.WrittenPathsComplete != nil && *d.WrittenPathsComplete, d.Shape.MayWrite)
	}
}

func TestFixture_BuiltRecordsDoNotAliasTheCallersPaths(t *testing.T) {
	paths := []string{"a.go"}
	r := runOf(t, bashWrite(1, paths...))
	paths[0] = "changed.go"
	if d := r.Declarations[0]; len(d.WrittenPaths) != 1 || d.WrittenPaths[0] != "a.go" {
		t.Fatalf("record follows the caller's slice or lost the path: %v", d.WrittenPaths)
	}
}

func TestFixture_BuiltRecordsDoNotAliasTheCallersGateOutput(t *testing.T) {
	reason := "budget"
	out := &store.GateOutput{Kind: "unread", Parser: "go", Reason: &reason}
	c := gateCall(1, "go-test", store.ExecOK)
	c.gateOutput = out
	r := runOf(t, c)
	out.Kind, reason = "summary_pass", "decode"
	if g := r.Executions[0].GateOutput; g == nil || g.Kind != "unread" || g.Reason == nil || *g.Reason != "budget" {
		t.Fatalf("record follows the caller's gate_output: set %t, kind %q, reason %q", g != nil, gateKind(g), gateReason(g))
	}
}

func TestFixture_BuiltRecordsDoNotAliasTheCallersMarks(t *testing.T) {
	added, removed := map[string]int{"go.t_skip": 1}, map[string]int{"go.test_def": 1}
	r := runOf(t, edit(1, "a_test.go").marks(added, removed), call{seq: 2, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"b_test.go"}}.marks(added, nil))
	added["go.t_skip"], removed["go.test_def"] = 9, 9
	if m := r.Executions[0].EditMarks; m == nil || m.Added["go.t_skip"] != 1 || m.Removed["go.test_def"] != 1 || m.Basis != "edit_delta" || m.Version != 1 {
		t.Fatalf("the edit's record follows the caller's maps or lost its marks: %+v", m)
	}
	if m := r.Executions[1].EditMarks; m == nil || m.Added["go.t_skip"] != 1 || m.Removed != nil || m.Basis != "write_vs_before" {
		t.Fatalf("the write's record follows the caller's map, or its basis is not write_vs_before: %+v", m)
	}
}

// fakeTB records Fatalf and unwinds by panicking with itself, so a test can
// prove that runOf rejects an impossible run. runOf may call only Helper and
// Fatalf on its TB; the embedded nil TB makes any other call a loud failure.
type fakeTB struct {
	testing.TB
	fatal string
}

func (f *fakeTB) Helper() {}
func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)
	panic(f)
}

// TestFixture_RunOfRejectsRunsTheHookNeverWrites builds each row through a
// fakeTB, whose Fatalf panics: that panic is what makes a refusal observable,
// since runOf never returns past it. A row with a want breaks exactly one
// rule, and runOf reports the first rule a call breaks; a row without one is
// the accepted twin at a boundary.
func TestFixture_RunOfRejectsRunsTheHookNeverWrites(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	numbered := func(n int) []string {
		ps := make([]string, n)
		for i := range ps {
			ps[i] = fmt.Sprintf("p%d.go", i)
		}
		return ps
	}
	// The written-path cap counts entries and bytes in all, nothing per path
	// (limits.go, writtenPathsFit), so the byte twins split the total in two.
	half := store.MaxWrittenPathsBytes / 2
	errEdit := edit(1, "a.go")
	errEdit.term = store.OutcomeError
	closedEdit := edit(1, "a.go")
	closedEdit.term = "closed"
	incompleteWrite := bashWrite(1, "a.go")
	incompleteWrite.incomplete = true
	incompleteEdit := edit(1, "a.go")
	incompleteEdit.incomplete = true
	gate := func(class, masked, kind, outcome string, exit int, bg bool) call {
		return call{seq: 1, tool: "Bash", prog: "go", class: class, runner: "go-test", kind: kind, masked: masked, outcome: outcome, exit: exit, bg: bg}
	}
	spilledGate := func(term string) call {
		c := gateCall(1, "go-test", store.ExecOK)
		c.spilled, c.term = true, term
		return c
	}
	lostEdit := func(term string) call {
		c := edit(1, "a.go")
		c.lostDeclaration, c.term = true, term
		return c
	}
	cases := []struct {
		name    string
		session string
		calls   []call
		want    string // a substring of the refusal; empty for a run runOf must accept
	}{
		{"duplicate seq", "s1", []call{edit(2, "a.go"), edit(2, "b.go")}, "seq"},
		{"decreasing seq", "s1", []call{edit(3, "a.go"), edit(1, "b.go")}, "seq"},
		{"pathless edit", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK}}, "no path"},
		{"runner on edit", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, paths: []string{"a.go"}}}, "not a shell"},
		{"bg on edit", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, bg: true, paths: []string{"a.go"}}}, "every tool but a shell"},
		{"mayWrite on edit", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, mayWrite: true, paths: []string{"a.go"}}}, "shell-line"},
		{"runner with bad masked", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, outcome: store.ExecOK}}, "masked \"\" is not one of"},
		{"runner with bad kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: "lint", masked: shape.MaskedNone, outcome: store.ExecOK}}, "runner kind \"lint\" is not one of"},
		{"kind without runner", "s1", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, kind: shape.RunnerTest, outcome: store.ExecOK}}, "and no runner"},
		{"paths on read", "s1", []call{call{seq: 1, tool: "Read", class: shape.VerbRead, outcome: store.ExecOK, paths: []string{"a.go"}}}, "names paths"},

		{"file path at MaxPathBytes", "s1", []call{edit(1, long(store.MaxPathBytes))}, ""},
		{"file path one byte over", "s1", []call{edit(1, long(store.MaxPathBytes+1))}, "store.CapDeclaration"},
		{"written paths at MaxWrittenPathsBytes", "s1", []call{bashWrite(1, long(half), long(store.MaxWrittenPathsBytes-half))}, ""},
		{"written paths one byte over", "s1", []call{bashWrite(1, long(half), long(store.MaxWrittenPathsBytes-half+1))}, "store.CapDeclaration"},
		{"MaxWrittenPaths written paths", "s1", []call{bashWrite(1, numbered(store.MaxWrittenPaths)...)}, ""},
		{"one written path too many", "s1", []call{bashWrite(1, numbered(store.MaxWrittenPaths+1)...)}, "store.CapDeclaration"},

		// The store keeps a session id of up to 128 bytes of [A-Za-z0-9._-] as
		// its run directory's name and digests any other (store.go, segment).
		{"session s2", "s2", []call{edit(1, "a.go")}, ""},
		{"session of 128 bytes", long(128), []call{edit(1, "a.go")}, ""},
		{"session of 129 bytes", long(129), []call{edit(1, "a.go")}, "session id"},
		{"empty session", "", []call{edit(1, "a.go")}, "session id"},
		{"dot session", ".", []call{edit(1, "a.go")}, "session id"},
		{"session with a slash", "a/b", []call{edit(1, "a.go")}, "session id"},
		{"session with a space", "a b", []call{edit(1, "a.go")}, "session id"},

		{"error terminal", "s1", []call{errEdit}, ""},
		{"terminal word outside the three", "s1", []call{closedEdit}, "terminal outcome \"closed\""},
		{"incomplete named write", "s1", []call{incompleteWrite}, ""},
		{"incomplete write naming no paths", "s1", []call{call{seq: 1, tool: "Bash", prog: "sh", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true, incomplete: true}}, "not a named Bash write"},
		{"incomplete edit", "s1", []call{incompleteEdit}, "not a named Bash write"},
		{"marks on an edit", "s1", []call{edit(1, "a_test.go").marks(map[string]int{"go.t_skip": 1}, nil)}, ""},
		{"marks on a bash write", "s1", []call{bashWrite(1, "a_test.go").marks(map[string]int{"go.t_skip": 1}, nil)}, "edit marks on Bash"},

		// The pairings the hook keeps between fields, each with its accepted twin.
		// A hidden status keeps go's base class, package (a stage after the
		// runner is never reclassed test).
		{"masked test on a build runner", "s1", []call{gate(shape.VerbPackage, shape.MaskedTest, shape.RunnerBuild, store.ExecOK, 0, false)}, "the runner's own kind"},
		{"masked build on a build runner", "s1", []call{gate(shape.VerbPackage, shape.MaskedBuild, shape.RunnerBuild, store.ExecOK, 0, false)}, ""},
		{"backgrounded failure", "s1", []call{gate(shape.VerbTest, shape.MaskedNone, shape.RunnerTest, store.ExecFailed, 0, true)}, "backgrounded"},
		{"backgrounded success", "s1", []call{gate(shape.VerbTest, shape.MaskedNone, shape.RunnerTest, store.ExecOK, 0, true)}, ""},
		{"exit code on success", "s1", []call{gate(shape.VerbTest, shape.MaskedNone, shape.RunnerTest, store.ExecOK, 3, false)}, "exit code 3"},
		{"exit code on failure", "s1", []call{gate(shape.VerbTest, shape.MaskedNone, shape.RunnerTest, store.ExecFailed, 3, false)}, ""},
		{"exit code on interrupt", "s1", []call{gate(shape.VerbTest, shape.MaskedNone, shape.RunnerTest, store.ExecInterrupted, 130, false)}, ""},
		{"outcome outside the three", "s1", []call{gate(shape.VerbTest, shape.MaskedNone, shape.RunnerTest, "done", 0, false)}, "outcome \"done\""},
		{"class test with masked test", "s1", []call{gate(shape.VerbTest, shape.MaskedTest, shape.RunnerTest, store.ExecOK, 0, false)}, "class \"test\" with masked"},
		{"class package with masked test", "s1", []call{gate(shape.VerbPackage, shape.MaskedTest, shape.RunnerTest, store.ExecOK, 0, false)}, ""},
		{"class test with masked test and no runner", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, masked: shape.MaskedTest, outcome: store.ExecOK}}, "class \"test\" with masked"},
		{"class test with a build kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-build", kind: shape.RunnerBuild, masked: shape.MaskedNone, outcome: store.ExecOK}}, "class \"test\" with runner kind"},
		{"class execute with a build kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", kind: shape.RunnerBuild, masked: shape.MaskedNone, outcome: store.ExecOK}}, ""},
		{"masked word on an edit", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, masked: shape.MaskedNone, outcome: store.ExecOK, paths: []string{"a.go"}}}, "masked word"},
		{"masked word on a runner-less Bash call", "s1", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, masked: shape.MaskedNone, outcome: store.ExecOK}}, ""},
		// Two hidden runners, or one a cd displaced: the word without a runner.
		{"masked test on a runner-less Bash call", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbPackage, masked: shape.MaskedTest, outcome: store.ExecOK}}, ""},
		{"runner-less Bash call with a bad masked word", "s1", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, masked: "hidden", outcome: store.ExecOK}}, "masked \"hidden\" is not one of"},

		// A record before runner_kind: schema 5, a runner and no kind.
		{"schema 5 runner without a kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", masked: shape.MaskedNone, outcome: store.ExecOK, schema: 5}}, ""},
		{"schema 5 hidden runner without a kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", masked: shape.MaskedBuild, outcome: store.ExecOK, schema: 5}}, ""},
		{"schema 5 runner with a kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, schema: 5}}, "runner_kind is a schema 6 key"},
		{"schema 6 runner without a kind", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", masked: shape.MaskedNone, outcome: store.ExecOK, schema: 6}}, "runner kind \"\" is not one of"},
		{"schema 4", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"a.go"}, schema: 4}}, "is not 5, 6, 7 or 8"},
		// runner_name and masked_by, schema 8 keys set with a runner.
		{"schema 8 runner name and stage", "s1", []call{call{seq: 1, tool: "Bash", prog: "python3", class: shape.VerbExecute, runner: "pytest", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, schema: 8, runnerName: "pytest", maskedBy: "tail"}}, ""},
		{"runner name before schema 8", "s1", []call{call{seq: 1, tool: "Bash", prog: "python3", class: shape.VerbExecute, runner: "pytest", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, schema: 7, runnerName: "pytest"}}, "schema 8 keys"},
		{"runner name without a runner", "s1", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, outcome: store.ExecOK, runnerName: "pytest"}}, "schema 8 keys"},
		{"runner name outside the lists", "s1", []call{call{seq: 1, tool: "Bash", prog: "python3", class: shape.VerbExecute, runner: "pytest", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, runnerName: "rm -rf"}}, "not a row of the runner lists"},
		{"masking stage outside the words", "s1", []call{call{seq: 1, tool: "Bash", prog: "python3", class: shape.VerbExecute, runner: "pytest", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, runnerName: "pytest", maskedBy: "awk"}}, "masked_by \"awk\" is not one of"},
		// A location in one of the two digest domains, or none.
		{"read location", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"a.go"}, loc: "d:repo"}}, ""},
		{"unread location", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"a.go"}, loc: "u:repo"}}, ""},
		{"location in neither domain", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"a.go"}, loc: "/repo"}}, "neither"},
		// A spilled call, and a lost declaration with the terminal Handler.Close
		// writes for it.
		{"spilled with a terminal", "s1", []call{spilledGate(store.OutcomeOK)}, ""},
		{"spilled without a terminal", "s1", []call{spilledGate("")}, "uncontrolled exit"},
		{"lost declaration with an error terminal", "s1", []call{lostEdit(store.OutcomeError)}, ""},
		{"lost declaration without a terminal", "s1", []call{lostEdit("")}, "uncontrolled exit"},
		{"lost declaration with an ok terminal", "s1", []call{lostEdit(store.OutcomeOK)}, "lock_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTB{}
			called := false
			var r *store.Run
			func() {
				defer func() {
					if r := recover(); r != nil {
						if r != f {
							panic(r)
						}
						called = true
					}
				}()
				r = runOfSession(f, tc.session, tc.calls...)
			}()
			if tc.want == "" {
				if called || r == nil {
					t.Fatalf("refused a run the hook writes: %q", f.fatal)
				}
				return
			}
			if !called || !strings.Contains(f.fatal, tc.want) {
				t.Fatalf("called %t, message %q", called, f.fatal)
			}
		})
	}
}

// TestFixture_RunOfRoundTripsThroughTheStore appends every record of a built
// run to a real store and reads the run back: the run an evaluator test is
// given is the run it would read.
func TestFixture_RunOfRoundTripsThroughTheStore(t *testing.T) {
	gate := gateCall(1, "go-test", store.ExecOK)
	gate.term = store.OutcomeOK
	// An Edit that added a skip and a suppression and took an assertion away
	// (archive 10.2.8): the field 1.8 reads, which no writer fills yet, read
	// back as built (REC caps: a few ids are well inside MaxRecordBytes).
	ed := edit(2, "a.go").marks(map[string]int{"go.t_skip": 1, "go.nolint": 2}, map[string]int{"go.assert": 3})
	ed.term = store.OutcomeOK
	complete := bashWrite(3, "b.go", "c.go")
	incomplete := bashWrite(4, "d.go")
	incomplete.incomplete, incomplete.term = true, store.OutcomeError
	// A `go test ./... | tail` whose status the pipe hides, reported
	// backgrounded by the tool response, whose PreToolUse hook was cancelled
	// by timeout after its declaration landed (a signal terminal) and whose
	// tool ran anyway.
	hidden := gateCall(5, "go-test", store.ExecOK)
	hidden.bg, hidden.masked, hidden.kind, hidden.term = true, shape.MaskedTest, shape.RunnerTest, store.OutcomeSignal
	hidden.class = shape.VerbPackage // a stage after the runner is never reclassed test (wholeCommand), so go's base class stays
	// A `go build ./... | tail -1; exit 2`: the build runner's status hidden,
	// the line failing on the failure event with "Exit code 2".
	build := gateCall(6, "go-build", store.ExecFailed)
	build.class, build.exit, build.masked, build.kind = shape.VerbPackage, 2, shape.MaskedBuild, shape.RunnerBuild
	build.gateOutput = &store.GateOutput{Kind: "summary_fail", Parser: "go"}
	// A gate run whose output was not read for want of budget: the one
	// gate_output that carries a Reason (record_v5.go GateOutput).
	reason := "budget"
	unread := gateCall(7, "go-test", store.ExecOK)
	unread.gateOutput = &store.GateOutput{Kind: "unread", Parser: "go", Reason: &reason}
	calls := []call{gate, ed, complete, incomplete, hidden, build, unread}
	r := runOfSession(t, "s2", calls...)

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	terms := make(map[string]store.Terminal, len(r.Terminals))
	for _, term := range r.Terminals {
		terms[term.ToolUseID] = term
	}
	// In the hook's order: the declaration and the terminal from the
	// PreToolUse process, then the execution from the PostToolUse one.
	for i := range calls {
		d, e := r.Declarations[i], r.Executions[i]
		if err := st.AppendDeclaration(d); err != nil {
			t.Fatalf("call %d: AppendDeclaration: %v", i, err)
		}
		if term, ok := terms[d.ToolUseID]; ok {
			if err := st.AppendTerminal(term); err != nil {
				t.Fatalf("call %d: AppendTerminal: %v", i, err)
			}
		}
		if err := st.AppendExecution(e); err != nil {
			t.Fatalf("call %d: AppendExecution: %v", i, err)
		}
	}
	got, err := st.ReadRun("s2")
	if err != nil {
		t.Fatal(err)
	}

	if got.Skipped != 0 || got.Ignored != 0 {
		t.Fatalf("read back with Skipped %d and Ignored %d, want 0 and 0: the store could not read a record as built", got.Skipped, got.Ignored)
	}
	// The store files a record by its SessionID, so Dir is compared on both
	// sides: a built Dir that is not the session's would pass got.Dir alone.
	if r.Dir != "s2" || got.Dir != r.Dir {
		t.Fatalf("Dir %q built and %q read back, want %q for both", r.Dir, got.Dir, "s2")
	}
	if len(got.Declarations) != len(r.Declarations) || len(got.Executions) != len(r.Executions) || len(got.Terminals) != len(r.Terminals) {
		t.Fatalf("read back %d declarations, %d executions and %d terminals, built %d, %d and %d",
			len(got.Declarations), len(got.Executions), len(got.Terminals), len(r.Declarations), len(r.Executions), len(r.Terminals))
	}
	if len(r.Terminals) != 4 {
		t.Fatalf("built %d terminals, want 4: four of the seven calls name a term", len(r.Terminals))
	}
	for i, w := range r.Declarations {
		g := got.Declarations[i]
		if g.ToolUseID != w.ToolUseID {
			t.Fatalf("declaration %d: ToolUseID %q read back, %q built", i, g.ToolUseID, w.ToolUseID)
		}
		for _, f := range []struct {
			name string
			same bool
		}{
			{"FilePath", samePtr(g.FilePath, w.FilePath)},
			{"WrittenPaths", slices.Equal(g.WrittenPaths, w.WrittenPaths)},
			{"WrittenPathsComplete", samePtr(g.WrittenPathsComplete, w.WrittenPathsComplete)},
			{"Shape.VerbClass", g.Shape.VerbClass == w.Shape.VerbClass},
			{"Shape.MayWrite", g.Shape.MayWrite == w.Shape.MayWrite},
			{"Shape.RunnerKind", samePtr(g.Shape.RunnerKind, w.Shape.RunnerKind)},
			{"Shape.RunnerDigest", samePtr(g.Shape.RunnerDigest, w.Shape.RunnerDigest)},
			{"Shape.StatusMasked", samePtr(g.Shape.StatusMasked, w.Shape.StatusMasked)},
		} {
			if !f.same {
				t.Errorf("declaration %d (%s): %s differs after the round trip", i, w.ToolUseID, f.name)
			}
		}
	}
	for i, w := range r.Executions {
		g := got.Executions[i]
		if g.ToolUseID != w.ToolUseID {
			t.Fatalf("execution %d: ToolUseID %q read back, %q built", i, g.ToolUseID, w.ToolUseID)
		}
		for _, f := range []struct {
			name string
			same bool
		}{
			{"Outcome", g.Outcome == w.Outcome},
			{"ExitCode", samePtr(g.ExitCode, w.ExitCode)},
			{"Backgrounded", g.Backgrounded == w.Backgrounded},
			{"RunnerDigest", samePtr(g.RunnerDigest, w.RunnerDigest)},
			{"StatusMasked", samePtr(g.StatusMasked, w.StatusMasked)},
			{"RunnerKind", samePtr(g.RunnerKind, w.RunnerKind)},
			{"GateOutput", sameGateOutput(g.GateOutput, w.GateOutput)},
			{"EditMarks", reflect.DeepEqual(g.EditMarks, w.EditMarks)},
		} {
			if !f.same {
				t.Errorf("execution %d (%s): %s differs after the round trip", i, w.ToolUseID, f.name)
			}
		}
	}
	for i, w := range r.Terminals {
		if g := got.Terminals[i]; g.ToolUseID != w.ToolUseID || g.Outcome != w.Outcome {
			t.Errorf("terminal %d: %s %q read back, %s %q built", i, g.ToolUseID, g.Outcome, w.ToolUseID, w.Outcome)
		}
	}
	// Against the calls, not the built run: a builder that ignored incomplete
	// would build and read back true for both writes.
	if c := got.Declarations[2].WrittenPathsComplete; c == nil || !*c {
		t.Errorf("the complete write reads back WrittenPathsComplete set %t, want true", c != nil)
	}
	if c := got.Declarations[3].WrittenPathsComplete; c == nil || *c {
		t.Errorf("the incomplete write reads back WrittenPathsComplete set %t value %t, want false", c != nil, c != nil && *c)
	}
	readTerm := make(map[string]store.Terminal, len(got.Terminals))
	for _, term := range got.Terminals {
		readTerm[term.ToolUseID] = term
	}
	if e := got.Executions[4]; !e.Backgrounded || e.ExitCode != nil || e.StatusMasked == nil || *e.StatusMasked != shape.MaskedTest {
		t.Errorf("the hidden backgrounded run reads back Backgrounded %t, ExitCode set %t, StatusMasked set %t; want true, nil and %q",
			e.Backgrounded, e.ExitCode != nil, e.StatusMasked != nil, shape.MaskedTest)
	}
	if term, ok := readTerm[got.Declarations[4].ToolUseID]; !ok || term.Outcome != store.OutcomeSignal {
		t.Errorf("the hidden backgrounded run reads back terminal %t outcome %q, want %q", ok, term.Outcome, store.OutcomeSignal)
	}
	if e := got.Executions[5]; e.Backgrounded || e.ExitCode == nil || *e.ExitCode != 2 ||
		e.StatusMasked == nil || *e.StatusMasked != shape.MaskedBuild || e.RunnerKind == nil || *e.RunnerKind != shape.RunnerBuild {
		t.Errorf("the failed build reads back Backgrounded %t, ExitCode %d (set %t), StatusMasked set %t, RunnerKind set %t; want false, 2, %q and %q",
			e.Backgrounded, intOr0(e.ExitCode), e.ExitCode != nil, e.StatusMasked != nil, e.RunnerKind != nil, shape.MaskedBuild, shape.RunnerBuild)
	}
	if g := got.Executions[5].GateOutput; g == nil || g.Kind != "summary_fail" || g.Parser != "go" {
		t.Errorf("the failed build reads back GateOutput set %t, want kind %q parser %q", g != nil, "summary_fail", "go")
	}
	if g := got.Executions[6].GateOutput; g == nil || g.Kind != "unread" || g.Reason == nil || *g.Reason != "budget" {
		t.Errorf("the unread run reads back GateOutput set %t, Reason set %t, want kind %q reason %q", g != nil, g != nil && g.Reason != nil, "unread", "budget")
	}
	// Against the call, not the built run: a builder that dropped the marks
	// would build and read back nil on both sides.
	want := &store.EditMarks{Added: map[string]int{"go.t_skip": 1, "go.nolint": 2}, Removed: map[string]int{"go.assert": 3}, Basis: "edit_delta", Version: 1}
	if m := got.Executions[1].EditMarks; !reflect.DeepEqual(m, want) {
		t.Errorf("the edit reads back EditMarks %+v, want %+v", m, want)
	}
	for i, e := range got.Executions {
		if i != 1 && e.EditMarks != nil {
			t.Errorf("execution %d reads back EditMarks %+v, want none: only the edit carries marks", i, e.EditMarks)
		}
	}

	// Seq is the store's, one per record in append order, and the builder
	// numbers the same order: record for record, built equals read back.
	for i, w := range r.Declarations {
		if g := got.Declarations[i].Seq; g != w.Seq {
			t.Errorf("declaration %d (%s): Seq %d built, %d read back", i, w.ToolUseID, w.Seq, g)
		}
	}
	for i, w := range r.Executions {
		if g := got.Executions[i].Seq; g == nil || w.Seq == nil || *g != *w.Seq {
			t.Errorf("execution %d (%s): Seq %d built (set %t), %d read back (set %t)", i, w.ToolUseID, intOr0(w.Seq), w.Seq != nil, intOr0(g), g != nil)
		}
	}
	for i, w := range r.Terminals {
		if g := got.Terminals[i].Seq; g == nil || w.Seq == nil || *g != *w.Seq {
			t.Errorf("terminal %d (%s): Seq %d built (set %t), %d read back (set %t)", i, w.ToolUseID, intOr0(w.Seq), w.Seq != nil, intOr0(g), g != nil)
		}
	}
}

func TestFixture_TerminalIsBuiltOnlyWhenAsked(t *testing.T) {
	c := gateCall(1, "go-test", store.ExecOK)
	if r := runOf(t, c); len(r.Terminals) != 0 {
		t.Fatalf("a call with no term built %d terminals, want none: a missing terminal is the uncontrolled exit's signature", len(r.Terminals))
	}
	c.term = store.OutcomeOK
	r := runOf(t, c)
	if len(r.Terminals) != 1 {
		t.Fatalf("a call with term %q built %d terminals, want 1", c.term, len(r.Terminals))
	}
	if term := r.Terminals[0]; term.ToolUseID != r.Declarations[0].ToolUseID || term.Outcome != store.OutcomeOK {
		t.Fatalf("terminal %s %q, want %s %q", term.ToolUseID, term.Outcome, r.Declarations[0].ToolUseID, store.OutcomeOK)
	}
}

// TestFixture_SpilledAndLostCallsReadBackAsBuilt writes a run the way the hook
// would under lock load and reads it back through the store: a spilled call's
// execution and terminal go to the spill file (Store.SpillExecution,
// Store.SpillTerminal), and a lost declaration leaves only its spilled
// lock_timeout terminal and an ordered execution (Handler.Close). The run an
// evaluator test is given for either flag is the run ReadRun returns, order,
// Seqs and reasons included, and so is the coverage record every run carries.
func TestFixture_SpilledAndLostCallsReadBackAsBuilt(t *testing.T) {
	spilled := gateCall(2, "go-test", store.ExecOK)
	spilled.spilled, spilled.term = true, store.OutcomeOK
	lost := edit(3, "a.go")
	lost.lostDeclaration, lost.term = true, store.OutcomeError
	sub := edit(4, "b.go")
	sub.agent = "a1"
	calls := []call{gateCall(1, "go-test", store.ExecOK), spilled, lost, sub}
	r := runOfSession(t, "s3", calls...)

	// Written out: t1's records are 1 and 2; t2's declaration is 3 and its
	// other two records are spilled; t3 has no declaration and its execution
	// is 4; t4's records are 5 and 6. The spill file reads last.
	type rec struct {
		id  string
		seq int64 // 0 is a nil Seq
	}
	seqOf := func(p *int64) int64 { return intOr0(p) }
	var decl, exec, term []rec
	for _, d := range r.Declarations {
		decl = append(decl, rec{d.ToolUseID, d.Seq})
	}
	for _, e := range r.Executions {
		exec = append(exec, rec{e.ToolUseID, seqOf(e.Seq)})
	}
	for _, x := range r.Terminals {
		term = append(term, rec{x.ToolUseID, seqOf(x.Seq)})
	}
	wantDecl := []rec{{"t1", 1}, {"t2", 3}, {"t4", 5}}
	wantExec := []rec{{"t1", 2}, {"t3", 4}, {"t4", 6}, {"t2", 0}}
	wantTerm := []rec{{"t2", 0}, {"t3", 0}}
	if !slices.Equal(decl, wantDecl) || !slices.Equal(exec, wantExec) || !slices.Equal(term, wantTerm) {
		t.Fatalf("built declarations %v, executions %v, terminals %v; want %v, %v, %v", decl, exec, term, wantDecl, wantExec, wantTerm)
	}
	if x := r.Terminals[1]; x.Outcome != store.OutcomeError || x.Reason == nil || *x.Reason != store.ReasonLockTimeout {
		t.Fatalf("lost declaration's terminal %q reason %v, want %q with %q", x.Outcome, x.Reason, store.OutcomeError, store.ReasonLockTimeout)
	}
	if d := r.Declarations[2]; d.AgentID == nil || *d.AgentID != "a1" || d.AgentType == nil || r.Declarations[0].AgentID != nil || r.Declarations[0].AgentType != nil {
		t.Fatalf("subagent call AgentID %v AgentType %v, main call %v %v: want both set inside the subagent and both nil outside", d.AgentID, d.AgentType, r.Declarations[0].AgentID, r.Declarations[0].AgentType)
	}

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	decls := make(map[string]store.Declaration)
	for _, d := range r.Declarations {
		decls[d.ToolUseID] = d
	}
	execs := make(map[string]store.Execution)
	for _, e := range r.Executions {
		execs[e.ToolUseID] = e
	}
	terms := make(map[string]store.Terminal)
	for _, x := range r.Terminals {
		terms[x.ToolUseID] = x
	}
	for _, c := range calls {
		id := "t" + strconv.FormatInt(c.seq, 10)
		if d, ok := decls[id]; ok {
			if err := st.AppendDeclaration(d); err != nil {
				t.Fatalf("%s: AppendDeclaration: %v", id, err)
			}
		}
		if x, ok := terms[id]; ok {
			write := st.AppendTerminal
			if x.Seq == nil {
				write = st.SpillTerminal
			}
			if err := write(x); err != nil {
				t.Fatalf("%s: terminal: %v", id, err)
			}
		}
		write := st.AppendExecution
		if execs[id].Seq == nil {
			write = st.SpillExecution
		}
		if err := write(execs[id]); err != nil {
			t.Fatalf("%s: execution: %v", id, err)
		}
	}
	for _, c := range r.Coverage {
		if err := st.AppendCoverage(c); err != nil {
			t.Fatalf("coverage: %v", err)
		}
	}
	got, err := st.ReadRun("s3")
	if err != nil {
		t.Fatal(err)
	}
	if got.Skipped != 0 || got.Ignored != 0 {
		t.Fatalf("read back with Skipped %d and Ignored %d, want 0 and 0", got.Skipped, got.Ignored)
	}
	var gdecl, gexec, gterm []rec
	for _, d := range got.Declarations {
		gdecl = append(gdecl, rec{d.ToolUseID, d.Seq})
	}
	for _, e := range got.Executions {
		gexec = append(gexec, rec{e.ToolUseID, seqOf(e.Seq)})
	}
	for _, x := range got.Terminals {
		gterm = append(gterm, rec{x.ToolUseID, seqOf(x.Seq)})
	}
	if !slices.Equal(gdecl, decl) || !slices.Equal(gexec, exec) || !slices.Equal(gterm, term) {
		t.Fatalf("read back declarations %v, executions %v, terminals %v; built %v, %v, %v", gdecl, gexec, gterm, decl, exec, term)
	}
	if !samePtr(got.Terminals[1].Reason, r.Terminals[1].Reason) || !samePtr(got.Declarations[2].AgentID, r.Declarations[2].AgentID) || !samePtr(got.Declarations[2].AgentType, r.Declarations[2].AgentType) {
		t.Fatal("the terminal's reason or the subagent's id or type differs after the round trip")
	}
	if len(r.Coverage) != 1 || !reflect.DeepEqual(got.Coverage, r.Coverage) {
		t.Fatalf("read back coverage %+v, built %+v: want the one verified post record, as built", got.Coverage, r.Coverage)
	}
}
