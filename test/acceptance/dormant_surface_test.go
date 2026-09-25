package acceptance

// The alpha offers no reader the dormant proxy path.
//
// env, run and --proxy-store stay in the binary as the bridge to the observing
// proxy, which does not ship with this alpha. Everything a new user reads --
// the README, the installer's output, the skills and the editor commands --
// must not send them there: env refuses on their machine, run records no
// destination, and --proxy-store reads a database they do not have. This is
// the spec's acceptance search, run as a test so it holds after the review
// that wrote it is gone.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dormantProxyPath matches the bridge offered as something to run: `rashomon
// run` or `rashomon env` as a COMMAND -- a word boundary, so the status
// skill's trigger phrase "is rashomon running" is not one -- and proxy-store
// in any spelling.
var dormantProxyPath = regexp.MustCompile(`(?i)\brashomon (run|env)\b|proxy-store`)

// dormantMentions returns every line of text that offers the dormant path.
func dormantMentions(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if dormantProxyPath.MatchString(line) {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

// dormantSurfaces are what a reader of this alpha reads: files, or
// directories walked whole.
var dormantSurfaces = []string{"README.md", "install.sh", "plugin/skills", "skills", ".cursor"}

func TestSurface_OffersNoDormantProxyPath(t *testing.T) {
	// The matcher first, both ways: a pattern that matched nothing would
	// make every absence below pass.
	for line, want := range map[string]bool{
		"rashomon run -- claude":              true,
		"`rashomon env --port N`":             true,
		"report --proxy-store PATH":           true,
		"RASHOMON ENV":                        true,
		"is rashomon running":                 false,
		"rashomon environment":                false,
		"rashomon report --session S --chain": false,
	} {
		if got := dormantProxyPath.MatchString(line); got != want {
			t.Errorf("dormantProxyPath.MatchString(%q) = %v, want %v", line, got, want)
		}
	}

	var files []string
	for _, surface := range dormantSurfaces {
		root := filepath.Join(moduleRoot, surface)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", surface, err)
		}
	}
	// Each surface contributed something, or its absence proves nothing.
	for _, surface := range dormantSurfaces {
		var n int
		for _, f := range files {
			if rel, _ := filepath.Rel(moduleRoot, f); rel == surface || strings.HasPrefix(rel, surface+"/") {
				n++
			}
		}
		if n == 0 {
			t.Errorf("premise broken: %s holds no file, so this test searched nothing there", surface)
		}
	}

	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(moduleRoot, f)
		for _, line := range dormantMentions(string(body)) {
			t.Errorf("%s offers the dormant proxy path, which this alpha does not serve: %q",
				rel, line)
		}
	}
}
