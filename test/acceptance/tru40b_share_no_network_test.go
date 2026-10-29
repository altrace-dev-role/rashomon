package acceptance

import (
	"slices"
	"strings"
	"testing"
)

// TestTRU40b_ShareLinksNoNetworkPackage: internal/share builds what a user may
// hand on, and the handing on is not rashomon's (Altrace out of rashomon,
// 10-23): no endpoint, no client, no certificate. TRU-40 makes the claim of the
// whole binary; this one stays with the package that would be the first place
// to reach for a client, whatever the binary's graph becomes. os/exec is
// banned here outright, as H-17 bans it for the recorder.
func TestTRU40b_ShareLinksNoNetworkPackage(t *testing.T) {
	pkgs := deps(t, "./internal/share")
	ours := 0
	for _, pkg := range pkgs {
		if why, bad := forbiddenImports[pkg]; bad {
			t.Errorf("internal/share depends on %s, which %s", pkg, why)
		}
		if strings.HasPrefix(pkg, "github.com/altrace-dev-role/rashomon/") {
			ours++
		}
	}
	// The failure mode of every dependency-scanning check is to scan nothing:
	// the listing must hold share itself and the store and steer packages it
	// reads through.
	for _, want := range []string{"internal/share", "internal/store", "internal/steer"} {
		if !slices.Contains(pkgs, "github.com/altrace-dev-role/rashomon/"+want) {
			t.Fatalf("the dependency listing of internal/share (%d packages, %d of this module's) lacks %s; "+
				"the command did not run as expected", len(pkgs), ours, want)
		}
	}
}
