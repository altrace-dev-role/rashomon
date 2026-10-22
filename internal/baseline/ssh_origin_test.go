package baseline

import (
	"testing"
	"time"
)

// TestUpdateOrigin_SSHAliasSurvivesRepeatedRenders: a numeric ssh alias
// folded by host.Canonical reads as a different host after one render and
// is gone, merged into the folded spelling, after a second -- fixing only
// the extraction side was not enough, because load's own canonicalizeHosts
// runs on every read and would refold it right back. UpdateOrigin's
// sshHosts marks the entry (entry.SSH) so canonicalizeHosts skips the fold
// for it, on this render and every one after.
func TestUpdateOrigin_SSHAliasSurvivesRepeatedRenders(t *testing.T) {
	root := t.TempDir()
	ssh := map[string]bool{"1234": true}

	// sess-1 establishes the baseline, so it reports "established" rather
	// than novel hosts (Result.Established's own doc); sess-2 below is the
	// one whose Novel answer this test is really about.
	first := UpdateOrigin(root, "/proj", "sess-1", t0, []string{"1234"}, ssh)
	if first.Err != nil {
		t.Fatal(first.Err)
	}
	if !first.Established {
		t.Fatalf("first render established = false, want true")
	}

	// A second render, of a different session, reloads the file through
	// canonicalizeHosts before folding anything new in. If that still
	// folded a numeric alias, "1234" would already be "0.0.4.210" by the
	// time this session's own, still-unfolded "1234" tried to match it.
	second := UpdateOrigin(root, "/proj", "sess-2", t0.Add(time.Hour), []string{"1234"}, ssh)
	if second.Err != nil {
		t.Fatal(second.Err)
	}
	if len(second.Novel) != 0 {
		t.Errorf("second render novel = %v, want empty: sess-2 named the same alias sess-1 already owns", second.Novel)
	}
	if second.Known != 1 {
		t.Errorf("known = %d, want 1: one alias, not split into a folded and an unfolded spelling", second.Known)
	}

	f, _, err := load(fileNameFor(t, root, "/proj"), "/proj")
	if err != nil {
		t.Fatal(err)
	}
	e, ok := f.Hosts["1234"]
	if !ok {
		t.Fatalf("hosts = %v, want the literal alias \"1234\", not its inet_aton fold", f.Hosts)
	}
	if !e.SSH {
		t.Error("the surviving entry lost its SSH marker across a load")
	}
}

// TestUpdateOrigin_WireHostStillFolds: a numeric WIRE host -- sshHosts does
// not name it -- still folds and still migrates exactly as Update's own
// tests pin, because UpdateOrigin with every host absent from sshHosts is
// Update.
func TestUpdateOrigin_WireHostStillFolds(t *testing.T) {
	root := t.TempDir()
	writeLegacyBaseline(t, root, "/proj", map[string]entry{
		"167838211": {FirstSeenSession: "sess-0", FirstSeenAtUnixMS: t0.UnixMilli()},
	})

	res := UpdateOrigin(root, "/proj", "sess-1", t0.Add(time.Hour), []string{"10.1.2.3"}, nil)
	if len(res.Novel) != 0 {
		t.Errorf("novel = %v, want empty: 167838211 and 10.1.2.3 are one host", res.Novel)
	}
	if res.Known != 1 {
		t.Errorf("known = %d, want 1: the legacy key and the canonical one must merge", res.Known)
	}
}

// fileNameFor is the path Update/UpdateOrigin reads and writes for key under
// root, for a test that wants to load() the file directly afterwards.
func fileNameFor(t *testing.T, root, key string) string {
	t.Helper()
	return root + "/baseline/" + fileName(key)
}
