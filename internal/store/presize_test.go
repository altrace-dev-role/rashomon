package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

// writeLines writes n non-empty, newline-terminated lines of body to path,
// creating path's parent directory first.
func writeLines(t *testing.T, path string, n int, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(body)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// realisticLine pads body past minPresizeRecordBytes with a trailing JSON
// field, so a test that writes n of it exercises the line-count clamp
// (maxPresizeLines) without also tripping cappedLineCount's byte-size
// floor -- the two clamps are independent and each gets its own test.
func realisticLine(body string) string {
	return body[:len(body)-1] + `,"pad":"` + strings.Repeat("x", minPresizeRecordBytes) + `"}`
}

// TestPresizeMatchesLineCount pins presizeRun's capacity to the run
// directory's own line counts on an ordinary, well within maxPresizeLines,
// run: records.ndjson and spill.ndjson together decide the declaration,
// execution and terminal slices' capacity -- split three ways and rounded
// up, callSlicePresize's own contract -- and coverage.ndjson decides the
// coverage slice's alone, exactly, not a default and not whatever append's
// own growth would have landed on.
func TestPresizeMatchesLineCount(t *testing.T) {
	dir := t.TempDir()
	// Bodies are padded past minPresizeRecordBytes: this test pins the
	// line-count presize on lines shaped like real records (no real line
	// this store writes is anywhere near minPresizeRecordBytes), not the
	// separate byte-size floor TestPresizeBoundedByFileSizeOnShortLinePadding
	// pins.
	writeLines(t, filepath.Join(dir, FileRecords), 7, realisticLine(`{"type":"declaration"}`))
	writeLines(t, filepath.Join(dir, FileSpill), 3, realisticLine(`{"type":"terminal"}`))
	writeLines(t, filepath.Join(dir, FileCoverage), 4, realisticLine(`{"type":"coverage"}`))

	run := presizeRun(dir, "a-run")

	want := callSlicePresize(10) // (7 + 3 lines, split three ways and rounded up) = 4
	if got := cap(run.Declarations); got != want {
		t.Errorf("cap(Declarations) = %d, want %d", got, want)
	}
	if got := cap(run.Executions); got != want {
		t.Errorf("cap(Executions) = %d, want %d", got, want)
	}
	if got := cap(run.Terminals); got != want {
		t.Errorf("cap(Terminals) = %d, want %d", got, want)
	}
	if got, want := cap(run.Coverage), 4; got != want {
		t.Errorf("cap(Coverage) = %d, want %d", got, want)
	}
}

// TestPresizeBoundedOnAdversarialFile pins the line-count clamp:
// records.ndjson AND spill.ndjson each made of far more realistically-sized
// lines than maxPresizeLines -- corrupted, or an attacker who can write to
// the store padding both -- never presize the three call slices' combined
// capacity past maxPresizeLines, however many lines the files actually
// hold between them. Both files, not one: countLinesCapped already clamps
// one file's own count at maxPresizeLines, so a single padded file cannot
// exceed that on its own -- it is the SUM of the two capped counts, up to
// 2*maxPresizeLines, that callSlicePresize's own clamp has to bound.
// Declaration is presize.go's bound computation's example: at 584 bytes an
// unclamped presize of that sum would ask for hundreds of megabytes before
// a single line is parsed. Lines here are realisticLine-padded so this
// test isolates the line-count clamp from cappedLineCount's byte-size
// floor, which TestPresizeBoundedByFileSizeOnShortLinePadding pins on its
// own, short-line-padded file.
func TestPresizeBoundedOnAdversarialFile(t *testing.T) {
	dir := t.TempDir()
	line := realisticLine(`{"type":"x"}`)
	writeLines(t, filepath.Join(dir, FileRecords), maxPresizeLines+50_000, line)
	writeLines(t, filepath.Join(dir, FileSpill), maxPresizeLines+50_000, line)

	run := presizeRun(dir, "a-run")

	want := callSlicePresize(maxPresizeLines) // the clamp, split three ways
	if got := cap(run.Declarations); got != want {
		t.Errorf("cap(Declarations) = %d, want the clamped %d", got, want)
	}
	if got := cap(run.Executions); got != want {
		t.Errorf("cap(Executions) = %d, want the clamped %d", got, want)
	}
	if got := cap(run.Terminals); got != want {
		t.Errorf("cap(Terminals) = %d, want the clamped %d", got, want)
	}
	// callSlicePresize rounds each third up, so the combined capacity can
	// exceed maxPresizeLines by at most 2 -- never by a multiple of it. This
	// reads the run's own capacities, not callSlicePresize(maxPresizeLines)
	// again, so it actually exercises presizeRun's combined clamp rather
	// than re-deriving the same constant the per-slice checks above already
	// pinned.
	if total := cap(run.Declarations) + cap(run.Executions) + cap(run.Terminals); total > maxPresizeLines+2 {
		t.Errorf("the three slices' combined capacity %d exceeds maxPresizeLines %d by more than the rounding slop", total, maxPresizeLines)
	}
}

// TestPresizeBoundedByFileSizeOnShortLinePadding pins the second clamp:
// countLinesCapped's own maxPresizeLines cap bounds a line count, but a
// line count is not retained bytes, and a file of nothing but very short
// lines can hold many more lines than real records ever would for the same
// size on disk. Without a size-tied limit, a run directory of a few hundred
// KB of single-character junk lines presizes as if it held tens of
// thousands of full declarations, executions and terminals -- costing
// megabytes of retained capacity for a file that could not possibly decode
// into that many real records. presizeRun must size the hint off the
// SMALLER of the line count and a floor derived from the file's own byte
// length, so padding with short lines cannot buy more capacity than the
// bytes on disk could plausibly contain.
func TestPresizeBoundedByFileSizeOnShortLinePadding(t *testing.T) {
	dir := t.TempDir()
	const padLines = 100_000 // "x\n" x 100,000 = ~200 KB per file
	writeLines(t, filepath.Join(dir, FileRecords), padLines, "x")
	writeLines(t, filepath.Join(dir, FileSpill), padLines, "x")
	writeLines(t, filepath.Join(dir, FileCoverage), padLines, "x")

	run := presizeRun(dir, "a-run")

	callBytes := cap(run.Declarations)*int(unsafe.Sizeof(Declaration{})) +
		cap(run.Executions)*int(unsafe.Sizeof(Execution{})) +
		cap(run.Terminals)*int(unsafe.Sizeof(Terminal{}))
	covBytes := cap(run.Coverage) * int(unsafe.Sizeof(Coverage{}))

	// Padding bytes/minPresizeRecordBytes per file is far below
	// maxPresizeLines (each file here is ~200 KB of 2-byte lines, not the
	// multi-megabyte file the line-count clamp alone would let through at
	// 50,000 lines), so a few hundred KB of junk must not buy anywhere
	// near the 25.7 MB the unclamped line-count math would otherwise
	// retain for this little data on disk.
	const budgetBytes = 8 << 20 // 8 MiB: comfortably under recap.stop's 16 MiB p95 ceiling, comfortably over any realistic file this small should ever presize to
	if total := callBytes + covBytes; total > budgetBytes {
		t.Errorf("presize retained %d bytes (calls %d + coverage %d) for %d bytes of on-disk padding per file, want under %d", total, callBytes, covBytes, padLines*2, budgetBytes)
	}
}

// TestPresizeMissingFilesReadAsZero pins presizeRun against a run directory
// that holds none of the three files yet -- the common case, a run just
// created -- to zero capacity rather than an error or a default guess,
// matching eachLine's own "a missing file has no lines".
func TestPresizeMissingFilesReadAsZero(t *testing.T) {
	dir := t.TempDir()

	run := presizeRun(dir, "a-run")

	if run.Declarations != nil {
		t.Errorf("Declarations = %#v, want nil", run.Declarations)
	}
	if run.Coverage != nil {
		t.Errorf("Coverage = %#v, want nil", run.Coverage)
	}
}

// TestCountLinesCappedStopsAtCap pins countLinesCapped's own contract: it
// never counts past limit, so a caller's clamp arithmetic can never be
// defeated by this function reporting a larger number than was asked for.
func TestCountLinesCappedStopsAtCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.ndjson")
	writeLines(t, path, 100, "y")

	n, err := countLinesCapped(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Errorf("countLinesCapped = %d, want the limit 10", n)
	}
}
