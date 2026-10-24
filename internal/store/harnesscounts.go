package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FileHarnessCounts is H1's hook-written mismatch counter (Technical spec
// 6.2.1, 6.2.3), held at the store root beside install.json and
// harnesses.json. Unlike those two -- harnesses.json is command-written
// only (watch, detach, steer, pause) and install.json names an install's
// identity -- this file is written only by a hook entry that has just
// decided to stand down, and RecordHarnessMismatch is the only writer.
const FileHarnessCounts = "harness_counts.json"

// FileHarnessCountsLock is a sidecar RecordHarnessMismatch locks instead of
// FileHarnessCounts itself, so the data file can be replaced by rename
// while the lock is held: a lock taken on the data file's own inode would
// be released from under itself the instant a rename swapped that inode out
// from under the open handle holding it, letting a second writer, which
// opens the post-rename path fresh, take the same "lock" concurrently. The
// sidecar's own content is never read; its only job is to exist and be
// lockable. It is never renamed, so this problem cannot recur on it.
const FileHarnessCountsLock = "harness_counts.lock"

// maxHarnessCountsBytes is this file's own hard cap (6.2.1: "at most 64
// KiB"). maxHarnessCountDays and maxHarnessCountSessions already keep it
// far under this in practice -- harness.ID is a five-value closed set and
// Family a three-value one, so at most 15 keys ever exist, each capped at
// 30 day-slots -- so this is a defensive ceiling, not a working limit:
// RecordHarnessMismatch refuses to write a document that would cross it
// rather than silently growing past it.
const maxHarnessCountsBytes = 64 << 10

// harnessCountsVersion is this file's "v" field.
const harnessCountsVersion = 1

// maxHarnessCountDays bounds each key's daily slots (6.2.1: "30 daily
// slots of seen and stood_down sessions").
const maxHarnessCountDays = 30

// maxHarnessCountSessions bounds the single, cross-key dedup list (6.2.1:
// "keyed digests of the last 256 sessions counted, so a session moves a
// counter once"). It is global, not per key: the spec holds one top-level
// "sessions" array, not one per counts entry, and 6.4's "one locked update
// ... on a session's first stand-down" only makes sense read that way --
// a session's SECOND stand-down, even against a different key, must cost
// nothing.
const maxHarnessCountSessions = 256

// harnessCountBudget is how long a write waits for this file's lock before
// giving up (6.2.1: "50 ms budget"; 6.4: "stand-down under 5ms beyond
// spawn ... one locked update (50ms budget)"). A dedicated constant rather
// than reusing spillBudget, which happens to share the value today for an
// unrelated reason (the spill file's own lock) -- the two are not the same
// promise, and a future change to one must not silently change the other.
const harnessCountBudget = 50 * time.Millisecond

// harnessSessionDigestDomain separates this file's keyed digests from every
// other HMAC this per-root key signs (HostDigest's "forget-host\x00",
// shape digests), so the same (key, sessionID) pair never collides with an
// unrelated digest computed under the same key for a different purpose.
const harnessSessionDigestDomain = "harness-session\x00"

// harnessDayCount is one day's seen/stood_down counts for one counter key.
type harnessDayCount struct {
	Seen      int `json:"seen"`
	StoodDown int `json:"stood_down"`
}

// harnessCountEntry is one "<installed_for>@<detected>" key's bookkeeping.
type harnessCountEntry struct {
	Days          map[string]harnessDayCount `json:"days"`
	LastStoodDown string                     `json:"last_stood_down,omitempty"`
	LastEvidence  []string                   `json:"last_evidence,omitempty"`
}

// harnessCounts is harness_counts.json's whole shape (6.2.1).
type harnessCounts struct {
	V        int                           `json:"v"`
	Counts   map[string]*harnessCountEntry `json:"counts"`
	Sessions []string                      `json:"sessions,omitempty"`
}

func newHarnessCounts() harnessCounts {
	return harnessCounts{V: harnessCountsVersion, Counts: map[string]*harnessCountEntry{}}
}

// readHarnessCounts reads the file at path, treating anything it cannot
// parse -- absent, corrupt, or simply never written -- as a fresh, empty
// document. This is deliberately as forgiving as internal/recap's
// recap.json read: a counter file that cannot be trusted must not block a
// hook from standing down, and re-establishing a clean one costs nothing a
// reader needs.
//
// A plain os.ReadFile is safe here, including under RecordHarnessMismatch's
// own lock, because that lock is taken on FileHarnessCountsLock, a sidecar,
// never on this file itself (see its own doc): on Windows, LockFileEx locks
// are mandatory and per-handle (lock_windows.go's own doc), so a read of
// THIS file while THIS file's own lock was held by any handle -- including
// one this same process opened -- used to fail with ERROR_LOCK_VIOLATION,
// which this function's error path turned into "nothing was ever written"
// and silently discarded every count and the session dedup list on every
// locked write, on Windows only.
func readHarnessCounts(path string) harnessCounts {
	b, err := os.ReadFile(path)
	if err != nil {
		return newHarnessCounts()
	}
	var hc harnessCounts
	if err := json.Unmarshal(b, &hc); err != nil {
		return newHarnessCounts()
	}
	if hc.Counts == nil {
		hc.Counts = map[string]*harnessCountEntry{}
	}
	return hc
}

// HarnessKey loads or creates the per-root HMAC key harness_counts.json
// digests session ids with -- the SAME key HostDigest signs with -- without
// creating install.json, Store.Open's other file. A hook entry that is
// about to stand down must never mint an install identity as a side effect
// of recording that it did; storeExistsAt and OpenExisting both decide "a
// store exists here" by statting install.json alone, never install.key, so
// creating only the key here is safe under that rule. A later, genuine
// Open on the same root loads this same key rather than minting a second
// one (loadOrCreateKey's own idempotence), so a session counted before
// install and one counted after digest identically.
func HarnessKey(root string) ([]byte, error) {
	if err := os.MkdirAll(root, dirMode); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, dirMode); err != nil {
		return nil, err
	}
	return loadOrCreateKey(filepath.Join(root, FileInstallKey))
}

// harnessSessionDigest is the keyed digest harness_counts.json's sessions
// list holds in place of a raw session id, for the reason HostDigest's own
// doc gives: two installs must not produce the same digest for the same
// session, so the file cannot be matched against a dictionary of ids built
// from a different install.
func harnessSessionDigest(key []byte, sessionID string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(harnessSessionDigestDomain))
	m.Write([]byte(sessionID))
	return hex.EncodeToString(m.Sum(nil))
}

// evictOldestDays drops days from entry until it holds at most
// maxHarnessCountDays, oldest date string first. Date strings are
// "YYYY-MM-DD", which sorts lexically in calendar order, so a plain string
// sort is a correct day-oldest-first order without parsing anything.
func evictOldestDays(entry *harnessCountEntry) {
	if len(entry.Days) <= maxHarnessCountDays {
		return
	}
	dates := make([]string, 0, len(entry.Days))
	for d := range entry.Days {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for _, d := range dates {
		if len(entry.Days) <= maxHarnessCountDays {
			return
		}
		delete(entry.Days, d)
	}
}

// evictOldestSessions drops digests from the front of sessions (the oldest,
// since every call appends to the end) until at most
// maxHarnessCountSessions-1 remain, leaving room for one more append.
func evictOldestSessions(sessions []string) []string {
	if len(sessions) < maxHarnessCountSessions {
		return sessions
	}
	drop := len(sessions) - maxHarnessCountSessions + 1
	return sessions[drop:]
}

// containsSession reports whether digest is already in sessions.
func containsSession(sessions []string, digest string) bool {
	for _, s := range sessions {
		if s == digest {
			return true
		}
	}
	return false
}

// errHarnessCountsTooLarge is returned, never silently swallowed, when a
// write would cross maxHarnessCountsBytes even after the day and session
// caps have both already been enforced -- a state this file's own bounds
// (15 keys at most, 30 days each, 256 sessions) make unreachable today, so
// this is a defensive refusal rather than a path any current input can
// reach.
var errHarnessCountsTooLarge = errors.New("store: harness_counts.json would exceed its 64 KiB cap")

// RecordHarnessMismatch bumps harness_counts.json's "<key>" counter for
// now's UTC date, at most once per session (6.2.1, 6.2.3): sessionID is
// turned into a keyed digest under this root's HarnessKey and checked
// against the last maxHarnessCountSessions counted GLOBALLY, across every
// key, not just this one -- a session that has already moved any counter
// this session changes nothing here, matching 6.4's "one locked update ...
// on a session's first stand-down".
//
// sessionID is "" when the caller's own family carries no field it can key
// a session by for this call (cmd/rashomon's harnessSessionKey's own doc:
// a Cursor-detected stand-down with no conversation_id, or recap's
// synthetic payload, which never holds a real field value at all). An
// empty sessionID is never added to the dedup list and never checked
// against it: harnessSessionDigest("") is one constant digest for every
// such call, and deduping against a constant made the FIRST session-less
// stand-down count and every later, genuinely different one read as
// already-counted -- an undercount with no ceiling, since nothing ever
// evicts a digest that keeps getting re-appended to the front of its own
// list. Every session-less call instead always counts: BRD-4's own bias
// ("fail toward recording, not toward silence") applied to the counter
// itself, not just the stand-down it is counting.
//
// counted reports whether this call actually changed the file: false for
// an already-counted session, for a held lock the call gave up on inside
// harnessCountBudget, or for the (currently unreachable) size refusal
// above. None of those is treated as an error a caller must act on: 6.2.1
// and 6.4 both describe this file as something a hook skips on contention,
// not something a stand-down fails over. A caller that wants to know
// whether anything actually happened reads counted, and a caller that only
// cares whether the stand-down itself succeeded can ignore both return
// values entirely -- RecordHarnessMismatch never has to run for "no
// record, no run directory, no output" to hold.
func RecordHarnessMismatch(root, key, sessionID string, evidence []string, now time.Time) (counted bool, err error) {
	return recordHarness(root, key, sessionID, evidence, now, true)
}

// RecordHarnessSeen is RecordHarnessMismatch for a detection that did not
// stand down (an Unknown harness, which this build records; policy pending
// Karthik's BRD-4 ruling): the
// same locked, deduplicated update, moving seen only -- stood_down and
// last_stood_down are untouched, so the counter still says how often each
// outcome happened.
func RecordHarnessSeen(root, key, sessionID string, now time.Time) (counted bool, err error) {
	return recordHarness(root, key, sessionID, nil, now, false)
}

// recordHarness is the body both share; standDown says whether the call
// also counts as a stand-down.
func recordHarness(root, key, sessionID string, evidence []string, now time.Time, standDown bool) (counted bool, err error) {
	hkey, err := HarnessKey(root)
	if err != nil {
		return false, err
	}
	dedup := sessionID != ""
	var digest string
	if dedup {
		digest = harnessSessionDigest(hkey, sessionID)
	}

	path := filepath.Join(root, FileHarnessCounts)

	// Lock-free bounded read, checked before the lock is ever touched (6.4:
	// "one read of harness_counts.json, and one locked update ... on a
	// session's FIRST stand-down" -- a session's second and later calls in
	// this process cost the read only). This is the common case: a hook,
	// post and probe invocation each call this once per stood-down session,
	// and every one after the first in that session is a dedup hit. Without
	// this, every one of them opened the sidecar and waited on its lock for
	// up to harnessCountBudget under contention before ever checking
	// whether there was anything left to do.
	//
	// A hit here is unconditionally correct whether or not a concurrent
	// writer is mid-update: readHarnessCounts reads whatever is on disk,
	// either the pre- or post-update document, and this session's digest is
	// already in BOTH if it is in either (the writer that put it there has
	// already finished, because a reader never observes a half-written file
	// -- writeHarnessCountsAtomic's rename is the reason that is true, not
	// an assumption this function makes on its own). A MISS proves nothing
	// either way (a concurrent writer could be about to add exactly this
	// digest) and falls through to the locked path below, which re-checks
	// under the lock before deciding to write.
	if dedup && containsSession(readHarnessCounts(path).Sessions, digest) {
		return false, nil
	}

	// The lock is taken on the sidecar, never on FileHarnessCounts itself:
	// see FileHarnessCountsLock's own doc for why the data file has to stay
	// free to be renamed out from under nothing while this holds it.
	lockPath := filepath.Join(root, FileHarnessCountsLock)
	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return false, err
	}
	defer lf.Close() //nolint:errcheck // nothing left to do with a close error once the lock below is released

	unlock, _, lockErr := lockCounted(lf, harnessCountBudget)
	if lockErr != nil {
		// Give up quietly: 6.2.1's own rule for exactly this case ("on a
		// timeout the count is skipped and retried later in the session").
		return false, nil
	}
	defer unlock()

	hc := readHarnessCounts(path)
	if dedup && containsSession(hc.Sessions, digest) {
		// The re-check the comment above promises: the lock-free read missed
		// because a concurrent writer had not yet landed this exact digest,
		// and now has.
		return false, nil
	}

	day := now.UTC().Format("2006-01-02")
	entry, ok := hc.Counts[key]
	if !ok {
		entry = &harnessCountEntry{Days: map[string]harnessDayCount{}}
		hc.Counts[key] = entry
	}
	c := entry.Days[day]
	c.Seen++
	if standDown {
		c.StoodDown++
		entry.LastStoodDown = day
		entry.LastEvidence = evidence
	}
	entry.Days[day] = c
	evictOldestDays(entry)

	if dedup {
		hc.Sessions = append(evictOldestSessions(hc.Sessions), digest)
	}

	encoded, merr := json.Marshal(hc)
	if merr != nil {
		return false, merr
	}
	if len(encoded) > maxHarnessCountsBytes {
		return false, errHarnessCountsTooLarge
	}

	if err := writeHarnessCountsAtomic(root, path, encoded); err != nil {
		return false, err
	}
	return true, nil
}

// writeHarnessCountsAtomic replaces path with encoded by temp file plus
// rename (Technical spec 6.2.1: "by atomic rename"), the same shape
// internal/settings.Write and internal/recap's writeState use for the same
// reason: a reader must never observe a half-written file, and a process
// killed between the old write's Truncate(0) and its Write left exactly
// that -- a file this package's own reader then read as "nothing was ever
// written" and replaced wholesale. The rename is safe to do while
// RecordHarnessMismatch's own lock is held because that lock is on the
// sidecar (FileHarnessCountsLock), never on this file's own inode.
func writeHarnessCountsAtomic(root, path string, encoded []byte) error {
	tmp, err := os.CreateTemp(root, "."+FileHarnessCounts+".rashomon-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck // a no-op after a successful rename; best effort after a failure

	if err := tmp.Chmod(fileMode); err != nil {
		tmp.Close() //nolint:errcheck // the chmod error is what matters
		return err
	}
	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close() //nolint:errcheck // the write error is what matters
		return err
	}
	// Through syncFull, not tmp.Sync() directly: every full sync the store
	// issues is required to (TestREC34h_EveryStoreLockAndSyncIsRecorded),
	// so the fault build's recorder sees this one too.
	if err := syncFull(tmp); err != nil {
		tmp.Close() //nolint:errcheck // the sync error is what matters
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// A read-only accessor for status/disclose's claude_entries_stood_down
// warning (BRD-31) is not added here: that display is a later PR's job to
// wire (see this change's own notes), and readHarnessCounts above already
// gives it the shape to read once it does.
