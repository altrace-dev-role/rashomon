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
	// evidenceSlack is how far past a note's time the transcript's last
	// record must be before the transcript is taken to have caught up with
	// the note: a record at the note's own instant may be the attachment
	// itself, written before the turn it belongs to ended.
	evidenceSlack = 250 * time.Millisecond
)

// Needle is a note's text to look for and the time it was delivered. Only an
// attachment recorded at or after At can show it: the same first line is
// printed by many notes, and an older attachment is another note's.
type Needle struct {
	Text string
	At   time.Time
}

var attachmentType = []byte(`"hook_additional_context"`)

// ScanTranscript looks for each needle in the text of the hook_additional_context
// attachments of the transcript at path, within lim, counting only an
// attachment recorded at or after the needle's At. found[i] is true when
// needles[i] is in one. complete reports that the read reached every record
// from the oldest needle's time to the newest's, so a needle not found is
// absent from the transcript: the window starts at or before the oldest, and
// the last record is at least evidenceSlack past the newest, which a
// transcript that has not caught up with the note is not. It is false when
// the file could not be opened or read, held no record, ran past the
// deadline, or carries a sessionId that is not sessionID (a record of
// another session; found is then all false too). An empty sessionID checks
// nothing.
func ScanTranscript(path, sessionID string, needles []Needle, lim ScanLimits) (found []bool, complete bool) {
	found = make([]bool, len(needles))
	var oldest, newest time.Time
	for i, n := range needles {
		if i == 0 || n.At.Before(oldest) {
			oldest = n.At
		}
		if i == 0 || n.At.After(newest) {
			newest = n.At
		}
	}
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
	s := scan{needles: needles, found: found, left: len(needles), session: sessionID}
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
		done := s.line(sc.Bytes(), off > 0)
		if s.mismatch {
			return make([]bool, len(needles)), false
		}
		if done {
			return found, true
		}
	}
	if sc.Err() != nil {
		return found, false
	}
	if s.last.IsZero() || s.last.Before(newest.Add(evidenceSlack)) {
		return found, false
	}
	return found, off == 0 || (!s.first.IsZero() && !s.first.After(oldest))
}

// scan is ScanTranscript's state across lines.
type scan struct {
	needles []Needle
	found   []bool
	session string
	// mismatch is set when a record names another session.
	mismatch bool
	left     int
	// last is the time of the newest record that has one, in file order: the
	// time the transcript has been written up to.
	last time.Time
	// first is the first record's time in a window that begins inside the
	// file, tried over stampLines records.
	first time.Time
	tried int
}

// line reads one transcript line and reports whether every needle is found.
// Every record's time and session are read; its content only when it is an
// attachment of the type. The attachment's content is the list of strings
// Claude Code writes, or one string.
func (s *scan) line(b []byte, windowed bool) bool {
	if len(b) == 0 || b[0] != '{' {
		return false
	}
	needTime := windowed && s.first.IsZero() && s.tried < stampLines
	if needTime {
		s.tried++
	}
	isAttachment := bytes.Contains(b, attachmentType)
	recKeys, attKeys := recordKeys()
	var rec [3]jsonscan.Span // timestamp, attachment, sessionId
	var att [2]jsonscan.Span // type, content
	if jsonscan.Top(b, recKeys, rec[:]) != nil {
		return false
	}
	if s.otherSession(b, rec[2]) {
		return false
	}
	at, ok := parseStamp(b, rec[0])
	if ok {
		s.last = at
		if needTime {
			s.first = at
		}
	}
	if !isAttachment || !ok || rec[1].Kind != jsonscan.Object || jsonscan.Field(b, rec[1], attKeys, att[:]) != nil ||
		textOf(b, att[0]) != "hook_additional_context" {
		return false
	}
	for _, text := range stringsOf(b, att[1]) {
		for i, n := range s.needles {
			if !s.found[i] && !at.Before(n.At) && strings.Contains(text, n.Text) {
				s.found[i] = true
				s.left--
			}
		}
	}
	return s.left == 0
}

// otherSession records, and reports, that a record's sessionId is not the
// session's. A record with no sessionId says nothing either way.
func (s *scan) otherSession(b []byte, sp jsonscan.Span) bool {
	if s.session != "" && sp.Kind == jsonscan.String && textOf(b, sp) != s.session {
		s.mismatch = true
	}
	return s.mismatch
}

// parseStamp is a record's timestamp span as a time. The form Claude Code
// writes, 2006-01-02T15:04:05.000Z, is read in place, so a record costs no
// string; any other is left to time.Parse.
func parseStamp(b []byte, sp jsonscan.Span) (time.Time, bool) {
	raw := sp.Raw(b)
	if sp.Kind == jsonscan.String && len(raw) == 26 && raw[11] == 'T' && raw[5] == '-' && raw[8] == '-' && raw[14] == ':' &&
		raw[17] == ':' && raw[20] == '.' && raw[24] == 'Z' {
		d := func(i, n int) (v int, ok bool) {
			for _, c := range raw[i : i+n] {
				if c < '0' || c > '9' {
					return 0, false
				}
				v = v*10 + int(c-'0')
			}
			return v, true
		}
		y, ok1 := d(1, 4)
		mo, ok2 := d(6, 2)
		dd, ok3 := d(9, 2)
		h, ok4 := d(12, 2)
		mi, ok5 := d(15, 2)
		se, ok6 := d(18, 2)
		ms, ok7 := d(21, 3)
		if ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7 && mo >= 1 && mo <= 12 && dd >= 1 && dd <= 28 && h < 24 && mi < 60 && se < 60 {
			return time.Date(y, time.Month(mo), dd, h, mi, se, ms*int(time.Millisecond), time.UTC), true
		}
	}
	t, err := time.Parse(time.RFC3339Nano, textOf(b, sp))
	return t, err == nil
}
