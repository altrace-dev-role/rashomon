// Package store is the on-disk record of declarations.
//
// Everything here is append-only. Nothing rewrites a line, because a record
// that can be rewritten is a record that can be quietly corrected, and the
// value of this store is that it cannot be.
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

// AppendDeclaration records what the agent asked to run.
func (s *Store) AppendDeclaration(rec Declaration) error {
	return s.append(rec.SessionID, FileRecords, rec)
}

// AppendTerminal closes a declaration.
func (s *Store) AppendTerminal(rec Terminal) error {
	return s.append(rec.SessionID, FileRecords, rec)
}

// AppendCoverage records the configuration state as resolved during this run.
func (s *Store) AppendCoverage(rec Coverage) error {
	return s.append(rec.SessionID, FileCoverage, rec)
}

// RunDir is the directory holding one session's records.
func (s *Store) RunDir(sessionID string) string {
	return filepath.Join(s.root, dirRuns, segment(sessionID))
}

func (s *Store) append(sessionID, name string, rec any) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	// encoding/json escapes control characters, so a newline cannot appear
	// inside a marshalled record and NDJSON framing cannot be broken by input.
	// Checking anyway costs one scan and turns a framing bug into a refusal
	// rather than a store that silently reads back as a different record.
	if bytes.ContainsAny(line, "\n\r") {
		return errors.New("store: record would break NDJSON framing")
	}
	line = append(line, '\n')

	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}

	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // the Sync below is what reports a write failure

	unlock, err := lockFile(f, lockBudget)
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := f.Write(line); err != nil {
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

func loadOrCreateKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		key, derr := hex.DecodeString(strings.TrimSpace(string(b)))
		if derr != nil || len(key) != keyLen {
			return nil, errors.New("store: install key is malformed")
		}
		return key, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, keyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	encoded := []byte(hex.EncodeToString(key) + "\n")

	if err := createExclusive(path, encoded); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Another handler created it first. Its key is the install's key;
			// ours was never written and is discarded.
			return loadOrCreateKey(path)
		}
		return nil, err
	}
	return key, nil
}

func loadOrCreateMeta(path string) (installMeta, error) {
	var meta installMeta
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &meta) != nil || meta.InstallID == "" {
			return meta, errors.New("store: install metadata is malformed")
		}
		return meta, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return meta, err
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

// createExclusive writes a file that must not already exist, so that two
// handlers racing on first run agree on one key and one install id rather than
// each overwriting the other's.
func createExclusive(path string, content []byte) error {
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
