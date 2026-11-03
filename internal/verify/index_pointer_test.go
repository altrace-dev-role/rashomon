package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRP2zc_TheIndexNamesTheLastTwoSessions (card REPORT1b): a Stop's
// NoteCheckpoint makes the checkpoint Last and the Last of another session
// Prior; a second checkpoint of the same session (a clear) leaves Prior alone;
// PreviousOf is Last unless Last is the current session's, then Prior, and
// nil when neither is an earlier session's. The pointers survive the write and
// read of their file (INDEX-POINTER-FILE1). Break: overwrite Prior on every
// note, or let PreviousOf return the current session.
func TestRP2zc_TheIndexNamesTheLastTwoSessions(t *testing.T) {
	var c Checkpoints
	if c.PreviousOf("s2") != nil || (*Checkpoints)(nil).PreviousOf("s2") != nil {
		t.Fatal("a file with no pointer names no session")
	}
	c.NoteCheckpoint("s1", 4, 100)
	if p := c.PreviousOf("s2"); p == nil || p.SessionID != "s1" || p.PointSeq != 4 || p.AtMS != 100 {
		t.Errorf("after s1: %+v", p)
	}
	if c.PreviousOf("s1") != nil {
		t.Error("s1's own checkpoint is not the session before s1")
	}
	c.NoteCheckpoint("s2", 9, 200)
	c.NoteCheckpoint("s2", 11, 300) // a clear continued s2
	if c.Last.SessionID != "s2" || c.Last.PointSeq != 11 || c.Prior == nil || c.Prior.SessionID != "s1" || c.Prior.PointSeq != 4 {
		t.Errorf("Last %+v Prior %+v; want s2 at 11 and s1 at 4", c.Last, c.Prior)
	}
	if p := c.PreviousOf("s2"); p == nil || p.SessionID != "s1" {
		t.Errorf("during s2 the previous session is s1: %+v", p)
	}
	if p := c.PreviousOf("s3"); p == nil || p.SessionID != "s2" {
		t.Errorf("during s3 the previous session is s2: %+v", p)
	}

	path := CheckpointsPath(t.TempDir(), "/p")
	if err := WriteCheckpoints(path, &c); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCheckpoints(path)
	if err != nil || got == nil || got.Last == nil || got.Prior == nil || *got.Last != *c.Last || *got.Prior != *c.Prior {
		t.Fatalf("the pointers after a write and read: %+v err %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file's mode: %v, %v; want 0600", info, err)
	}
}

// TestIPF1c_TheCheckpointFileReadsLaterKeysAndHoldsItsCap
// (INDEX-POINTER-FILE1): the checkpoint file's reader drops a key it does not
// name, so a later build's addition costs this one nothing, and keeps the
// index reader's other checks: one value, nothing after it, and the cap on
// read; the writer refuses an encoding over the cap and writes nothing; a
// missing file is no pointer and no error. Break: refuse an unknown key, or
// lift either cap.
func TestIPF1c_TheCheckpointFileReadsLaterKeysAndHoldsItsCap(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	const last = `"last":{"session_id":"s1","point_seq":4,"at_unix_ms":100,"later":1}`
	c, err := ReadCheckpoints(write("later.json", `{`+last+`,"later":{"x":[1]}}`+"\n"))
	if err != nil || c == nil || c.Last == nil || c.Last.SessionID != "s1" || c.Last.PointSeq != 4 {
		t.Errorf("a file with keys this build does not name: %+v, %v; want Last read", c, err)
	}
	for name, body := range map[string]string{
		"two.json":     `{` + last + `}{}`,
		"after.json":   `{` + last + `} x`,
		"over.json":    `{` + last + `}` + strings.Repeat(" ", MaxCheckpointsBytes),
		"not-one.json": `[1]`,
	} {
		if c, err := ReadCheckpoints(write(name, body)); err == nil || c != nil {
			t.Errorf("%s: %+v, %v; want no pointer and an error", name, c, err)
		}
	}
	if c, err := ReadCheckpoints(filepath.Join(dir, "none.json")); c != nil || err != nil {
		t.Errorf("a missing file: %+v, %v; want (nil, nil)", c, err)
	}

	path := CheckpointsPath(dir, "/p")
	big := &Checkpoints{Last: &LastCheckpoint{SessionID: strings.Repeat("s", MaxCheckpointsBytes)}}
	if err := WriteCheckpoints(path, big); err == nil {
		t.Error("an encoding over the cap was written")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("after a refused write the file is there (%v)", err)
	}
}
