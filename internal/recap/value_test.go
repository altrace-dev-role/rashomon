package recap

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// indent is the continuation lines' lead, under the text after the prefix.
var indent = strings.Repeat(" ", len([]rune(prefix)))

// gateAt is a required learned gate named name, of kind, in state s for
// reason r, whose last run is call seq with exit (nil: no code); a green
// gate's last green is that run.
func gateAt(name, kind string, s verify.State, r verify.Reason, seq int64, exit *int) verify.GateState {
	g := verify.GateState{Gate: name, Kind: kind, Family: "r:" + name + "@d:repo", Source: "learned", Required: true,
		State: s, Reason: r, LastRun: &verify.RunRef{Seq: seq, ExitCode: exit}}
	if s == verify.Green {
		g.LastGreen = &verify.RunRef{Seq: seq}
	}
	return g
}

func exitCode(n int) *int { return &n }

// viewOf is a Stop view over gates, with row and turn, whose Programs name
// each gate by the word before its colon: "go:test" runs go.
func viewOf(row verify.Row, turn verify.TurnCounts, gates ...verify.GateState) verify.StopView {
	v := verify.StopView{
		Snap:     verify.Snapshot{Recording: verify.Complete, Gates: gates},
		Summary:  verify.Summary{Row: row},
		Turn:     turn,
		Programs: map[string]string{},
	}
	for _, g := range gates {
		prog, _, _ := strings.Cut(g.Gate, ":")
		v.Programs[g.Gate] = prog
	}
	return v
}

// lineOf is the rendered value line: the state sentence, the clause when
// there is one, and the settings install's pointer for sess-1.
func lineOf(sentence, clause string) string {
	s := prefix + sentence + ".\n"
	if clause != "" {
		s += indent + clause + "\n"
	}
	return s + indent + "→ rashomon report --session sess-1"
}

var (
	wrote   = verify.TurnCounts{Writes: 2, PathsWritten: 2, GateRuns: 1, LastWriteSeq: 5}
	noWrite = verify.TurnCounts{GateRuns: 1}
)

// TestVER65_ValueLineRowsRender: each row's sentence from tech 2.2.11's
// table, with its placeholders filled from the view; the no-write variants;
// `and <k> more` past three gates; a prefix word in a program's place, which
// names the kind (D94); and the width rule, a state sentence over 80 columns
// naming fewer gates until it fits (card 2.4 R6).
func TestVER65_ValueLineRowsRender(t *testing.T) {
	goGreen := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	goRed := gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1))
	goStale := gateAt("go:test", "test", verify.Stale, verify.WritesAfterGreen, 3, nil)
	npmGreen := gateAt("npm:test", "test", verify.Green, verify.OK, 5, nil)
	unnamed := wrote
	unnamed.UnnamedWriteSeq = 9
	findings := viewOf(verify.RowFindings, wrote, goGreen)
	findings.Summary.Sentence = "1 record finding(s) in this turn"
	incomplete := viewOf(verify.RowUnknown, wrote)
	incomplete.Snap.Recording = verify.Incomplete
	incomplete.Summary.Sentence = "recording incomplete for this turn; verification state unknown"
	oneFile := verify.TurnCounts{Writes: 1, PathsWritten: 1, GateRuns: 1, LastWriteSeq: 5}
	advisoryGreen := goGreen
	advisoryGreen.Required = false
	advisory := viewOf(verify.RowUnverified, wrote, advisoryGreen)
	advisory.Summary.Sentence = "advisory only"
	learned := viewOf(verify.RowNoGate, wrote)
	learned.Index = &verify.ProjectIndex{ProjectKey: "/work/repo", Entries: []verify.IndexEntry{{Family: "r:go@d:repo", Kind: "test", Sessions: 2}}}

	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"verified", viewOf(verify.RowVerified, wrote, goGreen), "verified - go passed after the last edit"},
		{"verified, one program twice", viewOf(verify.RowVerified, wrote, goGreen, gateAt("go:build", "build", verify.Green, verify.OK, 4, nil)),
			"verified - go passed after the last edit"},
		{"verified, three gates", viewOf(verify.RowVerified, wrote, goGreen, npmGreen, gateAt("make:test", "test", verify.Green, verify.OK, 6, nil)),
			"verified - go, npm and make passed after the last edit"},
		{"verified, four gates", viewOf(verify.RowVerified, wrote, goGreen, npmGreen, gateAt("make:test", "test", verify.Green, verify.OK, 6, nil),
			gateAt("cargo:test", "test", verify.Green, verify.OK, 7, nil)),
			"verified - go, npm, make and 1 more passed after the last edit"},
		{"red", viewOf(verify.RowRed, wrote, goRed), "2 files edited; go exited 1 after the last edit (call 7)"},
		{"red, one file", viewOf(verify.RowRed, oneFile, goRed), "1 file edited; go exited 1 after the last edit (call 7)"},
		{"red, no write", viewOf(verify.RowRed, noWrite, goRed), "go exited 1 in this turn (call 7)"},
		{"red, no exit code", viewOf(verify.RowRed, wrote, gateAt("go:test", "test", verify.Red, verify.FailedOutput, 7, nil)),
			"2 files edited; go failed after the last edit (call 7)"},
		{"red beside a green", viewOf(verify.RowRed, wrote, npmGreen, goRed), "2 files edited; go exited 1 after the last edit (call 7)"},
		{"masked", viewOf(verify.RowMasked, wrote, gateAt("go:test", "test", verify.MaskedInconclusive, verify.MaskedNoSuccessLine, 7, nil)),
			"go ran after the last edit; its exit status was hidden by its line (call 7)"},
		{"masked, no write", viewOf(verify.RowMasked, noWrite, gateAt("go:test", "test", verify.MaskedInconclusive, verify.MaskedNoSuccessLine, 7, nil)),
			"go ran in this turn; its exit status was hidden by its line (call 7)"},
		{"backgrounded", viewOf(verify.RowBackgrounded, wrote, gateAt("go:test", "test", verify.Backgrounded, verify.BackgroundLaunch, 7, nil)),
			"go moved to the background after the last edit; its result is not in the record"},
		{"backgrounded, no write", viewOf(verify.RowBackgrounded, noWrite, gateAt("go:test", "test", verify.Backgrounded, verify.BackgroundLaunch, 7, nil)),
			"go moved to the background in this turn; its result is not in the record"},
		{"ran only", viewOf(verify.RowRanOnly, noWrite, goGreen), "no edits; go passed (call 3)"},
		{"ran only, a prefix word for program", viewOf(verify.RowRanOnly, noWrite, gateAt("set:test", "test", verify.Green, verify.OK, 3, nil)),
			"no edits; test passed (call 3)"},
		{"unverified beside a green", viewOf(verify.RowUnverified, wrote, npmGreen, goStale),
			"2 files edited; npm passed after the last edit, go not run after it"},
		{"unverified", viewOf(verify.RowUnverified, wrote, goStale), "2 files edited, no test or build run after the last edit"},
		{"unverified, empty run", viewOf(verify.RowUnverified, noWrite, gateAt("go:test", "test", verify.Absent, verify.EmptyRun, 3, nil)),
			"go ran in this turn and ran no checks (call 3)"},
		{"unverified, partial run", viewOf(verify.RowUnverified, noWrite, gateAt("go:test", "test", verify.Absent, verify.PartialOnly, 3, nil)),
			"go ran on part of the project in this turn (call 3)"},
		{"unknown, recording incomplete", incomplete, "recording incomplete for this turn; verification state unknown"},
		{"unknown, order", viewOf(verify.RowUnknown, wrote, gateAt("go:test", "test", verify.Unknown, verify.OrderUnknown, 7, nil)),
			"the record does not order every go run and edit in this turn; verification state unknown"},
		{"unknown, outcome", viewOf(verify.RowUnknown, wrote, gateAt("go:test", "test", verify.Unknown, verify.OutcomeUnknown, 7, nil)),
			"go ran after the last edit (call 7); its result is not in the record"},
		{"no gate, none known", viewOf(verify.RowNoGate, wrote), "2 files edited; no test or build is known for this project yet"},
		{"no gate, learned elsewhere", learned, "2 files edited; no known test or build applies to these files"},
		{"unnamed write", viewOf(verify.RowUnnamedWrite, unnamed, goGreen), "2 files edited; go passed (call 3); call 9 may have written files after that"},
		{"findings", findings, "2 files edited; go passed after the last edit; 1 record finding(s) in this turn"},
		{"advisory only", advisory, "2 files edited; go passed after the last edit; advisory only"},
	} {
		if got, want := ValueLine(c.v, nil, "sess-1", false), lineOf(c.want, ""); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}

	if got := ValueLine(viewOf(verify.RowNone, verify.TurnCounts{}, goGreen), nil, "sess-1", false); got != "" {
		t.Errorf("RowNone printed %q, want nothing", got)
	}
	if got, want := ValueLine(viewOf(verify.RowVerified, wrote, goGreen), nil, "sess-1", true),
		prefix+"verified - go passed after the last edit.\n"+indent+"→ /rashomon:report --session sess-1"; got != want {
		t.Errorf("plugin pointer:\n got %q\nwant %q", got, want)
	}

	// Width: three gates make an 83-column state line, so it names two.
	wide := viewOf(verify.RowVerified, wrote,
		gateAt("pytest:t", "test", verify.Green, verify.OK, 3, nil), gateAt("eslint:l", "lint", verify.Green, verify.OK, 4, nil),
		gateAt("cargo:t", "test", verify.Green, verify.OK, 5, nil), gateAt("gradle:t", "test", verify.Green, verify.OK, 6, nil),
		gateAt("mvn:t", "test", verify.Green, verify.OK, 7, nil))
	got := ValueLine(wide, nil, "sess-1", false)
	if want := lineOf("verified - pytest, eslint and 3 more passed after the last edit", ""); got != want {
		t.Errorf("width:\n got %q\nwant %q", got, want)
	}
	for _, l := range strings.Split(got, "\n") {
		if n := len([]rune(l)); n > 80 {
			t.Errorf("width: line %q is %d columns, want at most 80", l, n)
		}
	}
}

// steerLine is one steer log line with the keys DeliveredNotes reads.
func steerLine(t *testing.T, state, promptID, event, pat, modelText string) []byte {
	t.Helper()
	m := map[string]any{"type": "steer", "state": state, "event": event, "pattern": pat, "level": "nudge", "model_text": modelText}
	if promptID != "" {
		m["prompt_id"] = promptID
	} else {
		m["prompt_id"] = nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestFUN15_AutoSteerClauseCountsDeliveredNotes: the clause counts the
// prompt's delivered notes, never a delivered flag (no model text), a
// suppressed line, another prompt's line or a line with no prompt; one note
// is named by its title and its event, several by their count; a
// SessionStart note has no <when> (D78); no note, no clause (card 2.4 R8).
func TestFUN15_AutoSteerClauseCountsDeliveredNotes(t *testing.T) {
	lines := [][]byte{
		steerLine(t, "delivered", "p1", "PostToolUseFailure", "regression-location", "go test failed at call 7"),
		steerLine(t, "delivered", "p1", "PostToolUse", "claim-vs-record", ""),
		steerLine(t, "suppressed", "p1", "PostToolUse", "identical-retry", "the same call failed twice"),
		steerLine(t, "delivered", "p2", "PostToolUse", "identical-retry", "the same call failed twice"),
		steerLine(t, "delivered", "", "PostToolUse", "identical-retry", "the same call failed twice"),
		[]byte(`{"state":"delivered","prompt_id":"p1"`),
	}
	notes := DeliveredNotes(lines, "p1")
	want := []Note{{Pattern: "regression-location", Event: "PostToolUseFailure", ModelText: []byte("go test failed at call 7")}}
	if !slices.EqualFunc(notes, want, func(a, b Note) bool {
		return a.Pattern == b.Pattern && a.Event == b.Event && string(a.ModelText) == string(b.ModelText)
	}) {
		t.Fatalf("DeliveredNotes = %+v, want only the delivered note of p1 %+v", notes, want)
	}

	v := viewOf(verify.RowRed, wrote, gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1)))
	const sentence = "2 files edited; go exited 1 after the last edit (call 7)"
	for _, c := range []struct {
		name   string
		notes  []Note
		clause string
	}{
		{"one note", notes, "AutoSteer: 1 regression location given at the failure."},
		{"two notes", append(slices.Clone(notes), Note{Pattern: "identical-retry", Event: "PostToolUse"}), "AutoSteer: 2 notes given."},
		{"one note at the prompt", []Note{{Pattern: "subtask-gate", Event: "UserPromptSubmit"}}, "AutoSteer: 1 task check given with your prompt."},
		{"one note at session start", []Note{{Pattern: "continuity-briefing", Event: "SessionStart"}}, "AutoSteer: 1 briefing given."},
		{"no note", nil, ""},
	} {
		if got, want := ValueLine(v, c.notes, "sess-1", false), lineOf(sentence, c.clause); got != want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
}

// TestFUN15c_WidestClauseFitsEightyColumns: the clause has no rule for a
// line over 80 columns in v0 (D89), so every title with every <when>, under
// the continuation indent, is held to 80 here: a title or phrase that grows
// past it fails this test rather than printing a wide line.
func TestFUN15c_WidestClauseFitsEightyColumns(t *testing.T) {
	if len(titles()) == 0 || len(whens()) == 0 {
		t.Fatalf("%d titles and %d <when> phrases, want both tables filled", len(titles()), len(whens()))
	}
	for _, title := range titles() {
		for _, when := range whens() {
			clause := indent + "AutoSteer: 1 " + title + " given " + when + "."
			if n := len([]rune(clause)); n > 80 {
				t.Errorf("%q is %d columns, want at most 80", clause, n)
			}
		}
	}
}

// bannedWords is FUN-15's list (tech 2.2.11): no template or title holds one.
var bannedWords = []string{"saved", "prevent", "would have", "without rashomon", "completed", "done", "the task",
	"fixed", "compliant", "%", "rate"}

// banned reports the first banned word in text, matched whole and in any case.
func banned(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, w := range bannedWords {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(w) + `\b`)
		if w == "%" {
			re = regexp.MustCompile(`%`)
		}
		if re.MatchString(lower) {
			return w, true
		}
	}
	return "", false
}

// TestFUN15b_TemplatesUseNoBannedWord: every pattern the clause can name has
// a title, and no title, <when> phrase or string in value.go, the value
// line's templates, holds a banned word (card 2.4 R6). The done flag lives
// in done.go: its sentence names the claim class, "done", by design (R7).
func TestFUN15b_TemplatesUseNoBannedWord(t *testing.T) {
	for _, id := range pattern.All() {
		if id == pattern.Integrity || id == pattern.ModelSwitch || id == pattern.SequenceRisk {
			continue // flag only, user only, or reserved: never delivered (tech 2.2.11 titles; pattern.go)
		}
		if titles()[string(id)] == "" {
			t.Errorf("pattern %q has no title", id)
		}
	}
	for id, title := range titles() {
		if w, ok := banned(title); ok {
			t.Errorf("title %q of %q holds %q", title, id, w)
		}
	}
	for event, when := range whens() {
		if w, ok := banned(when); ok {
			t.Errorf("<when> %q of %q holds %q", when, event, w)
		}
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "value.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if w, ok := banned(text); ok {
			t.Errorf("%s: template %q holds %q", fset.Position(lit.Pos()), text, w)
		}
		return true
	})
}
