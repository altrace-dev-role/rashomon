package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestST2e_NoRunIsNoProofOfNoStoreWrite: TrustRoot hands the store root back
// only for a run it could check. With no run there is no proof the session
// did not write under the store, so it fails closed to the zero TrustedRoot
// (no repository spec reads as trusted); an empty run, which checked clean,
// keeps the root. Break: drop the run == nil test in TrustRoot.
func TestST2e_NoRunIsNoProofOfNoStoreWrite(t *testing.T) {
	const root = "/store"
	if got := TrustRoot(nil, root, 5); got != (TrustedRoot{}) {
		t.Errorf("TrustRoot(nil run) = %+v, want the zero TrustedRoot", got)
	}
	if got := TrustRoot(&store.Run{}, root, 5); got != NoRun(root) {
		t.Errorf("TrustRoot(empty run) = %+v, want the root kept", got)
	}
}
