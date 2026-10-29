package acceptance

import (
	"slices"
	"testing"
)

// TestTRU40c_ReceiptLinksNoNetworkPackage: internal/receipt names a
// checkpoint for a commit and hands its bytes to the git hooks the user
// installs; the git work, and any push, is theirs (5.5). TRU-40 makes the
// claim of the whole binary; this one stays with the package that would be
// the first place to reach for git or a remote, whatever the binary's graph
// becomes. os/exec is banned here outright, as H-17 bans it for the recorder.
func TestTRU40c_ReceiptLinksNoNetworkPackage(t *testing.T) {
	pkgs := deps(t, "./internal/receipt")
	for _, pkg := range pkgs {
		if why, bad := forbiddenImports[pkg]; bad {
			t.Errorf("internal/receipt depends on %s, which %s", pkg, why)
		}
	}
	// The failure mode of every dependency-scanning check is to scan nothing.
	for _, want := range []string{"internal/receipt", "internal/store"} {
		if !slices.Contains(pkgs, "github.com/altrace-dev-role/rashomon/"+want) {
			t.Fatalf("the dependency listing of internal/receipt (%d packages) lacks %s; the command did not run as expected",
				len(pkgs), want)
		}
	}
}
