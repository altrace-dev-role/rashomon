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

// A prompt with no verb clause and no modal has its first clause's
// subject as the target (T1c, W-M1.14): a fragment is its own subject, a
// symptom's subject ends at its verb, and the words after the verb are
// the symptom, not the thing.
func TestFrame_NoVerbClauseGivesTheSubject(t *testing.T) {
	f := Parse("purging the sessions")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("purging the sessions") || len(f.Additions) != 0 {
		t.Fatalf("frame %+v, want the fragment as the subject", f)
	}
	f = Parse("The intent preflight is over budget on long prompts; cut the identifier scan's allocations.")
	if len(f.Targets) != 2 || words(f.Targets[0]) != tokensOf("identifier scan") || !f.Targets[0].Owner || words(f.Targets[1]) != tokensOf("allocations") {
		t.Fatalf("targets %v, want the verb clause's object alone", f.Targets)
	}
	f = Parse("The panic barrier records the panic value after exiting.")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("panic barrier") {
		t.Fatalf("targets %v, want the subject cut at its verb", f.Targets)
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
// A phrase's last word in an adverb's form, after another word, is
// marked for the roles stage ("resolve the prompt root lazily": the
// manner of resolving, unless a name holds it); a one-word phrase and a
// stop word in that form ("only") are not.
func TestFrame_ATrailingAdverbIsMarked(t *testing.T) {
	f := Parse("resolve the prompt root lazily")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("prompt root lazily") || f.Targets[0].Adverb != Tokens("lazily")[0] {
		t.Fatalf("targets %+v, want the whole phrase with its adverb marked", f.Targets)
	}
	f = Parse("fix the message assembly")
	if len(f.Targets) != 1 || f.Targets[0].Adverb != Tokens("assembly")[0] {
		t.Fatalf("targets %+v, want assembly marked lexically; the index decides", f.Targets)
	}
	f = Parse("run the weekly")
	if len(f.Targets) != 1 || f.Targets[0].Adverb != "" {
		t.Fatalf("targets %+v, want a one-word phrase unmarked", f.Targets)
	}
	f = Parse("fix the store only")
	if len(f.Targets) != 1 || f.Targets[0].Adverb != "" || words(f.Targets[0]) != tokensOf("store") {
		t.Fatalf("targets %+v, want a stop word in the form unmarked", f.Targets)
	}
}

// A symptom's subject ends at a third-person verb of a known class
// ("the budget store drops rows"); a symptom sentence names the target
// when the verb sentences name nothing ("Note: ... Fix it."); a label
// sentence ("Part 2", "Phase B") is not a subject; a verb sentence's
// target stands alone over a symptom.
func TestFrame_SymptomSubjectsAndLabels(t *testing.T) {
	for _, c := range []struct{ prompt, want string }{
		{"The budget store drops rows.", "budget store"},
		{"Note: the budget store drops rows. Fix it.", "budget store"},
		{"The digest prints twice. Fix the recap store.", "recap store"},
		{"Phase B: the recap line prints twice.", "recap line"},
	} {
		f := Parse(c.prompt)
		if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf(c.want) {
			t.Errorf("%q: targets %+v, want [%s]", c.prompt, f.Targets, c.want)
		}
	}
	f := Parse("Part 2: recording state as a file, not a switch.")
	for _, p := range f.Targets {
		if words(p) == "part" {
			t.Fatalf("targets %+v, want no label as a subject", f.Targets)
		}
	}
	for _, s := range []string{"Part 2", "Phase B:", "Step iv", "Round 12."} {
		if !isLabel(s) {
			t.Errorf("%q, want a label", s)
		}
	}
	for _, s := range []string{"schema lint", "the store", "Part", "V2 call", "run tests"} {
		if isLabel(s) {
			t.Errorf("%q, want no label", s)
		}
	}
}

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
