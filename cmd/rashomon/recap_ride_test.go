package main

import (
	"encoding/json"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// rideStep is one prompt of rideSession: its text, the session's steer
// lines before it and the lines it added.
type rideStep struct {
	text          string
	before, added [][]byte
}

// rideSession runs five prompts in one session, with three lessons
// approved: a-gen and b-tag name a path, log names none. a-gen rides
// prompt-1's note; b-tag goes with prompt-2, which carries no note; log
// waits behind the notes of prompt-3 and prompt-4, whose paths name no
// lesson not yet given, and goes with prompt-5. It gives each step, the
// ledger's active lessons and the digest the prompt path keys paths with.
func rideSession(t *testing.T, sessionID string) ([]rideStep, learn.Active, func(string) string, *lessonEnv) {
	t.Helper()
	e := newLessonEnv(t)
	goFilesIn(t, e.cwd, "internal/a.go", "internal/b.go", "internal/c.go")
	e.ledger([][2]string{{"a-gen", aGenLesson}, {"b-tag", "internal/b.go keeps its build tag"},
		{"log", "keep the changelog current"}}, "a-gen", "b-tag", "log")
	e.start(sessionID)
	st, err := store.Open(e.root)
	if err != nil {
		t.Fatal(err)
	}
	var digest func(string) string
	if hook.PromptRecordScope(st) != store.RecordScopeV5 {
		key := st.Key()
		digest = func(path string) string { return intent.PathDigest(key, path) }
	}
	a, err := learn.ReadActive(e.root, e.project)
	if err != nil || len(a.Lessons) != 3 {
		t.Fatalf("premise: active lessons %v, %v; want three", a.Lessons, err)
	}
	var steps []rideStep
	for i, text := range []string{"fix internal/a.go", "continue please", "fix internal/a.go and internal/c.go",
		"now internal/b.go and internal/a.go", "continue please"} {
		before, _, _ := st.ReadSteer(sessionID)
		e.prompt(sessionID, "prompt-"+string(rune('1'+i)), text)
		after, _, _ := st.ReadSteer(sessionID)
		steps = append(steps, rideStep{text: text, before: before, added: after[len(before):]})
	}
	return steps, a, digest, e
}

// intentOf is the request as preflightNote resolves it in e's project,
// which holds no spec and no index, or nil when it names nothing.
func intentOf(e *lessonEnv, text string) *intent.Intent {
	if !intent.Names([]byte(text), false) {
		return nil
	}
	root := promptRoot(nil, e.cwd, baseline.Key(e.cwd))
	cwd := e.cwd
	if cwd != root {
		cwd = resolvedDir(cwd)
	}
	in := intent.Resolve([]byte(text), false, intent.Project{Root: root, Cwd: cwd})
	return &in
}

// decided is each finding as "pattern key lesson", the lesson its lesson
// slot names ("" with none), sorted.
func decided(found []steer.Finding) []string {
	var out []string
	for _, f := range found {
		lesson := ""
		for _, s := range f.Message.Slots() {
			if s.Name == "lesson" {
				lesson = s.Value.Text()
			}
		}
		out = append(out, string(f.Pattern)+" "+f.Key+" "+lesson)
	}
	slices.Sort(out)
	return out
}

// recorded is decided for steer lines: each task-preflight and learned-fact
// line as "pattern key lesson", sorted.
func recorded(t *testing.T, lines [][]byte) []string {
	t.Helper()
	var out []string
	for _, b := range lines {
		var l struct {
			Pattern string `json:"pattern"`
			Key     string `json:"key"`
			Slots   struct {
				Lesson string `json:"lesson"`
			} `json:"slots"`
		}
		if err := json.Unmarshal(b, &l); err != nil {
			t.Fatal(err)
		}
		if l.Pattern == "task-preflight" || l.Pattern == "learned-fact" {
			out = append(out, l.Pattern+" "+l.Key+" "+l.Slots.Lesson)
		}
	}
	slices.Sort(out)
	return out
}

// TestLRN17r_TheNotesAtAPromptAreTheOnesOverTheLinesBeforeIt (LEARN-RIDE1,
// RES-14b's form): at every prompt, the notes the hook routed, and the
// lesson each carried, are promptFindings' over the session's steer lines
// as they stood before that prompt. The ride is decided from the record
// alone, so the record explains every lesson given.
func TestLRN17r_TheNotesAtAPromptAreTheOnesOverTheLinesBeforeIt(t *testing.T) {
	steps, a, digest, e := rideSession(t, "s-lrn17r-prefix")
	want := [][]string{
		{"task-preflight note a-gen"},
		{"learned-fact b-tag b-tag"},
		{"learned-fact log log", "task-preflight note "},
		{"learned-fact log log", "task-preflight note "},
		{"learned-fact log log"},
	}
	for i, s := range steps {
		got := decided(promptFindings(intentOf(e, s.text), a, s.before, 0, digest))
		if rec := recorded(t, s.added); !slices.Equal(got, rec) || !slices.Equal(got, want[i]) {
			t.Errorf("prompt-%d %q: over the lines before it %q, the hook recorded %q, want %q", i+1, s.text, got, rec, want[i])
		}
	}
}

// TestLRN17r_TheNotesReadTheLinesAsASet (LEARN-RIDE1, VER-44's form): the
// prompt's notes are the same bytes in whatever order its steer lines are.
func TestLRN17r_TheNotesReadTheLinesAsASet(t *testing.T) {
	steps, a, digest, e := rideSession(t, "s-lrn17r-set")
	s := steps[3]
	encode := func(lines [][]byte) string {
		var b strings.Builder
		for _, f := range promptFindings(intentOf(e, s.text), a, lines, 0, digest) {
			b.WriteString(string(f.Pattern) + "|" + f.Key + "|" + f.Message.Text() + "|" + f.Record.Text() + "\n")
		}
		return b.String()
	}
	want := encode(s.before)
	if strings.Count(want, "|") != 6 || !strings.Contains(want, "; 1 given earlier") {
		t.Fatalf("premise: prompt-4 over its lines gives a note that counts a path given earlier and a lesson:\n%s", want)
	}
	lines := slices.Clone(s.before)
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(lines), func(i, j int) { lines[i], lines[j] = lines[j], lines[i] })
		if got := encode(lines); got != want {
			t.Fatalf("shuffle %d:\n%s\nwant\n%s", i, got, want)
		}
	}
}
