package install

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// PluginName is the name our own plugin manifest declares. A plugin's
// identity is its name plus the fact that its own directory carries a real
// recorder binary -- there is no install id to check, because a manifest
// shipped to every user cannot carry a per-machine secret.
const PluginName = "rashomon"

// pluginBinDir is the directory a plugin's exec-form hook commands resolve
// into, relative to CLAUDE_PLUGIN_ROOT. It doubles as the evidence that a
// resolved command belongs to the plugin and not to some unrelated path the
// manifest happened to name.
const pluginBinDir = "bin"

// pluginRootVar is the variable Claude Code substitutes into a plugin's hook
// commands. This package never asks Claude Code to do that substitution --
// it does the same replacement itself, against a root it already has from
// either self-identification or installed_plugins.json.
const pluginRootVar = "${CLAUDE_PLUGIN_ROOT}"

// PluginPresent reports whether THIS PROCESS is running as the rashomon
// plugin's own hook binary for event: evidence from where this process runs,
// not a reading of configuration -- and evidence, not proof.
//
// It answers a narrower and stronger question than "is a plugin enabled":
// there is no documented guarantee that writing enabledPlugins reaches an
// already-running session before its next hook invocation -- Claude Code
// documents /reload-plugins for the class of change that includes hooks, and
// this program's own spec names the mid-session case undocumented. Reading
// enabledPlugins off disk would therefore prove only "the file currently says
// enabled", never "the hook set this session is running right now includes
// it". Self-identification closes that gap a different way: Claude Code does
// not execute a binary from inside a disabled plugin's directory, so when
// Claude Code is what started a process running from one, that plugin's entry
// was invoked for this session a moment ago. What this function sees is only
// where the process runs, not who started it: the same binary run by hand
// from <root>/bin gets the same answer with no entry invoked and no session
// involved. So a true result is the strongest evidence available here, not
// the invocation itself.
//
// This is why PluginPresent takes no *settings.Document and does not read
// one: it has nothing to do with what settings.json currently says. Compare
// FindPlugin, which exists for callers -- watch's refusal, status's report --
// that are not themselves running as the plugin and have no such evidence
// available; those callers get a disk-based answer because a disk-based
// answer, honestly labelled as one, is the best they can do.
func PluginPresent(event string) (bool, error) {
	root, err := selfPluginRoot()
	if err != nil || root == "" {
		return false, err
	}
	return pluginDeclaresEvent(root, event)
}

// selfPluginRoot returns the root of the rashomon plugin running this
// process, or "" if this process is not running from inside one.
//
// A plugin's exec-form commands are "${CLAUDE_PLUGIN_ROOT}/bin/<name>" (see
// the manifest under plugin/hooks/hooks.json), so the executable's own
// directory name is the tell: a `watch` install or a developer's `go run`
// does not sit under a directory literally named "bin" whose parent declares
// itself as our plugin. Running the plugin's own binary by hand does, and
// reads as the plugin -- which is why this is evidence, not proof.
func selfPluginRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) != pluginBinDir {
		return "", nil
	}
	root := filepath.Dir(dir)
	name, err := readPluginName(root)
	if err != nil {
		// Absent or unparsable manifest: not a plugin we recognise, which is
		// a normal outcome and not a failure worth reporting up. A read error
		// on OUR OWN plugin's manifest, while we are demonstrably running as
		// its binary, would be a strange machine to be honest about either;
		// treating it as "not present" rather than surfacing the error keeps
		// this function's contract simple, and Resolve already falls back to
		// "unknown" when nothing resolves.
		return "", nil
	}
	if name != PluginName {
		return "", nil
	}
	return root, nil
}

// readPluginName reads the name field out of a plugin's own manifest.
func readPluginName(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		return "", err
	}
	var m struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	return m.Name, nil
}

// pluginDefaultEnabled reads a plugin's own defaultEnabled field, defaulting
// to true when the manifest omits it -- the same default Claude Code applies,
// so a plugin nobody has toggled reads the same way here as it does there.
func pluginDefaultEnabled(root string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json"))
	if err != nil {
		return false, err
	}
	m := struct {
		DefaultEnabled *bool `json:"defaultEnabled"`
	}{}
	if err := json.Unmarshal(data, &m); err != nil {
		return false, err
	}
	if m.DefaultEnabled == nil {
		return true, nil
	}
	return *m.DefaultEnabled, nil
}

// pluginHooksDoc is hooks/hooks.json's own wrapper shape -- the plugin format,
// not the settings format Document/matcherGroup read.
type pluginHooksDoc struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// pluginDeclaresEvent reports whether the plugin rooted at root declares, for
// event, an exec-form command hook whose argument vector matches our own
// subcommand contract and whose command resolves, under root, to a file that
// exists.
//
// The resolution check is not redundant with the caller having found root in
// the first place: root can come from self-identification (which only proves
// ONE event's command exists, the one that was just invoked) or from
// installed_plugins.json (which proves nothing about the filesystem at all).
// Either way, this is the one place that reads the plugin's own declaration
// for the SPECIFIC event being asked about.
func pluginDeclaresEvent(root, event string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var doc pluginHooksDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}

	want := strings.Fields(subcommand(event))
	for _, group := range doc.Hooks[event] {
		for _, h := range group.Hooks {
			if h.Type != "command" || !equalStrings(h.Args, want) {
				continue
			}
			resolved := strings.ReplaceAll(h.Command, pluginRootVar, root)
			if !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
				// Declares an event of ours but points outside its own tree --
				// not evidence of anything this plugin shipped.
				continue
			}
			if info, err := os.Stat(resolved); err == nil && info.Mode().IsRegular() {
				return true, nil
			}
		}
	}
	return false, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Plugin describes one rashomon plugin installation as found on disk, for
// callers that are not themselves running as it: watch's duplicate-install
// refusal and status's report. Unlike PluginPresent, this is a read of
// configuration and says so -- Enabled is what the files currently claim, not
// a fact about the running session.
type Plugin struct {
	// Key is the plugin's identity as installed_plugins.json and
	// enabledPlugins both spell it: "rashomon@<source>".
	Key string
	// Root is the installPath installed_plugins.json recorded, which is also
	// what CLAUDE_PLUGIN_ROOT resolves to for a copied (marketplace) install.
	Root string
	// Enabled is the resolved enabledPlugins value for Key, falling back to
	// the manifest's defaultEnabled when settings names no override.
	Enabled bool
	// Events lists which of install.Events this installation structurally
	// declares with a command that resolves to a file under its own bin/.
	Events []string
}

// installedPluginsFile is installed_plugins.json's own shape. It is Claude
// Code's internal bookkeeping and undocumented as a format, but it is the
// only place on disk that maps a plugin's identity to where it actually
// lives, and Part 1 has no other way to find that path without asking Claude
// Code to resolve CLAUDE_PLUGIN_ROOT for us.
type installedPluginsFile struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
	} `json:"plugins"`
}

// FindPlugin looks for an installed rashomon plugin without relying on this
// process being one. doc is the already-loaded settings document, reused
// rather than re-read, because enabledPlugins lives in the same file the
// settings-origin check already loaded.
//
// It returns nil, nil when no rashomon plugin is installed at all -- the
// common case, and not an error. A caller that needs to know plugin-not-
// enabled from plugin-not-present distinguishes them via the returned
// Plugin's Enabled field.
//
// If more than one rashomon-named installation is recorded -- a marketplace
// copy alongside a synced one, say -- this returns the first one Go's map
// iteration happens to visit. Two simultaneous installations of this same
// plugin is not a case any acceptance item exercises, and choosing between
// them deterministically would need a preference this document does not
// state.
func FindPlugin(doc *settings.Document) (*Plugin, error) {
	dir, err := settings.ConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugins", "installed_plugins.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var installed installedPluginsFile
	if err := json.Unmarshal(data, &installed); err != nil {
		return nil, err
	}

	enabled := map[string]bool{}
	if doc != nil {
		if raw, ok := doc.Get("enabledPlugins"); ok {
			_ = json.Unmarshal(raw, &enabled)
		}
	}

	for key, records := range installed.Plugins {
		name, _, _ := strings.Cut(key, "@")
		if name != PluginName {
			continue
		}
		for _, rec := range records {
			if rec.InstallPath == "" {
				continue
			}
			p := &Plugin{Key: key, Root: rec.InstallPath}
			if on, explicit := enabled[key]; explicit {
				p.Enabled = on
			} else if def, err := pluginDefaultEnabled(rec.InstallPath); err == nil {
				p.Enabled = def
			}
			for _, event := range Events {
				if ok, err := pluginDeclaresEvent(rec.InstallPath, event); err == nil && ok {
					p.Events = append(p.Events, event)
				}
			}
			return p, nil
		}
	}
	return nil, nil
}
