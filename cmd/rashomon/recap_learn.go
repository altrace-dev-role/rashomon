package main

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// promptLessons is the approved lessons of the payload's project for recap
// pending (LEARN-DELIVER1): cwd keyed as the learn verb keys it, its
// active.json read against the manifest and never written. A project with
// no ledger costs baseline.Key's walk and one Lstat (D171 LD9). The zero
// Active with no run to log a lesson under (H-107), no cwd or a read error:
// the prompt path stays silent rather than fail.
func promptLessons(root string, run *store.Run, cwd string) learn.Active {
	if run == nil || cwd == "" {
		return learn.Active{}
	}
	a, err := learn.ReadActive(root, strings.TrimSuffix(baseline.FileName(baseline.Key(cwd)), baseline.FileExt))
	if err != nil {
		return learn.Active{}
	}
	return a
}

// lessonFinding is the prompt's learned-fact finding, routed beside the
// request's preflight in preflightNote's one Route call: the first of a's
// lessons this session has not been given (lines, its steer log) that the
// prompt's paths make relevant (learn.Pick), at learned-fact's priority,
// below task-preflight's (D170 L3). A state that does not check gives the
// user one flag a session instead, under a key no lesson can hold.
func lessonFinding(a learn.Active, afterSeq int64, lines [][]byte, paths []string) (steer.Finding, bool) {
	if !a.Any() {
		return steer.Finding{}, false
	}
	given := learn.Delivered(lines)
	f := steer.Finding{Pattern: pattern.LearnedFact, Kind: "local", AfterSeq: afterSeq}
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
