package engine

import (
	"strings"
	"testing"
)

// The AC-M1.5 development-half unit misses (W-M1.31), one shape each:
// what the prompt names outranks what a heuristic guesses, a record the
// prompt named outranks a partial coincidence, a benchmark clause reads
// as a test clause, and a phrase naming two units stays between them.

// A declared identifier outranks a test name's guess at its unit (P9
// shape: TestFold_TooManyHosts guessed fold by its first word, while
// ErrTooManyHosts is declared in baseline). Both are in the paste; the
// one the index declares wins, whatever order the paste puts them in.
func TestNames_ADeclaredIdentifierOutranksATestNamesGuess(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "--- FAIL: TestJobs_SessionTTL (0.00s)\n    lock_cap_test.py:77: ttl = 0, want session_ttl")
	if r.Why != ClassNamed || r.Path != "sessions" {
		t.Fatalf("why %q path %q, want sessions by the declared session_ttl over the test name's jobs", r.Why, r.Path)
	}
}

// A record the prompt named by id, whose evidence lies in one unit,
// outranks a partial coverage claim on a generic phrase (N7 shape: "B374
// again; fix the root cause this time" is B374's unit, not the unit that
// happens to declare a Cause). A claim that covers the phrase whole still
// wins: the phrase then names the thing itself.
func TestNames_AKnowledgeIDOutranksAPartialClaim(t *testing.T) {
	g := synth(t, []synFile{
		{"jobs", "jobs/purge_sessions.py", []synSym{fn("purge_sessions"), fn("cause_tags")}},
		{"fs", "fs/paths.py", []synSym{fn("root_dir"), fn("root_path")}},
		{"orgs", "orgs/settings.py", []synSym{fn("get_org_settings"), fn("org_timeout")}},
	}, nil, nil)
	record(t, g, "les:B1", "lesson", "orgs/settings.py")
	r := roles(t, g, "B1 again; fix the root cause this time.")
	if r.Path != "orgs" || !strings.Contains(r.Why, "B1") {
		t.Fatalf("why %q path %q, want orgs through B1's evidence over a partial claim on 'root cause'", r.Why, r.Path)
	}
	whole := synth(t, []synFile{
		{"jobs", "jobs/purge_sessions.py", []synSym{fn("purge_sessions"), fn("root_cause")}},
		{"orgs", "orgs/settings.py", []synSym{fn("get_org_settings"), fn("org_timeout")}},
	}, nil, nil)
	record(t, whole, "les:B1", "lesson", "orgs/settings.py")
	if r := roles(t, whole, "B1 again; fix the root cause this time."); r.Path != "jobs" {
		t.Fatalf("why %q path %q, want jobs: root_cause covers the phrase whole", r.Why, r.Path)
	}
}

// A benchmark clause names the thing benchmarked, as a test clause names
// the thing under test (E9 shape): "add a benchmark for the line scanner
// on one-megabyte lines" is about the line scanner, not about lines.
func TestFrame_ABenchmarkClauseNamesTheThingBenchmarked(t *testing.T) {
	f := Parse("Add a benchmark for the line scanner on one-megabyte lines.")
	if len(f.Targets) == 0 || words(f.Targets[0]) != tokensOf("line scanner") {
		t.Fatalf("targets %v, want [line scanner] first; frame %+v", f.Targets, f)
	}
}

// A phrase that names two units by their own names is one of theirs,
// never a third unit's whose name merely holds another word of it (K9
// shape: "the spend report" is spend's or report's, not seed's for a
// failedAgain). Between the two, the modifier claims and the head is the
// kind of thing, as a lone unit name claims as a modifier.
func TestClaim_TwoNamedUnitsKeepThePhraseBetweenThem(t *testing.T) {
	g := synth(t, []synFile{
		{"spend", "spend/spend.py", []synSym{fn("model_spend"), fn("session_spend")}},
		{"report", "report/report.py", []synSym{fn("build_report")}},
		{"seed", "seed/class.py", []synSym{fn("failed_again")}},
	}, nil, nil)
	r := roles(t, g, "same again for the spend report")
	if r.Path != "spend" || r.Basis != BasisUnitName {
		t.Fatalf("why %q path %q basis %q, want spend by its name over seed's failed_again", r.Why, r.Path, r.Basis)
	}
}
