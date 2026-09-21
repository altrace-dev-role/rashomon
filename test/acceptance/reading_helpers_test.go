package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// readingPromptFromSettings reads back the ACTUAL prompt text Claude Code
// would see -- through settings.json, not through this program's own
// constant -- so a test here is checking what got written, not restating
// what the source says it meant to write.
func readingPromptFromSettings(t *testing.T, e *env) string {
	t.Helper()
	sf := e.settings()
	for _, event := range []string{"Stop", "StopFailure"} {
		for _, g := range sf.Hooks[event] {
			var loose struct {
				Hooks []struct {
					Type   string `json:"type"`
					Prompt string `json:"prompt"`
				} `json:"hooks"`
			}
			if json.Unmarshal(g.raw, &loose) != nil {
				continue
			}
			for _, h := range loose.Hooks {
				if h.Type == "prompt" && strings.Contains(h.Prompt, "rashomon:model-reading:v1") {
					return h.Prompt
				}
			}
		}
	}
	t.Fatal("no installed model-reading prompt found in settings.json")
	return ""
}

func mustContain(t *testing.T, haystack, needle, why string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s (looked for %q in):\n%s", why, needle, haystack)
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
