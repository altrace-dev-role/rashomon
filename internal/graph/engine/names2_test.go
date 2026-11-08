package engine

import (
	"slices"
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
// happens to declare a Cause). A claim that names the thing itself still
// wins: the unit's path and one of its names covering the phrase whole.
// (Until W-M1.41 one name covering the phrase whole also won; it is word
// coverage, and TestNames_ARecordOutranksAClaimByWordCoverage pins it.)
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
	if r := roles(t, whole, "B1 again; fix the jobs root cause this time."); r.Path != "jobs" || r.Picks[0].Class != ClassQualified || r.Unnamed {
		t.Fatalf("why %q path %q unnamed %v, want jobs: its path and root_cause cover the phrase whole", r.Why, r.Path, r.Unnamed)
	}
}

// A record the prompt named outranks every claim by word coverage on a
// unit its evidence does not lie in, one name covering the whole phrase
// among them (W-M1.41; KI05 and KI20's shape): the request named the
// record, and word coverage names nothing (E5). A record that links
// several units claims the one the ladder ranks first among them (KI14's
// shape), in whatever order the prompt cites its records, and the claim's
// reason names a record whose evidence lies there; with no words to rank
// them it claims none, while two records whose evidence lies in one unit
// claim it, listed once. A claim on a unit the record's evidence lies in
// stands, and the unit rule keeps it: the knowledge clause.
func TestNames_ARecordOutranksAClaimByWordCoverage(t *testing.T) {
	g := synth(t, []synFile{
		{"xray", "xray/timer.py", []synSym{fn("session_purge_timer")}},
		{"alpha", "alpha/purge.py", []synSym{fn("session_purge")}},
		{"beta", "beta/notes.py", []synSym{fn("render_notes")}},
		{"gamma", "gamma/session.py", []synSym{fn("open_session")}},
	}, nil, nil)
	record(t, g, "dec:D1", "decision", "alpha/purge.py", "beta/notes.py")
	record(t, g, "dec:D2", "decision", "gamma/session.py")
	record(t, g, "les:B3", "lesson", "gamma/session.py")
	record(t, g, "les:B4", "lesson", "xray/timer.py")
	if r := roles(t, g, "Fix the session purge timer."); r.Path != "xray" || r.Picks[0].Class != ClassSingle || !r.Unnamed {
		t.Fatalf("no record: why %q path %q unnamed %v, want xray by one name and withheld", r.Why, r.Path, r.Unnamed)
	}
	if r := roles(t, g, "B3 again: fix the session purge timer."); r.Path != "gamma" || r.Why != "named 'les:B3' applies to gamma over a claim by coverage on xray" || r.Unnamed {
		t.Errorf("one record: why %q path %q unnamed %v, want gamma through B3 over xray's claim", r.Why, r.Path, r.Unnamed)
	}
	for _, c := range []struct {
		prompt string
		units  []string
	}{
		{"D1 and D2 again: fix the session purge timer.", []string{"alpha", "beta", "gamma"}},
		{"D2 and D1 again: fix the session purge timer.", []string{"gamma", "alpha", "beta"}},
	} {
		r := roles(t, g, c.prompt)
		if !slices.Equal(r.Named.KnowledgeUnits, c.units) {
			t.Errorf("%q: record units %v, want %v, the first met first", c.prompt, r.Named.KnowledgeUnits, c.units)
		}
		if r.Path != "alpha" || r.Why != "named 'dec:D1' applies to alpha over a claim by coverage on xray" || r.Unnamed {
			t.Errorf("%q: why %q path %q unnamed %v, want alpha, the ladder's first among the records' units", c.prompt, r.Why, r.Path, r.Unnamed)
		}
	}
	for _, prompt := range []string{"D1 and D2 again.", "D2 and D1 again."} {
		if r := roles(t, g, prompt); r.Found() {
			t.Errorf("%q: why %q path %q, want no unit: three units and no words to rank them", prompt, r.Why, r.Path)
		}
	}
	if r := roles(t, g, "B3 and D2 again."); r.Path != "gamma" || r.Why != "named 'les:B3' applies to gamma" || !slices.Equal(r.Named.KnowledgeUnits, []string{"gamma"}) {
		t.Errorf("two records on one unit: why %q path %q record units %v, want gamma, listed once, with no words to rank", r.Why, r.Path, r.Named.KnowledgeUnits)
	}
	if r := roles(t, g, "B4 again: fix the session purge timer."); r.Path != "xray" || r.Basis != BasisCoverage || r.Unnamed {
		t.Errorf("record on the claim: why %q path %q basis %q unnamed %v, want xray by coverage, kept", r.Why, r.Path, r.Basis, r.Unnamed)
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
