package settings

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestCodexHome covers both ways the directory is found. Neither case may
// reach the real home directory of whoever runs the suite, so HOME is a
// throwaway in both.
func TestCodexHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("CODEX_HOME set", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "elsewhere")
		t.Setenv("CODEX_HOME", dir)
		got, err := CodexHooksPath()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(dir, "hooks.json"); got != want {
			t.Errorf("CodexHooksPath = %q, want %q", got, want)
		}
	})
	t.Run("CODEX_HOME unset", func(t *testing.T) {
		t.Setenv("CODEX_HOME", "")
		got, err := CodexHooksPath()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".codex", "hooks.json"); got != want {
			t.Errorf("CodexHooksPath = %q, want %q", got, want)
		}
	})
}

// TestCodexHooksFileRoundTrip edits one event of a Codex hooks.json and checks
// that the description and the other event come back as they were read.
func TestCodexHooksFileRoundTrip(t *testing.T) {
	const (
		description = `"hooks I keep"`
		stop        = `[ {"hooks":[{"type":"command","command":"notify-me"}]} ]`
	)
	in := "{\n  \"description\": " + description + ",\n  \"hooks\": {\n    \"Stop\": " + stop + "\n  }\n}\n"
	doc, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := doc.SetHookEntries("PreToolUse", []json.RawMessage{json.RawMessage(`{"hooks":[]}`)}); err != nil {
		t.Fatalf("SetHookEntries: %v", err)
	}
	out := string(doc.Bytes())
	for _, want := range []string{`"description": ` + description, `"Stop": ` + stop} {
		if !strings.Contains(out, want) {
			t.Errorf("%s is not in the output as it was read:\n%s", want, out)
		}
	}
}
