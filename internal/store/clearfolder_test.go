package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMeetClear_FolderIsTheDigestOfTheCwd (x35.5, x41.2): MeetClear keeps a
// /clear's files in <root>/clear/<hex of the SHA-256 of the cwd's path>/, so
// no name under the store holds the cwd. Two sides in one cwd pair, and the
// link holds the old id; a side in another cwd never pairs with them, and a
// visit sweeps only its own folder. A cwd that is empty or not absolute
// links nothing and writes nothing. The expected folder is computed here
// from crypto/sha256, not from the store's own helper. Break: name the
// folder by the cwd or the repository id, share one folder across cwds,
// sweep another folder, or take a relative cwd.
func TestMeetClear_FolderIsTheDigestOfTheCwd(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	cwd, other := filepath.Join(root, "w", "p"), filepath.Join(root, "w", "q")
	folder := func(c string) string {
		s := sha256.Sum256([]byte(c))
		return filepath.Join(root, "clear", hex.EncodeToString(s[:]))
	}
	const (
		oldID  = "0b6a9c52-1111-4222-8333-444455556666"
		newID  = "7c1d2e3f-aaaa-4bbb-8ccc-ddddeeeeffff"
		loneID = "5e4d3c2b-9999-4888-8777-666655554444"
	)
	if _, _, err := MeetClear(root, cwd, ClearEnd, oldID, now); err != nil {
		t.Fatalf("MeetClear(end): %v", err)
	}
	if _, err := os.Lstat(filepath.Join(folder(cwd), "end-"+oldID)); err != nil {
		t.Errorf("the end arm is not in clear/<digest of the cwd>")
	}
	if l, ok, err := MeetClear(root, other, ClearStart, loneID, now); err != nil || ok || l != (ClearLink{}) {
		t.Errorf("a start in another cwd = %+v, %v, %v; want no pair", l, ok, err)
	}
	if err := os.MkdirAll(folder(other), 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(folder(other), "end-"+loneID)
	touch(t, stale)
	backdate(t, stale, now, 2*ClearKeep)
	l, ok, err := MeetClear(root, cwd, ClearStart, newID, now)
	if err != nil || !ok || !l.Wrote || l.Old != oldID || l.New != newID {
		t.Errorf("a start in the end's cwd = %+v, %v, %v; want %s continued by %s, written", l, ok, err, oldID, newID)
	}
	if b, err := os.ReadFile(filepath.Join(folder(cwd), "link-"+newID)); err != nil || string(b) != oldID {
		t.Errorf("clear/<digest of the cwd>/link-<new id> holds %q; want the old id", b)
	}
	if _, err := os.Lstat(stale); err != nil {
		t.Errorf("a visit in one cwd's folder swept an old entry of another's")
	}
	for _, bad := range []string{"", filepath.Join("w", "p")} {
		r := t.TempDir()
		if l, ok, err := MeetClear(r, bad, ClearEnd, oldID, now); err != nil || ok || l != (ClearLink{}) {
			t.Errorf("MeetClear with cwd %q = %+v, %v, %v; want nothing", bad, l, ok, err)
		}
		if _, err := os.Lstat(filepath.Join(r, "clear")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("MeetClear with cwd %q made clear/", bad)
		}
	}
}
