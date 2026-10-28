package verify

import (
	"cmp"
	"maps"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// pairsOf is verify's tests_only_then_green pairs over the whole of r.
func pairsOf(r *store.Run) [][2]int64 {
	return testsOnlyThenGreen(r, pickExecutions(r), 0, math.MaxInt64)
}

// reportPairs is report.DetectTestBending's pairs over r as the report reads
// it: over a copy with report.MaskingAsRan applied first, AssembleSession's
// order (D44). MaskingAsRan writes the declarations it reads, so the copy is
// what leaves r as it was.
func reportPairs(r *store.Run) [][2]int64 {
	c := *r
	c.Declarations = slices.Clone(r.Declarations)
	report.MaskingAsRan(&c)
	out := [][2]int64{}
	for _, p := range report.DetectTestBending(&c, nil).TestsOnlyThenGreen {
		out = append(out, [2]int64(p))
	}
	return out
}

// samePairs reports whether a and b hold the same pairs, in any order.
func samePairs(a, b [][2]int64) bool {
	order := func(x, y [2]int64) int { return cmp.Or(cmp.Compare(x[1], y[1]), cmp.Compare(x[0], y[0])) }
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, order)
	slices.SortFunc(b, order)
	return slices.Equal(a, b)
}

// shuffled is a copy of r with its declarations, executions and terminals
// each in an order rng picks.
func shuffled(r *store.Run, rng *rand.Rand) *store.Run {
	c := *r
	c.Declarations, c.Executions, c.Terminals = slices.Clone(r.Declarations), slices.Clone(r.Executions), slices.Clone(r.Terminals)
	rng.Shuffle(len(c.Declarations), func(i, j int) { c.Declarations[i], c.Declarations[j] = c.Declarations[j], c.Declarations[i] })
	rng.Shuffle(len(c.Executions), func(i, j int) { c.Executions[i], c.Executions[j] = c.Executions[j], c.Executions[i] })
	rng.Shuffle(len(c.Terminals), func(i, j int) { c.Terminals[i], c.Terminals[j] = c.Terminals[j], c.Terminals[i] })
	return &c
}

// labelTests gives every Edit and Write declaration whose path ends in
// _test.go the test-file label, as the hook's label layer does for a path
// named like a Go test (shape.LabelTestFile); the builder sets no label.
func labelTests(r *store.Run) {
	for i := range r.Declarations {
		if p := r.Declarations[i].FilePath; p != nil && strings.HasSuffix(*p, "_test.go") {
			labelTest(r, i)
		}
	}
}

// digestAs gives the declaration of call seq the shape digest s. The builder
// leaves every digest "", so every run at one location is one command to the
// report until a test names two.
func digestAs(r *store.Run, seq int64, s string) {
	for i := range r.Declarations {
		if r.Declarations[i].ToolUseID == callID(seq) {
			r.Declarations[i].Shape.Digest = s
		}
	}
}

// execOf is the execution of call seq.
func execOf(r *store.Run, seq int64) *store.Execution {
	for i := range r.Executions {
		if r.Executions[i].ToolUseID == callID(seq) {
			return &r.Executions[i]
		}
	}
	return nil
}

// declOf is the declaration of call seq.
func declOf(r *store.Run, seq int64) *store.Declaration {
	for i := range r.Declarations {
		if r.Declarations[i].ToolUseID == callID(seq) {
			return &r.Declarations[i]
		}
	}
	return nil
}

// callID is the ToolUseID runOf gives call seq.
func callID(seq int64) string { return "t" + strconv.FormatInt(seq, 10) }

func redRun(seq int64) call   { return gateCall(seq, "go-test", store.ExecFailed) }
func greenRun(seq int64) call { return gateCall(seq, "go-test", store.ExecOK) }

// tool is a call of a tool that names no path, ok.
func tool(seq int64, name, class string) call {
	return call{seq: seq, tool: name, class: class, outcome: store.ExecOK}
}

func failedEdit(seq int64, path string) call {
	c := edit(seq, path)
	c.outcome = store.ExecFailed
	return c
}

// maskedLine is a go test line that hid its status (status_masked test),
// which ran ok, with its output read as out; "" reads none.
func maskedLine(seq int64, out string) call {
	c := call{seq: seq, tool: "Bash", prog: "go", class: shape.VerbExecute, runner: "go-test", kind: shape.RunnerTest,
		masked: shape.MaskedTest, outcome: store.ExecOK, loc: "d:repo"}
	if out != "" {
		c.gateOutput = &store.GateOutput{Kind: out, Parser: "go"}
	}
	return c
}

// timeoutRun is `timeout 60 go test ./...` failing with exit.
func timeoutRun(seq int64, exit int) call {
	c := redRun(seq)
	c.prog, c.exit = "timeout", exit
	return c
}

// lostCall is a call whose declaration was never written: its execution and
// the terminal Handler.Close writes are the only trace of its id.
func lostCall(seq int64) call {
	return call{seq: seq, tool: "Bash", prog: "make", class: shape.VerbExecute, outcome: store.ExecOK, term: store.OutcomeError, lostDeclaration: true}
}

// noWriteNames is report.noWrite's names as testbending.go:281-287 lists
// them, typed here so a name dropped from either copy shows as a pair one
// side finds and the other does not.
var noWriteNames = []string{
	"TodoWrite", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet",
	"TaskOutput", "TaskStop", "AskUserQuestion", "EnterPlanMode",
	"ExitPlanMode", "BashOutput", "KillShell", "KillBash", "Skill",
	"ToolSearch", "SendMessage", "CronCreate", "CronDelete", "CronList",
	"ListMcpResourcesTool", "ReadMcpResourceTool",
}

// pairRow is one INT14 fixture and the number of pairs the report finds in it.
type pairRow struct {
	name  string
	pairs int
	run   func(t *testing.T) *store.Run
}

// labelled is runOf with every test path labelled.
func labelled(t *testing.T, calls ...call) *store.Run {
	t.Helper()
	r := runOf(t, calls...)
	labelTests(r)
	return r
}

// pairRows is INT14's named fixtures: 1.8's pair fixtures, then one row per
// edge of the definition (R1-R5, D44), each built so the two definitions
// would part there if they differed.
func pairRows() []pairRow {
	const tf, sf = "pkg/calc_test.go", "pkg/calc.go"
	rows := []pairRow{
		{"1.8 INT4", 1, func(t *testing.T) *store.Run { return labelled(t, redRun(1), edit(2, tf), greenRun(3), edit(4, sf)) }},
		{"1.8 INT5", 1, func(t *testing.T) *store.Run { return int5Run(t) }},
		{"1.8 INT8", 1, func(t *testing.T) *store.Run { return int8Run(t) }},
		{"1.8 INT13", 1, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf).marks(ids{"go.t_skip": 1}, nil), greenRun(3), writeCall(4, tf).marks(ids{"go.nolint": 1}, nil))
		}},
		{"1.8 BenchmarkIntegrity", 1, func(t *testing.T) *store.Run { r, _ := benchIntegrityRun(t); return r }},
		{"a Write to a test file", 1, func(t *testing.T) *store.Run { return labelled(t, redRun(1), writeCall(2, tf), greenRun(3)) }},
		{"a source edit between", 0, func(t *testing.T) *store.Run { return labelled(t, redRun(1), edit(2, tf), edit(3, sf), greenRun(4)) }},
		{"a docs edit between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), edit(3, "README.md"), greenRun(4))
		}},
		{"no edit between", 0, func(t *testing.T) *store.Run { return labelled(t, redRun(1), greenRun(2), edit(3, tf)) }},
		{"a green, a test edit, a red", 0, func(t *testing.T) *store.Run { return labelled(t, greenRun(1), edit(2, tf), redRun(3)) }},
		// Every run with a result becomes its command's last run, whether or
		// not it is an edit: a passing run by a noWrite tool is no edit, so
		// only the mark it moves stops the second green pairing with the red.
		{"a passing run that is no edit moves the mark", 1, func(t *testing.T) *store.Run {
			quiet := call{seq: 3, tool: "TaskOutput", class: shape.VerbTest, outcome: store.ExecOK, loc: "d:repo"}
			return labelled(t, redRun(1), edit(2, tf), quiet, edit(4, tf), greenRun(5))
		}},
		{"two reds then a green", 1, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), redRun(3), edit(4, tf), greenRun(5))
		}},
		{"a green, a test edit, a green", 1, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), greenRun(3), edit(4, tf), greenRun(5))
		}},
		{"another command closes", 0, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3))
			digestAs(r, 3, "s:other")
			return r
		}},
		{"another command between", 0, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3), greenRun(4))
			digestAs(r, 3, "s:other")
			return r
		}},
		{"the same command elsewhere between", 0, func(t *testing.T) *store.Run {
			elsewhere := greenRun(3)
			elsewhere.loc = "d:other"
			return labelled(t, redRun(1), edit(2, tf), elsewhere, greenRun(4))
		}},
		{"the same command interrupted between", 0, func(t *testing.T) *store.Run {
			stopped := gateCall(3, "go-test", store.ExecInterrupted)
			return labelled(t, redRun(1), edit(2, tf), stopped, greenRun(4))
		}},
		{"an interrupted red with a code", 0, func(t *testing.T) *store.Run {
			stopped := gateCall(1, "go-test", store.ExecInterrupted)
			stopped.exit = 130
			return labelled(t, stopped, edit(2, tf), greenRun(3))
		}},
		{"the same command backgrounded between", 0, func(t *testing.T) *store.Run {
			bg := greenRun(3)
			bg.bg = true
			return labelled(t, redRun(1), edit(2, tf), bg, greenRun(4))
		}},
		{"a backgrounded green", 0, func(t *testing.T) *store.Run {
			bg := greenRun(3)
			bg.bg = true
			return labelled(t, redRun(1), edit(2, tf), bg)
		}},
		{"a timeout-124 red", 0, func(t *testing.T) *store.Run { return labelled(t, timeoutRun(1, 124), edit(2, tf), greenRun(3)) }},
		{"a timeout red with its own code", 1, func(t *testing.T) *store.Run { return labelled(t, timeoutRun(1, 2), edit(2, tf), greenRun(3)) }},
		{"a timeout-124 run between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), timeoutRun(3, 124), greenRun(4))
		}},
		{"no known location", 0, func(t *testing.T) *store.Run {
			a, b := redRun(1), greenRun(3)
			a.loc, b.loc = "", ""
			return labelled(t, a, edit(2, tf), b)
		}},
		{"a failed test edit alone", 0, func(t *testing.T) *store.Run { return labelled(t, redRun(1), failedEdit(2, tf), greenRun(3)) }},
		// A failed test-file edit moves neither count: it neither completes
		// nor stops the pair (testbending.go:187-192; Gate B F1).
		{"a failed test edit beside an ok one", 1, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), failedEdit(3, tf), greenRun(4))
		}},
		{"a failed source edit between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), failedEdit(3, sf), greenRun(4))
		}},
		{"an undeclared call after the green", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), greenRun(3), lostCall(4))
		}},
		{"an undeclared call before the red", 0, func(t *testing.T) *store.Run {
			return labelled(t, lostCall(1), redRun(2), edit(3, tf), greenRun(4))
		}},
		// A flush_failed marker with no id names no call (testbending.go:226).
		{"a flush_failed terminal with no id", 1, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3))
			s, reason := int64(7), store.ReasonFlushFailed
			r.Terminals = append(r.Terminals, store.Terminal{Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion, Seq: &s,
				SessionID: "s1", Outcome: store.OutcomeError, Reason: &reason})
			return r
		}},
		{"an unnamed shell write between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), unnamedWrite(3), greenRun(4))
		}},
		{"an MCP call between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), tool(3, "mcp__db__query", shape.VerbMCP), greenRun(4))
		}},
		{"a shell write to a test file between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), bashWrite(3, "pkg/b_test.go"), greenRun(4))
		}},
		{"a read, a fetch and an agent between", 1, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), tool(3, "Read", shape.VerbRead), tool(4, "WebFetch", shape.VerbNetwork),
				tool(5, "Agent", shape.VerbAgent), greenRun(6))
		}},
		{"a read-class shell line that may write", 0, func(t *testing.T) *store.Run {
			w := unnamedWrite(3)
			w.class = shape.VerbRead
			return labelled(t, redRun(1), edit(2, tf), w, greenRun(4))
		}},
		{"an unknown tool between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), tool(3, "Frobnicate", shape.VerbUnknown), greenRun(4))
		}},
		{"a masked line between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), maskedLine(3, ""), greenRun(4))
		}},
		{"a masked line with a pass summary between", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), maskedLine(3, "summary_pass"), greenRun(4))
		}},
		{"a masked line with a pass summary closes nothing", 0, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), maskedLine(3, "summary_pass"))
		}},
		// D44: masking as the line ran. Another hook rewrote the green's line
		// to hide its status, so it is no run and an edit.
		{"masked as the line ran", 0, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3))
			e, m := execOf(r, 3), shape.MaskedTest
			e.ExecutedDigest, e.StatusMasked = "s:rewritten", &m
			return r
		}},
		// And the other way: the declared line hid the status, the line that
		// ran did not, so it is a run.
		{"unmasked as the line ran", 1, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3))
			m, none := shape.MaskedTest, shape.MaskedNone
			declOf(r, 3).Shape.StatusMasked = &m
			e := execOf(r, 3)
			e.ExecutedDigest, e.StatusMasked = "s:rewritten", &none
			return r
		}},
		// A read line another hook rewrote into a masked runner line: as it
		// ran it hid a status, so it may have edited (mayEdit).
		{"a read line that ran as a masked runner between", 0, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), call{seq: 3, tool: "Bash", prog: "cat", class: shape.VerbRead, outcome: store.ExecOK}, greenRun(4))
			e, m := execOf(r, 3), shape.MaskedTest
			e.ExecutedDigest, e.StatusMasked = "s:rewritten", &m
			return r
		}},
		{"a green with no execution", 0, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3))
			r.Executions = slices.DeleteFunc(r.Executions, func(e store.Execution) bool { return e.ToolUseID == callID(3) })
			return r
		}},
		{"a spilled green", 1, func(t *testing.T) *store.Run {
			g := greenRun(3)
			g.spilled, g.term = true, store.OutcomeOK
			return labelled(t, redRun(1), edit(2, tf), g)
		}},
		// The pair reads the outcome, not the output (testbending.go:171).
		{"an ok run whose output says it failed", 1, func(t *testing.T) *store.Run {
			g := greenRun(3)
			g.gateOutput = &store.GateOutput{Kind: "summary_fail", Parser: "go"}
			return labelled(t, redRun(1), edit(2, tf), g)
		}},
		// A failed record outranks a later ok one of the same call (outranks).
		{"a green with a failed record too", 0, func(t *testing.T) *store.Run {
			r := labelled(t, redRun(1), edit(2, tf), greenRun(3))
			e := *execOf(r, 3)
			s, x := int64(99), 1
			e.Seq, e.Outcome, e.ExitCode = &s, store.ExecFailed, &x
			r.Executions = append(r.Executions, e)
			return r
		}},
		{"a red in a subagent, a green in the main agent", 1, func(t *testing.T) *store.Run {
			a := redRun(1)
			a.agent = "sub1"
			return labelled(t, a, edit(2, tf), greenRun(3))
		}},
	}
	for _, name := range noWriteNames {
		rows = append(rows, pairRow{"a " + name + " between", 1, func(t *testing.T) *store.Run {
			return labelled(t, redRun(1), edit(2, tf), tool(3, name, shape.VerbUnknown), greenRun(4))
		}})
	}
	return rows
}

// TestINT14_PairsEqualTheReportsOnAnyRecord: verify's tests_only_then_green
// pairs equal report.DetectTestBending's over a MaskingAsRan copy (D44), as
// sets of (red, green), on every INT14 row, on 100 shuffles of each row's
// records, and on generated records; each row's pair count is the report's,
// so neither side can agree by finding nothing. A divergence is a Gate A
// question with both definitions quoted, never a patch (card 1.8r).
func TestINT14_PairsEqualTheReportsOnAnyRecord(t *testing.T) {
	rng := rand.New(rand.NewSource(14))
	for _, row := range pairRows() {
		t.Run(row.name, func(t *testing.T) {
			r := row.run(t)
			want := reportPairs(r)
			if len(want) != row.pairs {
				t.Fatalf("the report finds %v, want %d pairs: the fixture is not the case it names", want, row.pairs)
			}
			if got := pairsOf(r); !samePairs(got, want) {
				t.Fatalf("verify %v, report %v", got, want)
			}
			for i := range 100 {
				s := shuffled(r, rng)
				if got, want := pairsOf(s), reportPairs(s); !samePairs(got, want) {
					t.Fatalf("shuffle %d: verify %v, report %v", i, got, want)
				}
			}
		})
	}
	t.Run("generated", func(t *testing.T) {
		total := 0
		for i := range 400 {
			r := generatedRun(t, rng)
			want := reportPairs(r)
			total += len(want)
			if got := pairsOf(r); !samePairs(got, want) {
				t.Fatalf("record %d: verify %v, report %v", i, got, want)
			}
			s := shuffled(r, rng)
			if got, want := pairsOf(s), reportPairs(s); !samePairs(got, want) {
				t.Fatalf("record %d shuffled: verify %v, report %v", i, got, want)
			}
		}
		if total < 40 {
			t.Fatalf("400 generated records hold %d pairs; the generator is not reaching the pattern", total)
		}
		t.Logf("400 generated records hold %d pairs", total)
	})
	t.Run("the noWrite map matches the test's copy of testbending.go:281-287", func(t *testing.T) {
		if got := slices.Sorted(maps.Keys(noWrite)); !slices.Equal(got, slices.Sorted(slices.Values(noWriteNames))) {
			t.Errorf("verify's noWrite %v, want the test's copy of testbending.go:281-287 %v", got, noWriteNames)
		}
	})
}

// generatedRun is a record of episodes and single calls drawn from the
// pair's vocabulary: an episode is a red run, one to three calls, mostly test
// edits, reads and noWrite tools, now and then one that breaks the pattern,
// and a closing run that passed, failed or hid its status. The single calls
// and the breakers cover runs of two commands at two locations and none,
// every outcome, test and source edits ok and failed, shell writes, unknown
// tools, masked lines, timeout runs and lost declarations.
func generatedRun(t *testing.T, rng *rand.Rand) *store.Run {
	t.Helper()
	var calls []call
	seq := int64(0)
	next := func() int64 { seq++; return seq }
	any := func() call {
		s := next()
		switch k := rng.Intn(14); k {
		case 0:
			c := gateCall(s, "go-test", []string{store.ExecFailed, store.ExecOK, store.ExecInterrupted}[rng.Intn(3)])
			c.loc = []string{"d:repo", "d:other", ""}[rng.Intn(3)]
			return c
		case 1:
			c := greenRun(s)
			c.bg = true
			return c
		case 2:
			return timeoutRun(s, []int{124, 2}[rng.Intn(2)])
		case 3:
			return maskedLine(s, []string{"", "summary_pass"}[rng.Intn(2)])
		case 4:
			return edit(s, "pkg/calc.go")
		case 5:
			return failedEdit(s, "pkg/calc.go")
		case 6:
			return bashWrite(s, "pkg/b_test.go")
		case 7:
			return unnamedWrite(s)
		case 8:
			return tool(s, "Frobnicate", shape.VerbUnknown)
		case 9:
			return tool(s, "mcp__db__query", shape.VerbMCP)
		case 10:
			return lostCall(s)
		case 11:
			return failedEdit(s, "pkg/calc_test.go")
		case 12:
			return tool(s, "Read", shape.VerbRead)
		}
		return edit(s, "pkg/calc_test.go")
	}
	benign := func() call {
		s := next()
		switch k := rng.Intn(10); {
		case k < 6:
			return edit(s, "pkg/calc_test.go")
		case k < 7:
			return writeCall(s, "pkg/calc_test.go")
		case k < 8:
			return tool(s, "Read", shape.VerbRead)
		case k < 9:
			return tool(s, noWriteNames[rng.Intn(len(noWriteNames))], shape.VerbUnknown)
		}
		return failedEdit(s, "pkg/calc_test.go")
	}
	for range 2 + rng.Intn(4) {
		if rng.Intn(5) < 2 {
			calls = append(calls, any())
			continue
		}
		calls = append(calls, redRun(next()))
		for range 1 + rng.Intn(3) {
			if rng.Intn(5) == 0 {
				calls = append(calls, any())
			} else {
				calls = append(calls, benign())
			}
		}
		switch k := rng.Intn(10); {
		case k < 7:
			calls = append(calls, greenRun(next()))
		case k < 9:
			calls = append(calls, redRun(next()))
		default:
			calls = append(calls, maskedLine(next(), "summary_pass"))
		}
	}
	r := runOf(t, calls...)
	labelTests(r)
	for i := range r.Declarations {
		if d := &r.Declarations[i]; d.Shape.VerbClass == shape.VerbTest && rng.Intn(6) == 0 {
			d.Shape.Digest = "s:other"
		}
	}
	return r
}

// unresolvedPairs is the tests_only_then_green entries of integrity over r
// for the window (from, at].
func unresolvedPairs(r *store.Run, from, at int64) []IntegrityEntry {
	var out []IntegrityEntry
	for _, e := range integrityOf(r, from, at).Unresolved {
		if e.Check == TestsOnlyThenGreen {
			out = append(out, e)
		}
	}
	return out
}

func TestINT15_AnUnnamedWriteBetweenBreaksThePair(t *testing.T) {
	const tf = "pkg/calc_test.go"
	for _, c := range []struct {
		name    string
		between call
		want    int
	}{
		{"a read (the control)", tool(3, "Read", shape.VerbRead), 1},
		{"an unnamed shell write", unnamedWrite(3), 0},
		{"an MCP call", tool(3, "mcp__db__query", shape.VerbMCP), 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := labelled(t, redRun(1), edit(2, tf), c.between, greenRun(4))
			if got := unresolvedPairs(r, 0, 100); len(got) != c.want {
				t.Errorf("%d pair entries %+v, want %d", len(got), got, c.want)
			}
			// And through Evaluate, which hands integrity classify's pick.
			n := 0
			for _, e := range evalAt(r, 8).Integrity.Unresolved {
				if e.Check == TestsOnlyThenGreen {
					n++
				}
			}
			if n != c.want {
				t.Errorf("Evaluate gives %d pair entries, want %d", n, c.want)
			}
		})
	}
}

// TestINT16_AMaskedLineBreaksThePairWhateverItsSummary: a line that hid a
// runner's status is no run and is an edit (testbending.go:83-87), so it
// neither closes a pair nor lets one through, whatever its output said
// (Gate A Q3, inverting the card's first item).
func TestINT16_AMaskedLineBreaksThePairWhateverItsSummary(t *testing.T) {
	const tf = "pkg/calc_test.go"
	if got := unresolvedPairs(labelled(t, redRun(1), edit(2, tf), greenRun(3)), 0, 100); len(got) != 1 {
		t.Fatalf("the control gives %+v, want one pair", got)
	}
	for _, out := range []string{"", "summary_pass", "no_summary"} {
		t.Run("summary "+cmp.Or(out, "none"), func(t *testing.T) {
			if got := unresolvedPairs(labelled(t, redRun(1), edit(2, tf), maskedLine(3, out), greenRun(4)), 0, 100); len(got) != 0 {
				t.Errorf("a masked line between: %+v, want no pair", got)
			}
			if got := unresolvedPairs(labelled(t, redRun(1), edit(2, tf), maskedLine(3, out)), 0, 100); len(got) != 0 {
				t.Errorf("a masked line as the closing run: %+v, want no pair", got)
			}
		})
	}
}

// TestINT17_TheWindowCutsOnTheGreenOnly: the pair is emitted at its passing
// run, so it is in the window (from, at] that holds the green, wherever the
// red fell; the red at or before from is still its red.
func TestINT17_TheWindowCutsOnTheGreenOnly(t *testing.T) {
	// The red is declaration 1, the test edit 3, the green 5.
	r := labelled(t, redRun(1), edit(2, "pkg/calc_test.go"), greenRun(3))
	picked := pickExecutions(r)
	for _, c := range []struct {
		from, at int64
		want     [][2]int64
	}{
		{0, 5, [][2]int64{{1, 5}}},
		{1, 5, [][2]int64{{1, 5}}}, // the red at the window start
		{4, 6, [][2]int64{{1, 5}}}, // the red and the edit before it
		{5, 6, nil},                // the green at the window start
		{0, 4, nil},                // the green after the point
	} {
		got := testsOnlyThenGreen(r, picked, c.from, c.at)
		if !slices.Equal(got, c.want) {
			t.Errorf("(%d, %d]: %v, want %v", c.from, c.at, got, c.want)
		}
		wantEntries := 0
		if len(c.want) > 0 {
			wantEntries = 1
		}
		if e := unresolvedPairs(r, c.from, c.at); len(e) != wantEntries || wantEntries == 1 && e[0].Seq != 5 {
			t.Errorf("(%d, %d]: integrity gives %+v, want %d entry at 5", c.from, c.at, e, wantEntries)
		}
	}
	// A call whose declaration was lost counts from the point that holds it
	// (byPoint), so the pairs at a point equal the pairs over the prefix
	// (INV-11): an orphan landing after the point cuts no pair at it.
	lost := int64(100)
	for _, c := range []struct {
		name string
		add  func(r *store.Run)
	}{
		{"execution", func(r *store.Run) {
			r.Executions = append(r.Executions, store.Execution{Type: store.TypeExecution, SchemaVersion: store.SchemaVersion, Seq: &lost,
				ToolUseID: "lost", SessionID: "s1", Outcome: store.ExecOK})
		}},
		{"terminal", func(r *store.Run) {
			r.Terminals = append(r.Terminals, store.Terminal{Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion, Seq: &lost,
				ToolUseID: "lost", SessionID: "s1", Outcome: store.OutcomeError})
		}},
	} {
		t.Run("an orphan "+c.name+" after the point", func(t *testing.T) {
			r := labelled(t, redRun(1), edit(2, "pkg/calc_test.go"), greenRun(3))
			c.add(r)
			picked := pickExecutions(r)
			if got := testsOnlyThenGreen(r, picked, 0, 6); !slices.Equal(got, [][2]int64{{1, 5}}) {
				t.Errorf("at 6: %v, want [[1 5]]", got)
			}
			if got := testsOnlyThenGreen(r, picked, 0, lost); len(got) != 0 {
				t.Errorf("at %d: %v, want none: the orphan is in the record by then", lost, got)
			}
		})
	}
}

// TestINT18_TwoRedsThenAGreenPairTheLastRed: each run with a result moves its
// command's mark (testbending.go:196-199), so the green pairs with the red
// just before it, once, as the report does.
func TestINT18_TwoRedsThenAGreenPairTheLastRed(t *testing.T) {
	const tf = "pkg/calc_test.go"
	r := labelled(t, redRun(1), edit(2, tf), redRun(3), edit(4, tf), greenRun(5))
	want := [][2]int64{{5, 9}}
	if got := reportPairs(r); !slices.Equal(got, want) {
		t.Fatalf("the report pairs %v, want %v", got, want)
	}
	if got := pairsOf(r); !slices.Equal(got, want) {
		t.Errorf("verify pairs %v, want %v", got, want)
	}
}
