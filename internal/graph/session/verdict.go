package session

import (
	"fmt"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// FollowedLabel is whether the agent acted on a delivered line.
type FollowedLabel string

// The labels.
const (
	FollowedYes     FollowedLabel = "yes"
	FollowedNo      FollowedLabel = "no"
	FollowedPartial FollowedLabel = "partial"
)

// Outcome is what happened after, by the strongest evidence available.
type Outcome string

// The outcomes. Only the hidden pair counts for a lesson
// (CountsForLesson): the agent's own gates are what a hidden check exists
// to second-guess.
const (
	OutcomeUnknown     Outcome = "unknown"
	OutcomeGreenLocal  Outcome = "green_local"
	OutcomeRedLocal    Outcome = "red_local"
	OutcomeGreenHidden Outcome = "green_hidden"
	OutcomeRedHidden   Outcome = "red_hidden"
)

// Followed is the label on a delivered line at Stop.
type Followed struct {
	Label    FollowedLabel
	Outcome  Outcome
	JudgedBy string
}

// CountsForLesson reports whether an outcome may move a lesson's tier or
// tally: hidden evidence only.
func CountsForLesson(o Outcome) bool { return o == OutcomeGreenHidden || o == OutcomeRedHidden }

// Delivered is one brief line: its predicate over the session says
// whether the agent acted on it, Unmet says what is still undone, and
// Hidden names the hidden test cases that judge it.
type Delivered struct {
	ID     string
	Line   int
	Pred   func(*Session) FollowedLabel
	Unmet  func(*Session) string
	Hidden []string
	Label  Followed
}

// Obligation is one owed row with the certain edges it was derived from.
type Obligation struct {
	Cmd   string
	Why   string
	Basis []graph.EdgeRef
}

// NewObligation is the one obligation writer: it refuses any basis that
// is not certain at rev (graph.CheckBasis) and writes no row.
func NewObligation(store *graph.Store, rev graph.Rev, cmd, why string, basis []graph.EdgeRef) (Obligation, error) {
	if err := graph.CheckBasis(store, rev, basis); err != nil {
		return Obligation{}, err
	}
	return Obligation{Cmd: cmd, Why: why, Basis: basis}, nil
}

// Verdict is the Stop surface.
type Verdict struct {
	Done    bool
	Rev     graph.Rev
	Owed    []Obligation
	Refused []error
	Text    string
}

// Stop is the verdict at seq: owed gates with their bases, pending
// conflicts moved to owed, the requirements, and a label on every
// delivered line.
func (s *Session) Stop(seq int64) Verdict {
	s.advance(seq)
	rev := s.Rev()
	v := Verdict{Rev: rev}
	var lines []string
	for _, og := range s.owedGates() {
		unit, ok := s.unitOf(og.Last.Path, rev)
		if !ok {
			v.Refused = append(v.Refused, fmt.Errorf("%s: no unit contains %s at %s", og.Gate.Cmd, og.Last.Path, rev))
			continue
		}
		why := fmt.Sprintf("%s written at call %d, no run since", og.Last.Path, og.Last.Seq)
		o, err := NewObligation(s.Store, rev, og.Gate.Cmd, why, []graph.EdgeRef{{Src: unit, Pred: graph.Contains, Dst: graph.FileID(og.Last.Path)}})
		if err != nil {
			v.Refused = append(v.Refused, err)
			continue
		}
		v.Owed = append(v.Owed, o)
		lines = append(lines, o.Cmd+" ("+o.Why+")")
	}
	v.Done = len(v.Owed) == 0 && len(s.ConflictsIn(Pending)) == 0
	var text []string
	if v.Done {
		text = append(text, fmt.Sprintf("rashomon: done (rev %s).", rev))
	} else {
		text = append(text, fmt.Sprintf("rashomon: not done (rev %s).", rev))
	}
	if len(lines) > 0 {
		text = append(text, "Owed: "+strings.Join(lines, "; ")+".")
	}
	for _, c := range s.ConflictsIn(Pending) {
		c.State = Owed
		text = append(text, fmt.Sprintf("%s signature is %s (rev %s); caller %s still passes %d arguments (rev %s).",
			short(c.Edge.Dst), c.Want, c.Rev, s.callerAt(c.Edge, rev), c.Got, c.Edge.Rev))
	}
	text = append(text, s.reqSummary())
	text = append(text, s.labelLines()...)
	v.Text = strings.Join(text, "\n")
	return v
}

// reqSummary is one line over the requirements.
func (s *Session) reqSummary() string {
	var parts []string
	for _, r := range s.Reqs {
		state := "met"
		if !r.Holds {
			state = "not met"
		}
		if r.Words != nil {
			parts = append(parts, r.ID+" necessary condition "+state)
		} else {
			parts = append(parts, r.ID+" "+state)
		}
	}
	return strings.Join(parts, "; ") + "."
}

// labelLines labels every delivered line and renders the "Not acted on"
// and "Recorded" lines.
func (s *Session) labelLines() []string {
	var notActed, recorded []string
	for _, d := range s.Delivered {
		if d.Pred == nil {
			continue
		}
		d.Label.Label = d.Pred(s)
		d.Label.JudgedBy = "predicate"
		if d.Label.Outcome == "" || d.Label.Outcome == OutcomeUnknown || d.Label.Outcome == OutcomeGreenLocal || d.Label.Outcome == OutcomeRedLocal {
			d.Label.Outcome = s.localOutcome()
		}
		if d.Label.Label != FollowedYes && d.Unmet != nil {
			notActed = append(notActed, fmt.Sprintf("brief line %d (%s)", d.Line, d.Unmet(s)))
		}
		recorded = append(recorded, fmt.Sprintf("line %d followed=%s, outcome=%s", d.Line, d.Label.Label, d.Label.Outcome))
	}
	var out []string
	if len(notActed) > 0 {
		out = append(out, "Not acted on: "+strings.Join(notActed, "; ")+".")
	}
	if len(recorded) > 0 {
		out = append(out, "Recorded: "+strings.Join(recorded, "; ")+".")
	}
	return out
}

// localOutcome is green when every gate ran after the last write and
// passed, red when one ran and failed, unknown otherwise.
func (s *Session) localOutcome() Outcome {
	if len(s.Gates) == 0 {
		return OutcomeUnknown
	}
	last := int64(0)
	if len(s.Touched) > 0 {
		last = s.Touched[len(s.Touched)-1].Seq
	}
	for _, g := range s.Gates {
		if g.RanAt <= last {
			return OutcomeUnknown
		}
		if !g.Passed {
			return OutcomeRedLocal
		}
	}
	return OutcomeGreenLocal
}

// IngestHidden records hidden test results against the delivered lines
// that name them: any failing case is red_hidden, all passing is
// green_hidden. Lines naming no case are untouched.
func (s *Session) IngestHidden(results map[string]bool) {
	for _, d := range s.Delivered {
		judged := false
		red := false
		for _, name := range d.Hidden {
			if pass, ran := results[name]; ran {
				judged = true
				red = red || !pass
			}
		}
		if !judged {
			continue
		}
		d.Label.Outcome = OutcomeGreenHidden
		if red {
			d.Label.Outcome = OutcomeRedHidden
		}
		d.Label.JudgedBy = "hidden"
	}
}

// unitOf is the unit that contains path at rev, by the contains edge.
func (s *Session) unitOf(path string, rev graph.Rev) (graph.ID, bool) {
	for _, e := range s.Store.In(graph.FileID(path), rev, graph.Contains) {
		return e.Src, true
	}
	return "", false
}

// short is a symbol id's qualified name.
func short(id graph.ID) string {
	parts := strings.Split(string(id), ":")
	return parts[len(parts)-1]
}

// callerAt is the caller's file and line: the file that declares the
// calling symbol at rev, and the line the edge carries.
func (s *Session) callerAt(e graph.Edge, rev graph.Rev) string {
	file := "?"
	for _, d := range s.Store.In(e.Src, rev, graph.Declares) {
		file = strings.TrimPrefix(string(d.Src), "file:")
	}
	return file + ":" + e.Attrs["line"]
}
