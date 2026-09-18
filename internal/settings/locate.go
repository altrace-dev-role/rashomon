package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Layer names, highest precedence first. This is Claude Code's own order:
// managed policy beats everything, then the project's local file, then the
// project's shared file, then the user's file.
const (
	LayerManaged = "managed"
	LayerLocal   = "local"
	LayerProject = "project"
	LayerUser    = "user"
)

// Locations are the settings files Claude Code consults. An empty path means
// that layer is not consulted.
type Locations struct {
	Managed string
	User    string
	Project string
	Local   string
}

// UserPath is the file watch writes: settings.json under the user's Claude
// configuration directory. It is named here and nowhere else.
//
// CLAUDE_CONFIG_DIR is Claude Code's own relocation mechanism, not a guess: when
// it is set, Claude Code reads settings from there and not from ~/.claude, so
// an entry written to ~/.claude would never run.
//
// This is the only settings file this program ever writes. Project files are
// committed to repositories; a hook entry there would ship to everyone who
// clones the repo, pointing at a binary on one person's machine.
func UserPath() (string, error) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "settings.json"), nil
}

// DefaultLocations resolves every layer for a working directory.
func DefaultLocations(cwd string) (Locations, error) {
	user, err := UserPath()
	if err != nil {
		return Locations{}, err
	}
	return Locations{
		Managed: managedPath(),
		User:    user,
		Project: filepath.Join(cwd, ".claude", "settings.json"),
		Local:   filepath.Join(cwd, ".claude", "settings.local.json"),
	}, nil
}

// managedPath is where enterprise policy lives on each platform.
//
// RASHOMON_MANAGED_SETTINGS_PATH exists so the refusal path can be tested
// against a policy file that is not really installed on the machine. Claude
// Code does not read this variable, so pointing it at a permissive file cannot
// make a hook run that policy has disabled; it can only make watch install an
// entry that will then never fire.
func managedPath() string {
	if v := os.Getenv("RASHOMON_MANAGED_SETTINGS_PATH"); v != "" {
		return v
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode/managed-settings.json"
	case "windows":
		return `C:\ProgramData\ClaudeCode\managed-settings.json`
	default:
		return "/etc/claude-code/managed-settings.json"
	}
}

// Decision is the resolved value of disableAllHooks and which layer set it.
type Decision struct {
	Disabled bool
	// Layer is empty when no layer sets the key, in which case hooks run.
	Layer string
}

// HooksDisabled resolves disableAllHooks across the layers in precedence
// order. The first layer that sets the key decides.
//
// Both directions matter. Managed true over project false must refuse, because
// managed wins. User true over project false must not refuse, because the
// project file outranks the user file and hooks do run in that arrangement. A
// resolver that only read the user file would get the second case wrong, and
// that is the false positive that makes the tool refuse to install on a
// machine where it would have worked.
func HooksDisabled(loc Locations) (Decision, error) {
	layers := []struct{ name, path string }{
		{LayerManaged, loc.Managed},
		{LayerLocal, loc.Local},
		{LayerProject, loc.Project},
		{LayerUser, loc.User},
	}
	for _, l := range layers {
		if l.path == "" {
			continue
		}
		doc, err := Load(l.path)
		if err != nil {
			return Decision{}, fmt.Errorf("%s settings at %s: %w", l.name, l.path, err)
		}
		raw, ok := doc.Get("disableAllHooks")
		if !ok {
			continue
		}
		var disabled bool
		if err := json.Unmarshal(raw, &disabled); err != nil {
			return Decision{}, fmt.Errorf("%s settings at %s: disableAllHooks is not a boolean", l.name, l.path)
		}
		return Decision{Disabled: disabled, Layer: l.name}, nil
	}
	return Decision{}, nil
}
