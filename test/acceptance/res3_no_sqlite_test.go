package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RES-3: the binary links neither modernc.org/sqlite nor modernc.org/libc, on
// any release target TRU-40 lists.
//
// This is the memory side of TRU-40. The driver and its libc were 54% of the
// binary and the reason a hook process could not start below its budget; the
// floor the specification budgets against exists only without them. go.mod is
// checked as well as the link, because a requirement nothing imports is still
// a module every reader and `go mod tidy` sees, and the next import would
// bring the subtree back without a go.mod change to review.
func TestRES3_BinaryLinksNoSQLite(t *testing.T) {
	for _, pkg := range releaseDeps(t) {
		if strings.HasPrefix(pkg, "modernc.org/") {
			t.Errorf("the binary depends on %s", pkg)
		}
	}
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "modernc.org") {
			t.Errorf("go.mod:%d names modernc.org: %s", i+1, strings.TrimSpace(line))
		}
	}
}
