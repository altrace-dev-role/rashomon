package main

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/pgraph"
)

// TestPW1b_TheCommandLinksNoNetworkPackage (review item 6 on #230): a
// Claude Code hook runs pgraph at session start and after each reply, so
// it keeps rashomon's no-egress promise (H-17, TRU-40) in the one form a
// program that must run go list can: no package on its import graph opens
// a socket, and the go tool it runs is offline (LoaderEnv,
// TestPL9_LoaderEnvForced). os/exec and go/packages must be listed, so an
// empty list cannot pass. Break: link net/http.
func TestPW1b_TheCommandLinksNoNetworkPackage(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = pgraph.LoaderEnv(os.Environ(), pgraph.Config{})
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	deps := strings.Fields(string(out))
	for _, forbidden := range []string{"net", "net/http", "crypto/tls", "net/http/httptrace"} {
		if slices.Contains(deps, forbidden) {
			t.Errorf("pgraph links %s, a path to a socket", forbidden)
		}
	}
	for _, want := range []string{"os/exec", "golang.org/x/tools/go/packages"} {
		if !slices.Contains(deps, want) {
			t.Errorf("premise: %s is not on pgraph's import graph (%d packages listed)", want, len(deps))
		}
	}
}
