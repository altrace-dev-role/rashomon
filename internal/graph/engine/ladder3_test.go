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

// A constraint clause's words weigh nothing in which unit the target is,
// the exclusion word and its gerund included: they say what the change
// leaves alone, so a unit named for them is where the work is not
// ("without changing the log format, make the linker skip anonymous
// receivers" is the linker's, whatever declares a change, a log format
// or a without) (W-M1.33, O7).
func TestRoles_AConstraintsWordsWeighNoUnit(t *testing.T) {
	g := synth(t, []synFile{
		{"report", "report/chains.py", []synSym{fn("build_link"), fn("without_change"), fn("change_log"), fn("skip_value")}},
		{"pgraph", "pgraph/link.py", []synSym{fn("link_pairs"), fn("log_file"), {kind: "const", name: "max_anonymous_receiver_bytes"}}},
	}, nil, nil)
	r := roles(t, g, "Without changing the log format, make the linker skip anonymous receivers.")
	if !r.Found() || r.Picks[0].Path != "pgraph" {
		t.Fatalf("unit %q why %q, want pgraph: report's names hold the constraint's words and 'skip', pgraph's the predicate's", r.Unit, r.Why)
	}
}

// A prompt word spelled from a unit's name and more ("hostname" from
// host) names the unit when the unit also holds the phrase's head: "the
// hostname canonicaliser" is host's where host declares Canonical,
// however many other units declare a canonical of their own (W-M1.33,
// E5). The spelling alone names nothing: "the performance counter" is
// not perf's when perf holds no counter.
func TestRoles_ACompoundWordOpensOnAUnitNameThatHoldsTheHead(t *testing.T) {
	g := synth(t, []synFile{
		{"host", "host/host.py", []synSym{fn("canonical"), fn("canonical_name"), fn("loopback")}},
		{"shape", "shape/canon.py", []synSym{fn("canonical"), fn("canonical_digest"), fn("is_case"), fn("literal_word")}},
	}, nil, nil)
	r := roles(t, g, "Cover the hostname canonicaliser with IPv6 literal cases.")
	if !r.Found() || r.Picks[0].Path != "host" {
		t.Fatalf("unit %q why %q, want host: 'hostname' opens on host, which holds the head", r.Unit, r.Why)
	}
	g = synth(t, []synFile{
		{"perf", "perf/gate.py", []synSym{fn("case_gate")}},
		{"budget", "budget/counter.py", []synSym{fn("performance_counter")}},
	}, nil, nil)
	r = roles(t, g, "Fix the performance counter.")
	if !r.Found() || r.Picks[0].Path != "budget" {
		t.Fatalf("unit %q why %q, want budget: perf holds no counter, so the spelling names nothing", r.Unit, r.Why)
	}
}
