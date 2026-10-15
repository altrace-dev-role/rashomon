package acceptance

import "testing"

// TestGuardDropsAKey_BlocksARemovedKeyWithNoVersionBump holds -update's own
// guard to the rule it exists to enforce: a key path present before and
// gone now, with the document's version unchanged, must block rather than
// write -- that pairing is exactly what makes a renamed or dropped key
// invisible, a silently stale JSON contract. The same removal WITH a version
// bump, or no removal at all, must not block.
func TestGuardDropsAKey_BlocksARemovedKeyWithNoVersionBump(t *testing.T) {
	old := []byte("a\nb\nc\nschema_version\n")
	newSame := []byte("a\nc\nschema_version\n") // "b" dropped
	newAll := []byte("a\nb\nc\nschema_version\n")

	if removed, blocked := guardDropsAKey(old, newSame, 1, 1); !blocked || len(removed) != 1 || removed[0] != "b" {
		t.Errorf("removed=%v blocked=%v; want [b] true", removed, blocked)
	}
	if removed, blocked := guardDropsAKey(old, newSame, 1, 2); blocked {
		t.Errorf("removed=%v blocked=%v; a version bump must let a removal through", removed, blocked)
	}
	if removed, blocked := guardDropsAKey(old, newAll, 1, 1); blocked || len(removed) != 0 {
		t.Errorf("removed=%v blocked=%v; no key was dropped, so nothing should block", removed, blocked)
	}
}

// TestVersionOf_UnknownNeverReadsAsUnchanged holds the escape hatch shut: a
// document without the version key asked for, or bytes that are not one,
// reads as -1, and two -1s are still "the versions differ" to the guard above
// (versionOf's own doc) -- an unreadable "before" must not be mistaken for
// "nothing changed". The report's key is report_version and digest's and
// spend's is schema_version; reading one document's version under the
// other's key is the unknown case, not a version.
func TestVersionOf_UnknownNeverReadsAsUnchanged(t *testing.T) {
	if v := versionOf([]byte(`{"report_version":2,"schema_version":3}`), "report_version"); v != 2 {
		t.Errorf("versionOf(report_version) = %d, want 2", v)
	}
	if v := versionOf([]byte(`{"schema_version":3}`), "schema_version"); v != 3 {
		t.Errorf("versionOf(schema_version) = %d, want 3", v)
	}
	if v := versionOf([]byte(`{"schema_version":3}`), "report_version"); v != -1 {
		t.Errorf("versionOf of a key the document does not carry = %d, want -1", v)
	}
	if v := versionOf([]byte(`not json`), "report_version"); v != -1 {
		t.Errorf("versionOf(not json) = %d, want -1", v)
	}
	if v := versionOf([]byte(`{}`), "report_version"); v != -1 {
		t.Errorf("versionOf({}) = %d, want -1", v)
	}
	if removed, blocked := guardDropsAKey([]byte("a\n"), []byte(""), -1, -1); !blocked || len(removed) != 1 {
		t.Errorf("removed=%v blocked=%v; two unreadable versions must still block a removal", removed, blocked)
	}
}
