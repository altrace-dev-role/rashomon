package acceptance

import (
	"encoding/json"
	"strings"
)

// removedPaths returns the key paths old names that new does not, each list
// in docKeyPaths' own form (sorted, one per line). It is how both goldens'
// -update guard against silently dropping a key: a path present before and
// absent now is a removal whether or not anything else in the document
// changed with it.
func removedPaths(old, new []byte) []string {
	have := map[string]bool{}
	for _, p := range strings.Split(strings.TrimRight(string(new), "\n"), "\n") {
		if p != "" {
			have[p] = true
		}
	}
	var removed []string
	for _, p := range strings.Split(strings.TrimRight(string(old), "\n"), "\n") {
		if p != "" && !have[p] {
			removed = append(removed, p)
		}
	}
	return removed
}

// versionOf reads a document's top-level version field, named key --
// report_version for the report, schema_version for digest and spend -- or -1
// when it carries none or does not parse. -1 never equals another document's
// -1 in the guard's own sense (unknown is never "unchanged"), because a
// schema the guard cannot read is not one it can say held still.
func versionOf(raw []byte, key string) int {
	var doc map[string]json.RawMessage
	var v *int
	if json.Unmarshal(raw, &doc) != nil || json.Unmarshal(doc[key], &v) != nil || v == nil {
		return -1
	}
	return *v
}

// guardDropsAKey reports whether writing new over old under -update would
// drop a key path with no version bump to show for it: the pairing that
// makes a renamed or removed key invisible, since the committed file simply
// becomes the new shape with nothing to diff it against.
func guardDropsAKey(oldPaths, newPaths []byte, oldVersion, newVersion int) (removed []string, blocked bool) {
	removed = removedPaths(oldPaths, newPaths)
	return removed, len(removed) > 0 && oldVersion == newVersion
}
