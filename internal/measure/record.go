package measure

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// calls fills the call rows from the run: declarations, the writes among
// them by shape class or the may-write bit, the test-class runs by their
// execution's outcome, and first to last record stamp.
func (s *Side) calls(run *store.Run) {
	ended := make(map[string]string, len(run.Executions))
	first, last := int64(0), int64(0)
	stamp := func(ms int64) {
		if ms <= 0 {
			return
		}
		if first == 0 || ms < first {
			first = ms
		}
		last = max(last, ms)
	}
	for i := range run.Executions {
		e := &run.Executions[i]
		ended[e.ToolUseID] = e.Outcome
		stamp(e.RecordedAtMS)
	}
	for i := range run.Terminals {
		stamp(run.Terminals[i].RecordedAtMS)
	}
	for i := range run.Declarations {
		d := &run.Declarations[i]
		stamp(d.RecordedAtMS)
		s.Calls++
		if _, ok := ended[d.ToolUseID]; ok {
			s.Executed++
		}
		if d.Shape.VerbClass == shape.VerbWrite || d.Shape.MayWrite {
			s.Writes++
		}
		if d.Shape.VerbClass != shape.VerbTest {
			continue
		}
		switch ended[d.ToolUseID] {
		case store.ExecOK:
			s.TestRuns.Passed++
		case store.ExecFailed:
			s.TestRuns.Failed++
		default:
			s.TestRuns.Unended++
		}
	}
	s.firstMS, s.WallMS = first, last-first
}

// incomplete reports a record that may undercount: a line the reader could
// not read, a coverage record that is not verified, or a gap naming the
// session (a pause, a forget).
func incomplete(st *store.Store, run *store.Run, session string) bool {
	if run.Skipped > 0 {
		return true
	}
	for i := range run.Coverage {
		if run.Coverage[i].State != store.StateVerified {
			return true
		}
	}
	gaps, skipped, err := st.ReadGaps()
	if err != nil || skipped > 0 {
		return true
	}
	return slices.ContainsFunc(gaps, func(g store.Gap) bool { return g.SessionID == session })
}

// line is the part of a steer line the pair reads.
type line struct {
	Pattern      string              `json:"pattern"`
	Kind         string              `json:"kind"`
	Key          string              `json:"key"`
	Arm          steer.Arm           `json:"arm"`
	PromptID     *string             `json:"prompt_id"`
	Level        string              `json:"level"`
	AfterSeq     int64               `json:"after_seq"`
	State        steer.LineState     `json:"state"`
	SuppressedBy *steer.SuppressedBy `json:"suppressed_by"`
	Delivery     *steer.Delivery     `json:"delivery"`
}

func decodeLines(raw [][]byte) ([]line, error) {
	out := make([]line, len(raw))
	for i, b := range raw {
		if err := json.Unmarshal(b, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// arm sets the recorded arm: the one arm every line names, or ArmUnknown
// with no line. Lines of two arms, or of an arm other than the one asked,
// are an error naming the session: a live line in a forced session is a call
// the forced arm did not reach.
func (s *Side) arm(lines []line, ask Ask) error {
	s.RecordedArm = ArmUnknown
	for i := range lines {
		a := string(lines[i].Arm)
		switch {
		case s.RecordedArm == ArmUnknown:
			s.RecordedArm = a
		case a != s.RecordedArm:
			return fmt.Errorf("session %s has steer lines of two arms, %q and %q; a pair needs one arm per session",
				ask.SessionID, s.RecordedArm, a)
		}
	}
	if s.RecordedArm != ArmUnknown && s.RecordedArm != string(ask.Arm) {
		return fmt.Errorf("session %s was asked as arm %q, and its steer lines name arm %q", ask.SessionID, ask.Arm, s.RecordedArm)
	}
	return nil
}

// notes counts the lines given to the agent (delivery delivered), the notes
// arm off withheld (a nudge suppressed arm_off: what the steer arm gave), and
// every line not given by why: its suppressed_by, else its delivery
// (flag_only, holdout), else its state. A prompt whose given and withheld
// notes pass the router's per-prompt cap is an error: the log contradicts
// the router that wrote it.
func (s *Side) notes(lines []line) error {
	counts := map[string]int{}
	perPrompt := map[string]int{}
	for i := range lines {
		l := &lines[i]
		given := l.Delivery != nil && *l.Delivery == steer.DeliveryDelivered
		withheld := l.SuppressedBy != nil && *l.SuppressedBy == steer.SuppressedByArmOff && l.Level == steer.LevelNudge.String()
		if given || withheld {
			prompt := ""
			if l.PromptID != nil {
				prompt = *l.PromptID
			}
			if perPrompt[prompt]++; perPrompt[prompt] > steer.NotesPerPrompt {
				return fmt.Errorf("session %s gives or withholds %d notes on prompt %q, over the router's cap of %d",
					s.SessionID, perPrompt[prompt], prompt, steer.NotesPerPrompt)
			}
		}
		if withheld {
			s.NotesWithheld++
		}
		switch {
		case given:
			s.NotesGiven++
		case l.SuppressedBy != nil:
			counts[string(*l.SuppressedBy)]++
		case l.Delivery != nil:
			counts[string(*l.Delivery)]++
		default:
			counts[string(l.State)]++
		}
	}
	s.Lines = len(lines)
	for r, n := range counts {
		s.NotesNotGiven = append(s.NotesNotGiven, Reason{Reason: r, Count: n})
	}
	slices.SortFunc(s.NotesNotGiven, func(a, b Reason) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(a.Reason, b.Reason)
	})
	return nil
}

// lastGates is the gates of the session's last checkpoint, none when it has
// none.
func lastGates(st *store.Store, session string) ([]verify.CheckpointGate, error) {
	b, ok, _, err := st.LatestRecordBytes(session, store.TypeVerify)
	if err != nil || !ok {
		return nil, err
	}
	var cp verify.Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, errors.New("the checkpoint does not decode: " + err.Error())
	}
	return cp.Gates, nil
}

// findings groups the finding patterns' lines by pattern, kind and key, in
// first-seen order. A finding is resolved when the checkpoint's gate of the
// same name (a regression's or a subtask gate's key is its gate) has a last
// green after the finding's last line.
func (s *Side) findings(lines []line, gates []verify.CheckpointGate) {
	type key struct{ pattern, kind, key string }
	at := map[key]int{}
	for i := range lines {
		l := &lines[i]
		if !slices.Contains(findingPatterns, pattern.ID(l.Pattern)) {
			continue
		}
		k := key{l.Pattern, l.Kind, l.Key}
		j, ok := at[k]
		if !ok {
			j = len(s.Findings)
			at[k] = j
			s.Findings = append(s.Findings, Finding{Pattern: l.Pattern, Key: l.Key})
		}
		f := &s.Findings[j]
		f.Lines++
		f.LastAfterSeq = max(f.LastAfterSeq, l.AfterSeq)
	}
	for i := range s.Findings {
		f := &s.Findings[i]
		for _, g := range gates {
			if g.Gate == f.Key && g.LastGreenSeq != nil && *g.LastGreenSeq > f.LastAfterSeq {
				seq := *g.LastGreenSeq
				f.ResolvedAtSeq = &seq
			}
		}
	}
}
