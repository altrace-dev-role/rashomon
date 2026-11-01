package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The Stop view in recap (card 2.4 R1, R12). recapTurn reads the run once,
// and the digest and this view read that one run. Stop and StopFailure write
// what the view saw: the checkpoint on the record and the project index. The
// catch-up at the next prompt evaluates the same view and writes nothing, so a
// turn no Stop reached still gets its value line. Stop and the catch-up print
// it; StopFailure does not (tech 2.2.11 condition 4).

// stopTurn is the Stop view as recapTurn's line reads it: the view, the
// prompt's delivered notes (the claim scan strips them), the session's impact
// and the fingerprint of the counts the last printed clause carried, whether
// the summary switch is on (tech 2.2.11 condition 3: config.json's summary,
// absent means on, D71), and whether the value line and the done flag may
// print.
type stopTurn struct {
	view    verify.StopView
	notes   []recap.Note
	impact  recap.Impact
	shown   string
	summary bool
	speaks  bool
	// owed is what the turn end leaves the session's next prompt (card
	// GATE-INFER1, W5).
	owed recap.Owed
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
	if err := fault.Fail(fault.PointRecapStopView); err != nil {
		return stopTurn{}, err
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
		Platform: platformRead,
	}
	if writes {
		in.ProjectKey = key
	}
	projectDir, managedDir := specDirs()
	sp := hook.DiscoverSpec(p.CWD, projectDir, managedDir, hook.TrustRoot(run, st.Root(), seq))
	in.Inferred = inferredGate(key, sp, idx)
	v := verify.EvaluateStop(run, idx, sp, pt, now, in)
	v.Unbound = unboundSpecs(run, &v.Snap, sp, st.Root(), seq)
	v.Untrusted = untrustedGates(run, sp, st.Root(), seq)
	// The steer lines are already read; no ledger opens at Stop (learn.Open
	// can write), so the lessons are not read here.
	impact := recap.ImpactOf(v, lines, nil)

	if writes {
		cp := verify.Cut(v.Snap, now)
		err := st.AppendRecord(p.SessionID, &cp)
		if err != nil && !errors.Is(err, store.ErrLockTimeout) {
			return stopTurn{}, err
		}
		if err == nil && key != "" {
			// After the append, so the pointer never names a record that was
			// dropped.
			noteCheckpoint(st.Root(), key, p.SessionID, cp.PointSeq, cp.RecordedAtUnixMS)
		}
		if path != "" {
			_, _ = verify.WriteIndex(path, v.Index)
		}
	}
	return stopTurn{view: v, notes: notes, impact: impact, shown: recap.ImpactShown(st.Root(), p.SessionID),
		summary: summaryOn(st.Root(), key), speaks: event == install.EventStop || pending, owed: owedNote(v, in.Inferred)}, nil
}

// unboundSpecs is each repository spec in sp, project-relative, that the
// session wrote (a store_touched entry names it), that reads untrusted, and
// that a binding names: the binding is at another digest, or discovery would
// have read it trusted, so the user trusted it and an edit dropped the
// trust, which `rashomon spec trust` gives back (SPEC-TRUST-DROP1, d4). A
// spec never trusted is the brief's to name. After a session write under
// the store root no binding holds for the rest of the session
// (hook.TrustRoot, D173), so a trust would not give it back, and nothing is
// named. The bindings and the store-write pass are read only for an
// untrusted spec the session wrote.
func unboundSpecs(run *store.Run, snap *verify.Snapshot, sp *spec.Compiled, root string, seq int64) []string {
	if sp == nil {
		return nil
	}
	var out []string
	var b *spec.Bindings
	for _, s := range sp.Sources {
		if s.Trust != spec.Untrusted || !specWritten(snap, s.Path) {
			continue
		}
		if b == nil {
			var err error
			if b, err = spec.ReadBindings(spec.OSFS{}, root); err != nil || verify.StoreWritten(run, root, seq) {
				return nil
			}
		}
		project := hook.SpecProject(s.Path)
		if _, ok := b.Of(project, s.Path); ok {
			if r, err := filepath.Rel(project, s.Path); err == nil {
				out = append(out, filepath.ToSlash(r))
			}
		}
	}
	return out
}

// untrustedGates is each gate sp declares from a source that reads
// untrusted, with the source project-relative as unboundSpecs names it
// (STOP-UNBOUND-FIRST). After a session write under the store root it is
// nil: no trust holds for the rest of the session then (D173), so the Stop
// could name no step that makes the gate count. The store-write pass is
// read only when an untrusted spec declares a gate.
func untrustedGates(run *store.Run, sp *spec.Compiled, root string, seq int64) []verify.UntrustedGate {
	if sp == nil {
		return nil
	}
	var out []verify.UntrustedGate
	for _, d := range sp.Gates {
		s := sp.Sources[d.Source]
		if s.Trust != spec.Untrusted {
			continue
		}
		if r, err := filepath.Rel(hook.SpecProject(s.Path), s.Path); err == nil {
			out = append(out, verify.UntrustedGate{ID: d.ID, Spec: filepath.ToSlash(r)})
		}
	}
	if out != nil && verify.StoreWritten(run, root, seq) {
		return nil
	}
	return out
}

// specWritten reports whether a store_touched entry in snap names path. The
// entry's path is relativised to the call's project or cwd, so it names
// path when it is path or ends it after a separator.
func specWritten(snap *verify.Snapshot, path string) bool {
	slashed := filepath.ToSlash(path)
	return slices.ContainsFunc(snap.Integrity.Unresolved, func(e verify.IntegrityEntry) bool {
		return e.Check == verify.StoreTouched && e.Path != "" && (e.Path == slashed || strings.HasSuffix(slashed, "/"+e.Path))
	})
}

// noteCheckpoint makes the checkpoint at seq the project's Last in its own
// file, never the index, which alpha.1 cannot read with a key it does not name
// (INDEX-POINTER-FILE1): read, noted and written back. A file that does not
// read starts over from this checkpoint, and a write that fails is dropped:
// the pointer is a hint, and receipt.Pointed validates what it names. Two
// Stops at once race on it (read, note, write, no lock), and the last writer
// wins.
func noteCheckpoint(root, key, sessionID string, seq, atMS int64) {
	path := verify.CheckpointsPath(root, key)
	c, _ := verify.ReadCheckpoints(path)
	if c == nil {
		c = &verify.Checkpoints{}
	}
	c.NoteCheckpoint(sessionID, seq, atMS)
	_ = verify.WriteCheckpoints(path, c)
}

// summaryOn is the summary switch for the project key (baseline.Key of the
// turn's cwd): on unless config.json says off for the user or the project. A file
// that does not read leaves it on, which is the default: a switch that cannot
// be read is not one a person was heard to set.
func summaryOn(root, project string) bool {
	c, err := store.ReadConfig(root)
	return err != nil || c.SummaryOnFor(project)
}

// line is what speaks when recap.Line has no sentence (card 2.4 R12): the
// done flag, else the value line while the summary switch is on. clause is
// the fingerprint of the AutoSteer clause the line carries, "" with none:
// the clause prints only when the counts differ from the last one printed.
func (s stopTurn) line(sessionID string, fromPlugin bool) (line string, ok bool, clause string) {
	if !s.speaks {
		return "", false, ""
	}
	if flag, ok := recap.DoneFlag(s.view, sessionID, fromPlugin); ok {
		return flag, true, ""
	}
	if !s.summary {
		return "", false, ""
	}
	im := s.impact
	if fp := im.Fingerprint(); fp == s.shown || im.Empty() {
		im = recap.Impact{}
	} else {
		clause = fp
	}
	l := recap.ValueLine(s.view, im, sessionID, fromPlugin)
	if l == "" {
		clause = ""
	}
	return l, l != "", clause
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
