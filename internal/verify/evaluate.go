package verify

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ProjectIndex is the per-project learned-gate index
// (`verify/projects/<project_key>.json`, 2.2.10). P-07 carries its identity
// only; its entries land with the evaluator (P-41).
type ProjectIndex struct {
	ProjectKey string `json:"project_key"`
}

// Evaluate is the evaluator of 2.2.4: it opens no file and has no clock or
// map-order dependence. Until P-41 builds it, it reports the recording as
// incomplete, which silences the comparator (2.2.5, Silence) and gives
// TurnSummary its unknown row, so no caller can read a pass out of an
// evaluator that does not exist yet.
func Evaluate(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, pt Point, now time.Time) Snapshot {
	snap := Snapshot{
		VerifyVersion:   Version,
		RulesVersion:    RulesVersion,
		PracticeVersion: PracticeVersion,
		Point:           pt,
		Recording:       Incomplete,
		Notes:           []string{"evaluator not built (P-41): recording reported incomplete"},
	}
	if sp != nil {
		// The golden names one path: the repository spec's, which is the last
		// source in precedence order (managed first), or the only one.
		info := &SpecInfo{Digest: sp.Digest, Trust: sp.Trust, Errors: []string{}}
		if n := len(sp.Sources); n > 0 {
			info.Path = sp.Sources[n-1].Path
		}
		snap.Spec = info
	}
	return snap
}

// TurnCounts is what the Stop hook counted for the turn (2.2.11).
// UnnamedWriteSeq is the turn's last call that may have written files the
// record cannot name (a Bash write with no known target, or an MCP call);
// zero means none.
type TurnCounts struct {
	Writes          int
	PathsWritten    int
	GateRuns        int
	LastWriteSeq    int64
	UnnamedWriteSeq int64
}

// Row is the end-of-turn value line's state row (2.2.11, first match wins).
type Row string

const (
	// RowNone is no line: the prompt had no write and no gate run.
	RowNone         Row = ""
	RowUnknown      Row = "unknown"
	RowRed          Row = "red"
	RowMasked       Row = "masked"
	RowBackgrounded Row = "backgrounded"
	RowRanOnly      Row = "ran_only"
	RowUnverified   Row = "unverified"
	RowNoGate       Row = "no_gate"
	RowUnnamedWrite Row = "unnamed_write"
	RowVerified     Row = "verified"
	RowFindings     Row = "findings"
)

// Rows is the table's order.
var Rows = []Row{
	RowUnknown, RowRed, RowMasked, RowBackgrounded, RowRanOnly,
	RowUnverified, RowNoGate, RowUnnamedWrite, RowVerified, RowFindings,
}

// SummaryMaxBytes bounds the value line (VER-65).
const SummaryMaxBytes = 16 << 10

// Summary is the end-of-turn value line: its row, and the sentence P-45
// renders for it (every output goes out through steer.Output.Marshal, TRU-10).
type Summary struct {
	Row      Row
	Sentence string
}

const sentenceRecordingIncomplete = "recording incomplete for this turn; verification state unknown"

// rowFor is the row of a non-green dominant state (2.2.11): stale and absent
// share `unverified`.
func rowFor(s State) Row {
	switch s {
	case Red:
		return RowRed
	case MaskedInconclusive:
		return RowMasked
	case Backgrounded:
		return RowBackgrounded
	case Stale, Absent:
		return RowUnverified
	}
	return RowUnknown
}

// TurnSummary picks the value line's row from the turn gates by the one Order
// (VER-65; VER 11 (a)). P-07 builds the row choice over the snapshot's
// required gates; P-45 adds the families run in the prompt, which need the
// prompt's runs, and renders the sentences.
//
// A gate state outside the vocabulary is a defect in the evaluator. It never
// reads as a pass, and it never hides a red: the valid states decide the row
// as usual, and the defect shows as unknown only when no valid non-green
// state outranks it.
func TurnSummary(snap Snapshot, turn TurnCounts) Summary {
	if turn.Writes == 0 && turn.GateRuns == 0 {
		return Summary{Row: RowNone}
	}
	if snap.Recording != Complete {
		return Summary{Row: RowUnknown, Sentence: sentenceRecordingIncomplete}
	}
	var states []State
	defect := false
	// The unnamed-write rule reads "after a turn gate's last green run": with
	// several gates, the earliest last green is the one a later unnamed write
	// can invalidate, and a gate with no green run counts as seq 0.
	earliestGreen := int64(-1)
	for _, g := range snap.Gates {
		if !g.Required {
			continue
		}
		if Rank(g.State) < 0 {
			defect = true
			continue
		}
		states = append(states, g.State)
		var green int64
		if g.LastGreen != nil {
			green = g.LastGreen.Seq
		}
		if earliestGreen < 0 || green < earliestGreen {
			earliestGreen = green
		}
	}
	worst, ok := Worst(states)
	switch {
	case !ok && !defect && turn.Writes == 0:
		return Summary{Row: RowNone}
	case !ok && !defect:
		return Summary{Row: RowNoGate}
	case ok && worst != Green:
		return Summary{Row: rowFor(worst)}
	case defect:
		return Summary{Row: RowUnknown}
	case turn.Writes == 0:
		return Summary{Row: RowRanOnly}
	case turn.UnnamedWriteSeq > earliestGreen:
		return Summary{Row: RowUnnamedWrite}
	case snap.RecordGreen:
		return Summary{Row: RowVerified}
	}
	return Summary{Row: RowFindings}
}
