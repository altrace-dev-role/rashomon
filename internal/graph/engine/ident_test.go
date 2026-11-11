package engine

import (
	"path"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The identifier gate (W-DEBT.14): a capitalised word off the sentence's
// start is a name without a kind word unless it is a word the engine's own
// lists hold; the declarations under test/ and those whose language is the
// word itself are dropped unless code font or a kind word says a name is
// meant; a survivor no other prompt word corroborates is refused unless
// its form needs no context. One case per part, each the shape of the
// development row it came from.

func identFixture(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"billing", "billing/invoice.py", []synSym{{"type", "Quorum", "counts the ballots cast in one round"}}},
		{"streams", "streams/reader.py", []synSym{{"func", "Read", "reads one frame from the socket"}}},
		{"test/perf", "test/perf/payload.py", []synSym{{"func", "Ballast", "pads each payload to the frame size"}}},
		{"cooling", "cooling/fans.py", []synSym{{"func", "Quench", "drops the fan speed"}, {"func", "QuenchAll", "drops every fan to idle"}}},
		{"test/heat", "test/heat/soak.py", []synSym{{"func", "Quench", "drops the oven heat"}}},
	}, nil, nil)
	addSym(g, "go", "runner", "runner/run.go", "func", "Go", "starts the job in the background")
	return g
}

// addSym enters one symbol of another language than synth's, with its
// unit, its file and the containing and declaring edges, as synth does.
func addSym(s *graph.Store, lang, unit, file, kind, name, doc string) {
	rev := graph.RevCommit(graphtest.Commit)
	prov := graph.Prov{Extractor: "synth", Version: "1"}
	uid, fid := graph.UnitID(lang, unit), graph.FileID(file)
	id := graph.SymID(lang, unit, kind, strings.TrimSuffix(path.Base(file), path.Ext(file))+"."+name)
	s.AssertNode(graph.Node{ID: uid, Kind: "unit", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	s.AssertNode(graph.Node{ID: fid, Kind: "file", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	s.AssertNode(graph.Node{ID: id, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov,
		Attrs: map[string]string{"symkind": kind, "doc": doc}})
	s.AssertEdge(graph.Edge{Src: uid, Pred: graph.Contains, Dst: fid, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
	s.AssertEdge(graph.Edge{Src: fid, Pred: graph.Declares, Dst: id, Tier: graph.Algorithmic, Conf: 1, Rev: rev, Prov: prov})
}

// identWord is R0's reading of word w in prompt, when it named one.
func identWord(g *graph.Store, prompt, w string) (NamedWord, bool) {
	for _, nw := range Build(g, graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true}).Roles.Named.Words {
		if nw.Word == w {
			return nw, true
		}
	}
	return NamedWord{}, false
}

func wantStated(t *testing.T, g *graph.Store, prompt, w, kind, unit, node string) {
	t.Helper()
	if nw, ok := identWord(g, prompt, w); !ok || nw.Kind != kind || nw.Unit != unit || string(nw.Node) != node {
		t.Errorf("%q in %q: named %v %+v, want %s %s (%s)", w, prompt, ok, nw, kind, unit, node)
	}
}

func wantUnnamed(t *testing.T, g *graph.Store, prompt, w string) {
	t.Helper()
	if nw, ok := identWord(g, prompt, w); ok {
		t.Errorf("%q in %q: named %+v, want no name", w, prompt, nw)
	}
}

// A capitalised name off a kind word, in none of the engine's word lists,
// declared in one unit, whose doc the rest of the prompt meets (ballots,
// round), is a name (NN21 shape: Layout, by its doc).
func TestIdent_ACorroboratedCapitalisedNameIsStated(t *testing.T) {
	g := identFixture(t)
	wantStated(t, g, "Make sure Quorum ignores ballots from the previous round.", "Quorum", NamedIdent, "billing",
		"sym:py:billing:type:invoice.Quorum")
}

// The same name with no other prompt word in its path or doc is refused:
// alone it is as often a proper noun as a symbol (DL10 shape: Codex).
func TestIdent_AnUncorroboratedCapitalisedNameIsRefused(t *testing.T) {
	g := identFixture(t)
	wantUnnamed(t, g, "Make sure Quorum waits for the late voters.", "Quorum")
}

// A capitalised verb or closed-class word the engine reads clauses by is
// English when bare, though the prompt meets its path and doc (stream,
// reader, frame); code font or a kind word still name it (Stop shape).
func TestIdent_AnEngineWordIsANameOnlyInCodeFontOrAfterAKindWord(t *testing.T) {
	g := identFixture(t)
	wantUnnamed(t, g, "Make sure the stream reader calls Read on one frame.", "Read")
	wantStated(t, g, "Make sure `Read` keeps one frame from the socket.", "Read", NamedIdent, "streams",
		"sym:py:streams:func:reader.Read")
	wantStated(t, g, "Make sure the function Read keeps one frame.", "Read", NamedIdent, "streams",
		"sym:py:streams:func:reader.Read")
}

// The word that is its own symbol's language (Go in a Go index) is the
// language when bare, though the prompt meets its path (runner); code font
// or a kind word still name it. The drop reads the id's language field, so
// the id form is pinned: a change to it fails here, not silently.
func TestIdent_TheLanguageWordIsANameOnlyInCodeFontOrAfterAKindWord(t *testing.T) {
	g := identFixture(t)
	wantUnnamed(t, g, "Make sure the runner calls Go after one crash.", "Go")
	wantStated(t, g, "Make sure `Go` restarts the runner after one crash.", "Go", NamedIdent, "runner", "sym:go:runner:func:run.Go")
	wantStated(t, g, "Make sure the function Go restarts the runner.", "Go", NamedIdent, "runner", "sym:go:runner:func:run.Go")
	if id := string(graph.SymID("go", "runner", "func", "run.Go")); !strings.HasPrefix(id, "sym:"+low("Go")+":") {
		t.Errorf("symbol id %q does not open with sym:<language>:, the form the language drop reads", id)
	}
}

// A symbol declared only under test/ is not named by its name: not bare,
// not in code font, not after a kind word (CC30 shape: SessionEnd stated
// test/perf/payload).
func TestIdent_ATestOnlySymbolIsNotNamedByItsName(t *testing.T) {
	g := identFixture(t)
	wantUnnamed(t, g, "Make sure Ballast pads every payload to the frame size.", "Ballast")
	wantUnnamed(t, g, "Make sure `Ballast` pads every payload to the frame size.", "Ballast")
	wantUnnamed(t, g, "Make sure the function Ballast pads every payload.", "Ballast")
}

// The test/ drop is resolveIdent's alone: unit.symbol resolves through the
// qualified reading, which reads every declaration.
func TestIdent_KnownLimit_AQualifiedNameStillNamesATestOnlySymbol(t *testing.T) {
	g := identFixture(t)
	wantStated(t, g, "perf.Ballast pads every payload to the frame size.", "perf.Ballast", NamedQualified, "test/perf",
		"sym:py:test/perf:func:payload.Ballast")
}

// Two units declare the name and the test/ drop leaves one, which no other
// prompt word corroborates: bare it is refused, not stated on no evidence
// (O1 shape: Stop between internal/hook and test/perf/payload). Code font
// asks for no corroboration, so there the survivor is stated. A form that
// needs no context (an inner capital) at score 0 is stated as before.
func TestIdent_ASurvivorAtScoreZeroIsRefusedBareAndAStrongFormIsNot(t *testing.T) {
	g := identFixture(t)
	wantUnnamed(t, g, "Make sure Quench holds until the timer fires.", "Quench")
	wantStated(t, g, "Make sure `Quench` holds until the timer fires.", "Quench", NamedIdent, "cooling",
		"sym:py:cooling:func:fans.Quench")
	wantStated(t, g, "Make sure QuenchAll holds until the timer fires.", "QuenchAll", NamedIdent, "cooling",
		"sym:py:cooling:func:fans.QuenchAll")
}

// A capitalised word that opens the sentence is no name, though the rest of
// the prompt meets its doc: the sentence's start capitalises any word.
func TestIdent_ASentenceInitialCapitalisedWordIsNoName(t *testing.T) {
	g := identFixture(t)
	wantUnnamed(t, g, "Quorum ignores ballots from the previous round.", "Quorum")
}
