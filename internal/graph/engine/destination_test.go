package engine

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The destination fixtures (W-M1.28): a construction of the closed list
// in destinationWords names its last phrase as what the change takes its
// object onto, a use and never a target, and the reuse role draws a line
// from it only when a symbol's name outside the target resolves it.

// destination is the frame's destination use, if it has one.
func destination(f Frame) (Phrase, bool) {
	for _, p := range f.Uses {
		if p.Destination {
			return p, true
		}
	}
	return Phrase{}, false
}

// Every construction of the list names its object as the target and its
// last phrase as a destination use, for each word that opens it.
func TestFrame_EachConstructionNamesItsDestinationAsAUse(t *testing.T) {
	for _, prompt := range []string{
		"Replace the line reader in report with the line scanner.",
		"Swap the line reader in report with the line scanner.",
		"Move the line reader in report onto the line scanner.",
		"Move the line reader in report to the line scanner.",
		"Migrate the line reader in report onto the line scanner.",
		"Migrate the line reader in report to the line scanner.",
		"Port the line reader in report onto the line scanner.",
		"Port the line reader in report to the line scanner.",
		"Relocate the line reader in report onto the line scanner.",
		"Relocate the line reader in report to the line scanner.",
		"Switch the line reader in report to use the line scanner.",
		"Switch the line reader in report to read the line scanner.",
		"Convert the line reader in report to use the line scanner.",
		"Convert the line reader in report to read the line scanner.",
		"Consolidate the line reader in report into the line scanner.",
		"Fold the line reader in report into the line scanner.",
		"Merge the line reader in report into the line scanner.",
		"Route the line reader in report through the line scanner.",
	} {
		f := Parse(prompt)
		if d, ok := destination(f); !ok || words(d) != tokensOf("line scanner") {
			t.Errorf("%q: uses %v, want [line scanner] a destination", prompt, f.Uses)
		}
		if len(f.Targets) == 0 || words(f.Targets[0]) != tokensOf("line reader") || hasPhrase(f.Targets, tokensOf("line scanner")) {
			t.Errorf("%q: targets %v, want [line reader] first and the destination none", prompt, f.Targets)
		}
	}
}

// Only the list's constructions name a destination: a "to" a verb
// follows opens an infinitive, a switch needs its object and its use
// word, a verb outside the list takes none ("onto main" after rebase),
// a use word's object stays a plain use, and an add's "to" is its place.
func TestFrame_OnlyTheListsConstructionsNameADestination(t *testing.T) {
	for _, prompt := range []string{
		"Move the flag parsing to make startup faster.",
		"Switch to verbose output.",
		"Switch the logger to verbose output.",
		"Rebase the branch onto main.",
		"Replace the reader using the line scanner.",
		"Add the line scanner to report.",
	} {
		if d, ok := destination(Parse(prompt)); ok {
			t.Errorf("%q: destination %v, want none", prompt, d)
		}
	}
}

// A phrase one sentence names as a destination and another names after a
// use word is a plain use: the use word's reading holds.
func TestFrame_ADestinationAUseWordAlsoNamesIsAPlainUse(t *testing.T) {
	f := Parse("Replace the line reader in report with the line scanner. Read the seed rows using the line scanner.")
	if !hasPhrase(f.Uses, tokensOf("line scanner")) {
		t.Fatalf("uses %v lack the line scanner", f.Uses)
	}
	if d, ok := destination(f); ok {
		t.Errorf("destination %v, want the line scanner a plain use", d)
	}
}

// premise is the weight of each word of phrase in g, so a fixture can
// assert the shape of its graph before it reads the rule's result.
func premise(g *graph.Store, phrase string) (map[string]float64, []string) {
	toks := Tokens(phrase)
	return scan(g, graph.RevCommit(graphtest.Commit)).weights(toks, true), toks
}

// A destination a symbol's name outside the target resolves is a reuse
// line on the use basis, for each word that opens a destination: the
// request presumes the line scanner exists and takes the reader onto it.
func TestReuse_AResolvedDestinationIsAUseLine(t *testing.T) {
	g := synth(t, []synFile{
		{"report", "report/render.py", []synSym{
			{"func", "renderReport", "Render the report from its input rows."},
			{"func", "lineReader", "Read the report's input one line at a time, by hand."},
		}},
		{"scan", "scan/lines.py", []synSym{{"func", "LineScanner", "Scan a stream into lines."}}},
		{"seed", "seed/load.py", []synSym{{"func", "loadSeed", "Load the seed rows."}}},
	}, [][2]string{{"report/render.py#renderReport", "report/render.py#lineReader"}, {"seed/load.py#loadSeed", "scan/lines.py#LineScanner"}}, nil)
	for _, prompt := range []string{
		"Replace the line reader in report with the line scanner.",
		"Move the line reader in report onto the line scanner.",
		"Port the line reader in report to the line scanner.",
		"Switch the line reader in report to use the line scanner.",
		"Convert the line reader in report to read the line scanner.",
		"Fold the line reader in report into the line scanner.",
		"Route the line reader in report through the line scanner.",
	} {
		r := roles(t, g, prompt)
		if r.File != graph.FileID("report/render.py") {
			t.Errorf("%q: file %s, want report/render.py", prompt, r.File)
			continue
		}
		if len(r.Reuse) != 1 || r.Reuse[0].File != graph.FileID("scan/lines.py") || r.Reuse[0].Basis != ReuseByUse {
			t.Errorf("%q: reuse %+v, want scan/lines.py on the use basis", prompt, r.Reuse)
		}
	}
}

// The counter-cases draw no line: a position ("to the end"), a switch
// with no object, a switch to a thing the graph declares no name for, a
// manner ("with care"), and a branch after a verb outside the list
// ("onto main", with main declared). The docs hold every destination
// word and no name does but main: a doc's word resolves nothing.
func TestReuse_AnUnresolvedDestinationDrawsNoLine(t *testing.T) {
	g := synth(t, []synFile{
		{"sync", "sync/loop.py", []synSym{
			{"func", "runLoop", "Run the sync loop to the end of the queue, with care for partial rows."},
			{"func", "checkLimit", "Check the row limit."},
			{"func", "parseRows", "Parse the rows; verbose output prints each one."},
		}},
		{"report", "report/render.py", []synSym{{"func", "renderReport", "Render the report in the branch's format."}}},
		{"cmd/app", "cmd/app/main.py", []synSym{fn("main")}},
		{"log", "log/logger.py", []synSym{{"func", "Logger", "Write log lines."}}},
	}, [][2]string{{"sync/loop.py#runLoop", "sync/loop.py#checkLimit"}, {"cmd/app/main.py#main", "sync/loop.py#runLoop"}}, nil)
	for _, c := range []struct{ prompt, dest string }{
		{"Move the check to the end.", "end"},
		{"Switch to verbose output.", ""},
		{"Switch the logger to use verbose output.", "verbose output"},
		{"Replace the parser with care.", "care"},
		{"Rebase the branch onto main.", ""},
	} {
		r := roles(t, g, c.prompt)
		if d, ok := destination(r.Frame); ok != (c.dest != "") || ok && words(d) != tokensOf(c.dest) {
			t.Errorf("%q: uses %v, want the destination %q", c.prompt, r.Frame.Uses, c.dest)
		}
		if len(r.Reuse) != 0 {
			t.Errorf("%q: reuse %+v, want none", c.prompt, r.Reuse)
		}
	}
}

// A destination the target's own file or a consumer of the target
// declares is no reuse line: those files hold the work, not the thing to
// take up.
func TestReuse_ADestinationInTheTargetOrAConsumerIsSilent(t *testing.T) {
	g := synth(t, []synFile{
		{"fetch", "fetch/client.py", []synSym{
			{"func", "fetchPage", "Fetch a page, retrying inline on error."},
			{"func", "Backoff", "Wait before the next attempt."},
		}},
		{"store", "store/rows.py", []synSym{{"func", "writeRows", "Write the rows to the table."}}},
		{"sync", "sync/push.py", []synSym{
			{"func", "pushRows", "Push the pending rows."},
			{"func", "Batcher", "Group rows into batches."},
		}},
	}, [][2]string{{"sync/push.py#pushRows", "store/rows.py#writeRows"}, {"sync/push.py#pushRows", "sync/push.py#Batcher"}}, nil)
	r := roles(t, g, "Replace the inline retry in fetchPage with the backoff.")
	if r.File != graph.FileID("fetch/client.py") || len(r.Reuse) != 0 {
		t.Errorf("file %s, reuse %+v: want the target file's backoff silent", r.File, r.Reuse)
	}
	r = roles(t, g, "Route the row writes in writeRows through the batcher.")
	if r.File != graph.FileID("store/rows.py") || !slices.Contains(files(r.Consumers), graph.FileID("sync/push.py")) || len(r.Reuse) != 0 {
		t.Errorf("file %s, consumers %v, reuse %+v: want the consumer's batcher silent", r.File, files(r.Consumers), r.Reuse)
	}
}

// A name holding most of a destination's mass but not its head (its
// heaviest word that names code) resolves nothing: "the shared event
// queue" where "shared" names code elsewhere and EventQueue holds the
// two lighter words. The premise is asserted: the name's share is over
// ReuseFloor, so the head check alone refuses it.
func TestReuse_ANameWithoutTheDestinationsHeadResolvesNothing(t *testing.T) {
	g := synth(t, []synFile{
		{"audit", "audit/log.py", []synSym{{"func", "writeAudit", "Write one audit record."}}},
		{"events", "events/queue.py", []synSym{{"func", "EventQueue", "Queue events for delivery."}}},
		{"events", "events/kinds.py", []synSym{fn("eventKind"), fn("queueDepth")}},
		{"shared", "shared/pool.py", []synSym{fn("Pool")}},
	}, nil, nil)
	w, toks := premise(g, "shared event queue")
	shared, event, queue := toks[0], toks[1], toks[2]
	if !(w[shared] > w[queue] && w[shared] > w[event] && (w[event]+w[queue])/(w[shared]+w[event]+w[queue]) >= ReuseFloor) {
		t.Fatalf("weights %v: want shared the head and the name's share at or over %v", w, ReuseFloor)
	}
	if r := roles(t, g, "Route the audit writes through the shared event queue."); r.File != graph.FileID("audit/log.py") || len(r.Reuse) != 0 {
		t.Errorf("file %s, reuse %+v: want EventQueue unresolved without the head", r.File, r.Reuse)
	}
}

// The generic head where the repository makes it generic: "helper" in
// several names, "shared" in a doc. Helper holds the destination's head
// but not ReuseFloor of its mass, so the floor refuses it; no name
// holding "helper" resolves "the shared helper". The premise is asserted.
func TestReuse_AGenericHeadIsRefusedByTheFloor(t *testing.T) {
	g := synth(t, []synFile{
		{"client", "client/fetch.py", []synSym{
			{"func", "fetch", "Fetch a page through the shared session."},
			{"func", "retryLoop", "Retry the request until it succeeds."},
		}},
		{"common", "common/util.py", []synSym{{"func", "Helper", "Run a function under a timeout."}}},
		{"test", "test/helpers.py", []synSym{fn("pageHelper"), fn("rowHelper"), fn("helperFor")}},
		{"report", "report/format.py", []synSym{fn("formatHelper"), fn("render")}},
	}, [][2]string{{"client/fetch.py#fetch", "client/fetch.py#retryLoop"}}, nil)
	w, toks := premise(g, "shared helper")
	if shared, helper := toks[0], toks[1]; !(w[shared] > w[helper] && w[helper] > 0) {
		t.Fatalf("weights %v: want helper lighter than shared", w)
	}
	if r := roles(t, g, "Fold the retry loop into the shared helper."); r.File != graph.FileID("client/fetch.py") || len(r.Reuse) != 0 {
		t.Errorf("file %s, reuse %+v: want the generic head refused", r.File, r.Reuse)
	}
}

// The generic head alone, the review's probe: "the shared helper" on a
// graph whose only match is a symbol named Helper, with no other word of
// the destination in its name and "shared" held nowhere. The review
// expected no line; it resolves, and this test pins that as the rule's
// boundary case. The corpus holds "helper" once and "shared" never, so
// "shared" weighs in at the mean of the words that name code, helper's
// own weight, and Helper holds exactly ReuseFloor of the mass; one more
// word the corpus lacks refuses it (the next test). A word list would
// refuse this case and is not added: fitted to the development rows, it
// is the overfit the rule refuses. The fresh read's line precision is the
// test of the limit, and a rule that refuses this case flips this test.
func TestReuse_AGenericHeadTheGraphHoldsAloneResolves(t *testing.T) {
	g := synth(t, []synFile{
		{"client", "client/fetch.py", []synSym{
			{"func", "fetch", "Fetch a page."},
			{"func", "retryLoop", "Retry the request until it succeeds."},
		}},
		{"common", "common/util.py", []synSym{{"func", "Helper", "Run a function under a timeout."}}},
	}, [][2]string{{"client/fetch.py#fetch", "client/fetch.py#retryLoop"}}, nil)
	w, toks := premise(g, "shared helper")
	if shared, helper := toks[0], toks[1]; w[shared] != 0 || w[helper] == 0 {
		t.Fatalf("weights %v: want shared held nowhere and helper held", w)
	}
	r := roles(t, g, "Fold the retry loop into the shared helper.")
	if r.File != graph.FileID("client/fetch.py") || len(r.Reuse) != 1 || r.Reuse[0].File != graph.FileID("common/util.py") || r.Reuse[0].Basis != ReuseByUse {
		t.Errorf("file %s, reuse %+v: want the known limit, common/util.py on the use basis", r.File, r.Reuse)
	}
}

// A destination mostly of words the corpus never holds names a thing the
// index lacks: "the shared generic helper" on the probe's graph, Helper
// the one name holding "helper", "shared" and "generic" held nowhere.
// Each word the corpus lacks weighs in at the mean of the words that
// name code, as Units weighs a target's, so Helper holds a third of the
// mass and the floor refuses it. The premise is asserted.
func TestReuse_ADestinationMostlyOfWordsTheCorpusLacksIsRefused(t *testing.T) {
	g := synth(t, []synFile{
		{"client", "client/fetch.py", []synSym{
			{"func", "fetch", "Fetch a page."},
			{"func", "retryLoop", "Retry the request until it succeeds."},
		}},
		{"common", "common/util.py", []synSym{{"func", "Helper", "Run a function under a timeout."}}},
	}, [][2]string{{"client/fetch.py#fetch", "client/fetch.py#retryLoop"}}, nil)
	w, toks := premise(g, "shared generic helper")
	if len(toks) != 3 || w[toks[0]] != 0 || w[toks[1]] != 0 || w[toks[2]] == 0 || scan(g, graph.RevCommit(graphtest.Commit)).nameDF[toks[2]] != 1 {
		t.Fatalf("tokens %v, weights %v: want shared and generic held nowhere and helper in one name", toks, w)
	}
	if r := roles(t, g, "Fold the retry loop into the shared generic helper."); r.File != graph.FileID("client/fetch.py") || len(r.Reuse) != 0 {
		t.Errorf("file %s, reuse %+v: want the destination refused, two of its three words unknown to the corpus", r.File, r.Reuse)
	}
}
