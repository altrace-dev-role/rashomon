package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// rh6Call is one test run of TestRH6: a plain run, whose line returns the
// runner's exit status, or a masked one, whose record may hold the runner's
// summary line (summary_pass, summary_fail) or the status the line printed
// (printed_pass, printed_fail).
type rh6Call struct {
	masked   bool
	outcome  string // store.ExecOK or store.ExecFailed
	kind     string // the record's GateOutput.Kind; "" for none
	delegate bool   // the runner is a delegate (make test)
}

// rh6Run builds the session: each masked call's record is at call schema 8,
// under the rules version its kind needs (11 for a summary, 12 for a printed
// status), with a tail stage that keeps the summary.
func rh6Run(calls ...rh6Call) *store.Run {
	var tb []tbCall
	for i, c := range calls {
		seq := int64(i + 1)
		if c.masked {
			tb = append(tb, maskedCall(seq, shape.MaskedTest, "r", c.outcome))
		} else {
			tb = append(tb, plainRun(seq, "r", c.outcome))
		}
	}
	run := tbRun(tb...)
	for i, c := range calls {
		if !c.masked {
			continue
		}
		x := &run.Executions[i]
		tail := "tail"
		x.SchemaVersion, x.RulesVersion, x.MaskedBy = 8, shape.OutputOutcomeRulesVersion, &tail
		if strings.HasPrefix(c.kind, "printed_") {
			x.RulesVersion = shape.PrintedStatusRulesVersion
		}
		if c.kind != "" {
			x.GateOutput = &store.GateOutput{Kind: c.kind, Parser: "pytest"}
		}
		if c.delegate {
			name := "make test"
			x.RunnerName = &name
		}
	}
	return run
}

// rh6Wrapped adds to run, at seq, a call countWrapped counts: a bash line
// that launches and names no runner, which ended ok, and whose record holds a
// runner's summary line of kind.
func rh6Wrapped(run *store.Run, seq int64, kind string) {
	w := tbRun(tbCall{seq: seq, tool: "Bash", verb: shape.VerbExecute, prog: "bash", outcome: store.ExecOK})
	w.Declarations[0].Launch = true
	w.Executions[0].GateOutput = &store.GateOutput{Kind: kind, Parser: "pytest"}
	run.Declarations = append(run.Declarations, w.Declarations...)
	run.Executions = append(run.Executions, w.Executions...)
}

func rh6Text(tr *TestRuns) string {
	var b bytes.Buffer
	writeTestRuns(&b, tr, "s1")
	return b.String()
}

func rh6JSON(tr *TestRuns) string {
	j, _ := json.Marshal(tr)
	return string(j)
}

// rh6Pair is one of report --json's read counts, read_from_summary or
// read_from_printed.
type rh6Pair struct {
	OK     int `json:"ok"`
	Failed int `json:"failed"`
}

// rh6Sum checks tr's JSON against the text's totals, which each case holds
// as literals: ok, read_from_summary's ok and read_from_printed's ok add up
// to the text's ok, and the three failed counts to its failed, so the text
// and the JSON cannot drift apart. A null part adds 0.
func rh6Sum(t *testing.T, tr *TestRuns, textOK, textFailed int) {
	t.Helper()
	var j struct {
		OK      int      `json:"ok"`
		Failed  int      `json:"failed"`
		Summary *rh6Pair `json:"read_from_summary"`
		Printed *rh6Pair `json:"read_from_printed"`
	}
	if err := json.Unmarshal([]byte(rh6JSON(tr)), &j); err != nil {
		t.Fatalf("json: %v", err)
	}
	ok, failed := j.OK, j.Failed
	for _, p := range []*rh6Pair{j.Summary, j.Printed} {
		if p != nil {
			ok, failed = ok+p.OK, failed+p.Failed
		}
	}
	if ok != textOK || failed != textFailed {
		t.Errorf("json parts add up to %d ok and %d failed; the text says %d and %d: %s", ok, failed, textOK, textFailed, rh6JSON(tr))
	}
}

const rh6Where = "    where each outcome was read: "

// TestRH6_PipedRunReadFromOutputCounts (report.md item 6): a masked test run
// whose record holds the runner's summary line (readFromSummary) or the status
// its line printed (readFromPrinted) is counted passed or failed by that read,
// and the test-run line says where each outcome was read: the exit status, the
// summary line, the printed status. The JSON keeps ok, failed and
// status_masked as they were and adds read_from_summary and read_from_printed
// (report version 2: a key is added, none changes meaning). A masked run with
// nothing read, one that ended failed, and a delegate's pass line stay
// "neither". Break: drop the readFromSummary branch (mutant RH-6) and the
// summary rows stay "neither".
func TestRH6_PipedRunReadFromOutputCounts(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	plainOK := rh6Call{outcome: ok}

	// Each kind at counts 0, 1 and 2, beside one plain run that passed, so the
	// block prints at 0 too. Every total is a literal, not a sum the test
	// computes: the sum is what rh6Sum checks the JSON against.
	for _, c := range []struct {
		kind                               string
		n                                  int
		textOK, textFailed                 int
		sumOK, sumFailed, prtOK, prtFailed int
	}{
		{"summary_pass", 0, 1, 0, 0, 0, 0, 0},
		{"summary_pass", 1, 2, 0, 1, 0, 0, 0},
		{"summary_pass", 2, 3, 0, 2, 0, 0, 0},
		{"summary_fail", 0, 1, 0, 0, 0, 0, 0},
		{"summary_fail", 1, 1, 1, 0, 1, 0, 0},
		{"summary_fail", 2, 1, 2, 0, 2, 0, 0},
		{"printed_pass", 0, 1, 0, 0, 0, 0, 0},
		{"printed_pass", 1, 2, 0, 0, 0, 1, 0},
		{"printed_pass", 2, 3, 0, 0, 0, 2, 0},
		{"printed_fail", 0, 1, 0, 0, 0, 0, 0},
		{"printed_fail", 1, 1, 1, 0, 0, 0, 1},
		{"printed_fail", 2, 1, 2, 0, 0, 0, 2},
	} {
		t.Run(fmt.Sprintf("%s/%d", c.kind, c.n), func(t *testing.T) {
			calls := []rh6Call{plainOK}
			for i := 0; i < c.n; i++ {
				calls = append(calls, rh6Call{masked: true, outcome: ok, kind: c.kind})
			}
			tr := testRunsOf(rh6Run(calls...))
			text := rh6Text(tr)
			if want := fmt.Sprintf("  test runs: %d (%d ok, %d failed)\n", 1+c.n, c.textOK, c.textFailed); !strings.Contains(text, want) {
				t.Errorf("text is missing %q:\n%s", want, text)
			}
			rh6Sum(t, tr, c.textOK, c.textFailed)
			where := fmt.Sprintf("%sthe exit status, 1 ok and 0 failed; the runner's summary line, %d ok and %d failed; the status the line printed, %d ok and %d failed\n",
				rh6Where, c.sumOK, c.sumFailed, c.prtOK, c.prtFailed)
			if c.n == 0 && strings.Contains(text, rh6Where) {
				t.Errorf("nothing was read from output, yet the text says where:\n%s", text)
			}
			if c.n > 0 && !strings.Contains(text, where) {
				t.Errorf("text is missing %q:\n%s", where, text)
			}
			j := rh6JSON(tr)
			for _, want := range []string{
				fmt.Sprintf(`"runs":%d,"ok":1,"failed":0,"status_masked":%d`, 1+c.n, c.n),
				fmt.Sprintf(`"read_from_summary":{"ok":%d,"failed":%d}`, c.sumOK, c.sumFailed),
				fmt.Sprintf(`"read_from_printed":{"ok":%d,"failed":%d}`, c.prtOK, c.prtFailed),
			} {
				if !strings.Contains(j, want) {
					t.Errorf("json is missing %s: %s", want, j)
				}
			}
		})
	}

	// Controls: each stays "neither", before item 6 and after it. A masked run
	// is read only when its call ended ok, as verify.readsOutput and
	// BuildMaskedRuns read it: a summary of either kind beside a call that
	// ended failed stays neither.
	for _, c := range []struct {
		name string
		call rh6Call
	}{
		{"unread stays neither", rh6Call{masked: true, outcome: ok}},
		{"a fail line stays neither", rh6Call{masked: true, outcome: ok, kind: "fail_line"}},
		{"ended failed stays neither", rh6Call{masked: true, outcome: failed, kind: "summary_pass"}},
		{"a fail summary that ended failed stays neither", rh6Call{masked: true, outcome: failed, kind: "summary_fail"}},
		{"a delegate's pass line stays neither", rh6Call{masked: true, outcome: ok, kind: "summary_pass", delegate: true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			tr := testRunsOf(rh6Run(plainOK, c.call))
			text := rh6Text(tr)
			want := "  test runs: 2 (1 ok, 0 failed, 1 whose line did not return the test run's exit status, so neither)\n"
			if !strings.Contains(text, want) || strings.Contains(text, rh6Where) {
				t.Errorf("want %q and no %q:\n%s", want, rh6Where, text)
			}
			rh6Sum(t, tr, 1, 0)
		})
	}

	// The breakdown over every source at once, and the one-clause summary of
	// the session's record line.
	t.Run("breakdown", func(t *testing.T) {
		tr := testRunsOf(rh6Run(
			plainOK, rh6Call{outcome: failed},
			rh6Call{masked: true, outcome: ok, kind: "summary_pass"}, rh6Call{masked: true, outcome: ok, kind: "summary_pass"},
			rh6Call{masked: true, outcome: ok, kind: "summary_fail"},
			rh6Call{masked: true, outcome: ok, kind: "printed_pass"},
			rh6Call{masked: true, outcome: ok, kind: "printed_fail"}, rh6Call{masked: true, outcome: ok, kind: "printed_fail"},
			rh6Call{masked: true, outcome: ok},
		))
		text := rh6Text(tr)
		for _, want := range []string{
			"  test runs: 9 (4 ok, 4 failed, 1 whose line did not return the test run's exit status, so neither)\n",
			rh6Where + "the exit status, 1 ok and 1 failed; the runner's summary line, 2 ok and 1 failed; the status the line printed, 1 ok and 2 failed\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("text is missing %q:\n%s", want, text)
			}
		}
		j := rh6JSON(tr)
		for _, want := range []string{
			`"runs":9,"ok":1,"failed":1,"status_masked":7`,
			`"read_from_summary":{"ok":2,"failed":1}`,
			`"read_from_printed":{"ok":1,"failed":2}`,
		} {
			if !strings.Contains(j, want) {
				t.Errorf("json is missing %s: %s", want, j)
			}
		}
		if got, want := callOutcomes(Session{TestRuns: tr}), "; 9 test runs (4 ok, 4 failed)"; !strings.HasSuffix(got, want) {
			t.Errorf("callOutcomes = %q, want the suffix %q", got, want)
		}
		rh6Sum(t, tr, 4, 4)
	})

	// A wrapped run (countWrapped) is in ok or failed by a runner's summary
	// line in a script's output, not by the call's exit status: this one's
	// call ended ok and its summary said failed. The exit-status clause says
	// so for the runs it holds that way, or it would credit the exit status
	// with a failure it never returned.
	t.Run("wrapped", func(t *testing.T) {
		run := rh6Run(plainOK, rh6Call{masked: true, outcome: ok, kind: "summary_pass"})
		rh6Wrapped(run, 3, "summary_fail")
		tr := testRunsOf(run)
		if tr == nil || tr.Wrapped != 1 || tr.Runs != 3 || tr.Failed != 1 {
			t.Fatalf("fixture: test runs %+v, want 1 wrapped of 3 runs, 1 failed", tr)
		}
		text := rh6Text(tr)
		for _, want := range []string{
			"  test runs: 3 (2 ok, 1 failed)\n",
			rh6Where + "the exit status, 1 ok and 1 failed (1 of them run by a script or a shell, and read from the runner's summary line in its output); the runner's summary line, 1 ok and 0 failed; the status the line printed, 0 ok and 0 failed\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("text is missing %q:\n%s", want, text)
			}
		}
		j := rh6JSON(tr)
		for _, want := range []string{
			`"runs":3,"ok":1,"failed":1,"status_masked":1`,
			`"wrapped":1,"read_from_summary":{"ok":1,"failed":0},"read_from_printed":{"ok":0,"failed":0}`,
		} {
			if !strings.Contains(j, want) {
				t.Errorf("json is missing %s: %s", want, j)
			}
		}
		if got, want := callOutcomes(Session{TestRuns: tr}), "; 3 test runs (2 ok, 1 failed)"; !strings.HasSuffix(got, want) {
			t.Errorf("callOutcomes = %q, want the suffix %q", got, want)
		}
		rh6Sum(t, tr, 2, 1)
	})

	// A session that never measured masking has no masked count, and so no
	// read counts: null, not a zero nobody measured.
	t.Run("unmeasured is null", func(t *testing.T) {
		run := tbRun(test(1, "d", ok))
		run.Declarations[0].SchemaVersion = 3
		tr := testRunsOf(run)
		j := rh6JSON(tr)
		for _, want := range []string{`"status_masked":null`, `"read_from_summary":null`, `"read_from_printed":null`} {
			if !strings.Contains(j, want) {
				t.Errorf("json is missing %s: %s", want, j)
			}
		}
		if text, want := rh6Text(tr), "  test runs: 1 (1 ok, 0 failed)\n"; !strings.Contains(text, want) || strings.Contains(text, rh6Where) {
			t.Errorf("want %q and no %q:\n%s", want, rh6Where, text)
		}
		rh6Sum(t, tr, 1, 0)
	})
}
