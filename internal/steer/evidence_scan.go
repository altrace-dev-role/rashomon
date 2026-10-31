package steer

import (
	"bytes"
	"io"
	"os"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/linescan"
)

// The transcript read of CHANNEL-EVIDENCE1. The Stop path read no transcript
// before this card (readFinalMessage reads the payload), so this is a new
// read on a hook, and it is bounded three ways: bytes (the last
// ScanLimits.MaxBytes of the file), time (ScanLimits.Deadline, checked every
// line), and memory (a line over ScanLimits.MaxLine is stepped over, never
// held). A note is not judged absent unless the read covered it; a read the
// bounds cut short is unreadable, not absent.

// ScanLimits bounds one transcript read.
type ScanLimits struct {
	// MaxBytes is how much of the file's end is read; a file longer than it
	// is read from MaxBytes before its end.
	MaxBytes int64
	// MaxLine is the longest line held, and the scanner's buffer bound; a
	// longer line is stepped over. A hook_additional_context attachment is a
	// note or a few, so none is near it.
	MaxLine int
	// Deadline ends the read; the zero time is no deadline.
	Deadline time.Time
}

// DefaultScanLimits is the Stop's bound: 16 MiB of the transcript's end,
// lines to 1 MiB, one second. The row BenchmarkEvidenceScan (budgets.json)
// is its allocation budget.
func DefaultScanLimits(now time.Time) ScanLimits {
	return ScanLimits{MaxBytes: 16 << 20, MaxLine: 1 << 20, Deadline: now.Add(time.Second)}
}

const (
	scanBuffer = 64 << 10
	// stampLines is how many records are tried for the window's first
	// timestamp before the window is taken as not reaching a note's time.
	stampLines = 64
)

var attachmentType = []byte(`"hook_additional_context"`)

// ScanTranscript looks for each needle in the text of the hook_additional_context
// attachments of the transcript at path, within lim. found[i] is true when
// needles[i] is in one. complete reports that the read reached every record
// from oldest on, so a needle not found is absent from the transcript; it is
// false when the file could not be opened or read, held no record, ran past
// the deadline, or, for a file longer than MaxBytes, began after oldest.
func ScanTranscript(path string, needles []string, oldest time.Time, lim ScanLimits) (found []bool, complete bool) {
	found = make([]bool, len(needles))
	f, err := os.Open(path)
	if err != nil {
		return found, false
	}
	defer f.Close() //nolint:errcheck // read only
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return found, false
	}
	size, off := fi.Size(), int64(0)
	if size > lim.MaxBytes {
		off = size - lim.MaxBytes
	}
	sc := linescan.New(io.NewSectionReader(f, off, size-off), scanBuffer, lim.MaxLine)
	s := scan{needles: needles, found: found, left: len(needles)}
	skipFirst := off > 0 // the window starts inside a line
	for sc.Scan() {
		if skipFirst {
			skipFirst = false
			continue
		}
		if !lim.Deadline.IsZero() && time.Now().After(lim.Deadline) {
			return found, false
		}
		if sc.TooLong() {
			continue
		}
		if s.line(sc.Bytes(), off > 0) {
			return found, true
		}
	}
	if sc.Err() != nil || s.records == 0 {
		return found, false
	}
	return found, off == 0 || (!s.first.IsZero() && !s.first.After(oldest))
}

// scan is ScanTranscript's state across lines.
type scan struct {
	needles []string
	found   []bool
	left    int
	records int
	// first is the first record's time in a window that begins inside the
	// file, tried over stampLines records.
	first time.Time
	tried int
}

// line reads one transcript line and reports whether every needle is found.
// A line is scanned only when it may hold an attachment of the type, or is
// one of the first stampLines of a window that has no time yet. The
// attachment's content is the list of strings Claude Code writes, or one
// string.
func (s *scan) line(b []byte, windowed bool) bool {
	if len(b) == 0 || b[0] != '{' {
		return false
	}
	s.records++
	needTime := windowed && s.first.IsZero() && s.tried < stampLines
	if needTime {
		s.tried++
	}
	isAttachment := bytes.Contains(b, attachmentType)
	if !needTime && !isAttachment {
		return false
	}
	recKeys, attKeys := recordKeys()
	var rec, att [2]jsonscan.Span // timestamp, attachment; type, content
	if jsonscan.Top(b, recKeys, rec[:]) != nil {
		return false
	}
	if needTime {
		if t, err := time.Parse(time.RFC3339Nano, textOf(b, rec[0])); err == nil {
			s.first = t
		}
	}
	if !isAttachment || rec[1].Kind != jsonscan.Object || jsonscan.Field(b, rec[1], attKeys, att[:]) != nil ||
		textOf(b, att[0]) != "hook_additional_context" {
		return false
	}
	for _, text := range stringsOf(b, att[1]) {
		for i, n := range s.needles {
			if !s.found[i] && strings.Contains(text, n) {
				s.found[i] = true
				s.left--
			}
		}
	}
	return s.left == 0
}
