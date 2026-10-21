package hook

import "testing"

// TestHarnessArg: no flag is Claude Code, and anything the build does not
// know -- a value it has never heard of, or the flag with nothing after it --
// is refused rather than read as Claude Code, because a payload read under
// the wrong harness's rules records a wrong fact.
func TestHarnessArg(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want Harness
		ok   bool
	}{
		{nil, ClaudeCode, true},
		{[]string{"--install", "abc"}, ClaudeCode, true},
		{[]string{"--harness", "codex"}, Codex, true},
		{[]string{"start", "--harness", "codex", "--install", "abc"}, Codex, true},
		{[]string{"--harness", "cursor"}, "", false},
		{[]string{"--harness", "Codex"}, "", false},
		{[]string{"--install", "abc", "--harness"}, "", false},
	} {
		got, ok := HarnessArg(tc.args)
		if got != tc.want || ok != tc.ok {
			t.Errorf("HarnessArg(%q) = %q, %v; want %q, %v", tc.args, got, ok, tc.want, tc.ok)
		}
	}
}
