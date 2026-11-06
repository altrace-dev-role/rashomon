package engine

import (
	"strings"
	"testing"
)

const t1Prompt = "Add blast radius to the platform read. For every Go symbol an edit touches, report two numbers: how many callers are proven, and how many are proven or possible. Show the numbers wherever the platform read already appears. Keep the existing tests green and add tests for the new numbers."

func words(p Phrase) string { return strings.Join(p.Tokens, " ") }

func tokensOf(s string) string { return strings.Join(uniq(Tokens(s)), " ") }

// The T1 prompt names the platform read as the place of the change in two
// sentences ("to the platform read", "wherever the platform read already
// appears"); blast radius is what it adds; the existing tests are kept.
func TestFrame_T1NamesThePlatformReadTwiceAsTheTarget(t *testing.T) {
	f := Parse(t1Prompt)
	if len(f.Targets) == 0 {
		t.Fatalf("no targets: %+v", f)
	}
	got := f.Targets[0]
	if words(got) != tokensOf("platform read") || got.Count != 2 || !got.Definite {
		t.Fatalf("target %s = %+v, want [platform read] definite x2; frame %+v", got, got, f)
	}
	if !hasPhrase(f.Additions, tokensOf("blast radius")) {
		t.Errorf("additions %v lack blast radius", f.Additions)
	}
	if !hasPhrase(f.Constraints, tokensOf("the existing tests green")) {
		t.Errorf("constraints %v lack the existing tests", f.Constraints)
	}
}

// One clause shape per row of the AC-M1.1 set and the specimen task: the
// first target is the phrase the unit stage resolves.
func TestFrame_ClauseShapesGiveTheFirstTarget(t *testing.T) {
	cases := []struct{ name, prompt, want string }{
		{"place clause after a comma attaches to the sentence's verb (HD9)",
			"Report which instruction files were actually loaded this session, not just the ones present in the tree, in the brief that already lists the instruction facts.",
			"brief"},
		{"and-joined show clause with a place (PG12)",
			"Record an edit mark whenever a test is removed, a skip is added or an assertion is deleted, as user-only integrity flags, and show them in the integrity report. Keep the existing integrity tests green.",
			"integrity report"},
		{"modal names its subject (ST12)",
			"When a session resumes after a compaction, the continuity briefing should name the requirements still open and the units touched so far, and keep the existing resume tests green.",
			"continuity briefing"},
		{"edit verb names its object up to the use word (specimen task A)",
			"make session expiry respect the org-level timeout override",
			"session expiry"},
		{"wherever gives the place (PL12 third sentence)",
			"Show it wherever the platform obligations already appear and add tests for the untracked case.",
			"platform obligations"},
	}
	for _, c := range cases {
		f := Parse(c.prompt)
		if len(f.Targets) == 0 || words(f.Targets[0]) != tokensOf(c.want) {
			t.Errorf("%s: targets %v, want first [%s]", c.name, f.Targets, tokensOf(c.want))
		}
	}
}

// A prompt with no verb clause and no modal has no frame: the roles stage
// falls back to the whole prompt, and Parse claims nothing.
func TestFrame_NoVerbClauseGivesNoTarget(t *testing.T) {
	f := Parse("purging the sessions")
	if len(f.Targets) != 0 || len(f.Additions) != 0 {
		t.Fatalf("frame %+v, want empty", f)
	}
	// "must never" is a keep, not an addition; the modal's subject is the target.
	f = Parse("The warning must never report zero obligations.")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("warning") || !f.Targets[0].Definite {
		t.Fatalf("targets %v, want [warn] definite", f.Targets)
	}
	if len(f.Additions) != 0 || !hasPhrase(f.Constraints, tokensOf("zero obligations")) {
		t.Fatalf("additions %v constraints %v, want the zero obligations kept", f.Additions, f.Constraints)
	}
}

func hasPhrase(ps []Phrase, want string) bool {
	for _, p := range ps {
		if words(p) == want {
			return true
		}
	}
	return false
}

// A use word (respect, honor, via, using) opens the thing the change must
// take up: the request presumes it exists, so it is the reuse's phrase,
// never the target's; after a modal the use word governs the same way,
// and a clause opening with one names nothing else.
func TestFrame_UseWordOpensTheThingToTakeUp(t *testing.T) {
	cases := []struct{ name, prompt, target, use string }{
		{"edit verb, use word inside the clause", "make session expiry respect the org-level timeout override", "session expiry", "org-level timeout override"},
		{"modal governs a use word", "the continuity briefing should honor the retry policy", "continuity briefing", "retry policy"},
		{"add with a place and using", "add caching to the resolver using the shared cache", "resolver", "shared cache"},
		{"clause opening with a use word", "Using the registry, fix the exporter.", "exporter", "registry"},
	}
	for _, c := range cases {
		f := Parse(c.prompt)
		if len(f.Targets) == 0 || words(f.Targets[0]) != tokensOf(c.target) {
			t.Errorf("%s: targets %v, want first [%s]", c.name, f.Targets, tokensOf(c.target))
		}
		if len(f.Uses) != 1 || words(f.Uses[0]) != tokensOf(c.use) || !f.Uses[0].Definite {
			t.Errorf("%s: uses %v, want one definite [%s]", c.name, f.Uses, tokensOf(c.use))
		}
	}
	if f := Parse("add tests for the new numbers"); len(f.Uses) != 0 {
		t.Errorf("no use word, uses %v", f.Uses)
	}
}
