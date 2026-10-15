package report

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Input is everything Assemble renders: each session's records as the store
// held them, and what the read step (internal/report/gather) read beside them
// -- the transcripts the declarations name, the project baseline's answer and
// the sandbox trail. This package opens none of it, which is what lets digest
// and spend link its verdict helpers without linking a way to read a file.
type Input struct {
	Sessions []SessionInput
	// NonoConfigured says a sandbox trail was named, whether or not it could
	// be read: a trail nobody asked for is silent, and one that was asked for
	// and not read says so.
	NonoConfigured bool
	// Forgotten reports a host the store was told to forget, so it does not
	// return under the sandbox heading. Nil forgets none.
	Forgotten func(host string) bool
	// ForgottenBefore reports a host forgotten at or after the declaration
	// recorded at declaredAtMS (store.Store.ForgottenHostBefore), so the
	// destinations, families and chains drop what a failed forget left
	// behind and keep a later, genuine re-use of the name. Nil forgets none.
	ForgottenBefore func(host string, declaredAtMS int64) bool
}

// SessionInput is one session as the read step gathered it.
type SessionInput struct {
	// Dir is the run directory's name: the one name a session has whether or
	// not its records still exist and whatever characters its id contains.
	Dir string
	// Run is the session's records. Assemble marks its masking in place
	// (MaskingAsRan) before anything reads it; nil is a run with none.
	Run *store.Run
	// Gaps are the store's gap records for this run directory.
	Gaps []store.Gap
	// Transcripts is what was read at each path TranscriptPaths(Run) names.
	Transcripts map[string]TranscriptRead
	// Novelty is the project baseline's answer (NoveltyQueryOf).
	Novelty Novelty
	// Nono is the sandbox trail over SessionWindow(Run).
	Nono nono.Observation
}

// TranscriptRead is what the read step found at one transcript path: the ids
// of the transcript and its subagent files, or the error that stopped the
// read -- an error satisfying errors.Is(err, fs.ErrNotExist) for a file that
// is not there -- and the final assistant message of the main file, when one
// could be read.
type TranscriptRead struct {
	Set       *TranscriptSet
	Err       error
	Final     string
	FinalRead bool
}

// Assemble renders the sessions the read step gathered, in the order given. It
// opens nothing: every fact it states about a file outside the store is one
// the input carries.
func Assemble(in Input, now time.Time) *Report {
	// Sessions starts as an empty slice rather than nil, so a store with no runs
	// marshals the same array a store with runs does. See the field comment.
	rep := &Report{ReportVersion: CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]Session, 0, len(in.Sessions))}
	for _, si := range in.Sessions {
		rep.Sessions = append(rep.Sessions, AssembleSession(si, in.NonoConfigured, in.Forgotten, in.ForgottenBefore))
	}
	return rep
}

// AssembleSession is one session's report, built from what the read step
// gathered for it alone. A caller that reads one SessionInput at a time --
// gather.Build does, so that a run's transcript and id sets are garbage
// before the next run is read -- renders the same report this way as calling
// Assemble with every SessionInput collected first.
func AssembleSession(in SessionInput, nonoConfigured bool, forgotten func(string) bool, forgottenBefore func(string, int64) bool) Session {
	run := in.Run
	if run == nil {
		run = &store.Run{}
	}
	MaskingAsRan(run)
	sess := build(run, in.Transcripts)
	sess.Destinations = buildDestinations(run, in.Novelty, forgottenBefore)
	sess.Families = buildFamilies(run, forgottenBefore)
	// The sandbox section is built per session, from the session's own
	// window: a window is a property of the run, and sharing one observation
	// across sessions would attribute each session's sandbox events to all of
	// them.
	sess.Nono = buildNono(in.Nono, nonoConfigured, madeMCPCall(run), declaredHosts(run), forgotten)
	denied := deniedSet(sess.Transcripts)
	// One grouping of the executions, shared by the chains, the timeline and
	// the test runs, and one detection, shared by the timeline's annotations
	// and the test runs' pairs.
	executed := executionsByID(run)
	sess.Chains = buildChains(run, executed, denied, forgottenBefore)
	tb := detectTestBending(run, executed, denied)
	sess.Timeline = timelineFrom(run, executed, denied, tb)
	sess.TestRuns = buildTestRuns(run, executed, denied, tb)
	sess.Account = buildAccount(run, in.Transcripts)
	sess.Subagents = buildSubagents(run)
	sess.SilentFailures = BuildSilentFailures(run, sess.Account)
	sess.MaskedRuns = sessionMaskedRuns(run, sess.Account)
	// Both of full's readers have now run; a report holding many sessions
	// must not hold every one's whole final message past this point.
	sess.Account = sess.Account.dropFull()
	sess.Gaps = in.Gaps
	if sess.Gaps == nil {
		sess.Gaps = []store.Gap{}
	}
	if len(sess.Gaps) > 0 {
		sess.Coverage.add(ReasonGap)
		if sess.SessionID == in.Dir && len(run.Declarations)+len(run.Terminals)+len(run.Coverage) == 0 {
			// Nothing left but the gap; it carries the real session id.
			sess.SessionID = sess.Gaps[0].SessionID
		}
	}
	sess.Coverage.finish()
	return sess
}
