package gateinfer

import (
	"strings"
	"testing"
)

// TestGI1_BoundedReadsMakeOversizedFilesUnknown (manager, 01:2x, "never
// slurp"): a CI line over MaxLine makes that file unknown, so the manifest
// after it names the command; a package.json over MaxJSON is not read, so
// its test script names nothing; and a test target past MaxRead in a
// Makefile is never reached. Controls: each file within its bound names its
// command. Break: read a file whole, or let the scanner's buffer grow past
// MaxLine.
func TestGI1_BoundedReadsMakeOversizedFilesUnknown(t *testing.T) {
	long := "      - run: go test -race ./... # " + strings.Repeat("x", MaxLine) + "\n"
	ci := "jobs:\n  t:\n    steps:\n" + long
	pkg := `{"scripts": {"test": "jest"}, "pad": "` + strings.Repeat("x", MaxJSON) + `"}`
	mk := strings.Repeat("# pad\n", MaxRead/6+1) + "test:\n\tgo test ./...\n"
	for _, c := range []struct {
		name  string
		files map[string]string
		argv  string
		from  string
	}{
		{"a CI line over MaxLine", map[string]string{".github/workflows/ci.yml": ci, "go.mod": goMod}, "go test ./...", "go.mod"},
		{"control: the same line within it", map[string]string{".github/workflows/ci.yml": "jobs:\n  t:\n    steps:\n      - run: go test -race ./...\n", "go.mod": goMod},
			"go test -race ./...", ".github/workflows/ci.yml"},
		{"a package.json over MaxJSON", map[string]string{"package.json": pkg, "go.mod": goMod}, "go test ./...", "go.mod"},
		{"a Makefile test target past MaxRead", map[string]string{"Makefile": mk, "go.mod": goMod}, "go test ./...", "go.mod"},
		{"control: a test target within it", map[string]string{"Makefile": "test:\n\tgo test ./...\n", "go.mod": goMod}, "make test", "Makefile"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Infer(project(t, c.files))
			if !ok || strings.Join(got.Argv, " ") != c.argv || got.From != c.from {
				t.Errorf("Infer = %+v, %v; want %q from %s", got, ok, c.argv, c.from)
			}
		})
	}
}
