package graph

import (
	"strings"
	"testing"
)

// TestGlobalID_RoundTrip (W-M1.4, AD14): a global id is g:<rid>#<local>,
// and SplitGlobal gives back the repository and the local id whatever
// colons or a later hash sign the local id holds.
func TestGlobalID_RoundTrip(t *testing.T) {
	for _, local := range []ID{
		"unit:go:internal/platform",
		"sym:go:internal/platform:func:ReadSnapshot",
		"decl:.rashomon/graph.json#services.auth",
		"file:a/b#c.go",
	} {
		g := GlobalID("github.com/x/y", local)
		if g != ID("g:github.com/x/y#"+string(local)) {
			t.Errorf("GlobalID = %q", g)
		}
		rid, l, ok := SplitGlobal(g)
		if !ok || rid != "github.com/x/y" || l != local {
			t.Errorf("SplitGlobal(%q) = %q, %q, %v", g, rid, l, ok)
		}
	}
	eph := "path:" + strings.Repeat("a", 64)
	if rid, l, ok := SplitGlobal(GlobalID(eph, "unit:py:.")); !ok || rid != eph || l != "unit:py:." {
		t.Errorf("an ephemeral rid does not round-trip: %q %q %v", rid, l, ok)
	}
	if RepoNodeID("github.com/x/y") != "repo:github.com/x/y" {
		t.Errorf("RepoNodeID = %q", RepoNodeID("github.com/x/y"))
	}
}

// TestGlobalID_RejectsMalformed (W-M1.4): a local id, an id with no hash
// sign, an empty repository or an empty local id is no global id.
func TestGlobalID_RejectsMalformed(t *testing.T) {
	for _, id := range []ID{"", "unit:go:x", "g:", "g:rid", "g:#unit:go:x", "g:rid#", "repo:rid", "G:rid#unit:go:x"} {
		if rid, l, ok := SplitGlobal(id); ok || rid != "" || l != "" {
			t.Errorf("SplitGlobal(%q) = %q, %q, %v; want nothing", id, rid, l, ok)
		}
	}
}
