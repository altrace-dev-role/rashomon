package gather

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The knowledge block's assembly (card REPORT1b). It reads four stores that
// already exist and writes none: the session's checkpoints, streamed in
// order and compared pairwise (the first against the project's newest
// checkpoint before it); the project index; spec-trust.json through the same
// MAC-checked read discovery uses; and the lesson ledger through
// learn.OpenReadOnly, which never quarantines on disk. It is assembled here
// and not in internal/report because it reads files, and not in recap
// because it needs none of it.

// factsNote is the block's last line: the brief's facts are read at every
// session start and never stored, so a past brief is not known.
const factsNote = "read at each session start, not stored (rashomon brief)"

// knowledgeOf is sessionID's Knowledge at now, nil when the session cut no
// checkpoint (there is nothing to fold). run is the session's records, read
// for the time of its first call.
func knowledgeOf(st *store.Store, run *store.Run, sessionID string, now time.Time) *report.Knowledge {
	first, ok := firstCheckpoint(st, sessionID)
	if !ok {
		return nil
	}
	project := first.ProjectRoot
	k := &report.Knowledge{Project: project, Discovered: []report.Change{}, Invalidated: []report.Change{},
		GateStates: []report.GateStates{}, Retained: report.Retained{Specs: []report.SpecHeld{}, Lessons: []report.LessonRef{}, Facts: factsNote}}
	f := &fold{k: k, tallies: map[string]*report.GateStates{}}
	var prev *receipt.Last
	base, sc, err := receipt.Previous(st, receipt.Query{Project: project, Skip: sessionID, Before: first.RecordedAtUnixMS})
	if err == nil && sc.Found {
		k.Baseline = &report.KnowledgeBaseline{SessionID: base.SessionID, RecordedAtUnixMS: base.RecordedAtUnixMS, PointSeq: base.PointSeq}
		f.cut = base.GatesOmitted
		prev = &base
	}
	var last receipt.Last
	_, _ = st.EachRecordOfKind(sessionID, store.TypeVerify, func(line []byte) bool {
		cur, ok := receipt.Decode(line)
		if !ok || cur.ProjectRoot != project {
			return true
		}
		f.cut = max(f.cut, cur.GatesOmitted)
		if prev != nil {
			f.pair(*prev, cur)
		}
		c := cur
		prev, last = &c, cur
		return true
	})
	f.finish()
	start, end := spanOf(run, first.RecordedAtUnixMS, last.RecordedAtUnixMS)
	if last.SpecTrust != "" {
		k.Retained.SpecAtLastStop = &report.SpecAtStop{Trust: last.SpecTrust, Compiled: last.SpecDigest, PointSeq: last.PointSeq}
	}
	knowIndex(st, k, project, now, start, end)
	knowBindings(st, k, project)
	knowLessons(st, k, project, start, end)
	return k
}

// firstCheckpoint is the session's first checkpoint that decodes, and with
// it the project the session's block is about.
func firstCheckpoint(st *store.Store, sessionID string) (receipt.Last, bool) {
	var first receipt.Last
	var found bool
	_, _ = st.EachRecordOfKind(sessionID, store.TypeVerify, func(line []byte) bool {
		first, found = receipt.Decode(line)
		return !found
	})
	return first, found
}

// spanOf is the session's span: from its first recorded call, else its first
// checkpoint, to its last checkpoint, inclusive. A concurrent session's
// first sighting of something also lands in it, which the text says.
func spanOf(run *store.Run, firstCP, lastCP int64) (start, end int64) {
	start = firstCP
	if run != nil {
		for _, d := range run.Declarations {
			if d.RecordedAtMS > 0 && d.RecordedAtMS < start {
				start = d.RecordedAtMS
			}
		}
	}
	return start, max(lastCP, firstCP)
}

// fold is the pairwise comparison's state: the per-gate tally (O(gates)) and
// the largest cut any checkpoint in the fold carried.
type fold struct {
	k       *report.Knowledge
	tallies map[string]*report.GateStates
	cut     int
}

// held is whether declared gates count under a trust: trusted or managed.
func held(trust string) bool { return trust == string(spec.Trusted) || trust == string(spec.Managed) }

// pair compares one checkpoint with the one before it. Required is not
// compared: it flips with the index's prior, which the checkpoint does not
// record, so only a gate's State and the spec's trust are changes.
func (f *fold) pair(prev, cur receipt.Last) {
	seq := cur.PointSeq
	compiled := func() string {
		if prev.SpecDigest == cur.SpecDigest {
			return cur.SpecDigest
		}
		return shortOr(prev.SpecDigest) + " to " + shortOr(cur.SpecDigest)
	}
	switch hp, hc := held(prev.SpecTrust), held(cur.SpecTrust); {
	case hp && !hc:
		f.k.Invalidated = append(f.k.Invalidated, report.Change{
			What: fmt.Sprintf("spec trust, %s to %s at the Stop after #%d (compiled %s)", trustWord(prev.SpecTrust), trustWord(cur.SpecTrust), seq, compiled()),
			From: trustWord(prev.SpecTrust), To: trustWord(cur.SpecTrust), Source: fmt.Sprintf("checkpoint #%d", seq), PointSeq: &seq})
	case !hp && hc:
		f.k.Discovered = append(f.k.Discovered, report.Change{
			What: fmt.Sprintf("spec %s at the Stop after #%d (compiled %s)", cur.SpecTrust, seq, shortOr(cur.SpecDigest)),
			From: trustWord(prev.SpecTrust), To: cur.SpecTrust, Source: fmt.Sprintf("checkpoint #%d", seq), PointSeq: &seq})
	case hp && hc && (prev.SpecTrust != cur.SpecTrust || prev.SpecDigest != cur.SpecDigest):
		f.k.Discovered = append(f.k.Discovered, report.Change{
			What: fmt.Sprintf("spec changed, now %s at the Stop after #%d (compiled %s)", cur.SpecTrust, seq, compiled()),
			From: prev.SpecTrust, To: cur.SpecTrust, Source: fmt.Sprintf("checkpoint #%d", seq), PointSeq: &seq})
	}
	before := make(map[string]string, len(prev.Gates))
	for _, g := range prev.Gates {
		before[g.Gate] = g.State
	}
	for _, g := range cur.Gates {
		from, ok := before[g.Gate]
		if !ok || from == g.State {
			continue
		}
		t := f.tallies[g.Gate]
		if t == nil {
			t = &report.GateStates{Gate: g.Gate}
			f.tallies[g.Gate] = t
		}
		t.Changes++
		t.LastFrom, t.LastTo, t.LastSeq = from, g.State, seq
	}
}

// finish writes the tally out, sorted by gate; a fold that saw any cut
// compares no gate and says by how much it was cut.
func (f *fold) finish() {
	if f.cut > 0 {
		f.k.GatesCut = f.cut
		return
	}
	for _, t := range f.tallies {
		f.k.GateStates = append(f.k.GateStates, *t)
	}
	slices.SortFunc(f.k.GateStates, func(a, b report.GateStates) int { return strings.Compare(a.Gate, b.Gate) })
}

func trustWord(t string) string {
	if t == "" {
		return "no spec"
	}
	return t
}

func shortOr(d string) string {
	if d == "" {
		return "none"
	}
	return d
}

// knowIndex reads the project index: the families required now, with the
// fail-closed prior when none is established, and the families first seen in
// the session's span.
func knowIndex(st *store.Store, k *report.Knowledge, project string, now time.Time, start, end int64) {
	idx, _ := verify.ReadIndex(verify.IndexPath(st.Root(), project))
	k.Retained.Prior = true
	if idx == nil {
		return
	}
	nowMS := now.UnixMilli()
	for _, e := range idx.Entries {
		if idx.Required(e.Family, nowMS) {
			k.Retained.RequiredFamilies++
			k.Retained.Prior = false
		}
		if e.FirstSeen >= start && e.FirstSeen <= end {
			at := e.FirstSeen
			k.Discovered = append(k.Discovered, report.Change{
				What: "gate family first seen in the project: " + e.Kind,
				To:   e.Family, Source: "index", AtMS: &at})
		}
	}
}

// knowBindings reads this project's bindings through the MAC-checked read
// discovery uses; a file that does not carry the store's MAC lists none.
func knowBindings(st *store.Store, k *report.Knowledge, project string) {
	b, err := spec.ReadBindings(spec.OSFS{}, st.Root())
	if errors.Is(err, spec.ErrBindingsUnsigned) {
		k.Retained.BindingsUnsigned = true
		return
	}
	if err != nil || b == nil {
		return
	}
	for _, x := range b.Bindings {
		if x.ProjectRoot != project {
			continue
		}
		p := x.SpecPath
		if r, err := filepath.Rel(project, p); err == nil && !strings.HasPrefix(r, "..") {
			p = filepath.ToSlash(r)
		}
		k.Retained.Specs = append(k.Retained.Specs, report.SpecHeld{Path: p, FileDigest: x.SpecDigest, BoundAtMS: x.BoundAtMS})
	}
}

// knowLessons reads the project's ledger without writing it: the lessons
// approved now, those approved inside the span, and a quarantine inside it.
// A ledger whose state does not check reads every lesson as proposed.
func knowLessons(st *store.Store, k *report.Knowledge, project string, start, end int64) {
	dir := strings.TrimSuffix(baseline.FileName(project), baseline.FileExt)
	l, err := learn.OpenReadOnly(st.Root(), dir)
	if err != nil {
		return
	}
	k.Retained.LedgerUnchecked = l.Quarantined
	for _, ls := range l.Lessons() {
		if ls.Tier != learn.TierProven && ls.Tier != learn.TierProbation {
			continue
		}
		k.Retained.Lessons = append(k.Retained.Lessons, report.LessonRef{ID: ls.ID, Tier: string(ls.Tier)})
		if ls.ApprovedUnixMS >= start && ls.ApprovedUnixMS <= end && ls.ApprovedUnixMS > 0 {
			at := ls.ApprovedUnixMS
			k.Discovered = append(k.Discovered, report.Change{
				What: "lesson " + ls.ID + " approved " + time.UnixMilli(at).UTC().Format("15:04"), To: string(ls.Tier), Source: "learn", AtMS: &at})
		}
	}
	if q := l.QuarantinedAt(); !q.IsZero() && q.UnixMilli() >= start && q.UnixMilli() <= end {
		at := q.UnixMilli()
		k.Invalidated = append(k.Invalidated, report.Change{
			What: "last quarantine " + q.UTC().Format("15:04") + ", every lesson back to proposed", Source: "learn", AtMS: &at})
	}
}
