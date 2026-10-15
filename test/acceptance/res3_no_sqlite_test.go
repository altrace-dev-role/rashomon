package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RES-3: the binary links no module outside the standard library, on any
// release target TRU-40 lists.
//
// This is the memory side of TRU-40. modernc.org/sqlite and its libc were 54%
// of the binary and the reason a hook process could not start below its
// budget; the floor the specification budgets against exists only without
// them. go.mod carries no requirement at all now, which this asserts
// directly rather than by naming the one module that used to be there --
// naming it would pass the day a different module took sqlite's place. go.mod
// is checked as well as the link, because a requirement nothing imports is
// still a module every reader and `go mod tidy` sees, and the next import
// would bring a subtree back without a go.mod change to review.
func TestRES3_BinaryLinksNoSQLite(t *testing.T) {
	for _, pkg := range releaseDeps(t) {
		if pkg == modulePath || strings.HasPrefix(pkg, modulePath+"/") {
			continue // this module's own packages
		}
		if strings.Contains(strings.SplitN(pkg, "/", 2)[0], ".") {
			// A stdlib import path never has a dot in its first element
			// (net/http, encoding/json); a module path does
			// (golang.org/x/..., modernc.org/...). Catches the next module,
			// not only the one this test used to name.
			t.Errorf("the binary depends on %s, which is not in the standard library", pkg)
		}
	}
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require") {
			t.Errorf("go.mod:%d has a require directive, naming a module outside the standard library: %s",
				i+1, strings.TrimSpace(line))
		}
	}
}
