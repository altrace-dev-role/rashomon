package intent_test

// PW1b-ON-230 step 3 (WED-SLICE row 1(c)): the symbols the request named,
// as the platform graph proves them, reach the model in the task-preflight
// note, the one note the prompt gives it.

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
)

const symLine = "symbols named: 'CountRows' declared in 'internal/a', direct callers 2 in 2 packages"

func symIntent() intent.Intent {
	return intent.Intent{Named: true, Paths: []string{"internal/a.go"}, Unknown: []string{"no spec found"},
		Symbols: []intent.Symbol{{Name: "CountRows", Dir: "internal/a", Callers: 2, Packages: 2}}}
}

func lines(s string) []string { return strings.Split(s, "\n") }

// TestPW1b3_TheNoteGivesTheSymbolsBeforeItsUnknowns: a later prompt's note
// carries the symbols line after the gates, the unknowns in the person's
// aside (TOKEN-BUDGET1); the first prompt's carries it too, its aside the
// project line; beside a lesson, the lesson follows it. The rules and gates
// list nothing here, so the model's text is the header, the path and the
// symbols. Break: render the symbols on no row, and this fails.
func TestPW1b3_TheNoteGivesTheSymbolsBeforeItsUnknowns(t *testing.T) {
	in := symIntent()
	f, ok := intent.FindingKeyed(in, intent.Delivered{}, nil, 0, nil)
	got := lines(f.Message.Text())
	if !ok || f.Message.Template() != "task-preflight-symbols/v2" || len(got) != 3 || got[2] != symLine || f.Message.Aside() != "unknown: no spec found" {
		t.Errorf("later note (%s):\n%s\naside %q\nwant the symbols line third, and the unknowns aside", f.Message.Template(), f.Message.Text(), f.Message.Aside())
	}

	b := &intent.Brief{Manifests: []string{"go.mod"}, Module: "example.com/m", GoDirs: 1}
	f, ok = intent.FindingKeyed(in, intent.Delivered{}, b, 0, nil)
	got = lines(f.Message.Text())
	if !ok || f.Message.Template() != "task-preflight-first-symbols/v2" || len(got) != 3 || got[2] != symLine ||
		!strings.HasPrefix(f.Message.Aside(), "project files: none; manifests: 'go.mod'") {
		t.Errorf("first note (%s):\n%s\naside %q\nwant the symbols line last, and the project line aside", f.Message.Template(), f.Message.Text(), f.Message.Aside())
	}

	ride := &intent.Ride{ID: "a-gen", Text: "internal/a.go is generated from gen/a.tmpl"}
	f, ok, rode := intent.FindingRiding(in, intent.Delivered{}, nil, 0, nil, ride)
	got = lines(f.Message.Text())
	if !ok || !rode || f.Message.Template() != "task-preflight-symbols-lesson/v2" || len(got) != 4 || got[2] != symLine ||
		!strings.HasPrefix(got[3], "lesson a-gen,") {
		t.Errorf("with a lesson (%s, rode %v):\n%s\nwant the symbols line, then the lesson", f.Message.Template(), rode, f.Message.Text())
	}
}

// TestPW1b3_ASymbolIsNewToTheSession: a request that names a proven symbol
// and nothing the session was not already given still gets its note, since
// a caller count is read now. Control: the same request with no symbol
// proven gets none. Break: leave the symbols out of what makes a note
// fresh.
func TestPW1b3_ASymbolIsNewToTheSession(t *testing.T) {
	in := symIntent()
	d := intent.Delivered{BriefGiven: true, Paths: map[string]bool{"internal/a.go": true}}
	if _, ok := intent.FindingKeyed(in, d, nil, 0, nil); !ok {
		t.Error("a proven symbol, every path given earlier: no note")
	}
	in.Symbols = nil
	if f, ok := intent.FindingKeyed(in, d, nil, 0, nil); ok {
		t.Errorf("control: no symbol, every path given earlier: a note\n%s", f.Message.Text())
	}
}

// TestPW1b3_TheV4RecordCountsTheSymbols: at record scope v4 the steer line
// holds the symbols as their count, never a name or a directory: the names
// are the request's words. Break: record the symbols as they printed.
func TestPW1b3_TheV4RecordCountsTheSymbols(t *testing.T) {
	f, _ := intent.FindingKeyed(symIntent(), intent.Delivered{}, nil, 0, keyedWith([]byte("k")))
	rec := f.Record.Text()
	if !strings.HasSuffix(rec, "\nsymbols named: 1 from the platform graph") || strings.Contains(rec, "CountRows") || strings.Contains(rec, "internal/a'") {
		t.Errorf("v4 record:\n%s\nwant the symbols as their count, no name or directory", rec)
	}
	if !strings.Contains(f.Message.Text(), symLine) {
		t.Errorf("the note that prints:\n%s\nwant %q", f.Message.Text(), symLine)
	}
}

// TestPW1b3_ResolveAsksTheGraphForTheNamedIdentifiers: Resolve hands
// Project.Symbols the request's identifiers, each once, at most
// MaxSymbolNames, in the order named, and keeps what it proves; the
// symbols-unknown is said only when it proves none. Break: say the unknown
// whatever the graph proved, or pass every identifier.
func TestPW1b3_ResolveAsksTheGraphForTheNamedIdentifiers(t *testing.T) {
	var asked []string
	p := intent.Project{Root: t.TempDir(), Symbols: func(names []string) []intent.Symbol {
		asked = names
		return []intent.Symbol{{Name: "CountRows", Dir: "a", Callers: 1, Packages: 1}}
	}}
	in := intent.Resolve([]byte("fix CountRows, then CountRows again, SumRows, TotalRows, MaxRows and MinRows"), false, p)
	if want := []string{"CountRows", "SumRows", "TotalRows", "MaxRows"}; !slices.Equal(asked, want) {
		t.Errorf("asked the graph for %q, want %q", asked, want)
	}
	if len(in.Symbols) != 1 || slices.Contains(in.Unknown, "symbols not matched, no platform graph log") {
		t.Errorf("symbols %+v, unknown %q: want the proven symbol and no symbols-unknown", in.Symbols, in.Unknown)
	}
	p.Symbols = func([]string) []intent.Symbol { return nil }
	if in := intent.Resolve([]byte("fix CountRows"), false, p); !slices.Contains(in.Unknown, "symbols not matched, no platform graph log") {
		t.Errorf("control, nothing proven: unknown %q, want the symbols-unknown", in.Unknown)
	}
}
