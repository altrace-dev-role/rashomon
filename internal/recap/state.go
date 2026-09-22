package recap

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// stateFileName is recap's own bookkeeping, a plain file under the store
// root beside install.json and baseline/ -- not a store record, because
// nothing here is evidence about a session; it is evidence about this
// COMMAND, kept for exactly one reader: status (H-92).
const stateFileName = "recap.json"

// maxTrackedSessions bounds the idempotency map. A machine that has run
// rashomon for a year across many projects has opened many sessions, and
// none of their ids are worth remembering once a claim has already done its
// one job (stopping a duplicate line on the SAME turn); an unbounded map
// would be this file's only way to grow forever.
const maxTrackedSessions = 64

// state is what recap.json holds. It is deliberately not store.Coverage
// shaped and carries no session content -- only ids it was told directly and
// the two facts H-92 asks status to be able to show.
type state struct {
	// LastEvaluatedAtUnixMS moves on EVERY run that reaches this file, found
	// something to say or not. This is the timestamp status reads: silence on
	// Stop is a notification policy (see package doc), and this field is what
	// lets status answer "was the last turn actually evaluated" instead of
	// letting a clean turn look identical to a recap that never ran at all.
	LastEvaluatedAtUnixMS int64 `json:"last_evaluated_at_unix_ms"`
	// Healthy is false when the run that last touched this file hit an
	// internal failure (RecordFailure) rather than completing normally. The
	// recap itself never prints on that path -- H-93 -- so this field is the
	// only place that failure is visible anywhere.
	Healthy bool `json:"healthy"`
	// Spoken maps a session id to the last prompt id a line was actually
	// PRINTED for in that session -- never merely evaluated. Keying on
	// "printed" rather than "evaluated" matters: a turn that found nothing
	// claims nothing here, so if the SAME prompt fires Stop again later
	// (a continuation that goes on to produce a real finding) it is still
	// free to speak once, which is what H-91 asks for and a blanket
	// "evaluated" claim would have broken.
	Spoken map[string]string `json:"spoken"`
}

func newState() state {
	return state{Spoken: map[string]string{}}
}

func statePath(root string) string {
	return filepath.Join(root, stateFileName)
}

// storeExists reports whether a store has ever been opened at root, using
// the same file storeInstalled() and RecordingState() check for the same
// reason: a plain stat, never store.Open, because asking recap's own
// question must not be what mints an install identity on a machine that has
// recorded nothing (H-87's rule, applied here to a different file).
func storeExists(root string) bool {
	_, err := os.Stat(filepath.Join(root, "install.json"))
	return err == nil
}

func readState(path string) (state, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return state{}, err
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return state{}, err
	}
	if s.Spoken == nil {
		s.Spoken = map[string]string{}
	}
	return s, nil
}

// writeState replaces the file atomically. temp+rename, the same shape
// internal/settings/write.go uses for the same reason: a reader (status, or
// a concurrent recap) must never observe a half-written file, and a process
// killed mid-write must leave either the old file or the new one, never a
// truncated third thing.
func writeState(path string, s state) error {
	if len(s.Spoken) > maxTrackedSessions {
		// Go's map iteration order is randomised, which is fine here: this
		// is a soft cap on a best-effort file, and which session ids it
		// drops first carries no meaning worth spending a real LRU on.
		for k := range s.Spoken {
			if len(s.Spoken) <= maxTrackedSessions {
				break
			}
			delete(s.Spoken, k)
		}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Claim is recap's one piece of persistent bookkeeping.
//
// It ALWAYS timestamps this evaluation when a store exists here -- see
// storeExists's doc for why nothing is written when one does not -- so
// status can move "last evaluated" on every run, whether or not this run
// found anything. It reports the final speak decision: wantSpeak, unless
// this exact (sessionID, promptID) pair already produced a line, in which
// case speak comes back false regardless of what the caller asked for --
// H-101's guard against a Stop a continuation replays for the same turn.
//
// A read or write failure never changes the speak decision it already made
// in memory: recap.json is bookkeeping, not evidence, and losing it must
// cost at most a future duplicate or a stale health timestamp, never a
// missing line and never a hook error. The caller is expected to ignore the
// returned error for exactly that reason and is told so at the call site.
func Claim(root, sessionID, promptID string, now time.Time, wantSpeak bool) (speak bool, err error) {
	if !storeExists(root) {
		return wantSpeak, nil
	}
	path := statePath(root)
	s, rerr := readState(path)
	if rerr != nil {
		// A corrupt bookkeeping file must not block a real finding, and must
		// not stop this run from re-establishing a clean one either.
		s = newState()
	}

	speak = wantSpeak
	if wantSpeak && sessionID != "" && promptID != "" && s.Spoken[sessionID] == promptID {
		speak = false
	}
	s.LastEvaluatedAtUnixMS = now.UnixMilli()
	s.Healthy = true
	if speak && sessionID != "" && promptID != "" {
		s.Spoken[sessionID] = promptID
	}
	return speak, writeState(path, s)
}

// RecordFailure marks this evaluation as unhealthy, for the one path where
// cmdRecap gave up before it could even build a decision: a digest read that
// itself errored. It is best-effort in the same sense Claim is -- see its
// doc -- and is a no-op where no store exists yet, for the same reason.
func RecordFailure(root string, now time.Time) {
	if !storeExists(root) {
		return
	}
	path := statePath(root)
	s, err := readState(path)
	if err != nil {
		s = newState()
	}
	s.LastEvaluatedAtUnixMS = now.UnixMilli()
	s.Healthy = false
	_ = writeState(path, s)
}

// Status reports recap's own evaluation history for `status` to render
// (H-92): whether a Stop has ever been evaluated here, when the last one
// was, and whether it completed cleanly. Silence on Stop is a notification
// policy and never a claim that the turn was clean -- see the package doc --
// so this is the fact silence itself cannot carry, read the same read-only
// way RecordingState reads pause's marker: a plain stat and ReadFile,
// nothing that opens or creates anything.
func Status(root string) (evaluated bool, lastAt time.Time, healthy bool, err error) {
	b, err := os.ReadFile(statePath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return false, time.Time{}, false, nil
	}
	if err != nil {
		return false, time.Time{}, false, err
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return false, time.Time{}, false, err
	}
	if s.LastEvaluatedAtUnixMS == 0 {
		return false, time.Time{}, false, nil
	}
	return true, time.UnixMilli(s.LastEvaluatedAtUnixMS), s.Healthy, nil
}
