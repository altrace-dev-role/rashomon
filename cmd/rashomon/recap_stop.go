package main

import (
	"errors"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The Stop view in recap (card 2.4 R1, R12). recapTurn reads the run once,
// and the digest and this view read that one run. Stop and StopFailure write
// what the view saw: the checkpoint on the record and the project index. The
// catch-up at the next prompt evaluates the same view and writes nothing, so a
// turn no Stop reached still gets its value line. Stop and the catch-up print
// it; StopFailure does not (tech 2.2.11 condition 4).

// summaryOn is the summary switch (tech 2.2.11 condition 3). No config file
// is read in v0 and no key names the switch, so it is on (D71: no config,
// on); a reader of one replaces this constant.
const summaryOn = true

// stopTurn is the Stop view as recapTurn's line reads it: the view, the
// prompt's delivered notes, and whether the value line and the done flag may
// print.
type stopTurn struct {
	view   verify.StopView
	notes  []recap.Note
	speaks bool
}

// stopView evaluates the turn at the Stop point (card 2.4 R3) over run, the
// read recapTurn made, for the prompt the digest chose. A Stop-family event
// or the catch-up has a view; any other payload, a payload naming no
// session, and a run with no seq have none. The turn is judged against the
// spec discovered from the payload's cwd as the steer step discovers it
// (hook.DiscoverSpec, card 3.1 (c)), and with the content now of each file
// the session wrote (hook.TreeShas, card P5A). On Stop and StopFailure it
// appends the checkpoint and, with a cwd, writes the observed index.
//
// A checkpoint append that times out on the lock is dropped and the turn
// still speaks (D76): the record is derivable from the run. Any other append
// error is returned, and recapTurn prints nothing. An index read or write
// error is not: the index is bookkeeping, and the prior a missing one falls
// back to, every learned gate required, is the safe side (D74).
func stopView(st *store.Store, run *store.Run, d *digest.Digest, p recapPayload, pending bool, now time.Time) (stopTurn, error) {
	event := p.Harness.HookEventName
	writes := event == install.EventStop || event == install.EventStopFailure
	if run == nil || !writes && !pending {
		return stopTurn{}, nil
	}
	seq := run.LastSeq()
	if seq == 0 {
		return stopTurn{}, nil
	}
	pt := verify.Point{Kind: verify.PointStop, Seq: seq, SessionID: p.SessionID, StoreRoot: st.Root()}
	if d.PromptID != "" {
		pt.PromptID = &d.PromptID
	}
	// The files the session's tree names, as they are now: the claim
	// point's side of the binding (card P5A, D140).
	pt.Tree = hook.TreeShas(verify.TreeTargets(run))

	var path string
	var idx *verify.ProjectIndex
	key := baseline.Key(p.CWD)
	if key != "" {
		path = verify.IndexPath(st.Root(), key)
		idx, _ = verify.ReadIndex(path)
	}
	lines, _, _ := st.ReadSteer(p.SessionID)
	notes := recap.DeliveredNotes(lines, d.PromptID)
	in := verify.StopInput{
		PromptID: d.PromptID, Key: st.Key(), Delivered: modelTexts(notes),
		Head: []byte(p.LastAssistantMessage), Tail: []byte(p.LastMessageTail),
	}
	if writes {
		in.ProjectKey = key
	}
	projectDir, managedDir := specDirs()
	sp := hook.DiscoverSpec(p.CWD, projectDir, managedDir, hook.TrustRoot(run, st.Root(), seq))
	v := verify.EvaluateStop(run, idx, sp, pt, now, in)

	if writes {
		cp := verify.Cut(v.Snap, now)
		if err := st.AppendRecord(p.SessionID, &cp); err != nil && !errors.Is(err, store.ErrLockTimeout) {
			return stopTurn{}, err
		}
		if path != "" {
			_, _ = verify.WriteIndex(path, v.Index)
		}
	}
	return stopTurn{view: v, notes: notes, speaks: event == install.EventStop || pending}, nil
}

// line is what speaks when recap.Line has no sentence (card 2.4 R12): the
// done flag, else the value line while the summary switch is on.
func (s stopTurn) line(sessionID string, fromPlugin bool) (string, bool) {
	if !s.speaks {
		return "", false
	}
	if flag, ok := recap.DoneFlag(s.view, sessionID, fromPlugin); ok {
		return flag, true
	}
	if !summaryOn {
		return "", false
	}
	l := recap.ValueLine(s.view, s.notes, sessionID, fromPlugin)
	return l, l != ""
}

// modelTexts is each note's model text, the lines the claim scan strips
// (card 2.4 R8b).
func modelTexts(notes []recap.Note) [][]byte {
	texts := make([][]byte, 0, len(notes))
	for _, n := range notes {
		texts = append(texts, n.ModelText)
	}
	return texts
}
