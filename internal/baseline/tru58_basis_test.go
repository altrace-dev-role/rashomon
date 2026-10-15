package baseline

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TRU-58. A baseline file from the proxy era holds the hosts a session REACHED
// and carries no basis marker. It is re-established from the record, not
// compared: a host it holds that this session names is not "named for the
// first time", and not "known" either, because the file never said what it
// knew.
func TestTRU58_PreDeclaredBaselineIsReestablishedNotCompared(t *testing.T) {
	root := t.TempDir()
	key := "project"
	dir := filepath.Join(root, "baseline")
	writePreDeclared(t, root, key, "pypi.org")

	res := Update(root, key, "s1", time.UnixMilli(1_000), []string{"pypi.org", "github.com"})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if !res.Established {
		t.Errorf("a marker-less baseline must be re-established by the session that meets it; Established = false")
	}
	if len(res.Novel) != 0 {
		t.Errorf("the establishing session reports no novel host; got %v", res.Novel)
	}
	if res.Known != 2 {
		t.Errorf("Known = %d, want the 2 hosts this session named: the old file's hosts are not carried over", res.Known)
	}

	// The rewritten file carries the marker, so the next session compares.
	f, existed, err := load(filepath.Join(dir, fileName(key)), key)
	if err != nil || !existed || f.Basis != Basis {
		t.Fatalf("after re-establishing: existed=%v basis=%q err=%v", existed, f.Basis, err)
	}
	next := Update(root, key, "s2", time.UnixMilli(2_000), []string{"pypi.org", "crates.io"})
	if next.Established || len(next.Novel) != 1 || next.Novel[0] != "crates.io" {
		t.Errorf("the second session compares against the re-established file: established=%v novel=%v", next.Established, next.Novel)
	}
}

// TRU-58, forget's half. A file without the basis marker is read as absent,
// and it can still hold a forgotten host in plaintext. forget --host must not
// skip it as absent: when it holds the host, the file goes whole rather than
// being rewritten, because rewriting it would stamp the marker onto hosts it
// held under the old basis. When it does not hold the host, this forget has
// no business with it and leaves it untouched.
func TestTRU58_ForgetRemovesAPreDeclaredBaselineWhole(t *testing.T) {
	const forgotten = "acme-secret.internal"
	for _, tc := range []struct {
		name    string
		hosts   []string
		cleared int
		removed bool
	}{
		{"holding the host", []string{forgotten, "pypi.org"}, 1, true},
		{"not holding it", []string{"pypi.org"}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := writePreDeclared(t, root, "project", tc.hosts...)

			n, err := ForgetFile(path, forgotten)
			if err != nil {
				t.Fatalf("ForgetFile: %v", err)
			}
			if n != tc.cleared {
				t.Errorf("ForgetFile cleared %d, want %d", n, tc.cleared)
			}
			_, statErr := os.Stat(path)
			if tc.removed && !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("the pre-declared baseline is still on disk (stat: %v); it holds "+
					"the host, so it goes whole", statErr)
			}
			if !tc.removed && statErr != nil {
				t.Errorf("a pre-declared baseline that never held the host was removed (stat: %v)", statErr)
			}
			assertNoFileNames(t, filepath.Join(root, "baseline"), forgotten)
		})
	}
}

// TestTRU58_ForgetClearsOnlyTheHostFromADeclaredBaseline is the control: a
// file that carries the marker keeps it, and keeps every other host.
func TestTRU58_ForgetClearsOnlyTheHostFromADeclaredBaseline(t *testing.T) {
	root := t.TempDir()
	res := Update(root, "project", "s1", time.UnixMilli(1_000), []string{"acme-secret.internal", "keep.example"})
	if res.Err != nil {
		t.Fatal(res.Err)
	}

	n, err := ForgetFile(res.Path, "acme-secret.internal")
	if err != nil || n != 1 {
		t.Fatalf("ForgetFile = %d, %v; want 1 cleared", n, err)
	}
	assertNoFileNames(t, filepath.Join(root, "baseline"), "acme-secret.internal")
	f, existed, err := load(res.Path, "project")
	if err != nil || !existed || f.Basis != Basis {
		t.Fatalf("after the forget: existed=%v basis=%q err=%v; the file is still this baseline", existed, f.Basis, err)
	}
	if _, ok := f.Hosts["keep.example"]; !ok || len(f.Hosts) != 1 {
		t.Errorf("hosts after the forget = %v, want only keep.example", f.Hosts)
	}
	if again, err := ForgetFile(res.Path, "acme-secret.internal"); err != nil || again != 0 {
		t.Errorf("a second forget = %d, %v; want 0 cleared", again, err)
	}
}

// TestTRU58_ForgetFailsWhenAPreDeclaredBaselineCannotBeRemoved: a file that
// could not be removed is an error, never a quiet zero, because the host is
// still in it.
func TestTRU58_ForgetFailsWhenAPreDeclaredBaselineCannotBeRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	root := t.TempDir()
	path := writePreDeclared(t, root, "project", "acme-secret.internal")
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Error(err)
		}
	})

	n, err := ForgetFile(path, "acme-secret.internal")
	if err == nil || n != 0 {
		t.Errorf("ForgetFile = %d, %v; want an error and nothing counted as cleared", n, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("premise: the file should still be there (stat: %v)", err)
	}
}

// writePreDeclared writes a baseline as the proxy era did, with no basis
// marker, holding hosts, and returns its path.
func writePreDeclared(t *testing.T, root, key string, hosts ...string) string {
	t.Helper()
	dir := filepath.Join(root, "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	entries := map[string]any{}
	for _, h := range hosts {
		entries[h] = map[string]any{"first_seen_session": "proxy-era", "first_seen_at_unix_ms": 1}
	}
	body, err := json.Marshal(map[string]any{
		"version": Version, "project": key,
		"established_by_session": "proxy-era", "established_at_unix_ms": 1,
		"hosts": entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName(key))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// assertNoFileNames fails if any file under dir holds host in its bytes.
func assertNoFileNames(t *testing.T, dir, host string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), host) {
			t.Errorf("%s still holds %q after the forget", e.Name(), host)
		}
	}
}

// TestTRU58_ForgetLeavesAnUnparseableBaselineWithoutTheHost is TRU-58c's
// control: a file that does not parse is deleted only when its bytes hold the
// host. One that does not is no business of this forget, and its project
// keeps reading as unknown, as before.
func TestTRU58_ForgetLeavesAnUnparseableBaselineWithoutTheHost(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName("project"))
	if err := os.WriteFile(path, []byte(`{"version": 1, "hosts": {"pypi.org"`), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := ForgetFile(path, "acme-secret.internal")
	if err != nil || n != 0 {
		t.Errorf("ForgetFile = %d, %v; want 0 cleared and no error", n, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("an unparseable baseline without the host was removed (stat: %v)", err)
	}
}

// TestTRU58_ForgetFailsOnABaselineItCannotRead: a current baseline that cannot
// be read may hold the host, so the forget cannot say it cleared it. An error,
// never a quiet zero. A file that is gone by the time it is read is fine.
func TestTRU58_ForgetFailsOnABaselineItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	root := t.TempDir()
	res := Update(root, "project", "s1", time.UnixMilli(1_000), []string{"acme-secret.internal"})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if err := os.Chmod(res.Path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(res.Path, 0o600); err != nil {
			t.Error(err)
		}
	})

	if n, err := ForgetFile(res.Path, "acme-secret.internal"); err == nil || n != 0 {
		t.Errorf("ForgetFile on an unreadable baseline = %d, %v; want an error and nothing cleared", n, err)
	}
	if n, err := ForgetFile(filepath.Join(root, "baseline", "gone.json"), "acme-secret.internal"); err != nil || n != 0 {
		t.Errorf("ForgetFile on a file that is not there = %d, %v; want 0 and no error", n, err)
	}
}

// TestTRU58_ForgetMatchesTheWholeHostInAnUnparseableBaseline: the byte search
// looks for the host as JSON writes it, quoted, so a short host does not match
// inside another name or inside a key. --host io must not delete a file that
// holds pypi.org and data.io, nor count it as cleared.
func TestTRU58_ForgetMatchesTheWholeHostInAnUnparseableBaseline(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "baseline")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fileName("other"))
	body := `{"version": 2, "basis": "declared", "project": "other", "hosts": {"pypi.org": {}, "data.io": {}}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"io", "a.io", "pypi", "data", "sion"} {
		if n, err := ForgetFile(path, host); err != nil || n != 0 {
			t.Errorf("ForgetFile(%q) = %d, %v; want 0 cleared: the file does not hold that host", host, n, err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a baseline that does not hold the host was removed (stat: %v)", err)
	}
	if n, err := ForgetFile(path, "data.io"); err != nil || n != 1 {
		t.Errorf("ForgetFile(data.io) = %d, %v; want 1: the file holds it", n, err)
	}
}
