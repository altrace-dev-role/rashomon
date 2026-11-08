package engine

import (
	"strings"
	"testing"
)

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

// The whole-prompt fallback claims a unit by the things the prompt names,
// never by its verbs: "without changing the spec file format, report the
// line of a duplicate id" names spec, and 'report' is what the prompt
// does, whatever unit is called report (W-M1.34, O3).
func TestRoles_AFallbackClaimSkipsTheVerbs(t *testing.T) {
	g := synth(t, []synFile{
		{"spec", "spec/parse.py", []synSym{fn("parse_spec"), {kind: "const", name: "reason_duplicate_id"}}},
		{"report", "report/text.py", []synSym{fn("has_duplicate_tool_id"), fn("changes_text")}},
	}, nil, nil)
	r := roles(t, g, "Without changing the spec file format, report the line of a duplicate id.")
	if len(r.Picks) == 0 || r.Picks[0].Refused != "" || r.Picks[0].Path != "spec" {
		t.Fatalf("picks %d why %q, want spec: 'report' is the verb, not a thing the prompt names", len(r.Picks), r.Why)
	}
}

// A constraint that names a thing rather than a property of one is the
// thing left alone, and the fallback never claims it: "without touching
// the store, report the duplicate id" names no unit and refuses naming
// the store, whatever else goes unnamed (W-M1.34, O3 counter-case).
func TestRoles_AFallbackNeverClaimsAnExcludedUnit(t *testing.T) {
	g := synth(t, []synFile{
		{"store", "store/open.py", []synSym{fn("open_store"), fn("sync_store")}},
		{"spec", "spec/parse.py", []synSym{fn("parse_spec")}},
	}, nil, nil)
	r := roles(t, g, "Without touching the store, report the duplicate id.")
	if len(r.Picks) > 0 && r.Picks[0].Refused == "" {
		t.Fatalf("unit %q why %q, want no claim: the store is what the change leaves alone", r.Picks[0].Path, r.Why)
	}
	if !strings.Contains(r.Why, "store") {
		t.Fatalf("why %q, want the refusal to name the store", r.Why)
	}
}

// A thing-constraint that names a unit bars it from the candidates on
// every path, a hard filter and not a lighter weight: the fallback never
// claims it however its names cover the rest ("without touching the
// store, report the duplicate id" with a store that declares
// find_duplicate_id), the target path never claims it by coverage
// ("without touching the store, fix the duplicate id check"), and when it
// was the only candidate the claim refuses naming it (W-M1.34, review
// finding 2).
func TestRoles_ABarredUnitIsNoCandidate(t *testing.T) {
	for _, tc := range []struct {
		name, prompt, want string
		spec               []synSym
	}{
		{"fallback by coverage", "Without touching the store, report the duplicate id.", "", []synSym{fn("parse_spec")}},
		{"target path, only candidate", "Without touching the store, fix the duplicate id check.", "", []synSym{fn("parse_spec")}},
		{"target path, another candidate", "Without touching the store, fix the duplicate id check.", "spec", []synSym{fn("parse_spec"), fn("duplicate_id_check")}},
	} {
		g := synth(t, []synFile{
			{"store", "store/open.py", []synSym{fn("open_store"), fn("find_duplicate_id"), fn("duplicate_id_check")}},
			{"spec", "spec/parse.py", tc.spec},
		}, nil, nil)
		r := roles(t, g, tc.prompt)
		if len(r.Picks) > 0 && r.Picks[0].Refused == "" && r.Picks[0].Path == "store" {
			t.Fatalf("%s: unit store why %q: the store is what the change leaves alone", tc.name, r.Why)
		}
		if tc.want == "" && !strings.Contains(r.Why, "store") {
			t.Fatalf("%s: why %q, want the refusal to name the store", tc.name, r.Why)
		}
		if tc.want != "" && (len(r.Picks) == 0 || r.Picks[0].Refused != "" || r.Picks[0].Path != tc.want) {
			t.Fatalf("%s: picks %v why %q, want %s", tc.name, r.Picks, r.Why, tc.want)
		}
	}
}

// The property's owner may follow it: "the format of the spec file"
// keeps the work spec's as "the spec file format" does (W-M1.34, review
// finding 3; the same request as O3).
func TestRoles_APropertyOwnedThroughOfKeepsItsOwner(t *testing.T) {
	g := synth(t, []synFile{
		{"spec", "spec/parse.py", []synSym{fn("parse_spec"), {kind: "const", name: "reason_duplicate_id"}}},
		{"report", "report/text.py", []synSym{fn("has_duplicate_tool_id"), fn("changes_text")}},
	}, nil, nil)
	for _, prompt := range []string{
		"Without changing the spec file format, report the line of a duplicate id.",
		"Without changing the format of the spec file, report the line of a duplicate id.",
	} {
		r := roles(t, g, prompt)
		if len(r.Picks) == 0 || r.Picks[0].Refused != "" || r.Picks[0].Path != "spec" {
			t.Fatalf("%q: picks %d why %q, want spec", prompt, len(r.Picks), r.Why)
		}
	}
}

// A kept thing is an invariant on the work, not an exclusion: "keep the
// existing store tests green" names the store as the place whose tests
// must still pass, so the store stays a candidate (W-M1.34, review
// finding 2; the T1 prompt's shape).
func TestRoles_AKeptUnitStaysACandidate(t *testing.T) {
	g := synth(t, []synFile{
		{"store", "store/open.py", []synSym{fn("open_store"), fn("duplicate_id_check")}},
		{"spec", "spec/parse.py", []synSym{fn("parse_spec")}},
	}, nil, nil)
	r := roles(t, g, "Fix the duplicate id check and keep the existing store tests green.")
	if len(r.Picks) == 0 || r.Picks[0].Refused != "" || r.Picks[0].Path != "store" {
		t.Fatalf("picks %v why %q, want store", r.Picks, r.Why)
	}
}
