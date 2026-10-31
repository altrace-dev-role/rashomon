package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSR1y_AnUnreadableRunNeverYieldsATrustedSpec (D173, #173 + #155): a
// repository spec bound in the store's bindings reads trusted only once the
// session's own record has been read and shown to hold no write under the
// store root. With the records file unreadable (here a directory where the
// file was) a step that fires has no run to check, so the spec must come
// back untrusted, and a spec that is untrusted raises no model-visible rule.
// Break: let discoverFor trust a stepped call whose run is nil, and
// TrustRoot hand the store root back for a nil run.
func TestSR1y_AnUnreadableRunNeverYieldsATrustedSpec(t *testing.T) {
	f := boundST(t)
	p := f.call("toolu_r", "Bash", goTest, 1)
	records := filepath.Join(f.st.RunDir(stSession), store.FileRecords)
	if err := os.Remove(records); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(records, 0o700); err != nil {
		t.Fatal(err)
	}
	_, view, ok := p.judge(stEnv(0))
	if ok && len(view.Findings) != 0 {
		t.Errorf("an unreadable run still raised %d finding(s): %+v", len(view.Findings), view.Findings)
	}
	if p.sp == nil {
		t.Fatal("no spec was discovered")
	}
	if p.sp.Trust != spec.Untrusted {
		t.Errorf("trust %q with the run unreadable, want untrusted", p.sp.Trust)
	}
}
