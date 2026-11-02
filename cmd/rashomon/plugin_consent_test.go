package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/brief"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// TestPC1b_TheFirstSessionLinesNameThePluginsOwnBinary (PLUGIN-CONSENT1, the
// DE's addition 2): a plugin user has no rashomon on PATH (B287), so the
// lines name /rashomon:report and the plugin's launcher beside the running
// binary, with home as ~ when the path needs no quoting, the whole path
// quoted when it does, and then the full list of what is kept, the same
// launcher's consent --full (the DE's #228 verdict). Under a 40-character
// home, at the plugin cache's default place, every line is 80 columns or
// fewer. Break: name a bare `rashomon report`, let a quoted path take a ~, or
// drop the full list's line, and a row fails.
func TestPC1b_TheFirstSessionLinesNameThePluginsOwnBinary(t *testing.T) {
	const fullListLine = "Full list of what it keeps: the same, with consent --full in place of report"
	home := "/Users/" + strings.Repeat("h", 33)
	t.Setenv("HOME", home)
	cache := "/.claude/plugins/cache/rashomon/rashomon/1.2.0-alpha.1/bin"
	for _, tc := range []struct {
		name, exe, want string
		fits            bool
	}{
		{"the plugin cache under home", home + cache + "/rashomon-darwin-arm64", "  ~" + cache + "/rashomon report", true},
		{"outside home", "/opt/rashomon/bin/rashomon-linux-amd64", "  /opt/rashomon/bin/rashomon report", true},
		{"a path that needs quoting", home + "/my plugins/bin/rashomon-darwin-arm64", "  '" + home + "/my plugins/bin/rashomon' report", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := strings.Split(pluginLead(tc.exe), "\n")
			if len(lines) != 4 || lines[0] != disclose.PluginOnLine || !strings.Contains(lines[1], brief.PluginReport) ||
				lines[2] != tc.want || lines[3] != fullListLine {
				t.Fatalf("pluginLead(%q) = %q; want the on line, %s, %q, then %q", tc.exe, lines, brief.PluginReport, tc.want, fullListLine)
			}
			for i, l := range lines {
				if n := utf8.RuneCountInString(l); n > 80 && (tc.fits || i != 2) {
					t.Errorf("line %d is %d columns: %q", i+1, n, l)
				}
			}
		})
	}
}
