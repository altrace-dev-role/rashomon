package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The T1c ladder rules (W-M1.14): each test is one shape the pre-registered
// edge set missed at d527ed53, named by its row.

func ladderFixture(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"recap", "recap/recap.go", []synSym{fn("Run"), fn("judge")}},
		{"settings", "settings/decide.go", []synSym{fn("decision"), fn("locate")}},
		{"gateout", "gateout/classify.go", []synSym{fn("classify"), fn("truncated")}},
		{"verify", "verify/gates.go", []synSym{fn("gateOutput"), fn("classifyRunner"), fn("ledger")}},
		{"learn", "learn/ledger.go", []synSym{fn("Ledger"), fn("openLedger")}},
		{"budget", "budget/budget.go", []synSym{fn("left"), fn("deadline")}},
		{"store", "store/left.go", []synSym{fn("leftover"), fn("left")}},
		{"scripts/mergeguard", "scripts/mergeguard/main.go", []synSym{fn("guard")}},
		{"scripts/reanchor", "scripts/reanchor/main.go", []synSym{fn("reanchor")}},
		{"install", "install/install.go", []synSym{fn("Entries"), fn("writeHooks")}},
		{"platform", "platform/impact.go", []synSym{fn("Impact")}},
	}, nil, nil)
	g.AssertNode(graph.Node{ID: graph.SymID(synLang, "recap", "type", "impact.Impact"), Kind: "type", Tier: graph.Declared, Conf: 1, Rev: graph.RevCommit(graphtest.Commit)})
	g.AssertEdge(graph.Edge{Src: graph.FileID("recap/recap.go"), Pred: graph.Declares, Dst: graph.SymID(synLang, "recap", "type", "impact.Impact"), Tier: graph.Declared, Conf: 1, Rev: graph.RevCommit(graphtest.Commit)})
	record(t, g, "les:B1", "lesson", "verify/gates.go", "recap/recap.go")
	// A file in no unit: its words are names in the index that no unit's
	// claim can cover.
	g.AssertNode(graph.Node{ID: graph.FileID(".github/workflows/ci.yml"), Kind: "file", Tier: graph.Declared, Conf: 1, Rev: graph.RevCommit(graphtest.Commit)})
	return g
}

// A phrase that holds a unit's own name claims that unit (C1 shape: "the
// recap decision" is recap's, though settings declares a decision).
func TestLadder_APhraseNamingTheUnitClaimsIt(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Rename the recap decision so it is called by the turn it judges.")
	if r.Why != ClassUnit || r.Path != "recap" {
		t.Fatalf("why %q path %q, want recap by its name", r.Why, r.Path)
	}
}

// A unit named by a compound ("gateout" from "gate output") is named
// (E3 shape), and a parent directory's name is not the unit's (G4 shape:
// "script" names no unit under scripts/).
func TestLadder_ACompoundSegmentAndAParentDirectory(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Write a table-driven test for the gate output classifier covering truncated runner output.")
	if r.Why != ClassUnit || r.Path != "gateout" {
		t.Fatalf("why %q path %q, want gateout by its compound name", r.Why, r.Path)
	}
	r = roles(t, g, "Rewrite the mutation sweep script so it clears its own cache per batch.")
	if r.Found() {
		t.Fatalf("why %q path %q, want no unit: 'script' is a directory and the rest names nothing", r.Why, r.Path)
	}
}

// A one-word phrase that several units name is ambiguous, not the one
// with the tidiest name (K2 shape: "left").
func TestLadder_AOneWordPhraseHeldByTwoUnitsIsAmbiguous(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Fix left.")
	if r.Found() || !strings.Contains(r.Why, "ambiguous") {
		t.Fatalf("why %q path %q, want ambiguous", r.Why, r.Path)
	}
}

// A partial claim must name the phrase's head: a unit that names the
// modifier alone ("CI" of "the CI workflow") is not the thing (G2 shape).
func TestLadder_APartialClaimNamesTheHead(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Fix the ledger workflow so the perf-tagged tests run on linux only.")
	if r.Found() || !strings.Contains(r.Why, "head") {
		t.Fatalf("why %q path %q, want refused for the head", r.Why, r.Path)
	}
}

// A knowledge id the prompt names settles a tie among the units its
// evidence lies in (N2 shape: "ledger" is verify's and learn's; B1 applies
// to verify).
func TestLadder_ANamedRecordSettlesATie(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Address B1 in the ledger.")
	if r.Path != "verify" || !strings.Contains(r.Why, "B1") {
		t.Fatalf("why %q path %q, want verify settled by B1", r.Why, r.Path)
	}
	if len(r.Knowledge) == 0 || r.Knowledge[0] != "les:B1" {
		t.Errorf("knowledge %v, want les:B1 first", r.Knowledge)
	}
}

// An identifier two units declare resolves to the callable when the
// prompt calls it a function (J5 shape: platform's func over recap's type).
func TestLadder_AKindWordPrefersTheKind(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "The function Impact drops facts whose config is empty; keep them.")
	if r.Why != ClassNamed || r.Path != "platform" {
		t.Fatalf("why %q path %q, want platform's function", r.Why, r.Path)
	}
}

// A file the prompt places the change in that the index lacks refuses
// the ladder (G3 shape: go.mod), as does a capitalised owner the index
// lacks (G1 shape: the README's section); a test name resolves to the
// unit it tests (P3 shape).
func TestLadder_UnknownPlacesAndOwnersRefuse(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Bump the ledger version in go.mod and the toolchain line.")
	if r.Found() || !strings.Contains(r.Why, "go.mod") {
		t.Fatalf("why %q path %q, want refused on go.mod", r.Why, r.Path)
	}
	r = roles(t, g, "Update the README's install section to mention the plugin.")
	if r.Found() || !strings.Contains(r.Why, "README") {
		t.Fatalf("why %q path %q, want refused on the README", r.Why, r.Path)
	}
	r = roles(t, g, "--- FAIL: TestInstall_Events (0.00s)\n    install_test.go:88: events differ")
	if r.Why != ClassNamed || r.Path != "install" {
		t.Fatalf("why %q path %q, want install by the test name", r.Why, r.Path)
	}
}

// The whole-prompt fallback (a verb with no object) claims a unit only
// by its name (K4 shape: "clean up" is not a claim on whatever declares a
// clean), and a bare word is its own subject.
func TestLadder_TheFallbackClaimsOnlyByName(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Clean up.")
	if r.Found() || !r.Fallback {
		t.Fatalf("why %q path %q fallback %v, want no unit from a fallback", r.Why, r.Path, r.Fallback)
	}
	r = roles(t, g, "budget")
	if r.Path != "budget" {
		t.Fatalf("why %q path %q, want budget by name", r.Why, r.Path)
	}
}

// A possessive splits the phrase: the owner is the target first (D3
// shape: "the seed counter's duplicate of the secret-shape check").
func TestFrame_APossessiveSplitsThePhrase(t *testing.T) {
	f := Parse("Delete the recap counter's duplicate of the ledger check and call the shared one.")
	if len(f.Targets) < 2 || words(f.Targets[0]) != tokensOf("recap counter") || words(f.Targets[1]) != tokensOf("duplicate ledger check") {
		t.Fatalf("targets %v, want the owner then the owned", f.Targets)
	}
	// A causative's object ends at its bare verb: what the digest should do is not its name.
	f = Parse("Without changing the record schema, make the digest carry the turn's elapsed time.")
	if len(f.Targets) == 0 || words(f.Targets[0]) != tokensOf("digest") {
		t.Fatalf("targets %v, want the digest cut at its bare verb", f.Targets)
	}
	f = Parse("Let the obligation ledger carry the turn window each obligation was opened in.")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("obligation ledger") {
		t.Fatalf("targets %v, want the obligation ledger", f.Targets)
	}
	// A verb-class word after a determiner is a noun; the question's subject ends at the bare verb after it.
	f = Parse("Why does the Stop verdict say unknown when the test command was piped?")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("stop verdict") {
		t.Fatalf("targets %v, want the stop verdict", f.Targets)
	}
	// A place inside a consequence clause is not the change's place, and a clause after a place word names no thing.
	f = Parse("Do not touch the store; change only the render so the kept count shows in the Stop line.")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("render") || !f.Targets[0].Definite {
		t.Fatalf("targets %v, want the render alone, definite", f.Targets)
	}
	f = Parse("continue where we left off")
	if len(f.Targets) != 0 {
		t.Fatalf("targets %v, want none", f.Targets)
	}
	f = Parse("Add a context parameter to every hook entry so the budget deadline is passed in instead of read from a global.")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("hook entry") {
		t.Fatalf("targets %v, want the hook entry", f.Targets)
	}
	// A question's subject ends at its bare verb.
	f = Parse("How does the harness decide which agent fired a hook?")
	if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("harness") {
		t.Fatalf("targets %v, want the harness", f.Targets)
	}
}

// A unit named by the phrase's head yields to the ladder: "the integrity
// report" is a report that verify renders (PG12 shape), so the unit whose
// names cover the modifier takes it, by coverage, and a report unit's own
// name is one word of evidence against it. Here the store unit is named
// by the head and budget declares the modifier.
func TestLadder_AHeadNamedUnitYieldsToTheModifiersOwner(t *testing.T) {
	g := ladderFixture(t)
	r := roles(t, g, "Show them in the deadline store.")
	if r.Path != "budget" || r.Why != ClassPartial {
		t.Fatalf("path %q why %q, want budget (partial): the head names a unit, the modifier's owner decides", r.Path, r.Why)
	}
}

// A unit named by a modifier yields to one symbol elsewhere whose name is
// the whole phrase (PL12 shape: platformObligations in verify over the
// platform unit): a declared name that is the phrase outweighs a
// directory that is one word of it. The same modifier still claims
// against partial and full readings (TestLadder_APhraseNamingTheUnitClaimsIt).
func TestLadder_AModifierNamedUnitYieldsToASingleSymbol(t *testing.T) {
	g := synth(t, []synFile{
		{"platform", "platform/impact.go", []synSym{fn("Impact"), fn("Read")}},
		{"verify", "verify/platform.go", []synSym{fn("platformObligations"), fn("obligations")}},
		{"recap", "recap/recap.go", []synSym{fn("Run")}},
	}, nil, nil)
	r := roles(t, g, "Show it wherever the platform obligations already appear.")
	if r.Path != "verify" || r.Why != ClassSingle {
		t.Fatalf("path %q why %q, want verify (single): one name is the whole phrase", r.Path, r.Why)
	}
}

// A phrase whose head no name holds, but the tree mentions ("the CI
// workflow" over a tree whose workflow file is not indexed, whose docs
// name CI in many places and the workflow in one), names a thing the
// index lacks: the one unit that names the modifier (fromCI in
// gateinfer) does not take it.
func TestLadder_AHeadNoNameHoldsIsNotTakenByTheModifiersUnit(t *testing.T) {
	g := synth(t, []synFile{
		{"gateinfer", "gateinfer/gateinfer.go", []synSym{fn("fromCI"), fn("scan")}},
		{"brief", "brief/brief.go", []synSym{{kind: "func", name: "testsFact", doc: "names the workflow that ran the tests"}}},
		{"verify", "verify/gates.go", []synSym{{kind: "func", name: "gates", doc: "the gates read under CI"}}},
	}, nil, nil)
	r := roles(t, g, "Fix the CI workflow so the tests run on linux only.")
	if r.Found() {
		t.Fatalf("path %q why %q, want no unit: no name holds the head 'workflow'", r.Path, r.Why)
	}
}

// A trailing word in an adverb's form that no name holds is the verb's
// manner and leaves the phrase ("resolve the prompt root lazily" is the
// prompt root's unit, whatever the docs say about laziness); one a name
// holds stays its head ("the message assembly" over an assembly name).
func TestRoles_ATrailingAdverbNoNameHoldsLeavesThePhrase(t *testing.T) {
	g := synth(t, []synFile{
		{"cmd", "cmd/preflight.go", []synSym{fn("promptRoot"), fn("promptSymbols")}},
		{"store", "store/root.go", []synSym{{kind: "func", name: "DefaultRoot", doc: "the root, resolved lazily on first use"}}},
		{"shape", "shape/assembly.go", []synSym{fn("messageAssembly"), fn("assemble")}},
	}, nil, nil)
	r := roles(t, g, "Resolve the prompt root lazily.")
	if r.Path != "cmd" || len(r.Target) != 2 {
		t.Fatalf("path %q target %v why %q, want cmd on [prompt root]", r.Path, r.Target, r.Why)
	}
	r = roles(t, g, "Fix the message assembly.")
	if r.Path != "shape" || len(r.Target) != 2 {
		t.Fatalf("path %q target %v why %q, want shape on [message assembly]: a name holds assembly", r.Path, r.Target, r.Why)
	}
}

// A head-named unit keeps the phrase when no other unit declares the
// thing itself: "the test harness" is harness's, though report's names
// touch both words (testBending, harnessRow); it yields when another
// unit's name is wholly in the phrase (the integrity report shape).
// The ladder reads the lower-case "blind"; capitalised, "Blind" is an
// identifier that names caps.Blind, so the named claim decides first.
func TestLadder_AHeadNamedUnitKeepsThePhraseOverTouchingNames(t *testing.T) {
	g := synth(t, []synFile{
		{"harness", "harness/caps.go", []synSym{fn("Blind"), fn("identityTable")}},
		{"report", "report/testbending.go", []synSym{fn("testBending"), fn("harnessRow"), fn("account")}},
		{"consent", "consent/consent.go", []synSym{fn("testMode")}},
	}, nil, nil)
	r := roles(t, g, "The test harness builds no table for an identity's blind.")
	if r.Path != "harness" || r.Why != ClassUnit {
		t.Fatalf("path %q why %q, want harness (unit): no name elsewhere is the thing", r.Path, r.Why)
	}
	r = roles(t, g, "The test harness builds no table for an identity's Blind.")
	if r.Why != ClassNamed || r.Path != "harness" {
		t.Fatalf("path %q why %q, want harness (named): Blind names caps.Blind", r.Path, r.Why)
	}
	g = synth(t, []synFile{
		{"harness", "harness/caps.go", []synSym{fn("Blind")}},
		{"report", "report/test.go", []synSym{fn("test"), fn("harnessRow")}},
	}, nil, nil)
	r = roles(t, g, "The test harness builds no table.")
	if r.Path != "report" {
		t.Fatalf("path %q why %q, want report: its name test is the modifier itself", r.Path, r.Why)
	}
}

// A derived reading is an inflected form of an index name ("installer"
// through install): the rest of the word is a suffix, so a compound
// ("leftmost" through left, "workflow" through work in the G2 shape)
// stays unknown and the phrase it heads is refused, not claimed by
// whatever declares the shorter word.
func TestRoles_ADerivedNameIsAnInflectedForm(t *testing.T) {
	g := ladderFixture(t)
	c := scan(g, graph.RevCommit(graphtest.Commit))
	got, _ := c.derive([]string{"installer", "leftmost"}, nil)
	if got[0] != "install" || got[1] != "leftmost" {
		t.Fatalf("derived %v, want [install leftmost]: er is a suffix, most is a word", got)
	}
}
