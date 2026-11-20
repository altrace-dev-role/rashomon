package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// foldsCase reports whether dir's disk finds an entry under another case of
// its name, as the macOS default disk does: it makes a probe folder and
// Lstats it in upper case.
func foldsCase(t *testing.T, dir string) bool {
	t.Helper()
	p := filepath.Join(dir, "case-probe")
	if err := os.Mkdir(p, 0o700); err != nil {
		t.Fatal(err)
	}
	_, lerr := os.Lstat(filepath.Join(dir, "CASE-PROBE"))
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	return lerr == nil
}

// TestMkdirExact_MakesTheFolderOnce (CLEAR-EXACT1): MkdirExact makes every
// missing level and returns the joined path; a second call finds the folder
// and makes nothing more. Break: return another path, or refuse a real
// folder that is already there.
func TestMkdirExact_MakesTheFolderOnce(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "top", "mid", "low")
	for call := 1; call <= 2; call++ {
		got, err := MkdirExact(root, 2, "top", "mid", "low")
		if err != nil || got != want {
			t.Fatalf("call %d: error %v, the joined path %v; want the joined path and no error", call, err != nil, got == want)
		}
	}
	if fi, err := os.Lstat(want); err != nil || !fi.IsDir() {
		t.Errorf("top/mid/low is not a folder")
	}
	if got := strings.Join(names(t, root), " "); got != "top" {
		t.Errorf("the root holds %q; want top alone", got)
	}
}

// TestMkdirExact_RefusesALinkOrAFileAtAnyLevel (CLEAR-EXACT1, the link RED):
// a link at any level under the root, to a folder outside the store or
// inside it, or a plain file where a folder belongs, is refused with
// ErrNotExactDir, and nothing is made through the link. os.MkdirAll starts
// with Stat, which follows the link, so it makes the missing levels at the
// link's target, where purge never names them. Break: Stat in place of
// Lstat, or check only some of the levels.
func TestMkdirExact_RefusesALinkOrAFileAtAnyLevel(t *testing.T) {
	segs := []string{"top", "mid", "low"}
	for _, c := range []struct {
		name         string
		at           int
		inside, file bool
	}{
		{"a link at level 1 to a folder outside the store", 1, false, false},
		{"a link at level 2 to a folder inside the store", 2, true, false},
		{"a link at level 3 to a folder outside the store", 3, false, false},
		{"a plain file at level 2", 2, false, true},
	} {
		root, target := t.TempDir(), t.TempDir()
		parent := filepath.Join(append([]string{root}, segs[:c.at-1]...)...)
		if err := os.MkdirAll(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		if c.inside {
			target = filepath.Join(root, "elsewhere")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		at := filepath.Join(parent, segs[c.at-1])
		if c.file {
			touch(t, at)
		} else if err := os.Symlink(target, at); err != nil {
			t.Fatal(err)
		}
		_, err := MkdirExact(root, 2, segs...)
		if !errors.Is(err, ErrNotExactDir) {
			t.Errorf("%s: refused false, no error %v; want ErrNotExactDir", c.name, err == nil)
		}
		if n := len(names(t, target)); !c.file && n != 0 {
			t.Errorf("%s: the link's target holds %d entries; want none", c.name, n)
		}
	}
}

// TestMkdirExact_RefusesAnotherCaseAtAnExactLevel (CLEAR-EXACT1, the case
// RED): at a level purge judges by exact name, a folder there under another
// case of the name is refused with ErrNotExactDir on a disk that folds case,
// and nothing is made in it; a disk that does not fold makes the asked name
// beside it. Below the exact levels any case is taken, as purge takes it.
// os.MkdirAll takes the other case's folder on a folding disk. Break: Lstat
// alone, a listing compared with strings.EqualFold, or a listing at every
// level.
func TestMkdirExact_RefusesAnotherCaseAtAnExactLevel(t *testing.T) {
	folds := foldsCase(t, t.TempDir())
	t.Logf("the test disk folds case: %v", folds)
	for _, c := range []struct {
		name  string
		plant []string
		exact bool
	}{
		{"TOP at level 1", []string{"TOP"}, true},
		{"top/MID at level 2", []string{"top", "MID"}, true},
		{"top/mid/LOW at level 3", []string{"top", "mid", "LOW"}, false},
	} {
		root := t.TempDir()
		planted := filepath.Join(append([]string{root}, c.plant...)...)
		if err := os.MkdirAll(planted, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := MkdirExact(root, 2, "top", "mid", "low")
		if folds && c.exact {
			if !errors.Is(err, ErrNotExactDir) {
				t.Errorf("%s: refused false, no error %v; want ErrNotExactDir", c.name, err == nil)
			}
			if n := len(names(t, planted)); n != 0 {
				t.Errorf("%s: the folder under another case holds %d entries; want none", c.name, n)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused %v; want top/mid/low made", c.name, errors.Is(err, ErrNotExactDir))
		}
		if fi, err := os.Lstat(filepath.Join(root, "top", "mid", "low")); err != nil || !fi.IsDir() {
			t.Errorf("%s: top/mid/low is not a folder", c.name)
		}
	}
}

// TestMeetClear_RefusesALinkOrAnotherCase (CLEAR-EXACT1): MeetClear writes
// nothing unless clear/ and clear/<digest of the cwd>/ are real folders
// listed under exactly those names: a link at either level, or on a disk
// that folds case a folder there under another case, is refused with
// ErrNotExactDir and links nothing. Purge removes only clear/ and its
// lowercase digest folders, so an arm written through a link or into
// another case's folder is a file that purge leaves. Break: make the folder
// with os.MkdirAll, or call MkdirExact with fewer than two exact levels.
func TestMeetClear_RefusesALinkOrAnotherCase(t *testing.T) {
	folds := foldsCase(t, t.TempDir())
	t.Logf("the test disk folds case: %v", folds)
	now := time.Now()
	const oldID = "0b6a9c52-1111-4222-8333-444455556666"
	for _, c := range []struct {
		name    string
		variant bool
		plant   func(root, digest, out string) string
	}{
		{"clear/ is a link to a folder outside the store", false, func(root, digest, out string) string {
			if err := os.Symlink(out, filepath.Join(root, "clear")); err != nil {
				t.Fatal(err)
			}
			return out
		}},
		{"clear/<digest> is a link to a folder inside the store", false, func(root, digest, out string) string {
			in := filepath.Join(root, "elsewhere")
			for _, d := range []string{in, filepath.Join(root, "clear")} {
				if err := os.Mkdir(d, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(in, filepath.Join(root, "clear", digest)); err != nil {
				t.Fatal(err)
			}
			return in
		}},
		{"Clear/ under another case", true, func(root, digest, out string) string {
			p := filepath.Join(root, "Clear")
			if err := os.Mkdir(p, 0o700); err != nil {
				t.Fatal(err)
			}
			return p
		}},
		{"the digest folder in upper case", true, func(root, digest, out string) string {
			p := filepath.Join(root, "clear", strings.ToUpper(digest))
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
			return p
		}},
	} {
		root, out := t.TempDir(), t.TempDir()
		cwd := filepath.Join(root, "w", "p")
		sum := sha256.Sum256([]byte(cwd))
		digest := hex.EncodeToString(sum[:])
		planted := c.plant(root, digest, out)
		l, ok, err := MeetClear(root, cwd, ClearEnd, oldID, now)
		if ok || l != (ClearLink{}) {
			t.Errorf("%s: MeetClear linked %v; want no link", c.name, ok)
		}
		if c.variant && !folds {
			if err != nil {
				t.Errorf("%s: refused %v on a disk that does not fold case; want the arm written", c.name, errors.Is(err, ErrNotExactDir))
			}
			if _, err := os.Lstat(filepath.Join(root, "clear", digest, "end-"+oldID)); err != nil {
				t.Errorf("%s: the end arm is not in clear/<digest>", c.name)
			}
			if n := len(names(t, planted)); n != 0 {
				t.Errorf("%s: the folder under another case holds %d entries; want none", c.name, n)
			}
			continue
		}
		if !errors.Is(err, ErrNotExactDir) {
			t.Errorf("%s: refused false, no error %v; want ErrNotExactDir", c.name, err == nil)
		}
		if n := len(names(t, planted)); n != 0 {
			t.Errorf("%s: the folder written through holds %d entries; want none", c.name, n)
		}
	}
}
