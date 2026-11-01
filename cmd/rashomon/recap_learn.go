package main

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// readPromptProject is recap pending's one read of the payload's project
// (LEARN-DELIVER1, D204): baseline.Key's walk from cwd, once a prompt, and
// the approved lessons of the ledger it names, as the learn verb names it,
// active.json read against the manifest and never written. A project with no
// ledger costs the walk and one Lstat (D171 LD9). Nothing is read with no
// run to log a note under (H-107) or no cwd, and a read error gives no
// lessons: the prompt path stays silent rather than fail.
func readPromptProject(root string, run *store.Run, cwd string) promptProject {
	if run == nil || cwd == "" {
		return promptProject{}
	}
	proj := promptProject{key: baseline.Key(cwd)}
	if a, err := learn.ReadActive(root, strings.TrimSuffix(baseline.FileName(proj.key), baseline.FileExt)); err == nil {
		proj.lessons = a
	}
	return proj
}

// promptFindings is the prompt's notes from the request, when in is not
// nil (one that named nothing has none), proj's lessons and the session's
// steer lines as they stand before the prompt, and from nothing else: the
// request's task-preflight note, carrying as its last line the lesson
// learn.PickRide gives for the paths it names (LEARN-RIDE1, R13 D1), and,
// when no lesson rode it, a lesson beside it (lessonFinding), which ranks
// below the note and so is superseded on a prompt that carries one (D170
// L3). The given sets are sets, so the lines' order does not matter.
func promptFindings(in *intent.Intent, a learn.Active, lines [][]byte, afterSeq int64, digest func(string) string) []steer.Finding {
	given := learn.Delivered(lines)
	var found []steer.Finding
	var paths []string
	rode := false
	if in != nil {
		paths = in.Paths
		var f steer.Finding
		var ok bool
		if f, ok, rode = intent.FindingRiding(*in, intent.ReadDelivered(lines), nil, afterSeq, digest, rideFor(a, given, paths)); ok {
			found = append(found, f)
		}
	}
	if f, ok := lessonFinding(a, afterSeq, given, paths); ok && !rode {
		found = append(found, f)
	}
	return found
}

// rideFor is the lesson the request's note may carry, learn.PickRide's
// among the lessons not yet given, or nil.
func rideFor(a learn.Active, given map[string]bool, paths []string) *intent.Ride {
	ls, ok := learn.PickRide(a, func(id string) bool { return given[id] }, paths)
	if !ok {
		return nil
	}
	return &intent.Ride{ID: ls.ID, Text: ls.Text}
}

// lessonFinding is the prompt's learned-fact finding, routed beside the
// request's preflight in preflightNote's one Route call: the first of a's
// lessons this session has not been given (given, from its steer log) that
// the prompt's paths make relevant (learn.Pick), at learned-fact's priority,
// below task-preflight's (D170 L3). A state that does not check gives the
// user one flag a session instead, under a key no lesson can hold.
func lessonFinding(a learn.Active, afterSeq int64, given map[string]bool, paths []string) (steer.Finding, bool) {
	if !a.Any() {
		return steer.Finding{}, false
	}
	f := steer.Finding{Pattern: pattern.LearnedFact, Kind: "local", StopForm: steer.StopFormNone, AfterSeq: afterSeq}
	if p, ok := pattern.Priority(pattern.LearnedFact); ok {
		f.Priority = p
	}
	if a.Unchecked != "" {
		m, err := learn.UncheckedMessage(a.Unchecked)
		if err != nil || given[learn.UncheckedKey] {
			return steer.Finding{}, false
		}
		f.Key, f.MaxLevel, f.Message = learn.UncheckedKey, steer.LevelFlag, m
		return f, true
	}
	ls, m, ok := learn.Pick(a, func(id string) bool { return given[id] }, paths)
	if !ok {
		return steer.Finding{}, false
	}
	f.Key, f.MaxLevel, f.Message = ls.ID, steer.LevelNudge, m
	return f, true
}
