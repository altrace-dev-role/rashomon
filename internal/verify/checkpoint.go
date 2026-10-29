package verify

import (
	"cmp"
	"encoding/json"
	"math"
	"slices"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The checkpoint (2.2.10, PF-PG8; card 1.10) is the Snapshot projected into
// the frozen Checkpoint record and cut to the budget. It computes nothing the
// Snapshot does not hold: the header is the snapshot's point and spec, the
// gates keep their refs' seqs, the rule verdicts lose hard_control, and the
// background is every Backgrounded gate at its launch. Cut is pure: the
// record shares no memory with the snapshot, and the same snapshot and time
// give the same bytes.
//
// The cut takes one entry at a time in a fixed order of categories (cutOrder)
// and re-marshals, until the record fits or nothing cuttable remains. What
// survives is decided by severity: practice, then green gates, then loose and
// supported claims, then background, integrity and spec entries oldest
// first, then non-green gates, the Red last, then strict non-green claims
// (D45), the red claim last. How what survives is arranged is not: gates and
// claims keep their places, background, integrity and spec entries their Seq
// order (R3).
//
// The budget is the record as the store writes it. The store fills the seq
// Cut leaves null, so Cut measures against CheckpointMaxBytes less the widest
// seq's room (Q1). The header alone can be over the budget (a ProjectRoot of
// up to store.MaxPathBytes escaped bytes, about 24 KiB): such a record comes
// back whole with Truncated set, so a reader is always told the record is
// over, whether or not anything was cut.
//
// Truncated is part of the record it describes, and "true" is a byte shorter
// than "false": a record one byte over unflagged fits flagged, and neither
// value would then be true of it. So the flag is decided on the record as
// built, nothing cut and the flag false, and every cut after is measured
// flagged, as it will be written. Truncated is therefore any *_omitted count
// set, or the record, unflagged, over the budget.

// seqReserve is the room the store's seq takes over the "null" Cut measures:
// the widest int64, 19 digits, less four bytes. The seq is the only field the
// store fills, so this is the whole reserve.
const seqReserve = len("9223372036854775807") - len("null")

// practiceComponents is what PracticeOmitted counts when the practice goes:
// its five components (2.2.10).
const practiceComponents = 5

// Cut is the snapshot's checkpoint record at now: its projection into the
// Checkpoint, cut in the fixed order until it fits the budget.
func Cut(snap Snapshot, now time.Time) Checkpoint {
	rec := header(snap, now)
	c := cutter{p: project(snap), rec: &rec}
	plan := c.plan()
	c.fill()
	n := size(&rec)
	rec.Truncated = !fits(n)
	if rec.Truncated {
		n = size(&rec)
	}
	for _, it := range plan {
		if fits(n) {
			break
		}
		c.gone[it.list][it.at] = true
		c.fill()
		n = size(&rec)
	}
	return rec
}

// fits is whether a record of n marshalled bytes, seq null, is within the
// budget once the store gives it a seq.
func fits(n int) bool { return n <= CheckpointMaxBytes-seqReserve }

// size is the record's length as the store's marshalLine marshals it. A
// record json cannot encode (a NaN or an infinity in the practice
// components, its only floats) is over every budget, so the practice goes
// first and the record becomes one the store can write.
func size(rec *Checkpoint) int {
	b, err := json.Marshal(rec)
	if err != nil {
		return math.MaxInt
	}
	return len(b)
}

// header is R1: the record's fields that are not cut, from the snapshot's
// point and spec. Seq is null for the store to assign; the point has no
// tool-use id in v0.
func header(snap Snapshot, now time.Time) Checkpoint {
	pt := snap.Point
	rec := Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion,
		RecordedAtUnixMS: now.UnixMilli(), SessionID: pt.SessionID, PromptID: copyOf(pt.PromptID),
		AgentID: copyOf(pt.AgentID), Point: pt.Kind, PointSeq: pt.Seq, TaskRef: copyOf(pt.TaskRef),
		VerifyVersion: snap.VerifyVersion, RulesVersion: snap.RulesVersion, Recording: snap.Recording,
		ProjectRoot: snap.ProjectRoot, Integrity: Integrity{Counts: snap.Integrity.Counts,
			UnknownMarks: snap.Integrity.UnknownMarks, UnreadBases: snap.Integrity.UnreadBases},
		RecordGreen: snap.RecordGreen}
	if sp := snap.Spec; sp != nil {
		rec.SpecDigest, rec.SpecTrust = copyOf(&sp.Digest), copyOf(&sp.Trust)
	}
	return rec
}

func copyOf[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// projected is the snapshot's cuttable lists as the record keeps them, in the
// record's arrangement, in memory of their own.
type projected struct {
	practice   *Practice
	gates      []CheckpointGate
	claims     []ClaimVerdict
	background []BackgroundRef
	unresolved []IntegrityEntry
	spec       []CheckpointRuleVerdict
}

// project is R2 and R3 over the snapshot's lists. The gates' seqs share one
// array and the claims' slots and the verdicts' seqs one each, so copying is
// a handful of allocations, not one per pointer.
func project(snap Snapshot) projected {
	p := projected{gates: make([]CheckpointGate, len(snap.Gates)), background: []BackgroundRef{},
		claims: make([]ClaimVerdict, len(snap.Claims)), spec: make([]CheckpointRuleVerdict, len(snap.SpecVerdicts)),
		unresolved: append(make([]IntegrityEntry, 0, len(snap.Integrity.Unresolved)), snap.Integrity.Unresolved...)}
	seqs := make([]int64, 3*len(snap.Gates))
	for i := range snap.Gates {
		g, s := &snap.Gates[i], seqs[3*i:3*i+3]
		p.gates[i] = CheckpointGate{Gate: g.Gate, Kind: g.Kind, Source: g.Source, Required: g.Required, State: g.State,
			Reason: g.Reason, LastRunSeq: seqInto(g.LastRun, &s[0]), LastGreenSeq: seqInto(g.LastGreen, &s[1]),
			WritesSinceGreen: g.WritesSinceGreen, UnknownTarget: g.UnknownTarget}
		if g.LastWrite != nil {
			s[2] = g.LastWrite.Seq
			p.gates[i].LastWriteSeq = &s[2]
		}
		if g.State == Backgrounded {
			p.background = append(p.background, BackgroundRef{Family: g.Family, Seq: s[0]})
		}
	}
	n := 0
	for _, c := range snap.Claims {
		n += len(c.Gates)
	}
	slots := make([]SlotID, 0, n)
	for i, c := range snap.Claims {
		from := len(slots)
		slots = append(slots, c.Gates...)
		c.Gates = slots[from:len(slots):len(slots)]
		p.claims[i] = c
	}
	n = 0
	for _, v := range snap.SpecVerdicts {
		n += len(v.Seqs)
	}
	vs := make([]int64, 0, n)
	for i, v := range snap.SpecVerdicts {
		from := len(vs)
		vs = append(vs, v.Seqs...)
		p.spec[i] = CheckpointRuleVerdict{Rule: v.Rule, Kind: v.Kind, Verdict: v.Verdict, Seqs: vs[from:len(vs):len(vs)]}
	}
	if pr := snap.Practice; pr != nil {
		c := *pr
		c.Score, c.NotApplicable = copyOf(pr.Score), append(make([]string, 0, len(pr.NotApplicable)), pr.NotApplicable...)
		p.practice = &c
	}
	slices.SortStableFunc(p.background, func(a, b BackgroundRef) int { return cmp.Compare(a.Seq, b.Seq) })
	slices.SortStableFunc(p.unresolved, func(a, b IntegrityEntry) int { return cmp.Compare(a.Seq, b.Seq) })
	slices.SortStableFunc(p.spec, func(a, b CheckpointRuleVerdict) int { return cmp.Compare(firstSeq(a), firstSeq(b)) })
	return p
}

// seqInto is r's seq stored at dst, nil without r. A Backgrounded gate never
// run reads 0 at dst, the oldest launch (Q4).
func seqInto(r *RunRef, dst *int64) *int64 {
	if r == nil {
		return nil
	}
	*dst = r.Seq
	return dst
}

// firstSeq is a rule verdict's smallest seq, which the cut orders it by; one
// with none is older than any.
func firstSeq(v CheckpointRuleVerdict) int64 {
	if len(v.Seqs) == 0 {
		return math.MinInt64
	}
	return slices.Min(v.Seqs)
}

// cutList names a cuttable list of the record.
type cutList int

const (
	cutPractice cutList = iota
	cutGates
	cutClaims
	cutBackground
	cutIntegrity
	cutSpec
	cutLists
)

// category is one row of the cut order: the list it cuts from, which of that
// list's entries it takes, and the order it takes them in.
type category struct {
	list  cutList
	takes func(i int) bool
	order func(a, b int) int
}

// item is one entry the cut can take.
type item struct {
	list cutList
	at   int
}

// cutter holds the projection, the record cut from it, and what the cut has
// taken, by list and position.
type cutter struct {
	p    projected
	rec  *Checkpoint
	gone [cutLists][]bool
}

// plan is every entry the cut can take, in the order it takes them: each
// category of cutOrder in turn, its entries sorted by the category's order.
// Every entry is in exactly one category.
func (c *cutter) plan() []item {
	lens := [cutLists]int{cutGates: len(c.p.gates), cutClaims: len(c.p.claims), cutBackground: len(c.p.background),
		cutIntegrity: len(c.p.unresolved), cutSpec: len(c.p.spec)}
	if c.p.practice != nil {
		lens[cutPractice] = 1
	}
	total := 0
	for l, n := range lens {
		c.gone[l], total = make([]bool, n), total+n
	}
	plan, idx := make([]item, 0, total), make([]int, 0, slices.Max(lens[:]))
	for _, cat := range cutOrder(&c.p) {
		idx = idx[:0]
		for i := range lens[cat.list] {
			if cat.takes(i) {
				idx = append(idx, i)
			}
		}
		slices.SortFunc(idx, cat.order)
		for _, i := range idx {
			plan = append(plan, item{cat.list, i})
		}
	}
	return plan
}

// cutOrder is the fixed order of 2.2.10 as data, with D45 deciding that the
// non-green gates go before the strict non-green claims. Ties fall to the
// older run and then the earlier place (Q3); background, integrity and spec
// entries are already in Seq order, so their place is their age. A state or
// verdict outside the vocabulary ranks -1, before Red (Rank), and goes last.
func cutOrder(p *projected) []category {
	g, cl := p.gates, p.claims
	every := func(int) bool { return true }
	inPlace := func(a, b int) int { return cmp.Compare(a, b) }
	fromEnd := func(a, b int) int { return cmp.Compare(b, a) }
	older := func(a, b int) int { return cmp.Or(cmp.Compare(runKey(g[a]), runKey(g[b])), cmp.Compare(a, b)) }
	supported := func(i int) bool {
		return !cl[i].Strict || cl[i].Verdict == Verdict(Green) || cl[i].Verdict == NoGateKnown
	}
	return []category{
		{cutPractice, every, inPlace},
		{cutGates, func(i int) bool { return g[i].State == Green }, older},
		{cutClaims, supported, fromEnd},
		{cutBackground, every, inPlace},
		{cutIntegrity, every, inPlace},
		{cutSpec, every, inPlace},
		{cutGates, func(i int) bool { return g[i].State != Green }, func(a, b int) int {
			return cmp.Or(cmp.Compare(Rank(g[b].State), Rank(g[a].State)), older(a, b))
		}},
		{cutClaims, func(i int) bool { return !supported(i) }, func(a, b int) int {
			return cmp.Or(cmp.Compare(Rank(State(cl[b].Verdict)), Rank(State(cl[a].Verdict))), fromEnd(a, b))
		}},
	}
}

// runKey is a gate's last run seq, a gate never run older than any.
func runKey(g CheckpointGate) int64 {
	if g.LastRunSeq == nil {
		return math.MinInt64
	}
	return *g.LastRunSeq
}

// fill sets the record's lists to the projection less what the cut took, and
// each list's *_omitted to what it took from it.
func (c *cutter) fill() {
	r, p, g := c.rec, &c.p, &c.gone
	r.Practice, r.PracticeOmitted = p.practice, 0
	if slices.Contains(g[cutPractice], true) {
		r.Practice, r.PracticeOmitted = nil, practiceComponents
	}
	r.Gates, r.GatesOmitted = uncut(r.Gates, p.gates, g[cutGates])
	r.Claims, r.ClaimsOmitted = uncut(r.Claims, p.claims, g[cutClaims])
	r.BackgroundInFlight, r.BackgroundOmitted = uncut(r.BackgroundInFlight, p.background, g[cutBackground])
	r.Integrity.Unresolved, r.IntegrityOmitted = uncut(r.Integrity.Unresolved, p.unresolved, g[cutIntegrity])
	r.SpecVerdicts, r.SpecVerdictsOmitted = uncut(r.SpecVerdicts, p.spec, g[cutSpec])
}

// uncut is all less the entries gone marks, in all's order, written over dst
// (a buffer of its own, never all's), and how many gone marks.
func uncut[T any](dst, all []T, gone []bool) ([]T, int) {
	if dst == nil {
		dst = make([]T, 0, len(all))
	}
	dst = dst[:0]
	for i := range all {
		if !gone[i] {
			dst = append(dst, all[i])
		}
	}
	return dst, len(all) - len(dst)
}
