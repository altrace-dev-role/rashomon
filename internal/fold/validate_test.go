package fold

import (
	"os"
	"path/filepath"
	"testing"
)

// runFiles is one run directory's three record files plus the root gaps
// file, the four paths Validate checks a saved File against.
type runFiles struct {
	records, spill, coverage, gaps string
}

func newRunFiles(t *testing.T) runFiles {
	t.Helper()
	dir := t.TempDir()
	return runFiles{
		records:  filepath.Join(dir, "records.ndjson"),
		spill:    filepath.Join(dir, "spill.ndjson"),
		coverage: filepath.Join(dir, "coverage.ndjson"),
		gaps:     filepath.Join(dir, "gaps.ndjson"),
	}
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

// validFile is a File that Validate accepts against rf as both stand right
// after it was written: every cursor at a line boundary, every offset no
// greater than its file's size, a V and Writer this reader understands.
func validFile(t *testing.T, rf runFiles) *File {
	t.Helper()
	writeFile(t, rf.records, "one\ntwo\n")
	writeFile(t, rf.spill, "")
	writeFile(t, rf.coverage, "cov\n")
	writeFile(t, rf.gaps, "")
	dev, inode, exists, err := identity(rf.records)
	if err != nil || !exists {
		t.Fatalf("identity(%s) = %v, %v, want a real file", rf.records, exists, err)
	}
	return &File{
		V: Version, Writer: "rashomon-test",
		Dev: dev, Inode: inode,
		Records:  Cursor{Offset: int64(len("one\ntwo\n")), Seq: 2, SizeSeen: int64(len("one\ntwo\n"))},
		Spill:    Cursor{},
		Coverage: Cursor{Offset: int64(len("cov\n")), SizeSeen: int64(len("cov\n"))},
		Gaps:     Cursor{},
	}
}

func TestValidate_FreshFoldIsValid(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("a freshly written, consistent fold was rejected")
	}
}

func TestValidate_InodeChangeInvalidates(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	// A rewrite: forget or eviction removed records.ndjson and wrote a new
	// one at the same path, so every offset into the old file is void.
	if err := os.Remove(rf.records); err != nil {
		t.Fatal(err)
	}
	writeFile(t, rf.records, "one\ntwo\n")

	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a fold whose records.ndjson was rewritten under it was accepted")
	}
}

func TestValidate_OffsetPastFileSizeInvalidates(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	f.Coverage.Offset = 1_000_000 // the cursor claims more than coverage.ndjson holds

	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a cursor offset past its file's current size was accepted")
	}
}

func TestValidate_OffsetNotAtLineBoundaryInvalidates(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	f.Records.Offset-- // one byte short of the line boundary

	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a cursor one byte off a line boundary was accepted")
	}
}

func TestValidate_UnknownSchemaVersionInvalidates(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	f.V = Version + 1

	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a fold of an unknown schema version was accepted")
	}
}

func TestValidate_EmptyWriterInvalidates(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	f.Writer = ""

	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a fold with no writer recorded was accepted")
	}
}

func TestValidate_ZeroOffsetNeverFailsTheBoundaryCheck(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	f.Spill = Cursor{} // offset 0 on a file that exists but holds nothing yet

	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("a zero offset into an empty file was rejected")
	}
}

func TestValidate_MissingFileWithNonZeroOffsetInvalidates(t *testing.T) {
	rf := newRunFiles(t)
	f := validFile(t, rf)
	if err := os.Remove(rf.coverage); err != nil {
		t.Fatal(err)
	}
	// f.Coverage.Offset is still positive, but the file is gone.
	ok, err := Validate(f, rf.records, rf.spill, rf.coverage, rf.gaps)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("a cursor into a file that no longer exists was accepted")
	}
}
