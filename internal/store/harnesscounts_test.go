//go:build unix || windows

package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func readHarnessCountsFile(t *testing.T, root string) harnessCounts {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, FileHarnessCounts))
	if err != nil {
		t.Fatalf("reading %s: %v", FileHarnessCounts, err)
	}
	var hc harnessCounts
	if err := json.Unmarshal(b, &hc); err != nil {
		t.Fatalf("unmarshal %s: %v", FileHarnessCounts, err)
	}
	return hc
}

// TestRecordHarnessMismatch_FirstCallCreatesFile pins the file's shape
// against the spec's own example (6.2.1): version 1, a day slot under
// today's date, last_stood_down and last_evidence set, one keyed (not raw)
// session digest, 0600 permissions -- and that NO install identity was
// minted as a side effect (6.2.3's "no record" extends to never fabricating
// one just to count a mismatch that found none).
func TestRecordHarnessMismatch_FirstCallCreatesFile(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", []string{"cursor_version"}, now)
	if err != nil {
		t.Fatalf("RecordHarnessMismatch: %v", err)
	}
	if !counted {
		t.Fatal("counted = false on a fresh file's first mismatch")
	}

	if _, err := os.Stat(filepath.Join(root, FileInstallMeta)); err == nil {
		t.Fatal("install.json exists: a stand-down must never mint an install identity")
	}

	info, err := os.Stat(filepath.Join(root, FileHarnessCounts))
	if err != nil {
		t.Fatalf("stat %s: %v", FileHarnessCounts, err)
	}
	if perm := info.Mode().Perm(); perm != fileMode {
		t.Errorf("mode = %o, want %o", perm, fileMode)
	}

	hc := readHarnessCountsFile(t, root)
	if hc.V != harnessCountsVersion {
		t.Errorf("v = %d, want %d", hc.V, harnessCountsVersion)
	}
	entry, ok := hc.Counts["claude@cursor"]
	if !ok {
		t.Fatal(`counts["claude@cursor"] missing`)
	}
	day, ok := entry.Days["2026-10-21"]
	if !ok || day.Seen != 1 || day.StoodDown != 1 {
		t.Errorf("days[2026-10-21] = %+v, ok=%v, want seen=1 stood_down=1", day, ok)
	}
	if entry.LastStoodDown != "2026-10-21" {
		t.Errorf("last_stood_down = %q, want 2026-10-21", entry.LastStoodDown)
	}
	if len(entry.LastEvidence) != 1 || entry.LastEvidence[0] != "cursor_version" {
		t.Errorf("last_evidence = %v, want [cursor_version]", entry.LastEvidence)
	}
	if len(hc.Sessions) != 1 {
		t.Fatalf("sessions = %v, want exactly one digest", hc.Sessions)
	}
	if hc.Sessions[0] == "sess-1" {
		t.Error("sessions holds the raw session id; it must hold a keyed digest")
	}
}

// TestRecordHarnessMismatch_SameSessionSameKeyDeduped: a session's first
// stand-down moves the counter; a second stand-down for the SAME session,
// even against the same key, must not (6.2.1: "a session moves a counter
// once"; 6.4: "one locked update ... on a session's first stand-down").
func TestRecordHarnessMismatch_SameSessionSameKeyDeduped(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	if counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, now); err != nil || !counted {
		t.Fatalf("first call: counted=%v err=%v", counted, err)
	}
	counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if counted {
		t.Fatal("counted = true on a session's second stand-down")
	}

	hc := readHarnessCountsFile(t, root)
	day := hc.Counts["claude@cursor"].Days["2026-10-21"]
	if day.Seen != 1 || day.StoodDown != 1 {
		t.Errorf("days[2026-10-21] = %+v, want unchanged at seen=1 stood_down=1", day)
	}
}

// TestRecordHarnessMismatch_SameSessionDifferentKeyDeduped: the session
// dedup list is global, not per key (6.2.1 holds exactly one "sessions"
// array, not one per counts entry) -- a session that already moved
// "claude@cursor" must not also move "claude@codex".
func TestRecordHarnessMismatch_SameSessionDifferentKeyDeduped(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	if counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, now); err != nil || !counted {
		t.Fatalf("first call: counted=%v err=%v", counted, err)
	}
	counted, err := RecordHarnessMismatch(root, "claude@codex", "sess-1", nil, now)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if counted {
		t.Fatal("counted = true for a different key under the same already-counted session")
	}

	hc := readHarnessCountsFile(t, root)
	if _, ok := hc.Counts["claude@codex"]; ok {
		t.Error(`counts["claude@codex"] exists; the dedup should have skipped the write entirely`)
	}
}

// TestRecordHarnessMismatch_DifferentSessionsIncrement: distinct sessions
// against the same key on the same day both count.
func TestRecordHarnessMismatch_DifferentSessionsIncrement(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	for i, sess := range []string{"sess-1", "sess-2"} {
		counted, err := RecordHarnessMismatch(root, "claude@unknown", sess, nil, now)
		if err != nil || !counted {
			t.Fatalf("call %d: counted=%v err=%v", i, counted, err)
		}
	}
	hc := readHarnessCountsFile(t, root)
	day := hc.Counts["claude@unknown"].Days["2026-10-21"]
	if day.Seen != 2 || day.StoodDown != 2 {
		t.Errorf("days[2026-10-21] = %+v, want seen=2 stood_down=2", day)
	}
	if len(hc.Sessions) != 2 {
		t.Errorf("sessions = %v, want 2 entries", hc.Sessions)
	}
}

// TestRecordHarnessMismatch_EmptySessionIDNeverDedupedAgainstItself: a
// caller with no field to key a session by (cmd/rashomon's
// harnessSessionKey, e.g. a Cursor-detected stand-down with no
// conversation_id) passes sessionID == "". Three such calls must all
// count: digesting "" and deduping against it, the bug this pins, made
// the SECOND and every later one read as already-counted, because every
// empty-sessionID call digests to the same constant and the first call's
// digest never ages out (it is re-appended to the list's front on every
// subsequent empty call too, under the old code), so the counter could
// never move again. Session == "" must therefore never enter the dedup
// list at all, not just avoid colliding with a real session's digest.
func TestRecordHarnessMismatch_EmptySessionIDNeverDedupedAgainstItself(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		counted, err := RecordHarnessMismatch(root, "claude@cursor", "", nil, now)
		if err != nil || !counted {
			t.Fatalf("call %d: counted=%v err=%v, want true, <nil>", i, counted, err)
		}
	}
	hc := readHarnessCountsFile(t, root)
	day := hc.Counts["claude@cursor"].Days["2026-10-21"]
	if day.StoodDown != 3 {
		t.Errorf("stood_down = %d, want 3 (every session-less call counts)", day.StoodDown)
	}
	for _, s := range hc.Sessions {
		if s == "" {
			t.Fatalf("sessions = %v, want no entry for an empty sessionID", hc.Sessions)
		}
	}
	// A real session recorded after three session-less calls must dedupe
	// normally: the fix must not have disabled dedup altogether.
	if counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-real", nil, now); err != nil || !counted {
		t.Fatalf("real session: counted=%v err=%v, want true, <nil>", counted, err)
	}
	if counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-real", nil, now); err != nil || counted {
		t.Fatalf("real session repeated: counted=%v err=%v, want false, <nil>", counted, err)
	}
}

// TestRecordHarnessMismatch_SessionsCapped256: the global session dedup
// list holds at most the last 256 counted sessions (6.2.1); the oldest is
// evicted first (FIFO), and the dedup property keeps holding for whichever
// sessions remain.
func TestRecordHarnessMismatch_SessionsCapped256(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	for i := 0; i < maxHarnessCountSessions+1; i++ {
		sess := "sess-" + strconv.Itoa(i)
		if _, err := RecordHarnessMismatch(root, "claude@unknown", sess, nil, now); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	hc := readHarnessCountsFile(t, root)
	if len(hc.Sessions) != maxHarnessCountSessions {
		t.Fatalf("sessions length = %d, want %d", len(hc.Sessions), maxHarnessCountSessions)
	}
	// The first session (sess-0) should have aged out, so counting it again
	// must be accepted as new, not deduped.
	counted, err := RecordHarnessMismatch(root, "claude@unknown", "sess-0", nil, now)
	if err != nil {
		t.Fatalf("recount sess-0: %v", err)
	}
	if !counted {
		t.Fatal("sess-0 was not evicted from the dedup list after the cap was exceeded")
	}
}

// TestRecordHarnessMismatch_DaysCapped30: a key's days map holds at most 30
// slots (6.2.1); the oldest is evicted when a new day is added past the cap.
func TestRecordHarnessMismatch_DaysCapped30(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < maxHarnessCountDays+1; i++ {
		day := base.AddDate(0, 0, i)
		sess := "sess-" + strconv.Itoa(i)
		if _, err := RecordHarnessMismatch(root, "claude@unknown", sess, nil, day); err != nil {
			t.Fatalf("day %d: %v", i, err)
		}
	}
	hc := readHarnessCountsFile(t, root)
	entry := hc.Counts["claude@unknown"]
	if len(entry.Days) != maxHarnessCountDays {
		t.Fatalf("days = %d entries, want %d", len(entry.Days), maxHarnessCountDays)
	}
	if _, ok := entry.Days[base.Format("2006-01-02")]; ok {
		t.Error("the oldest day slot was not evicted")
	}
	lastDay := base.AddDate(0, 0, maxHarnessCountDays).Format("2006-01-02")
	if _, ok := entry.Days[lastDay]; !ok {
		t.Errorf("the newest day slot (%s) is missing", lastDay)
	}
}

// TestRecordHarnessMismatch_LockTimeoutSkipsWithoutError: a held lock makes
// the write give up inside its budget, reporting counted=false and err=nil
// (6.2.1: "on a timeout the count is skipped and retried later in the
// session") -- never an error a caller would have to decide whether to
// surface, and never a wait anywhere near the full hold.
func TestRecordHarnessMismatch_LockTimeoutSkipsWithoutError(t *testing.T) {
	root := t.TempDir()
	// The sidecar, not FileHarnessCounts itself: RecordHarnessMismatch locks
	// FileHarnessCountsLock (see its own doc), so that is the file a test
	// simulating contention has to hold.
	lockPath := filepath.Join(root, FileHarnessCountsLock)
	if err := os.MkdirAll(root, dirMode); err != nil {
		t.Fatal(err)
	}
	holdFor(t, lockPath, 300*time.Millisecond)

	start := time.Now()
	counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, time.Now())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RecordHarnessMismatch under a held lock returned an error: %v", err)
	}
	if counted {
		t.Fatal("counted = true while the file's lock was held by another writer")
	}
	if elapsed >= 250*time.Millisecond {
		t.Errorf("elapsed = %v, want bounded near the lock budget, not the full 300ms hold", elapsed)
	}
}

// TestRecordHarnessMismatch_AlreadyCountedSessionNeverLocks: a session's
// SECOND stand-down in this process never touches the sidecar lock at all
// (6.4: "one read ... and one locked update ... on a session's first
// stand-down" -- everything after the first costs a read, not a lock).
// Proof: the sidecar lock is held externally for the whole test, well past
// harnessCountBudget; a call that still tried to acquire it would block for
// up to harnessCountBudget before giving up (as
// TestRecordHarnessMismatch_LockTimeoutSkipsWithoutError measures directly),
// while a call that skips the lock on an already-counted session returns in
// microseconds. The cutoff is harnessCountBudget/2: below it only the
// lock-free path lands, above it only the lock-and-give-up path does, so
// this is not a tuned guess -- it is the midpoint between the two actual
// code paths this test tells apart.
func TestRecordHarnessMismatch_AlreadyCountedSessionNeverLocks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root, dirMode); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	// Establish sess-1 as already counted, lock released before the hold
	// below starts: this call must succeed normally.
	counted, err := RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, now)
	if err != nil || !counted {
		t.Fatalf("first call: counted=%v err=%v, want true, <nil>", counted, err)
	}

	lockPath := filepath.Join(root, FileHarnessCountsLock)
	holdFor(t, lockPath, 10*harnessCountBudget)

	start := time.Now()
	counted, err = RecordHarnessMismatch(root, "claude@cursor", "sess-1", nil, now)
	elapsed := time.Since(start)
	if err != nil || counted {
		t.Fatalf("already-counted session under a held lock: counted=%v err=%v, want false, <nil>", counted, err)
	}
	if elapsed >= harnessCountBudget/2 {
		t.Errorf("elapsed = %v, want well under %v (an already-counted session must never wait on the lock)", elapsed, harnessCountBudget/2)
	}
}

// TestRecordHarnessMismatch_Concurrent exercises the actual lock under
// -race: N goroutines, N distinct sessions, same key, same day.
//
// It does NOT assert that every one of the N calls counts. 6.2.1 and 6.4
// both say a write that cannot get the file's lock inside harnessCountBudget
// is skipped, not retried in place -- "on a timeout the count is skipped
// and retried later in the session" -- and N genuinely simultaneous
// distinct-session mismatches against the same file is already a heavier
// load than the design budgets for (6.4's "one locked update ... on a
// session's FIRST stand-down" describes one write per session, not N
// racing at once). Measured on this machine, N=16 reliably pushes some
// callers past the 50ms budget; asserting 16-for-16 here would be pinning
// a liveness property the spec explicitly trades away, not the safety
// property this test exists to pin.
//
// What must hold regardless of how many calls actually counted: the file
// stays internally consistent -- seen equals stood_down (every write this
// function makes bumps both together), seen never exceeds the number of
// calls that reported counted=true, and it never falls below it (no lost
// update: a counted=true call's effect always lands), and the session
// digest list holds exactly one entry per counted call, with no duplicate
// digest -- the thing a "lost update" would actually corrupt.
func TestRecordHarnessMismatch_Concurrent(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	const n = 16

	type result struct {
		counted bool
		err     error
	}
	results := make(chan result, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			counted, err := RecordHarnessMismatch(root, "claude@unknown", fmt.Sprintf("sess-%d", i), nil, now)
			results <- result{counted, err}
		}()
	}
	nCounted := 0
	for i := 0; i < n; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("goroutine error: %v", r.err)
		}
		if r.counted {
			nCounted++
		}
	}
	if nCounted == 0 {
		t.Fatal("nCounted = 0: every single call was skipped, which the lock budget should not do to a 16-way burst on an idle disk")
	}

	hc := readHarnessCountsFile(t, root)
	day := hc.Counts["claude@unknown"].Days["2026-10-21"]
	if day.Seen != nCounted || day.StoodDown != nCounted {
		t.Errorf("days[2026-10-21] = %+v, want seen=%d stood_down=%d (no lost update against nCounted=%d)", day, nCounted, nCounted, nCounted)
	}
	if len(hc.Sessions) != nCounted {
		t.Errorf("sessions = %d entries, want %d (one per counted call, no duplicates)", len(hc.Sessions), nCounted)
	}
	seen := map[string]bool{}
	for _, s := range hc.Sessions {
		if seen[s] {
			t.Errorf("duplicate session digest %q in sessions", s)
		}
		seen[s] = true
	}
}

// TestHarnessKey_NeverCreatesInstallMeta: the keyed digest's key must be
// mintable without ever creating install.json -- the file storeExistsAt and
// OpenExisting both treat as "a store exists here".
func TestHarnessKey_NeverCreatesInstallMeta(t *testing.T) {
	root := t.TempDir()
	key, err := HarnessKey(root)
	if err != nil {
		t.Fatalf("HarnessKey: %v", err)
	}
	if len(key) != keyLen {
		t.Errorf("key length = %d, want %d", len(key), keyLen)
	}
	if _, err := os.Stat(filepath.Join(root, FileInstallMeta)); err == nil {
		t.Fatal("install.json exists after HarnessKey alone")
	}
	if _, err := os.Stat(filepath.Join(root, FileInstallKey)); err != nil {
		t.Fatalf("install.key does not exist after HarnessKey: %v", err)
	}
}

// TestHarnessKey_IdempotentAndSharedWithOpen: HarnessKey called twice
// returns the same bytes, and a LATER real Open (a genuine install,
// happening afterwards on the same root) picks up the SAME key rather than
// minting a second one -- so a session counted before install is digested
// under the same key a session counted after install is.
func TestHarnessKey_IdempotentAndSharedWithOpen(t *testing.T) {
	root := t.TempDir()
	first, err := HarnessKey(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HarnessKey(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("HarnessKey is not idempotent")
	}
	st, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(st.Key()) != string(first) {
		t.Fatal("Open minted a different key than HarnessKey had already created")
	}
}

// TestRecordHarnessMismatch_FileNeverExceedsCap is a defensive assertion,
// not a scenario expected in practice: Days and Sessions are both bounded
// (30, 256) well under the 64 KiB ceiling on their own, so this just pins
// that the write path enforces the ceiling rather than trusting the other
// two caps to always be enough.
func TestRecordHarnessMismatch_FileNeverExceedsCap(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 50; i++ {
		if _, err := RecordHarnessMismatch(root, "claude@unknown", "sess-"+strconv.Itoa(i), []string{"cursor_version", "conversation_id+generation_id"}, now); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, FileHarnessCounts))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > maxHarnessCountsBytes {
		t.Errorf("file size = %d bytes, want at most %d", len(b), maxHarnessCountsBytes)
	}
}

// TestHarnessSeenCount: the read half of RecordHarnessSeen sums seen over the
// key's day slots, leaves other keys out, reads a file that was never written
// as a real zero, and reads one it cannot parse as an error, never zero.
func TestHarnessSeenCount(t *testing.T) {
	root := t.TempDir()
	if n, err := HarnessSeenCount(root, "claude@unknown"); n != 0 || err != nil {
		t.Fatalf("no file: (%d, %v), want (0, nil)", n, err)
	}
	day1 := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for i, at := range []time.Time{day1, day1, day1.Add(48 * time.Hour)} {
		if _, err := RecordHarnessSeen(root, "claude@unknown", "sess-"+strconv.Itoa(i), at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RecordHarnessSeen(root, "claude@unknown", "sess-1", day1); err != nil { // deduped
		t.Fatal(err)
	}
	if _, err := RecordHarnessMismatch(root, "claude@cursor", "sess-c", []string{"cursor_version"}, day1); err != nil {
		t.Fatal(err)
	}
	if n, err := HarnessSeenCount(root, "claude@unknown"); n != 3 || err != nil {
		t.Errorf("claude@unknown: (%d, %v), want (3, nil)", n, err)
	}
	if n, err := HarnessSeenCount(root, "claude@codex"); n != 0 || err != nil {
		t.Errorf("an absent key: (%d, %v), want (0, nil)", n, err)
	}
	if err := os.WriteFile(filepath.Join(root, FileHarnessCounts), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := HarnessSeenCount(root, "claude@unknown"); err == nil {
		t.Errorf("a corrupt file read as (%d, nil), want an error", n)
	}
}
