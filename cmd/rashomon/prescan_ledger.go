package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/replay"
)

// The import ledger and the per-session summaries (prescan_import.go says what
// the import is). Everything here is in memory: B3a persists it.

// flagSubagentResponseAssumed: a run of a subagent was fed on the assumption
// that its foreground Bash call was not moved to the background, which a
// subagent's result line (it carries no toolUseResult) cannot show. Such runs
// are not counted as matching recorded inputs.
const flagSubagentResponseAssumed = "subagent_tool_response_assumed"

// basisMatchesRecordedInputs is what a Verified run is.
const basisMatchesRecordedInputs = "matches_recorded_inputs"

// sessionBasis is what a session's summary can and cannot support. Its fields
// are constants of the replay today (a transcript cannot show a rewrite, a
// start or an end); they are fields, not comments, so that a consumer must read
// them and a later change that earns one changes it in one place.
type sessionBasis struct {
	// Reconstruction: how the counted runs were graded.
	Reconstruction string
	// ExecutedInputsVerified: the inputs the recorder saw are the inputs the
	// tools executed. False while rewrite_evidence_absent holds, which is for
	// every replayed session.
	ExecutedInputsVerified bool
	// CaptureComplete: the source shows that nothing the session did was
	// missed. False: the start is the replay's own and the end is unknown.
	CaptureComplete bool
	// SessionEnd: "unknown", the only value a transcript supports.
	SessionEnd string
}

// replayBasis is the basis of every replayed session today.
var replayBasis = sessionBasis{Reconstruction: basisMatchesRecordedInputs, SessionEnd: "unknown"}

type transitionCounts struct {
	GreenToRed, RedToGreen, GreenRedGreen int
}

type lastObserved struct {
	// State is the family's verify state at the session's end (GateState.State).
	State string
	// At is the time of the family's last run, any grade; Verified says whether
	// that run matches recorded inputs. SessionComplete: the session reads as
	// finished (replay.MergeSummary.Complete), so a red last state may be the
	// session's end; otherwise it says nothing about what the agent did next.
	At              time.Time
	Verified        bool
	SessionComplete bool
}

// familySummary is one session's contribution to one gate family (§6.3).
type familySummary struct {
	Project string
	Kind    string
	// SessionsVerified is this session's contribution to the family's count:
	// 1 if it has at least one run that matches recorded inputs, else 0.
	// SessionsAny is 1 for a session with a run of any grade (report only).
	SessionsVerified, SessionsAny int
	// RunsAny are the family's runs at any grade; RunsVerified those that match
	// recorded inputs, and Greens, Reds, Timeouts, NotRun and Other split them.
	RunsAny, RunsVerified int
	// RunsAssumed are runs fed on an assumption (replay.Event.Assumed): they
	// are in RunsAny and not RunsVerified.
	RunsAssumed                           int
	Greens, Reds, Timeouts, NotRun, Other int
	FirstSeen, LastSeen                   time.Time
	// Transitions are counted over the ordered runs that match recorded inputs,
	// a run of another kind or grade breaking the chain. A transition between
	// runs of two files is PartialTransitions, not a transition.
	Transitions        transitionCounts
	PartialTransitions int
	LastObserved       lastObserved
}

// sessionSummary is the in-memory input B3a turns into the seed.
type sessionSummary struct {
	// SessionHMAC is HMAC(key, sessionId), hex. The session id itself is not
	// held.
	SessionHMAC string
	// Files are the digests of the files that contributed, sorted.
	Files    []string
	Families map[string]*familySummary
	Basis    sessionBasis
	// Flags are the named ways the replayed record is not a live one
	// (replayFlags), plus any the session's own evidence adds.
	Flags []string
	// Complete: session_complete.
	Complete bool
	// Partial: the session's evidence is incomplete in a way that is not one
	// run's gap, with the reasons: a cap hit, copies that disagree, a subagent
	// without a type, an order not established, lines that could not be read.
	Partial        bool
	PartialReasons []string
	// Skipped counts the events the replay could not feed, by the reason of
	// each; Fed counts those it did. TaintFrom and TaintReason say where
	// evidence became incomplete, if it did.
	Skipped     map[string]int
	Fed         int
	TaintFrom   replay.Place
	TaintReason string
	Agents      []replay.AgentLink
}

// importLedger is the in-memory ledger B3a will persist.
type importLedger struct {
	key      []byte
	sessions map[string]*sessionSummary
}

func newImportLedger(key []byte) *importLedger {
	return &importLedger{key: append([]byte(nil), key...), sessions: map[string]*sessionSummary{}}
}

// ID is the ledger's name for a session: HMAC(key, sessionId), hex.
func (l *importLedger) ID(sessionID string) string {
	m := hmac.New(sha256.New, l.key)
	m.Write([]byte(sessionID))
	return hex.EncodeToString(m.Sum(nil))
}

// Put makes s the session's entry, over any entry it had: a rescan replaces a
// session's contribution and never adds to it. It reports whether it replaced
// one.
func (l *importLedger) Put(sessionID string, s *sessionSummary) (replaced bool) {
	id := l.ID(sessionID)
	_, replaced = l.sessions[id]
	s.SessionHMAC = id
	l.sessions[id] = s
	return replaced
}

// Remove drops a session's entry, if it has one: the session is no longer one
// the import counts.
func (l *importLedger) Remove(sessionID string) (had bool) {
	id := l.ID(sessionID)
	_, had = l.sessions[id]
	delete(l.sessions, id)
	return had
}

// Has reports whether the session is in the ledger.
func (l *importLedger) Has(sessionID string) bool {
	_, ok := l.sessions[l.ID(sessionID)]
	return ok
}

// Get is the session's entry, nil for none.
func (l *importLedger) Get(sessionID string) *sessionSummary { return l.sessions[l.ID(sessionID)] }

// Len is the number of sessions imported.
func (l *importLedger) Len() int { return len(l.sessions) }

// Unchanged reports whether the session is in the ledger with exactly these
// file digests: nothing to redo.
func (l *importLedger) Unchanged(sessionID string, files []string) bool {
	s := l.Get(sessionID)
	if s == nil || len(s.Files) != len(files) {
		return false
	}
	for i := range files {
		if s.Files[i] != files[i] {
			return false
		}
	}
	return true
}

// familyTotal is a family's imported figures, summed over the ledger's
// sessions.
type familyTotal struct {
	SessionsVerified, SessionsAny         int
	RunsAny, RunsVerified                 int
	Greens, Reds, Timeouts, NotRun, Other int
	FirstSeen, LastSeen                   time.Time
}

// Totals sums the ledger by family, from the entries as they stand. A family's
// SessionsVerified is the number of distinct sessions with a run that matches
// recorded inputs (DEDUP-1): one entry per session, and a session counted once
// whatever the scans, files, copies, resumes or subagent files behind it.
func (l *importLedger) Totals() map[string]*familyTotal {
	out := map[string]*familyTotal{}
	for _, s := range l.sessions {
		for fam, f := range s.Families {
			t := out[fam]
			if t == nil {
				t = &familyTotal{}
				out[fam] = t
			}
			t.SessionsVerified += f.SessionsVerified
			t.SessionsAny += f.SessionsAny
			t.RunsAny += f.RunsAny
			t.RunsVerified += f.RunsVerified
			t.Greens += f.Greens
			t.Reds += f.Reds
			t.Timeouts += f.Timeouts
			t.NotRun += f.NotRun
			t.Other += f.Other
			if !f.FirstSeen.IsZero() && (t.FirstSeen.IsZero() || f.FirstSeen.Before(t.FirstSeen)) {
				t.FirstSeen = f.FirstSeen
			}
			if f.LastSeen.After(t.LastSeen) {
				t.LastSeen = f.LastSeen
			}
		}
	}
	return out
}
