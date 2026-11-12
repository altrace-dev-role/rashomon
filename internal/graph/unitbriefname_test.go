package graph

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestGraph_UnitBriefNameRoundTrip (W-M2.3 hook half, D2, D8): a unit's
// brief file is named by the index root and the unit's local id together,
// always in store.Segment's digest form, so two indexes in one session
// never share a unit's file and the name holds no path; every name the
// writer builds satisfies the predicate purge reads, and the near misses a
// person's files make do not. The session directory is one segment under
// the store's graph directory, whatever the hook's session id holds.
// Break: name the file by the unit alone, and the two roots collide.
func TestGraph_UnitBriefNameRoundTrip(t *testing.T) {
	unit := ID("unit:go:internal/steer")
	a := UnitBriefFileName("/src/a", unit)
	if a == UnitBriefFileName("/src/b", unit) {
		t.Errorf("two index roots share the unit's file %q", a)
	}
	if a == UnitBriefFileName("/src/a", "unit:go:internal/hook") {
		t.Errorf("two units share the file %q", a)
	}
	if a != UnitBriefFileName("/src/a", unit) {
		t.Error("the name is not a function of root and unit")
	}
	if !strings.HasPrefix(a, "x-") || !strings.HasSuffix(a, ".json") || len(a) != len("x-")+32+len(".json") {
		t.Errorf("name %q is not the digest form", a)
	}
	for _, name := range []string{a, UnitBriefFileName("", ""), UnitBriefFileName("/", "u")} {
		if !IsUnitBriefFileName(name) {
			t.Errorf("IsUnitBriefFileName(%q) = false for a written name", name)
		}
	}
	hex := strings.TrimSuffix(strings.TrimPrefix(a, "x-"), ".json")
	for _, near := range []string{"x-" + hex + ".txt", "steer.json", "x-" + hex[1:] + ".json", "x-" + strings.ToUpper(hex) + ".json",
		"." + a, a + ".tmp-1", "x-" + hex, ""} {
		if IsUnitBriefFileName(near) {
			t.Errorf("IsUnitBriefFileName(%q) = true for a name the writer never builds", near)
		}
	}
	dir := UnitBriefDir("/store", "../../etc")
	if rel, err := filepath.Rel(filepath.Join("/store", StoreDir, UnitBriefSubdir), dir); err != nil || strings.Contains(rel, "/") ||
		strings.HasPrefix(rel, ".") {
		t.Errorf("session dir %q is not one segment under the unit-brief dir (rel %q)", dir, rel)
	}
	if !IsUnitBriefSessionDir(filepath.Base(dir)) || IsUnitBriefSessionDir(".x") || IsUnitBriefSessionDir("a/b") {
		t.Error("the session directory predicate does not read the writer's rule")
	}
}
