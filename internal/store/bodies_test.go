package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Card 4.1's store rows (PF-RC14). The caller names the project twice: its
// key, which the switch lists, and its directory, the one path element its
// copies live under (internal/baseline imports this package, so the caller
// computes both from baseline.Key and baseline.FileName).
const (
	rc14Key = "/work/proj"
	rc14Dir = "work_proj-0123456789ab"
)

func rc14Store(t *testing.T, set *BodiesSetting) *Store {
	t.Helper()
	root := t.TempDir()
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if set != nil {
		if err := WriteBodies(root, *set); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func rc14Source(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func rc14Body(st *Store, dir, session string) string {
	return filepath.Join(st.Root(), DirBodies, dir, st.DirName(session)+".jsonl")
}

func rc14NoBodies(t *testing.T, st *Store) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(st.Root(), DirBodies)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s exists or cannot be checked (%v); nothing should have been written", DirBodies, err)
	}
}

// TestRC14a_OffByDefaultWritesNothing: with no marker the switch is off, and
// the copy neither reads the transcript nor makes a directory.
func TestRC14a_OffByDefaultWritesNothing(t *testing.T) {
	st := rc14Store(t, nil)
	wrote, err := st.CopyBody("s1", rc14Source(t, []byte("{}\n")), rc14Key, rc14Dir, time.Now())
	if wrote || err != nil {
		t.Fatalf("CopyBody with no marker = (%v, %v), want (false, nil)", wrote, err)
	}
	rc14NoBodies(t, st)
}

// TestRC14f_ProjectNotListedWritesNothing: the switch is per project; a
// marker that lists another key copies nothing for this one.
func TestRC14f_ProjectNotListedWritesNothing(t *testing.T) {
	st := rc14Store(t, &BodiesSetting{MaxBytes: 1 << 20, Projects: []string{"/work/other"}})
	wrote, err := st.CopyBody("s1", rc14Source(t, []byte("{}\n")), rc14Key, rc14Dir, time.Now())
	if wrote || err != nil {
		t.Fatalf("CopyBody for an unlisted project = (%v, %v), want (false, nil)", wrote, err)
	}
	rc14NoBodies(t, st)
}

// TestRC14b_CopyIsByteExactAnd0600: a transcript is bytes, not records: a NUL
// and a torn final line copy as they are, the copy is the user's alone, and
// the temporary file the rename consumed is gone. A directory that is not one
// path element is refused before anything is written.
func TestRC14b_CopyIsByteExactAnd0600(t *testing.T) {
	st := rc14Store(t, &BodiesSetting{MaxBytes: 1 << 30, Projects: []string{rc14Key}})
	var src bytes.Buffer
	for src.Len() < 3<<20 {
		src.WriteString(`{"type":"user","message":{"content":"x"}}` + "\n")
	}
	src.WriteByte(0)
	src.WriteString(`{"type":"assistant","message":{"content":"tor`)
	wrote, err := st.CopyBody("s1", rc14Source(t, src.Bytes()), rc14Key, rc14Dir, time.Now())
	if !wrote || err != nil {
		t.Fatalf("CopyBody = (%v, %v), want (true, nil)", wrote, err)
	}
	dst := rc14Body(st, rc14Dir, "s1")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src.Bytes()) {
		t.Errorf("the copy is %d bytes and differs from the %d-byte source", len(got), src.Len())
	}
	if fi, err := os.Stat(dst); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the copy's mode is %v (%v), want 0600", fi.Mode().Perm(), err)
	}
	entries, err := os.ReadDir(filepath.Dir(dst))
	if err != nil || len(entries) != 1 {
		t.Errorf("the project directory holds %d entries (%v), want the copy alone", len(entries), err)
	}

	for _, dir := range []string{"", ".", "..", "a/b", "../x", `a\b`} {
		wrote, err := st.CopyBody("s2", rc14Source(t, []byte("{}\n")), rc14Key, dir, time.Now())
		if wrote || err == nil {
			t.Errorf("CopyBody into directory %q = (%v, %v), want a refusal", dir, wrote, err)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(st.Root(), DirBodies)); len(entries) != 1 {
		t.Errorf("%s holds %d entries after the refusals, want the one project", DirBodies, len(entries))
	}
}

// TestRC14c_CapEvictsOldestFirst: the cap holds across every project's
// copies. Three 4 KiB copies fill a 12 KiB cap exactly; a fourth removes the
// oldest by mtime and no other. A source exactly at the cap copies once
// everything else is gone; one byte over is refused and removes nothing
// (strict > on refusal, <= on the eviction target).
func TestRC14c_CapEvictsOldestFirst(t *testing.T) {
	const kib = 1 << 10
	st := rc14Store(t, &BodiesSetting{MaxBytes: 12 * kib, Projects: []string{rc14Key}})
	now := time.Now()
	four := bytes.Repeat([]byte("a"), 4*kib)
	// The user's file, oldest of all and a copy's size: counted, it would be
	// evicted before s1; it is neither counted nor removed.
	foreign := filepath.Join(st.Root(), DirBodies, rc14Dir, "notes.txt")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, four, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(foreign, now.Add(-24*time.Hour), now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i, s := range []string{"s1", "s2", "s3"} {
		if wrote, err := st.CopyBody(s, rc14Source(t, four), rc14Key, rc14Dir, now); !wrote || err != nil {
			t.Fatalf("CopyBody(%s) = (%v, %v)", s, wrote, err)
		}
		at := now.Add(time.Duration(i-3) * time.Hour)
		if err := os.Chtimes(rc14Body(st, rc14Dir, s), at, at); err != nil {
			t.Fatal(err)
		}
	}
	if wrote, err := st.CopyBody("s4", rc14Source(t, four), rc14Key, rc14Dir, now); !wrote || err != nil {
		t.Fatalf("CopyBody(s4) = (%v, %v)", wrote, err)
	}
	for s, want := range map[string]bool{"s1": false, "s2": true, "s3": true, "s4": true} {
		if _, err := os.Stat(rc14Body(st, rc14Dir, s)); (err == nil) != want {
			t.Errorf("%s present = %v, want %v", s, err == nil, want)
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("eviction removed the user's file: %v", err)
	}

	if wrote, err := st.CopyBody("s5", rc14Source(t, bytes.Repeat([]byte("b"), 12*kib)), rc14Key, rc14Dir, now); !wrote || err != nil {
		t.Fatalf("CopyBody of a source at the cap = (%v, %v), want (true, nil)", wrote, err)
	}
	entries, _ := os.ReadDir(filepath.Join(st.Root(), DirBodies, rc14Dir))
	if len(entries) != 2 {
		t.Errorf("after a copy at the cap the project holds %d entries, want s5 and the user's file", len(entries))
	}

	wrote, err := st.CopyBody("s6", rc14Source(t, bytes.Repeat([]byte("c"), 12*kib+1)), rc14Key, rc14Dir, now)
	if wrote || !errors.Is(err, ErrBodyOverCap) {
		t.Fatalf("CopyBody one byte over the cap = (%v, %v), want (false, ErrBodyOverCap)", wrote, err)
	}
	if _, err := os.Stat(rc14Body(st, rc14Dir, "s5")); err != nil {
		t.Errorf("the refused copy removed s5: %v", err)
	}
}

// TestRC14h_ForgetBodiesRemovesTheProjectOnly: forget --bodies is per
// project; another project's copies stay.
func TestRC14h_ForgetBodiesRemovesTheProjectOnly(t *testing.T) {
	const otherKey, otherDir = "/work/other", "work_other-ba9876543210"
	st := rc14Store(t, &BodiesSetting{MaxBytes: 1 << 20, Projects: []string{rc14Key, otherKey}})
	for _, c := range []struct{ session, key, dir string }{{"s1", rc14Key, rc14Dir}, {"s2", rc14Key, rc14Dir}, {"s3", otherKey, otherDir}} {
		if wrote, err := st.CopyBody(c.session, rc14Source(t, []byte("{}\n")), c.key, c.dir, time.Now()); !wrote || err != nil {
			t.Fatalf("CopyBody(%s) = (%v, %v)", c.session, wrote, err)
		}
	}
	removed, err := st.ForgetBodies(rc14Dir)
	if removed != 2 || err != nil {
		t.Fatalf("ForgetBodies = (%d, %v), want (2, nil)", removed, err)
	}
	if _, err := os.Stat(rc14Body(st, rc14Dir, "s1")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("s1 survived its project's forget: %v", err)
	}
	if _, err := os.Stat(rc14Body(st, otherDir, "s3")); err != nil {
		t.Errorf("the other project's copy went too: %v", err)
	}
}
