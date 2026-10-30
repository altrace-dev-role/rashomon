package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/history"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The summary of one replayed session: what its records say, read through
// verify as a Stop reads them (prescan_import.go says what a run that "matches
// recorded inputs" is).

// callObs is what the replay saw of one call.
type callObs struct {
	preFed, postFed bool
	postGrade       history.Grade
	postTainted     bool
	// assumed: the execution was fed on an assumption the transcript does not
	// show (replay.Event.Assumed), so the run does not match recorded inputs
	// without it.
	assumed bool
	postAt  time.Time
	file    int
}

// matches reports whether the call's run matches recorded inputs: its
// declaration and execution were both fed, the call joined its result by id
// with every field known (history.Verified), and no gap lies at or before the
// execution.
func (o *callObs) matches() bool {
	return o != nil && o.preFed && o.postFed && o.postGrade == history.Verified && !o.postTainted && !o.assumed
}

// summarise reads a replayed session's records through verify into the
// session's summary.
func summarise(run *store.Run, sessionID string, res *replayResult, ms replay.MergeSummary, calls map[string]*callObs, now time.Time) *sessionSummary {
	s := &sessionSummary{Families: map[string]*familySummary{}, Basis: replayBasis, Flags: append([]string(nil), replayFlags...),
		Complete: ms.Complete, Skipped: map[string]int{}, Fed: res.Fed, TaintFrom: ms.TaintFrom, TaintReason: ms.TaintReason, Agents: ms.Agents}
	for _, k := range res.Skipped {
		s.Skipped[string(k.Reasons[0])]++
	}
	s.PartialReasons = partialReasons(ms)
	s.Partial = len(s.PartialReasons) > 0
	if run != nil {
		summariseRuns(s, run, sessionID, calls, now)
	}
	return s
}

// partialReasons are the ways a merged session's evidence is incomplete other
// than at one run: each makes the whole session Partial, with its name.
func partialReasons(ms replay.MergeSummary) []string {
	reasons := map[string]bool{}
	if ms.CapHit {
		reasons[string(replay.SessionIDCap)] = true
	}
	if ms.FilesDropped > 0 {
		reasons["session_file_cap"] = true
	}
	if ms.Conflicts > 0 {
		reasons[string(replay.CopiesDisagree)] = true
	}
	if ms.OrderGaps > 0 {
		reasons["order_not_established"] = true
	}
	if ms.Unreadable > 0 {
		reasons["unreadable_lines"] = true
	}
	for _, a := range ms.Agents {
		if a.Status != replay.LinkOK {
			reasons["subagent_"+string(a.Status)] = true
		}
	}
	var out []string
	for r := range reasons {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// summariseRuns reads the replayed run through verify: the gate runs as the
// state table classifies them, in seq order, each joined to what the replay saw
// of its call, and the state of each family at the session's end.
func summariseRuns(s *sessionSummary, run *store.Run, sessionID string, calls map[string]*callObs, now time.Time) {
	projects := map[string]string{}
	for i := range run.Declarations {
		if d := &run.Declarations[i]; d.Project != nil {
			projects[d.ToolUseID] = *d.Project
		}
	}
	snap := verify.Evaluate(run, nil, nil, verify.Point{Kind: verify.PointQuery, Seq: run.LastSeq(), SessionID: sessionID}, now)
	chains := runChains{}
	assumed := false
	for _, r := range verify.GateRuns(run) {
		if r.Family == "" {
			continue // no known runner or place: it credits no family
		}
		f := s.Families[r.Family]
		if f == nil {
			f = &familySummary{Kind: r.Kind}
			s.Families[r.Family] = f
		}
		o := calls[r.ToolUseID]
		if f.Project == "" {
			f.Project = projects[r.ToolUseID]
		}
		f.seen(o, s.Complete)
		assumed = assumed || o != nil && o.assumed
		if !o.matches() {
			chains[r.Family] = nil
			continue
		}
		f.tally(r.Class, o.postAt)
		chains.add(f, r.Family, r.Class, o.file)
	}
	if assumed {
		s.Flags = append(s.Flags, flagSubagentResponseAssumed)
	}
	for _, g := range snap.Gates {
		if f := s.Families[g.Family]; f != nil {
			f.LastObserved.State = string(g.State)
			f.Kind = g.Kind
		}
	}
}

// seen counts a run of any grade and notes it as the family's latest: a run
// that does not match recorded inputs is still the last result the transcript
// shows.
func (f *familySummary) seen(o *callObs, complete bool) {
	f.RunsAny++
	f.SessionsAny = 1
	lo := lastObserved{Verified: o.matches(), SessionComplete: complete}
	if o != nil {
		lo.At = o.postAt
		if o.assumed {
			f.RunsAssumed++
		}
	}
	f.LastObserved = lo
}

// tally counts a run that matches recorded inputs, by class, and the times of
// the first and the last.
func (f *familySummary) tally(class verify.RunClass, at time.Time) {
	f.RunsVerified++
	f.SessionsVerified = 1
	switch class {
	case verify.RunGreen:
		f.Greens++
	case verify.RunRed:
		f.Reds++
	case verify.RunTimeout:
		f.Timeouts++
	case verify.RunNotRun:
		f.NotRun++
	default:
		f.Other++
	}
	if f.FirstSeen.IsZero() || at.Before(f.FirstSeen) {
		f.FirstSeen = at
	}
	if at.After(f.LastSeen) {
		f.LastSeen = at
	}
}

// runStep is one collapsed state of a family's chain of runs, and the file the
// latest run in it came from.
type runStep struct {
	state string
	file  int
}

// runChains are the chains of runs that match recorded inputs, by family,
// collapsed (green green red is a green and a red) and kept to the last three
// states, which is all a transition needs. A run that is neither green nor red
// breaks its family's chain.
type runChains map[string][]runStep

// add extends the family's chain with a run and counts the transition it
// makes. A transition between runs of two files is partial: the order of runs
// across files is the files' and not a seq the recorder held.
func (c runChains) add(f *familySummary, fam string, class verify.RunClass, file int) {
	var state string
	switch class {
	case verify.RunGreen:
		state = "green"
	case verify.RunRed:
		state = "red"
	default:
		c[fam] = nil
		return
	}
	ch := c[fam]
	if n := len(ch); n > 0 && ch[n-1].state == state {
		ch[n-1].file = file
		return
	}
	ch = append(ch, runStep{state, file})
	if len(ch) > 3 {
		ch = ch[1:]
	}
	c[fam] = ch
	n := len(ch)
	if n < 2 {
		return
	}
	prev, cur := ch[n-2], ch[n-1]
	span := prev.file != cur.file
	switch {
	case span:
		f.PartialTransitions++
	case prev.state == "green":
		f.Transitions.GreenToRed++
	default:
		f.Transitions.RedToGreen++
	}
	if n == 3 && ch[0].state == "green" && !span && ch[0].file == prev.file {
		f.Transitions.GreenRedGreen++
	}
}

// describe is a summary as one line, for a test's failure message.
func (s *sessionSummary) describe() string {
	var fams []string
	for k, f := range s.Families {
		fams = append(fams, fmt.Sprintf("%s: verified sessions %d runs %d/%d", k[:min(len(k), 12)], f.SessionsVerified, f.RunsVerified, f.RunsAny))
	}
	sort.Strings(fams)
	return strings.Join(fams, "; ")
}
