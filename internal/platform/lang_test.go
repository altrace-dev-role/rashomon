package platform_test

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// TestLang_EqualsGopgLang holds platform.Lang equal to gopg.Lang. cmd's
// blast-radius port takes its Lang from platform.Lang, and the hook routes a
// file to the port only where the registry's reader for that Lang claims it;
// gopg's reader reports gopg.Lang. A rename on either side that left the two
// unequal would route no file to the port, and the package read of every Go
// edit would go silent; this test is what stops that loss. It sits in the
// external test package because gopg imports platform.
func TestLang_EqualsGopgLang(t *testing.T) {
	if platform.Lang != gopg.Lang {
		t.Fatalf("platform.Lang is %q and gopg.Lang is %q: the blast-radius port would take no file",
			platform.Lang, gopg.Lang)
	}
}
