package platform

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
// log and digests where LogPath and the digests path name them, and a/a.go
// and b/b.go holding the bytes the digests were taken over.
func synced(t testing.TB) (root, logPath string) {
	t.Helper()
	root = t.TempDir()
	put(t, filepath.Join(root, "go.mod"), []byte("module example.com/m\n\ngo 1.25\n"))
	put(t, filepath.Join(root, "a", "a.go"), []byte("package a\n"))
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

// TestPLW1i_AWrittenFileTheDigestsDoNotVouchForMakesItsPackageStale (card
// PL-WIRE1 e, PF-PL6 first half): the known-answer set. A package is stale,
// its hash withheld, where a written .go file of it differs from its
// recorded digest, is not recorded, or is gone, or where the digests are
// missing, under another header, malformed, or past the hash cap; a written
// _test.go file, which no sync reads, never makes one stale, and neither does
// a file whose bytes match; the synced bytes back give the hash back (a
// re-sync is the acceptance item's). One case per term of staleOf's
// condition. Break: compare nothing, and an edited package keeps the hash of
// code it no longer holds.
func TestPLW1i_AWrittenFileTheDigestsDoNotVouchForMakesItsPackageStale(t *testing.T) {
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
		{"a test file alone", func(r string) { put(t, filepath.Join(r, "a", "a_test.go"), []byte("package a\n")) },
			[]string{"a/a_test.go"}, pkgA + "=known"},
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
		var written []string
		var digests strings.Builder
		digests.Write(fixtureDigests(t))
		for i := 0; i < maxHashed; i++ {
			p := fmt.Sprintf("a/f%02d.go", i)
			put(t, filepath.Join(root, filepath.FromSlash(p)), []byte("package a\n"))
			fmt.Fprintf(&digests, "%s\t%s\t%s\n", pkgA, p, sumOf("package a\n"))
			written = append(written, p)
		}
		put(t, digestsPath(root), []byte(digests.String()))
		got := factsOf(Read(RunView{Root: root, WrittenPaths: append(written, "b/b.go"), LogPath: lp}))
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
	if err != nil || len(got) != 2 || got["a/a.go"] != sumOf("package a\n") || got["b/b.go"] != sumOf("package b\n") {
		t.Errorf("parseDigests = %v, %v; want a/a.go and b/b.go by their sha256", got, err)
	}
}

// TestPLW1k_PathsArePgraphs: the log and digests this package reads are where
// pgraph sync writes them, root/.rashomon/pgraph.log and pgraph.digests, and
// the header is the one pgraph writes: the names are the boundary, read here
// from pgraph's own source. Break: move the log, and the hook reads none.
func TestPLW1k_PathsArePgraphs(t *testing.T) {
	if got, want := LogPath("/r"), filepath.Join("/r", ".rashomon", "pgraph.log"); got != want {
		t.Errorf("LogPath = %s, want %s", got, want)
	}
	if got, want := digestsPath("/r"), filepath.Join("/r", ".rashomon", "pgraph.digests"); got != want {
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
	if got, err := parseDigests(strings.NewReader(DigestsHeader + "\n")); err != nil || got == nil || len(got) != 0 {
		t.Errorf("the header alone: %v, %v; want an empty set", got, err)
	}
	if got, err := parseDigests(strings.NewReader(DigestsHeader + "\np\ta.go\t" + sum + "\n")); err != nil || got["a.go"] != sum {
		t.Errorf("one good line: %v, %v", got, err)
	}
}
