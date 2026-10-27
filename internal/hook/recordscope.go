package hook

import (
	"errors"
	"io/fs"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ConsentRecordScope is consent.RecordScope, the record scope consent.json
// names, which cmd/rashomon sets before any hook runs, as it sets Writer.
// It is handed in rather than imported: internal/consent reaches
// internal/report through disclose and recap, and report's own tests import
// this package, so the import would be a cycle. Nil, in a test that builds a
// probe without it, skips syncRecordScope; the binary always sets it.
var ConsentRecordScope func(root string) (string, error)

// syncRecordScope sets root's record-scope marker (store.FileRecordScope) to
// the record scope consent.json names, and reports whether it could. The
// SessionStart probe calls it, once per session: an install whose consent
// predates the marker gains it at its next session start, and a marker a
// crash or a hand edit left disagreeing with consent.json is put right.
//
// This is the one place a hook reads consent.json, and it is on SessionStart
// only, never on PreToolUse or PostToolUse: those read the marker, one small
// file, and a consent.json of up to 64 KiB is parsed here once per session
// rather than on every tool call.
//
// It writes only on a difference. No marker already reads v4, so a store
// with no consent on file keeps none. A consent.json it cannot read, or a
// marker it cannot write, leaves no marker -- v4, which withholds -- and the
// false return is the caller's to name on the coverage record.
func syncRecordScope(root string) bool {
	if ConsentRecordScope == nil {
		return true
	}
	want, err := ConsentRecordScope(root)
	if err != nil {
		_ = store.RemoveRecordScopeMarker(root)
		return false
	}
	if want != store.RecordScopeV5 {
		want = store.RecordScopeV4
	}
	have, err := store.RecordScopeMarker(root)
	if have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) {
		return true
	}
	if err := store.WriteRecordScopeMarker(root, want); err != nil {
		_ = store.RemoveRecordScopeMarker(root)
		return false
	}
	return true
}
