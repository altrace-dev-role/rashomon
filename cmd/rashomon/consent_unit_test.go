package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// consentHere is an installed store under RASHOMON_HOME, outside an agent
// session, with the test's own working directory (card 4.2).
func consentHere(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	benchSetenv(t, root, t.TempDir())
	t.Setenv("CLAUDECODE", "")
	if _, err := store.Open(root); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	return root
}

// grantBodiesHere is the user's `rashomon consent bodies --yes` on root, with
// the probe handed consent.Has as main hands it, for this test alone.
func grantBodiesHere(t *testing.T, root string) {
	t.Helper()
	if err := consent.Append(root, consent.Record{V: consent.RecordVersion, Via: consent.ViaConsentYes,
		Unit: "bodies", UnitOn: true, At: 1}); err != nil {
		t.Fatal(err)
	}
	prev := hook.ConsentHas
	hook.ConsentHas = consent.Has
	t.Cleanup(func() { hook.ConsentHas = prev })
}

// TestTR1f_UnknownUnitIsRefused: a word that is no unit is refused naming
// the four, and a share unit nothing in this release builds is refused as
// nothing to agree to; exit 1, nothing on stdout, nothing written.
func TestTR1f_UnknownUnitIsRefused(t *testing.T) {
	root := consentHere(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"consent", "bogus", "--yes"}, "bodies, share.outcomes, share.lessons, share.bodies"},
		{[]string{"consent", "share.lessons", "--yes"}, "nothing to agree to"},
		{[]string{"consent", "share.bodies"}, "nothing to agree to"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(c.args, nil, &stdout, &stderr); code != exitFail || stdout.Len() != 0 || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("rashomon %s: exit %d, stdout %q, stderr %q; want exit 1, nothing on stdout, stderr naming %q",
				strings.Join(c.args, " "), code, stdout.String(), stderr.String(), c.want)
		}
	}
	if _, err := os.Stat(consent.Path(root)); !os.IsNotExist(err) {
		t.Errorf("a refused unit wrote consent.json (%v)", err)
	}
}
