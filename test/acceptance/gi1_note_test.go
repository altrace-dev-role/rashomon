package acceptance

import (
	"strings"
	"testing"
)

// GI1n: a turn that edited and left the project's own test command unrun
// gives the agent one note with the session's next prompt (card
// GATE-INFER1, W5): the runner's name and the file it was read from, once
// per turn end, only where the project declares the command or the index
// has seen a test run. A Stop cannot reach the model without continuing the
// turn, so the next prompt carries it, the user seeing the same words as
// they are added.

// gi1Note runs the next prompt in dir for testSession and returns what the
// agent was told and the user's line.
func (e *env) gi1Note(dir, promptID string) (model, user string) {
	e.t.Helper()
	res, out := e.prompt(dir, "next", promptID)
	if res.exitCode != 0 {
		e.t.Fatalf("prompt: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	return out.HookSpecificOutput.AdditionalContext, out.SystemMessage
}

// TestGI1na_TheNextPromptNamesTheDeclaredCommandTheTurnLeftUnrun: a session
// in a project whose CI runs pytest edits and runs no test; its next prompt,
// which names nothing, tells the agent, word for word, and the user sees
// the same words; the prompt after it says nothing (once per turn end).
// Controls: a turn that ran pytest after its edit, and a project whose
// command is a convention (go.mod) the index has not seen run, give no
// note. Break: leave the turn end's note unwritten or the prompt unwired.
func TestGI1na_TheNextPromptNamesTheDeclaredCommandTheTurnLeftUnrun(t *testing.T) {
	const want = "rashomon record: gate 'pytest' (from '.github/workflows/ci.yml') was not run after the last edit."
	ci := map[string]string{"go.mod": "module m\n", ".github/workflows/ci.yml": gi1Workflow}
	t.Run("declared, not run", func(t *testing.T) {
		e := newSteerEnv(t)
		dir := gi1Project(t, ci)
		e.gi1Session(dir, testSession)
		model, user := e.gi1Note(dir, "prompt-2")
		if model != want || !strings.Contains(user, want) {
			t.Errorf("next prompt: agent told %q, user shown %q, want %q to both", model, user, want)
		}
		if model, _ := e.gi1Note(dir, "prompt-3"); model != "" {
			t.Errorf("the prompt after: agent told %q, want nothing: once per turn end", model)
		}
	})
	for _, c := range []struct {
		name     string
		files    map[string]string
		commands []string
	}{
		{"control: run after the edit", ci, []string{"pytest -q"}},
		{"control: a convention alone", map[string]string{"go.mod": "module m\n"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir := gi1Project(t, c.files)
			e.gi1Session(dir, testSession, c.commands...)
			if model, _ := e.gi1Note(dir, "prompt-2"); model != "" {
				t.Errorf("next prompt: agent told %q, want nothing", model)
			}
		})
	}
}

// TestGI1na2_ALearnedCommandIsNamedInTheNextSession: in a go.mod project,
// after a session whose test run taught the index, the next session's edit
// with no run gives its next prompt the note, the convention now backed by
// a run (W3'). Break: require a declaration alone.
func TestGI1na2_ALearnedCommandIsNamedInTheNextSession(t *testing.T) {
	const want = "rashomon record: gate 'go test' (from 'go.mod') was not run after the last edit."
	e := newSteerEnv(t)
	dir := gi1Project(t, map[string]string{"go.mod": "module m\n"})
	e.gi1Session(dir, "sess-gi1-one", "go test ./...")
	e.gi1Session(dir, testSession)
	if model, _ := e.gi1Note(dir, "prompt-2"); model != want {
		t.Errorf("next prompt: agent told %q, want %q", model, want)
	}
}
