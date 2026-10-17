package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shippedHooks is the hooks.json this repository ships in its plugin, read
// from the source tree rather than restated here, so these tests judge the
// file users receive.
const shippedHooks = "../../plugin/hooks/hooks.json"

// pluginRoot builds a plugin tree in a temporary directory: the given
// hooks.json and a stub bin/rashomon, which is all pluginDeclaresEvent reads.
func pluginRoot(t *testing.T, hooks []byte) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range map[string][]byte{
		filepath.Join(root, "hooks", "hooks.json"):    hooks,
		filepath.Join(root, pluginBinDir, "rashomon"): []byte("stub"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readShippedHooks(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(shippedHooks)
	if err != nil {
		t.Fatalf("reading the shipped plugin hooks: %v", err)
	}
	return b
}

// TestRubricHLD1_ShippedHooksDeclareEveryEvent: the hooks.json the plugin
// ships fires our command on every call, under the same coverageIntact
// check a settings entry is held to for coverage. A shipped matcher of
// "Bash" fails here, not in a user's report.
func TestRubricHLD1_ShippedHooksDeclareEveryEvent(t *testing.T) {
	root := pluginRoot(t, readShippedHooks(t))
	for _, event := range Events() {
		ok, err := pluginDeclaresEvent(root, event)
		if err != nil {
			t.Fatalf("%s: %v", event, err)
		}
		if !ok {
			t.Errorf("the shipped hooks.json does not declare %s as watch installs it", event)
		}
	}
}

// TestRubricHLD1_NarrowedPluginEntryIsNotDeclared: a plugin entry whose
// matcher narrows which calls it fires on -- "Bash" on a tool event, any
// value at all on an event with no matcher to narrow -- does not count as
// declaring the event whole. A narrowed PreToolUse matcher means Edit,
// Write and every MCP call go unrecorded, and reading that as
// present_plugin would let the run render verified over them.
func TestRubricHLD1_NarrowedPluginEntryIsNotDeclared(t *testing.T) {
	cases := []struct {
		name  string
		event string
		field string
		value any
	}{
		{"PreToolUse matcher Bash", EventPreToolUse, "matcher", "Bash"},
		{"PostToolUseFailure matcher Bash", EventPostToolUseFailure, "matcher", "Bash"},
		{"SessionStart given a matcher", EventSessionStart, "matcher", "startup"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := pluginRoot(t, editShipped(t, c.event, c.field, c.value))
			ok, err := pluginDeclaresEvent(root, c.event)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				t.Errorf("a %s entry with %s=%v reads as declared; it must read absent", c.event, c.field, c.value)
			}
			// Every other event is untouched and must still read declared:
			// the check is per group, not a verdict on the whole file.
			for _, other := range Events() {
				if other == c.event {
					continue
				}
				if ok, err := pluginDeclaresEvent(root, other); err != nil || !ok {
					t.Errorf("editing %s cost %s its declaration (ok=%v err=%v)", c.event, other, ok, err)
				}
			}
		})
	}
}

// TestRubricHLD1_TimeoutMatcherEquivalenceAndExtraHooksDoNotCostCoverage:
// a timeout different from the one watch installs, a matcher spelled nil
// or "" rather than the literal "*", and a second hook added alongside
// ours all still read declared AND whole for coverage, because none of
// them changes whether our command fires on every call -- a different
// timeout changes how long Claude Code waits for it, an equivalent matcher
// spelling is still match-all, and the other hook is something else
// running beside ours. Before coverageIntact existed, all three read
// absent beside a session that had in fact recorded cleanly.
func TestRubricHLD1_TimeoutMatcherEquivalenceAndExtraHooksDoNotCostCoverage(t *testing.T) {
	cases := []struct {
		name  string
		event string
		field string
		value any
	}{
		{"PreToolUse timeout 600", EventPreToolUse, "timeout", 600},
		{"Stop timeout 600", EventStop, "timeout", 600},
		{"PreToolUse matcher removed", EventPreToolUse, "matcher", nil},
		{"PreToolUse matcher empty", EventPreToolUse, "matcher", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := pluginRoot(t, editShipped(t, c.event, c.field, c.value))
			declared, whole, err := pluginDeclaration(root, c.event)
			if err != nil {
				t.Fatal(err)
			}
			if !declared || !whole {
				t.Errorf("%s with %s=%v: declared=%v whole=%v, want both true", c.event, c.field, c.value, declared, whole)
			}
		})
	}

	t.Run("an extra hook in the group", func(t *testing.T) {
		var doc map[string]any
		if err := json.Unmarshal(readShippedHooks(t), &doc); err != nil {
			t.Fatal(err)
		}
		group := doc["hooks"].(map[string]any)[EventPreToolUse].([]any)[0].(map[string]any)
		group["hooks"] = append(group["hooks"].([]any), map[string]any{
			"type": "command", "command": "a-linter", "timeout": 5,
		})
		out, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		root := pluginRoot(t, out)
		declared, whole, err := pluginDeclaration(root, EventPreToolUse)
		if err != nil {
			t.Fatal(err)
		}
		if !declared || !whole {
			t.Errorf("PreToolUse with an extra hook in the group: declared=%v whole=%v, want both true", declared, whole)
		}
	})
}

// TestRubricHLD1_HandEditedPluginEntryStillDeclares: a group whose matcher
// narrows it is not the entry watch installs, but Claude Code still runs it
// on the calls that matcher does match, so it still counts as declaring
// the event. That second answer is the one watch's refusal and status's
// overlap need: a narrowed plugin entry records every call it fires on,
// twice beside a settings install.
func TestRubricHLD1_HandEditedPluginEntryStillDeclares(t *testing.T) {
	cases := []struct {
		name  string
		event string
		field string
		value any
	}{
		{"PreToolUse matcher Bash", EventPreToolUse, "matcher", "Bash"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := pluginRoot(t, editShipped(t, c.event, c.field, c.value))
			declared, whole, err := pluginDeclaration(root, c.event)
			if err != nil {
				t.Fatal(err)
			}
			if !declared || whole {
				t.Errorf("%s with %s=%v: declared=%v intact=%v, want declared=true intact=false",
					c.event, c.field, c.value, declared, whole)
			}
		})
	}
	// A group whose command does not resolve under the plugin's own bin/ is
	// not ours, edited or not.
	t.Run("command outside the plugin", func(t *testing.T) {
		body := strings.ReplaceAll(string(readShippedHooks(t)), pluginRootVar+"/bin/", "/nonexistent/bin/")
		root := pluginRoot(t, []byte(body))
		declared, whole, err := pluginDeclaration(root, EventPreToolUse)
		if err != nil || declared || whole {
			t.Errorf("declared=%v intact=%v err=%v, want false false nil", declared, whole, err)
		}
	})
}

// editShipped returns the shipped hooks.json with one field of one event's
// group changed. A nil value removes the field. timeout lives on the hook,
// matcher on the group, as in the file itself.
func editShipped(t *testing.T, event, field string, value any) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(readShippedHooks(t), &doc); err != nil {
		t.Fatal(err)
	}
	group := doc["hooks"].(map[string]any)[event].([]any)[0].(map[string]any)
	target := group
	if field == "timeout" {
		target = group["hooks"].([]any)[0].(map[string]any)
	}
	if value == nil {
		delete(target, field)
	} else {
		target[field] = value
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestRubricNAMING11_EventsCannotBeChangedByACaller: the event list decides
// what watch writes and what detach removes, so a caller that edits the slice
// it was handed must not change what the next caller gets.
func TestRubricNAMING11_EventsCannotBeChangedByACaller(t *testing.T) {
	got := Events()
	want := append([]string(nil), got...)
	got[0] = "Narrowed"
	if extended := append(got[:1], "Extra"); len(extended) != 2 {
		t.Fatalf("append produced %d events, want 2", len(extended))
	}
	if again := Events(); strings.Join(again, ",") != strings.Join(want, ",") {
		t.Errorf("Events() after a caller edited its copy = %v, want %v", again, want)
	}
}
