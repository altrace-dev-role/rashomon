package store

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// maxPresizeLines bounds how many lines presizeRun will ever read as a
// sizing hint for the three call slices combined, from a count it took no
// longer than the cap to find (countLinesCapped never counts past it).
//
// It exists because the line count itself is not evidence of how many
// records a file actually holds -- records.ndjson is capped at half the
// store's own 512 MiB default (store.cap_bytes), so a single run's file can
// legitimately reach 256 MiB, and a corrupted or adversarially padded file
// of nothing but short lines turns a line count into a number with no
// relationship to what will actually be appended. Without the clamp, that
// number would become a slice's initial capacity before a single line is
// parsed.
//
// Declaration is the bound's worst case at 584 bytes (unsafe.Sizeof,
// 2026-10-17): callSlicePresize splits the clamped total three ways, so
// presizing all three call slices to their shares costs at most
// maxPresizeLines * (584 + 392 + 112) / 3 bytes, about 18 MB --
// TestPresizeBoundedOnAdversarialFile pins the clamp itself, and that
// arithmetic is the reason the number is this one and not larger. 50,000
// is also three orders of magnitude past BenchmarkRecap's own largest
// fixture (1,000 calls, about 3,000 lines), so no benchmarked or realistic
// session is ever affected by it -- only a file shaped unlike any this
// store writes.
const maxPresizeLines = 50_000

// presizeRun returns an empty Run for name, with its declaration, execution,
// terminal and coverage slices pre-allocated from dir's own files, so that
// classifyRunLine's appends do not have to grow and copy a value as large
// as Declaration (584 B) through slice-doubling as a run is read -- the
// read path's own measurement (BenchmarkRecap/1000, schema 5) found the
// allocation growth was exactly that: element size times doubling, not new
// allocations, once every v5 key landed on every call record.
//
// The count behind each slice is a line count, not a decode and not the
// file's byte length: countLinesCapped scans for newlines only, capped at
// maxPresizeLines (see its doc), so a run directory shaped unlike any this
// store writes presizes to the clamp rather than to whatever the file
// actually claims to hold. records.ndjson and spill.ndjson are counted
// together, because a declaration, an execution or a terminal can land in
// either (lld-3's spill path), and callSlicePresize splits that combined
// total three ways: the three kinds run roughly 1:1:1 per call
// (payload.WriteSession's own fixtures measure one of each), and giving
// each slice the whole combined count, as an earlier version of this
// function did, presized every one of the three to three times what it
// needed. coverage.ndjson is counted and presized on its own, since nothing
// else shares that file. A file that cannot be counted for any reason
// besides being absent presizes as zero: this is a sizing hint, and it must
// never be able to change what a read returns, only how it gets there
// (TestPresizeMissingFilesReadAsZero).
func presizeRun(dir, name string) *Run {
	run := &Run{Dir: name}

	calls := cappedLineCount(filepath.Join(dir, FileRecords)) + cappedLineCount(filepath.Join(dir, FileSpill))
	if n := callSlicePresize(calls); n > 0 {
		run.Declarations = make([]Declaration, 0, n)
		run.Executions = make([]Execution, 0, n)
		run.Terminals = make([]Terminal, 0, n)
	}

	if cov := cappedLineCount(filepath.Join(dir, FileCoverage)); cov > 0 {
		if cov > maxPresizeLines {
			cov = maxPresizeLines
		}
		run.Coverage = make([]Coverage, 0, cov)
	}
	return run
}

// callSlicePresize is the per-slice capacity presizeRun gives Declarations,
// Executions and Terminals from lines, the combined line count of
// records.ndjson and spill.ndjson: lines clamped at maxPresizeLines, split
// three ways and rounded up, so the three slices' combined worst-case
// capacity is maxPresizeLines plus at most the rounding slop (2), never
// 3*maxPresizeLines, however the clamp or the split is reached. Rounding up
// rather than down means a run of exactly one call -- one declaration, one
// execution, one terminal, three lines -- still presizes each slice to 1,
// not 0.
func callSlicePresize(lines int) int {
	if lines > maxPresizeLines {
		lines = maxPresizeLines
	}
	return (lines + 2) / 3
}

// cappedLineCount is countLinesCapped with any error folded to zero: a
// sizing hint that failed to size must cost performance, never correctness.
func cappedLineCount(path string) int {
	n, err := countLinesCapped(path, maxPresizeLines)
	if err != nil {
		return 0
	}
	return n
}

// countLinesCapped counts the '\n' bytes in path, stopping as soon as the
// count reaches limit, without decoding or retaining any line. A missing
// file counts as zero, the same outcome eachLine gives it ("a missing file
// has no lines"); any other open or read error is returned, and the count
// so far with it.
//
// It reads in readChunk-sized pieces and keeps only the current one: a file
// at records.ndjson's own 256 MiB ceiling is scanned through, never held in
// memory, which is what lets presizeRun treat this as cheap enough to pay
// on every read rather than something a caller must opt into.
func countLinesCapped(path string, limit int) (int, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close() //nolint:errcheck // read-only

	buf := make([]byte, readChunk)
	count := 0
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			count += bytes.Count(buf[:n], newline)
			if count >= limit {
				return limit, nil
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return count, nil
			}
			return count, rerr
		}
	}
}

// newline is the one byte countLinesCapped scans for.
var newline = []byte{'\n'}
