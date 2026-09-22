package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
)

// The keys a plugin hook entry may carry, and the subcommands it may run.
//
// Closed, like the record key sets in h13: a hook entry is the one place
// outside Go source where what runs on every tool call is decided, and none of
// this repository's import checks can see it. Claude Code runs a hook of type
// "prompt" or "agent" by asking a model, so a one-line edit here would put a
// model in the path of every call for every user who enabled the plugin --
// against the README's "no model in any path" -- with every Go test green.
var (
	pluginHookKeys    = map[string]bool{"type": true, "command": true, "args": true, "timeout": true}
	recorderSubcommds = map[string]bool{"hook": true, "post": true, "probe": true}
)

// TestH71_PluginHooksRunOnlyTheRecorder: every hook the plugin declares is a
// command, runs the plugin's own rashomon binary, and runs one of its hook
// subcommands -- nothing else, and no key that could carry a prompt.
func TestH71_PluginHooksRunOnlyTheRecorder(t *testing.T) {
	events := pluginHookEvents(t, func(event string, h map[string]any) {
		for k := range h {
			if !pluginHookKeys[k] {
				t.Errorf("%s: a plugin hook carries %q, a key this test does not know; "+
					"it may be one Claude Code acts on", event, k)
			}
		}
		if h["type"] != "command" {
			t.Errorf("%s: a plugin hook has type %v, want \"command\" -- any other type "+
				"is run by a model", event, h["type"])
		}
		if h["command"] != "${CLAUDE_PLUGIN_ROOT}/bin/rashomon" {
			t.Errorf("%s: a plugin hook runs %v, want the plugin's own rashomon binary", event, h["command"])
		}
		args, _ := h["args"].([]any)
		var sub string
		if len(args) > 0 {
			sub, _ = args[0].(string)
		}
		if !recorderSubcommds[sub] {
			t.Errorf("%s: a plugin hook runs rashomon with %v, want one of its hook subcommands", event, h["args"])
		}
	})
	if len(events) == 0 {
		t.Fatal("found no hooks in the plugin's hooks.json; the walk is not finding them")
	}
}

// TestH71_BothOriginsDeclareTheSameEvents: the plugin and `watch` hook the same
// events. If one origin records an event the other does not, the same session
// produces different evidence depending on how it was installed, and nothing
// in either report says so.
func TestH71_BothOriginsDeclareTheSameEvents(t *testing.T) {
	plugin := pluginHookEvents(t, func(string, map[string]any) {})
	watch := append([]string(nil), install.Events...)
	sort.Strings(watch)
	if strings.Join(plugin, ",") != strings.Join(watch, ",") {
		t.Errorf("the plugin declares %v and watch installs %v", plugin, watch)
	}
}

// TestH71_WatchInstallsOnlyCommands: the settings entries watch writes are
// commands too. The same model-in-the-path edit is one field away there.
func TestH71_WatchInstallsOnlyCommands(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	body, err := os.ReadFile(e.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	ours := 0
	for event, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				// Ours by the ownership marker, the way install itself tells.
				cmd, _ := h["command"].(string)
				if !strings.Contains(cmd, " "+install.Marker+" ") {
					continue
				}
				ours++
				if h["type"] != "command" {
					t.Errorf("%s: watch installed type %v, want \"command\"", event, h["type"])
				}
			}
		}
	}
	if ours != len(install.Events) {
		t.Fatalf("found %d of our entries after watch, want %d; the walk is not finding them", ours, len(install.Events))
	}
}

// pluginHookEvents walks plugin/hooks/hooks.json, calls visit on every hook
// object, and returns the sorted events that declare at least one.
func pluginHookEvents(t *testing.T, visit func(event string, hook map[string]any)) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, "plugin", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	var events []string
	for event, groups := range doc.Hooks {
		n := 0
		for _, g := range groups {
			for _, h := range g.Hooks {
				visit(event, h)
				n++
			}
		}
		if n > 0 {
			events = append(events, event)
		}
	}
	sort.Strings(events)
	return events
}
