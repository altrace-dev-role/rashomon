package acceptance

import (
	"slices"
	"strings"
	"testing"
)

// bannedDeps is every package in pkgs that forbiddenImports bans, sorted by
// the order pkgs lists them.
func bannedDeps(pkgs []string) []string {
	var out []string
	for _, p := range pkgs {
		if _, bad := forbiddenImports[p]; bad {
			out = append(out, p)
		}
	}
	return out
}

// TestTRU40b_ShareLinksNoNetworkPackage: internal/share, which builds what a
// person may hand over, links no networking package -- `go list -deps` of it
// holds no net, net/http, crypto/tls, net/http/httptrace or os/exec. TRU-40
// makes the claim of the whole binary; this one names the package whose job
// is to produce files for sharing, so a bundle writer that learned to send
// itself fails here before it fails there.
func TestTRU40b_ShareLinksNoNetworkPackage(t *testing.T) {
	pkgs := deps(t, "./internal/share")
	// The failure mode of a dependency scan is to scan nothing, or the wrong
	// thing: the listing must be long enough to be a real one and must reach
	// the package this test is about.
	if len(pkgs) < 10 {
		t.Fatalf("dependency listing returned only %d packages; the command did not run as expected", len(pkgs))
	}
	if !slices.Contains(pkgs, modulePath+"/internal/share") {
		t.Fatalf("the listing of ./internal/share does not include internal/share itself: %v", pkgs)
	}
	for _, bad := range bannedDeps(pkgs) {
		t.Errorf("internal/share depends on %s, which %s", bad, forbiddenImports[bad])
	}
}

// TestTRU40b_TheBanSeesTheNetworkPackages: the check above names net when a
// listing holds it, and nothing for a listing that does not.
func TestTRU40b_TheBanSeesTheNetworkPackages(t *testing.T) {
	if got := bannedDeps([]string{"fmt", "net", "encoding/json"}); strings.Join(got, ",") != "net" {
		t.Errorf("bannedDeps over a listing holding net = %v, want [net]", got)
	}
	if got := bannedDeps([]string{"fmt", "net/http", "os/exec"}); len(got) != 2 {
		t.Errorf("bannedDeps over net/http and os/exec = %v, want both", got)
	}
	if got := bannedDeps([]string{"fmt", "network", "encoding/json", "internal/nettest"}); len(got) != 0 {
		t.Errorf("bannedDeps over a clean listing = %v, want none (a name containing net is not net)", got)
	}
}
