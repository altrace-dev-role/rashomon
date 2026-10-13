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
	"sync"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/fault"
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

// ErrLockTimeout is returned when a lock could not be taken inside the budget
// (LockFile), and by nothing else: it means busy, never broken. It is a
// give-up, and callers record it rather than hiding it.
var ErrLockTimeout = errors.New("store: timed out waiting for the append lock")

// Store is an opened store rooted at a directory.
//
// A Store comes from Open, which creates the store on first use, or from
// OpenExisting, which never does; its root, install id and key are set there
// and never changed. Every path it writes is joined onto that root, so the
// zero Store has none: it would write under the process's working directory
// and digest under an empty key, and every writer refuses it with
// errUnopenedStore instead.
//
// Appends from separate processes -- every hook invocation is one -- are
// serialised by an advisory lock on each file they write, taken through a
// descriptor of the append's own, which is also what orders appends from
// goroutines sharing one Store. A Store is safe for concurrent use until
// DeferDurability or LimitLockWaits (TestRubricDOCS9_OneStoreIsSafeAcrossGoroutines,
// run under -race centrally). Either call gives it mutable state --
// DeferDurability the file its one full sync has to cover and whether that
// sync failed, LimitLockWaits the deadline its lock waits share -- which only
// one goroutine may touch: a Store after either belongs to the one hook
// invocation that opened it. mu guards DeferDurability's state anyway.
// Nothing in this program holds it across a second goroutine yet, but a
// field three places read and write as one unit, left unguarded, is a race
// waiting for whichever future caller reaches it from two; the lock makes
// that caller's bug a serialised wait instead of an undefined one (REC-34,
// Go memory model).
type Store struct {
	root      string
	installID string
	key       []byte

	// Group commit (REC-34); see DeferDurability and Flush. barrier is
	// barrierSync; a test stands another platform's in for it.
	barrier func(*os.File) (durable bool, err error)

	mu       sync.Mutex
	deferred bool
	dirty    string
	flushErr error

	// waitUntil is the deadline every lock wait shares, zero for none
	// (LimitLockWaits).
	waitUntil time.Time

	// readRefusal says which read errors mean another handle's lock refused
	// the read; nil is readRefused, the platform's own answer. On unix a held
	// flock never refuses a read, so this is how a unix test drives the
	// refusal handling through the readers production calls (test-11).
	readRefusal func(error) bool
}

type installMeta struct {
	InstallID       string `json:"install_id"`
	CreatedAtUnixMS int64  `json:"created_at_unix_ms"`
	SchemaVersion   int    `json:"schema_version"`
}

// DefaultRoot resolves the store location.
//
// RASHOMON_HOME wins so that a test never has to touch a real home directory, and
// so that a user can put the store on a volume they control.
func DefaultRoot() (string, error) {
	if v := os.Getenv("RASHOMON_HOME"); v != "" {
		return v, nil
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "rashomon"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "rashomon"), nil
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
	return &Store{root: root, installID: meta.InstallID, key: key, barrier: barrierSync}, nil
}

// ErrNoStore reports that no store has been created at a location. It is a
// sentinel rather than a plain error because the read commands must be able to
// tell "nothing has been recorded here" from "the store is broken", and only
// the second is a failure.
var ErrNoStore = errors.New("store: no store at this location")

// OpenExisting opens a store WITHOUT creating one, returning ErrNoStore when
// none is there.
//
// Open creates the root, the install identity and the per-install HMAC key on
// first use, which is right for watch and wrong for every command that only
// asks a question: a read that mints key material means a user who ran `report`
// once now has an install identity they never installed, and the next `status`
// truthfully reports a store as present because a different command fabricated
// it. Presence is decided by the same file a plain detach looks for.
func OpenExisting(root string) (*Store, error) {
	if root == "" {
		return nil, ErrNoStore
	}
	if _, err := os.Stat(filepath.Join(root, fileInstallMeta)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoStore
		}
		return nil, err
	}
	return Open(root)
}

// Root is the directory this store occupies.
func (s *Store) Root() string { return s.root }

// InstallID identifies this install, not this session.
func (s *Store) InstallID() string { return s.installID }

// Key is the per-install HMAC key used to digest tool input. Two installs hold
// different keys, so the same command digests differently in each, and a store
// cannot be matched against a dictionary of digests built elsewhere.
//
// It returns a copy: a caller that wrote into the Store's own slice would
// change every digest the Store made after it (lld-8).
func (s *Store) Key() []byte { return bytes.Clone(s.key) }

// errUnopenedStore is a write to a Store that did not come from Open or
// OpenExisting. It is not ErrNoStore, which a read command shows the user as
// "nothing recorded here": this is a bug in the caller, never a state of the
// disk.
var errUnopenedStore = errors.New("store: write to a Store that was never opened")

// opened refuses the zero Store before it writes anything (see Store).
func (s *Store) opened() error {
	if s.root == "" {
		return errUnopenedStore
	}
	return nil
}

// RunDir is the directory holding one session's records.
func (s *Store) RunDir(sessionID string) string {
	return filepath.Join(s.root, dirRuns, segment(sessionID))
}

// DeferDurability is group commit (REC-34). Every later append on this Store
// orders its write with a barrier inside its lock instead of syncing it in
// full, and the one full sync is left to Flush, which the caller issues
// after its last append, outside the lock and before it writes anything to
// stdout.
//
// It is for a process that appends a few records and exits: hook and post.
// Durability moves from "when each append returns" to "when the process has
// flushed", and PreToolUse flushes before it exits, so a declaration is still
// on stable storage before the tool runs -- when the flush succeeds; when it
// fails the tool runs anyway and the call is marked flush_failed instead.
//
// One case weakens, on darwin only, where a barrier is cheaper than a full
// sync: a process that ends between its barriers and Flush leaves records
// ordered on the device but not flushed from its cache. SIGINT, SIGTERM,
// SIGHUP and SIGQUIT are all caught (hook.WatchSignals) and still reach
// Flush. Anything else that ends the process first opens the window:
// SIGKILL; SIGPIPE on a write to a closed stderr; and a Go runtime fatal
// error, which no recover catches. What it leaves exposed stays exposed after
// the process is gone: until the device next empties its cache, at the next
// full sync on that volume by any process (usually the next hook's Flush) or
// the drive's own write-back, a power failure can lose it. Under 1.1.0 such a declaration was durable when
// its append returned. Linux and Windows have no such window, because there
// every barrier is a full sync: each append is durable when it returns, as
// under 1.1.0, and Flush has nothing left to sync.
//
// On darwin the one full sync is the only one a hook issues once its store
// is open, and off darwin a hook issues none beyond its barriers; a
// first-ever hook, which creates the store, syncs the install files in full
// before that. probe, recap and the paused path never call this, and keep a
// full sync per append.
//
// After this call the Store is not for concurrent use.
func (s *Store) DeferDurability() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deferred = true
}

// Flush is the group commit's one full sync: it makes every append since
// DeferDurability durable. It syncs the last file appended to, and that one
// sync covers the others: on darwin F_FULLFSYNC empties the device's whole
// cache, persisting everything a barrier already pushed to it (fcntl(2)).
// With nothing left to sync it does nothing: nothing appended, or every
// append's barrier itself a full sync, as it always is off darwin and is on
// darwin when the filesystem refuses the barrier (syncAppend). It takes no
// lock.
//
// A failure is final, whether it was this sync's or a barrier's before it
// (syncFailed): this Store never reports durability again, and a later Flush
// returns the same error without syncing. A retried fsync can succeed
// for pages the kernel dropped when the first one failed, which would turn
// "possibly lost" into "fine". The caller marks the records instead, with
// ReasonFlushFailed. A failure also ends the deferral: no later Flush can
// cover a barrier-ordered append, so every append after it -- that marker
// first -- is synced in full inside its own lock, as an undeferred one is.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.flushErr != nil {
		return s.flushErr
	}
	if s.dirty == "" {
		return nil
	}
	if err := flushFile(s.dirty); err != nil {
		return s.syncFailedLocked(err)
	}
	s.dirty = ""
	return nil
}

// syncFailed makes a failed sync of a deferred append final, and is the one
// place a failure is recorded, whether Flush's full sync failed or a barrier
// inside an append did. The error is sticky: every later Flush returns it and
// syncs nothing, because a retried sync can succeed for pages the kernel
// dropped when the first one failed. And the deferral ends, because no later
// Flush can cover a barrier-ordered append: every append after this one -- the
// flush_failed marker first -- is synced in full inside its own lock.
func (s *Store) syncFailed(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncFailedLocked(err)
}

// syncFailedLocked is syncFailed's body, for a caller already holding mu.
func (s *Store) syncFailedLocked(err error) error {
	s.flushErr = err
	s.deferred = false
	return err
}

// flushFile syncs path in full. It opens the file for writing as well as
// reading, because Windows' FlushFileBuffers refuses a read-only handle.
//
// A reopen that fails because the run is gone -- forget or eviction removed
// the whole directory (gaps.go's os.RemoveAll) between the append that left
// this path dirty and this Flush -- is not a sync failure: there is nothing
// left anywhere to make durable, and the caller marking a failure would
// recreate the very directory that was removed (SpillTerminal's MkdirAll).
// A path whose FILE alone is missing, its run directory still there, is a
// real failure and falls through to the error below, unchanged.
func flushFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && runGone(path) {
			return nil
		}
		return err
	}
	defer f.Close() //nolint:errcheck // the sync reports the failure that matters
	return syncFull(f)
}

// runGone reports whether path's parent directory no longer exists: the
// difference between "this run was removed" and "this one file within a
// surviving run is missing", which a bare ENOENT from OpenFile cannot tell
// apart on its own.
func runGone(path string) bool {
	_, err := os.Stat(filepath.Dir(path))
	return errors.Is(err, fs.ErrNotExist)
}

// AppendDeclaration records what the agent asked to run, allocating its seq.
func (s *Store) AppendDeclaration(rec Declaration) error {
	return s.appendOrdered(rec.SessionID, func(seq int64) any {
		rec.Seq = seq
		return rec
	})
}

// AppendExecution records that a declared call ran, allocating its seq. If the
// ordered stream's lock cannot be taken it falls back to the spill file: an
// execution is the only record its tool_use_id gets, so dropping it would read
// afterwards as a call that never ran.
func (s *Store) AppendExecution(rec Execution) error {
	err := s.appendOrdered(rec.SessionID, func(seq int64) any {
		rec.Seq = &seq
		return rec
	})
	if !errors.Is(err, ErrLockTimeout) {
		return err
	}
	return s.SpillExecution(rec)
}

// SpillExecution writes an execution to the spill file without touching the
// ordered stream, under the same rule as SpillTerminal.
func (s *Store) SpillExecution(rec Execution) error {
	rec.Seq = nil
	return s.appendLine(filepath.Join(s.RunDir(rec.SessionID), FileSpill), rec, spillBudget, true)
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

// NewGap is a gap record of reason for a session, recorded at now: the one
// constructor every gap is built with, so each carries its type and version
// (naming-10). The caller sets the span, the count and the host digest.
func NewGap(reason, sessionID string, now time.Time) Gap {
	return stampGap(Gap{RecordedAtMS: now.UnixMilli(), SessionID: sessionID, Reason: reason})
}

// stampGap sets what every gap carries whoever built it: its type, and the
// version of the session records, which a gap is.
func stampGap(g Gap) Gap {
	g.Type, g.SchemaVersion = TypeGap, SessionSchemaVersion
	return g
}

// AppendGap records that records left the store. It stamps the gap's type
// and version itself, so a caller cannot write a gap a reader skips.
func (s *Store) AppendGap(rec Gap) error {
	return s.appendLine(filepath.Join(s.root, FileGaps), stampGap(rec), lockBudget, false)
}

// DirName is the run directory name a session id maps to.
func (s *Store) DirName(sessionID string) string { return segment(sessionID) }

// MarkProbe records that the liveness probe ran for a session. It is a file
// rather than a record so that a handler can check it with one stat instead of
// reading a coverage file that grows with every call.
func (s *Store) MarkProbe(sessionID string, now time.Time) error {
	if err := s.opened(); err != nil {
		return err
	}
	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	content := []byte(strconv.FormatInt(now.UnixMilli(), 10) + "\n")
	return writeSmall(filepath.Join(dir, FileProbe), content, syncFull)
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

// appendFlags open a record file for appending. The read access is not for
// reading: Windows grants an O_APPEND handle FILE_APPEND_DATA and neither
// FILE_READ_DATA nor FILE_WRITE_DATA, and LockFileEx refuses such a handle, so
// without O_RDWR every append on Windows fails to take its lock.
const appendFlags = os.O_CREATE | os.O_RDWR | os.O_APPEND

// appendOrdered appends one record to records.ndjson under the run's lock,
// allocating its seq inside the same critical section.
func (s *Store) appendOrdered(sessionID string, build func(seq int64) any) error {
	if err := s.opened(); err != nil {
		return err
	}
	dir := s.RunDir(sessionID)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, FileRecords), appendFlags, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure

	unlock, err := lockCounted(f, s.lockWait(lockBudget))
	if err != nil {
		return err
	}
	defer unlock()
	return s.writeOrderedLocked(f, dir, build)
}

// writeOrderedLocked is an ordered append's critical section: it allocates
// the seq, writes the record and syncs it, with the caller holding f's lock.
// It is all the lock is held for, and BenchmarkOrderedAppendHold times
// exactly this function.
func (s *Store) writeOrderedLocked(f *os.File, dir string, build func(seq int64) any) error {
	seq, err := nextSeq(dir, s.syncAppend)
	if err != nil {
		return err
	}
	line, err := marshalLine(build(seq))
	if err != nil {
		return err
	}
	if err := appendHealed(f, line); err != nil {
		return err
	}
	return s.syncAppend(f)
}

// lockCounted takes f's lock, and every lock the store takes goes through
// it: the appends', forget's and eviction's, and the readers'. The fault
// build counts each as held until it is released, so the sync recorder can
// say whether a sync ran inside any lock at all (REC-34); a released build
// compiles the count away.
func lockCounted(f *os.File, budget time.Duration) (func(), error) {
	unlock, err := LockFile(f, budget)
	if err != nil {
		return nil, err
	}
	fault.LockTaken()
	return func() {
		unlock()
		fault.LockReleased()
	}, nil
}

// syncAppend is the durability step of every append, taken inside its lock:
// a full sync, or on a deferred Store a barrier, which leaves the full sync
// to Flush. A barrier that was itself a full sync -- every barrier off
// darwin, and darwin's fallback -- leaves nothing to Flush: the full sync
// that made this append durable made every earlier one on the device durable
// too, so a deferred hook issues no sync an undeferred one would not.
//
// A barrier that fails is a sync that failed, and is final the way a failed
// Flush is (syncFailed): Flush must not full-sync the same file and report it
// durable, which on Linux, where the barrier is fsync, would be the very
// retry it refuses.
func (s *Store) syncAppend(f *os.File) error {
	s.mu.Lock()
	deferred := s.deferred
	if deferred {
		s.dirty = f.Name()
	}
	s.mu.Unlock()
	if !deferred {
		return syncFull(f)
	}
	durable, err := s.barrier(f)
	if err == nil {
		err = fault.Fail(fault.PointStoreBarrier)
	}
	if err != nil {
		return s.syncFailed(err)
	}
	if durable {
		s.mu.Lock()
		s.dirty = ""
		s.mu.Unlock()
	}
	return nil
}

// syncFull is a full sync: F_FULLFSYNC on darwin, fsync elsewhere. Every full
// sync the store issues goes through it, the darwin barrier's fallback
// included, so the fault build's recorder sees each one.
func syncFull(f *os.File) error {
	return syncAs(f, fault.SyncFull)
}

// syncFullAsBarrier is syncFull's own full sync, issued where a barrier was
// wanted instead of after the last append: off darwin, where nothing cheaper
// than a full sync orders a write at all (sync_other.go), and on darwin when
// the filesystem refuses F_BARRIERFSYNC (sync_darwin.go's barrierRefused
// fallback). The kernel call is identical to syncFull's -- it shares the
// same fault point, PointStoreFullSync, because failing one is failing the
// other -- but it runs INSIDE the append's lock, which is exactly what
// SyncFull's name promises never happens. Logging it as SyncBarrierFull
// instead keeps the recorder honest about where a real full sync ran: off
// darwin every ordered append pays this cost, not the cheap barrier
// SyncBarrier's name describes, and REC-34's acceptance test reads the
// difference rather than being unable to see it.
func syncFullAsBarrier(f *os.File) error {
	return syncAs(f, fault.SyncBarrierFull)
}

func syncAs(f *os.File, kind string) error {
	fault.RecordSync(kind, f.Name())
	if err := f.Sync(); err != nil {
		return err
	}
	return fault.Fail(fault.PointStoreFullSync)
}

// appendLine appends one record to a file that carries no seq, waiting up to
// budget for its lock. With bestEffort set, a lock timeout is not an error: the
// record is appended anyway, relying on a single O_APPEND write, which the
// kernel does not interleave for a regular file.
func (s *Store) appendLine(path string, rec any, budget time.Duration, bestEffort bool) error {
	if err := s.opened(); err != nil {
		return err
	}
	line, err := marshalLine(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}

	f, err := os.OpenFile(path, appendFlags, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure

	unlock, err := lockCounted(f, s.lockWait(budget))
	switch {
	case err == nil:
		defer unlock()
		err = appendHealed(f, line)
	case bestEffort && errors.Is(err, ErrLockTimeout):
		// Without the lock the last byte could change under the read, and
		// a cut-back could take another writer's record with it, so this
		// append neither heals a torn tail nor cuts itself back.
		_, err = f.Write(line)
	default:
		return err
	}
	if err != nil {
		return err
	}
	return s.syncAppend(f)
}

// marshalLine is rec as one NDJSON line, or ErrRecordOversize when that line
// would be over MaxRecordBytes (limits.go).
func marshalLine(rec any) ([]byte, error) {
	line, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	if len(line)+1 > MaxRecordBytes {
		return nil, ErrRecordOversize
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
// the run's records lock held, and sync is that append's own sync.
//
// The counter is advanced and ordered before the record that uses it is
// written: synced in full, or, on a deferred Store, put behind a barrier, so
// it reaches stable storage no later than the record does. A crash between
// the two skips a number, which is visible and harmless; the other order
// could hand the same number to two records, which is neither.
func nextSeq(dir string, sync func(*os.File) error) (int64, error) {
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
	if err := writeSmall(path, []byte(strconv.FormatInt(next, 10)+"\n"), sync); err != nil {
		return 0, err
	}
	fault.Inject(fault.PointStoreSeqAdvanced)
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

// writeSmall replaces a small file's content and syncs it with sync. It is
// for the seq counter and the probe marker, both written under conditions
// where a torn write is recoverable from other state.
func writeSmall(path string, content []byte, sync func(*os.File) error) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // sync below reports the write failure
	if _, err := f.Write(content); err != nil {
		return err
	}
	return sync(f)
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

// maxPlainSegment is the longest session id used verbatim as a run directory
// name; a longer one is digested like any other unsafe id. A Claude Code
// session id is a 36-character UUID, so a real one is never digested. 128
// keeps the name at half of NAME_MAX (255 on Linux and darwin), and keeps a
// run's longest path -- the root, \runs\, the name and \records.ndjson --
// inside Windows' 260-character MAX_PATH for any root up to 110 characters.
const maxPlainSegment = 128

func isPlainSegment(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > maxPlainSegment {
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
	if err := syncFull(tmp); err != nil {
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
	return syncFull(f)
}
