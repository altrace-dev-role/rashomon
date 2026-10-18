package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// fileHarnessCounts is H1's hook-written mismatch counter (Technical spec
// 6.2.1, 6.2.3), held at the store root beside install.json and
// harnesses.json. Unlike those two -- harnesses.json is command-written
// only (watch, detach, steer, pause) and install.json names an install's
// identity -- this file is written only by a hook entry that has just
// decided to stand down, and RecordHarnessMismatch is the only writer.
const fileHarnessCounts = "harness_counts.json"

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

// readHarnessCounts reads the file at root, treating anything it cannot
// parse -- absent, corrupt, or simply never written -- as a fresh, empty
// document. This is deliberately as forgiving as internal/recap's
// recap.json read: a counter file that cannot be trusted must not block a
// hook from standing down, and re-establishing a clean one costs nothing a
// reader needs.
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
	return loadOrCreateKey(filepath.Join(root, fileInstallKey))
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
	hkey, err := HarnessKey(root)
	if err != nil {
		return false, err
	}
	digest := harnessSessionDigest(hkey, sessionID)

	path := filepath.Join(root, fileHarnessCounts)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return false, err
	}
	defer f.Close() //nolint:errcheck // nothing left to do with a close error after a completed write or a skip

	unlock, _, lockErr := lockCounted(f, harnessCountBudget)
	if lockErr != nil {
		// Give up quietly: 6.2.1's own rule for exactly this case ("on a
		// timeout the count is skipped and retried later in the session").
		return false, nil
	}
	defer unlock()

	hc := readHarnessCounts(path)
	if containsSession(hc.Sessions, digest) {
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
	c.StoodDown++
	entry.Days[day] = c
	evictOldestDays(entry)
	entry.LastStoodDown = day
	entry.LastEvidence = evidence

	hc.Sessions = append(evictOldestSessions(hc.Sessions), digest)

	encoded, merr := json.Marshal(hc)
	if merr != nil {
		return false, merr
	}
	if len(encoded) > maxHarnessCountsBytes {
		return false, errHarnessCountsTooLarge
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	if err := f.Truncate(0); err != nil {
		return false, err
	}
	if _, err := f.Write(encoded); err != nil {
		return false, err
	}
	return true, nil
}

// A read-only accessor for status/disclose's claude_entries_stood_down
// warning (BRD-31) is not added here: that display is a later PR's job to
// wire (see this change's own notes), and readHarnessCounts above already
// gives it the shape to read once it does.
