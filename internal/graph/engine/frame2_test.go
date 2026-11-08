package engine

import "testing"

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
