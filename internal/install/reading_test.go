package install

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// stringLiterals collects every quoted string literal in this package's own
// non-test source, the same AST walk internal/recap/vocabulary_test.go uses
// and for the same reason: a plain substring grep over the raw file would
// also catch this very doc comment's mention of the forbidden value, which
// is prose ABOUT the rule and not a JSON value the binary could produce.
// Only a literal the compiler treats as data can end up in rendered JSON.
func stringLiterals(t *testing.T) []string {
	t.Helper()
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if text, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, text)
			}
			return true
		})
	}
	if len(out) < 5 {
		t.Fatalf("found only %d string literals; the walk is not reaching this package's source", len(out))
	}
	return out
}

func emptyDoc(t *testing.T) *settings.Document {
	t.Helper()
	doc, err := settings.Parse([]byte(`{}`))
	if err != nil {
		t.Fatalf("seeding an empty document: %v", err)
	}
	return doc
}

// TestReadingHookTypeIsNeverAgent is the mechanical half of the spec's own
// hard requirement -- "type": "agent" is FORBIDDEN, because a reader with
// Read, Grep and Glob is a reader injected text can aim at the filesystem.
// It reads the RENDERED entry, not the constant, so a future refactor that
// stops going through ReadingHookType is still caught.
func TestReadingHookTypeIsNeverAgent(t *testing.T) {
	raw, err := readingEntry()
	if err != nil {
		t.Fatalf("readingEntry: %v", err)
	}
	var g struct {
		Hooks []struct {
			Type string `json:"type"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("readingEntry did not render valid JSON: %v\n%s", err, raw)
	}
	if len(g.Hooks) != 1 {
		t.Fatalf("readingEntry rendered %d hooks, want 1", len(g.Hooks))
	}
	if g.Hooks[0].Type != "prompt" {
		t.Errorf("hook type = %q, want %q", g.Hooks[0].Type, "prompt")
	}
	if strings.Contains(string(raw), `"agent"`) {
		t.Errorf("the rendered entry contains the literal %q:\n%s", `"agent"`, raw)
	}
}

// TestReadingSourceNeverMentionsAgentType is the same guarantee from the
// other direction: even if a future edit stopped rendering through
// readingEntry, this file's own DATA (its string literals, never its prose
// comments -- see stringLiterals' doc) must never spell out the forbidden
// type as a JSON value. This is the spec's own H-98/H-99 acceptance
// language ("a test that greps the shipped hooks.json for '\"type\":
// \"agent\"' and fails") applied to this branch: there is no shipped
// hooks.json here (Part 1's plugin work is a separate, unmerged branch), so
// this scans the one file that plays that role instead.
func TestReadingSourceNeverMentionsAgentType(t *testing.T) {
	joined := strings.Join(stringLiterals(t), "")
	if strings.Contains(joined, `"type": "agent"`) || strings.Contains(joined, `"type":"agent"`) {
		t.Errorf("internal/install's own string data spells out a \"type\": \"agent\" hook")
	}
}

// TestApplyReadingDefaultOff is H-105's unit-level twin: a document nothing
// has touched carries no reading entry, and ReadingPresent says so.
func TestApplyReadingDefaultOff(t *testing.T) {
	doc := emptyDoc(t)
	present, err := ReadingPresent(doc)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("a fresh document reads as already carrying the model-reading entry")
	}
}

// TestApplyReadingEnableIsIdempotent: a second enable on top of the first
// changes nothing and leaves exactly one entry per event, the same shape
// Apply's own "found/eventChanged" guards against watch appending a second
// entry of its own (H-6).
func TestApplyReadingEnableIsIdempotent(t *testing.T) {
	doc := emptyDoc(t)

	changed, err := ApplyReading(doc, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first enable reported no change")
	}
	for _, event := range readingEvents {
		if n := len(entriesOf(t, doc, event)); n != 1 {
			t.Fatalf("under %s, %d entries after enabling, want 1", event, n)
		}
	}

	changed, err = ApplyReading(doc, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("second enable reported a change")
	}
	for _, event := range readingEvents {
		if n := len(entriesOf(t, doc, event)); n != 1 {
			t.Fatalf("under %s, %d entries after re-enabling, want 1 (a duplicate, not a no-op)", event, n)
		}
	}
}

// TestApplyReadingDisableRemovesIt is enable's undo.
func TestApplyReadingDisableRemovesIt(t *testing.T) {
	doc := emptyDoc(t)
	if _, err := ApplyReading(doc, true); err != nil {
		t.Fatal(err)
	}

	changed, err := ApplyReading(doc, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("disable reported no change over an enabled document")
	}
	present, err := ReadingPresent(doc)
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Error("the entry is still present after disabling it")
	}
	for _, event := range readingEvents {
		if n := len(entriesOf(t, doc, event)); n != 0 {
			t.Errorf("under %s, %d entries remain after disabling, want 0", event, n)
		}
	}
}

// TestApplyReadingDisableOnAnUntouchedDocumentIsANoOp mirrors watch's own
// no-op-writes-nothing discipline (H-18's family): disabling something that
// was never enabled must not report a change or touch the document.
func TestApplyReadingDisableOnAnUntouchedDocumentIsANoOp(t *testing.T) {
	doc := emptyDoc(t)
	before := string(doc.Bytes())

	changed, err := ApplyReading(doc, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("disabling an entry that was never present reported a change")
	}
	if got := string(doc.Bytes()); got != before {
		t.Errorf("a no-op ApplyReading still changed the document:\n%s", got)
	}
}

// TestApplyReadingNeverTouchesACommandEntry: a Stop entry already carries
// Part 4's own command hook (or, in this fixture, a stand-in for it) before
// enable-reading ever runs. Identity here is the marker inside the rendered
// prompt text, not position, so every entry that does not match it must
// survive both an enable and a disable untouched, in place.
func TestApplyReadingNeverTouchesACommandEntry(t *testing.T) {
	commandEntry := mustEntry(t, spec(oneID), EventStop)
	doc, err := settings.Parse([]byte(`{"hooks":{"Stop":[` + string(commandEntry) + `]}}`))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if _, err := ApplyReading(doc, true); err != nil {
		t.Fatal(err)
	}
	entries := entriesOf(t, doc, EventStop)
	if len(entries) != 2 {
		t.Fatalf("got %d entries under Stop after enabling, want 2 (the command entry and the reading entry)", len(entries))
	}
	foundCommand := false
	for _, e := range entries {
		if Owner(e, EventStop) == oneID {
			foundCommand = true
		}
	}
	if !foundCommand {
		t.Error("Part 4's command entry is gone after enabling the reading entry")
	}

	if _, err := ApplyReading(doc, false); err != nil {
		t.Fatal(err)
	}
	entries = entriesOf(t, doc, EventStop)
	if len(entries) != 1 || Owner(entries[0], EventStop) != oneID {
		t.Errorf("Part 4's command entry did not survive disabling the reading entry: %v", entries)
	}
}

// TestIsReadingEntryRejectsAForeignPromptHook: the marker, not the type
// alone, is what identity rests on. A THIRD PARTY's own "type": "prompt"
// hook must never be mistaken for ours and silently removed by disable, or
// silently treated as "already enabled" by enable.
func TestIsReadingEntryRejectsAForeignPromptHook(t *testing.T) {
	foreign := json.RawMessage(`{"hooks":[{"type":"prompt","prompt":"unrelated","timeout":30}]}`)
	if isReadingEntry(foreign) {
		t.Error("a foreign \"type\": \"prompt\" hook with no marker reads as ours")
	}
}
