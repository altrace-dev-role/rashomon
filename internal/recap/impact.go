package recap

import (
	"encoding/json"
	"slices"
	"strconv"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The session's impact (card REPORT1a, D162: evidence, never a score). One
// derivation feeds both places a person reads it, the AutoSteer clause on the
// Stop line and the report's autosteer block, so the two cannot disagree.
// Every number is the length of a list of records with their seqs beside
// them, and nothing is estimated: a finding is resolved only by a later green
// run of the gate its line names, and a row the record cannot settle is
// unknown, never met.

// NoteRef is one note delivered to the agent: the steer line's id, its
// pattern, the event that carried it, the prompt it went with (empty with
// none) and the seq the line was decided after.
type NoteRef struct {
	ID, Pattern, Event, PromptID string
	Seq                          int64
}

// FindingState is whether a later green run of a finding's gate resolved it.
type FindingState string

const (
	// FindingResolved: the gate the line names has a green run after Seq.
	FindingResolved FindingState = "resolved"
	// FindingOpen: the line names a gate and none of its runs after Seq is
	// green.
	FindingOpen FindingState = "open"
	// FindingUnchecked: the line names no gate, so no run can settle it.
	FindingUnchecked FindingState = "unchecked"
)

// FindingRef is one finding a steer line raised. Seq is the line's last
// evidence seq (its after_seq with none); ResolvedBy is the seq of the green
// run that resolved it, 0 unless the state is resolved.
type FindingRef struct {
	ID, Pattern, Gate string
	Seq, ResolvedBy   int64
	State             FindingState
}

// CheckRef is one expected evidence row of the task at the Stop: its kind,
// its state, the gate that settles it ("" with none) and the seq of that
// gate's last green run (0 with none).
type CheckRef struct {
	Kind, Gate, Reason string
	State              verify.ObligationState
	Seq                int64
}

// UnknownRef is something the record cannot settle: a masked run (Seq its
// call) or an expected row with no readable state (Seq 0).
type UnknownRef struct {
	Kind string
	Seq  int64
}

// The Kind of an UnknownRef.
const (
	UnknownMasked = "masked run"
	UnknownCheck  = "check"
)

// LessonCounts is the project's lesson ledger by tier. Read is false when
// no ledger was opened, which is unknown and never zero; Approved is the
// proven and the probation tiers.
type LessonCounts struct {
	Read                        bool
	Approved, Proposed, Demoted int
}

// Impact is what the record shows of the session's AutoSteer notes.
type Impact struct {
	Delivered []NoteRef
	Findings  []FindingRef
	Checks    []CheckRef
	Lessons   LessonCounts
	Unknown   []UnknownRef
}

// Resolved is the findings a later green run resolved.
func (im Impact) Resolved() []FindingRef {
	return slices.DeleteFunc(slices.Clone(im.Findings), func(f FindingRef) bool { return f.State != FindingResolved })
}

// Met is how many checks are met, and Unknown how many are unknown.
func (im Impact) Met() (met, unknown int) {
	for _, c := range im.Checks {
		switch c.State {
		case verify.ObligationMet:
			met++
		case verify.ObligationUnknown:
			unknown++
		}
	}
	return met, unknown
}

// Empty is whether there is nothing to print: no note, finding, check or
// unknown.
func (im Impact) Empty() bool {
	return len(im.Delivered)+len(im.Findings)+len(im.Checks)+len(im.Unknown) == 0
}

// Fingerprint is the counts as one string: the Stop line prints its clause
// only when this differs from the one printed last (recap.json keeps it).
func (im Impact) Fingerprint() string {
	met, unknown := im.Met()
	return "d" + strconv.Itoa(len(im.Delivered)) + "f" + strconv.Itoa(len(im.Findings)) +
		"r" + strconv.Itoa(len(im.Resolved())) + "c" + strconv.Itoa(met) + "/" + strconv.Itoa(len(im.Checks)) +
		"u" + strconv.Itoa(unknown+len(im.Unknown))
}

// findingPatterns are the patterns whose lines are findings: a claim the
// record contradicts, a gate that failed or went stale, a rule the session
// broke. A briefing, a preflight or a known fix is a note and no finding. A
// function with sync.Once and never built at init (RES-2).
func findingPatterns() map[string]bool {
	findingPatternsOnce.Do(func() {
		findingPatternsOnce.v = map[string]bool{}
		for _, p := range []pattern.ID{pattern.RegressionLocation, pattern.SubtaskGate, pattern.ClaimVsRecord,
			pattern.MaskedStatus, pattern.OrderingRule, pattern.IdenticalRetry, pattern.StaleEvidence} {
			findingPatternsOnce.v[string(p)] = true
		}
	})
	return findingPatternsOnce.v
}

var findingPatternsOnce struct {
	sync.Once
	v map[string]bool
}

// lineKeys is the KeySet one steer line is read with for the impact.
func lineKeys() *jsonscan.KeySet {
	lineKeysOnce.Do(func() {
		lineKeysOnce.v = jsonscan.NewKeySet("id", "state", "prompt_id", "event", "pattern", "model_text",
			"after_seq", "evidence_seqs", "slots")
	})
	return lineKeysOnce.v
}

var lineKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

const (
	lkID = iota
	lkState
	lkPrompt
	lkEvent
	lkPattern
	lkModelText
	lkAfter
	lkEvidence
	lkSlots
	lkCount
)

// ImpactOf is the session's impact from the Stop view v, the session's
// steer lines and, when a ledger opened, its lessons. A finding is resolved
// when the gate its line names has a green run after the finding's seq (v's
// gates, LastGreen); checks are v.Expected's rows; unknown is the masked-
// status findings and the expected rows with no readable state. A line that
// does not read is passed over, and a line seen twice counts once.
func ImpactOf(v verify.StopView, lines [][]byte, ls *learn.Ledger) Impact {
	var im Impact
	seen := map[string]bool{}
	var out [lkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, lineKeys(), out[:]) != nil || text(b, out[lkState]) != string(steer.LineStateDelivered) {
			continue
		}
		id, pat := text(b, out[lkID]), text(b, out[lkPattern])
		if id != "" && seen[id] {
			continue
		}
		seen[id] = true
		after := number(b, out[lkAfter])
		if text(b, out[lkModelText]) != "" {
			im.Delivered = append(im.Delivered, NoteRef{ID: id, Pattern: pat, Event: text(b, out[lkEvent]),
				PromptID: text(b, out[lkPrompt]), Seq: after})
		}
		if !findingPatterns()[pat] {
			continue
		}
		f := FindingRef{ID: id, Pattern: pat, Seq: after, Gate: slotGate(b, out[lkSlots])}
		if ev := seqs(b, out[lkEvidence]); len(ev) > 0 {
			f.Seq = slices.Max(ev)
		}
		f.State, f.ResolvedBy = resolution(v, f)
		im.Findings = append(im.Findings, f)
		if pat == string(pattern.MaskedStatus) {
			im.Unknown = append(im.Unknown, UnknownRef{Kind: UnknownMasked, Seq: f.Seq})
		}
	}
	for _, e := range v.Expected {
		c := CheckRef{Kind: e.Kind, Gate: e.Gate, Reason: e.Reason, State: e.State}
		for _, g := range v.Snap.Gates {
			if g.Gate == e.Gate && e.State == verify.ObligationMet && g.LastGreen != nil {
				c.Seq = g.LastGreen.Seq
			}
		}
		im.Checks = append(im.Checks, c)
		if e.State == verify.ObligationUnknown {
			im.Unknown = append(im.Unknown, UnknownRef{Kind: UnknownCheck})
		}
	}
	if ls != nil {
		im.Lessons.Read = true
		for _, l := range ls.Lessons() {
			switch l.Tier {
			case learn.TierProven, learn.TierProbation:
				im.Lessons.Approved++
			case learn.TierProposed:
				im.Lessons.Proposed++
			case learn.TierDemoted:
				im.Lessons.Demoted++
			}
		}
	}
	return im
}

// resolution reads f against the view's gates. The line's gate slot is a
// program word, or the gate's kind when the program is a prefix word, which
// is how the clause names a gate too (gateSet.name).
func resolution(v verify.StopView, f FindingRef) (FindingState, int64) {
	if f.Gate == "" {
		return FindingUnchecked, 0
	}
	state, by := FindingOpen, int64(0)
	for _, g := range v.Snap.Gates {
		if g.Gate == "" || v.Programs[g.Gate] != f.Gate && g.Kind != f.Gate {
			continue
		}
		if g.LastGreen != nil && g.LastGreen.Seq > f.Seq && g.State == verify.Green {
			if state != FindingResolved || g.LastGreen.Seq < by {
				state, by = FindingResolved, g.LastGreen.Seq
			}
		}
	}
	return state, by
}

// number is a number value's integer, 0 for any other kind.
func number(b []byte, sp jsonscan.Span) int64 {
	if sp.Kind != jsonscan.Number {
		return 0
	}
	n, err := strconv.ParseInt(string(sp.Raw(b)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// seqs is an array of integers, nil for any other kind.
func seqs(b []byte, sp jsonscan.Span) []int64 {
	if sp.Kind != jsonscan.Array {
		return nil
	}
	var out []int64
	if json.Unmarshal(sp.Raw(b), &out) != nil {
		return nil
	}
	return out
}

// slotGate is the gate slot of a line's slots object, "" with none.
func slotGate(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.Object {
		return ""
	}
	g, err := jsonscan.Lookup(b, sp, "gate")
	if err != nil {
		return ""
	}
	return text(b, g)
}

// GateName is g's name as the Stop line gives it: its program word, else its
// kind when the program is a prefix word or unknown.
func GateName(v verify.StopView, g verify.GateState) string {
	return gateSet{programs: v.Programs}.name(&g)
}
