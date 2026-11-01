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
//
// Its keys, here and in IndexEntry, are v1.2.0-alpha.1's and no other:
// alpha.1 reads the index with DisallowUnknownFields and its Stop writes a
// fresh one over a file it cannot read, so a key added here costs a person
// who goes back to alpha.1 the project's learned gates. A new key goes in a
// file of its own, as the last two checkpoints did (Checkpoints,
// INDEX-POINTER-FILE1).
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
// added and never removed holds RecordGreen false for the session. The spec's
// rules are judged over the same classification (ruleVerdicts). The record
// is green when at least one gate is required, every required gate is Green
// (D36), no integrity check is unresolved (PF-IV3, D39) and no trusted or
// managed rule is violated (PF-PG8), as of the latest LastGreen among the
// gates. Every list is non-nil, so a consumer never reads null as unknown.
//
// Evaluate at a point equals Evaluate over the record's prefix by declaration
// at that point (INV-11, D37; RES14b), with one stated exception. A gate's
// kind, and so the kind token of its name, is read over the whole family
// record (kindOf, D33 item 3). So a runner whose recorded kind lands only
// after the point names its gate by that kind at the point, and by its
// inferred kind in the prefix (RES14c). A nil run is an empty one.
//
// Evaluate holds no install key, so no line a spec declares digests as the
// hook digested the runs, and it reads no declared gate: its rows are the
// learned ones alone. EvaluateStop and SteerAt take the key and join them
// (card DECLARED-GATES1).
func Evaluate(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, pt Point, now time.Time) Snapshot {
	return evaluate(run, idx, sp, nil, pt, now, &classified{})
}

// classified is what evaluate classified beside the snapshot, for the Stop
// view, its ledger and the steer view (EvaluateStop, SteerAt): classify's
// four returns, in seq order; nil where the record is incomplete, which
// classifies nothing.
type classified struct {
	runs      []gateRun
	writes    []write
	unnamed   []int64
	rewritten []int64
	// learned is indexLearned's answer at the point, which stale-evidence
	// reads: nothing raises it before a family is established.
	learned bool
}

// evaluate is Evaluate, which also fills c, and joins the gates sp declares
// under key, the install key (declaredGates), to the learned rows after the
// index has set theirs required or not: a declared row's Required is its
// spec's (G2).
func evaluate(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, key []byte, pt Point, now time.Time, c *classified) Snapshot {
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
	runs, writes, unnamed, rewritten := classifyWith(run, picked)
	c.runs, c.writes, c.unnamed, c.rewritten = runs, writes, unnamed, rewritten
	snap.Gates = gateStates(runs, writes, unnamed, at, pt.Tree)
	nowMS := now.UnixMilli()
	learned := indexLearned(idx, nowMS)
	c.learned = learned
	for i := range snap.Gates {
		snap.Gates[i].Required = !learned || idx.Required(snap.Gates[i].Family, nowMS)
	}
	if dgs := declaredGates(sp, key, snap.ProjectRoot); len(dgs) > 0 {
		snap.Gates = withDeclared(snap.Gates, dgs, lineRuns(run, picked), runs, writes, unnamed, at, pt.Tree)
	}
	snap.Integrity = integrity(run, picked, writes, 0, at, pt.StoreRoot)
	snap.SpecVerdicts = ruleVerdicts(sp, run, picked, c, &snap, sp != nil && wroteIn(writes, at))
	snap.RecordGreen, snap.RecordGreenAtSeq = recordGreenOf(&snap)
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
// in precedence order (managed first), or the only one. Errors is the
// Compiled's, a copy, never null.
func specInfo(sp *spec.Compiled) *SpecInfo {
	if sp == nil {
		return nil
	}
	info := &SpecInfo{Digest: sp.Digest, Trust: sp.Trust, Errors: append([]string{}, sp.Errors...)}
	if n := len(sp.Sources); n > 0 {
		info.Path = sp.Sources[n-1].Path
	}
	return info
}

// ProjectRoot is the Snapshot's ProjectRoot for run at seq at, read before
// any evaluation: what a replay discovers the spec from (card LEDGER-WIRE1).
func ProjectRoot(run *store.Run, at int64) string {
	if run == nil {
		return ""
	}
	return projectRoot(run.Declarations, at)
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

// recordFindings is the snapshot's record findings: its unresolved integrity
// checks and its binding rules' violations, the k of the findings row and
// the count recordGreen takes.
func recordFindings(snap *Snapshot) int {
	return len(snap.Integrity.Unresolved) + bindingViolations(snap.SpecVerdicts)
}

// recordGreenOf is recordGreen over the snapshot's gates and its record
// findings: the one reading evaluate and pinRequired (the replay's pinned
// Required) both take, so the live Stop and the ledger count the same thing
// (#173 F4, card SPEC-RULES-STOP1).
func recordGreenOf(snap *Snapshot) (bool, *int64) {
	return recordGreen(snap.Gates, recordFindings(snap))
}

// recordGreen is whether the record backs a "done" at the point (D36): no
// integrity check unresolved and no binding rule violated (unresolved is
// their count; archive 20.2.4's record-green point needs the integrity
// component at 1, PF-IV3, and no spec violation, PF-PG8), at least
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
// it yet: it counts how often a rewrite costs a gate its run. FirstSeq is the
// lowest seq in the turn's window, zero for none: a run before it is an earlier
// turn's. Sound one way only: a window need not be contiguous (a subagent's late
// call), so another prompt's run inside its span reads as this turn's (2.4r).
type TurnCounts struct {
	Writes           int
	PathsWritten     int
	GateRuns         int
	LastWriteSeq     int64
	UnnamedWriteSeq  int64
	RewrittenGateSeq int64
	FirstSeq         int64
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
	// Findings is k, the record findings at the point (recordFindings). The
	// findings row's Sentence counts them, and every other row that prints
	// ends with the same clause when k > 0 (FindingsClause; card
	// SPEC-RULES-STOP1, D-f), so a recorded violation the turn's gates did
	// not reach still reads on the line.
	Findings int
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

// Advisory reports whether no gate in gates is required, so the learned
// gates are read as advisory (D38).
func Advisory(gates []GateState) bool {
	return !slices.ContainsFunc(gates, func(g GateState) bool { return g.Required })
}

// TurnGate reports whether g is a turn gate as TurnSummary reads one: a
// required gate, or, when the gates are advisory, a learned one. The value
// line names its gates by the same rule.
func TurnGate(g GateState, advisory bool) bool {
	return g.Required || advisory && g.Source == sourceLearned
}

// Advisory reports whether the row was read from advisory gates alone, which
// its rendered sentence says (D38).
func (s Summary) Advisory() bool { return s.Sentence == sentenceAdvisoryOnly }

// FindingsClause is the count clause a row other than findings ends with when
// the record holds k > 0 findings (D-f): a trusted protect_path write in a
// project with no gate, or a forbidden call beside a red gate, is counted on
// the line its row prints. The findings row's Sentence is that clause
// already. TurnSummary returns the row with no line before it counts k, and
// verified never carries one: record-green needs k = 0.
func (s Summary) FindingsClause() string {
	if s.Findings == 0 || s.Row == RowFindings {
		return ""
	}
	return findingsSentence(s.Findings)
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
// an unresolved integrity check or a binding rule's violation, so its
// sentence counts them, k = len(Integrity.Unresolved) plus the violations of
// trusted and managed rules (findingsSentence, bindingViolations). While the
// window is the run (D40), k counts the session's, not only this turn's,
// until 1.8w. Every row that prints carries k as Findings, and a row other
// than findings ends with its clause when k > 0 (FindingsClause, D-f).
func TurnSummary(snap Snapshot, turn TurnCounts) Summary {
	if turn.Writes == 0 && turn.GateRuns == 0 {
		return Summary{Row: RowNone}
	}
	k := recordFindings(&snap)
	if snap.Recording != Complete {
		return Summary{Row: RowUnknown, Sentence: sentenceRecordingIncomplete, Findings: k}
	}
	advisory := Advisory(snap.Gates)
	var states []State
	defect := false
	// The unnamed-write rule reads "after a turn gate's last green run": with
	// several gates, the earliest last green is the one a later unnamed write
	// can invalidate, and a gate with no green run counts as seq 0.
	earliestGreen := int64(-1)
	for _, g := range snap.Gates {
		if !TurnGate(g, advisory) {
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
		return Summary{Row: RowNoGate, Findings: k}
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
	s := Summary{Row: row, Findings: k}
	switch {
	case advisory:
		s.Sentence = sentenceAdvisoryOnly
	case row == RowFindings:
		s.Sentence = findingsSentence(k)
	}
	return s
}
