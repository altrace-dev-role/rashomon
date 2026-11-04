package platform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// sumOf is the hex sha256 of s.
func sumOf(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// fixtureDigests is what pgraph's encodeDigests writes for a/a.go and b/b.go
// as synced() writes them (pgraph's TestPLW1d_DigestsFormat writes the same
// lines from its side).
func fixtureDigests(t testing.TB) []byte {
	t.Helper()
	return []byte(DigestsHeader + "\n" +
		pkgA + "\ta/a.go\t" + sumOf("package a\n") + "\n" +
		pkgB + "\tb/b.go\t" + sumOf("package b\n") + "\n")
}

func put(t testing.TB, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// synced is the module example.com/m as pgraph sync leaves it: the fixture
// log and digests where LogPath and the digests path name them, a/a.go and
// b/b.go holding the bytes the digests were taken over, and a test file in a
// alone.
func synced(t testing.TB) (root, logPath string) {
	t.Helper()
	root = t.TempDir()
	put(t, filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.25\n"))
	put(t, filepath.Join(root, "a", "a.go"), []byte("package a\n"))
	put(t, filepath.Join(root, "a", "a_test.go"), []byte("package a\n"))
	put(t, filepath.Join(root, "b", "b.go"), []byte("package b\n"))
	logPath = LogPath(root)
	put(t, logPath, fixtureLog(t))
	put(t, digestsPath(root), fixtureDigests(t))
	return root, logPath
}

// factsOf is each family's package and facts state from Read.
func factsOf(r verify.PlatformRead) string {
	var b strings.Builder
	for _, f := range r.Families {
		fmt.Fprintf(&b, "%s=%s ", f.Package, f.Facts)
	}
	return strings.TrimSpace(b.String())
}

// TestPLW1i_AFileTheDigestsDoNotVouchForMakesItsPackageStale (card PL-WIRE1
// e, S3): the known-answer set. A package is stale, its hash withheld, where
// any file the digests list for it differs from its recorded digest or is
// gone, whether or not the session wrote it; where a .go file of it the
// session wrote is not listed; or where the digests are missing, under
// another header, malformed, or past the hash cap. A written _test.go file,
// which no sync reads, never makes one stale, and neither does a file whose
// bytes match; the synced bytes back give the hash back (a re-sync is the
// acceptance item's). One case per term of fresh's condition. Break: hash
// only the files the session wrote, and a package changed by `git pull`
// keeps the hash of code it no longer holds.
func TestPLW1i_AFileTheDigestsDoNotVouchForMakesItsPackageStale(t *testing.T) {
	for _, c := range []struct {
		name    string
		edit    func(root string)
		written []string
		want    string
	}{
		{"matching bytes", nil, []string{"a/a.go", "b/b.go"}, pkgA + "=known " + pkgB + "=known"},
		{"edited file", func(r string) { put(t, filepath.Join(r, "a", "a.go"), []byte("package a\n\nfunc New() {}\n")) },
			[]string{"a/a.go", "b/b.go"}, pkgA + "=stale " + pkgB + "=known"},
		{"new file in a known package", func(r string) { put(t, filepath.Join(r, "a", "new.go"), []byte("package a\n")) },
			[]string{"a/new.go"}, pkgA + "=stale"},
		{"deleted file", func(r string) { os.Remove(filepath.Join(r, "b", "b.go")) }, []string{"b/b.go"}, pkgB + "=stale"},
		{"a test file alone", func(r string) { put(t, filepath.Join(r, "a", "a_test.go"), []byte("package a\n\n// x\n")) },
			[]string{"a/a_test.go"}, pkgA + "=known"},
		{"an unwritten file of the package changed", func(r string) { put(t, filepath.Join(r, "a", "a.go"), []byte("package a\n\nvar X int\n")) },
			[]string{"a/a_test.go"}, pkgA + "=stale"},
		{"an absolute path matching", nil, nil, pkgB + "=known"},
		{"no digests", func(r string) { os.Remove(digestsPath(r)) }, []string{"a/a.go"}, pkgA + "=stale"},
		{"digests under another header", func(r string) {
			put(t, digestsPath(r), bytes.Replace(fixtureDigests(t), []byte("v0"), []byte("v1"), 1))
		}, []string{"a/a.go"}, pkgA + "=stale"},
		{"malformed digests", func(r string) { put(t, digestsPath(r), []byte(DigestsHeader+"\nexample.com/m/a\ta/a.go\n")) },
			[]string{"a/a.go"}, pkgA + "=stale"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, lp := synced(t)
			if c.edit != nil {
				c.edit(root)
			}
			written := c.written
			if written == nil {
				written = []string{filepath.Join(root, "b", "b.go")}
			}
			if got := factsOf(Read(RunView{Root: root, WrittenPaths: written, LogPath: lp})); got != c.want {
				t.Errorf("Read(%q) = %s, want %s", written, got, c.want)
			}
		})
	}
	t.Run("past the hash cap", func(t *testing.T) {
		root, lp := synced(t)
		var digests strings.Builder
		digests.Write(fixtureDigests(t))
		for i := 1; i < maxHashed; i++ {
			p := fmt.Sprintf("a/f%03d.go", i)
			put(t, filepath.Join(root, filepath.FromSlash(p)), []byte("package a\n"))
			fmt.Fprintf(&digests, "%s\t%s\t%s\n", pkgA, p, sumOf("package a\n"))
		}
		put(t, digestsPath(root), []byte(digests.String()))
		got := factsOf(Read(RunView{Root: root, WrittenPaths: []string{"a/a.go", "b/b.go"}, LogPath: lp}))
		if want := pkgA + "=known " + pkgB + "=stale"; got != want {
			t.Errorf("%d matching files and one more: %s, want %s (the one past the cap unknown)", maxHashed, got, want)
		}
	})
	t.Run("the synced bytes back", func(t *testing.T) {
		root, lp := synced(t)
		put(t, filepath.Join(root, "a", "a.go"), []byte("package a\n\nfunc New() {}\n"))
		if got := factsOf(Read(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp})); got != pkgA+"=stale" {
			t.Fatalf("after the edit: %s", got)
		}
		put(t, filepath.Join(root, "a", "a.go"), []byte("package a\n"))
		r := Read(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp})
		if factsOf(r) != pkgA+"=known" || r.Families[0].Hash != hashA {
			t.Errorf("with the synced bytes back: %+v, want a known with its hash", r)
		}
	})
}

// TestPLW1j_DigestsParseAsPgraphWritesThem: the lines pgraph's
// encodeDigests writes (TestPLW1d_DigestsFormat) parse here to each path's
// sha256. Break: read the package field as the path, and no file is found.
func TestPLW1j_DigestsParseAsPgraphWritesThem(t *testing.T) {
	got, err := parseDigests(bytes.NewReader(fixtureDigests(t)))
	if err != nil || len(got.files) != 2 || got.files["a/a.go"] != sumOf("package a\n") || got.files["b/b.go"] != sumOf("package b\n") ||
		!slices.Equal(got.byPkg[pkgA], []string{"a/a.go"}) || !slices.Equal(got.byPkg[pkgB], []string{"b/b.go"}) {
		t.Errorf("parseDigests = %+v, %v; want a/a.go and b/b.go by their sha256 and package", got, err)
	}
}

// TestPLW1k_PathsArePgraphs: the log and digests this package reads are where
// pgraph sync writes them, root/.pgraph/pgraph.log and pgraph.digests, and
// the header is the one pgraph writes: the names are the boundary, read here
// from pgraph's own source. Break: move the log, and the verb reads none.
func TestPLW1k_PathsArePgraphs(t *testing.T) {
	if got, want := LogPath("/r"), filepath.Join("/r", ".pgraph", "pgraph.log"); got != want {
		t.Errorf("LogPath = %s, want %s", got, want)
	}
	if got, want := digestsPath("/r"), filepath.Join("/r", ".pgraph", "pgraph.digests"); got != want {
		t.Errorf("digestsPath = %s, want %s", got, want)
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "pgraph", "syncroot.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`StateDir    = "` + stateDir + `"`, `LogFile     = "` + logFile + `"`,
		`DigestsFile = "` + digestsFile + `"`, `const DigestsHeader = "` + DigestsHeader + `"`} {
		if !bytes.Contains(src, []byte(s)) {
			t.Errorf("pgraph/syncroot.go does not declare %s", s)
		}
	}
}

// TestPLW1l_DigestsParserRefusesWhatItCannotReadWhole: a torn last line, a
// line of two fields or four, an empty field, a sha256 that is short, upper
// case or not hex, a path named twice and a line over the cap are each
// refused; the header alone is an empty set.
func TestPLW1l_DigestsParserRefusesWhatItCannotReadWhole(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	for name, body := range map[string]string{
		"torn":      "p\ta.go\t" + sum,
		"two":       "p\t" + sum + "\n",
		"four":      "p\ta.go\t" + sum + "\tx\n",
		"empty":     "\ta.go\t" + sum + "\n",
		"short":     "p\ta.go\t" + sum[:62] + "\n",
		"upper":     "p\ta.go\t" + strings.ToUpper(sum) + "\n",
		"not hex":   "p\ta.go\t" + strings.Repeat("zz", 32) + "\n",
		"twice":     "p\ta.go\t" + sum + "\nq\ta.go\t" + sum + "\n",
		"over line": "p\t" + strings.Repeat("x", maxDigestLine) + "\t" + sum + "\n",
	} {
		if got, err := parseDigests(strings.NewReader(DigestsHeader + "\n" + body)); err == nil {
			t.Errorf("%s: parsed to %v, want refused", name, got)
		}
	}
	if got, err := parseDigests(strings.NewReader(DigestsHeader + "\n")); err != nil || got == nil || len(got.files) != 0 {
		t.Errorf("the header alone: %v, %v; want an empty set", got, err)
	}
	if got, err := parseDigests(strings.NewReader(DigestsHeader + "\np\ta.go\t" + sum + "\n")); err != nil || got.files["a.go"] != sum {
		t.Errorf("one good line: %v, %v", got, err)
	}
}

// TestPW1b2_TheStopsGuardReplacesTheCrashGuard: a RunView's Deadline cuts
// a held decode at its own bound, not the crash guard's (PW1b-ON-230 step
// 2: the Stop passes 2 s inside its entry's 10 s timeout). With the crash
// guard at 10 s and the run's at 20 ms, Read is timed out within a second.
// Break: ignore run.Deadline, and Read waits out the 2 s hold and reads the
// log.
func TestPW1b2_TheStopsGuardReplacesTheCrashGuard(t *testing.T) {
	root, lp := project(t, synthLog(t, 5000))
	hang(t)
	deadline = 10 * time.Second // hang's guard restores it
	start := time.Now()
	r := Read(RunView{Root: root, WrittenPaths: []string{"a/a.go"}, LogPath: lp, Deadline: 20 * time.Millisecond})
	if elapsed := time.Since(start); r.Log != verify.LogTimedOut || elapsed > time.Second {
		t.Errorf("a held decode under a 20 ms Deadline: %v after %v, want timed out within a second", r.Log, elapsed)
	}
}
