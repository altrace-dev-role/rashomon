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
// The full worst case at this clamp, across every slice presizeRun fills,
// is bigger than the three call slices alone: Declaration is 584 bytes
// (unsafe.Sizeof, 2026-10-17), Execution 392, Terminal 112, so
// callSlicePresize splitting the clamped total three ways costs at most
// maxPresizeLines * (584 + 392 + 112) / 3 bytes, about 18.1 MB for the
// three call slices -- TestPresizeBoundedOnAdversarialFile pins that
// clamp. presizeRun ALSO presizes Coverage from its own capped count, up
// to maxPresizeLines * 152 bytes, about 7.6 MB more. Combined, the clamp's
// own worst case is about 25.7 MB, not 18 MB. cappedLineCount's file-size
// floor (minPresizeRecordBytes, below) is what keeps that 25.7 MB tied to
// a file large enough to plausibly hold that many real records, rather
// than letting a run directory of a few hundred KB of short junk lines
// reach it (TestPresizeBoundedByFileSizeOnShortLinePadding). 50,000 is
// also three orders of magnitude past BenchmarkRecap's own largest fixture
// (1,000 calls, about 3,000 lines), so no benchmarked or realistic session
// is ever affected by either clamp -- only a file shaped unlike any this
// store writes.
const maxPresizeLines = 50_000

// minPresizeRecordBytes is the second, independent bound cappedLineCount
// applies to its line count: no line in records.ndjson, spill.ndjson or
// coverage.ndjson this store has ever written is anywhere near this short
// (the smallest observed in BenchmarkRecordBytesPerCall's own fixtures is
// 220 bytes; a Terminal's JSON alone cannot get under about that), so a
// file cannot hold more than fileBytes/minPresizeRecordBytes real records,
// however many lines it contains.
//
// maxPresizeLines alone bounds presizeRun's worst case to a file at least
// maxPresizeLines*minPresizeRecordBytes bytes (3.2 MB) before that clamp is
// the binding one; below that size, a file padded with short lines -- junk,
// corruption, or an adversary who can write to the store -- would otherwise
// let a line count run far ahead of what the bytes on disk could decode
// into. cappedLineCount uses the SMALLER of the two clamps, so a small file
// of short lines presizes off its own size, and a large file of
// realistically-sized lines presizes off maxPresizeLines, whichever bites
// first. 64 is conservative on purpose: understating a real run's line
// count only costs one slice-growth later, never correctness, so there is
// no reason to size this close to the true 220-byte floor.
const minPresizeRecordBytes = 64

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
// the smaller of maxPresizeLines and the file's own byte-size floor (see
// cappedLineCount), so a run directory shaped unlike any this store writes
// presizes to one of those clamps rather than to whatever the file actually
// claims to hold. records.ndjson and spill.ndjson are counted together,
// because a declaration, an execution or a terminal can land in either
// (lld-3's spill path), and callSlicePresize splits that combined total
// three ways: the three kinds run roughly 1:1:1 per call
// (payload.WriteSession's own fixtures measure one of each), and giving
// each slice the whole combined count presizes every one of the three to
// three times what it needed. coverage.ndjson is counted and presized on
// its own, since nothing else shares that file. A file that cannot be
// counted for any reason besides being absent presizes as zero: this is a
// sizing hint, and it must never be able to change what a read returns,
// only how it gets there (TestPresizeMissingFilesReadAsZero).
func presizeRun(dir, name string) *Run {
	run := &Run{Dir: name}

	calls := cappedLineCount(filepath.Join(dir, FileRecords)) + cappedLineCount(filepath.Join(dir, FileSpill))
	if n := callSlicePresize(calls); n > 0 {
		run.Declarations = make([]Declaration, 0, n)
		run.Executions = make([]Execution, 0, n)
		run.Terminals = make([]Terminal, 0, n)
	}

	// cappedLineCount already clamps at maxPresizeLines (and, below that,
	// at the file's own byte-size floor), so cov can never exceed it here;
	// there is no second clamp to apply.
	if cov := cappedLineCount(filepath.Join(dir, FileCoverage)); cov > 0 {
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
//
// The limit it passes down is the SMALLER of maxPresizeLines and a floor
// derived from path's own size on disk (path's byte length divided by
// minPresizeRecordBytes): a line count is not retained bytes, and a file
// of nothing but very short lines can hold many more lines than real
// records ever would for the same size on disk. Without this floor, a run
// directory of a few hundred KB of single-character junk lines would
// presize as if it held tens of thousands of full records, retaining
// megabytes of capacity for a file that cannot possibly decode into that
// many. A Stat that fails leaves the limit at maxPresizeLines unchanged --
// countLinesCapped's own open call will then see the same error (or, for a
// missing file, read zero lines), so nothing here needs to special-case
// Stat's failure modes.
func cappedLineCount(path string) int {
	limit := maxPresizeLines
	if info, err := os.Stat(path); err == nil {
		if bySize := int(info.Size() / minPresizeRecordBytes); bySize < limit {
			limit = bySize
		}
	}
	n, err := countLinesCapped(path, limit)
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
