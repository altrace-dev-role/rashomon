// Package report renders what the store holds for a session, and whether that
// can be trusted.
//
// It reads the coverage records the run wrote about itself. It never reads
// today's configuration to judge a past run: if it did, detach would
// retroactively invalidate every run ever captured, and the one action users
// are told is safe would destroy everything they had collected.
package report

import (
	"errors"
	"io/fs"
	"sort"
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/store"
)

// Coverage reasons a report can add beyond those the run recorded.
const (
	ReasonRunNotClosed       = "run_not_closed"
	ReasonTranscriptMismatch = "transcript_mismatch"
	ReasonGap                = "gap"
)

// Coverage is the run's trust state, as recorded at run time and as derived
// from the records themselves.
type Coverage struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons"`
	// StartRecorded and EndRecorded say whether the probe wrote its start and
	// end records: whether the run was watched from its first moment to its
	// last, according to the run itself.
	StartRecorded    bool   `json:"start_recorded"`
	EndRecorded      bool   `json:"end_recorded"`
	HookEntryAtStart string `json:"hook_entry_at_start"`
	HookEntryAtEnd   string `json:"hook_entry_at_end"`
}

// Declarations summarises what was recorded. Recorded is always known: it is a
// count of this store's own records. What is not known is how many were
// missed, and that number is never rendered as a count.
type Declarations struct {
	Recorded     int            `json:"recorded"`
	Unterminated []string       `json:"unterminated"`
	Dropped      []string       `json:"dropped"`
	ByTool       map[string]int `json:"by_tool"`
}

// Transcript is the accounting equation: the recorded id set against the
// distinct id set parsed from the transcript and its subagent files. Every
// field after Readable is null when the transcript could not be read, because
// "zero ids in the transcript" and "could not read the transcript" are
// different facts and only one of them is a count.
type Transcript struct {
	Path                  string   `json:"path"`
	Readable              bool     `json:"readable"`
	Files                 *int     `json:"files"`
	IDsInTranscript       *int     `json:"ids_in_transcript"`
	IDsRecorded           int      `json:"ids_recorded"`
	MissingFromStore      []string `json:"missing_from_store"`
	MissingFromTranscript []string `json:"missing_from_transcript"`
}

// Session is one run's report.
type Session struct {
	SessionID      string       `json:"session_id"`
	InstallID      string       `json:"install_id"`
	Coverage       Coverage     `json:"coverage"`
	Declarations   Declarations `json:"declarations"`
	Transcript     *Transcript  `json:"transcript"`
	Gaps           []store.Gap  `json:"gaps"`
	SkippedRecords int          `json:"skipped_records"`
}

// Report is the rendered output.
type Report struct {
	GeneratedAtUnixMS int64     `json:"generated_at_unix_ms"`
	Sessions          []Session `json:"sessions"`
}

// Build renders one session, or every session when sessionID is empty.
func Build(st *store.Store, sessionID string, now time.Time) (*Report, error) {
	gaps, err := st.ReadGaps()
	if err != nil {
		return nil, err
	}
	// Gaps are keyed by run directory, the one name a session has whether or
	// not its records still exist and whatever characters its id contains.
	byDir := map[string][]store.Gap{}
	for _, g := range gaps {
		dir := st.DirName(g.SessionID)
		byDir[dir] = append(byDir[dir], g)
	}

	var names []string
	if sessionID != "" {
		names = []string{st.DirName(sessionID)}
	} else {
		names, err = st.Runs()
		if err != nil {
			return nil, err
		}
		// A run that eviction removed has no directory but still has a gap,
		// and a report that could not show it would be hiding a deletion.
		seen := map[string]bool{}
		for _, n := range names {
			seen[n] = true
		}
		var evicted []string
		for dir := range byDir {
			if !seen[dir] {
				evicted = append(evicted, dir)
			}
		}
		sort.Strings(evicted)
		names = append(names, evicted...)
	}

	rep := &Report{GeneratedAtUnixMS: now.UnixMilli()}
	for _, name := range names {
		run, err := st.ReadRunDir(name)
		if err != nil {
			return nil, err
		}
		sess := build(run)
		sess.Gaps = byDir[name]
		if sess.Gaps == nil {
			sess.Gaps = []store.Gap{}
		}
		if len(sess.Gaps) > 0 {
			sess.Coverage.add(ReasonGap)
			if sess.SessionID == name && len(run.Declarations)+len(run.Terminals)+len(run.Coverage) == 0 {
				// Nothing left but the gap; it carries the real session id.
				sess.SessionID = sess.Gaps[0].SessionID
			}
		}
		sess.Coverage.finish()
		rep.Sessions = append(rep.Sessions, sess)
	}
	return rep, nil
}

func build(run *store.Run) Session {
	sess := Session{
		SessionID:      run.SessionID(),
		SkippedRecords: run.Skipped,
		Declarations: Declarations{
			Recorded:     len(run.Declarations),
			Unterminated: nonNil(run.Unterminated()),
			Dropped:      nonNil(run.Dropped()),
			ByTool:       map[string]int{},
		},
		Coverage: Coverage{
			Reasons:          []string{},
			HookEntryAtStart: store.EntryUnknown,
			HookEntryAtEnd:   store.EntryUnknown,
		},
	}
	for _, d := range run.Declarations {
		sess.Declarations.ByTool[d.ToolName]++
	}

	// What the run said about itself, phase by phase.
	for _, c := range run.Coverage {
		if sess.InstallID == "" {
			sess.InstallID = c.InstallID
		}
		switch c.Phase {
		case store.PhaseStart:
			sess.Coverage.StartRecorded = true
			sess.Coverage.HookEntryAtStart = c.HookEntry
		case store.PhaseEnd:
			sess.Coverage.EndRecorded = true
			sess.Coverage.HookEntryAtEnd = c.HookEntry
		}
		if c.State == store.StateUnverified && c.Reason != nil {
			sess.Coverage.add(*c.Reason)
		}
	}
	if !sess.Coverage.StartRecorded {
		sess.Coverage.add(store.ReasonProbeAbsent)
	}
	if !sess.Coverage.EndRecorded {
		sess.Coverage.add(ReasonRunNotClosed)
	}

	// What the records show, independent of what the run said.
	if len(sess.Declarations.Unterminated) > 0 {
		sess.Coverage.add(store.ReasonUnterminatedEntry)
	}
	if len(sess.Declarations.Dropped) > 0 {
		sess.Coverage.add(store.ReasonLockTimeout)
	}

	// The accounting equation, when the transcript can be read.
	if path := transcriptPath(run); path != "" {
		sess.Transcript = accounting(path, run)
		if sess.Transcript.Readable &&
			(len(sess.Transcript.MissingFromStore) > 0 || len(sess.Transcript.MissingFromTranscript) > 0) {
			sess.Coverage.add(ReasonTranscriptMismatch)
		}
	}
	return sess
}

func accounting(path string, run *store.Run) *Transcript {
	t := &Transcript{Path: path}

	recorded := map[string]bool{}
	for _, d := range run.Declarations {
		recorded[d.ToolUseID] = true
	}
	t.IDsRecorded = len(recorded)

	ids, files, err := TranscriptIDs(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// A transcript that exists but cannot be read is still unreadable;
			// the distinction is not one the accounting can act on.
			t.Readable = false
		}
		return t
	}
	t.Readable = true
	t.Files = &files
	n := len(ids)
	t.IDsInTranscript = &n

	t.MissingFromStore = []string{}
	for id := range ids {
		if !recorded[id] {
			t.MissingFromStore = append(t.MissingFromStore, id)
		}
	}
	t.MissingFromTranscript = []string{}
	for id := range recorded {
		if !ids[id] {
			t.MissingFromTranscript = append(t.MissingFromTranscript, id)
		}
	}
	sort.Strings(t.MissingFromStore)
	sort.Strings(t.MissingFromTranscript)
	return t
}

func transcriptPath(run *store.Run) string {
	for _, d := range run.Declarations {
		if d.TranscriptPath != "" {
			return d.TranscriptPath
		}
	}
	return ""
}

func (c *Coverage) add(reason string) {
	for _, r := range c.Reasons {
		if r == reason {
			return
		}
	}
	c.Reasons = append(c.Reasons, reason)
}

func (c *Coverage) finish() {
	c.State = store.StateVerified
	if len(c.Reasons) > 0 {
		c.State = store.StateUnverified
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
