package baseline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRubricLLD4_ForgetFileMatchesEverySpelling: a baseline holds a host in
// the spelling its session's records named it in -- one spelling today, the
// call's own in a file written before -- and forget --host must find it from
// whichever spelling the user types. An exact key lookup left 10.1.2.3 in
// every baseline after `forget --host 167838211` and counted none cleared.
func TestRubricLLD4_ForgetFileMatchesEverySpelling(t *testing.T) {
	for _, tc := range []struct{ name, held, asked string }{
		{"inet_aton decimal against today's key", "10.1.2.3", "167838211"},
		{"IPv4-mapped IPv6", "10.1.2.3", "[::ffff:10.1.2.3]"},
		{"expanded upper-case IPv6", "[2001:db8::1]", "[2001:DB8:0:0:0:0:0:1]"},
		{"a name with the root dot and a port", "pypi.org", "PyPI.org.:443"},
		{"a key from before one spelling", "167838211", "10.1.2.3"},
	} {
		root := t.TempDir()
		res := Update(root, "/a-project", "s1", time.UnixMilli(1_000), []string{tc.held, "kept.example"})
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		n, err := ForgetFile(res.Path, tc.asked)
		if err != nil || n != 1 {
			t.Errorf("%s: forget %q from a baseline holding %q cleared %d (%v); want 1", tc.name, tc.asked, tc.held, n, err)
			continue
		}
		hosts := onDisk(t, root, "/a-project")
		if _, ok := hosts[tc.held]; ok {
			t.Errorf("%s: %q is still in the baseline after forgetting %q", tc.name, tc.held, tc.asked)
		}
		if _, ok := hosts["kept.example"]; !ok {
			t.Errorf("%s: the forget took a host it was not asked for", tc.name)
		}
	}
}

// TestRubricLLD4_ForgetFileRemovesAnUnreadableFileNamingAnySpelling: a file
// this build cannot parse is searched as bytes, and it holds hosts in the
// recorded spelling. It goes whole when it names the host in the spelling
// asked for or in the canonical one, and stays when it names neither.
func TestRubricLLD4_ForgetFileRemovesAnUnreadableFileNamingAnySpelling(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body, asked string
		cleared     int
	}{
		{`{"hosts":{"10.1.2.3":{`, "167838211", 1},
		{`{"hosts":{"167838211":{`, "167838211", 1},
		{`{"hosts":{"10.1.2.4":{`, "167838211", 0},
	} {
		path := filepath.Join(dir, "x.json")
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		n, err := ForgetFile(path, tc.asked)
		_, statErr := os.Stat(path)
		gone := os.IsNotExist(statErr)
		if err != nil || n != tc.cleared || gone != (tc.cleared == 1) {
			t.Errorf("forget %q from %s: cleared %d (%v), file gone %v; want %d", tc.asked, tc.body, n, err, gone, tc.cleared)
		}
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRubricLLD4_ForgetFileRefusesWhatIsNotAHost: an argument that names no
// host clears nothing and says so, rather than reading as a forget that ran.
func TestRubricLLD4_ForgetFileRefusesWhatIsNotAHost(t *testing.T) {
	root := t.TempDir()
	res := Update(root, "/a-project", "s1", time.UnixMilli(1_000), []string{"pypi.org"})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	for _, bad := range []string{"deploy:secret@pypi.org", "[pypi.org]", " "} {
		if _, err := ForgetFile(res.Path, bad); err == nil {
			t.Errorf("forget %q was accepted: it names no host", bad)
		}
	}
	if _, ok := onDisk(t, root, "/a-project")["pypi.org"]; !ok {
		t.Error("a refused forget changed the baseline")
	}
}
