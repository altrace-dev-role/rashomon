package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// TestPresizeMatchesLineCount pins presizeRun's capacity to the run
// directory's own line counts on an ordinary, well within maxPresizeLines,
// run: records.ndjson and spill.ndjson together decide the declaration,
// execution and terminal slices' capacity -- split three ways and rounded
// up, callSlicePresize's own contract -- and coverage.ndjson decides the
// coverage slice's alone, exactly, not a default and not whatever append's
// own growth would have landed on.
func TestPresizeMatchesLineCount(t *testing.T) {
	dir := t.TempDir()
	writeLines(t, filepath.Join(dir, FileRecords), 7, `{"type":"declaration"}`)
	writeLines(t, filepath.Join(dir, FileSpill), 3, `{"type":"terminal"}`)
	writeLines(t, filepath.Join(dir, FileCoverage), 4, `{"type":"coverage"}`)

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

// TestPresizeBoundedOnAdversarialFile pins the clamp: records.ndjson AND
// spill.ndjson each made of far more lines than maxPresizeLines --
// corrupted, or an attacker who can write to the store padding both with
// short lines -- never presize the three call slices' combined capacity
// past maxPresizeLines, however many lines the files actually hold between
// them. Both files, not one: countLinesCapped already clamps one file's
// own count at maxPresizeLines, so a single padded file cannot exceed that
// on its own -- it is the SUM of the two capped counts, up to
// 2*maxPresizeLines, that callSlicePresize's own clamp has to bound.
// Declaration is presize.go's bound computation's example: at 584 bytes an
// unclamped presize of that sum would ask for hundreds of megabytes before
// a single line is parsed.
func TestPresizeBoundedOnAdversarialFile(t *testing.T) {
	dir := t.TempDir()
	writeLines(t, filepath.Join(dir, FileRecords), maxPresizeLines+50_000, "x")
	writeLines(t, filepath.Join(dir, FileSpill), maxPresizeLines+50_000, "x")

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
	// exceed maxPresizeLines by at most 2 -- never by a multiple of it.
	if total := 3 * want; total > maxPresizeLines+2 {
		t.Errorf("the three slices' combined capacity %d exceeds maxPresizeLines %d by more than the rounding slop", total, maxPresizeLines)
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
