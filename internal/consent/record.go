// Package consent is what the user agreed to and how they were asked: the
// consent screen rendered from the inventory, the questions asked on the
// terminal, and consent.json, the one file every layer reads consent from
// (Technical spec 7.2.3).
//
// consent.json is newline-delimited: one accepted record per line, appended
// with O_APPEND and synced, never rewritten in place except by compaction at
// its cap. Three readers answer three questions from the same lines, and a
// missing file, or one with no line that parses, is no consent at all three:
//
//   - Current: may a note reach the agent (STEER)? It is SteerDisclosure, the
//     highest steer disclosure on file, against this build's version.
//   - RecordScope: which record shape may RECORD write?
//   - ScopeCovered: does the consent on file cover this build's capability
//     classes, so watch can say "already watching" instead of asking again?
//
// The hook reads none of them on a tool call: it reads the record-scope
// marker (store.FileRecordScope) and the steer-disclosure marker
// (store.FileSteerDisclosure), which every Append keeps equal to RecordScope's
// and SteerDisclosure's answers.
package consent

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// RecordVersion is the v every record carries.
const RecordVersion = 1

// CapBytes is consent.json's cap. An append that would take the file past it
// compacts the file first.
const CapBytes = 64 << 10

// maxReadBytes bounds what a reader takes from a file that has outgrown its
// cap (a writer that does not compact, or a hand edit). The lines past it are
// not read; a line cut by the bound does not parse and is skipped like a torn
// append.
const maxReadBytes = 4 << 20

// Record scopes RECORD can write (Technical spec 1.2; 7.2.3), the store's
// two words: the store's marker (store.FileRecordScope) holds one of them.
const (
	RecordScopeV4 = store.RecordScopeV4
	RecordScopeV5 = store.RecordScopeV5
)

// Steering states a record carries: whether notes to the agent were on when
// the user agreed.
const (
	SteerOn  = "on"
	SteerOff = "off"
)

// Via is how a consent record came to be written.
type Via string

// The closed set of Via values (Technical spec 7.2.3).
const (
	ViaWatchTTY       Via = "watch_tty"
	ViaWatchYes       Via = "watch_yes"
	ViaPluginEnable   Via = "plugin_enable"
	ViaFirstRunNotice Via = "first_run_notice"
	ViaUpgradeNotice  Via = "upgrade_notice"
	ViaConsentTTY     Via = "consent_tty"
	ViaConsentYes     Via = "consent_yes"
	ViaSteerOn        Via = "steer_on"
)

// Vias is every Via value, in the order the specification lists them.
var Vias = []Via{
	ViaWatchTTY, ViaWatchYes, ViaPluginEnable, ViaFirstRunNotice,
	ViaUpgradeNotice, ViaConsentTTY, ViaConsentYes, ViaSteerOn,
}

// Valid reports whether v is one of Vias.
func (v Via) Valid() bool {
	for _, w := range Vias {
		if v == w {
			return true
		}
	}
	return false
}

// Record is one line of consent.json.
type Record struct {
	V               int    `json:"v"`
	ScopeVersion    int    `json:"scope_version"`
	RecordScope     string `json:"record_scope"`
	SteerDisclosure int    `json:"steer_disclosure"`
	Steer           string `json:"steer"`
	// TextSHA256 is the hex SHA-256 of the exact bytes shown: the screen for
	// watch and consent, the notice text for a notice.
	TextSHA256    string `json:"text_sha256"`
	BinaryVersion string `json:"binary_version"`
	BinarySHA256  string `json:"binary_sha256"`
	Via           Via    `json:"via"`
	// TTY says whether the answer was read from a terminal.
	TTY bool `json:"tty"`
	// PluginOptionSteering is the plugin's userConfig.steering when a plugin
	// wrote the record, and null for a settings install.
	PluginOptionSteering *bool  `json:"plugin_option_steering"`
	InvocationID         string `json:"invocation_id"`
	// At is when the record was appended, in Unix milliseconds.
	At int64 `json:"at"`
}

// parses reports whether a decoded line is a record a reader may count: the
// version this build reads and a Via from the closed set. Anything else is
// skipped like a torn line, never read as consent.
func (r Record) parses() bool {
	return r.V == RecordVersion && r.Via.Valid()
}

// Path is consent.json under a store root.
func Path(root string) string { return filepath.Join(root, store.FileConsent) }

// NewInvocationID returns 32 hex characters from crypto/rand, the
// invocation_id of a record written by a command rather than a supervised
// hook child.
func NewInvocationID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Read returns every record in root's consent.json that parses, in file
// order. A missing file is no records and no error. A line that does not
// parse -- a torn last append, a hand edit -- is skipped and the lines around
// it are read.
func Read(root string) ([]Record, error) {
	f, err := os.Open(Path(root))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(f, maxReadBytes))
	if err != nil {
		return nil, err
	}
	return parse(data), nil
}

func parse(data []byte) []Record {
	var out []Record
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r Record
		if json.Unmarshal(line, &r) != nil || !r.parses() {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Current is STEER's reader: whether the highest steer_disclosure on file is
// at least this build's steer.DisclosureVersion. An unreadable file is no
// consent, and the error says why. It is SteerDisclosure compared, so the
// steer-disclosure marker, written from SteerDisclosure, answers the same.
func Current(root string) (bool, error) {
	n, err := SteerDisclosure(root)
	return n >= steer.DisclosureVersion, err
}

// SteerDisclosure is the highest steer_disclosure any record on file
// carries, the fact the steer-disclosure marker holds: 0 with no records,
// and 0 with the error for a file that cannot be read. A negative highest,
// which no build writes and a hand edit could, reads 0: no consent either
// way, and a value the marker can hold.
func SteerDisclosure(root string) (int, error) {
	recs, err := Read(root)
	if err != nil {
		return 0, err
	}
	best, _ := maxBy(recs, func(r Record) int { return r.SteerDisclosure })
	return max(best, 0), nil
}

// RecordScope is RECORD's reader: the record_scope of the latest record. With
// no consent on file, or none that can be read, RECORD keeps the v4 shape.
func RecordScope(root string) (string, error) {
	recs, err := Read(root)
	if err != nil {
		return RecordScopeV4, err
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if s := recs[i].RecordScope; s != "" {
			return s, nil
		}
	}
	return RecordScopeV4, nil
}

// ScopeCovered is TRUST's reader: whether the highest scope_version on file
// is at least this build's disclose.ScopeVersion. An unreadable file covers
// nothing.
func ScopeCovered(root string) (bool, error) {
	recs, err := Read(root)
	if err != nil {
		return false, err
	}
	best, ok := maxBy(recs, func(r Record) int { return r.ScopeVersion })
	return ok && best >= disclose.ScopeVersion, nil
}

// LastAccepted is the latest record written by an act of consent -- watch,
// consent or a plugin enable -- the record the consent screen's CHANGED SINCE
// block compares this build against. ok is false when there is none.
func LastAccepted(recs []Record) (Record, bool) {
	for i := len(recs) - 1; i >= 0; i-- {
		switch recs[i].Via {
		case ViaWatchTTY, ViaWatchYes, ViaConsentTTY, ViaConsentYes, ViaPluginEnable:
			return recs[i], true
		}
	}
	return Record{}, false
}

func maxBy(recs []Record, field func(Record) int) (int, bool) {
	if len(recs) == 0 {
		return 0, false
	}
	best := field(recs[0])
	for _, r := range recs[1:] {
		if v := field(r); v > best {
			best = v
		}
	}
	return best, true
}

// ErrScopeMarker wraps Append's error when the consent line is durable and
// the record-scope marker after it could not be written: the marker is then
// absent, which the hook reads as v4, until a later write succeeds.
var ErrScopeMarker = errors.New("the record_scope marker was not written")

// writeMarker is store.WriteRecordScopeMarker, which TestRS8 replaces to fail
// it: the process dying between the consent line and the marker.
var writeMarker = store.WriteRecordScopeMarker

// ErrSteerMarker wraps Append's error when the consent line is durable and
// the steer-disclosure marker after it could not be written: the marker is
// then absent, which the hook reads as 0, no consent, until a later write
// succeeds.
var ErrSteerMarker = errors.New("the steer_disclosure marker was not written")

// writeSteerMarker is store.WriteSteerDisclosureMarker, which TestSC3b
// replaces to fail it.
var writeSteerMarker = store.WriteSteerDisclosureMarker

// Append adds rec to root's consent.json as one line, appended with O_APPEND
// and synced before it returns, the file at mode 0600. A file whose last line
// was torn gets a line feed first, so the torn bytes stay one skipped line
// instead of swallowing this record. When the append would take the file past
// CapBytes the file is compacted first (compact).
//
// It keeps the store's two markers, record scope (store.FileRecordScope) and
// steer disclosure (store.FileSteerDisclosure), views of the file: it removes
// both before it touches consent.json, compaction included, and writes each
// again from consent.json's own answer once the line is durable. A marker
// lags and never leads: a crash anywhere in between leaves it absent, which
// the hook reads as v4 or as 0, and so withholds, whichever way this record
// moved it. The two are independent: each is written whatever the other's
// write did. An error wrapping ErrScopeMarker or ErrSteerMarker means the
// line is on file and that marker is not; both can be joined in one error.
func Append(root string, rec Record) error {
	if !rec.parses() {
		return fmt.Errorf("consent: refusing to append a record no reader would count (v %d, via %q)", rec.V, rec.Via)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	if err := store.RemoveRecordScopeMarker(root); err != nil {
		return err
	}
	if err := store.RemoveSteerDisclosureMarker(root); err != nil {
		return err
	}
	if err := appendRecord(root, line); err != nil {
		return err
	}
	var scopeErr, steerErr error
	if err := syncMarker(root); err != nil {
		scopeErr = fmt.Errorf("%w: %w", ErrScopeMarker, err)
	}
	if err := syncSteerMarker(root); err != nil {
		steerErr = fmt.Errorf("%w: %w", ErrSteerMarker, err)
	}
	return errors.Join(scopeErr, steerErr)
}

// syncSteerMarker writes root's steer-disclosure marker from consent.json's
// own answer, SteerDisclosure, 0 included. A file SteerDisclosure cannot
// read leaves the marker absent.
func syncSteerMarker(root string) error {
	n, err := SteerDisclosure(root)
	if err != nil {
		return err
	}
	return writeSteerMarker(root, n)
}

// syncMarker writes root's marker from consent.json's own answer, never from
// the record just appended: a record that names no scope leaves the scope
// where the log has it. A scope that is neither word (a hand edit, a later
// build's) is written as v4. A file RecordScope cannot read leaves the
// marker absent.
func syncMarker(root string) error {
	scope, err := RecordScope(root)
	if err != nil {
		return err
	}
	if scope != RecordScopeV5 {
		scope = RecordScopeV4
	}
	return writeMarker(root, scope)
}

// appendRecord is Append's write of one encoded line, compaction first when
// the line would take the file past CapBytes.
func appendRecord(root string, line []byte) error {
	path := Path(root)
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line))+1 > CapBytes {
		if err := compact(root); err != nil {
			return fmt.Errorf("consent: compacting %s: %w", store.FileConsent, err)
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := appendLine(f, line); err != nil {
		f.Close() //nolint:errcheck // the append error is what matters
		return err
	}
	return f.Close()
}

func appendLine(f *os.File, line []byte) error {
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if size := info.Size(); size > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, size-1); err != nil {
			return err
		}
		if last[0] != '\n' {
			line = append([]byte{'\n'}, line...)
		}
	}
	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Sync()
}

// compact rewrites consent.json to the records that decide every reader's
// answer: the latest record per (via, scope_version), which keeps the latest
// record overall and the highest scope_version; the record with the
// highest steer_disclosure, which a newer via of the same scope could
// otherwise push out and so withdraw a consent a downgrade must keep; and
// the latest record that names a record_scope, which a newer record of the
// same via and scope that names none would otherwise push out and so move
// RecordScope's answer. The result is written to a temporary file in the
// same directory, synced and renamed over consent.json, so a crash leaves
// the old file or the new one. It runs only inside Append, after the
// record-scope marker is removed and before it is written again.
func compact(root string) error {
	recs, err := Read(root)
	if err != nil {
		return err
	}
	keep := keepForCompaction(recs)
	var buf bytes.Buffer
	for _, r := range keep {
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return replace(Path(root), buf.Bytes())
}

// keepForCompaction returns the records compact keeps, in file order.
func keepForCompaction(recs []Record) []Record {
	type key struct {
		via   Via
		scope int
	}
	latest := map[key]int{}
	maxDisclosure, scoped := -1, -1
	for i, r := range recs {
		latest[key{r.Via, r.ScopeVersion}] = i
		if maxDisclosure < 0 || r.SteerDisclosure > recs[maxDisclosure].SteerDisclosure {
			maxDisclosure = i
		}
		if r.RecordScope != "" {
			scoped = i
		}
	}
	var out []Record
	for i, r := range recs {
		if latest[key{r.Via, r.ScopeVersion}] == i || i == maxDisclosure || i == scoped {
			out = append(out, r)
		}
	}
	return out
}

// replace writes content to a temporary file beside path, syncs it and
// renames it over path.
func replace(path string, content []byte) error {
	dir, base := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // a no-op after the rename; cleanup before it

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close() //nolint:errcheck // the chmod error is what matters
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close() //nolint:errcheck // the write error is what matters
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close() //nolint:errcheck // the sync error is what matters
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
