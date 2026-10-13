package render

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// exampleBinding is a binding struct of the shape a pattern's PR adds: slot
// types only, one `slot` tag each. Its template is the regression-location
// card's (archive 40.4.2), labelled as a test row.
type exampleBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Exit    slot.Exit     `slot:"exit"`
	Paths   slot.PathList `slot:"paths"`
}

type briefingBinding struct {
	Gate slot.Gate `slot:"gate"`
	Exit slot.Exit `slot:"exit"`
}

type extraBinding struct {
	Gate  slot.Gate  `slot:"gate"`
	Extra slot.Count `slot:"extra"`
}

type (
	untaggedBinding  struct{ Gate slot.Gate }
	duplicateBinding struct {
		A slot.Gate `slot:"gate"`
		B slot.Gate `slot:"gate"`
	}
	stringBinding struct {
		Gate string `slot:"gate"`
	}
	pointerBinding struct {
		Path *slot.Path `slot:"path"`
	}
	interfaceBinding struct {
		Value slot.Value `slot:"value"`
	}
	unexportedBinding struct {
		gate slot.Gate `slot:"gate"`
	}
	badNameBinding struct {
		Gate slot.Gate `slot:"Gate"`
	}
)

const exampleFormat = "rashomon record: {gate} exited 0 earlier in this session ({minutes} min ago) and exited {exit} on this run. Files written between the two runs: {paths}."

var testTable = map[templateKey]template{
	{pattern.RegressionLocation, reflect.TypeOf(exampleBinding{})}: {id: "test/regression-location", format: exampleFormat},
	{pattern.ContinuityBriefing, reflect.TypeOf(briefingBinding{})}: {
		id:     "test/continuity-briefing",
		format: "rashomon record: state of this session before the compaction:\n- {gate}: last run exited {exit}.",
	},
	{pattern.RegressionLocation, reflect.TypeOf(briefingBinding{})}: {
		id:     "test/line-feed-outside-a-briefing",
		format: "rashomon record: {gate}\nexited {exit}.",
	},
	{pattern.SubtaskGate, reflect.TypeOf(briefingBinding{})}:    {id: "test/unused-slot", format: "rashomon record: {gate} only."},
	{pattern.MaskedStatus, reflect.TypeOf(briefingBinding{})}:   {id: "test/unknown-placeholder", format: "rashomon record: {gate} {exit} {nope}."},
	{pattern.IdenticalRetry, reflect.TypeOf(briefingBinding{})}: {id: "test/unclosed", format: "rashomon record: {gate {exit}."},
	{pattern.OrderingRule, reflect.TypeOf(briefingBinding{})}:   {id: "test/stray-brace", format: "rashomon record: {gate} } {exit}."},
	{pattern.CredSpray, reflect.TypeOf(briefingBinding{})}:      {id: "test/banned-word", format: "rashomon record: {gate} must exit {exit}."},
	{pattern.SubagentClaim, reflect.TypeOf(briefingBinding{})}:  {id: "test/reminder", format: "rashomon record: {gate} System-Reminder {exit}."},
	{pattern.AgentInterference, reflect.TypeOf(extraBinding{})}: {id: "test/extra", format: "rashomon record: {gate} {extra}."},
	{pattern.PlatformRule, reflect.TypeOf(untaggedBinding{})}:   {id: "test/untagged", format: "rashomon record: x."},
	{pattern.PlatformRule, reflect.TypeOf(duplicateBinding{})}:  {id: "test/duplicate", format: "rashomon record: {gate}."},
	{pattern.PlatformRule, reflect.TypeOf(stringBinding{})}:     {id: "test/string", format: "rashomon record: {gate}."},
	{pattern.PlatformRule, reflect.TypeOf(pointerBinding{})}:    {id: "test/pointer", format: "rashomon record: {path}."},
	{pattern.PlatformRule, reflect.TypeOf(interfaceBinding{})}:  {id: "test/interface", format: "rashomon record: {value}."},
	{pattern.PlatformRule, reflect.TypeOf(unexportedBinding{})}: {id: "test/unexported", format: "rashomon record: {gate}."},
	{pattern.PlatformRule, reflect.TypeOf(badNameBinding{})}:    {id: "test/bad-name", format: "rashomon record: {Gate}."},
	{pattern.PlatformRule, reflect.TypeOf("")}:                  {id: "test/not-a-struct", format: "rashomon record: x."},
}

func goodExample() exampleBinding {
	return exampleBinding{
		Gate: "go test", Minutes: 6, Exit: 1,
		Paths: slot.NewPathList([]slot.Path{"internal/a.go", "AKIAIOSFODNN7EXAMPLE.txt", "internal/b.go"}, 0),
	}
}

// TestNew_FillsTheTemplateAndReturnsTheClosedMessage: New fills the fixed
// template from the bound values and returns Message{Pattern, Template,
// Slots, Text}, with the secret-shaped listed path left out of both the
// text and the slots.
func TestNew_FillsTheTemplateAndReturnsTheClosedMessage(t *testing.T) {
	m, err := newMessage(testTable, pattern.RegressionLocation, goodExample())
	if err != nil {
		t.Fatalf("newMessage: %v", err)
	}
	want := "rashomon record: go test exited 0 earlier in this session (6 min ago) and exited 1 on this run. " +
		"Files written between the two runs: 'internal/a.go', 'internal/b.go' and 1 more."
	if m.Text() != want {
		t.Errorf("Text() = %q, want %q", m.Text(), want)
	}
	if m.Pattern() != pattern.RegressionLocation || m.Template() != "test/regression-location" {
		t.Errorf("Pattern(), Template() = %q, %q", m.Pattern(), m.Template())
	}
	var names []string
	for _, s := range m.Slots() {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"gate", "minutes", "exit", "paths"}) {
		t.Errorf("slot names = %q", names)
	}
	paths := m.Slots()[3].Value.(slot.PathList).Paths()
	if !slices.Equal(paths, []slot.Path{"internal/a.go", "internal/b.go"}) {
		t.Errorf("bound paths = %q, want the secret-shaped one left out", paths)
	}
	m.Slots()[0] = Slot{Name: "edited"}
	if m.Slots()[0].Name != "gate" {
		t.Error("editing the returned slots edited the Message")
	}
}

// TestNew_ProductionTableHoldsNoTemplateYet: a pattern's PR adds its binding
// struct and its row; until then New refuses every binding.
func TestNew_ProductionTableHoldsNoTemplateYet(t *testing.T) {
	m, err := New(pattern.RegressionLocation, goodExample())
	if !errors.Is(err, ErrNoTemplate) || !isZero(m) {
		t.Errorf("New = (%+v, %v), want (zero Message, ErrNoTemplate)", m, err)
	}
}

// TestSTR11b_NewRefusesAnyFailingSlot: a slot that fails its check gives an
// error and the zero Message, never a Message.
func TestSTR11b_NewRefusesAnyFailingSlot(t *testing.T) {
	bad := []exampleBinding{
		{Gate: "rm -rf", Minutes: 6, Exit: 1, Paths: slot.NewPathList([]slot.Path{"a.go"}, 0)},
		{Gate: "go test", Minutes: 6, Exit: 300, Paths: slot.NewPathList([]slot.Path{"a.go"}, 0)},
		{Gate: "go test", Minutes: -1, Exit: 1, Paths: slot.NewPathList([]slot.Path{"a.go"}, 0)},
		{Gate: "go test", Minutes: 6, Exit: 1, Paths: slot.NewPathList([]slot.Path{"../a.go"}, 0)},
		{Gate: "go test", Minutes: 6, Exit: 1, Paths: slot.NewPathList([]slot.Path{"AKIAIOSFODNN7EXAMPLE.txt"}, 0)},
		{Gate: "go test", Minutes: 6, Exit: 1},
	}
	for _, b := range bad {
		m, err := newMessage(testTable, pattern.RegressionLocation, b)
		if err == nil || !isZero(m) {
			t.Errorf("newMessage(%+v) = (%+v, %v), want (zero Message, an error)", b, m, err)
		}
	}
}

// TestSTR11b_NewTakesSlotTypesOnly: a binding is a struct whose fields are
// all exported slot types with a well-formed, unique `slot` tag; anything
// else is refused before a slot is read.
func TestSTR11b_NewTakesSlotTypesOnly(t *testing.T) {
	for _, b := range []any{
		untaggedBinding{Gate: "go"}, duplicateBinding{A: "go", B: "go"}, stringBinding{Gate: "go"},
		pointerBinding{}, interfaceBinding{Value: slot.Gate("go")}, interfaceBinding{},
		unexportedBinding{gate: "go"}, badNameBinding{Gate: "go"}, "a string",
	} {
		m, err := newMessage(testTable, pattern.PlatformRule, b)
		if !errors.Is(err, ErrBinding) || !isZero(m) {
			t.Errorf("newMessage(%T) = (%+v, %v), want ErrBinding", b, m, err)
		}
	}
	for _, c := range []struct {
		p pattern.ID
		b any
	}{{pattern.RegressionLocation, nil}, {pattern.RegressionLocation, &exampleBinding{}}, {pattern.ModelSwitch, goodExample()}} {
		if _, err := newMessage(testTable, c.p, c.b); !errors.Is(err, ErrNoTemplate) {
			t.Errorf("newMessage(%q, %T) = %v, want ErrNoTemplate", c.p, c.b, err)
		}
	}
	if _, err := newMessage(testTable, pattern.ID("not-a-pattern"), goodExample()); !errors.Is(err, ErrUnknownPattern) {
		t.Errorf("unknown pattern: %v, want ErrUnknownPattern", err)
	}
}

// TestNew_TemplateAndSlotsMustAgree: every placeholder names a bound slot,
// every bound slot is used, and a brace outside a placeholder is refused.
func TestNew_TemplateAndSlotsMustAgree(t *testing.T) {
	b := briefingBinding{Gate: "go test", Exit: 1}
	for _, p := range []pattern.ID{pattern.SubtaskGate, pattern.MaskedStatus, pattern.IdenticalRetry, pattern.OrderingRule} {
		if m, err := newMessage(testTable, p, b); !errors.Is(err, ErrTemplate) || !isZero(m) {
			t.Errorf("%q: (%+v, %v), want ErrTemplate", p, m, err)
		}
	}
}

// TestSTR12b_NewRunsTheWholeStringCheck: a filled template that fails the
// whole-string rule is refused; a briefing's template line feeds pass only
// for the briefing.
func TestSTR12b_NewRunsTheWholeStringCheck(t *testing.T) {
	b := briefingBinding{Gate: "go test", Exit: 1}
	for _, p := range []pattern.ID{pattern.CredSpray, pattern.SubagentClaim, pattern.RegressionLocation} {
		if m, err := newMessage(testTable, p, b); err == nil || !isZero(m) {
			t.Errorf("%q: (%+v, %v), want an error", p, m, err)
		}
	}
	m, err := newMessage(testTable, pattern.ContinuityBriefing, b)
	if err != nil {
		t.Fatalf("briefing: %v", err)
	}
	if want := "rashomon record: state of this session before the compaction:\n- go test: last run exited 1."; m.Text() != want {
		t.Errorf("briefing Text() = %q, want %q", m.Text(), want)
	}
}

// isZero reports the zero Message: Message holds a slice, so it does not
// compare with ==.
func isZero(m Message) bool {
	return m.Text() == "" && m.Pattern() == "" && m.Template() == "" && m.Slots() == nil
}

func TestMessage_ZeroValueCarriesNothing(t *testing.T) {
	var m Message
	if m.Text() != "" || m.Pattern() != "" || m.Template() != "" || m.Slots() != nil {
		t.Errorf("zero Message = %q %q %q %v", m.Text(), m.Pattern(), m.Template(), m.Slots())
	}
}

func TestMirror_WrapsByteForByte(t *testing.T) {
	got := Mirror(MessagePrefix + "frozen path touched")
	want := "※ rashomon told the agent: \"" + MessagePrefix + "frozen path touched\""
	if got != want {
		t.Errorf("Mirror() = %q, want %q", got, want)
	}
}

func TestValidate_PrefixAndByteCaps(t *testing.T) {
	if err := Validate(MessagePrefix + "ok"); err != nil {
		t.Errorf("Validate(valid) = %v", err)
	}
	if err := Validate("not the right prefix"); err == nil {
		t.Error("Validate(no prefix) = nil")
	}
	ok := MessagePrefix + strings.Repeat("a", MaxMessageBytes-len(MessagePrefix))
	if err := Validate(ok); err != nil {
		t.Errorf("Validate(%d bytes) = %v", len(ok), err)
	}
	if err := Validate(ok + "a"); err == nil {
		t.Errorf("Validate(%d bytes) = nil", len(ok)+1)
	}
	brief := MessagePrefix + strings.Repeat("a", MaxBriefingMessageBytes-len(MessagePrefix))
	if err := ValidateBriefing(brief); err != nil {
		t.Errorf("ValidateBriefing(%d bytes) = %v", len(brief), err)
	}
	if err := ValidateBriefing(brief + "a"); err == nil {
		t.Errorf("ValidateBriefing(%d bytes) = nil", len(brief)+1)
	}
	if err := Validate(brief); err == nil {
		t.Error("a note took the briefing's wider cap")
	}
}

// TestSTR12b_ReminderTagRefusedInAnyCase: no tag character, control rune or
// `system-reminder` in any case (tech 4.2.7; G4), in a note or any line of a
// briefing.
func TestSTR12b_ReminderTagRefusedInAnyCase(t *testing.T) {
	for _, bad := range []string{
		"system-reminder", "System-Reminder", "SYSTEM-REMINDER", "sYsTeM-rEmInDeR",
		"a < b", "a > b", "`code`", "tab\there", "cr\rhere", "del\x7fhere", "non-ascii é", "c1 \u0085 here",
	} {
		if err := Validate(MessagePrefix + bad); err == nil {
			t.Errorf("Validate(%q) = nil", bad)
		}
		if err := ValidateBriefing(MessagePrefix + "header\n- " + bad); err == nil {
			t.Errorf("ValidateBriefing(line %q) = nil", bad)
		}
	}
}

// TestSTR12b_LineFeedOnlyBetweenBriefingLines: a line feed is the
// separator of a briefing's lines and nothing else (tech 4.2.7: "except a
// briefing's template line feeds: every slot still rejects them, and each
// line is checked on its own"; G4: "a line feed anywhere else is refused").
func TestSTR12b_LineFeedOnlyBetweenBriefingLines(t *testing.T) {
	brief := MessagePrefix + "state of this session before the compaction:\n- go test: last run exited 1.\n- task 7: marked completed with no passing gate run after its writes."
	if err := ValidateBriefing(brief); err != nil {
		t.Errorf("ValidateBriefing(three lines) = %v", err)
	}
	if err := Validate(brief); err == nil {
		t.Error("Validate took a line feed outside a briefing")
	}
	for _, bad := range []string{
		MessagePrefix + "header\n\n- a.",
		MessagePrefix + "header\n- a.\n",
		"\n" + MessagePrefix + "header",
		MessagePrefix + "header" + strings.Repeat("\n- a.", 6),
		MessagePrefix + "header\r\n- a.",
	} {
		if err := ValidateBriefing(bad); err == nil {
			t.Errorf("ValidateBriefing(%q) = nil", bad)
		}
	}
	if err := ValidateBriefing(MessagePrefix + "header" + strings.Repeat("\n- a.", 5)); err != nil {
		t.Errorf("a header and five lines: %v", err)
	}
}

// TestSTR14b_BannedWordsOutsideAQuote: outside a quote, none of the banned
// words as ASCII case-insensitive whole words (tech 4.2.7); inside a quote,
// a repository's or a spec's own words stay as they are.
func TestSTR14b_BannedWordsOutsideAQuote(t *testing.T) {
	for _, w := range []string{"must", "should", "please", "need", "verify", "verified", "compliant", "catch", "caught", "lie", "cheat", "deleted"} {
		for _, form := range []string{w, strings.ToUpper(w), strings.ToUpper(w[:1]) + w[1:]} {
			if err := Validate(MessagePrefix + "the note " + form + " here."); err == nil {
				t.Errorf("Validate(banned %q) = nil", form)
			}
		}
	}
	for _, good := range []string{
		"the filesystem check passed.",
		"Files written between the two runs: 'internal/verify/a.go', 'cmd/must.go' and 2 more.",
		"last run's exit status was hidden by a pipe.",
		"This project's spec, trusted on 2026-10-12, states: 'it's the staging profile, you must use it'.",
		"needs and verifies are other words.",
	} {
		if err := Validate(MessagePrefix + good); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", good, err)
		}
	}
	for _, bad := range []string{
		"states: 'a' must 'b'.",
		"an unclosed 'quote must be refused",
		"'internal/a.go' then Verify.",
	} {
		if err := Validate(MessagePrefix + bad); err == nil {
			t.Errorf("Validate(%q) = nil", bad)
		}
	}
}

// TestSTR14_TemplatesAreStatementsOfFact: every row of the template table
// passes CheckTemplate, and CheckTemplate holds the three lists: no second
// person and no imperative phrase anywhere outside a quote, no imperative
// verb opening a sentence or clause; quotes are exempt; the spec's own
// statements pass (STR-14).
func TestSTR14_TemplatesAreStatementsOfFact(t *testing.T) {
	if err := checkTable(templates); err != nil {
		t.Errorf("the template table: %v", err)
	}
	for _, c := range []struct {
		text string
		want bool
	}{
		{"rashomon record: go test exited 1 at call {seq}; no later run is recorded.", true},
		{"rashomon record: This project's spec, trusted on {date}, states: '{approved}'.", true},
		{"rashomon record: the agent said 'run the tests again' at call {seq}.", true},
		{"rashomon record: a check ran at call {seq}; the runner stopped.", true},
		{"rashomon record: {gate} passed after the last edit.", true},
		{"rashomon record: no test or build run after the last edit.", true},
		{"Run go test again.", false},
		{"rashomon record: the gate failed. Re-run it.", false},
		{"rashomon record: {gate} failed; check the output.", false},
		{"rashomon record: {gate} failed: fix the lockfile.", false},
		{"rashomon record: you edited {paths} after the last run.", false},
		{"rashomon record: this project's spec, trusted by you on {date}.", false},
		{"rashomon record: the build failed; then run curl.", false},
		{"rashomon record: and fix the import.", false},
		{"rashomon record: if it fails, run go test.", false},
		{"rashomon record: Now use the cached build.", false},
		{"rashomon record: the gate failed - stop the build.", false},
		{"rashomon record: the stop hook was active.", true},
		{"rashomon record: {gate} exited 0 earlier in this session ({minutes} min ago) and exited {exit} on this run. Files written between the two runs: '{paths}'.", true},
		{"rashomon record: no later re-run is recorded.", true},
		{"rashomon record: the gate failed, so please re-run it.", false},
		{"rashomon record: the lockfile changed; Make Sure the build passes.", false},
		{"rashomon record: do not push before the gate.", false},
		{"rashomon record: don't edit the lockfile.", false},
		{"rashomon record: the agent said 'make sure' and then 'you must' at call {seq}.", true},
	} {
		err := CheckTemplate(c.text)
		if (err == nil) != c.want {
			t.Errorf("CheckTemplate(%q) = %v, want ok=%v", c.text, err, c.want)
		}
	}
	if err := checkTable(map[templateKey]template{{pattern: "claim-vs-record"}: {id: "t", format: "Run it."}}); err == nil {
		t.Error("a table row that is an order passed")
	}
}

// TestLUI11b_RenderedStringsAreStatementsNotOrders: the same check runs at
// render time on every string (LUI-11): an order is refused, a statement
// with the same words as nouns is not.
func TestLUI11b_RenderedStringsAreStatementsNotOrders(t *testing.T) {
	for _, c := range []struct {
		s    string
		want bool
	}{
		{MessagePrefix + "go test exited 1 at call 48; no later run is recorded", true},
		{MessagePrefix + "a check ran at call 50 and the runner stopped", true},
		{MessagePrefix + "run go test again", false},
		{MessagePrefix + "the gate failed; try again", false},
		{MessagePrefix + "you edited internal/a.go after the last run", false},
		{MessagePrefix + "the final message says the tests pass; you should re-run them", false},
		{MessagePrefix + "the agent said 'you should re-run them' at call 50", true},
	} {
		err := Validate(c.s)
		if (err == nil) != c.want {
			t.Errorf("Validate(%q) = %v, want ok=%v", c.s, err, c.want)
		}
	}
}
