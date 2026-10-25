package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

// recordtestPkg is the package the record tests check their copies with.
const recordtestPkg = modulePath + "/internal/store/recordtest"

// TestB39_RecordtestNeverEntersTheBinary: no release build links
// internal/store/recordtest, on any release target TRU-40 lists. It is a
// reflection filler and copy checker: test apparatus, which the shipped
// binary never carries.
//
// Of the two properties a ban like this could assert, this is the shipped
// one: the release builds' dependency graph (releaseDeps), not the module's
// import statements. A non-test file that imports recordtest, in a package no
// release build links, ships nothing and is outside this row, by design; a
// walk over the source would flag it. Break: import recordtest from
// internal/hook/post.go and reference it, and the row names the package. The
// break is made in a tree, not by -overlay: releaseListEnv clears GOFLAGS, so
// the listing reads the files on disk.
func TestB39_RecordtestNeverEntersTheBinary(t *testing.T) {
	// A row about a package that is not there passes forever.
	if _, err := os.Stat(filepath.Join(moduleRoot, "internal", "store", "recordtest")); err != nil {
		t.Fatalf("%s is not where this row looks: %v", recordtestPkg, err)
	}
	for _, pkg := range releaseDeps(t) {
		if pkg == recordtestPkg {
			t.Errorf("the binary depends on %s, which is test apparatus: only test files import it", pkg)
		}
	}
}
