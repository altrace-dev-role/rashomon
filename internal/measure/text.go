package measure

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Hypothesis is the pair's closing sentence: one session per arm cannot
// measure an effect.
const Hypothesis = "n = 1 per arm: a difference here is a hypothesis to test, not a measured effect of rashomon's notes."

// caveatText is each caveat's sentence, "" for n_is_one (its sentence is
// Hypothesis) and for a word that is no caveat. A switch, not a map: a map
// literal is built at package init, in every hook call (RES-2's init budget).
func caveatText(c string) string {
	switch c {
	case CaveatArmUnrecorded:
		return "a session wrote no steer line, so its arm is the one asked, not one read from the record"
	case CaveatTranscriptMissing:
		return "a session has no transcript under the configuration directory, so its transcript rows are blank"
	case CaveatTranscriptPartial:
		return "some transcript lines did not read, so the transcript rows undercount"
	case CaveatRecordingIncomplete:
		return "a session's record has an unreadable line, an unverified hook or a gap, so its record rows may undercount"
	case CaveatDifferentModels:
		return "the two sessions ran different models"
	}
	return ""
}

// armTitle heads a side's column, "" for an arm that is neither.
func armTitle(arm string) string {
	switch arm {
	case "off":
		return "arm off (nothing sent)"
	case "steer":
		return "arm steer (AutoSteer)"
	}
	return ""
}

// Text writes the pair as a table, one column per side, its caveats, and
// Hypothesis last.
func Text(w io.Writer, p *Pair) error {
	a, b := &p.Sides[0], &p.Sides[1]
	var out strings.Builder
	out.WriteString("rashomon measure pair: two sessions on one task, read from the record and the transcripts\n")
	tw := tabwriter.NewWriter(&out, 0, 0, 3, ' ', 0)
	row := func(label string, cell func(s *Side) string) {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", label, cell(a), cell(b))
	}
	row("", func(s *Side) string { return armTitle(s.Arm) })
	row("session", func(s *Side) string { return s.SessionID })
	row("lines in the steer log", func(s *Side) string { return strconv.Itoa(s.Lines) })
	row("notes given to the agent", notesCell)
	row("lines not given, by why", notGivenCell)
	row("findings raised", findingsCell)
	row("findings resolved by a green", resolvedCell)
	row("calls declared / executed", func(s *Side) string { return fmt.Sprintf("%d / %d", s.Calls, s.Executed) })
	row("writes", func(s *Side) string { return strconv.Itoa(s.Writes) })
	row("test runs, failed / passed", testsCell)
	row("gates at the last Stop", gatesCell)
	row("model responses (transcript)", func(s *Side) string {
		return transcriptCell(s, func(t *Transcript) string {
			return fmt.Sprintf("%d (%d by subagents)", t.Responses, t.SubagentResponses)
		})
	})
	row("tokens in / out / cache read", func(s *Side) string {
		return transcriptCell(s, func(t *Transcript) string {
			return fmt.Sprintf("%d / %d / %d", t.Tokens.Input, t.Tokens.Output, t.Tokens.CacheRead)
		})
	})
	row("wall time, first to last record", func(s *Side) string {
		return (time.Duration(s.WallMS) * time.Millisecond).Round(time.Second).String()
	})
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, c := range p.Caveats {
		if t := caveatText(c); t != "" {
			out.WriteString("  caveat: " + t + ".\n")
		}
	}
	out.WriteString("  " + Hypothesis + "\n")
	_, err := io.WriteString(w, out.String())
	return err
}

func notesCell(s *Side) string {
	if s.NotesWithheld > 0 {
		return fmt.Sprintf("%d, %d withheld by arm off", s.NotesGiven, s.NotesWithheld)
	}
	return strconv.Itoa(s.NotesGiven)
}

func notGivenCell(s *Side) string {
	if len(s.NotesNotGiven) == 0 {
		return "none"
	}
	parts := make([]string, len(s.NotesNotGiven))
	for i, r := range s.NotesNotGiven {
		parts[i] = fmt.Sprintf("%d %s", r.Count, r.Reason)
	}
	return strings.Join(parts, ", ")
}

func findingsCell(s *Side) string {
	if len(s.Findings) == 0 {
		return "0"
	}
	var patterns []string
	for _, f := range s.Findings {
		if !slices.Contains(patterns, f.Pattern) {
			patterns = append(patterns, f.Pattern)
		}
	}
	return fmt.Sprintf("%d (%s)", len(s.Findings), strings.Join(patterns, ", "))
}

func resolvedCell(s *Side) string {
	var seqs []string
	for _, f := range s.Findings {
		if f.ResolvedAtSeq != nil {
			seqs = append(seqs, "#"+strconv.FormatInt(*f.ResolvedAtSeq, 10))
		}
	}
	if len(seqs) == 0 {
		return "0"
	}
	return fmt.Sprintf("%d (%s)", len(seqs), strings.Join(seqs, ", "))
}

func testsCell(s *Side) string {
	c := fmt.Sprintf("%d / %d", s.TestRuns.Failed, s.TestRuns.Passed)
	if s.TestRuns.Unended > 0 {
		c += fmt.Sprintf(", %d unended", s.TestRuns.Unended)
	}
	return c
}

func gatesCell(s *Side) string {
	if len(s.Gates) == 0 {
		return "no Stop checkpoint"
	}
	parts := make([]string, len(s.Gates))
	for i, g := range s.Gates {
		parts[i] = g.Gate + " " + g.State
	}
	return strings.Join(parts, ", ")
}

func transcriptCell(s *Side, cell func(*Transcript) string) string {
	if s.Transcript == nil {
		return "no transcript"
	}
	return cell(s.Transcript)
}

// JSON writes the pair as one JSON object and a newline.
func JSON(w io.Writer, p *Pair) error {
	return json.NewEncoder(w).Encode(p)
}
