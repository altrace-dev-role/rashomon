package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// noneLine is a preflight line, or a part of one, that lists nothing: the
// slot text "none" at the end of the line or before the next part.
var noneLine = regexp.MustCompile(`(?m): none(;|$)`)

// tb1Model is the model's text checked against TOKEN-BUDGET1's bar: no line
// that lists nothing, no unknowns line, no internal enum word.
func tb1Model(t *testing.T, label, model string) {
	t.Helper()
	if m := noneLine.FindString(model); m != "" {
		t.Errorf("%s: additionalContext holds a line that lists nothing (%q):\n%s", label, m, model)
	}
	for _, ln := range strings.Split(model, "\n") {
		if strings.HasPrefix(ln, "unknown:") {
			t.Errorf("%s: additionalContext holds the unknowns line %q: it is the person's, never the model's", label, ln)
		}
	}
	for _, k := range spec.EvidenceKinds {
		if strings.Contains(model, k) {
			t.Errorf("%s: additionalContext holds the enum word %q:\n%s", label, k, model)
		}
	}
}

// TestTB1_ANoteWithNothingProvenAddsNothingToTheModel (card TOKEN-BUDGET1,
// R55, FHV 01:57): a request that names a path the project does not have, in
// a project with no spec and no gate, has nothing proven to tell the agent.
// The prompt adds 0 bytes to the model; the person still sees what was not
// known. Today the whole note goes to the model: every line reads none or
// unknown, and the fix verb's evidence kind prints as its enum word.
func TestTB1_ANoteWithNothingProvenAddsNothingToTheModel(t *testing.T) {
	e := newLessonEnv(t)
	goFilesIn(t, e.cwd, "internal/a.go")
	out := e.prompt("s-tb1", "prompt-1", "fix internal/zzz.go")
	// Premise: the request is read and the person is told what was not
	// known, as before the card.
	if !strings.Contains(out.system, "no named path exists in the project") {
		t.Fatalf("premise: systemMessage %q, want the unknown that no named path exists", out.system)
	}
	if out.context != "" {
		t.Errorf("additionalContext %d bytes, want 0: nothing in it is proven\n%s", len(out.context), out.context)
	}
	tb1Model(t, "nothing proven", out.context)
}

// TestTB1b_TheModelGetsOnlyTheProvenLines: a request naming a path the
// project has sends the agent that path's line, and nothing that lists
// nothing, no unknowns line, and no enum word, for each verb class that
// carries an evidence kind; the person's systemMessage still mirrors what the
// agent was told and adds the unknowns. Controls: the path line reaches the
// model and the mirror holds it, today and after.
func TestTB1b_TheModelGetsOnlyTheProvenLines(t *testing.T) {
	e := newLessonEnv(t)
	goFilesIn(t, e.cwd, "internal/a.go")
	for _, c := range []struct{ session, prompt string }{
		{"s-tb1b-fix", "fix internal/a.go"},
		{"s-tb1b-deploy", "deploy internal/a.go"},
	} {
		out := e.prompt(c.session, "prompt-1", c.prompt)
		if !strings.Contains(out.context, "internal/a.go") {
			t.Fatalf("%s: premise: additionalContext %q, want the named path's line", c.prompt, out.context)
		}
		if !strings.Contains(out.system, out.context) {
			t.Errorf("%s: systemMessage %q does not mirror the agent's text %q", c.prompt, out.system, out.context)
		}
		if !strings.Contains(out.system, "no spec found") {
			t.Errorf("%s: systemMessage %q, want the unknowns the person is told", c.prompt, out.system)
		}
		tb1Model(t, c.prompt, out.context)
	}
}
