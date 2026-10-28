package verify

import (
	"slices"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ProjectIndex is the per-project learned-gate index (2.2.10): its file is
// IndexPath's, and index.go reads, observes and writes it. Entries is sorted
// by Family, and never null in the encoding.
type ProjectIndex struct {
	ProjectKey string       `json:"project_key"`
	Entries    []IndexEntry `json:"entries"`
}

// Evaluate is the evaluator of 2.2.4: a pure fold from the record, the index,
// the spec and the point to a Snapshot. It opens no file, reads no clock but
// the now it is given, which only the index reads, and depends on no map
// order.
//
// The point is the only cut: each reader takes pt.Seq, and Seq 0 counts no
// call. None needs a sort here, since each is independent of the order of
// the records (VER44). What the turn is, its ProjectRoot and its Spec, is set
// first. Then recording decides whether anything may speak: an incomplete
// record says nothing, no gate and no record green, and its notes say why
// (2.2.5, Silence; VER 11). Otherwise the gates are classify then gateStates
// at the point. A gate is required when the index requires its family at now;
// with no index, or one that requires no family yet, every gate is (D38,
// indexLearned). Integrity is the window's integrity view (integrity.go),
// from 0, the run: v0 deviation (D40), window = the run until 1.8w; a skip
// added and never removed holds RecordGreen false for the session. The record
// is green when at least one gate is required, every required gate is Green
// (D36) and no integrity check is unresolved (PF-IV3, D39), as of the latest
// LastGreen among the gates. Every list is non-nil, so a consumer never reads
// null as unknown.
//
// Evaluate at a point equals Evaluate over the record's prefix by declaration
// at that point (INV-11, D37; RES14b), with one stated exception. A gate's
// kind, and so the kind token of its name, is read over the whole family
// record (kindOf, D33 item 3). So a runner whose recorded kind lands only
// after the point names its gate by that kind at the point, and by its
// inferred kind in the prefix (RES14c). A nil run is an empty one.
func Evaluate(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, pt Point, now time.Time) Snapshot {
	if run == nil {
		run = &store.Run{}
	}
	at := pt.Seq
	snap := Snapshot{
		VerifyVersion:   Version,
		RulesVersion:    RulesVersion,
		PracticeVersion: PracticeVersion,
		Point:           pt,
		ProjectRoot:     projectRoot(run.Declarations, at),
		Spec:            specInfo(sp),
		Gates:           []GateState{},
		Claims:          []ClaimVerdict{},
		SpecVerdicts:    []RuleVerdict{},
		Integrity:       Integrity{Unresolved: []IntegrityEntry{}},
	}
	snap.Recording, snap.Notes = recording(run, at)
	if snap.Notes == nil {
		snap.Notes = []string{}
	}
	if snap.Recording != Complete {
		return snap
	}
	picked := pickExecutions(run)
	runs, writes, unnamed, _ := classifyWith(run, picked)
	snap.Gates = gateStates(runs, writes, unnamed, at)
	nowMS := now.UnixMilli()
	learned := indexLearned(idx, nowMS)
	for i := range snap.Gates {
		snap.Gates[i].Required = !learned || idx.Required(snap.Gates[i].Family, nowMS)
	}
	snap.Integrity = integrity(run, picked, writes, 0, at)
	snap.RecordGreen, snap.RecordGreenAtSeq = recordGreen(snap.Gates, len(snap.Integrity.Unresolved))
	return snap
}

// indexLearned reports whether idx requires any family at nowMS (D38). An
// index that requires none has learned nothing yet, and Evaluate reads it as
// no index, every gate required: the fail-closed prior holds until a family
// is established. The test is over the index, not the snapshot's gates: a
// learned project whose established gates did not run this turn keeps its
// gates advisory. A project whose families have all aged out returns to the
// prior. Cost: one Required per entry, O(E log E), E capped by MaxIndexBytes.
func indexLearned(idx *ProjectIndex, nowMS int64) bool {
	if idx == nil {
		return false
	}
	for _, e := range idx.Entries {
		if idx.Required(e.Family, nowMS) {
			return true
		}
	}
	return false
}

// specInfo is the spec's identity as a snapshot carries it, nil with no spec.
// The golden names one path: the repository spec's, which is the last source
// in precedence order (managed first), or the only one.
func specInfo(sp *spec.Compiled) *SpecInfo {
	if sp == nil {
		return nil
	}
	info := &SpecInfo{Digest: sp.Digest, Trust: sp.Trust, Errors: []string{}}
	if n := len(sp.Sources); n > 0 {
		info.Path = sp.Sources[n-1].Path
	}
	return info
}

// projectRoot is where the turn ran: the Project of the declaration with the
// largest Seq at or before at that names one, else "". One pass, and no slice
// order, since Evaluate does not sort.
func projectRoot(decls []store.Declaration, at int64) string {
	var root *string
	var seq int64
	for _, d := range decls {
		if d.Seq <= at && d.Project != nil && (root == nil || d.Seq > seq) {
			root, seq = d.Project, d.Seq
		}
	}
	if root == nil {
		return ""
	}
	return *root
}

// recordGreen is whether the record backs a "done" at the point (D36): no
// integrity check unresolved (unresolved is their count; archive 20.2.4's
// record-green point needs the integrity component at 1, PF-IV3), at least
// one required gate, and every required gate Green; and then the latest
// LastGreen among them, else nil. A gate that is not required is reported
// with its state and never blocks. Green with no LastGreen is a gate
// gateStates never builds, and it fails closed, as TurnSummary reads it.
func recordGreen(gates []GateState, unresolved int) (bool, *int64) {
	if unresolved > 0 {
		return false, nil
	}
	var at int64
	n := 0
	for _, g := range gates {
		if !g.Required {
			continue
		}
		if g.State != Green || g.LastGreen == nil {
			return false, nil
		}
		if n == 0 || g.LastGreen.Seq > at {
			at = g.LastGreen.Seq
		}
		n++
	}
	if n == 0 {
		return false, nil
	}
	return true, &at
}

// TurnCounts is what the Stop hook counted for the turn (2.2.11).
// UnnamedWriteSeq is the turn's last call that may have written files the
// record cannot name (a Bash write with no known target, or an MCP call);
// zero means none. RewrittenGateSeq is the turn's last gate run whose line a
// hook rewrote after its declaration, so that where it ran is not known and
// it credits no gate (classify's rewritten); zero means none. Nothing reads
// it yet: it counts how often a rewrite costs a gate its run.
type TurnCounts struct {
	Writes           int
	PathsWritten     int
	GateRuns         int
	LastWriteSeq     int64
	UnnamedWriteSeq  int64
	RewrittenGateSeq int64
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

// sentenceAdvisoryOnly marks a row read from advisory gates alone (D38);
// P-45 renders it.
const sentenceAdvisoryOnly = "advisory only"

// findingsSentence is the findings row's count clause (2.2.11's row table:
// "<n> files edited; <gates> passed after the last edit; <k> record
// finding(s) in this turn"), the one part of the sentence only the snapshot
// decides; P-45 renders the rest before it, as it renders every row's. k is a
// count and never names a check or a marker (1.8 Gate B F1): the names are
// 2.2's PostToolUse facts.
func findingsSentence(k int) string {
	return strconv.Itoa(k) + " record finding(s) in this turn"
}

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
//
// With no required gate, the learned gates are advisory, and the row reads
// them with one cap (D38): a non-green keeps its row, so an advisory red is
// never hidden, but a green is unverified, or ran_only with no write, never
// verified, since no established gate ran; and the row says so in its
// sentence. A gate that is neither required nor learned is not a turn gate.
//
// The findings row is every turn gate green without the record-green point:
// today an unresolved integrity check, so its sentence counts them,
// k = len(Integrity.Unresolved) (findingsSentence); executed spec violations
// join k when they are evaluated. While the window is the run (D40), k counts
// the session's unresolved checks, not only this turn's, until 1.8w.
func TurnSummary(snap Snapshot, turn TurnCounts) Summary {
	if turn.Writes == 0 && turn.GateRuns == 0 {
		return Summary{Row: RowNone}
	}
	if snap.Recording != Complete {
		return Summary{Row: RowUnknown, Sentence: sentenceRecordingIncomplete}
	}
	advisory := !slices.ContainsFunc(snap.Gates, func(g GateState) bool { return g.Required })
	var states []State
	defect := false
	// The unnamed-write rule reads "after a turn gate's last green run": with
	// several gates, the earliest last green is the one a later unnamed write
	// can invalidate, and a gate with no green run counts as seq 0.
	earliestGreen := int64(-1)
	for _, g := range snap.Gates {
		if !g.Required && !(advisory && g.Source == sourceLearned) {
			continue
		}
		if Rank(g.State) < 0 {
			defect = true
			continue
		}
		// Green with no LastGreen is a snapshot Evaluate never builds: a
		// green run that never happened. Reading it as a pass is the
		// defect, not the fix (VER 11).
		if g.State == Green && g.LastGreen == nil {
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
	var row Row
	switch {
	case !ok && !defect && turn.Writes == 0:
		return Summary{Row: RowNone}
	case !ok && !defect:
		return Summary{Row: RowNoGate}
	case ok && worst != Green:
		row = rowFor(worst)
	case defect:
		row = RowUnknown
	case turn.Writes == 0:
		row = RowRanOnly
	case advisory:
		row = RowUnverified // the cap: an advisory green never verifies
	case turn.UnnamedWriteSeq > earliestGreen:
		row = RowUnnamedWrite
	case snap.RecordGreen:
		row = RowVerified
	default:
		row = RowFindings
	}
	s := Summary{Row: row}
	switch {
	case advisory:
		s.Sentence = sentenceAdvisoryOnly
	case row == RowFindings:
		s.Sentence = findingsSentence(len(snap.Integrity.Unresolved))
	}
	return s
}
