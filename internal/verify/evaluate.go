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
	return evaluate(run, idx, sp, nil, nil, "", pt, now, &classified{})
}

// classified is what evaluate classified beside the snapshot, for the Stop
// view, its ledger and the steer view (EvaluateStop, SteerAt): classify's
// four returns and its held writes, in seq order; nil where the record is
// incomplete, which classifies nothing.
type classified struct {
	runs      []gateRun
	writes    []write
	unnamed   []int64
	held      []heldWrite
	rewritten []int64
	// learned is indexLearned's answer at the point, which stale-evidence
	// reads: nothing raises it before a family is established.
	learned bool
	// inferred is the inferred gate's runner name from the derivation that
	// keyed its row, which the Stop names the row by; "" for none.
	inferred string
	// gates are the declared and inferred gates evaluate joined, and
	// inferredDeclared whether the inferred one is the project's own word
	// (InferredGate.Declared): the platform rows' gate (testRunners), kept
	// here so the Stop derives no gate line twice (INFER-BUDGET1).
	gates            []declaredGate
	inferredDeclared bool
}

// evaluate is Evaluate, which also fills c, and joins the gates sp declares
// under key, the install key (declaredGates), and the inferred gate inf
// (inferredGate; nil for none, as every caller but the Stop's passes), to
// the learned rows after the index has set theirs required or not: a
// declared row's Required is its spec's (G2), and the inferred row's true.
//
// root is the project the caller read sp and inf from, the Stop's and the
// post's cwd key (card PROJECT-RESOLVE1): the snapshot's ProjectRoot, and
// so where every declared and inferred gate is placed. A declaration's
// Project is the repository that one call ran in, so the latest one's is
// not the turn's: a last `git -C b status` would move a's gates to b. ""
// takes the latest declaration's (projectRoot, VER47), which only a caller
// with no cwd, or Evaluate's learned rows alone, takes. A record that names
// no project at all (v4 withholds it) keeps "": the snapshot, its steer
// line and its checkpoint follow the record (A3, TestSTR43_).
func evaluate(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, key []byte, inf *InferredGate, root string, pt Point, now time.Time, c *classified) Snapshot {
	if run == nil {
		run = &store.Run{}
	}
	at := pt.Seq
	if latest := projectRoot(run.Declarations, at); root == "" || latest == "" {
		root = latest
	}
	snap := Snapshot{
		VerifyVersion:   Version,
		RulesVersion:    RulesVersion,
		PracticeVersion: PracticeVersion,
		Point:           pt,
		ProjectRoot:     root,
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
	keys := classifyWith(run, picked, c)
	runs, writes, unnamed := c.runs, c.writes, c.unnamed
	snap.Gates = gateStates(runs, writes, unnamed, keys, at, pt.Tree)
	nowMS := now.UnixMilli()
	learned := indexLearned(idx, nowMS)
	c.learned = learned
	for i := range snap.Gates {
		snap.Gates[i].Required = !learned || idx.Required(snap.Gates[i].Family, nowMS)
	}
	dgs := declaredGates(sp, key, snap.ProjectRoot)
	if g, name, ok := inferredGate(inf, key, snap.ProjectRoot); ok {
		dgs = append(dgs, g)
		c.inferred = name
		c.inferredDeclared = inf.Declared
	}
	c.gates = dgs
	if len(dgs) > 0 {
		snap.Gates = withDeclared(snap.Gates, dgs, lineRuns(run, picked), runs, writes, unnamed, snap.ProjectRoot, at, pt.Tree)
	}
	snap.Integrity = integrity(run, picked, writes, c.held, 0, at, pt.StoreRoot)
	snap.SpecVerdicts = ruleVerdicts(sp, run, picked, c, &snap, sp != nil && wroteIn(writes, at))
	markHeld(snap.Gates, c.held, at)
	snap.RecordGreen, snap.RecordGreenAtSeq = recordGreenOf(&snap)
	return snap
}

// markHeld sets each green gate's heldSince: the last held write at or
// before at, declared after its last green run, that may have written a
// file the gate covers (relevant and named under its root, as stateOf
// counts an edit). Such a call stales no gate, since the record does not say
// it wrote, and it does not leave the record backing a "done" either
// (recordGreenOf). One that names no path is passed over, as an unnamed edit
// is (D30): the turn's row is where that fails closed.
//
// One declared before the last green is passed over too, even with no
// recorded ending, though the turn's row names such a call wherever it
// stands (TurnCounts.unconfirmed): the row speaks for one turn, while the
// point is the session's, and a call refused or whose post was lost long
// before would hold it not green for the rest of the session. Whether a call
// with no ending wrote after a green declared later is the question of calls
// still in flight (a subagent's concurrent Edit), which this rule leaves to
// its own. The cost: such a call, where it did write after the green, leaves
// the point green, as it was before this rule, when it was read as an edit
// before the green; the turn's row and the report's line name it, and the
// next session's start line does not.
func markHeld(gates []GateState, held []heldWrite, at int64) {
	for i := range gates {
		g := &gates[i]
		if g.LastGreen == nil || g.State != Green {
			continue
		}
		for j := range held {
			w := &held[j]
			if w.End.mayHaveWritten() && w.Path != nil && Relevant(w.Class) && w.Seq > g.LastGreen.Seq && w.Seq <= at && covers(g.root, &w.write) {
				g.heldSince = max(g.heldSince, w.Seq)
			}
		}
	}
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
// (#173 F4, card SPEC-RULES-STOP1). A required gate green while a call that
// may have written its files stands after its last green (markHeld) does not
// back a "done": the checkpoint, and the next session's start line from it,
// read not green, as they did when such a call staled the gate. The turn's
// row reads recordGreen and not this (TurnSummary): it names the turn's own
// such call, and an earlier turn's is no finding of this one. The cost: the
// start line says "not green" with no reason it can name, since every gate
// it counts is green.
func recordGreenOf(snap *Snapshot) (bool, *int64) {
	ok, at := recordGreen(snap.Gates, recordFindings(snap))
	if ok && slices.ContainsFunc(snap.Gates, func(g GateState) bool { return g.Required && !g.offTurn && g.heldSince > 0 }) {
		return false, nil
	}
	return ok, at
}

// recordGreen is whether the record backs a "done" at the point (D36): no
// integrity check unresolved and no binding rule violated (unresolved is
// their count; archive 20.2.4's record-green point needs the integrity
// component at 1, PF-IV3, and no spec violation, PF-PG8), at least
// one required gate, and every required gate Green; and then the latest
// LastGreen among them, else nil. A gate that is not required is reported
// with its state and never blocks; nor does one not the Stop's turn's
// (offTurn, card F2), which only TurnSummary's read meets. Green with no
// LastGreen is a gate gateStates never builds, and it fails closed, as
// TurnSummary reads it.
func recordGreen(gates []GateState, unresolved int) (bool, *int64) {
	if unresolved > 0 {
		return false, nil
	}
	var at int64
	n := 0
	for _, g := range gates {
		if !g.Required || g.offTurn {
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
//
// Writes, PathsWritten, LastWriteSeq and UnnamedWriteSeq count edits: calls
// whose execution ended ok (classify). Of the turn's held writes that bear
// where an edit would (relevant and covered by a gate, or naming nothing:
// countIn), FailedEditSeq is the last file tool that failed (EndFailedEdit,
// which applied nothing); FailedWriteSeq the last call that failed or was
// interrupted and may have written (EndFailedCall, EndInterrupted), and
// FailedWriteEnd its ending; UnendedWriteSeq the last with no recorded
// ending (EndNone). Zero means none. None of them is an edit: the row's
// guard reads them (unconfirmed).
type TurnCounts struct {
	Writes           int
	PathsWritten     int
	GateRuns         int
	LastWriteSeq     int64
	UnnamedWriteSeq  int64
	RewrittenGateSeq int64
	FirstSeq         int64
	FailedEditSeq    int64
	FailedWriteSeq   int64
	UnendedWriteSeq  int64
	FailedWriteEnd   WriteEnd
}

// mayHaveWritten reports a held call in the turn that may have written: one
// that failed other than as a file tool, was interrupted, or has no ending.
func (t TurnCounts) mayHaveWritten() bool { return t.FailedWriteSeq > 0 || t.UnendedWriteSeq > 0 }

// unconfirmed is the call that bears on a row resting on green, the
// earliest last green of the turn's gates, and its ending; seq 0 for none.
// A call with no recorded ending bears wherever it stands in the window,
// since it may have written after the green whenever it was declared. One
// that failed or was interrupted and may have written bears where it was
// declared after that green, since a green after it ran over whatever it
// wrote; under ranOnly, the row that would say "no edits", it bears
// wherever it stands, since that row speaks for the whole turn. A failed
// edit bears where it was declared after the green: one before it, the
// common fail-then-retry turn, leaves the run after it as it is. A call that
// may have written is named before a failed edit, which applied nothing, so
// the line never names the harmless call and leaves the other out; of two
// that may have written, the later.
//
// The cost: where a failed edit and a call that may have written both bear,
// the line names only the second.
func (t TurnCounts) unconfirmed(green int64, ranOnly bool) (int64, WriteEnd) {
	seq, end := int64(0), EndOK
	if t.FailedWriteSeq > green || ranOnly && t.FailedWriteSeq > 0 {
		seq, end = t.FailedWriteSeq, t.FailedWriteEnd
	}
	if t.UnendedWriteSeq > seq {
		seq, end = t.UnendedWriteSeq, EndNone
	}
	if seq == 0 && t.FailedEditSeq > green {
		seq, end = t.FailedEditSeq, EndFailedEdit
	}
	return seq, end
}

// WroteAfter is the call that may have written and bears on a run the line
// places after the last edit, read as unconfirmed reads one against a green:
// one that failed or was interrupted declared after run, or one with no
// recorded ending anywhere in the turn; seq 0 for none. A failed edit
// applied nothing, so the run still came after every change the turn made,
// and it is left out. The red, unknown, masked and backgrounded rows, and
// the unverified row's greens, read it (recap): "after the last edit" there
// is placed by the edits alone, which a call that may have written after the
// run does not change, so the line names it beside the run.
func (t TurnCounts) WroteAfter(run int64) (int64, WriteEnd) {
	if seq, end := t.unconfirmed(run, false); end != EndFailedEdit {
		return seq, end
	}
	return 0, EndOK
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
	// RowUnconfirmedWrite is a row that would read verified or ran_only
	// while a call that may write and did not end ok bears on its green
	// (TurnCounts.unconfirmed), or a turn with no edit and no gate run whose
	// call may have written, which would print nothing: the line says what
	// was measured and names that call and its ending.
	RowUnconfirmedWrite Row = "unconfirmed_write"
	RowVerified         Row = "verified"
	RowFindings         Row = "findings"
)

// Rows is the table's order.
var Rows = []Row{
	RowUnknown, RowRed, RowMasked, RowBackgrounded, RowRanOnly,
	RowUnverified, RowNoGate, RowUnnamedWrite, RowUnconfirmedWrite, RowVerified, RowFindings,
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
	// Unconfirmed is the call the unconfirmed_write row names and Ending how
	// the record holds its end (TurnCounts.unconfirmed); zero on every
	// other row.
	Unconfirmed int64
	Ending      WriteEnd
	// GreenSeq is the green run the unconfirmed_write and unnamed_write
	// rows placed their call against: the earliest last green of the turn's
	// gates. The line places the call by it, not by the latest green it
	// names, which the call may stand before (two gates). Zero on the
	// unconfirmed_write row of a turn that ran no gate, and on every other
	// row.
	GreenSeq int64
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
// required gate, or, when the gates are advisory, a learned one; and at
// Stop, one a call of the turn ran in the project of (offTurn, card F2), so
// a gate masked in another worktree hours before names no row here. The
// value line names its gates by the same rule.
func TurnGate(g GateState, advisory bool) bool {
	return !g.offTurn && (g.Required || advisory && g.Source == sourceLearned)
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
	// An incomplete turn that may have written reads unknown, never none
	// (D80), whether or not the call that may have written ended ok: the
	// record that would say is the one with the gap.
	held := turn.FailedEditSeq > 0 || turn.mayHaveWritten()
	quiet := turn.Writes == 0 && turn.GateRuns == 0
	if quiet && (snap.Recording == Complete && !turn.mayHaveWritten() || snap.Recording != Complete && !held) {
		return Summary{Row: RowNone}
	}
	k := recordFindings(&snap)
	if snap.Recording != Complete {
		return Summary{Row: RowUnknown, Sentence: sentenceRecordingIncomplete, Findings: k}
	}
	// A complete turn with no edit and no gate run whose call may have
	// written names that call: no line would read as a turn that changed
	// nothing, which the record does not say (a failed sed -i may have
	// rewritten its first file). A failed file tool alone applied nothing,
	// and that turn has no line (above).
	if quiet {
		seq, end := turn.unconfirmed(0, true)
		return Summary{Row: RowUnconfirmedWrite, Findings: k, Unconfirmed: seq, Ending: end}
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
	// The record-green point the row reads is over the turn's gates (card
	// F2): the snapshot's, over every gate, until one is not the turn's. Nor
	// does it wait on a held call after a green (recordGreenOf): the guard
	// below names the turn's own, and an earlier turn's is no finding of
	// this one, which the findings row would count as "0 record finding(s)".
	green := snap.RecordGreen
	if slices.ContainsFunc(snap.Gates, func(g GateState) bool { return g.offTurn || g.heldSince > 0 }) {
		green, _ = recordGreen(snap.Gates, k)
	}
	// The guard on the green rows: a call that did not end ok and bears on
	// the green names itself where the row would read verified or "no
	// edits", which neither the record nor the call's ending measured.
	unconfirmed, ending := turn.unconfirmed(max(earliestGreen, 0), turn.Writes == 0)
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
	case turn.Writes == 0 && unconfirmed > 0:
		row = RowUnconfirmedWrite
	case turn.Writes == 0:
		row = RowRanOnly
	case advisory:
		row = RowUnverified // the cap: an advisory green never verifies
	case turn.UnnamedWriteSeq > earliestGreen:
		row = RowUnnamedWrite
	case unconfirmed > 0:
		row = RowUnconfirmedWrite
	case green:
		row = RowVerified
	default:
		row = RowFindings
	}
	s := Summary{Row: row, Findings: k}
	if row == RowUnconfirmedWrite {
		s.Unconfirmed, s.Ending = unconfirmed, ending
	}
	if row == RowUnconfirmedWrite || row == RowUnnamedWrite {
		s.GreenSeq = earliestGreen
	}
	switch {
	case advisory:
		s.Sentence = sentenceAdvisoryOnly
	case row == RowFindings:
		s.Sentence = findingsSentence(k)
	}
	return s
}
