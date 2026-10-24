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
	seq      int64
	tool     string // Bash | Edit | Write, or any tool; only these three carry paths
	prog     string
	class    string
	runner   string // non-empty marks a recognised runner; becomes RunnerDigest
	kind     string // shape.RunnerTest | shape.RunnerBuild, with a runner only; becomes RunnerKind
	masked   string // shape.MaskedNone | shape.MaskedTest | shape.MaskedBuild
	outcome  string // store.ExecOK | store.ExecFailed
	exit     int
	bg       bool
	paths    []string
	mayWrite bool // Bash write with no lexical target, the unnamed-write case; paths empty
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

// runOf writes one Declaration and one Execution per call, and fails the test
// on a run the store could not have written: a fixture the store forbids makes
// every evaluator test built on it prove nothing. It enforces these invariants
// and no others:
//   - Declarations and executions pair by index and by ToolUseID.
//   - Seq strictly increases, so it is at least 1, as the store allocates it.
//   - An Edit or Write names its path.
//   - A call that is not Bash names no runner, is not backgrounded and does not set mayWrite.
//   - A call that is not Bash, Edit or Write names no paths.
//   - A runner's masked word is one of shape.MaskedKinds.
//   - A runner's kind is one of shape.RunnerKinds, and a call with no runner has none (shape: RunnerKind is set exactly when RunnerDigest is).
//   - The execution's runner fields mirror the declaration's.
//   - ExitCode is set only on a failure with a known code.
//   - A Bash write that names paths is complete; one that names none carries MayWrite alone.
func runOf(t testing.TB, calls ...call) *store.Run {
	t.Helper()
	r := &store.Run{Dir: "s1"}
	var last int64
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
		}
		if c.runner != "" && !slices.Contains(shape.MaskedKinds(), c.masked) {
			t.Fatalf("runOf: call %d masked %q is not one of %v", i, c.masked, shape.MaskedKinds())
		}
		if c.runner != "" && !slices.Contains(shape.RunnerKinds(), c.kind) {
			t.Fatalf("runOf: call %d runner kind %q is not one of %v", i, c.kind, shape.RunnerKinds())
		}
		if c.runner == "" && c.kind != "" {
			t.Fatalf("runOf: call %d has runner kind %q and no runner (shape: RunnerKind is set exactly when RunnerDigest is)", i, c.kind)
		}
		id := "t" + strconv.FormatInt(c.seq, 10)
		d := store.Declaration{Seq: c.seq, ToolUseID: id, SessionID: "s1", ToolName: c.tool}
		d.Shape.VerbClass = c.class
		if c.prog != "" {
			p := c.prog
			d.Shape.Program = &p
		}
		if c.runner != "" {
			rd := "r:" + c.runner
			d.Shape.RunnerDigest = &rd
			m := c.masked
			d.Shape.StatusMasked = &m
			k := c.kind
			d.Shape.RunnerKind = &k
		}
		if c.tool == "Edit" || c.tool == "Write" {
			p := c.paths[0]
			d.FilePath = &p
		} else if len(c.paths) > 0 {
			complete := true // every write-capable segment yielded a target (record_v5.go WrittenPathsComplete)
			d.WrittenPaths, d.WrittenPathsComplete = append([]string(nil), c.paths...), &complete
			d.Shape.MayWrite = true
		} else if c.mayWrite {
			d.Shape.MayWrite = true // a Bash write with no known target: the unnamed-write case
		}
		seq := c.seq
		e := store.Execution{Seq: &seq, ToolUseID: id, SessionID: "s1", ToolName: c.tool, Outcome: c.outcome, Backgrounded: c.bg}
		e.RunnerDigest, e.StatusMasked, e.RunnerKind = d.Shape.RunnerDigest, d.Shape.StatusMasked, d.Shape.RunnerKind
		if c.outcome == store.ExecFailed && c.exit != 0 {
			ex := c.exit // store.Execution: null on success, null on a failure without a code; never 0
			e.ExitCode = &ex
		}
		r.Declarations = append(r.Declarations, d)
		r.Executions = append(r.Executions, e)
	}
	return r
}

func intOr0(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func TestFixture_RunOfPairsEveryDeclarationWithItsExecution(t *testing.T) {
	bgRun := gateRun(4, "go-test", store.ExecOK)
	bgRun.bg = true
	calls := []call{
		edit(1, "a.go"),
		gateRun(2, "go-test", store.ExecOK),
		gateRun(3, "go-test", store.ExecFailed),
		bgRun,
		call{seq: 5, tool: "Bash", prog: "make", class: shape.VerbTest, runner: "make-test", kind: shape.RunnerTest, masked: shape.MaskedTest, outcome: store.ExecOK},
		call{seq: 6, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"w.go"}},
		call{seq: 7, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", kind: shape.RunnerBuild, masked: shape.MaskedBuild, outcome: store.ExecOK},
	}
	r := runOf(t, calls...)
	if len(r.Declarations) != len(calls) || len(r.Executions) != len(calls) {
		t.Fatalf("builder mispaired: %d declarations and %d executions, want %d of each", len(r.Declarations), len(r.Executions), len(calls))
	}
	for i, c := range calls {
		d, e := r.Declarations[i], r.Executions[i]
		if d.Seq != c.seq {
			t.Fatalf("call %d: declaration Seq %d, want %d", i, d.Seq, c.seq)
		}
		if e.Seq == nil {
			t.Fatalf("call %d: execution has no Seq", i)
		} else if *e.Seq != c.seq {
			t.Fatalf("call %d: execution Seq %d, want %d", i, *e.Seq, c.seq)
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
		if c.runner == "" {
			if dm != nil || em != nil {
				t.Fatalf("call %d: StatusMasked set %t on the declaration and %t on the execution of a non-runner, want neither", i, dm != nil, em != nil)
			}
		} else if dm == nil || em == nil {
			t.Fatalf("call %d: StatusMasked set %t on the declaration and %t on the execution, want both", i, dm != nil, em != nil)
		} else if *dm != c.masked || *em != c.masked {
			t.Fatalf("call %d: StatusMasked %q on the declaration, %q on the execution, want %q", i, *dm, *em, c.masked)
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
	r := runOf(t, gateRun(1, "go-test", store.ExecOK), gateRun(2, "go-test", store.ExecFailed), call{seq: 3, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecFailed}, call{seq: 4, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, exit: 3}, call{seq: 5, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecFailed, exit: 2})
	if r.Executions[0].ExitCode != nil {
		t.Fatalf("ok run carries an exit code: %d", *r.Executions[0].ExitCode)
	}
	if e := r.Executions[1].ExitCode; e == nil || *e != 1 {
		t.Fatalf("failed run should carry exit 1, got set %t value %d", e != nil, intOr0(e))
	}
	if r.Executions[2].ExitCode != nil {
		t.Fatalf("failure with an unknown code must carry nil, got %d", *r.Executions[2].ExitCode)
	}
	if r.Executions[3].ExitCode != nil {
		t.Fatalf("ok run with a stray code must carry nil, got %d", *r.Executions[3].ExitCode)
	}
	if e := r.Executions[4].ExitCode; e == nil || *e != 2 {
		t.Fatalf("failed run with exit 2 should carry 2, got set %t value %d", e != nil, intOr0(e))
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

func TestFixture_RunOfRejectsRunsTheStoreForbids(t *testing.T) {
	// Each row breaks exactly one rule; runOf reports the first rule a call breaks.
	cases := []struct {
		name  string
		calls []call
		want  string
	}{
		{"duplicate seq", []call{edit(2, "a.go"), edit(2, "b.go")}, "seq"},
		{"decreasing seq", []call{edit(3, "a.go"), edit(1, "b.go")}, "seq"},
		{"pathless edit", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK}}, "no path"},
		{"runner on edit", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, runner: "go-test", kind: shape.RunnerTest, masked: shape.MaskedNone, outcome: store.ExecOK, paths: []string{"a.go"}}}, "not a shell"},
		{"bg on edit", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, bg: true, paths: []string{"a.go"}}}, "every tool but a shell"},
		{"mayWrite on edit", []call{call{seq: 1, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, mayWrite: true, paths: []string{"a.go"}}}, "shell-line"},
		{"runner with bad masked", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: shape.RunnerTest, outcome: store.ExecOK}}, "masked \"\" is not one of"},
		{"runner with bad kind", []call{call{seq: 1, tool: "Bash", prog: "go", class: shape.VerbTest, runner: "go-test", kind: "lint", masked: shape.MaskedNone, outcome: store.ExecOK}}, "runner kind \"lint\" is not one of"},
		{"kind without runner", []call{call{seq: 1, tool: "Bash", prog: "ls", class: shape.VerbRead, kind: shape.RunnerTest, outcome: store.ExecOK}}, "and no runner"},
		{"paths on read", []call{call{seq: 1, tool: "Read", class: shape.VerbRead, outcome: store.ExecOK, paths: []string{"a.go"}}}, "names paths"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTB{}
			called := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						if r != f {
							panic(r)
						}
						called = true
					}
				}()
				runOf(f, tc.calls...)
			}()
			if !called || !strings.Contains(f.fatal, tc.want) {
				t.Fatalf("called %t, message %q", called, f.fatal)
			}
		})
	}
}
