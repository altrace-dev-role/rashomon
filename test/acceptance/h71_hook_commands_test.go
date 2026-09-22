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

// The keys plugin/.claude-plugin/plugin.json may carry: exactly the ones it has
// today.
//
// hooks.json is not the only place a plugin declares hooks. The manifest's own
// `hooks` field takes inline hook configuration or paths to more hook files,
// and Claude Code loads those alongside hooks/hooks.json -- so a prompt hook
// added there puts a model in the path of every call while the walk above,
// which reads hooks.json only, stays green. That was measured, not supposed: a
// PreToolUse prompt hook in the manifest passed every H-71 test. Forbidding
// just "hooks" would miss the next manifest field that points at runnable
// configuration (the reference also lists paths for commands, agents, MCP
// servers), so the set is closed, like pluginHookKeys: a key this test has not
// seen fails until someone decides here what it does.
var pluginManifestKeys = map[string]bool{
	"name": true, "version": true, "description": true, "author": true,
	"homepage": true, "repository": true, "license": true, "defaultEnabled": true,
}

// TestH71_ManifestDeclaresNoHooks: the manifest carries no key outside the set
// above, and in particular no `hooks`.
func TestH71_ManifestDeclaresNoHooks(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(moduleRoot, "plugin", ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc) == 0 {
		t.Fatal("found no keys in plugin.json; the read is not finding them")
	}
	for k := range doc {
		switch {
		case k == "hooks":
			t.Errorf("plugin.json declares hooks; Claude Code runs them beside hooks/hooks.json, " +
				"where the H-71 walk does not look, and a prompt or agent hook there is run by a model")
		case !pluginManifestKeys[k]:
			t.Errorf("plugin.json carries %q, a key this test does not know; "+
				"it may point Claude Code at something it runs", k)
		}
	}
}

// TestH71_MarkdownDeclaresNoHooks: no markdown file the plugin ships declares
// hooks in its frontmatter.
//
// A skill's frontmatter may carry a `hooks` key (so may an agent's), and those
// hooks run while the skill is active -- narrower than every call, but still a
// model-run hook the walk above cannot see. Every .md under plugin/ is read,
// not only skills/*/SKILL.md, because commands/ and agents/ are discovered by
// location with no manifest entry to catch. There is no YAML parser in this
// module, so the frontmatter is read line by line and fails closed: a
// top-level line that is not a plain `key:` (a quoted key, a flow mapping, a
// `?` complex key -- all ways YAML can spell `hooks`) fails as unreadable
// rather than being skipped.
func TestH71_MarkdownDeclaresNoHooks(t *testing.T) {
	root := filepath.Join(moduleRoot, "plugin")
	skills := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		if filepath.Base(path) == "SKILL.md" {
			skills++
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, k := range frontmatterKeys(t, rel, string(body)) {
			if k == "hooks" {
				t.Errorf("%s declares hooks in its frontmatter; they run while it is active, "+
					"where the H-71 walk does not look", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if skills == 0 {
		t.Fatal("found no SKILL.md under plugin/; the walk is not finding them")
	}
}

// frontmatterKeys returns the top-level keys of body's YAML frontmatter, or
// none if it has no frontmatter. A top-level line it cannot read as a plain
// key fails the test instead of being skipped.
func frontmatterKeys(t *testing.T, name, body string) []string {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	var keys []string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return keys
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") ||
			strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		k, _, ok := strings.Cut(line, ":")
		if !ok || k == "" || strings.ContainsAny(k, " \t\"'{}[]?,&*!|>%@`") {
			t.Errorf("%s: frontmatter line %q is not a plain key this test can read; "+
				"it could spell hooks", name, line)
			continue
		}
		keys = append(keys, k)
	}
	t.Errorf("%s: frontmatter opens with --- and never closes; this test cannot tell where it ends", name)
	return keys
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
