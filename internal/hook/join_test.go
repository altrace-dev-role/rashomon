package hook

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// joinStandIn is a finding routed as cmd's unit brief is (graphFinding,
// card m2-unitbrief-hook1 D4): task-preflight at that pattern's priority, a
// nudge with no stop form, from the graph, after the call's landed seq,
// under key. Those are the fields Route ranks and charges a finding by, so
// to the router the stand-in is the unit brief; its message is the unit
// brief's row (render NewUnitBriefBinding), as Brief.UnitSaid makes it.
func joinStandIn(t *testing.T, p *Post, key string) steer.Finding {
	t.Helper()
	line := slot.NewUnitLine(slot.UnitFact{Path: slot.RelPath("a"), Basis: slot.BasisNamed})
	m, err := render.New(pattern.TaskPreflight, render.NewUnitBriefBinding("a", "c1", []slot.GraphLine{line}))
	if err != nil {
		t.Fatal(err)
	}
	prio, _ := pattern.Priority(pattern.TaskPreflight)
	return steer.Finding{Pattern: pattern.TaskPreflight, Key: key, Priority: prio, MaxLevel: steer.LevelNudge,
		StopForm: steer.StopFormNone, Message: m, AfterSeq: p.landedSeq, Unit: "g:r#unit:go:a",
		Source: steer.Source{Kind: steer.SourceGraph, ID: "g:r#unit:go:a", Tier: "algorithmic"}}
}

// joinLine is the steer log's last line of pattern id, nil for none.
func joinLine(t *testing.T, f *stFix, id pattern.ID) *steer.Line {
	t.Helper()
	var got *steer.Line
	for _, l := range steerLines(t, f.st) {
		if l.Pattern == string(id) {
			got = &l
		}
	}
	return got
}

// joinGiven reports whether the session was given the unit brief keyed
// prefix, as cmd's unitBriefJoin asks it.
func joinGiven(t *testing.T, f *stFix, prefix string) bool {
	t.Helper()
	raw, _, err := f.st.ReadSteer(stSession)
	if err != nil {
		t.Fatal(err)
	}
	given := false
	Given(raw, pattern.TaskPreflight, prefix, true, func(string) { given = true })
	return given
}

// blastWords open every blast-radius note (render's blast template), and
// briefWords the unit brief's head as the post says it (render
// unitBriefHead, through Brief.UnitSaid).
const (
	blastWords = "names other packages with code calling into the package this call edited"
	briefWords = "noted once a session; the repository graph at rev"
)

// TestPost_UnitBriefOutranksBlast (F13b; card R4, as built): a first write
// in a package with both a joined unit brief and blast's callers delivers
// the unit brief, one note per call, and leaves blast's line superseded, not
// given; the package's next edit, with nothing joined, delivers blast.
// Witness: blast's port answered at the first write. Whether blast should
// count a delivered unit brief as given for its package is the director's
// question (R4), not built.
func TestPost_UnitBriefOutranksBlast(t *testing.T) {
	f := newST(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.cwd = dir
	env := stEnv(steer.DisclosureVersion)
	reads := 0
	env.Blast = BlastPort{
		Locate: func(root, _ string) (BlastSite, bool) {
			return BlastSite{Root: root, Package: "example.com/m/a", Log: "log-1"}, true
		},
		Callers: func(BlastSite) ([]BlastCaller, error) {
			reads++
			return []BlastCaller{{Package: "example.com/m/b", Dir: "b"}, {Package: "example.com/m/c", Dir: "c", Tests: true}}, nil
		},
	}
	edit := map[string]any{"file_path": filepath.Join(dir, "a", "a.go"), "old_string": "1", "new_string": "2"}
	p := f.call("toolu_e1", "Edit", edit, 0)
	env.Join = []steer.Finding{joinStandIn(t, p, "unitbrief:g:r#unit:go:a@0123456789ab")}
	out := string(p.Steer(env))
	if !strings.Contains(out, briefWords) || strings.Contains(out, blastWords) {
		t.Fatalf("the first write printed %q, want the unit brief alone", out)
	}
	if reads == 0 {
		t.Fatal("witness: blast's port was not read at the first write, so nothing was outranked")
	}
	if l := joinLine(t, f, pattern.TaskPreflight); l == nil || l.State != steer.LineStateDelivered {
		t.Errorf("unit brief line %+v, want delivered", l)
	}
	if l := joinLine(t, f, pattern.BlastRadius); l == nil || l.State != steer.LineStateSuperseded {
		t.Errorf("blast line %+v, want superseded", l)
	}
	env.Join = nil
	if out := string(f.call("toolu_e2", "Edit", edit, 0).Steer(env)); !strings.Contains(out, blastWords) {
		t.Errorf("the package's next edit printed %q, want blast's note", out)
	}
}

// TestPost_JoinRoutesWithinThreeNotes (PLAN 1425): a joined unit brief
// spends the prompt's note budget as any task-preflight note does. Three
// edits under one prompt deliver their units' briefs; the fourth unit's is
// suppressed for the prompt's budget, prints nothing, and is not given, so
// it is offered again at that unit's next edit.
func TestPost_JoinRoutesWithinThreeNotes(t *testing.T) {
	f := newST(t)
	env := stEnv(steer.DisclosureVersion)
	for i, u := range []string{"a", "b", "c", "d"} {
		edit := map[string]any{"file_path": filepath.Join(f.cwd, u, u+".go"), "old_string": "1", "new_string": "2"}
		p := f.call("toolu_j"+u, "Edit", edit, 0)
		env.Join = []steer.Finding{joinStandIn(t, p, "unitbrief:g:r#unit:go:"+u+"@0123456789ab")}
		out := string(p.Steer(env))
		if said := strings.Contains(out, briefWords); said != (i < 3) {
			t.Errorf("edit %d in unit %s printed %q, want a note %v", i+1, u, out, i < 3)
		}
	}
	l := joinLine(t, f, pattern.TaskPreflight)
	if l == nil || l.SuppressedBy == nil || *l.SuppressedBy != steer.SuppressedByBudgetPrompt {
		t.Fatalf("the fourth unit's line %+v, want suppressed for the prompt's budget", l)
	}
	if joinGiven(t, f, "unitbrief:g:r#unit:go:d@") {
		t.Error("the fourth unit's brief reads as given, so it would never be offered again")
	}
	if !joinGiven(t, f, "unitbrief:g:r#unit:go:a@") {
		t.Error("witness: the first unit's delivered brief does not read as given")
	}
}

// TestPost_JoinWithheldWhereNoOneReads (card D6): a joined unit brief passes
// the gate blast and the plan check pass (Post.join). Where its note could
// reach no one (steering off, a config that does not read, no consent, no
// prompt) it prints nothing and leaves no line, so no reader takes it as
// given, and the unit's next edit with the gate open delivers it. A line
// suppressed there would be a steer row for a note the session never met.
// Post.Unheard, which cmd asks before it reads the brief, reads each shut
// gate shut and the open one open: were it to read one open the join
// shuts, cmd would read for nothing; shut where the join is open, the
// brief would never be said.
func TestPost_JoinWithheldWhereNoOneReads(t *testing.T) {
	const key = "unitbrief:g:r#unit:go:a@0123456789ab"
	for _, tc := range []struct {
		name string
		shut func(t *testing.T, f *stFix, env *SteerEnv, p *Post)
	}{
		{"steering off", func(t *testing.T, f *stFix, _ *SteerEnv, _ *Post) { stcConfig(t, f.st, `{"steer_user":"off"}`) }},
		{"config unreadable", func(t *testing.T, f *stFix, _ *SteerEnv, _ *Post) { stcConfig(t, f.st, `{`) }},
		{"no consent", func(_ *testing.T, _ *stFix, env *SteerEnv, _ *Post) { *env = stEnv(0) }},
		{"no prompt", func(_ *testing.T, _ *stFix, _ *SteerEnv, p *Post) { p.promptID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newST(t)
			env := stEnv(steer.DisclosureVersion)
			edit := map[string]any{"file_path": filepath.Join(f.cwd, "a", "a.go"), "old_string": "1", "new_string": "2"}
			p := f.call("toolu_w1", "Edit", edit, 0)
			tc.shut(t, f, &env, p)
			if !p.Unheard(env) {
				t.Error("Post.Unheard reads the shut gate open")
			}
			env.Join = []steer.Finding{joinStandIn(t, p, key)}
			if out := string(p.Steer(env)); strings.Contains(out, briefWords) {
				t.Errorf("printed %q, want no unit brief", out)
			}
			if l := joinLine(t, f, pattern.TaskPreflight); l != nil {
				t.Fatalf("the withheld brief left the line %+v, want none", *l)
			}
			if err := os.Remove(filepath.Join(f.st.Root(), store.FileConfig)); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			env = stEnv(steer.DisclosureVersion)
			p = f.call("toolu_w2", "Edit", edit, 0)
			if p.Unheard(env) {
				t.Error("Post.Unheard reads the open gate shut")
			}
			env.Join = []steer.Finding{joinStandIn(t, p, key)}
			if out := string(p.Steer(env)); !strings.Contains(out, briefWords) {
				t.Errorf("the unit's next edit with the gate open printed %q, want the brief", out)
			}
		})
	}
}
