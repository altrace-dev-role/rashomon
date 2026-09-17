// Package store is the on-disk record of declarations.
//
// records.ndjson is append-only and totally ordered by seq. Nothing rewrites a
// line in it except forget, and forget leaves a gap record behind, because a
// record that can be quietly corrected is worth nothing as evidence.
package store

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	dirMode  fs.FileMode = 0o700
	fileMode fs.FileMode = 0o600

	keyLen       = 32
	installIDLen = 16

	fileInstallKey  = "install.key"
	fileInstallMeta = "install.json"
	dirRuns         = "runs"
)

// ErrLockTimeout is returned when an append lock could not be taken inside the
// budget. It is a give-up, and callers record it rather than hiding it.
var ErrLockTimeout = errors.New("store: timed out waiting for the append lock")

// Store is an opened store rooted at a directory.
type Store struct {
	root      string
	installID string
	key       []byte
}

type installMeta struct {
	InstallID       string `json:"install_id"`
	CreatedAtUnixMS int64  `json:"created_at_unix_ms"`
	SchemaVersion   int    `json:"schema_version"`
}

// DefaultRoot resolves the store location.
//
// ATTEST_HOME wins so that a test never has to touch a real home directory, and
// so that a user can put the store on a volume they control.
func DefaultRoot() (string, error) {
	if v := os.Getenv("ATTEST_HOME"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "attest"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "attest"), nil
}

// Open prepares the store, creating the root, the per-install HMAC key and the
// install identity on first use.
//
// Install identity is deliberately not session identity. watch runs before any
// Claude Code session exists, so session_id is unavailable at install time, and
// an owner concept that depends on it cannot describe who installed what.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, dirMode); err != nil {
		return nil, err
	}
	// MkdirAll honours the umask and does nothing to a directory that already
	// exists, so neither path guarantees the mode on its own.
	if err := os.Chmod(root, dirMode); err != nil {
		return nil, err
	}

	key, err := loadOrCreateKey(filepath.Join(root, fileInstallKey))
	if err != nil {
		return nil, err
	}
	meta, err := loadOrCreateMeta(filepath.Join(root, fileInstallMeta))
	if err != nil {
		return nil, err
	}
	return &Store{root: root, installID: meta.InstallID, key: key}, nil
}

// Root is the directory this store occupies.
func (s *Store) Root() string { return s.root }

// InstallID identifies this install, not this session.
func (s *Store) InstallID() string { return s.installID }

// Key is the per-install HMAC key used to digest tool input. Two installs hold
// different keys, so the same command digests differently in each, and a store
// cannot be matched against a dictionary of digests built elsewhere.
func (s *Store) Key() []byte { return s.key }

// RunDir is the directory holding one session's records.
func (s *Store) RunDir(sessionID string) string {
	return filepath.Join(s.root, dirRuns, segment(sessionID))
}

// AppendDeclaration records what the agent asked to run, allocating its seq.
func (s *Store) AppendDeclaration(rec Declaration) error {
	return s.appendOrdered(rec.SessionID, func(seq int64) any {
		rec.Seq = seq
		return rec
	})
}

// AppendTerminal closes a declaration. If the ordered stream's lock cannot be
// taken it falls back to the spill file, so that the terminal -- and the
// tool_use_id it names -- lands regardless.
func (s *Store) AppendTerminal(rec Terminal) error {
	err := s.appendOrdered(rec.SessionID, func(seq int64) any {
		rec.Seq = &seq
		return rec
	})
	if !errors.Is(err, ErrLockTimeout) {
		return err
	}
	return s.SpillTerminal(rec)
}

// SpillTerminal writes a terminal to the spill file without touching the
// ordered stream. It is for the caller that has already spent the lock budget
// failing to write the declaration and must not spend it again.
func (s *Store) SpillTerminal(rec Terminal) error {
	rec.Seq = nil
	return s.appendLine(filepath.Join(s.RunDir(rec.SessionID), FileSpill), rec, spillBudget, true)
}

// AppendCoverage records the configuration state as resolved during this run.
func (s *Store) AppendCoverage(rec Coverage) error {
	return s.appendLine(filepath.Join(s.RunDir(rec.SessionID), FileCoverage), rec, lockBudget, false)
}

// AppendGap records that records left the store.
func (s *Store) AppendGap(rec Gap) error {
	return s.appendLine(filepath.Join(s.root, FileGaps), rec, lockBudget, false)
}

// DirName is the run directory name a session id maps to.
func (s *Store) DirName(sessionID string) string { return segment(sessionID) }

// MarkProbe records that the liveness probe ran for a session. It is a file
// rather than a record so that a handler can check it with one stat instead of
// reading a coverage file that grows with every call.
func (s *Store) MarkProbe(sessionID string, now time.Time) error {
	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	content := []byte(strconv.FormatInt(now.UnixMilli(), 10) + "\n")
	return writeSmall(filepath.Join(dir, FileProbe), content)
}

// ProbeFresh reports whether the liveness probe ran for a session.
func (s *Store) ProbeFresh(sessionID string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.RunDir(sessionID), FileProbe))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// appendOrdered appends one record to records.ndjson under the run's lock,
// allocating its seq inside the same critical section.
func (s *Store) appendOrdered(sessionID string, build func(seq int64) any) error {
	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, FileRecords), os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure

	unlock, err := lockFile(f, lockBudget)
	if err != nil {
		return err
	}
	defer unlock()

	seq, err := nextSeq(dir)
	if err != nil {
		return err
	}
	line, err := marshalLine(build(seq))
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Sync()
}

// spillBudget is how long a spill write waits for the spill file's lock. It is
// short because the spill path exists for a handler that has already waited
// once; the lock is taken so that forget, which rewrites the spill file under
// it, is not raced, and given up so that no handler ever blocks on it.
const spillBudget = 50 * time.Millisecond

// appendLine appends one record to a file that carries no seq, waiting up to
// budget for its lock. With bestEffort set, a lock timeout is not an error: the
// record is appended anyway, relying on a single O_APPEND write, which the
// kernel does not interleave for a regular file.
func (s *Store) appendLine(path string, rec any, budget time.Duration, bestEffort bool) error {
	line, err := marshalLine(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure

	unlock, err := lockFile(f, budget)
	switch {
	case err == nil:
		defer unlock()
	case bestEffort && errors.Is(err, ErrLockTimeout):
	default:
		return err
	}

	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Sync()
}

func marshalLine(rec any) ([]byte, error) {
	line, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	// encoding/json escapes control characters, so a newline cannot appear
	// inside a marshalled record and NDJSON framing cannot be broken by input.
	// Checking anyway turns a framing bug into a refusal rather than a store
	// that silently reads back as a different record.
	if bytes.ContainsAny(line, "\n\r") {
		return nil, errors.New("store: record would break NDJSON framing")
	}
	return append(line, '\n'), nil
}

// nextSeq allocates the next sequence number for a run. It must be called with
// the run's records lock held.
//
// The counter is advanced and synced before the record that uses it is
// written. A crash between the two skips a number, which is visible and
// harmless; the other order could hand the same number to two records, which
// is neither.
func nextSeq(dir string) (int64, error) {
	path := filepath.Join(dir, FileSeq)

	last, err := readSeq(path)
	if err != nil {
		// Missing or unreadable: the records themselves are the authority.
		last, err = maxSeq(filepath.Join(dir, FileRecords))
		if err != nil {
			return 0, err
		}
	}

	next := last + 1
	if err := writeSmall(path, []byte(strconv.FormatInt(next, 10)+"\n")); err != nil {
		return 0, err
	}
	return next, nil
}

func readSeq(path string) (int64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
}

func maxSeq(recordsPath string) (int64, error) {
	var max int64
	err := eachLine(recordsPath, func(line []byte) {
		var probe struct {
			Seq int64 `json:"seq"`
		}
		if json.Unmarshal(line, &probe) == nil && probe.Seq > max {
			max = probe.Seq
		}
	})
	return max, err
}

// writeSmall replaces a small file's content and syncs it. It is for the seq
// counter and the probe marker, both written under conditions where a torn
// write is recoverable from other state.
func writeSmall(path string, content []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure
	if _, err := f.Write(content); err != nil {
		return err
	}
	return f.Sync()
}

// segment turns a session id into one path element that cannot escape the store.
//
// The id arrives from outside this program. Joining it into a path unchecked is
// how a value containing ../.. writes wherever it likes, so anything outside a
// conservative allowlist is replaced by a digest of itself: still stable, still
// one directory per session, and no longer a path.
func segment(id string) string {
	if isPlainSegment(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "x-" + hex.EncodeToString(sum[:16])
}

func isPlainSegment(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 128 {
		return false
	}
	return strings.IndexFunc(id, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		case r == '-', r == '_', r == '.':
			return false
		}
		return true
	}) < 0
}

// settleAttempts bounds how long a reader waits for a file that exists but is
// not yet complete. That state is only reachable on a filesystem without
// link(2), where createExclusive falls back to create-then-write; a few short
// waits cover the window, and a file that stays empty is then reported as
// malformed rather than retried forever.
const (
	settleAttempts = 20
	settleDelay    = 5 * time.Millisecond
)

func loadOrCreateKey(path string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, err
		}
		key, derr := hex.DecodeString(strings.TrimSpace(string(b)))
		if derr == nil && len(key) == keyLen {
			return key, nil
		}
		if attempt >= settleAttempts {
			return nil, errors.New("store: install key is malformed")
		}
		time.Sleep(settleDelay)
	}

	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	if err := createExclusive(path, []byte(hex.EncodeToString(key)+"\n")); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Another handler created it first. Its key is the install's key;
			// ours was never linked and is discarded.
			return loadOrCreateKey(path)
		}
		return nil, err
	}
	return key, nil
}

func loadOrCreateMeta(path string) (installMeta, error) {
	var meta installMeta
	for attempt := 0; ; attempt++ {
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return meta, err
		}
		if json.Unmarshal(b, &meta) == nil && meta.InstallID != "" {
			return meta, nil
		}
		if attempt >= settleAttempts {
			return meta, errors.New("store: install metadata is malformed")
		}
		time.Sleep(settleDelay)
	}

	raw := make([]byte, installIDLen)
	if _, err := rand.Read(raw); err != nil {
		return meta, err
	}
	meta = installMeta{
		InstallID:       hex.EncodeToString(raw),
		CreatedAtUnixMS: time.Now().UnixMilli(),
		SchemaVersion:   SchemaVersion,
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return meta, err
	}

	if err := createExclusive(path, append(encoded, '\n')); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return loadOrCreateMeta(path)
		}
		return meta, err
	}
	return meta, nil
}

// createExclusive publishes a fully written file under a name that must not
// already exist.
//
// Creating the final name first and writing into it afterwards opens a window
// in which a second process sees the file exist, reads it, and finds it empty.
// On a first run with many concurrent handlers that window is hit, and every
// loser then refuses to open the store. Writing to a temporary name and
// linking it into place closes the window: the final name either does not
// exist or holds the complete content, and link(2) fails with EEXIST for every
// process but one.
func createExclusive(path string, content []byte) error {
	dir, base := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Whether or not the link is made, the temporary name goes; a successful
	// link keeps the inode alive under the final name.
	defer os.Remove(tmpPath) //nolint:errcheck // best effort cleanup

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

	err = os.Link(tmpPath, path)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	// A filesystem without link(2). Fall back to create-then-write, which
	// reopens the window described above; readers settle over it.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure
	if _, err := f.Write(content); err != nil {
		return err
	}
	return f.Sync()
}

// DefaultCapBytes bounds the store before eviction starts removing the oldest
// runs. ATTEST_STORE_CAP_BYTES overrides it; zero disables eviction.
const DefaultCapBytes = 512 << 20

// CapBytes resolves the store size cap.
func CapBytes() int64 {
	if v := os.Getenv("ATTEST_STORE_CAP_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return DefaultCapBytes
}
