package consent

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// TestCSCOPE1b_AnAlpha1ConsentSeesTheChangedScreen (CONSENT-SCOPE1): an install
// that agreed on alpha.1's screen (scope 4, steer disclosure 2) is not
// steer-current on this build, so its notes come to its user as flags, and
// the screen it agrees on next opens with CHANGED SINCE: the screen that
// says record scope v5 keeps paths in the clear is read before a note
// reaches the agent again. The CHANGED SINCE half is a control; it holds on
// main. Break: leave the steer disclosure at 2, and alpha.1's yes keeps
// sending notes under a screen its user never saw.
func TestCSCOPE1b_AnAlpha1ConsentSeesTheChangedScreen(t *testing.T) {
	root := t.TempDir()
	prior := record(ViaWatchTTY, 4, 2, RecordScopeV4)
	if err := Append(root, prior); err != nil {
		t.Fatal(err)
	}
	if cur, err := Current(root); err != nil || cur {
		t.Errorf("Current = %v (%v) for alpha.1's consent (scope 4, steer disclosure 2), want false: its notes wait for the new screen", cur, err)
	}
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Screen(inv, ScreenInput{Version: "test", Prior: &prior})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "CHANGED SINCE YOU LAST AGREED (test, scope 4): this build asks for scope 5.") {
		t.Errorf("the screen after alpha.1's consent does not open with CHANGED SINCE:\n%s", b)
	}
}
