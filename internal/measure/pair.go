// Package measure reads two sessions on one task side by side, from the
// record and from Claude Code's transcripts (card MEASURE1). It reads only:
// no network, no subprocess, no write (MSR-2). The driver that started the
// sessions and ran the oracle lives outside the binary.
package measure

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Schema is the pair's JSON schema version.
const Schema = 1

// ArmUnknown is a side's recorded arm when its session wrote no steer line:
// the record cannot say which arm it ran under, and it is never read as live.
const ArmUnknown = "unknown"

// The caveats, a closed list, in the order a pair lists them.
const (
	CaveatNIsOne              = "n_is_one"
	CaveatArmUnrecorded       = "arm_unrecorded"
	CaveatTranscriptMissing   = "transcript_missing"
	CaveatTranscriptPartial   = "transcript_partial"
	CaveatRecordingIncomplete = "recording_incomplete"
	CaveatDifferentModels     = "different_models"
)

// Caveats returns the closed list in its order.
func Caveats() []string {
	return []string{CaveatNIsOne, CaveatArmUnrecorded, CaveatTranscriptMissing, CaveatTranscriptPartial,
		CaveatRecordingIncomplete, CaveatDifferentModels}
}

// Ask names one side: a session, the arm the driver forced on it, and where
// it ran: the store's root and Claude Code's configuration directory. Each
// arm of a paired run starts from its own copy of both.
type Ask struct {
	SessionID string
	Arm       steer.Arm
	Root      string
	ConfigDir string
}

// Pair is two sides and what the reader of them must know.
type Pair struct {
	Schema      int   `json:"schema"`
	GeneratedAt int64 `json:"generated_at_unix_ms"`
	// RulesVersion names the classifier behind the writes and test runs
	// rows: the shape rules the recording binary classified each call by.
	RulesVersion int      `json:"rules_version"`
	Sides        [2]Side  `json:"sides"`
	Caveats      []string `json:"caveats"`
}

// Side is one session's rows. Every count is of records, steer lines or
// transcript lines; nothing is estimated.
type Side struct {
	SessionID string `json:"session_id"`
	// Arm is the arm asked; RecordedArm the one every steer line names, or
	// ArmUnknown with no line.
	Arm         string `json:"arm"`
	RecordedArm string `json:"recorded_arm"`
	// Lines counts the session's steer lines. NotesGiven counts those
	// delivered to the agent, NotesWithheld the notes arm off decided as
	// the steer arm would and withheld, and NotesNotGiven every line not
	// given, by why. Every line is counted in exactly one of the two, so
	// given plus not given is Lines, an identity a reader of the JSON can
	// check.
	Lines         int       `json:"lines"`
	NotesGiven    int       `json:"notes_given"`
	NotesWithheld int       `json:"notes_withheld"`
	NotesNotGiven []Reason  `json:"notes_not_given"`
	Findings      []Finding `json:"findings"`
	// Calls counts declarations, Executed those with an execution record (a
	// call the permission flow denied is declared and never executed),
	// Writes those shape classes write or marks may write, and TestRuns the
	// test-class calls by how they ended.
	Calls    int      `json:"calls"`
	Executed int      `json:"executed"`
	Writes   int      `json:"writes"`
	TestRuns TestRuns `json:"test_runs"`
	// Gates are the last Stop checkpoint's, none when the session has none.
	Gates []Gate `json:"gates"`
	// WallMS is first to last record stamp.
	WallMS     int64       `json:"wall_ms"`
	Transcript *Transcript `json:"transcript"`

	firstMS int64
}

// Reason is a count of lines not given to the agent for one reason: the
// line's suppressed_by, or flag_only (shown to the user only) or holdout.
type Reason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// Finding is one finding a session's lines raised, by pattern, kind and key,
// however many calls decided it. ResolvedAtSeq is the gate's last green at
// the last Stop when it is after the finding's last line, as stored on the
// checkpoint; nil when no gate the checkpoint holds is the finding's key.
type Finding struct {
	Pattern       string `json:"pattern"`
	Key           string `json:"key"`
	Lines         int    `json:"lines"`
	LastAfterSeq  int64  `json:"last_after_seq"`
	ResolvedAtSeq *int64 `json:"resolved_at_seq"`
}

// TestRuns counts test-class calls: failed and passed by their execution's
// outcome, unended with none or another.
type TestRuns struct {
	Failed  int `json:"failed"`
	Passed  int `json:"passed"`
	Unended int `json:"unended"`
}

// Gate is one gate at the last Stop.
type Gate struct {
	Gate  string `json:"gate"`
	State string `json:"state"`
}

// findingPatterns are the patterns a session report counts as findings; the
// rest (briefings, facts, lessons) are notes.
var findingPatterns = []pattern.ID{pattern.RegressionLocation, pattern.SubtaskGate, pattern.ClaimVsRecord,
	pattern.MaskedStatus, pattern.OrderingRule, pattern.IdenticalRetry, pattern.StaleEvidence}

// PairOf reads each side from its store and the transcripts under its
// configuration directory, opening neither for writing. It refuses a side
// whose arm is not off or steer, one session asked twice, a store or session
// with no records, and a session whose lines name more than one arm or
// another arm than the one asked: each error names the session.
func PairOf(a, b Ask, now time.Time) (*Pair, error) {
	if a.SessionID == b.SessionID {
		return nil, fmt.Errorf("session %s is asked for twice; a pair is two sessions", a.SessionID)
	}
	p := &Pair{Schema: Schema, GeneratedAt: now.UnixMilli(), RulesVersion: shape.RulesVersion, Caveats: []string{CaveatNIsOne}}
	var caveats [2][]string
	var tr [2]*Transcript
	for i, ask := range []Ask{a, b} {
		st, err := store.OpenExisting(ask.Root)
		if err != nil {
			return nil, fmt.Errorf("session %s: the store at %s: %w", ask.SessionID, ask.Root, err)
		}
		if !store.Lab(ask.Root) {
			return nil, fmt.Errorf("session %s: the store at %s is not a lab (no `rashomon measure --prepare`), so its RASHOMON_STEER_ARM was ignored", ask.SessionID, ask.Root)
		}
		side, cv, err := sideOf(st, ask)
		if err != nil {
			return nil, err
		}
		if tr[i], err = transcriptOfSession(ask.ConfigDir, ask.SessionID, side.firstMS); err != nil {
			return nil, fmt.Errorf("session %s: the transcripts under %s: %w", ask.SessionID, ask.ConfigDir, err)
		}
		p.Sides[i], caveats[i] = side, cv
	}
	for i := range p.Sides {
		p.Sides[i].Transcript = tr[i]
		if tr[i] == nil {
			caveats[i] = append(caveats[i], CaveatTranscriptMissing)
		} else if tr[i].UnreadLines > 0 {
			caveats[i] = append(caveats[i], CaveatTranscriptPartial)
		}
	}
	if tr[0] != nil && tr[1] != nil && !slices.Equal(tr[0].Models, tr[1].Models) {
		caveats[0] = append(caveats[0], CaveatDifferentModels)
	}
	for _, c := range Caveats()[1:] {
		if slices.Contains(caveats[0], c) || slices.Contains(caveats[1], c) {
			p.Caveats = append(p.Caveats, c)
		}
	}
	return p, nil
}

// sideOf reads one side's record: its run, its steer log and its last
// checkpoint. The caveats are the side's own.
func sideOf(st *store.Store, ask Ask) (Side, []string, error) {
	if ask.Arm != steer.ArmOff && ask.Arm != steer.ArmSteer {
		return Side{}, nil, fmt.Errorf("session %s: arm %q is not a forced arm (off or steer)", ask.SessionID, ask.Arm)
	}
	run, err := st.ReadRun(ask.SessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Side{}, nil, fmt.Errorf("session %s: %w", ask.SessionID, err)
	}
	if run == nil || len(run.Declarations)+len(run.Executions)+len(run.Terminals) == 0 {
		return Side{}, nil, fmt.Errorf("session %s has no records in %s", ask.SessionID, st.Root())
	}
	s := Side{SessionID: ask.SessionID, Arm: string(ask.Arm), NotesNotGiven: []Reason{}, Findings: []Finding{}, Gates: []Gate{}}
	var caveats []string
	s.calls(run)
	raw, skipped, err := st.ReadSteer(ask.SessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Side{}, nil, fmt.Errorf("session %s: the steer log: %w", ask.SessionID, err)
	}
	if skipped > 0 || incomplete(st, run, ask.SessionID) {
		caveats = append(caveats, CaveatRecordingIncomplete)
	}
	lines, err := decodeLines(raw)
	if err != nil {
		return Side{}, nil, fmt.Errorf("session %s: the steer log: %w", ask.SessionID, err)
	}
	if err := s.arm(lines, ask); err != nil {
		return Side{}, nil, err
	}
	if s.RecordedArm == ArmUnknown {
		caveats = append(caveats, CaveatArmUnrecorded)
	}
	if err := s.notes(lines); err != nil {
		return Side{}, nil, err
	}
	gates, err := lastGates(st, ask.SessionID)
	if err != nil {
		return Side{}, nil, fmt.Errorf("session %s: the last checkpoint: %w", ask.SessionID, err)
	}
	s.findings(lines, gates)
	for _, g := range gates {
		s.Gates = append(s.Gates, Gate{Gate: g.Gate, State: string(g.State)})
	}
	return s, caveats, nil
}
