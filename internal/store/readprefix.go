package store

import (
	"bytes"
	"encoding/json"
	"errors"
)

// The one-pass reader's parts (card 2.3b). Every record the store writes is
// marshalled from a struct whose first two fields are Type and SchemaVersion
// (marshalLine), so its line begins exactly
//
//	{"type":"<kind>","schema_version":<n>,
//
// and classifyRunLine reads the pair from those bytes instead of decoding a
// head before the record. A line in any other form takes the decode path.
const (
	prefixType    = `{"type":"`
	prefixVersion = `","schema_version":`
	// prefixDigits is the most digits prefixPair reads a version from; no
	// version is near it, and a longer one is read by decoding.
	prefixDigits = 3
)

// runKindNames is kindVersion's names, each mapped to itself, so that
// prefixPair returns the kind a line names as the table's own string:
// indexing a map with a converted byte slice allocates nothing. A kind added
// to kindVersion is read from its prefix with no row here.
var runKindNames = func() map[string]string {
	names := make(map[string]string, len(kindVersion))
	for kind := range kindVersion {
		names[kind] = kind
	}
	return names
}()

// prefixPair reads a line's pair from its first bytes when the line begins in
// the writer's form: prefixType, a kind named in kindVersion, prefixVersion,
// one to prefixDigits ASCII digits and a comma. ok is false for any other
// line -- leading whitespace, keys in another order, an unknown kind or one
// written with an escape, a signed or longer version -- and the caller
// decodes the pair instead. It reads at most the longest kind name plus 32
// bytes, decodes nothing and allocates nothing.
func prefixPair(line []byte) (kind string, version int, ok bool) {
	rest, ok := cutPrefix(line, prefixType)
	if !ok {
		return "", 0, false
	}
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return "", 0, false
	}
	if kind, ok = runKindNames[string(rest[:end])]; !ok {
		return "", 0, false
	}
	if rest, ok = cutPrefix(rest[end:], prefixVersion); !ok {
		return "", 0, false
	}
	n := 0
	for n < len(rest) && n <= prefixDigits && '0' <= rest[n] && rest[n] <= '9' {
		version = version*10 + int(rest[n]-'0')
		n++
	}
	if n == 0 || n > prefixDigits || n == len(rest) || rest[n] != ',' {
		return "", 0, false
	}
	return kind, version, true
}

// cutPrefix is bytes.CutPrefix with a string prefix, compared in place.
func cutPrefix(b []byte, prefix string) ([]byte, bool) {
	if len(b) < len(prefix) || string(b[:len(prefix)]) != prefix {
		return b, false
	}
	return b[len(prefix):], true
}

// errPairChanged is decodeAs's refusal of a record whose decoded pair is not
// the pair its line was read under.
var errPairChanged = errors.New("store: a record's type or schema_version key repeats with another value")

// pairRecord is a record that reports the pair it decoded.
type pairRecord interface {
	pair() (kind string, version int)
}

func (d *Declaration) pair() (string, int) { return d.Type, d.SchemaVersion }
func (x *Execution) pair() (string, int)   { return x.Type, x.SchemaVersion }
func (t *Terminal) pair() (string, int)    { return t.Type, t.SchemaVersion }
func (c *Coverage) pair() (string, int)    { return c.Type, c.SchemaVersion }
func (h *EventHeader) pair() (string, int) { return h.Type, h.SchemaVersion }
func (h *runHead) pair() (string, int)     { return h.Type, h.SchemaVersion }

// decodeAs decodes line into rec and refuses the record when the pair it
// decoded is not (kind, version), the pair the line was read under (D97).
// The two differ only when a later type or schema_version key, which
// encoding/json reads last-wins and without regard to case, changed it: not
// a record this store wrote, so it is collected under neither pair. Every
// path through classifyRunLine checks here: each accepted kind, the
// default branch, and the head a refused pair is passed over by.
func decodeAs(line []byte, rec pairRecord, kind string, version int) error {
	if err := json.Unmarshal(line, rec); err != nil {
		return err
	}
	if k, v := rec.pair(); k != kind || v != version {
		return errPairChanged
	}
	return nil
}
