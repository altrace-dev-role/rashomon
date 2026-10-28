package hook

import (
	"errors"
	"io/fs"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ConsentSteerDisclosure is consent.SteerDisclosure, the highest steer
// disclosure version consent.json holds, which cmd/rashomon sets before any
// hook runs, beside ConsentRecordScope and for its reason: importing
// internal/consent here would be a cycle. Nil, in a test that builds a probe
// without it, skips syncSteerDisclosure; the binary always sets it.
var ConsentSteerDisclosure func(root string) (int, error)

// syncSteerDisclosure sets root's steer-disclosure marker
// (store.FileSteerDisclosure) to the version consent.json names, and reports
// whether it could. The SessionStart probe calls it, once per session, beside
// syncRecordScope and with its shape: an install whose consent predates the
// marker gains it at its next session start, and a marker a crash or a hand
// edit left disagreeing with consent.json is put right.
//
// It writes only on a difference. No marker already reads 0, so a store with
// no steering consent on file keeps none. A consent.json it cannot read, or a
// marker it cannot write, leaves no marker -- 0, no consent, so every note is
// a flag -- and the false return names no coverage reason: a failed steer
// sync withholds a convenience, not evidence, and the steer log's missing
// lines are its trace.
func syncSteerDisclosure(root string) bool {
	if ConsentSteerDisclosure == nil {
		return true
	}
	want, err := ConsentSteerDisclosure(root)
	if err != nil || want < 0 {
		_ = store.RemoveSteerDisclosureMarker(root)
		return false
	}
	have, err := store.SteerDisclosureMarker(root)
	if have == want && (err == nil || errors.Is(err, fs.ErrNotExist)) {
		return true
	}
	if err := store.WriteSteerDisclosureMarker(root, want); err != nil {
		_ = store.RemoveSteerDisclosureMarker(root)
		return false
	}
	return true
}
