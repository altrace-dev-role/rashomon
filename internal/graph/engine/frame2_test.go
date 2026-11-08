package engine

import (
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
