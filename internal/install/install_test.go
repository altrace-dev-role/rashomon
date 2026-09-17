package install

import "testing"

// TestShellQuote covers the three shapes an executable path reaches the
// settings file in. Claude Code hands the command line to a shell, so a path
// that is not quoted where it needs to be installs a hook that parses, reads
// plausibly and never runs.
func TestShellQuote(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a path of safe characters is left bare",
			"/usr/local/bin/attest", "/usr/local/bin/attest"},
		{"a space is quoted",
			"/Users/sam/Application Support/attest", "'/Users/sam/Application Support/attest'"},
		{"a single quote is closed, escaped and reopened",
			"/home/o'brien/bin/attest", `'/home/o'\''brien/bin/attest'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellQuote(tc.in); got != tc.want {
				t.Errorf("shellQuote(%q) is %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
