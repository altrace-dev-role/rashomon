package acceptance

import (
	"strings"
	"testing"
)

// TestGI2na_TheNextPromptNamesAMultiWordRunnerTheTurnLeftUnrun: a session
// in a project whose CI runs `python3 -m pytest -q` edits and runs no test;
// its next prompt tells the agent the runner's row and the file, word for
// word, and the user sees the same words, where the gate slot's one or two
// words gave no note (card GATE-INFER2); the prompt after it says nothing.
// Control: a turn that ran the command after its edit gives no note. Break:
// refuse a runner's row at the gate slot.
func TestGI2na_TheNextPromptNamesAMultiWordRunnerTheTurnLeftUnrun(t *testing.T) {
	const want = "rashomon record: gate 'python3 -m pytest' (from '.github/workflows/ci.yml') was not run after the last edit."
	ci := map[string]string{".github/workflows/ci.yml": strings.Replace(gi1Workflow, "run: pytest -q", "run: python3 -m pytest -q", 1)}
	if !strings.Contains(ci[".github/workflows/ci.yml"], "python3 -m pytest -q") {
		t.Fatal("the workflow does not run python3 -m pytest")
	}
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
	t.Run("control: run after the edit", func(t *testing.T) {
		e := newSteerEnv(t)
		dir := gi1Project(t, ci)
		e.gi1Session(dir, testSession, "python3 -m pytest -q")
		if model, _ := e.gi1Note(dir, "prompt-2"); model != "" {
			t.Errorf("next prompt: agent told %q, want nothing", model)
		}
	})
}
