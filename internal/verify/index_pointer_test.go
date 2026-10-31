package verify

import (
	"fmt"
	"path/filepath"
	"testing"
)

// TestRP2zc_TheIndexNamesTheLastTwoSessions (card REPORT1b): a Stop's
// NoteCheckpoint makes the checkpoint Last and the Last of another session
// Prior; a second checkpoint of the same session (a clear) leaves Prior alone;
// PreviousOf is Last unless Last is the current session's, then Prior, and
// nil when neither is an earlier session's. The pointers survive the write and
// read of the index, with its cap counted over them. Break: overwrite Prior
// on every note, or let PreviousOf return the current session.
func TestRP2zc_TheIndexNamesTheLastTwoSessions(t *testing.T) {
	var idx ProjectIndex
	if idx.PreviousOf("s2") != nil || (*ProjectIndex)(nil).PreviousOf("s2") != nil {
		t.Fatal("an index with no pointer names no session")
	}
	idx.NoteCheckpoint("s1", 4, 100)
	if p := idx.PreviousOf("s2"); p == nil || p.SessionID != "s1" || p.PointSeq != 4 || p.AtMS != 100 {
		t.Errorf("after s1: %+v", p)
	}
	if idx.PreviousOf("s1") != nil {
		t.Error("s1's own checkpoint is not the session before s1")
	}
	idx.NoteCheckpoint("s2", 9, 200)
	idx.NoteCheckpoint("s2", 11, 300) // a clear continued s2
	if idx.Last.SessionID != "s2" || idx.Last.PointSeq != 11 || idx.Prior == nil || idx.Prior.SessionID != "s1" || idx.Prior.PointSeq != 4 {
		t.Errorf("Last %+v Prior %+v; want s2 at 11 and s1 at 4", idx.Last, idx.Prior)
	}
	if p := idx.PreviousOf("s2"); p == nil || p.SessionID != "s1" {
		t.Errorf("during s2 the previous session is s1: %+v", p)
	}
	if p := idx.PreviousOf("s3"); p == nil || p.SessionID != "s2" {
		t.Errorf("during s3 the previous session is s2: %+v", p)
	}

	path := filepath.Join(t.TempDir(), "p.json")
	idx.ProjectKey = "/p"
	for i := range 400 {
		idx.Entries = append(idx.Entries, IndexEntry{Family: fmt.Sprintf("%064x@%064x", i, i), Kind: "test", Sessions: 1, LastSession: "s", FirstSeen: int64(i), LastSeen: int64(i)})
	}
	if _, err := WriteIndex(path, &idx); err != nil {
		t.Fatal(err)
	}
	got, err := ReadIndex(path)
	if err != nil || got == nil || got.Last == nil || got.Prior == nil || *got.Last != *idx.Last || *got.Prior != *idx.Prior {
		t.Fatalf("the pointers after a write and read: %+v err %v", got, err)
	}
}
