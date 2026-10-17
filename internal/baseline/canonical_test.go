package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// writeLegacyBaseline writes a baseline file as a pre-upgrade build would
// have: a host key in whatever form that build's host.Canonical (if it had
// one at all) produced, never re-keyed for an IP literal's one spelling.
func writeLegacyBaseline(t *testing.T, root, key string, hosts map[string]entry) string {
	t.Helper()
	dir := filepath.Join(root, "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f := File{Version: Version, Project: key, Basis: Basis, Hosts: hosts}
	body, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName(key))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestUpdate_LegacyIPLiteralIsNotNovelAgain: a baseline written before
// host.Canonical folded an IP literal to its one spelling holds the
// uncanonicalised string. A session naming the same host in its canonical
// form must not read as novel, and the two keys must merge into one host,
// not two.
func TestUpdate_LegacyIPLiteralIsNotNovelAgain(t *testing.T) {
	root := t.TempDir()
	writeLegacyBaseline(t, root, "/proj", map[string]entry{
		"167838211": {FirstSeenSession: "sess-0", FirstSeenAtUnixMS: t0.UnixMilli()},
	})

	res := Update(root, "/proj", "sess-1", t0.Add(time.Hour), []string{"10.1.2.3"})

	if len(res.Novel) != 0 {
		t.Errorf("novel = %v, want empty: 167838211 and 10.1.2.3 are one host", res.Novel)
	}
	if res.Known != 1 {
		t.Errorf("known = %d, want 1: the legacy key and the canonical one must merge", res.Known)
	}
}

// TestLoad_MergesCollidingLegacyKeys: two pre-upgrade keys that canonicalise
// to the same host merge into one entry, keeping the earlier
// FirstSeenSession and the later LastSeenAtUnixMS, as Fold's own ownership
// rule decides both.
//
// "10.1.2.3" sorts before "167838211", so the merge processes it first and
// has to correct away from it: the later session landed in the map first,
// and the earlier one -- read second -- has to displace it, which is the
// direction a map walked in insertion order would never need to.
func TestLoad_MergesCollidingLegacyKeys(t *testing.T) {
	root := t.TempDir()
	path := writeLegacyBaseline(t, root, "/proj", map[string]entry{
		// "10.1.2.3" sorts first and carries the later FirstSeenAtUnixMS and
		// the earlier LastSeenAtUnixMS, so a correct merge has to take the
		// FirstSeenSession from the *other* key and the LastSeenAtUnixMS
		// from it too -- the one direction a merge that only ever kept the
		// first-processed entry's fields would get both halves wrong in.
		"10.1.2.3":  {FirstSeenSession: "sess-late", FirstSeenAtUnixMS: t0.Add(2 * time.Hour).UnixMilli(), LastSeenAtUnixMS: t0.Add(2 * time.Hour).UnixMilli()},
		"167838211": {FirstSeenSession: "sess-early", FirstSeenAtUnixMS: t0.UnixMilli(), LastSeenAtUnixMS: t0.Add(3 * time.Hour).UnixMilli()},
	})

	f, existed, err := load(path, "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if !existed {
		t.Fatal("existed = false for a readable basis-matching file")
	}
	if len(f.Hosts) != 1 {
		t.Fatalf("hosts = %v, want one merged entry", f.Hosts)
	}
	e, ok := f.Hosts["10.1.2.3"]
	if !ok {
		t.Fatalf("hosts = %v, want the 10.1.2.3 key", f.Hosts)
	}
	if e.FirstSeenSession != "sess-early" {
		t.Errorf("first_seen_session = %q, want sess-early: the earlier session owns it", e.FirstSeenSession)
	}
	if want := t0.Add(3 * time.Hour).UnixMilli(); e.LastSeenAtUnixMS != want {
		t.Errorf("last_seen_at_unix_ms = %d, want %d, the later of the two", e.LastSeenAtUnixMS, want)
	}
}

// TestLoad_RefusesAnUnshapedLegacyKey: a key host.Canonical maps to
// something host.Shaped refuses -- or that it refuses outright -- is
// dropped rather than carried forward under a new spelling.
func TestLoad_RefusesAnUnshapedLegacyKey(t *testing.T) {
	root := t.TempDir()
	path := writeLegacyBaseline(t, root, "/proj", map[string]entry{
		"api.example.com=ghp_x": {FirstSeenSession: "sess-0", FirstSeenAtUnixMS: t0.UnixMilli()},
		"good.example":          {FirstSeenSession: "sess-0", FirstSeenAtUnixMS: t0.UnixMilli()},
	})

	f, _, err := load(path, "/proj")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.Hosts, map[string]entry{"good.example": f.Hosts["good.example"]}) {
		t.Errorf("hosts = %v, want only good.example", f.Hosts)
	}
}
