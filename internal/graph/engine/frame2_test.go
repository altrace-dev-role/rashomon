package engine

import (
	"slices"
	"strings"
	"testing"
)

// A question whose question word stands alone as the subject ("what
// decides whether a lesson is proven", "who calls the installer") asks
// about the verb's complement: the clause after whether, if or that is
// the thing whole, less its auxiliary (W-M1.32, H7). A question with a
// subject of its own keeps it ("where is the deadline enforced").
func TestFrame_AQuestionWordAsTheSubjectAsksAboutTheComplement(t *testing.T) {
	f := Parse("What decides whether a lesson is proven?")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("lesson proven") {
		t.Fatalf("targets %v, want [lesson proven]: the complement clause whole", f.Targets)
	}
	f = Parse("Who calls the installer?")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("installer") {
		t.Fatalf("targets %v, want [installer]: the verb's object", f.Targets)
	}
	f = Parse("Where is the five-second hook deadline enforced?")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("five-second hook deadline enforced") {
		t.Fatalf("targets %v, want the question's own subject kept", f.Targets)
	}
	f = Parse("What does the harness decide?")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("harness") {
		t.Fatalf("targets %v, want [harness]: after an auxiliary the subject follows", f.Targets)
	}
}

// An exclusion word ("without", "except", "excluding") opens what the
// change must leave alone, at the sentence's head or after the object,
// less the gerund that carries it: "without changing the log format"
// leaves the log format alone. The words are a constraint, never the
// target's (W-M1.33, O7). A gerund with nothing after it is the thing
// itself ("without logging").
func TestFrame_AnExclusionPhraseIsAConstraint(t *testing.T) {
	for _, tc := range []struct{ prompt, target, constraint, aside string }{
		{"Without changing the log format, make the linker skip anonymous receivers.", "linker", "log format", "without changing"},
		{"Make the linker skip anonymous receivers without changing the log format.", "linker", "log format", "without changing"},
		{"Without logging, fix the digest writer.", "digest writer", "logging", "without"},
	} {
		f := Parse(tc.prompt)
		if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf(tc.target) {
			t.Fatalf("%q: targets %v, want [%s]", tc.prompt, f.Targets, tc.target)
		}
		if len(f.Constraints) != 1 || words(f.Constraints[0]) != tokensOf(tc.constraint) {
			t.Fatalf("%q: constraints %v, want [%s]", tc.prompt, f.Constraints, tc.constraint)
		}
		if got := strings.Join(f.Aside, " "); got != tokensOf(tc.aside) {
			t.Fatalf("%q: aside %q, want %q", tc.prompt, got, tokensOf(tc.aside))
		}
	}
}

// A negated verb clause names what the change leaves alone ("do not
// embed report.MaskedRuns", "it must not touch the store"): an
// exclusion, as a "without" clause is, where a kept thing ("keep the
// existing store tests green") is an invariant on the work (W-M1.35).
func TestFrame_ANegatedVerbClauseIsAnExclusion(t *testing.T) {
	for _, prompt := range []string{
		"project masked_runs, do not embed report.MaskedRuns",
		"Fix the duplicate id check, it should not change the store.",
	} {
		f := Parse(prompt)
		if len(f.Constraints) != 1 || !f.Constraints[0].Excluded {
			t.Errorf("%q: constraints %+v, want one, excluded", prompt, f.Constraints)
		}
	}
	f := Parse("Fix the duplicate id check and keep the existing store tests green.")
	if len(f.Constraints) != 1 || f.Constraints[0].Excluded {
		t.Fatalf("constraints %+v, want one kept thing, not excluded", f.Constraints)
	}
}

// A negated keep-class verb means the opposite of a keep: "do not keep the
// old lock file" and "never leave the lock file behind" are work on the
// lock file. The phrase is a kept thing as before, never an exclusion, so
// it bars nothing (W-M1.35 amend, gate finding 1).
func TestFrame_ANegatedKeepVerbIsNotAnExclusion(t *testing.T) {
	for _, prompt := range []string{
		"Fix the cleanup, do not keep the old lock file.",
		"Fix the cleanup, it should not leave the lock file behind.",
		"Fix the cleanup and never leave the lock file behind.",
		"Do not keep the deprecated flag in the settings loader.",
	} {
		f := Parse(prompt)
		if len(f.Constraints) != 1 || f.Constraints[0].Excluded {
			t.Errorf("%q: constraints %+v, want one kept thing, not excluded", prompt, f.Constraints)
		}
	}
}

// The negated verb at a clause head is a verb, as it is after a modal: it
// leaves the excluded thing's tokens and joins the frame's Verbs, so a unit
// named by the verb's stem is not barred ("do not update the schema" bars
// no update package) (W-M1.35 amend, gate finding 2).
func TestFrame_ANegatedHeadVerbIsAVerb(t *testing.T) {
	for _, tc := range []struct{ prompt, verb string }{
		{"do not touch the store", "touch"},
		{"Do not change the store", "change"},
		{"never touch the store", "touch"},
	} {
		f := Parse(tc.prompt)
		if len(f.Constraints) != 1 || !f.Constraints[0].Excluded || strings.Join(f.Constraints[0].Tokens, " ") != "stor" {
			t.Errorf("%q: constraints %+v, want one excluded [stor]", tc.prompt, f.Constraints)
		}
		if want := Tokens(tc.verb)[0]; !slices.Contains(f.Verbs, want) {
			t.Errorf("%q: verbs %v, want %q", tc.prompt, f.Verbs, want)
		}
	}
}
