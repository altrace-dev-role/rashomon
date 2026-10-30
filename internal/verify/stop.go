package verify

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The Stop view (card 2.4) is the third view over evaluate's one
// classification, after the obligation ledger and the steer view: at the
// turn's end it gives recap the value line's row and counts, the claim scan
// the done flag reads, the gate programs the sentences name, and the project
// index observed for this session. It is pure, as Evaluate is: the caller
// reads the index and the steer log before and writes the index and the
// checkpoint after.

// StopInput is what the Stop hook read beside the record: the prompt the
// digest chose, the project key of the payload's cwd ("" when it named none),
// the final message's head and tail windows, the install key the quotes are
// HMACed under and the declared gates' lines digested under, and the model
// texts of the notes delivered in the prompt.
type StopInput struct {
	PromptID   string
	ProjectKey string
	Head, Tail []byte
	Key        []byte
	Delivered  [][]byte
}

// StopView is EvaluateStop's result. Summary is TurnSummary over Snap and
// Turn; Programs maps each gate of Snap to its last run's program word ("" when
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
}

// EvaluateStop is the Stop hook's view at pt. One classification feeds the
// snapshot, the programs and the index. An incomplete record classifies
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
	v.Expected = expected(snap, evidenceIn{sp: sp, atStop: true, disclosed: v.Scan.Disclosed, wrote: wrote})
	if snap.Recording == Complete {
		v.Snap.Claims = v.Scan.Claims
	}
	return v
}

// programs maps each gate to the program of the run its LastRun names, found
// by seq among runs (seq order, so a binary search would do; a gate list is
// short and a map over runs is one pass).
func programs(gates []GateState, runs []gateRun) map[string]string {
	bySeq := make(map[int64]string, len(runs))
	for _, r := range runs {
		bySeq[r.Seq] = r.Program
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
