package brief

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestGI1b_TheTestsLineNamesTheInferredCommandAndItsFile: with no spec test
// gate and no index, the tests line quotes the command gateinfer read and
// names the file, the CI's before a manifest's, within the line's width,
// and its source is that file. Controls: a spec's test gate at any trust
// wins, and a project with nothing to infer keeps today's line (BR1o holds
// the index's precedence). Break: leave testsFact's fallback unwired.
func TestGI1b_TheTestsLineNamesTheInferredCommandAndItsFile(t *testing.T) {
	const workflow = "jobs:\n  test:\n    steps:\n      - run: go test -race ./...\n"
	for _, c := range []struct {
		name    string
		files   map[string]string
		untrust bool
		want    string
		source  string
	}{
		{"go.mod alone", map[string]string{"go.mod": "module m\n"}, false, `"go test ./..." (inferred from go.mod)`, "go.mod"},
		{"CI first", map[string]string{"go.mod": "module m\n", ".github/workflows/ci.yml": workflow}, false,
			`"go test -race ./..." (inferred from .github/workflows/ci.yml)`, ".github/workflows/ci.yml"},
		{"control: an untrusted spec gate wins", map[string]string{"go.mod": "module m\n"}, true, `"go test ./..." (spec, untrusted)`, ""},
		{"control: nothing to infer", map[string]string{"main.go": "package main\n"}, false, "no spec gate; no test runner in the index yet", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := project(t, c.files)
			var sp *spec.Compiled
			if c.untrust {
				sp = specAt(root, spec.Untrusted, "go", "test", "./...")
			}
			b := Build(root, "", Pointer{}, sp)
			f := fact(t, b, KindTests)
			if f.Text != c.want {
				t.Errorf("tests %q, want %q", f.Text, c.want)
			}
			if c.source != "" && f.Source != filepath.Join(root, filepath.FromSlash(c.source)) {
				t.Errorf("source %q, want the file it was read from", f.Source)
			}
			for _, l := range strings.Split(Render(b), "\n") {
				if utf8.RuneCountInString(l) > Width {
					t.Errorf("line over %d columns: %q", Width, l)
				}
			}
		})
	}
}
