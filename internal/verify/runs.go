package verify

import "github.com/altrace-dev-role/rashomon/internal/store"

// RunClass is the class of one gate run's outcome, for a reader that counts
// runs (the prescan's per-session summary, docs/autosteer-prescan.md §6.3).
// It is the same reading of the run the gate table makes (green, failed,
// masked, empty), with the exit codes that are no verdict on the code split
// out, as the doc rules: 124 is a timeout and 126 and 127 are a gate that did
// not run, each its own class of unknown cause, never folded into red.
type RunClass string

const (
	// RunGreen: the run is a green by the table (green).
	RunGreen RunClass = "green"
	// RunRed: the run failed by its exit or its output, with a code that is
	// not 124, 126 or 127 (or none).
	RunRed RunClass = "red"
	// RunTimeout: exit 124.
	RunTimeout RunClass = "timeout"
	// RunNotRun: exit 126 or 127.
	RunNotRun RunClass = "not_run"
	// RunOther: a run that is neither: backgrounded, its status masked, no
	// tests ran, or its outcome not known.
	RunOther RunClass = "other"
)

// GateRun is one gate run of a run, as classify reads it. It is a view of the
// classification Evaluate and the Stop view are built on, not another one.
type GateRun struct {
	Seq       int64
	ToolUseID string
	AgentID   *string
	// Family is the GateState.Family the run belongs to (runner "@" location),
	// "" for a run with none: where it ran, or its runner, is not known.
	Family  string
	Kind    string
	Program string
	Class   RunClass
}

// GateRuns lists the gate runs of run in seq order, each classified. A nil run
// has none.
func GateRuns(run *store.Run) []GateRun {
	runs, _, _, _ := classify(run)
	out := make([]GateRun, 0, len(runs))
	for i := range runs {
		r := &runs[i]
		g := GateRun{Seq: r.Seq, ToolUseID: r.ToolUseID, AgentID: r.AgentID, Kind: r.Kind, Program: r.Program, Class: classOf(r)}
		if k, ok := r.family(); ok {
			g.Family = k.runner + "@" + k.location
		}
		out = append(out, g)
	}
	return out
}

func classOf(r *gateRun) RunClass {
	switch {
	case green(r):
		return RunGreen
	case r.Backgrounded || masked(r) || emptyRun(r):
		return RunOther
	case failedByExit(r) && r.ExitCode != nil:
		switch *r.ExitCode {
		case 124:
			return RunTimeout
		case 126, 127:
			return RunNotRun
		}
		return RunRed
	case failed(r):
		return RunRed
	}
	return RunOther
}
