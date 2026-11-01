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
	Snap     Snapshot
	Summary  Summary
	Turn     TurnCounts
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
}

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
	snap := evaluate(run, idx, sp, in.Key, pt, now, &c)
	pinRequired(&snap, in.Required)
	counted := c
	if snap.Recording != Complete {
		counted.runs, counted.writes, counted.unnamed, counted.rewritten = classifyWith(run, pickExecutions(run))
	}
	turn := countTurn(run, counted, in.PromptID, pt.Seq)
	v := StopView{Snap: snap, Summary: TurnSummary(snap, turn), Turn: turn, Programs: programs(snap.Gates, c.runs), Index: idx}
	if v.Index == nil {
		v.Index = &ProjectIndex{ProjectKey: in.ProjectKey}
	}
	if in.ProjectKey != "" {
		v.Index.Observe(c.runs, pt.SessionID, now.UnixMilli())
	}
	wrote := WroteRelevant(run, pt.Seq)
	v.Scan = Claims(in.Head, in.Tail, in.Key, snap.Gates, wrote, SourceFinalMessage, in.Delivered)
	v.Expected = expected(snap, evidenceIn{sp: sp, atStop: true, unread: in.MessageUnread, disclosed: v.Scan.Disclosed, wrote: wrote})
	v.Obligations = ledgerOf(snap, &c, v.Expected)
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
// (D229); an older record's is the line's program word.
func programs(gates []GateState, runs []gateRun) map[string]string {
	bySeq := make(map[int64]string, len(runs))
	for _, r := range runs {
		bySeq[r.Seq] = cmp.Or(shape.RunnerNameOf(r.RunnerID), r.Program)
	}
	out := make(map[string]string, len(gates))
	for _, g := range gates {
		if g.LastRun != nil {
			out[g.Gate] = bySeq[g.LastRun.Seq]
		}
	}
	return out
}

// countTurn counts the prompt's turn up to seq at (2.2.11 TurnCounts). The
// window is the seq set of the prompt's declarations: gate runs and writes
// carry no prompt id, so membership is by their declaration's seq. A
// declaration whose prompt id is nil or "" is in no window. promptID ""
// selects the prompt digest's selectTurn selects, the one whose first
// declaration is latest (D72); a run with no prompt id has an empty window,
// never the whole run. Writes are the relevant named writes, as gateStates
// counts them. Cost: one pass over the declarations, then one over each
// classified slice, O(D + R + W + U).
func countTurn(run *store.Run, c classified, promptID string, at int64) TurnCounts {
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
	paths := map[string]bool{}
	for _, w := range c.writes {
		if w.Path != nil && Relevant(w.Class) && window[w.Seq] {
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
	return t
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
