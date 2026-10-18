package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFoldStore_FoldStreamsFromOffset: a Fold call starting at offset 0 folds
// every complete line and returns the offset just past the last one, so a
// second call from that offset folds nothing until more is appended.
func TestFoldStore_FoldStreamsFromOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileRecords)
	write(t, path, "line one\nline two\n")

	var got []string
	offset, size, err := Fold(path, 0, func(line []byte) { got = append(got, string(line)) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "line one" || got[1] != "line two" {
		t.Fatalf("folded %v, want [line one, line two]", got)
	}
	if size != int64(len("line one\nline two\n")) {
		t.Errorf("size = %d, want the file's size", size)
	}
	if offset != size {
		t.Errorf("offset = %d, want %d (every line complete)", offset, size)
	}

	// Nothing new: a second Fold from the returned offset folds no line.
	got = nil
	offset2, _, err := Fold(path, offset, func(line []byte) { got = append(got, string(line)) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("second Fold from the end folded %v, want nothing", got)
	}
	if offset2 != offset {
		t.Errorf("offset2 = %d, want unchanged %d", offset2, offset)
	}

	// Append more, resume from the saved offset: only the new line folds.
	appendRaw(t, path, []byte("line three\n"))
	got = nil
	offset3, _, err := Fold(path, offset, func(line []byte) { got = append(got, string(line)) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "line three" {
		t.Errorf("resumed fold = %v, want [line three]", got)
	}
	if offset3 <= offset {
		t.Errorf("offset3 = %d, want it to have advanced past %d", offset3, offset)
	}
}

// TestFoldStore_FoldLeavesATornTailForNextTime: a final line with no trailing
// newline -- a write still in flight -- is not folded, and the returned
// offset stops before it so the next Fold call reads it whole once it is
// terminated.
func TestFoldStore_FoldLeavesATornTailForNextTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileRecords)
	write(t, path, "whole line\n")
	appendRaw(t, path, []byte(`{"type":"declaration","tool_use_id":"toolu_torn"`)) // no newline

	var got []string
	offset, _, err := Fold(path, 0, func(line []byte) { got = append(got, string(line)) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "whole line" {
		t.Fatalf("folded %v, want only [whole line]", got)
	}
	wantOffset := int64(len("whole line\n"))
	if offset != wantOffset {
		t.Errorf("offset = %d, want %d (stopping before the torn line)", offset, wantOffset)
	}

	// Complete the line: the next Fold from the same offset now folds it.
	appendRaw(t, path, []byte("}\n"))
	got = nil
	offset2, _, err := Fold(path, offset, func(line []byte) { got = append(got, string(line)) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("after completion folded %v, want one line", got)
	}
	if offset2 == offset {
		t.Errorf("offset2 did not advance past the now-complete line")
	}
}

// TestFoldStore_FoldOnMissingFileFoldsNothing: a file that does not exist yet
// (a run with no spill, say) folds nothing from offset 0, the same outcome
// eachLine's other callers already give a missing file.
func TestFoldStore_FoldOnMissingFileFoldsNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileSpill)
	calls := 0
	offset, size, err := Fold(path, 0, func(line []byte) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || offset != 0 || size != 0 {
		t.Errorf("Fold on a missing file = (%d, %d, calls=%d), want (0, 0, 0)", offset, size, calls)
	}
}

// TestFoldStore_FoldRejectsAnOffsetPastTheFilesSize: a file shorter than the
// offset -- forget or eviction rewrote it under a stale cursor -- is
// reported rather than silently read from the wrong place (Cursor's
// SizeSeen doc: "a file that became shorter is detected before an offset
// past its end is trusted").
func TestFoldStore_FoldRejectsAnOffsetPastTheFilesSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileRecords)
	write(t, path, "one line\n")

	_, _, err := Fold(path, 1000, func(line []byte) {})
	if err == nil {
		t.Fatal("want an error for an offset past the file's size, got nil")
	}
}

// TestFoldStore_FoldNeverHoldsMoreThanOneLine is not a memory assertion by
// itself (test/perf's RSS gates cover that end to end); it pins the
// over-length behaviour a bounded-buffer reader must have: a line longer
// than maxLine is not read whole, reported to fn as nil (as forEachLine's
// other callers already see it), counted once it is actually terminated,
// never while it is still arriving.
func TestFoldStore_FoldOverlongLineReadsAsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileRecords)
	long := make([]byte, maxLine+10)
	for i := range long {
		long[i] = 'a'
	}
	write(t, path, string(long)+"\nshort\n")

	var got []string
	nilCount := 0
	_, _, err := Fold(path, 0, func(line []byte) {
		if line == nil {
			nilCount++
			return
		}
		got = append(got, string(line))
	})
	if err != nil {
		t.Fatal(err)
	}
	if nilCount != 1 {
		t.Errorf("nilCount = %d, want 1 (the overlong line)", nilCount)
	}
	if len(got) != 1 || got[0] != "short" {
		t.Errorf("folded lines = %v, want [short]", got)
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), fileMode); err != nil {
		t.Fatal(err)
	}
}
