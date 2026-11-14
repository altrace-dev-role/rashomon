package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// appendedSession is a plan session whose markers consent.Append wrote, from
// a consent at the current scope at record scope v5, with steer on; then its
// record_scope marker is set to scope, "" for absent. Absent with the content
// marker on is what a failed scope-marker write in Append leaves
// (internal/consent TestSessionContent_AFailedScopeWriteLeavesContentOnScopeAbsent);
// the content marker here is Append's own, never one written by hand.
func appendedSession(t *testing.T, scope string) *planSession {
	t.Helper()
	s := newPlanSession(t, store.RecordScopeV5)
	if err := consent.Append(s.root, consent.Record{
		V: consent.RecordVersion, ScopeVersion: disclose.ScopeVersion, RecordScope: consent.RecordScopeV5,
		SteerDisclosure: steer.DisclosureVersion, Steer: consent.SteerOn, Via: consent.ViaWatchYes, At: 1760000000001,
	}); err != nil {
		t.Fatal(err)
	}
	if !store.SessionContentMarker(s.root) {
		t.Fatal("premise: consent.Append at the current scope at v5 left no session-content marker")
	}
	var err error
	if scope == "" {
		err = store.RemoveRecordScopeMarker(s.root)
	} else {
		err = store.WriteRecordScopeMarker(s.root, scope)
	}
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// sessionLogGroups is the number of overlay groups in the session log of the
// project's index, and whether the log exists.
func (s *planSession) sessionLogGroups() (int, bool) {
	s.t.Helper()
	rid, _ := baseline.RepoID(s.project)
	b, err := os.ReadFile(graph.SessionLogPath(s.root, rid))
	if os.IsNotExist(err) {
		return 0, false
	}
	if err != nil {
		s.t.Fatal(err)
	}
	_, _, groups, err := graph.LoadSessionLogGroups(graph.NewStore(), bytes.NewReader(b))
	if err != nil {
		s.t.Fatal(err)
	}
	return len(groups), true
}

// TestPost_MarkerOnScopeMarkerAbsentKeepsNothing: with the session-content
// marker on and the record_scope marker absent, which the hook reads as v4,
// a Go edit's PreToolUse and PostToolUse keep nothing of its content: no
// session log, no unit brief printed, no task-preflight line. The control
// at v5: a log with the edit's groups, the brief, one line. Break: keep
// content on the content marker alone, or pass v5 at either writer, and the
// absent row finds a log.
func TestPost_MarkerOnScopeMarkerAbsentKeepsNothing(t *testing.T) {
	for _, tc := range []struct {
		name, scope string
		kept        bool
	}{
		{"absent", "", false},
		{"v5", store.RecordScopeV5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := appendedSession(t, tc.scope)
			s.writeBrief("c1")
			in := map[string]any{"file_path": filepath.Join(s.project, filepath.FromSlash(planHookHeld)),
				"old_string": "package x", "new_string": "package x // toolu_sk_1"}
			s.call(false, "Edit", "toolu_sk_1", in)
			out := s.call(true, "Edit", "toolu_sk_1", in)
			groups, logged := s.sessionLogGroups()
			said, lines := strings.Contains(out, ubPostTitle), len(s.briefLines())
			if !tc.kept {
				if logged || said || lines != 0 {
					t.Errorf("scope marker absent: log %v (%d groups), brief printed %v, %d lines; want none of them", logged, groups, said, lines)
				}
				return
			}
			if !logged || groups == 0 || !said || lines != 1 {
				t.Errorf("at v5: log %v (%d groups), brief printed %v, %d lines; want the log with groups, the brief, 1 line", logged, groups, said, lines)
			}
		})
	}
}

// TestPrompt_ContentKeptOnlyAtScopeV5: the prompt entry keeps a session's
// content, the unit briefs and the conflict base, only where the record
// scope is v5, as the hook does. With the session-content marker on (Append's
// own), a record_scope marker that is absent or names v4 gets no unit brief
// and no conflict base; v5 gets both. Each writer is read on a fixture that
// exercises it, in its own session directory of the one store: the unit
// briefs on TestGraphBrief_StoresEachUnitsBriefWhenConsented's project and
// prompt (no spec, so no base), the base on the conflicts fixture (whose
// prompt stores no unit brief). Break: gate either writer on the content
// marker alone, or pass v5 to it, and the absent and v4 rows find its file.
func TestPrompt_ContentKeptOnlyAtScopeV5(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	cwd := filepath.Join(dir, "services", "auth")
	const prompt = "make session expiry in sessions.py respect the org-level timeout override."
	for _, tc := range []struct {
		name, scope string
		kept        bool
	}{
		{"absent", "", false},
		{"v4", store.RecordScopeV4, false},
		{"v5", store.RecordScopeV5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := appendedSession(t, tc.scope)
			s.project = cfProject(t)
			cfBindSpec(t, s.root, s.project)
			s.prompt("fix " + cfGraph)
			_, err := os.Stat(s.basePath())
			base := err == nil
			if _, ok := graphBrief(s.root, cwd, "s1", "p1", []byte(prompt)); !ok {
				t.Fatal("no brief")
			}
			ents, _ := os.ReadDir(graph.UnitBriefDir(s.root, "s1"))
			briefs := len(ents)
			if tc.kept != base || tc.kept != (briefs > 0) {
				t.Errorf("scope %q: conflict base %v, %d unit briefs; want both %v", tc.scope, base, briefs, tc.kept)
			}
		})
	}
}
