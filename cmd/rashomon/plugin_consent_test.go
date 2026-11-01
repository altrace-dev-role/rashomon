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
// lines name /rashomon:report last and the plugin's launcher beside the
// running binary, with home as ~ when the path needs no quoting, the whole
// path quoted when it does. Under a 40-character home, at the plugin cache's
// default place, every line is 80 columns or fewer. Break: name a bare
// `rashomon report`, or let a quoted path take a ~, and a row fails.
func TestPC1b_TheFirstSessionLinesNameThePluginsOwnBinary(t *testing.T) {
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
			if len(lines) != 3 || lines[0] != disclose.PluginOnLine || !strings.Contains(lines[1], brief.PluginReport) || lines[2] != tc.want {
				t.Fatalf("pluginLead(%q) = %q; want the on line, %s, then %q", tc.exe, lines, brief.PluginReport, tc.want)
			}
			for i, l := range lines {
				if n := utf8.RuneCountInString(l); n > 80 && (tc.fits || i < 2) {
					t.Errorf("line %d is %d columns: %q", i+1, n, l)
				}
			}
		})
	}
}
