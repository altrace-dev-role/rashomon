package install

import (
	"encoding/json"
	"os"
	"path/filepath"
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
// ships declares every event watch installs, as watch installs it -- the
// same matcher and the same timeout, under the same intact() check a
// settings entry is held to. A shipped timeout of 600 or a matcher of "Bash"
// fails here, not in a user's report.
func TestRubricHLD1_ShippedHooksDeclareEveryEvent(t *testing.T) {
	root := pluginRoot(t, readShippedHooks(t))
	for _, event := range Events {
		ok, err := pluginDeclaresEvent(root, event)
		if err != nil {
			t.Fatalf("%s: %v", event, err)
		}
		if !ok {
			t.Errorf("the shipped hooks.json does not declare %s as watch installs it", event)
		}
	}
}

// TestRubricHLD1_NarrowedPluginEntryIsNotDeclared: a plugin entry edited the
// way a settings entry would be refused for -- a matcher narrowed to "Bash",
// a timeout of 600 -- does not count as declaring the event. A narrowed
// PreToolUse matcher means Edit, Write and every MCP call go unrecorded, and
// reading that as present_plugin would let the run render verified over them.
func TestRubricHLD1_NarrowedPluginEntryIsNotDeclared(t *testing.T) {
	cases := []struct {
		name  string
		event string
		field string
		value any
	}{
		{"PreToolUse matcher Bash", EventPreToolUse, "matcher", "Bash"},
		{"PostToolUseFailure matcher Bash", EventPostToolUseFailure, "matcher", "Bash"},
		{"PreToolUse matcher removed", EventPreToolUse, "matcher", nil},
		{"SessionStart given a matcher", EventSessionStart, "matcher", "startup"},
		{"PreToolUse timeout 600", EventPreToolUse, "timeout", 600},
		{"Stop timeout 600", EventStop, "timeout", 600},
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
			for _, other := range Events {
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
