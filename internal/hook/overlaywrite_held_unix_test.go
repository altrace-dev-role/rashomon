//go:build unix

package hook

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestHook_DeclarationNotHeldWritesNoGroup: with the records lock held
// elsewhere the declaration does not land, so there is no seq to cite and Pre
// writes no group, the marker being present; the Post whose execution
// spilled for the same reason writes none either.
func TestHook_DeclarationNotHeldWritesNoGroup(t *testing.T) {
	f := newOvFix(t, "1\n")
	holdLocks(t, filepath.Join(f.st.RunDir(ovSess), store.FileRecords))
	in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")

	h := New(f.st, time.Now)
	h.lockWaits = 100 * time.Millisecond
	err := h.Capture(strings.NewReader(f.payload("PreToolUse", "toolu_held", "Edit", in)))
	h.Close(&Signals{}, err)
	if err == nil {
		t.Fatal("the declaration landed under a held lock")
	}
	p := NewPost(f.st, time.Now)
	p.lockWaits = 100 * time.Millisecond
	p.Close(&Signals{}, p.Capture(strings.NewReader(f.payload("PostToolUse", "toolu_held", "Edit", in))))

	if _, _, ok := f.read(); ok {
		t.Error("a session log was written with no declaration in the ordered stream")
	}
}
