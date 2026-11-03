package verify

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// maskedStatus is masked-status's producer at the act (card MASKED-AT-ACT1):
// at a PostToolUse, for each gate, in snapshot order, whose last run is this
// call and whose state is masked_inconclusive, the fact of that run. The
// state is the record's whole answer: the line hid the runner's status
// (masked), and neither a summary its stage kept (readsOutput) nor a status
// it printed (readsPrinted) said how the run ended. Said at the call, where
// the agent can still run the gate again, never at a failure event, where the
// line's own failure is the outcome the card does not raise at. Once per gate
// per prompt, by keyFor; F2 keeps the same fact from riding later turns.
func maskedStatus(snap Snapshot, runs []gateRun, pt Point, event, toolUseID string) []Finding {
	if event != string(steer.EventPostToolUse) {
		return nil
	}
	var out []Finding
	for i := range snap.Gates {
		g := &snap.Gates[i]
		if g.State != MaskedInconclusive || g.LastRun == nil || g.LastRun.ToolUseID != toolUseID {
			continue
		}
		last := g.LastRun.Seq
		fact := GateFact{GateID: SlotID(g.Gate), State: g.State, Reason: g.Reason, LastRunSeq: &last,
			PartialRuns: g.PartialRunsSince}
		if p := Program(programOf(runs, last)); p.Valid() {
			fact.Program = p
		}
		prompt := ""
		if pt.PromptID != nil {
			prompt = *pt.PromptID
		}
		out = append(out, Finding{Pattern: pattern.MaskedStatus, Class: Absence, Event: event,
			DedupeKey: string(pattern.MaskedStatus) + "|" + g.Gate + "|" + prompt, Gate: &fact})
	}
	return out
}

// renderMasked is masked-status's note, or the zero Message when the record
// holds no execution for the run the fact cites. The gate is named by its
// runner's row words where they fit the slot ("go vet", as the Stop names
// it), else as factGate names it; the stage by maskedStage, both from the
// execution the post wrote.
func renderMasked(run *store.Run, f Finding) render.Message {
	e := executionOf(run, *f.Gate.LastRunSeq)
	if e == nil {
		return render.Message{}
	}
	runner := ""
	if e.RunnerName != nil {
		runner = *e.RunnerName
	}
	m, _ := render.New(pattern.MaskedStatus, render.MaskedStatusBinding{Gate: factGate(f.Gate, runner),
		Stage: slot.NewEnum(slot.EnumMaskStage, maskedStage(e.MaskedBy))})
	return m
}

// factGate is the word a note names g's gate by: a declared gate by its id,
// the word its spec gave it (D222); else runner, the runner's row words,
// where it fits the slot ("" for a note that names the program); else the
// program; else the gate's kind.
func factGate(g *GateFact, runner string) slot.Gate {
	if id, ok := strings.CutPrefix(string(g.GateID), "spec:"); ok && slot.Gate(id).Validate() == nil {
		return slot.Gate(id)
	}
	if runner != "" && slot.Gate(runner).Validate() == nil {
		return slot.Gate(runner)
	}
	if g.Program == "" {
		return slot.Gate(kindOfGateID(string(g.GateID)))
	}
	return slot.Gate(g.Program)
}

// maskedStage is the words for what hid a run's exit status, from its
// execution's masked_by (call schema 8), as the Stop's sentence words a
// dropping stage (recap droppingStage): "the pipe to <stage>" for a stage
// the lists name, "a pipe" for one they do not, and "its line" where the
// record names none (a sequence, `go vet ./...; echo done`, or a record
// before schema 8). Unlike the Stop's, a stage that keeps the summary is
// named too: the note is said where the output did not say how the run
// ended, so that stage hid it. Every value is in slot.EnumMaskStage.
func maskedStage(by *string) string {
	switch {
	case by == nil || shape.MaskedByID(*by) == 0:
		return "its line"
	case *by == "other":
		return "a pipe"
	}
	return "the pipe to " + *by
}

// executionOf is the execution of the call declared at seq, the seq a gate
// fact cites, or nil: the declaration's tool_use_id, then the execution
// with that id (both records store the capped id). A scan, run once per
// masked-status fact.
func executionOf(run *store.Run, seq int64) *store.Execution {
	id := ""
	for i := range run.Declarations {
		if run.Declarations[i].Seq == seq {
			id = run.Declarations[i].ToolUseID
			break
		}
	}
	if id == "" {
		return nil
	}
	for i := range run.Executions {
		if e := &run.Executions[i]; e.ToolUseID == id {
			return e
		}
	}
	return nil
}
