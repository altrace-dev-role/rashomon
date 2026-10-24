package verify

import (
	"fmt"
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
}

func gateRun(seq int64, runner, outcome string) call {
	exit := 0
	if outcome == store.ExecFailed {
		exit = 1 // a plain failed run; a failure with an unknown code is a call literal with exit 0
	}
	return call{seq: seq, tool: "Bash", prog: "go", class: shape.VerbTest, runner: runner, kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: outcome, exit: exit}
}
func edit(seq int64, path string) call {
	return call{seq: seq, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{path}}
}
func bashWrite(seq int64, paths ...string) call {
	return call{seq: seq, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, paths: paths}
}

// runOf is runOfSession for the session "s1", the one most tests need.
func runOf(t testing.TB, calls ...call) *store.Run {
	t.Helper()
	return runOfSession(t, "s1", calls...)
}

// runOfSession writes one Declaration and one Execution per call and a
// Terminal for each call with a term, and fails the test on a run the store
// could not read back as built, or whose records break an invariant the hook
// and shape keep; the two shapes no writer produces yet, a named Bash write and
// a gate_output, are marked below. A fixture that breaks one makes every
// evaluator test built on it prove nothing. It enforces these invariants and
// no others, each where it comes from:
//   - Declarations and executions pair by index and by ToolUseID, and a terminal pairs by ToolUseID.
//   - Seq strictly increases, so it is at least 1 (store.appendOrdered allocates it).
//   - An Edit or Write names its path (the payload's file_path, which the hook records).
//   - A call that is not Bash names no runner, is not backgrounded and does not set mayWrite (deriveCut derives the runner and mayWrite only for a tool whose class is execute and whose input carries a command field, shape.go:158-163; of the three execute tools, verb.go:39, only Bash carries one; post.go reads Backgrounded for Bash alone).
//   - A call that is not Bash, Edit or Write names no paths.
//   - A runner's masked word is one of shape.MaskedKinds and its kind one of shape.RunnerKinds; with a runner the word is none or the runner's own kind (statusMasked and runnerOf take both from the runner group); a runner-less Bash call carries its word, none unless the line hid two runners or a displaced one (masked.go:177-178; runnerDigest, masked_eval.go:34-39); a tool that is not Bash carries no word (deriveCut returns before statusMasked). A line statusMasked could not parse (StatusMasked nil on a shell call) is not built: no evaluator test needs it yet.
//   - The execution's runner fields mirror the declaration's (post.go copies them from the shape it derives).
//   - The outcome is one of the Execution outcomes (record.go). A backgrounded call's outcome is ok: post.go reads Backgrounded on the success event only.
//   - ExitCode is set only on the failure event, whose outcome is failed or interrupted, with a known code (post.go parses it before the interrupt check); an exit on an ok call is refused.
//   - A Bash write that names paths is complete unless incomplete says otherwise, and one that names none carries MayWrite alone. This is the schema's written_paths_complete (record_v5.go): no writer fills written_paths yet, so a named Bash write is the schema's shape, not the hook's.
//   - A gate_output is what the call literal says, copied onto the execution. No writer fills gate_output yet (record_v5.go GateOutput is the schema), so a run carrying one is the schema's shape, not the hook's.
//   - No field is over the cap the hook would withhold it at (store.CapDeclaration and store.CapExecution, which the hook applies before its write).
//   - A terminal's outcome is one of the Terminal outcomes (record.go).
//   - The session id is its own run directory name (the store's DirName method, over segment): the builder does not digest. DirName reads no Store field (store.go:453, segment), which is what lets a zero Store answer; no test pins that.
//
// The cap functions guarantee caps and nothing else. The derived fields
// (VerbClass, RunnerDigest, RunnerKind, StatusMasked, MayWrite) are what the
// call literal says, not what shape.Derive would derive from any command, and
// CWDDigest, Hosts, SSHHosts and FileLabel are never set, since the hook
// computes them before its cap and the builder does not.
func runOfSession(t testing.TB, sessionID string, calls ...call) *store.Run {
	t.Helper()
	var zero store.Store
	if zero.DirName(sessionID) != sessionID {
		t.Fatalf("runOf: session id %q is not its own run directory name: the store digests it (Store.DirName), and the builder does not digest", sessionID)
	}
	r := &store.Run{Dir: sessionID}
	var last int64
	// next numbers the records as the store does, one Seq per record from 1,
	// in the hook's order for one call: its PreToolUse records (the
	// declaration, then the terminal Close writes) before its PostToolUse
	// record (the execution). Records of other calls interleaving in a real
	// run are not modelled.
	next := int64(1)
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
		if c.runner != "" && !slices.Contains(shape.RunnerKinds(), c.kind) {
			t.Fatalf("runOf: call %d runner kind %q is not one of %v", i, c.kind, shape.RunnerKinds())
		}
		if c.runner == "" && c.kind != "" {
			t.Fatalf("runOf: call %d has runner kind %q and no runner (shape: RunnerKind is set exactly when RunnerDigest is)", i, c.kind)
		}
		if c.runner != "" && c.masked != shape.MaskedNone &&
			!(c.masked == shape.MaskedTest && c.kind == shape.RunnerTest) && !(c.masked == shape.MaskedBuild && c.kind == shape.RunnerBuild) {
			t.Fatalf("runOf: call %d masked %q with runner kind %q: with a runner the masked word is none or the runner's own kind (shape: statusMasked and runnerOf take both from one runner group)", i, c.masked, c.kind)
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
		if c.incomplete && (c.tool != "Bash" || len(c.paths) == 0) {
			t.Fatalf("runOf: call %d is incomplete but is not a named Bash write (written_paths_complete describes a Bash write's named targets; the unnamed write carries MayWrite alone)", i)
		}
		id := "t" + strconv.FormatInt(c.seq, 10)
		dseq, tseq := next, int64(0)
		next++
		if c.term != "" {
			tseq = next
			next++
		}
		eseq := next
		next++
		d := store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, Seq: dseq, ToolUseID: id, SessionID: sessionID, ToolName: c.tool}
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
			k := c.kind
			d.Shape.RunnerKind = &k
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
		e := store.Execution{Type: store.TypeExecution, SchemaVersion: store.SchemaVersion, Seq: &eseq, ToolUseID: id, SessionID: sessionID, ToolName: c.tool, Outcome: c.outcome, Backgrounded: c.bg}
		e.RunnerDigest, e.StatusMasked, e.RunnerKind = d.Shape.RunnerDigest, d.Shape.StatusMasked, d.Shape.RunnerKind
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
		r.Declarations = append(r.Declarations, d)
		r.Executions = append(r.Executions, e)
		if c.term != "" {
			r.Terminals = append(r.Terminals, store.Terminal{Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion, Seq: &tseq,
				ToolUseID: id, SessionID: sessionID, Outcome: c.term, TerminalV5: store.TerminalV5{Writer: "fixture"}})
		}
	}
	return r
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

// sameGateOutput is samePtr for a GateOutput, whose Reason is itself a pointer.
func sameGateOutput(a, b *store.GateOutput) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Kind == b.Kind && a.Parser == b.Parser && samePtr(a.Reason, b.Reason))
}

func TestFixture_RunOfPairsEveryDeclarationWithItsExecution(t *testing.T) {
	failed := gateRun(3, "go-test", store.ExecFailed)
	failed.term = store.OutcomeError
	bgRun := gateRun(4, "go-test", store.ExecOK)
	bgRun.bg = true
	calls := []call{
		edit(1, "a.go"),
		gateRun(2, "go-test", store.ExecOK),
		failed,
		bgRun,
		call{seq: 5, tool: "Bash", prog: "make", class: shape.VerbTest, runner: "make-test", kind: shape.RunnerTest, masked: shape.MaskedTest, outcome: store.ExecOK},
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
	// The rows compare a record against its own call's kind, so gateRun's
	// default is pinned here or nowhere.
	if k := r.Executions[1].RunnerKind; k == nil || *k != shape.RunnerTest {
		t.Fatalf("gateRun's execution: RunnerKind set %t, want %q", k != nil, shape.RunnerTest)
	}
	if e := r.Executions[0]; e.RunnerDigest != nil || e.StatusMasked != nil || e.RunnerKind != nil || e.ExitCode != nil {
		t.Fatalf("edit execution carries runner fields: RunnerDigest set %t, StatusMasked set %t, RunnerKind set %t, ExitCode set %t", e.RunnerDigest != nil, e.StatusMasked != nil, e.RunnerKind != nil, e.ExitCode != nil)
	}
}

// RunnerDigest is the gate family key (D2) that Task 1.3 keys families on: one
// runner word gives one digest and two words give two. This tests that
// property, not the "r:" spelling the builder happens to use.
func TestFixture_RunnerDigestIsOneFamilyPerRunnerWord(t *testing.T) {
	r := runOf(t, gateRun(1, "go-test", store.ExecOK), gateRun(2, "go-vet", store.ExecOK), gateRun(3, "go-test", store.ExecOK))
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
	r := runOf(t, gateRun(1, "go-test", store.ExecOK), gateRun(2, "go-test", store.ExecFailed), call{seq: 3, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecFailed}, call{seq: 4, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecFailed, exit: 2}, call{seq: 5, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecInterrupted, exit: 130})
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
	gate := func(masked, kind, outcome string, exit int, bg bool) call {
		return call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: kind, masked: masked, outcome: outcome, exit: exit, bg: bg}
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

		// The pairings the hook keeps between fields, each with its accepted twin.
		{"masked test on a build runner", "s1", []call{gate(shape.MaskedTest, shape.RunnerBuild, store.ExecOK, 0, false)}, "the runner's own kind"},
		{"masked build on a build runner", "s1", []call{gate(shape.MaskedBuild, shape.RunnerBuild, store.ExecOK, 0, false)}, ""},
		{"backgrounded failure", "s1", []call{gate(shape.MaskedNone, shape.RunnerTest, store.ExecFailed, 0, true)}, "backgrounded"},
		{"backgrounded success", "s1", []call{gate(shape.MaskedNone, shape.RunnerTest, store.ExecOK, 0, true)}, ""},
		{"exit code on success", "s1", []call{gate(shape.MaskedNone, shape.RunnerTest, store.ExecOK, 3, false)}, "exit code 3"},
		{"exit code on failure", "s1", []call{gate(shape.MaskedNone, shape.RunnerTest, store.ExecFailed, 3, false)}, ""},
		{"exit code on interrupt", "s1", []call{gate(shape.MaskedNone, shape.RunnerTest, store.ExecInterrupted, 130, false)}, ""},
		{"outcome outside the three", "s1", []call{gate(shape.MaskedNone, shape.RunnerTest, "done", 0, false)}, "outcome \"done\""},
		{"masked word on an edit", "s1", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, masked: shape.MaskedNone, outcome: store.ExecOK, paths: []string{"a.go"}}}, "masked word"},
		{"masked word on a runner-less Bash call", "s1", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, masked: shape.MaskedNone, outcome: store.ExecOK}}, ""},
		// Two hidden runners, or one a cd displaced: the word without a runner.
		{"masked test on a runner-less Bash call", "s1", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, masked: shape.MaskedTest, outcome: store.ExecOK}}, ""},
		{"runner-less Bash call with a bad masked word", "s1", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, masked: "hidden", outcome: store.ExecOK}}, "masked \"hidden\" is not one of"},
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
	gate := gateRun(1, "go-test", store.ExecOK)
	gate.term = store.OutcomeOK
	ed := edit(2, "a.go")
	ed.term = store.OutcomeOK
	complete := bashWrite(3, "b.go", "c.go")
	incomplete := bashWrite(4, "d.go")
	incomplete.incomplete, incomplete.term = true, store.OutcomeError
	// A `go test ./... | tail` whose status the pipe hides, reported
	// backgrounded by the tool response, whose PreToolUse hook was cancelled
	// by timeout after its declaration landed (a signal terminal) and whose
	// tool ran anyway.
	hidden := gateRun(5, "go-test", store.ExecOK)
	hidden.bg, hidden.masked, hidden.kind, hidden.term = true, shape.MaskedTest, shape.RunnerTest, store.OutcomeSignal
	// A `go build ./... | tail -1; exit 2`: the build runner's status hidden,
	// the line failing on the failure event with "Exit code 2".
	build := gateRun(6, "go-build", store.ExecFailed)
	build.class, build.exit, build.masked, build.kind = shape.VerbPackage, 2, shape.MaskedBuild, shape.RunnerBuild
	build.gateOutput = &store.GateOutput{Kind: "summary_fail", Parser: "go"}
	calls := []call{gate, ed, complete, incomplete, hidden, build}
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
		t.Fatalf("built %d terminals, want 4: four of the six calls name a term", len(r.Terminals))
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
	c := gateRun(1, "go-test", store.ExecOK)
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
