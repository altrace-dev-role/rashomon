package engine

import "testing"

// Two units whose callables cover the phrase's one token are settled by
// the rest of the prompt their names hold, and every name counts (W-M1.32,
// O7): a constant named for the prompt's rarest words ("make the linker
// skip anonymous receivers" beside maxAnonReceiverBytes) says what the
// unit is about as surely as a function does; which name is edited is
// the claim's question, not the tie's.
func TestUnits_MassCountsEveryNameAUnitHolds(t *testing.T) {
	g := synth(t, []synFile{
		{"report", "report/chains.py", []synSym{fn("run_linker"), fn("skip_chain")}},
		{"pgraph", "pgraph/identity.py", []synSym{fn("run_linker"), {kind: "const", name: "max_anonymous_receiver_bytes"}}},
	}, nil, nil)
	picks, _ := units(t, g, "make the linker skip anonymous receivers")
	if len(picks) != 2 || picks[0].Refused != "" || picks[0].Path != "pgraph" {
		t.Fatalf("picks %+v, want pgraph first: its constant names two of the prompt's words, report's callable one", picks)
	}
}

// A phrase that opens with a pronoun and goes on with words no index
// name holds names nothing: the pronoun stands for a thing another clause
// names, and the words after it say what the thing should be ("make it
// faster"), not what it is; reading "faster" through a FastMode is a
// guess, so no unit is claimed (W-M1.32, K5). A determiner after the
// pronoun opens a thing of its own ("rename it to the canonicaliser").
func TestRoles_APronounPhraseOfUnknownWordsNamesNothing(t *testing.T) {
	g := synth(t, []synFile{
		{"spend", "spend/mode.py", []synSym{fn("fast_mode"), fn("fast_mode_pricing")}},
		{"host", "host/host.py", []synSym{fn("canonical_name")}},
	}, nil, nil)
	r := roles(t, g, "make it faster")
	if r.Found() {
		t.Fatalf("unit %q why %q, want none: 'it' names a thing no clause names and 'faster' is no name", r.Unit, r.Why)
	}
	r = roles(t, g, "rename it to the canonicaliser")
	if !r.Found() || r.Picks[0].Path != "host" {
		t.Fatalf("unit %q why %q, want host: the determiner opens a thing of its own", r.Unit, r.Why)
	}
}
