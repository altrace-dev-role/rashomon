package verify

import (
	"cmp"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The Stop view (card 2.4) is a view over evaluate's one classification,
// beside the steer view: at the turn's end it gives recap the value line's
// row and counts, the claim scan the done flag reads, the gate programs the
// sentences name, the project index observed for this session, and the
// obligation ledger (card LEDGER-WIRE1). It is pure, as Evaluate is: the
// caller reads the index and the steer log before and writes the index and
// the checkpoint after.

// StopInput is what the Stop hook read beside the record: the prompt the
// digest chose, the project key of the payload's cwd ("" when it named none),
// the final message's head and tail windows, the install key the quotes are
// HMACed under and the declared gates' lines digested under, and the model
// texts of the notes delivered in the prompt. A replay of a Stop from the
// record sets the last two: MessageUnread says no final message is held, so
// what is read from it is unknown, never absent (INV-2); Required pins each
// gate it names, by gate name, to the Required the Stop's checkpoint
// recorded, the Stop-time truth the index on disk may since have moved past
// (nil pins nothing).
type StopInput struct {
	PromptID      string
	ProjectKey    string
	Head, Tail    []byte
	Key           []byte
	Delivered     [][]byte
	MessageUnread bool
	Required      map[string]bool
	// Inferred is the project's test command as gateinfer read it from the
	// project's own files, judged as one more gate (card GATE-INFER1); nil
	// for none. The Stop's caller sets it only while no spec declares a test
	// gate.
	Inferred *InferredGate
	// Root is the project the Stop read its spec and inferred gate from,
	// baseline.Key of the payload's cwd (card PROJECT-RESOLVE1), under
	// which evaluate places every gate; "" for none (evaluate).
	Root string
	// Platform reads the pgraph log under root, the snapshot's project root,
	// for written, the session's relevant .go writes, project-relative (card
	// PL-WIRE1): the ledger verb's replay and the Stop hook (PW1b-ON-230
	// step 2) set it to internal/platform's Read, which imports this
	// package; StopFailure and the catch-up leave it nil. EvaluateStop
	// calls it once, after classifying, only where the session wrote a Go
	// file; nil reads no log and owes no platform row.
	Platform func(root string, written []string) PlatformRead
}

// InferredGate is a test command read from the project's own files: its
// words and the directory, relative to the project root, it runs in. The
// Stop judges it as the gate InferredGateID, required, since it stands for
// the project's own declaration (card GATE-INFER1, W2'). Learned is whether
// the index has seen a test run in the project: the row's trust is then the
// learned gates' word, builtin, and else SourceInferred, which the Stop's
// line labels. Its words never reach the record: the row's id is closed.
// From, the file it was read from, and Declared, whether the project names
// the command there rather than by convention, are the next prompt's note's
// (W5); EvaluateStop reads neither.
type InferredGate struct {
	Argv     []string
	Dir      string
	Learned  bool
	From     string
	Declared bool
}

// StopView is EvaluateStop's result. Summary is TurnSummary over Snap and
// Turn; Programs maps each gate of Snap to its last run's program word or runner's name ("" when
// the record names none); Index is the index EvaluateStop was given, or a new
// one under the project key, observed in place when the key is set; Scan is
// the final message's claims against Snap's gates. Snap.Claims is Scan.Claims
// when the recording is complete, so the checkpoint keeps the verdicts the
// next prompt reads (D90); an incomplete record speaks no verdict (2.2.5,
// Silence).
type StopView struct {
	Snap    Snapshot
	Summary Summary
	Turn    TurnCounts
	// Done is the window the done flag reads where it is more than the turn
	// (doneWindow, BG-OUTCOME phase 2a): the turns since the last edit in the
	// gates' projects; nil where the turn holds that edit, or the session
	// made none.
	Done     *DoneWindow
	Programs map[string]string
	Index    *ProjectIndex
	Scan     ClaimScan
	// Expected is the task's expected evidence rows at Stop (card 3.3), in
	// spec.EvidenceKinds order, never nil.
	Expected []Expected
	// Obligations is the ledger at the Stop point (PF-PG5): the gates' rows
	// over the classification Snap came from, then Expected's rows, never
	// nil.
	Obligations []Obligation
	// Unbound is each repository spec, project-relative, the session edited
	// after the user trusted it (SPEC-TRUST-DROP1): its binding names
	// another digest, so what it declares flags to the user only until
	// `rashomon spec trust`. EvaluateStop reads no bindings, so the Stop's
	// caller fills it; nil for none.
	Unbound []string
	// Untrusted is each gate a spec declares from a source that reads
	// untrusted, in the spec's order (STOP-UNBOUND-FIRST): the brief names
	// it "(spec, untrusted)" and no run binds it (declaredGates), so the
	// Stop says it does not count rather than that no test or build is
	// known. The Stop's caller fills it beside Unbound; nil for none, and
	// nil after a session write under the store root, when no trust holds
	// for the rest of the session (hook.TrustRoot, D173).
	Untrusted []UntrustedGate
	// Platform is what StopInput.Platform read, the families Obligations'
	// platform_impact rows were built from, in their order; its Families nil
	// where nothing was read.
	Platform PlatformRead
	// Violations is each trusted or managed rule's violation in this turn,
	// its Seqs cut to the turn's (turnViolations), in spec order: the Stop
	// names them, where Summary's k counts the session's and names none
	// (D40; 1.8 Gate B F1; card RECALL-15). Nil for none.
	Violations []RuleVerdict
}

// UntrustedGate is a gate a spec declares while the spec reads untrusted:
// its id, and the spec's path, project-relative as Unbound names it.
type UntrustedGate struct{ ID, Spec string }

// EvaluateStop is the Stop hook's view at pt. One classification feeds the
// snapshot, the programs, the index and the ledger, whose expected rows are
// the at-Stop ones the done flag reads. An incomplete record classifies
// nothing in evaluate, a contract the obligation view relies on, so here the
// run is classified once more for that case only, and that classification
// feeds the turn's counts and nothing else: an incomplete turn that wrote
// reads unknown, never none (D80).
func EvaluateStop(run *store.Run, idx *ProjectIndex, sp *spec.Compiled, pt Point, now time.Time, in StopInput) StopView {
	if run == nil {
		run = &store.Run{}
	}
	var c classified
	snap := evaluate(run, idx, sp, in.Key, in.Inferred, in.Root, pt, now, &c)
	pinRequired(&snap, in.Required)
	counted := c
	if snap.Recording != Complete {
		classifyWith(run, pickExecutions(run), &counted)
	}
	if snap.Recording == Complete {
		markTurn(snap.Gates, turnProjects(run, in.PromptID, pt.Seq))
	}
	roots := gateRoots(&snap)
	turn := countTurn(run, counted, in.PromptID, pt.Seq, roots)
	v := StopView{Snap: snap, Summary: TurnSummary(snap, turn), Turn: turn, Programs: programs(snap.Gates, c.runs), Index: idx,
		Violations: turnViolations(snap.SpecVerdicts, turn.FirstSeq)}
	if w, ok := doneWindow(run, counted, turn, pt.Seq, onTurnRoots(&snap, roots)); ok {
		v.Done = &DoneWindow{Turn: w, Summary: TurnSummary(snap, w)}
	}
	inferredProgram(v.Programs, c.inferred)
	if v.Index == nil {
		v.Index = &ProjectIndex{ProjectKey: in.ProjectKey}
	}
	if in.ProjectKey != "" {
		v.Index.Observe(c.runs, pt.SessionID, now.UnixMilli())
	}
	// Over the writes already classified, counted's, which an incomplete
	// record classifies too.
	wrote := wroteIn(counted.writes, pt.Seq)
	v.Scan = Claims(in.Head, in.Tail, in.Key, snap.Gates, wrote, SourceFinalMessage, in.Delivered)
	v.Expected = expected(snap, evidenceIn{sp: sp, atStop: true, unread: in.MessageUnread, disclosed: v.Scan.Disclosed, wrote: wrote})
	v.Obligations = ledgerOf(snap, &c, v.Expected)
	if written := platformPaths(in.Platform, c.writes, pt.Seq); len(written) > 0 {
		v.Platform = in.Platform(snap.ProjectRoot, written)
		v.Obligations = append(v.Obligations, platformObligations(snap, &c, run, in.Key, v.Platform)...)
	}
	if snap.Recording == Complete {
		v.Snap.Claims = v.Scan.Claims
	}
	return v
}

// pinRequired sets each gate pins names to the Required it names, and then
// the record green that follows (D36), read as evaluate reads it, binding
// rule violations included (recordGreenOf); a gate pins does not name keeps
// evaluate's. Nil pins nothing.
func pinRequired(snap *Snapshot, pins map[string]bool) {
	if pins == nil {
		return
	}
	for i := range snap.Gates {
		if r, ok := pins[snap.Gates[i].Gate]; ok {
			snap.Gates[i].Required = r
		}
	}
	snap.RecordGreen, snap.RecordGreenAtSeq = recordGreenOf(snap)
}

// programs maps each gate to the program of the run its LastRun names, found
// by seq among runs (seq order, so a binary search would do; a gate list is
// short and a map over runs is one pass). A run that records the runner it
// launched (call schema 8) is named by it, in the binary's own words --
// `python3 -m pytest`, not the interpreter `python3` the line starts with
// (D229); an older record's is the line's program word. The inferred gate
// with no run, or a wrapped one, is named by inferredProgram.
func programs(gates []GateState, runs []gateRun) map[string]string {
	bySeq := make(map[int64]string, len(runs))
	for _, r := range runs {
		bySeq[r.Seq] = cmp.Or(shape.RunnerNameOf(r.RunnerID), r.Program)
	}
	out := make(map[string]string, len(gates))
	for _, g := range gates {
		// A wrapped last run's line is the wrapper's (make, bash), never the
		// gate's runner, so it names no gate (WRAPPER-RUN1).
		if g.LastRun != nil && g.LastRun.Wrapped == WrapNone {
			out[g.Gate] = bySeq[g.LastRun.Seq]
		}
	}
	return out
}

// inferredProgram names the inferred gate, where programs left it unnamed
// (no run bound it), by its command's runner row, name, which evaluate took
// from the derivation that keyed the row (inferredGate; INFER-BUDGET1): a
// word of the binary's own lists, never of the project's file (GATE-INFER1,
// W4). None where the command names no runner.
func inferredProgram(programs map[string]string, name string) {
	if name != "" && programs[InferredGateID] == "" {
		programs[InferredGateID] = name
	}
}

// countTurn counts the prompt's turn up to seq at (2.2.11 TurnCounts). The
// window is the seq set of the prompt's declarations: gate runs and writes
// carry no prompt id, so membership is by their declaration's seq. A
// declaration whose prompt id is nil or "" is in no window. promptID ""
// selects the prompt digest's selectTurn selects, the one whose first
// declaration is latest (D72); a run with no prompt id has an empty window,
// never the whole run. Writes are the relevant named edits, as gateStates
// counts them, and the held writes bear on the row the same way (countIn):
// those a gate of roots covers (gateRoots; nil counts every
// one), so a turn that wrote only outside its project edited nothing a gate
// checks (card STALE-COVER1). Cost: one pass over the declarations, then one
// over each classified slice, O(D + R + W x G + U).
func countTurn(run *store.Run, c classified, promptID string, at int64, roots []string) TurnCounts {
	if promptID == "" {
		promptID = latestPrompt(run.Declarations)
	}
	window := map[int64]bool{}
	var t TurnCounts
	for _, d := range run.Declarations {
		if d.PromptID != nil && *d.PromptID != "" && *d.PromptID == promptID && d.Seq <= at {
			window[d.Seq] = true
			if t.FirstSeq == 0 || d.Seq < t.FirstSeq {
				t.FirstSeq = d.Seq
			}
		}
	}
	if len(window) == 0 {
		return t
	}
	n := countIn(c, window, roots)
	n.FirstSeq = t.FirstSeq
	return n
}

// countIn counts c's records whose declaration's seq is in window, as
// countTurn counts a turn; FirstSeq is the caller's.
func countIn(c classified, window map[int64]bool, roots []string) TurnCounts {
	var t TurnCounts
	paths := map[string]bool{}
	for _, w := range c.writes {
		if w.Path != nil && Relevant(w.Class) && window[w.Seq] && coveredByAny(roots, &w) {
			t.Writes++
			paths[*w.Path] = true
			t.LastWriteSeq = max(t.LastWriteSeq, w.Seq)
		}
	}
	t.PathsWritten = len(paths)
	for _, r := range c.runs {
		if window[r.Seq] {
			t.GateRuns++
		}
	}
	for _, s := range c.unnamed {
		if window[s] {
			t.UnnamedWriteSeq = max(t.UnnamedWriteSeq, s)
		}
	}
	for _, s := range c.rewritten {
		if window[s] {
			t.RewrittenGateSeq = max(t.RewrittenGateSeq, s)
		}
	}
	// A held write bears as an edit would have: a named one where it is
	// relevant and a gate covers it, an unnamed one always.
	for i := range c.held {
		w := &c.held[i]
		if !window[w.Seq] || w.Path != nil && !(Relevant(w.Class) && coveredByAny(roots, &w.write)) {
			continue
		}
		// Each ending kept apart, so a failed edit, which applied nothing,
		// never hides a call that may have written (unconfirmed).
		switch {
		case w.End == EndNone:
			t.UnendedWriteSeq = max(t.UnendedWriteSeq, w.Seq)
		case w.End == EndFailedEdit:
			t.FailedEditSeq = max(t.FailedEditSeq, w.Seq)
		case w.Seq > t.FailedWriteSeq:
			t.FailedWriteSeq, t.FailedWriteEnd = w.Seq, w.End
		}
	}
	return t
}

// DoneWindow is the turns a done claim answers to where they are more than
// its own (doneWindow): their counts, and the row they read.
type DoneWindow struct {
	Turn    TurnCounts
	Summary Summary
}

// doneWindow is the window the done flag reads (BG-OUTCOME phase 2a, the
// manager's ruling of 11-03): the turns from the one holding the session's
// last relevant write a gate of roots covers, through at. A done claim is
// judged by the evidence since the last edit in the gates' projects, as the
// gates' states are, never by its turn alone; an edit no gate covers opens
// nothing (coveredByAny, as the turn counts it), and the Stop's roots are
// its turn's gates' (onTurnRoots). ok is false where the turn
// holds a covered write itself, or the session has none: the turn is the
// window, and the line is the turn's. Cost: three passes over the
// declarations and one over the writes, only for a turn that wrote nothing.
func doneWindow(run *store.Run, c classified, turn TurnCounts, at int64, roots []string) (TurnCounts, bool) {
	if turn.Writes > 0 {
		return TurnCounts{}, false
	}
	var last int64
	for i := range c.writes {
		if w := &c.writes[i]; w.Path != nil && Relevant(w.Class) && w.Seq <= at && w.Seq > last && coveredByAny(roots, w) {
			last = w.Seq
		}
	}
	prompt := ""
	for i := range run.Declarations {
		if d := &run.Declarations[i]; d.Seq == last && d.PromptID != nil {
			prompt = *d.PromptID
		}
	}
	if last == 0 || prompt == "" {
		return TurnCounts{}, false
	}
	from := last
	for i := range run.Declarations {
		if d := &run.Declarations[i]; d.PromptID != nil && *d.PromptID == prompt && d.Seq < from {
			from = d.Seq
		}
	}
	window := map[int64]bool{}
	for i := range run.Declarations {
		if s := run.Declarations[i].Seq; s >= from && s <= at {
			window[s] = true
		}
	}
	t := countIn(c, window, roots)
	t.FirstSeq = from
	return t, true
}

// latestPrompt is the prompt id whose first declaration has the greatest
// seq, digest.selectTurn's "current turn"; "" when no declaration names one.
func latestPrompt(decls []store.Declaration) string {
	first := map[string]int64{}
	for _, d := range decls {
		if d.PromptID == nil || *d.PromptID == "" {
			continue
		}
		if s, ok := first[*d.PromptID]; !ok || d.Seq < s {
			first[*d.PromptID] = d.Seq
		}
	}
	latest, at := "", int64(-1)
	for id, s := range first {
		if s > at {
			latest, at = id, s
		}
	}
	return latest
}

// Kind is the checkpoint's record kind and version, for store.AppendRecord.
func (c *Checkpoint) Kind() (string, int) { return c.Type, c.SchemaVersion }

// SetSeq is the seq the store allocated the checkpoint under the run's lock.
func (c *Checkpoint) SetSeq(seq *int64) { c.Seq = seq }
