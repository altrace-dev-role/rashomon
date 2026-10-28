package verify

import (
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The builder numbers records, not calls: a call's declaration Seq is runOf's
// counter, so a test reads it from the built run rather than from call.seq.
func declSeq(r *store.Run, i int) int64 { return r.Declarations[i].Seq }

func ptr[T any](v T) *T { return &v }

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestVER1_GateRunsAreKeyedOnRunnerAndLocationNotClass(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK))
	runs, _, _, _ := classify(r)
	if len(runs) != 1 {
		t.Fatalf("runs = %+v, want one", runs)
	}
	want, ok := runs[0].family()
	if !ok || want != (familyKey{location: "d:repo", runner: "r:go-test"}) {
		t.Fatalf("family = %+v, %v; want r:go-test at d:repo", want, ok)
	}

	r.Declarations[0].Shape.VerbClass = shape.VerbPackage // the sequencing flip the probe found
	runs, _, _, _ = classify(r)
	if len(runs) != 1 || str(runs[0].Runner) != "r:go-test" || runs[0].Location != "d:repo" || runs[0].Kind != kindTest {
		t.Fatalf("class package: runs = %+v, want runner r:go-test at d:repo, kind test", runs)
	}
	if got, ok := runs[0].family(); !ok || got != want {
		t.Fatalf("class package moved the family: %+v, %v; want %+v", got, ok, want)
	}
}

func TestVER1b_KindFallsBackToInferenceThenCustomOnOldRecords(t *testing.T) {
	// A record written before runner_kind existed: schema 5, the kind nil.
	old := func(c call) call { c.kind, c.schema = "", 5; return c }
	sibling := old(gateCall(1, "go-test", store.ExecOK))
	sibling.loc = "d:other"
	r := runOf(t, sibling, old(gateCall(2, "go-test", store.ExecOK)))
	for i := range r.Declarations {
		if r.Declarations[i].SchemaVersion != 5 || r.Declarations[i].Shape.RunnerKind != nil || r.Executions[i].RunnerKind != nil {
			t.Fatalf("premise: call %d is not a schema 5 record without a kind: %+v", i, r.Declarations[i])
		}
	}
	r.Declarations[1].Shape.VerbClass = shape.VerbPackage // second run sequenced; the sibling's class says test
	runs, _, _, _ := classify(r)
	if len(runs) != 2 || runs[0].Kind != kindTest || runs[1].Kind != kindTest {
		t.Fatalf("inference from a sibling run at another location: %+v", runs)
	}
	a, _ := runs[0].family()
	b, _ := runs[1].family()
	if a == b {
		t.Fatalf("the inference pooled across locations and so did the family: %+v", a)
	}

	r = runOf(t, old(gateCall(1, "go-test", store.ExecOK)))
	r.Declarations[0].Shape.VerbClass = shape.VerbPackage
	runs, _, _, _ = classify(r)
	if len(runs) != 1 || runs[0].Kind != kindCustom {
		t.Fatalf("nothing in the record says the kind: want custom, got %+v", runs)
	}

	hidden := old(call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, runner: "make-release", masked: shape.MaskedBuild, outcome: store.ExecOK, loc: "d:repo"})
	plain := old(call{seq: 2, tool: "Bash", prog: "make", class: shape.VerbPackage, runner: "make-release", masked: shape.MaskedNone, outcome: store.ExecOK, loc: "d:repo"})
	r = runOf(t, hidden, plain)
	runs, _, _, _ = classify(r)
	if len(runs) != 2 || runs[0].Kind != kindBuild || runs[1].Kind != kindBuild {
		t.Fatalf("a sibling masked build infers build: %+v", runs)
	}
}

func TestVER2_WritesCarryTheirPathClassAndUnknownTargetsAreUnnamed(t *testing.T) {
	unnamed3 := call{seq: 3, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true}
	r := runOf(t, edit(1, "a.go"), edit(2, "README.md"), unnamed3, bashWrite(4, "b_test.go"))
	_, writes, unnamed, _ := classify(r)
	if len(writes) != 4 || writes[0].Class != ClassSource || writes[1].Class != ClassDocs || writes[3].Class != ClassTest {
		t.Fatalf("writes = %+v", writes)
	}
	if writes[2].Path != nil || writes[2].Class != "" || writes[2].Seq != declSeq(r, 2) {
		t.Fatalf("the unnamed write = %+v, want no path and no class at seq %d", writes[2], declSeq(r, 2))
	}
	if !slices.Equal(unnamed, []int64{declSeq(r, 2)}) {
		t.Fatalf("unnamed = %v, want [%d]", unnamed, declSeq(r, 2))
	}
}

func TestVER3_ADeclarationWithoutAnExecutionIsNotARun(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK))
	r.Executions = nil
	runs, _, _, _ := classify(r)
	if len(runs) != 0 {
		t.Fatalf("a run without an execution has no outcome; got %+v", runs)
	}
}

func TestVER4_UnnamedWriteIsListed(t *testing.T) {
	for _, c := range []struct {
		name  string
		build func(t *testing.T) *store.Run
		named int // writes that carry a path
	}{
		{"a shell line that may write and names no target", func(t *testing.T) *store.Run {
			return runOf(t, call{seq: 1, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true})
		}, 0},
		{"an incomplete named list", func(t *testing.T) *store.Run {
			c := bashWrite(1, "a.go")
			c.incomplete = true
			return runOf(t, c)
		}, 1},
		{"a file tool whose path is withheld", func(t *testing.T) *store.Run {
			r := runOf(t, edit(1, "a.go"))
			r.Declarations[0].FilePath = nil
			return r
		}, 0},
		{"an MCP call", func(t *testing.T) *store.Run {
			return runOf(t, call{seq: 1, tool: "mcp__fs__write", class: shape.VerbMCP, outcome: store.ExecOK})
		}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := c.build(t)
			_, writes, unnamed, _ := classify(r)
			if !slices.Equal(unnamed, []int64{declSeq(r, 0)}) {
				t.Fatalf("unnamed = %v, want [%d]", unnamed, declSeq(r, 0))
			}
			named := 0
			for _, w := range writes {
				if w.Path != nil {
					named++
				} else if w.Class != "" {
					t.Fatalf("a write with no path has class %q", w.Class)
				}
			}
			if named != c.named {
				t.Fatalf("writes = %+v, want %d with a path", writes, c.named)
			}
		})
	}
}

func TestVER5_UnidentifiedRunnerIsNoFamily(t *testing.T) {
	// Two hidden runners or a displaced one: masked test, no runner digest,
	// class execute (masked.go:177-178; masked_eval.go:30-41). And the test
	// class with no runner, which 65 leading cd steps or a stray ")" derive.
	hidden := call{seq: 1, tool: "Bash", prog: "make", class: shape.VerbExecute, masked: shape.MaskedTest, outcome: store.ExecOK, loc: "d:repo"}
	unread := call{seq: 2, tool: "Bash", prog: "go", class: shape.VerbTest, outcome: store.ExecOK, loc: "d:repo"}
	write := call{seq: 3, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true, loc: "d:repo"}
	r := runOf(t, hidden, unread, write)
	runs, _, _, _ := classify(r)
	if len(runs) != 2 {
		t.Fatalf("runs = %+v, want the two runner-less gate lines and not the write", runs)
	}
	for _, g := range runs {
		if g.Runner != nil || g.Kind != kindCustom {
			t.Fatalf("run %+v: want no runner and kind custom", g)
		}
		if k, ok := g.family(); ok {
			t.Fatalf("run %+v has family %+v: a run of no known runner credits none", g, k)
		}
	}
	if str(runs[0].Masked) != shape.MaskedTest {
		t.Fatalf("the hidden runner's run lost its masked word: %+v", runs[0])
	}
}

func TestVER5b_LocationDomains(t *testing.T) {
	at := func(seq int64, loc string) call { c := gateCall(seq, "go-test", store.ExecOK); c.loc = loc; return c }
	r := runOf(t, at(1, "d:x"), at(2, "u:x"), at(3, ""))
	runs, _, _, _ := classify(r)
	if len(runs) != 3 {
		t.Fatalf("runs = %+v", runs)
	}
	read, okRead := runs[0].family()
	unreadKey, okUnread := runs[1].family()
	if !okRead || !okUnread {
		t.Fatalf("a read and an unread location are each a family: %v, %v", okRead, okUnread)
	}
	if read == unreadKey {
		t.Fatalf("an unread location's family equals a read one's: %+v", read)
	}
	if k, ok := runs[2].family(); ok {
		t.Fatalf("a location not known is no family, got %+v", k)
	}
}

func TestVER6_RewrittenGateLineIsNoFamily(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), gateCall(2, "go-test", store.ExecOK), edit(3, "a.go"))
	for i := range r.Declarations {
		r.Declarations[i].Shape.Digest = "as-declared"
		r.Executions[i].ExecutedDigest = "as-declared"
	}
	r.Executions[0].ExecutedDigest = "as-rewritten"
	r.Executions[2].ExecutedDigest = "as-rewritten" // an Edit a hook rewrote: not a gate run
	runs, _, _, rewritten := classify(r)
	if !slices.Equal(rewritten, []int64{declSeq(r, 0)}) {
		t.Fatalf("rewritten = %v, want [%d] (the Edit is not a gate run)", rewritten, declSeq(r, 0))
	}
	if len(runs) != 2 || runs[0].Location != "" || runs[1].Location != "d:repo" {
		t.Fatalf("runs = %+v, want the rewritten run at no location and the other at d:repo", runs)
	}
	if k, ok := runs[0].family(); ok {
		t.Fatalf("a rewritten line credits family %+v", k)
	}
}

func TestVER7_Relativise(t *testing.T) {
	for _, c := range []struct {
		name, path   string
		project, cwd *string
		want         string
	}{
		{"project prefix", "/repo/pkg/a.go", ptr("/repo"), nil, "pkg/a.go"},
		{"cwd prefix", "/work/b.go", ptr("/repo"), ptr("/work"), "b.go"},
		{"both match, project wins", "/repo/sub/a.go", ptr("/repo"), ptr("/repo/sub"), "sub/a.go"},
		{"outside both, kept", "/elsewhere/a.go", ptr("/repo"), ptr("/work"), "/elsewhere/a.go"},
		{"a prefix that is not a directory", "/repo2/a.go", ptr("/repo"), nil, "/repo2/a.go"},
		{"project with a trailing slash", "/repo/a.go", ptr("/repo/"), nil, "a.go"},
		{"the root is no prefix", "/a.go", ptr("/"), nil, "/a.go"},
		{"./x", "./x", nil, nil, "x"},
		{"././x", "././x", nil, nil, "x"},
		{"symlinked cwd: the path as written strips against CWD", "/tmp/repo/a.go", ptr("/private/tmp/repo"), ptr("/tmp/repo"), "a.go"},
		{"a path that does not exist here", "/nonexistent-ver7/repo/lib/a.go", ptr("/nonexistent-ver7/repo"), nil, "lib/a.go"},
		{"a path that does", "/usr/lib/a.go", ptr("/usr"), nil, "lib/a.go"},
		{"drive letter without case", `C:\repo\a.go`, ptr("c:/repo"), nil, "a.go"},
		{"backslashes", `sub\a.go`, nil, nil, "sub/a.go"},
		{"UNC under a UNC project", `\\server\share\repo\a.go`, ptr(`\\server\share\repo`), nil, "a.go"},
		{"UNC outside it", `\\other\share\a.go`, ptr(`\\server\share\repo`), nil, "//other/share/a.go"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := relativise(c.path, c.project, c.cwd); got != c.want {
				t.Fatalf("relativise(%q, %s, %s) = %q, want %q", c.path, str(c.project), str(c.cwd), got, c.want)
			}
		})
	}
	if ClassOf(relativise("/nonexistent-ver7/repo/lib/a.go", ptr("/nonexistent-ver7/repo"), nil)) != ClassOf(relativise("/usr/lib/a.go", ptr("/usr"), nil)) {
		t.Fatal("a path classifies by whether it exists on this machine")
	}

	r := runOf(t, edit(1, "/repo/docs/x.md"), edit(2, "a.go"))
	r.Declarations[0].Project = ptr("/repo")
	r.Declarations[1].FilePath = nil
	_, writes, unnamed, _ := classify(r)
	if len(writes) != 2 || str(writes[0].Path) != "docs/x.md" || writes[0].Class != ClassDocs || writes[1].Path != nil {
		t.Fatalf("writes = %+v, want docs/x.md stored relative and classed docs, then the nil FilePath unnamed", writes)
	}
	if !slices.Equal(unnamed, []int64{declSeq(r, 1)}) {
		t.Fatalf("unnamed = %v, want [%d]", unnamed, declSeq(r, 1))
	}
}

func TestVER8_OutputIsSeqOrderedAndMapIndependent(t *testing.T) {
	built := runOf(t, gateCall(1, "go-test", store.ExecOK), edit(2, "a.go"), gateCall(3, "go-vet", store.ExecFailed),
		call{seq: 4, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true},
		call{seq: 5, tool: "mcp__fs__write", class: shape.VerbMCP, outcome: store.ExecOK})
	for _, i := range []int{0, 2} { // both gate lines rewritten, so rewritten holds two seqs to order
		built.Declarations[i].Shape.Digest = "as-declared"
		built.Executions[i].ExecutedDigest = "as-rewritten"
	}
	built.Executions = slices.Delete(built.Executions, 1, 2) // the Edit's: an unmatched declaration
	wantRuns, wantWrites, wantUnnamed, wantRewritten := classify(built)
	if len(wantRuns) != 2 || len(wantWrites) != 3 || len(wantUnnamed) != 2 || len(wantRewritten) != 2 {
		t.Fatalf("premise: runs %+v, writes %+v, unnamed %v, rewritten %v", wantRuns, wantWrites, wantUnnamed, wantRewritten)
	}
	n := len(built.Declarations)
	for shift := 0; shift < n; shift++ {
		for _, reverse := range []bool{false, true} {
			r := &store.Run{Dir: built.Dir}
			for k := range n {
				i := (k + shift) % n
				if reverse {
					i = n - 1 - i
				}
				r.Declarations = append(r.Declarations, built.Declarations[i])
			}
			r.Executions = slices.Clone(built.Executions)
			slices.Reverse(r.Executions)
			runs, writes, unnamed, rewritten := classify(r)
			if !reflect.DeepEqual(runs, wantRuns) || !reflect.DeepEqual(writes, wantWrites) ||
				!slices.Equal(unnamed, wantUnnamed) || !slices.Equal(rewritten, wantRewritten) {
				t.Fatalf("shift %d reverse %v: runs %+v, writes %+v, unnamed %v, rewritten %v; want %+v, %+v, %v, %v",
					shift, reverse, runs, writes, unnamed, rewritten, wantRuns, wantWrites, wantUnnamed, wantRewritten)
			}
		}
	}

	// A spilled execution has no Seq; the run's is its declaration's.
	spilled := runOf(t, gateCall(1, "go-test", store.ExecOK))
	spilled.Executions[0].Seq = nil
	runs, _, _, _ := classify(spilled)
	if len(runs) != 1 || runs[0].Seq != declSeq(spilled, 0) {
		t.Fatalf("runs = %+v, want one at the declaration's seq %d", runs, declSeq(spilled, 0))
	}
}

// TestVER9_ThePickAgreesWithTheReport is an equivalence oracle: classify's
// pick of a call's execution (outranks) is a copy of report.outcomeRecord's,
// and the two copies must agree. report.MaskingAsRan replaces a declaration's
// runner digest with the picked record's, so the runner each side reports is
// the record each side picked. This is the only import of report under
// internal/verify.
func TestVER9_ThePickAgreesWithTheReport(t *testing.T) {
	for _, c := range []struct {
		name   string
		second func(first store.Execution) store.Execution
	}{
		{"a failed record before an ok one wins", func(e store.Execution) store.Execution {
			e.Seq, e.Outcome, e.RunnerDigest = ptr(*e.Seq-1), store.ExecFailed, ptr("r:failed")
			return e
		}},
		{"of two ok records the higher seq wins, read first", func(e store.Execution) store.Execution {
			e.Seq, e.RunnerDigest = ptr(*e.Seq+5), ptr("r:later")
			return e
		}},
		{"a record with a seq wins over one without", func(e store.Execution) store.Execution {
			e.Seq, e.RunnerDigest = nil, ptr("r:spilled")
			return e
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, gateCall(1, "go-test", store.ExecOK))
			r.Declarations[0].Shape.Digest = "line"
			r.Executions[0].ExecutedDigest = "line"
			second := c.second(r.Executions[0])
			r.Executions = []store.Execution{second, r.Executions[0]}

			runs, _, _, _ := classify(r)
			oracle := &store.Run{Declarations: slices.Clone(r.Declarations), Executions: slices.Clone(r.Executions)}
			report.MaskingAsRan(oracle)
			if len(runs) != 1 {
				t.Fatalf("runs = %+v, want one", runs)
			}
			if str(runs[0].Runner) != str(oracle.Declarations[0].Shape.RunnerDigest) {
				t.Fatalf("classify picked runner %s, the report %s", str(runs[0].Runner), str(oracle.Declarations[0].Shape.RunnerDigest))
			}
		})
	}
}

func TestTurnCounts_RewrittenGateSeq(t *testing.T) {
	snap := Snapshot{Recording: Complete}
	if got, want := TurnSummary(snap, TurnCounts{Writes: 1, RewrittenGateSeq: 7}).Row, TurnSummary(snap, TurnCounts{Writes: 1}).Row; got != want {
		t.Fatalf("RewrittenGateSeq moved the row: %q, want %q (nothing reads it yet)", got, want)
	}
}

// BenchmarkClassify is one classification of a run of 100 calls, a long
// turn's: gate runs of two runners, one of them failing, edits named by an
// absolute path under the project, named shell writes and unnamed ones. An
// evaluation classifies its run once, so this is that walk's cost.
func BenchmarkClassify(b *testing.B) {
	b.Run("run_100", func(b *testing.B) {
		r := benchRun(b, 100)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			classify(r)
		}
	})
}

func benchRun(b *testing.B, n int) *store.Run {
	calls := make([]call, 0, n)
	for i := range n {
		seq := int64(i + 1)
		switch i % 10 {
		case 0, 4:
			calls = append(calls, gateCall(seq, "go-test", store.ExecOK))
		case 7:
			calls = append(calls, gateCall(seq, "go-vet", store.ExecFailed))
		case 1, 2, 5, 8:
			calls = append(calls, edit(seq, "/repo/pkg/f"+strconv.Itoa(i)+".go"))
		case 3, 6:
			calls = append(calls, bashWrite(seq, "/repo/docs/n"+strconv.Itoa(i)+".md"))
		default:
			calls = append(calls, call{seq: seq, tool: "Bash", prog: "sed", class: shape.VerbExecute, outcome: store.ExecOK, mayWrite: true})
		}
	}
	r := runOf(b, calls...)
	project := "/repo"
	for i := range r.Declarations {
		r.Declarations[i].Project = &project
	}
	return r
}

// TestVER9b_ExecSeqIsThePickedExecutions: a gate run carries the Seq of the
// execution classify picked, which places its outcome in time (gateStates),
// and nil where that execution was spilled.
func TestVER9b_ExecSeqIsThePickedExecutions(t *testing.T) {
	spilled := gateCall(2, "go-test", store.ExecOK)
	spilled.spilled, spilled.term = true, store.OutcomeOK
	r := runOf(t, gateCall(1, "go-test", store.ExecOK), spilled, gateCall(3, "go-test", store.ExecOK))
	// A second record for the first call, failed: the pick's winner.
	failed := r.Executions[0]
	failed.Outcome, failed.Seq = store.ExecFailed, ptr(int64(9))
	r.Executions = append(r.Executions, failed)
	runs, _, _, _ := classify(r)
	var got []string
	for _, g := range runs {
		s := "nil"
		if g.ExecSeq != nil {
			s = strconv.FormatInt(*g.ExecSeq, 10)
		}
		got = append(got, s)
	}
	if want := []string{"9", "nil", "5"}; !slices.Equal(got, want) {
		t.Fatalf("ExecSeq %v, want %v: the picked execution's, nil when spilled", got, want)
	}
}
