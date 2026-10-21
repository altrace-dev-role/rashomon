package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// TestCodexCommand pins the command lines watch --harness codex installs. The
// order is the one H-145 replays the capture with, and the one the hook entry
// points read the flag from.
func TestCodexCommand(t *testing.T) {
	for event, want := range map[string]string{
		EventPreToolUse:   "/usr/local/bin/rashomon hook --harness codex --install " + oneID,
		EventPostToolUse:  "/usr/local/bin/rashomon post --harness codex --install " + oneID,
		EventSessionStart: "/usr/local/bin/rashomon probe start --harness codex --install " + oneID,
	} {
		if got := spec(oneID).CodexCommand(event); got != want {
			t.Errorf("%s: %q, want %q", event, got, want)
		}
	}
}

// TestCodexEventsLeaveOutWhatCodexCannotUse: no failure events (Codex has
// none), no SessionEnd (never fired, and capped under our timeout), and no
// recap entries (recap is silent under Codex).
func TestCodexEventsLeaveOutWhatCodexCannotUse(t *testing.T) {
	want := []string{EventPreToolUse, EventPostToolUse, EventSessionStart}
	if fmt.Sprint(CodexEvents) != fmt.Sprint(want) {
		t.Errorf("CodexEvents = %v, want %v", CodexEvents, want)
	}
}

// TestCodexOwner is TestOwner for Codex's file, plus the cross-harness cases:
// each harness's command line is not ours in the other's file.
func TestCodexOwner(t *testing.T) {
	cases := []struct {
		name  string
		owner func(json.RawMessage, string) string
		raw   string
		want  string
	}{
		{"ours", codex.owner, string(mustCodexEntry(t, spec(oneID), EventPreToolUse)), oneID},
		{"another install", codex.owner, string(mustCodexEntry(t, spec(otherID), EventPreToolUse)), otherID},
		{"a foreign group", codex.owner, foreignHook, ""},
		{"a Claude Code line in Codex's file", codex.owner, string(mustEntry(t, spec(oneID), EventPreToolUse)), ""},
		{"a Codex line in Claude Code's file", Owner, string(mustCodexEntry(t, spec(oneID), EventPreToolUse)), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.owner(json.RawMessage(c.raw), EventPreToolUse); got != c.want {
				t.Errorf("owner = %q, want %q", got, c.want)
			}
		})
	}
}

// TestApplyCodexIsANoOpTheSecondTime: Codex trusts an entry by its hash, so a
// watch that rewrote an unchanged entry would cost the user a trip to /hooks.
func TestApplyCodexIsANoOpTheSecondTime(t *testing.T) {
	doc := codexDocument(t, `{"description":"mine","hooks":{"PreToolUse":[`+foreignHook+`]}}`)
	changed, err := ApplyCodex(doc, spec(oneID))
	if err != nil || !changed {
		t.Fatalf("first ApplyCodex: changed %v, %v", changed, err)
	}
	first := string(doc.Bytes())
	for _, event := range CodexEvents {
		if ok, err := CodexPresent(doc, oneID, event); err != nil || !ok {
			t.Errorf("%s: present %v, %v after ApplyCodex", event, ok, err)
		}
	}
	for _, event := range []string{EventPostToolUseFailure, EventSessionEnd, EventStop, EventStopFailure, EventUserPromptSubmit} {
		if entries := entriesOf(t, doc, event); len(entries) != 0 {
			t.Errorf("%s: %d entries installed, want none", event, len(entries))
		}
	}

	changed, err = ApplyCodex(doc, spec(oneID))
	if err != nil || changed {
		t.Errorf("second ApplyCodex: changed %v, %v; want an unchanged document", changed, err)
	}
	if got := string(doc.Bytes()); got != first {
		t.Errorf("second ApplyCodex moved the file:\n%s\n%s", first, got)
	}
	if raw, ok := doc.Get("description"); !ok || string(raw) != `"mine"` {
		t.Errorf("the description reads back as %s", raw)
	}
	if !strings.Contains(first, foreignHook) {
		t.Errorf("the user's hook did not survive byte for byte:\n%s", first)
	}
}

// TestApplyCodexRefreshesAMovedBinary and refuses an entry someone edited, the
// two halves of Apply's contract, under Codex.
func TestApplyCodexRefreshesAMovedBinary(t *testing.T) {
	doc := codexDocument(t, `{}`)
	if _, err := ApplyCodex(doc, spec(oneID)); err != nil {
		t.Fatal(err)
	}
	moved := Spec{Executable: "/opt/rashomon/bin/rashomon", InstallID: oneID}
	changed, err := ApplyCodex(doc, moved)
	if err != nil || !changed {
		t.Fatalf("ApplyCodex after a move: changed %v, %v", changed, err)
	}
	for _, event := range CodexEvents {
		entries := entriesOf(t, doc, event)
		if len(entries) != 1 || !strings.Contains(string(entries[0]), moved.CodexCommand(event)) {
			t.Errorf("%s: %s, want one entry for the moved binary", event, entries)
		}
	}
}

func TestApplyCodexRefusesAnEditedEntry(t *testing.T) {
	narrowed := `{"matcher":"Bash","hooks":[{"type":"command","command":"` +
		spec(oneID).CodexCommand(EventPreToolUse) + `","timeout":5}]}`
	doc := codexDocument(t, `{"hooks":{"PreToolUse":[`+narrowed+`]}}`)
	before := string(doc.Bytes())
	_, err := ApplyCodex(doc, spec(oneID))
	var modified *ErrModified
	if !errors.As(err, &modified) || !strings.Contains(modified.Detail, "matcher") {
		t.Fatalf("ApplyCodex returned %v, want a refusal naming the matcher", err)
	}
	if got := string(doc.Bytes()); got != before {
		t.Errorf("a refusing ApplyCodex changed the document:\n%s", got)
	}
}

// TestRemoveCodex covers detach --harness codex: the named install's entries
// go, and the other install's, the foreign hook, a Claude Code rashomon line
// and the description stay as found. An event emptied loses its key.
func TestRemoveCodex(t *testing.T) {
	claude := string(mustEntry(t, spec(oneID), EventPreToolUse))
	doc := codexDocument(t, `{"description":"mine","hooks":{"PreToolUse":[`+foreignHook+`,`+claude+`]}}`)
	for _, s := range []Spec{spec(oneID), spec(otherID)} {
		if _, err := ApplyCodex(doc, s); err != nil {
			t.Fatal(err)
		}
	}

	n, _, err := RemoveCodex(doc, spec(oneID), false)
	if err != nil || n != len(CodexEvents) {
		t.Fatalf("RemoveCodex removed %d, %v; want %d", n, err, len(CodexEvents))
	}
	if got := entriesOf(t, doc, EventPreToolUse); len(got) != 3 {
		t.Errorf("PreToolUse holds %d entries, want the foreign hook, the Claude Code line and the other install", len(got))
	}
	if others, _ := CodexForeignOwners(doc, oneID); fmt.Sprint(others) != fmt.Sprint([]string{otherID}) {
		t.Errorf("CodexForeignOwners = %v, want [%s]", others, otherID)
	}

	n, _, err = RemoveCodexIf(doc, func(string) bool { return true }, false)
	if err != nil || n != len(CodexEvents) {
		t.Fatalf("RemoveCodexIf removed %d, %v; want %d", n, err, len(CodexEvents))
	}
	if installed, _ := CodexInstalled(doc); installed {
		t.Error("CodexInstalled after removing every install")
	}
	want := `{"description":"mine","hooks":{"PreToolUse":[` + foreignHook + `,` + claude + `]}}`
	if got := compact(t, doc.Bytes()); got != compact(t, []byte(want)) {
		t.Errorf("after removing every install:\n%s\nwant\n%s", got, want)
	}
}

func codexDocument(t *testing.T, in string) *settings.Document {
	t.Helper()
	doc, err := settings.Parse([]byte(in))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return doc
}

func mustCodexEntry(t *testing.T, s Spec, event string) json.RawMessage {
	t.Helper()
	raw, err := codex.entry(s, event)
	if err != nil {
		t.Fatalf("rendering the %s entry: %v", event, err)
	}
	return raw
}

func compact(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%v:\n%s", err, b)
	}
	out, _ := json.Marshal(v)
	return string(out)
}
