package pgraph

import (
	"bytes"
	"fmt"
	"strings"
)

// Caps on what DecodeLog will read, named so the over-cap path is a test.
const (
	// MaxLogBytes bounds a whole encoded log.
	MaxLogBytes = 256 << 20
	// MaxRecordBytes bounds one line: eight fields at their cap plus separators.
	MaxRecordBytes = 8*MaxFieldBytes + 16
)

// LogHeader is the first line of an encoded log. A change of record meaning
// takes a new header; fields are never reused.
const LogHeader = "pgraph-log v0"

const recordFields = 8

// Encode renders the log: the header, then one line per record,
// "op subject predicate object tier config commit extractor", tab-separated,
// op being + or -. Appending a record only appends bytes.
func (l *Log) Encode() []byte {
	var b strings.Builder
	b.WriteString(LogHeader + "\n")
	for _, r := range l.recs {
		op := "+"
		if r.op == Retract {
			op = "-"
		}
		f := r.fact
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", op, f.t.Subject, f.t.Predicate, f.t.Object, f.tier, f.config, f.commit, f.extractor)
	}
	return []byte(b.String())
}

// DecodeLog parses an encoded log into closed types, replaying it under the
// Log's invariants. It is strict: input that is not exactly what Encode would
// produce for the log it describes is refused, so decode-then-encode is the
// identity. Caps are checked before any parsing.
func DecodeLog(b []byte) (*Log, error) {
	if len(b) > MaxLogBytes {
		return nil, fmt.Errorf("pgraph: log is %d bytes, cap %d", len(b), MaxLogBytes)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		return nil, fmt.Errorf("pgraph: log is empty or lacks a final newline")
	}
	l := &Log{}
	for n := 1; len(b) > 0; n++ {
		i := bytes.IndexByte(b, '\n')
		if i > MaxRecordBytes {
			return nil, fmt.Errorf("pgraph: line %d is over %d bytes", n, MaxRecordBytes)
		}
		line := string(b[:i])
		b = b[i+1:]
		if n == 1 {
			if line != LogHeader {
				return nil, fmt.Errorf("pgraph: bad log header")
			}
			continue
		}
		r, err := decodeRecord(line)
		if err != nil {
			return nil, fmt.Errorf("pgraph: line %d: %w", n, err)
		}
		_, isLive := l.live[r.fact.key()]
		if (r.op == Assert) == isLive {
			return nil, fmt.Errorf("pgraph: line %d: %s of a fact that is %slive", n, map[Op]string{Assert: "assert", Retract: "retract"}[r.op], map[bool]string{true: "already ", false: "not "}[isLive])
		}
		l.apply(r)
	}
	return l, nil
}

func decodeRecord(line string) (Record, error) {
	c := strings.Split(line, "\t")
	if len(c) != recordFields {
		return Record{}, fmt.Errorf("%d fields, want %d", len(c), recordFields)
	}
	var r Record
	switch c[0] {
	case "+":
		r.op = Assert
	case "-":
		r.op = Retract
	default:
		return Record{}, fmt.Errorf("unknown op %q", c[0])
	}
	for i, name := range []string{"op", "subject", "predicate", "object", "tier", "config", "commit", "extractor"} {
		if i > 0 {
			if err := checkField(name, c[i]); err != nil {
				return Record{}, err
			}
		}
	}
	p, err := ParsePredicate(c[2])
	if err != nil {
		return Record{}, err
	}
	t, err := ParseTier(c[4])
	if err != nil {
		return Record{}, err
	}
	if err := admitted(t, p); err != nil {
		return Record{}, err
	}
	if name, ver, ok := strings.Cut(c[7], "@"); !ok || name == "" || ver == "" {
		return Record{}, fmt.Errorf("extractor %q is not name@version", c[7])
	}
	r.fact = Fact{Triple{c[1], p, c[3]}, t, c[5], c[6], c[7]}
	return r, nil
}
