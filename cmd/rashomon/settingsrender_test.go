package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// editSettings parses its own output before it writes: bytes that are not JSON
// are refused, and the user's file is left as it was.
func TestEditSettings_RefusesToWriteInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	const seed = "{\"model\": \"opus\"}\n"
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	render := renderSettings
	renderSettings = func(*settings.Document) []byte { return []byte("{\"model\": \"opus\",\n{}") }
	t.Cleanup(func() { renderSettings = render })
	_, err := editSettings(path, func(*settings.Document) (bool, error) { return true, nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "left unchanged") {
		t.Fatalf("err = %v, want one saying settings.json was left unchanged", err)
	}
	if got, _ := os.ReadFile(path); string(got) != seed {
		t.Errorf("settings.json changed: %q", got)
	}
}
