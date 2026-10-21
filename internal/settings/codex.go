package settings

import (
	"os"
	"path/filepath"
)

// CodexHome resolves the Codex CLI's configuration directory: CODEX_HOME when
// it is set, and ~/.codex otherwise.
//
// Codex's hooks page names only ~/.codex. The variable is Codex's own: the
// 0.163.0 CLI binary resolves it and carries the error "failed to resolve
// CODEX_HOME". It relocates the whole directory the way CLAUDE_CONFIG_DIR does
// for Claude Code; see ConfigDir.
func CodexHome() (string, error) {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

// CodexHooksPath is the file watch --harness codex writes: hooks.json under
// the Codex directory. It is named here and nowhere else.
//
// A repository's .codex/hooks.json is never written, for the reason UserPath
// gives about project settings: it ships to everyone who clones the repo. The
// file has the shape of a settings file's hooks member -- {"hooks": {event:
// [matcher groups]}} beside an optional "description" -- so Document reads
// and edits it under the same rules.
func CodexHooksPath() (string, error) {
	dir, err := CodexHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hooks.json"), nil
}
