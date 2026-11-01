package verify

import (
	"encoding/json"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestWR1_AWrappedRunIsDecidedByItsOutput: a run a declared gate bound by its
// output (wrapped) is red only where its runner's summary failed, green only
// where it passed and the call ended ok, and unknown otherwise -- never red by
// the wrapper's exit status, and never masked by its line, whose status is
// the wrapper's. Controls: the same output on a run its line named, which
// the exit status and the fail line still decide. Break: let failed read the
// exit of a wrapped run, or masked hide one.
func TestWR1_AWrappedRunIsDecidedByItsOutput(t *testing.T) {
	maskedTest := shape.MaskedTest
	for _, c := range []struct {
		name          string
		outcome, kind string
		masked        *string
		wrap          bool
		state         State
		reason        Reason
	}{
		{"wrapped pass, call ok", store.ExecOK, outSummaryPass, nil, true, Green, OK},
		{"wrapped pass, line masked (make test | tail)", store.ExecOK, outSummaryPass, &maskedTest, true, Green, OK},
		{"wrapped pass, call failed", store.ExecFailed, outSummaryPass, nil, true, Unknown, OutcomeUnknown},
		{"wrapped fail, call ok (a pipe kept the exit 0)", store.ExecOK, outSummaryFail, nil, true, Red, FailedOutput},
		{"wrapped fail, call failed", store.ExecFailed, outSummaryFail, nil, true, Red, FailedOutput},
		{"wrapped empty run", store.ExecOK, outEmptyRun, nil, true, Absent, EmptyRun},
		{"wrapped fail line only, call failed", store.ExecFailed, outFailLine, nil, true, Unknown, OutcomeUnknown},
		{"wrapped no summary", store.ExecOK, outNoSummary, nil, true, Unknown, OutcomeUnknown},
		{"wrapped no summary, line masked", store.ExecOK, outNoSummary, &maskedTest, true, Unknown, OutcomeUnknown},
		{"control: named run, call failed, pass line", store.ExecFailed, outSummaryPass, nil, false, Red, FailedExit},
		{"control: named run, fail line", store.ExecOK, outFailLine, nil, false, Red, FailedOutput},
		{"control: named run masked, pass", store.ExecOK, outSummaryPass, &maskedTest, false, MaskedInconclusive, MaskedWithSuccessLine},
	} {
		t.Run(c.name, func(t *testing.T) {
			exec := int64(6)
			r := gateRun{Seq: 5, ExecSeq: &exec, ToolUseID: "toolu_w", Outcome: c.outcome, Masked: c.masked,
				Output: &store.GateOutput{Kind: c.kind, Parser: "go"}}
			if c.wrap {
				r = wrapped(r)
			}
			g := stateOf(familyKey{runner: InferredGateID}, "test", []gateRun{r}, nil, nil, 10, nil)
			if g.State != c.state || g.Reason != c.reason {
				t.Errorf("state %s %s, want %s %s", g.State, g.Reason, c.state, c.reason)
			}
			if want := map[bool]WrapWord{true: WrapRead, false: WrapNone}[c.wrap]; g.LastRun.Wrapped != want {
				t.Errorf("last run wrapped %q, want %q", g.LastRun.Wrapped, want)
			}
		})
	}
}

// TestWR1_AnUnreadWrapperAfterTheLastEditIsUnknown (D5): a gate absent or
// stale whose last relevant edit a wrapper call follows, a call that bound
// nothing to it, is Unknown with that call as its last run, marked unread;
// an edit after the call, or a run of the gate after it, leaves the gate as
// it was. Break: drop the unread rule, or let it override a green.
func TestWR1_AnUnreadWrapperAfterTheLastEditIsUnknown(t *testing.T) {
	path := "internal/a.go"
	edit := func(seq int64) write { return write{Seq: seq, Path: &path, Class: ClassSource, Tool: "Edit"} }
	run := func(seq int64, kind string) gateRun {
		exec := seq + 1
		return gateRun{Seq: seq, ExecSeq: &exec, ToolUseID: "toolu_r", Outcome: store.ExecOK,
			Output: &store.GateOutput{Kind: kind, Parser: "go"}}
	}
	unreadCall := func(seq int64) *gateRun {
		exec := seq + 1
		return &gateRun{Seq: seq, ExecSeq: &exec, ToolUseID: "toolu_u", Outcome: store.ExecOK}
	}
	g := declaredGate{id: InferredGateID, kind: "test", program: "go"}
	for _, c := range []struct {
		name   string
		fr     []gateRun
		unread *gateRun
		writes []write
		state  State
		reason Reason
		wrap   WrapWord
	}{
		{"no run, call after the edit", nil, unreadCall(4), []write{edit(2)}, Unknown, OutcomeUnknown, WrapUnread},
		{"green, edit, call after it", []gateRun{run(1, outSummaryPass)}, unreadCall(4), []write{edit(3)}, Unknown, OutcomeUnknown, WrapUnread},
		{"control: no call", nil, nil, []write{edit(2)}, Absent, NeverRunAfterWrite, WrapNone},
		{"control: call before the edit", nil, unreadCall(2), []write{edit(3)}, Absent, NeverRunAfterWrite, WrapNone},
		{"control: a run of the gate after the call", []gateRun{run(5, outSummaryPass)}, unreadCall(4), []write{edit(2)}, Green, OK, WrapNone},
		{"control: no edit", nil, unreadCall(4), nil, Absent, EmptyRun, WrapNone},
		{"control: red stays red", []gateRun{run(3, outSummaryFail)}, unreadCall(4), []write{edit(2)}, Red, FailedOutput, WrapNone},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := g.state(c.fr, c.unread, c.writes, nil, 10, nil)
			if s.State != c.state || s.Reason != c.reason {
				t.Errorf("state %s %s, want %s %s", s.State, s.Reason, c.state, c.reason)
			}
			var wrap WrapWord
			if s.LastRun != nil {
				wrap = s.LastRun.Wrapped
			}
			if wrap != c.wrap {
				t.Errorf("last run wrapped %q, want %q", wrap, c.wrap)
			}
		})
	}
}

// TestWR1_AWrapperBindsOnlyByItsRunnerAtTheGatesLocation: a gate binds a
// wrapper's run where the gate's runner's row read its output at the gate's
// location (readBy, D6), never another runner's read, another location's,
// a call that is no wrapper, or a gate with no runner row; and a wrapped
// last run is never marked as other arguments. Break: drop a term of readBy,
// or let state mark a wrapped run OtherArgs.
func TestWR1_AWrapperBindsOnlyByItsRunnerAtTheGatesLocation(t *testing.T) {
	read := func(parser, location string, wrapper bool) *lineRun {
		return &lineRun{gateRun: gateRun{Seq: 4, Location: location, Outcome: store.ExecOK,
			Output: &store.GateOutput{Kind: outSummaryPass, Parser: parser}}, wrapper: wrapper}
	}
	g := declaredGate{id: InferredGateID, kind: "test", program: "go", location: "here", words: map[string]bool{"go-words": true}}
	for _, c := range []struct {
		name string
		gate declaredGate
		run  *lineRun
		want bool
	}{
		{"go's read, here", g, read("go", "here", true), true},
		{"pytest's read", g, read("pytest", "here", true), false},
		{"a delegate's own word", g, read("make", "here", true), false},
		{"another location", g, read("go", "there", true), false},
		{"no wrapper (a cat of a log)", g, read("go", "here", false), false},
		{"a gate with no runner row (make test)", declaredGate{id: "spec:t", location: "here"}, read("", "here", true), false},
	} {
		if got := c.gate.readBy(c.run); got != c.want {
			t.Errorf("%s: readBy %v, want %v", c.name, got, c.want)
		}
	}
	make := "make-words"
	exec := int64(5)
	r := wrapped(gateRun{Seq: 4, ExecSeq: &exec, Runner: &make, Outcome: store.ExecOK, Output: &store.GateOutput{Kind: outSummaryPass, Parser: "go"}})
	s := g.state([]gateRun{r}, nil, nil, nil, 10, nil)
	if s.LastRun == nil || s.LastRun.OtherArgs || s.LastRun.Wrapped != WrapRead {
		t.Errorf("wrapped last run %+v, want wrapped read and no other_args", s.LastRun)
	}
}

// TestWR1_TheWrapWordKeepsItsJSON: wrapped marshals as its word, is left out
// as none, and reads back.
func TestWR1_TheWrapWordKeepsItsJSON(t *testing.T) {
	for _, w := range []WrapWord{WrapRead, WrapUnread} {
		b, err := json.Marshal(RunRef{Wrapped: w})
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		var back RunRef
		if json.Unmarshal(b, &m) != nil || m["wrapped"] != w.String() || json.Unmarshal(b, &back) != nil || back.Wrapped != w {
			t.Errorf("%q marshals %s, reads back %q", w, b, back.Wrapped)
		}
	}
	b, _ := json.Marshal(RunRef{})
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["wrapped"]; ok {
		t.Errorf("a run read by its line marshals wrapped: %s", b)
	}
}

// TestRS1_AWrappedRunIsANoteCandidate: a gate whose red or green was read in
// a wrapper is regression-location's candidate, and a wrapped green is
// stale-evidence's, as an unwrapped one is: the rows say it ran inside
// another command (WRAPPER-RED-STEP1). Controls: an unwrapped red after an
// unwrapped green, and an unread wrapper, which decides nothing. Break: keep
// WRAPPER-RUN1's refusal of either.
func TestRS1_AWrappedRunIsANoteCandidate(t *testing.T) {
	code := 1
	red := func(last, green WrapWord) *GateState {
		return &GateState{State: Red, Reason: FailedExit, LastGreen: &RunRef{Seq: 1, Wrapped: green},
			LastRun: &RunRef{Seq: 3, ToolUseID: "toolu_r", ExitCode: &code, Wrapped: last}}
	}
	for _, c := range []struct{ last, green WrapWord }{{WrapNone, WrapNone}, {WrapRead, WrapNone}, {WrapNone, WrapRead}, {WrapRead, WrapRead}} {
		if !regressedHere(red(c.last, c.green), "toolu_r") {
			t.Errorf("red %q after green %q raised no regression", c.last, c.green)
		}
	}
	path := "internal/a.go"
	writes := []write{{Seq: 2, Path: &path, Class: ClassSource}}
	stale := func(green WrapWord) *GateState {
		return &GateState{Required: true, State: Stale, Reason: WritesAfterGreen, LastGreen: &RunRef{Seq: 1, Wrapped: green}}
	}
	for _, green := range []WrapWord{WrapNone, WrapRead} {
		if !staleHere(stale(green), writes, 2) {
			t.Errorf("a write after green %q raised no stale evidence", green)
		}
	}
}

// TestRS1_TheRowSaysWhereTheRunWasRead: regression-location and
// stale-evidence say a run read in a wrapper ran inside another command,
// never that a pipe hid it, and never that a wrapped green exited 0; with no
// path to name, the degraded row says the same. Controls: a piped red keeps
// D229's row, after a wrapped green too (the pipe is this run's, and
// "passed earlier" is true of both greens), and a red by exit after an
// unwrapped green keeps the exit row. Break: choose a row by OutcomeFrom
// alone, or the exit row for a wrapped green.
func TestRS1_TheRowSaysWhereTheRunWasRead(t *testing.T) {
	r := runOf(t, gateCall(2, "go", store.ExecOK), edit(3, "internal/a.go"), gateCall(8, "go", store.ExecFailed))
	minuteClock(t, r)
	seqOf := func(id string) int64 {
		for _, d := range r.Declarations {
			if d.ToolUseID == id {
				return d.Seq
			}
		}
		t.Fatalf("no declaration %s", id)
		return 0
	}
	green, wrote, red := seqOf("t2"), seqOf("t3"), seqOf("t8")
	exit := 1
	const (
		inside  = "(read from its output; it ran inside another command)"
		between = " Files written between the two runs: 'internal/a.go'."
		counted = " 1 file writes were recorded between the two runs."
	)
	fact := func(f GateFact, named bool) GateFact {
		f.Program, f.LastGreenSeq, f.LastRunSeq, f.WritesSince = "go", &green, &red, 1
		if named {
			f.Paths = []PathSlot{"internal/a.go"}
		}
		return f
	}
	for _, c := range []struct {
		name  string
		fact  GateFact
		named bool
		want  string
	}{
		{"wrapped red", GateFact{OutcomeFrom: OutcomeFromOutput, RedWrapped: true}, true,
			"gate 'go' passed earlier in this session (6 min ago) and its output reports a failure on this run " + inside + "." + between},
		{"wrapped red, no path", GateFact{OutcomeFrom: OutcomeFromOutput, RedWrapped: true}, false,
			"gate 'go' passed earlier in this session (6 min ago) and its output reports a failure on this run " + inside + "." + counted},
		{"red by exit after a wrapped green", GateFact{ExitCode: &exit, GreenWrapped: true}, true,
			"gate 'go' passed earlier in this session (6 min ago; read from its output, it ran inside another command) and exited 1 on this run." + between},
		{"red by exit after a wrapped green, no path", GateFact{ExitCode: &exit, GreenWrapped: true}, false,
			"gate 'go' passed earlier in this session (6 min ago; read from its output, it ran inside another command) and exited 1 on this run." + counted},
		{"control: piped red", GateFact{OutcomeFrom: OutcomeFromOutput}, true,
			"gate 'go' passed earlier in this session (6 min ago) and its output reports a failure on this run (read from its output; the pipe hid the exit status)." + between},
		{"control: piped red after a wrapped green", GateFact{OutcomeFrom: OutcomeFromOutput, GreenWrapped: true}, true,
			"gate 'go' passed earlier in this session (6 min ago) and its output reports a failure on this run (read from its output; the pipe hid the exit status)." + between},
		{"control: red by exit", GateFact{ExitCode: &exit}, true,
			"gate 'go' exited 0 earlier in this session (6 min ago) and exited 1 on this run." + between},
	} {
		f := fact(c.fact, c.named)
		m := renderRegression(r, &declTimes{run: r}, Finding{Pattern: pattern.RegressionLocation, Gate: &f})
		if got := m.Text(); got != "rashomon record: "+c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, "rashomon record: "+c.want)
		}
	}
	for _, c := range []struct {
		name  string
		fact  GateFact
		named bool
		want  string
	}{
		{"wrapped green", GateFact{OutcomeFrom: OutcomeFromOutput, GreenWrapped: true}, true,
			"gate 'go' passed earlier in this session (1 min ago), read from its output as it ran inside another command; this call then wrote 'internal/a.go'. No later run of the gate is recorded."},
		{"wrapped green, no path", GateFact{OutcomeFrom: OutcomeFromOutput, GreenWrapped: true}, false,
			"gate 'go' passed earlier in this session (1 min ago), read from its output as it ran inside another command; this call then wrote 1 files. No later run of the gate is recorded."},
		{"control: piped green", GateFact{OutcomeFrom: OutcomeFromOutput}, true,
			"gate 'go' passed earlier in this session (1 min ago), read from its output as the pipe hid the exit status; this call then wrote 'internal/a.go'. No later run of the gate is recorded."},
	} {
		f := fact(c.fact, c.named)
		m := renderStale(&declTimes{run: r}, Finding{Pattern: pattern.StaleEvidence, Gate: &f}, wrote)
		if got := m.Text(); got != "rashomon record: "+c.want {
			t.Errorf("stale %s:\n got %q\nwant %q", c.name, got, "rashomon record: "+c.want)
		}
	}
}

// TestRS1_TheFactSaysWhichRunWasWrapped: regression-location's fact carries
// whether its red and its green were read in a wrapper, and names a wrapped
// red by the runner whose table read it (go), never by the wrapper's
// program (bash). Control: unwrapped runs carry neither, and keep the run's
// program. Break: drop either flag, or name the wrapper.
func TestRS1_TheFactSaysWhichRunWasWrapped(t *testing.T) {
	path := "internal/a.go"
	writes := []write{{Seq: 3, Path: &path, Class: ClassSource, Tool: "Edit"}}
	code := 1
	for _, c := range []struct {
		name               string
		red, green         WrapWord
		program            string
		redWrapped, greenW bool
		want               Program
	}{
		{"wrapped red", WrapRead, WrapNone, "bash", true, false, "go"},
		{"red by exit after a wrapped green", WrapNone, WrapRead, "go", false, true, "go"},
		{"control: neither", WrapNone, WrapNone, "go", false, false, "go"},
	} {
		last := &RunRef{Seq: 5, ToolUseID: "toolu_r", Wrapped: c.red, GateOutput: &store.GateOutput{Kind: outSummaryFail, Parser: "go"}}
		if c.red == WrapRead {
			last.OutcomeFrom = SourceOutput
		} else {
			last.ExitCode = &code
		}
		snap := Snapshot{Gates: []GateState{{Gate: "spec:unit", State: Red, Reason: FailedOutput,
			LastGreen: &RunRef{Seq: 1, Wrapped: c.green}, LastRun: last}}}
		if c.red == WrapNone {
			snap.Gates[0].Reason = FailedExit
		}
		got := regressionLocations(snap, []gateRun{{Seq: 5, Program: c.program}}, writes, Point{}, stEvent, "toolu_r")
		if len(got) != 1 || got[0].Gate == nil {
			t.Fatalf("%s: %d findings, want 1", c.name, len(got))
		}
		f := got[0].Gate
		if f.RedWrapped != c.redWrapped || f.GreenWrapped != c.greenW || f.Program != c.want {
			t.Errorf("%s: red wrapped %v green wrapped %v program %q, want %v %v %q", c.name, f.RedWrapped, f.GreenWrapped, f.Program, c.redWrapped, c.greenW, c.want)
		}
	}
}
