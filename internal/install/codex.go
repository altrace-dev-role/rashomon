package install

import (
	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// CodexEvents are the Codex hook events watch --harness codex installs into,
// in the order they are installed.
//
// Three, where Claude Code gets eight. Codex has no PostToolUseFailure and no
// StopFailure: a failed call fires a plain PostToolUse, which is why a Codex
// execution's outcome is left unobserved. SessionEnd is left out because it
// never fired in the capture (test/fixtures/codex), and because Codex caps
// that event's timeout at three seconds, under the five this program's
// entries are held to. Stop and UserPromptSubmit are left out because recap
// prints nothing under Codex: the entries would ask the user to trust two
// commands that do nothing, and put a process on every turn for it.
var CodexEvents = []string{
	EventPreToolUse,
	EventPostToolUse,
	EventSessionStart,
}

// CodexFlag is what a Codex command line carries between the subcommand and
// Marker. It is the flag the hook entry points read (hook.HarnessArg), in the
// order H-145 replays the capture with.
const CodexFlag = " --harness codex"

var codex = harness{events: CodexEvents, subcommand: codexSubcommand, flag: CodexFlag}

// codexSubcommand maps a Codex event to the subcommand Claude Code's entry for
// the same event runs. The payloads share their keys; the flag is what tells
// the recorder to read them under Codex's rules.
func codexSubcommand(event string) string {
	switch event {
	case EventPreToolUse:
		return "hook"
	case EventPostToolUse:
		return "post"
	case EventSessionStart:
		return "probe start"
	}
	return ""
}

// CodexCommand is the shell command line installed for a Codex event.
func (s Spec) CodexCommand(event string) string { return codex.command(s, event) }

// ApplyCodex is Apply for Codex's hooks.json, under the same rules: one entry
// of ours per event, refreshed if the binary moved, and an edited one refused.
//
// Codex trusts a hook against a hash of its definition, so an entry this
// rewrites is skipped until the user trusts it again. A no-op therefore has to
// write nothing, which Apply already guarantees.
func ApplyCodex(doc *settings.Document, spec Spec) (bool, error) { return codex.apply(doc, spec) }

// RemoveCodex is Remove for Codex's hooks.json.
func RemoveCodex(doc *settings.Document, spec Spec, force bool) (int, []Modified, error) {
	return RemoveCodexIf(doc, func(id string) bool { return id == spec.InstallID }, force)
}

// RemoveCodexIf is RemoveIf for Codex's hooks.json. A Claude Code command line
// in that file is not ours, so it is never offered to match.
func RemoveCodexIf(doc *settings.Document, match func(installID string) bool, force bool) (int, []Modified, error) {
	return codex.removeIf(doc, match, force)
}

// CodexPresent is Present for Codex's hooks.json. It says what the file holds
// and nothing about whether Codex runs it: Codex skips an entry until the user
// trusts it in /hooks, and that trust is not something this program reads.
// Coverage therefore does not call this.
func CodexPresent(doc *settings.Document, installID, event string) (bool, error) {
	return codex.present(doc, installID, event)
}

// CodexForeignOwners is ForeignOwners for Codex's hooks.json.
func CodexForeignOwners(doc *settings.Document, installID string) ([]string, error) {
	return codex.foreignOwners(doc, installID)
}

// CodexInstalled reports whether any rashomon Codex entry, of any install, is
// in the document.
func CodexInstalled(doc *settings.Document) (bool, error) {
	for _, event := range CodexEvents {
		entries, err := doc.HookEntries(event)
		if err != nil {
			return false, err
		}
		for _, e := range entries {
			if codex.owner(e, event) != "" {
				return true, nil
			}
		}
	}
	return false, nil
}
