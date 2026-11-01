package verify

import (
	"encoding/json"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
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

// TestWR1_AWrappedRunRaisesNoSteerNote: regression-location and
// stale-evidence have no row that says a run was read in a wrapper, so a
// gate whose red or green is wrapped raises neither (end to end:
// TestWR1_AWrappedRunRaisesNoNoteThatNamesAPipe). Controls: the same gates
// unwrapped raise each. Break: drop either guard.
func TestWR1_AWrappedRunRaisesNoSteerNote(t *testing.T) {
	code := 1
	red := func(last, green WrapWord) *GateState {
		return &GateState{State: Red, Reason: FailedExit, LastGreen: &RunRef{Seq: 1, Wrapped: green},
			LastRun: &RunRef{Seq: 3, ToolUseID: "toolu_r", ExitCode: &code, Wrapped: last}}
	}
	if !regressedHere(red(WrapNone, WrapNone), "toolu_r") {
		t.Fatal("premise: an unwrapped red after an unwrapped green raises no regression")
	}
	for _, c := range []struct{ last, green WrapWord }{{WrapRead, WrapNone}, {WrapNone, WrapRead}} {
		if regressedHere(red(c.last, c.green), "toolu_r") {
			t.Errorf("red %s after green %s raised a regression", c.last, c.green)
		}
	}
	path := "internal/a.go"
	writes := []write{{Seq: 2, Path: &path, Class: ClassSource}}
	stale := func(green WrapWord) *GateState {
		return &GateState{Required: true, State: Stale, Reason: WritesAfterGreen, LastGreen: &RunRef{Seq: 1, Wrapped: green}}
	}
	if !staleHere(stale(WrapNone), writes, 2) {
		t.Fatal("premise: a write after an unwrapped green raises no stale evidence")
	}
	if staleHere(stale(WrapRead), writes, 2) {
		t.Error("a write after a wrapped green raised stale evidence")
	}
}
