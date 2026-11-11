package session

import (
	"fmt"
	"strings"
)

// BoundaryBrief is the continuity brief after a compaction (contracts-v2
// sections 1.2 and 1.3): one form, where each line about the person's or
// the model's words carries them when the reference resolves and says what
// it could not deliver when it does not. Every other line is project
// content and is the same in both cases.
func (s *Session) BoundaryBrief() string {
	rev := s.Rev()
	lines := []string{fmt.Sprintf("rashomon: after compaction (boundary %d; rev %s).", s.Boundary, rev)}
	if s.Goal != nil {
		lines = append(lines, s.goalLine())
	}
	if owed := s.owedGates(); len(owed) > 0 {
		cmds := make([]string, 0, len(owed))
		for _, og := range owed {
			cmds = append(cmds, og.Gate.Cmd)
		}
		lines = append(lines, "Owed: "+strings.Join(cmds, "; ")+".")
	}
	for _, r := range s.Reqs {
		lines = append(lines, s.reqLine(r, rev.String()))
	}
	for _, d := range s.Decisions {
		lines = append(lines, s.decisionLine(d))
	}
	if len(s.Touched) > 0 {
		parts := make([]string, 0, len(s.Touched))
		for _, t := range s.Touched {
			parts = append(parts, fmt.Sprintf("%s (ov%d)", t.Path, t.Seq))
		}
		lines = append(lines, "Touched: "+strings.Join(parts, ", ")+".")
	}
	return strings.Join(lines, "\n")
}

// goalLine is the task line in either form.
func (s *Session) goalLine() string {
	words, how := s.Goal.Words(s.Digest, s.Extract, s.CapBytes)
	if words != "" {
		where := s.Goal.Origin
		if s.Goal.Ref != nil && how != ReasonCopy {
			where += fmt.Sprintf(", transcript line %d, %s", s.Goal.Ref.Line, how)
		}
		return fmt.Sprintf("Task (%s): %q", where, words)
	}
	return fmt.Sprintf("Task: 1 goal stated at %s (%d bytes). Words not retained; %s. Re-read your %s or ask the user to restate it.",
		s.Goal.Origin, s.Goal.Bytes, how, s.Goal.Origin)
}

// reqLine is a requirement's line: a stated requirement in either form,
// an inferred one in its own.
func (s *Session) reqLine(r *Req, at string) string {
	holds := "Holds at " + at
	if !r.Holds {
		holds = "Fails at " + at
	}
	if r.Words == nil {
		return fmt.Sprintf("%s (%s, conf %.1f): %s. %s.", r.ID, r.Tier, r.Conf, r.Text, holds)
	}
	words, how := r.Words.Words(s.Digest, s.Extract, s.CapBytes)
	if words != "" {
		return fmt.Sprintf("%s (%s, %s): %q. %s. %s. Sufficiency not checked by the engine.", r.ID, r.Words.Origin, how, words, r.Text, holds)
	}
	return fmt.Sprintf("%s: 1 requirement stated at %s (%d bytes). Words not retained. %s. %s.", r.ID, r.Words.Origin, r.Words.Bytes, r.Text, holds)
}

// decisionLine is a held decision's line in either form.
func (s *Session) decisionLine(d Decision) string {
	words, how := d.Words.Words(s.Digest, s.Extract, s.CapBytes)
	if words != "" {
		return fmt.Sprintf("Held decision (%s, %s): %q", d.Words.Origin, how, words)
	}
	named := ""
	if len(d.Names) > 0 {
		named = " It named " + strings.Join(d.Names, " and ") + "."
	}
	return fmt.Sprintf("Held decision: 1, from %s (%d bytes). Words not retained.%s", d.Words.Origin, d.Words.Bytes, named)
}
