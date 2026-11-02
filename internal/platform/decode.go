package platform

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// LogHeader is the one pgraph log format this package reads, the first line
// pgraph's (*Log).Encode writes (pgraph/codec.go LogHeader). A log under any
// other header is a format this package does not know: its answer is unknown,
// never a guess.
const LogHeader = "pgraph-log v0"

// MaxLogBytes caps the log Impact reads (INV-10). pgraph's own cap is 256 MiB
// (pgraph/codec.go MaxLogBytes): a log between the two is valid to pgraph and
// over the cap here, so its answer is unknown: the same bytes give the same
// answer, whatever the clock (INV-11).
const MaxLogBytes = 64 << 20

// The v0 record's field caps, as pgraph/tier.go MaxFieldBytes and
// pgraph/codec.go MaxRecordBytes set them: eight fields at their cap and the
// separators.
const (
	maxFieldBytes  = 8192
	maxRecordBytes = 8*maxFieldBytes + 16
)

// checkEvery is how many records the decoder reads between looks at its
// context, so a cancelled decode stops within that many records.
const checkEvery = 256

// The v0 format's predicate and tier spellings (pgraph/tier.go). Only
// declares is read for meaning; the others are carried into a family's Hash.
const (
	predDeclares   = "declares"
	predCalls      = "calls"
	predEmbeds     = "embeds"
	predImplements = "implements"
	predImplPtr    = "implements-via-pointer"
	predMayCall    = "may-call"

	tierProven      = "PROVEN"
	tierAlgorithmic = "ALGORITHMIC"
	tierProposed    = "model-proposed"
)

// ErrUnknownHeader is a log whose first line is not LogHeader.
var ErrUnknownHeader = errors.New("platform: the log's header is not " + LogHeader)

// ErrOverCap is a log over the byte cap Impact reads under.
var ErrOverCap = errors.New("platform: the log is over its byte cap")

// recordHook, when set, runs before each record is read: a test's hold on
// the decode (TestPL8c, TestPL8c2), nil outside tests. Impact reads it once, in the caller's
// goroutine, and hands it to the worker, so a test may reset it while a
// timed-out worker still runs.
var recordHook func()

// fact is one statement with the two provenance fields that are part of its
// identity in the log (pgraph's factKey): tier and build config. Commit and
// extractor are provenance only and are not kept.
type fact struct {
	subject, predicate, object, tier, config string
}

// decode replays a v0 log read from r into its live set: an assert adds a
// fact, a retraction removes it. It refuses a header other than LogHeader, a
// line over maxRecordBytes, a field over maxFieldBytes, and any record that
// is not op and seven non-empty fields with a v0 predicate and tier, so an
// answer is never built from a log this package cannot read whole. It looks
// at ctx every checkEvery records and returns ctx's error, with nothing else,
// once ctx is done. hook, when not nil, runs before each record. keep, when
// not nil, is the subjects the set holds: every record is still read and
// checked in place, and one whose subject keep refuses is dropped before
// anything is allocated for it, so the set, and the memory a read takes,
// follow keep's packages, not the size of the log.
func decode(ctx context.Context, r io.Reader, hook func(), keep func(subject []byte) bool) (map[fact]struct{}, error) {
	br := bufio.NewReaderSize(r, maxRecordBytes+1)
	header, err := readLine(br)
	if err != nil {
		return nil, fmt.Errorf("platform: reading the log's header: %w", err)
	}
	if string(header) != LogHeader {
		return nil, ErrUnknownHeader
	}
	live := map[fact]struct{}{}
	for n := 1; ; n++ {
		if n%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if hook != nil {
			hook()
		}
		line, err := readLine(br)
		if err == io.EOF {
			return live, nil
		}
		if err != nil {
			return nil, fmt.Errorf("platform: line %d: %w", n+1, err)
		}
		c, assert, err := parseRecord(line)
		if err != nil {
			return nil, fmt.Errorf("platform: line %d: %w", n+1, err)
		}
		if keep != nil && !keep(c[1]) {
			continue
		}
		f := c.fact()
		if assert {
			live[f] = struct{}{}
		} else {
			delete(live, f)
		}
	}
}

// readLine is the next line without its newline. A last line without one is
// torn and refused, as is a line over maxRecordBytes; io.EOF is the end of a
// whole log.
func readLine(br *bufio.Reader) ([]byte, error) {
	line, err := br.ReadSlice('\n')
	switch {
	case err == nil:
		return line[:len(line)-1], nil
	case err == io.EOF && len(line) == 0:
		return nil, io.EOF
	case err == io.EOF:
		return nil, errors.New("the log lacks a final newline")
	case errors.Is(err, bufio.ErrBufferFull):
		return nil, fmt.Errorf("a line is over %d bytes", maxRecordBytes)
	}
	return nil, err
}

// record is a v0 record's eight fields, in place in the line they were read
// from: valid only until the next read.
type record [8][]byte

// parseRecord reads "op subject predicate object tier config commit
// extractor", tab-separated, op being + or -, without allocating.
func parseRecord(line []byte) (record, bool, error) {
	var c record
	rest := line
	for i := range c {
		if i == len(c)-1 {
			c[i] = rest
			break
		}
		j := bytes.IndexByte(rest, '\t')
		if j < 0 {
			return record{}, false, fmt.Errorf("%d fields, want %d", i+1, len(c))
		}
		c[i], rest = rest[:j], rest[j+1:]
	}
	for i, s := range c[1:] {
		if len(s) == 0 || len(s) > maxFieldBytes || bytes.IndexByte(s, '\t') >= 0 {
			return record{}, false, fmt.Errorf("field %d is empty, over %d bytes or holds a tab", i+2, maxFieldBytes)
		}
	}
	var assert bool
	switch string(c[0]) {
	case "+":
		assert = true
	case "-":
	default:
		return record{}, false, fmt.Errorf("unknown op %q", c[0])
	}
	switch string(c[2]) {
	case predDeclares, predCalls, predEmbeds, predImplements, predImplPtr, predMayCall:
	default:
		return record{}, false, fmt.Errorf("unknown predicate %q", c[2])
	}
	switch string(c[4]) {
	case tierProven, tierAlgorithmic, tierProposed:
	default:
		return record{}, false, fmt.Errorf("unknown tier %q", c[4])
	}
	return c, assert, nil
}

// fact is the record's statement in one allocation of the five fields it
// keeps, so a held fact holds neither its commit nor its extractor.
func (c *record) fact() fact {
	var b strings.Builder
	b.Grow(len(c[1]) + len(c[2]) + len(c[3]) + len(c[4]) + len(c[5]))
	for _, f := range c[1:6] {
		b.Write(f)
	}
	s := b.String()
	var out [5]string
	for i, f := range c[1:6] {
		out[i], s = s[:len(f)], s[len(f):]
	}
	return fact{subject: out[0], predicate: out[1], object: out[2], tier: out[3], config: out[4]}
}

// tierRank is a tier's place in the view's order, PROVEN first; 0 for a
// spelling v0 does not have.
func tierRank(t string) int {
	switch t {
	case tierProven:
		return 1
	case tierAlgorithmic:
		return 2
	case tierProposed:
		return 3
	}
	return 0
}
