package hook

import "github.com/altrace-dev-role/rashomon/internal/store"

// ConsentSessionContent is consent.SessionContent, whether the consent on
// file lets the hook keep the session's content, which cmd/rashomon sets
// before any hook runs, beside ConsentRecordScope and for its reason:
// importing internal/consent here would be a cycle. Nil, in a test that
// builds a probe without it, skips syncSessionContent; the binary always
// sets it.
var ConsentSessionContent func(root string) (bool, error)

// syncSessionContent sets root's session-content marker
// (store.FileSessionContent) to consent.json's answer, and reports whether it
// could. The SessionStart probe calls it, once per session, beside
// syncRecordScope and with its shape: a marker that a crash between a consent
// line and its marker, or a hand edit, left disagreeing with consent.json is
// put right, written where the consent covers the session's content and
// removed where it does not (W-M2.11 step 1d).
//
// It writes only on a difference. A consent.json it cannot read, or a marker
// it cannot write, leaves no marker -- none of the session's content kept --
// and the false return names no coverage reason: like a failed steer sync, it
// withholds content the hook may keep, not evidence the record needs.
func syncSessionContent(root string) bool {
	if ConsentSessionContent == nil {
		return true
	}
	want, err := ConsentSessionContent(root)
	if err != nil {
		_ = store.RemoveSessionContentMarker(root)
		return false
	}
	if want == store.SessionContentMarker(root) {
		return true
	}
	if !want {
		return store.RemoveSessionContentMarker(root) == nil
	}
	if err := store.WriteSessionContentMarker(root); err != nil {
		_ = store.RemoveSessionContentMarker(root)
		return false
	}
	return true
}
