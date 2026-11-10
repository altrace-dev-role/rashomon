package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The R0 ladder tests (W-M1.13): a prompt word in name form (a path, an
// identifier, a knowledge id, a scope tag) is a claim at the top of the
// ladder, each shape one prompt the edge set missed at d527ed53.

func namesFixture(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"jobs", "jobs/purge_sessions.py", []synSym{fn("purge_sessions"), fn("decode"), fn("drop_fact")}},
		{"orgs", "orgs/settings.py", []synSym{fn("get_org_settings"), fn("org_timeout"), fn("Impact")}},
		{"sessions", "sessions/expiry.py", []synSym{fn("expire_session"), fn("session_ttl"), fn("decode"), fn("drop_fact")}},
	}, [][2]string{{"jobs/purge_sessions.py#purge_sessions", "sessions/expiry.py#expire_session"}}, nil)
	record(t, g, "req:PF-X1", "requirement", "orgs/settings.py")
	return g
}

// A path the index holds names its file and unit outright (J4 shape).
func TestNames_APathNamesItsFileAndUnit(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Fix orgs/settings.py so the override is read once.")
	if r.Why != ClassNamed || r.Path != "orgs" || string(r.File) != "file:orgs/settings.py" {
		t.Fatalf("why %q path %q file %q, want a named claim on orgs via its file", r.Why, r.Path, r.File)
	}
	if w := r.Named.Words; len(w) != 1 || w[0].Kind != NamedPath || w[0].Unit != "orgs" {
		t.Errorf("named words %+v", w)
	}
}

// A path the index lacks resolves to the unit of its directory, with the
// gap disclosed (J1 shape: a test file is not indexed, its package is).
func TestNames_AnUnindexedPathNamesItsDirectory(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Fix jobs/purge_sessions_test.py so the race detector passes.")
	if r.Why != ClassNamed || r.Path != "jobs" || r.File != "" {
		t.Fatalf("why %q path %q file %q, want jobs by directory and no file", r.Why, r.Path, r.File)
	}
	if w := r.Named.Words[0]; w.Node != "" || !strings.Contains(w.Why, "not in the index") {
		t.Errorf("word %+v, want the missing file disclosed", w)
	}
}

// A path form the index has no unit for refuses the ladder: the request
// names a file the graph cannot own, and a phrase claim over the leftover
// words would be a false positive (G1 shape: a root README). What counts
// as a file name is read from the index's own extensions, never a list.
func TestNames_AnUnknownPathRefusesTheLadder(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Update cleanup.py so the org settings section mentions the timeout.")
	if r.Found() || !strings.Contains(r.Why, "cleanup.py") {
		t.Fatalf("unit %q why %q, want no unit and the unknown path named", r.Path, r.Why)
	}
	// An extension no indexed file has is not a path form: the word is an
	// English word to the frame, and the phrase ladder reads the rest.
	r = roles(t, g, "Update README.md so the org settings section mentions the timeout.")
	if len(r.Named.Words) != 0 {
		t.Fatalf("named %+v, want no named word", r.Named.Words)
	}
}

// An identifier (an underscore, a case change) names its symbol, its file
// and its unit (J2 shape); one held by two units resolves by the rest of
// the prompt's words against each, else stands disclosed and unclaimed. A
// bare lower-case word is an English word even when a symbol shares it
// ("decode", "report", "scan"): the phrase ladder reads those.
func TestNames_AnIdentifierNamesItsSymbol(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "expire_session double counts when two sessions share a ttl; fix it.")
	if r.Why != ClassNamed || r.Path != "sessions" || string(r.File) != "file:sessions/expiry.py" || len(r.Targets) != 1 {
		t.Fatalf("why %q path %q file %q targets %v", r.Why, r.Path, r.File, r.Targets)
	}
	r = roles(t, g, "drop_fact drops empty facts; keep them.")
	if r.Named.Unit != "" || len(r.Named.Words) != 1 || !strings.Contains(r.Named.Words[0].Why, "2 units") {
		t.Errorf("ambiguous identifier: named %+v, want it disclosed and unclaimed", r.Named)
	}
	r = roles(t, g, "drop_fact drops empty facts when a session expires; keep them.")
	if r.Why != ClassNamed || r.Path != "sessions" {
		t.Errorf("identifier held by two units, prompt names one: why %q path %q, want sessions", r.Why, r.Path)
	}
	r = roles(t, g, "decode drops empty facts; keep them.")
	if len(r.Named.Words) != 0 {
		t.Errorf("bare lower-case word read as a name: %+v", r.Named.Words)
	}
	r = roles(t, g, "The function Impact drops facts whose org is empty.")
	if r.Why != ClassNamed || r.Path != "orgs" {
		t.Errorf("capitalised mid-sentence word: why %q path %q, want orgs", r.Why, r.Path)
	}
}

// A qualified name (unit.symbol) resolves inside the unit it names (J3 shape).
func TestNames_AQualifiedNameResolvesInItsUnit(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "sessions.decode should report the elapsed time separately.")
	if r.Why != ClassNamed || r.Path != "sessions" || len(r.Targets) != 1 || !strings.HasSuffix(string(r.Targets[0]), "expiry.decode") {
		t.Fatalf("why %q path %q targets %v", r.Why, r.Path, r.Targets)
	}
}

// A knowledge id names its record: the record leads the knowledge role,
// and when the words claim no unit the record's evidence does (N1 shape).
func TestNames_AKnowledgeIDNamesItsRecord(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Implement PF-X1.")
	if len(r.Knowledge) == 0 || string(r.Knowledge[0]) != "req:PF-X1" {
		t.Fatalf("knowledge %v, want req:PF-X1 first", r.Knowledge)
	}
	if r.Path != "orgs" || !strings.Contains(r.Why, "PF-X1") {
		t.Errorf("why %q path %q, want orgs through the record's evidence", r.Why, r.Path)
	}
}

// A scope tag ("the jobs package") names the unit whose last path segment
// it is (E2 shape).
func TestNames_AScopeTagNamesTheUnit(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "The flaky test in the jobs package fails under -race; make it deterministic.")
	if r.Why != ClassNamed || r.Path != "jobs" {
		t.Fatalf("why %q path %q, want jobs by its tag", r.Why, r.Path)
	}
}

// A name inside a constraint clause is what not to touch, so it claims
// nothing (O1 shape); the first name outside one wins.
func TestNames_AConstraintClauseNamesNothing(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Do not touch orgs/settings.py; change only sessions/expiry.py so the ttl is read once.")
	if r.Why != ClassNamed || r.Path != "sessions" {
		t.Fatalf("why %q path %q, want sessions (orgs is excluded)", r.Why, r.Path)
	}
}

// The anchor phrase (a definite phrase the prompt marks as already
// existing) wins over a name when they disagree, and the disagreement is
// disclosed (W-M1.13's yield).
func TestNames_TheAnchorWinsOverANameWithDisclosure(t *testing.T) {
	g := namesFixture(t)
	r := roles(t, g, "Add the ttl to sessions/expiry.py wherever the org settings already appear.")
	if r.Path != "orgs" || !strings.Contains(r.Why, "sessions/expiry.py") {
		t.Fatalf("why %q path %q, want orgs by the anchor with the name disclosed", r.Why, r.Path)
	}
}

// The frame keeps an identifier's case so its tokens split (J3 shape:
// verify.EvaluateStop is three tokens, not one unknown word).
func TestFrame_AnIdentifierKeepsItsCase(t *testing.T) {
	f := Parse("sessions.ExpireSession should report the fold step's time separately.")
	if len(f.Targets) == 0 || strings.Join(f.Targets[0].Tokens, " ") != "session expir" {
		t.Fatalf("targets %v, want [session expir]", f.Targets)
	}
}

// possessiveFixture is namesFixture with a lesson and a decision beside
// its requirement: one record of each kind a knowledge id can name.
func possessiveFixture(t *testing.T) *graph.Store {
	t.Helper()
	g := namesFixture(t)
	record(t, g, "les:L7", "lesson", "jobs/purge_sessions.py")
	record(t, g, "dec:D7", "decision", "sessions/expiry.py")
	return g
}

// A knowledge id in the possessive names its record as the bare id does,
// with either apostrophe, the word kept as the prompt wrote it (W-M1.44,
// KI08 shape: D165's). Before it the word was read as an owner's name,
// never as its record.
func TestNames_APossessiveKnowledgeIDNamesItsRecord(t *testing.T) {
	g := possessiveFixture(t)
	for _, tc := range []struct{ prompt, word, id string }{
		{"Implement PF-X1's check.", "PF-X1's", "req:PF-X1"},
		{"Implement PF-X1’s check.", "PF-X1’s", "req:PF-X1"},
		{"Apply L7's fix to the purge.", "L7's", "les:L7"},
		{"Apply L7’s fix to the purge.", "L7’s", "les:L7"},
		{"Follow D7's ruling for the expiry.", "D7's", "dec:D7"},
		{"Follow D7’s ruling for the expiry.", "D7’s", "dec:D7"},
	} {
		t.Run(tc.word, func(t *testing.T) {
			r := roles(t, g, tc.prompt)
			n := r.Named
			if len(n.Knowledge) != 1 || string(n.Knowledge[0]) != tc.id || len(n.Unknown) != 0 {
				t.Fatalf("knowledge %v unknown %+v, want %s alone and nothing refused", n.Knowledge, n.Unknown, tc.id)
			}
			i := slices.IndexFunc(n.Words, func(w NamedWord) bool { return w.Kind == NamedKnowledge })
			if i < 0 || n.Words[i].Word != tc.word || string(n.Words[i].Node) != tc.id {
				t.Errorf("words %+v, want %q read as %s", n.Words, tc.word, tc.id)
			}
			if len(r.Knowledge) == 0 || string(r.Knowledge[0]) != tc.id {
				t.Errorf("knowledge role %v, want %s first", r.Knowledge, tc.id)
			}
		})
	}
}

// The possessive read's known limit (W-M1.44): an s' ending names nothing.
// The ASCII apostrophe is trimmed before the word is read, so D7s' arrives
// as D7s, a knowledge id's form with no such record, and falls through
// every other reading; D7s’ keeps its mark, which no reading takes. dec:D7
// exists, yet the word names nothing and refuses nothing: the brief reads
// the prompt as if the word were absent. A change that reads s' changes
// this test.
func TestNames_KnownLimit_AnSApostropheKnowledgeIDNamesNothing(t *testing.T) {
	g := possessiveFixture(t)
	for _, prompt := range []string{"Follow D7s' ruling for the expiry.", "Follow D7s’ ruling for the expiry."} {
		n := roles(t, g, prompt).Named
		if len(n.Words) != 0 || len(n.Knowledge) != 0 || len(n.Unknown) != 0 {
			t.Errorf("%q: words %+v knowledge %v unknown %+v, want nothing named and nothing refused", prompt, n.Words, n.Knowledge, n.Unknown)
		}
	}
}

// One record named twice in a prompt, once in the possessive and once
// bare, is one record: one id in the named knowledge and one knowledge
// line in the brief (W-M1.44). The loop's seen key is the word as written,
// so both words are read; the record is kept once where it is added.
func TestNames_ARecordNamedTwiceIsOneRecord(t *testing.T) {
	g := possessiveFixture(t)
	b := Build(g, graph.RevCommit(graphtest.Commit), "Implement PF-X1's check as PF-X1 says.", Options{IDF: true})
	t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
	n := b.Roles.Named
	var words []string
	for _, w := range n.Words {
		if w.Kind == NamedKnowledge {
			words = append(words, w.Word)
		}
	}
	if strings.Join(words, " ") != "PF-X1's PF-X1" {
		t.Fatalf("knowledge words %q, want both spellings read", words)
	}
	if len(n.Knowledge) != 1 || n.Knowledge[0] != "req:PF-X1" {
		t.Errorf("named knowledge %v, want req:PF-X1 once", n.Knowledge)
	}
	lines := 0
	for _, l := range b.Lines {
		if l.Key == "req:PF-X1" {
			lines++
		}
	}
	if lines != 1 {
		t.Errorf("%d brief lines for req:PF-X1, want 1", lines)
	}
}

// programFixture holds a unit named install that declares cmd, and one
// named fold: a stem a program file and a data file can share.
func programFixture(t *testing.T) *graph.Store {
	t.Helper()
	return synth(t, []synFile{
		{"install", "install/install.py", []synSym{fn("install"), fn("cmd")}},
		{"fold", "fold/fold.py", []synSym{fn("fold")}},
	}, nil, nil)
}

// A program file the index does not hold names no unit by its stem
// (W-M1.46, NG01 shape: install.sh, the release installer at the root, is
// not internal/install); after a place word it refuses the ladder. Written
// with its directory it names that directory's unit, as before; a qualified
// word whose right part is a declared symbol reads as that symbol, though
// the symbol is spelled as a program kind; a data file keeps its stem's
// reading (fold.json), and so does a program kind the corpus holds files
// of (install.py names the file install/install.py).
func TestNames_AProgramFileTheIndexLacksNamesNoUnitByItsStem(t *testing.T) {
	g := programFixture(t)
	word := func(t *testing.T, prompt, w string) (Named, NamedWord) {
		t.Helper()
		n := roles(t, g, prompt).Named
		for _, nw := range n.Words {
			if nw.Word == w {
				return n, nw
			}
		}
		t.Fatalf("%q: no word %q in %+v", prompt, w, n.Words)
		return n, NamedWord{}
	}
	t.Run("install.sh", func(t *testing.T) {
		n, w := word(t, "Run install.sh before the deploy.", "install.sh")
		if w.Kind != NamedPath || w.Unit != "" || w.Node != "" || w.Refuses || len(n.Unknown) != 1 {
			t.Errorf("word %+v unknown %+v, want an unknown path that names no unit", w, n.Unknown)
		}
		if _, w := word(t, "Fix the check in install.sh.", "install.sh"); !w.Refuses || w.Unit != "" {
			t.Errorf("after a place word %+v, want the ladder refused", w)
		}
	})
	t.Run("install/install.sh", func(t *testing.T) {
		if _, w := word(t, "Run install/install.sh before the deploy.", "install/install.sh"); w.Unit != "install" {
			t.Errorf("word %+v, want the install unit by its directory", w)
		}
	})
	t.Run("install.cmd", func(t *testing.T) {
		if _, w := word(t, "Call install.cmd before the deploy.", "install.cmd"); w.Kind != NamedQualified || w.Unit != "install" || w.Node == "" {
			t.Errorf("word %+v, want the declared symbol cmd in install", w)
		}
	})
	t.Run("fold.json", func(t *testing.T) {
		if _, w := word(t, "Update fold.json with the new key.", "fold.json"); w.Unit != "fold" {
			t.Errorf("word %+v, want the fold unit by its stem", w)
		}
	})
	t.Run("install.py", func(t *testing.T) {
		if _, w := word(t, "Run install.py before the deploy.", "install.py"); w.Kind != NamedPath || w.Unit != "install" || string(w.Node) != "file:install/install.py" {
			t.Errorf("word %+v, want the file install/install.py in install", w)
		}
	})
}

// The .js limit of W-M1.46: .js is a program kind, so a runtime spelled as
// a file name (Node.js, Vue.js, D3.js) is a program file the index lacks.
// Away from a place word it is an unknown path that names no unit: the
// ladder's own claim stands, and only a ladder that claims nothing is
// refused on it, as on any path the index lacks; after a place word ("in
// Node.js") it refuses the ladder. Before W-M1.46 it named nothing. A unit named by its path in the same
// request keeps its line in either order, because the named unit is read
// before any refusal. A change that reads a runtime name changes this test.
func TestNames_KnownLimit_ARuntimeSpelledAsAJSFileIsAProgramFile(t *testing.T) {
	g := programFixture(t)
	node := func(t *testing.T, r Roles) NamedWord {
		t.Helper()
		for _, nw := range r.Named.Words {
			if nw.Word == "Node.js" {
				return nw
			}
		}
		t.Fatalf("no word Node.js in %+v", r.Named.Words)
		return NamedWord{}
	}
	t.Run("away from a place word", func(t *testing.T) {
		r := roles(t, g, "Port fold to Node.js.")
		if w := node(t, r); w.Kind != NamedPath || w.Unit != "" || w.Node != "" || w.Refuses {
			t.Errorf("word %+v, want an unknown path that names no unit and refuses nothing", w)
		}
		if r.Named.Unit != "" || len(r.Picks) == 0 || r.Picks[0].Path != "fold" || r.Picks[0].Refused != "" {
			t.Errorf("named unit %q picks %+v, want the ladder's claim on fold to stand", r.Named.Unit, r.Picks)
		}
		r = roles(t, g, "Port the hook to Node.js.")
		if len(r.Picks) == 0 || r.Picks[0].Refused != "named 'Node.js': Node.js is not in the index" {
			t.Errorf("picks %+v, want the ladder, which claims nothing, refused on Node.js as on any unknown path", r.Picks)
		}
	})
	t.Run("after a place word", func(t *testing.T) {
		r := roles(t, g, "Run the hook in Node.js.")
		if w := node(t, r); !w.Refuses || w.Unit != "" {
			t.Errorf("word %+v, want it to refuse the ladder", w)
		}
		if len(r.Picks) == 0 || r.Picks[0].Refused != "named 'Node.js': Node.js is not in the index" {
			t.Errorf("picks %+v, want the ladder refused on Node.js", r.Picks)
		}
	})
	t.Run("beside a unit named by its path", func(t *testing.T) {
		for _, prompt := range []string{"Fix the check in install/install.py in Node.js.", "In Node.js, fix the check in install/install.py."} {
			b := Build(g, graph.RevCommit(graphtest.Commit), prompt, Options{IDF: true})
			t.Logf("\n%s\nbrief:\n%s", b.Explain(), b.Text())
			if w := node(t, b.Roles); !w.Refuses {
				t.Errorf("%q: word %+v, want Node.js read as refusing", prompt, w)
			}
			unit := slices.IndexFunc(b.Lines, func(l Line) bool { return l.Role == "unit" && l.Key == "unit:py:install" })
			if b.Roles.Named.Unit != "install" || unit < 0 {
				t.Errorf("%q: named unit %q lines %+v, want the install unit's line kept", prompt, b.Roles.Named.Unit, b.Lines)
			}
		}
	})
}
