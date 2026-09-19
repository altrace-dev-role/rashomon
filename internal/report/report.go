// Package report renders what the store holds for a session, and whether that
// can be trusted.
//
// It reads the coverage records the run wrote about itself. It never reads
// today's configuration to judge a past run: if it did, detach would
// retroactively invalidate every run ever captured, and the one action users
// are told is safe would destroy everything they had collected.
//
// A declaration is a request; an execution record is that request having run.
// What the absence of one means is three things at once -- the call was denied,
// or it failed, or its PostToolUse invocation recorded nothing -- and no field
// here picks between them. The ids are named and the permission mode each was
// declared in is named beside them, because that is what a consumer needs to
// exclude the modes in which nothing is ever denied.
//
// The accounting equation is computed per transcript, not per run. A nested
// claude -p inherits the parent session's id and writes its own transcript, so
// one run directory holds declarations against two or more transcript paths.
// Checking every recorded id against one of those transcripts reports the
// other transcripts' ids as missing from the store and its own as missing from
// the transcript: a mismatch in both directions, manufactured by the grouping
// rather than found in the records.
package report

import (
	"errors"
	"io/fs"
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// Coverage reasons a report can add beyond those the run recorded.
const (
	ReasonRunNotClosed       = "run_not_closed"
	ReasonTranscriptMismatch = "transcript_mismatch"
	ReasonExecutionMismatch  = "execution_mismatch"
	ReasonGap                = "gap"
)

// Reasons lists the coverage reason codes this package derives, as
// store.Reasons lists the ones a record carries. Together they are the whole
// vocabulary a reader of a report can meet.
func Reasons() []string {
	return []string{
		ReasonRunNotClosed,
		ReasonTranscriptMismatch,
		ReasonExecutionMismatch,
		ReasonGap,
	}
}

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

// Declarations summarises what was recorded. Recorded and WithoutTranscript
// are always known: both are counts of this store's own records, so zero is an
// honest answer for either. What is not known is how many were missed, and
// that number is never rendered as a count.
//
// WithoutTranscript counts the declarations that named no transcript path.
// They form no accounting group, because there is nothing to check them
// against.
type Declarations struct {
	Recorded          int            `json:"recorded"`
	WithoutTranscript int            `json:"without_transcript"`
	Unterminated      []string       `json:"unterminated"`
	Dropped           []string       `json:"dropped"`
	WithoutExecution  []Unexecuted   `json:"without_execution"`
	ByTool            map[string]int `json:"by_tool"`
}

// Unexecuted names a declaration that no execution record answers. It is not a
// denial: PreToolUse fires before the permission flow, so this list holds the
// denied, the failed and the unrecorded together, and it carries the
// permission mode so that a consumer can drop the modes in which nothing is
// denied rather than being handed a verdict this program cannot reach.
type Unexecuted struct {
	ToolUseID      string `json:"tool_use_id"`
	PermissionMode string `json:"permission_mode"`
}

// Executions counts what the PostToolUse recorder wrote. It is a count of this
// store's own records, so zero is an honest answer: it says no execution was
// recorded, which is exactly what is known.
type Executions struct {
	Recorded int `json:"recorded"`
}

// Transcript is the accounting equation for one transcript path: the ids
// recorded against that path, set against the distinct id set parsed from that
// transcript and its subagent files. Every field after Readable is null when
// the transcript could not be read, because "zero ids in the transcript" and
// "could not read the transcript" are different facts and only one of them is
// a count. That holds per group: one unreadable transcript renders null beside
// another's counts.
type Transcript struct {
	Path                  string   `json:"path"`
	Readable              bool     `json:"readable"`
	Files                 *int     `json:"files"`
	IDsInTranscript       *int     `json:"ids_in_transcript"`
	IDsRecorded           int      `json:"ids_recorded"`
	MissingFromStore      []string `json:"missing_from_store"`
	MissingFromTranscript []string `json:"missing_from_transcript"`
	// The execution half of the same equation. IDsExecuted counts this store's
	// records and is therefore always known; ResultsInTranscript is null when
	// the transcript could not be read, under the same rule as the counts
	// above.
	//
	// ExecutedButUnrecorded is a failure: the transcript says the call
	// finished and no execution record says so.
	//
	// DeniedByUser is not a failure and not a recording gap. It is the user
	// refusing the call at the permission prompt, which is the product working.
	// It is named separately because it used to be counted twice -- once in
	// ExecutedButUnrecorded, because a denial does produce a tool_result, and
	// once in DeclaredWithoutResult -- so a user exercising the prompt inflated
	// a list that exists to surface a broken recorder, and added
	// execution_mismatch to the coverage reasons.
	//
	// DeclaredWithoutResult is now what remains: the failed, and the transcript
	// that has not caught up. Denials are no longer among them, so the three
	// lists are disjoint and each means one thing.
	IDsExecuted           int      `json:"ids_executed"`
	ResultsInTranscript   *int     `json:"results_in_transcript"`
	ExecutedButUnrecorded []string `json:"executed_but_unrecorded"`
	DeniedByUser          []string `json:"denied_by_user"`
	DeclaredWithoutResult []string `json:"declared_without_result"`
}

// Session is one run's report. Transcripts holds one accounting group per
// distinct transcript path the run's declarations named, sorted by path.
type Session struct {
	SessionID      string       `json:"session_id"`
	InstallID      string       `json:"install_id"`
	Coverage       Coverage     `json:"coverage"`
	Declarations   Declarations `json:"declarations"`
	Executions     Executions   `json:"executions"`
	Transcripts    []Transcript `json:"transcripts"`
	Gaps           []store.Gap  `json:"gaps"`
	SkippedRecords int          `json:"skipped_records"`
	// Destinations is what the proxy observed, reconciled against what this
	// session declared. Always present: when no proxy store was configured it
	// carries Observed false with a reason, because a report that simply
	// omitted the section would read as "nothing was reached".
	Destinations Destinations `json:"destinations"`

	// Account is the agent's own summary, read from the transcript at render
	// time and never stored. Subagents is what the main transcript never
	// shows. SilentFailures sets the failure count against that summary.
	//
	// All three are always present, for the same reason Destinations is: a
	// section that vanishes when it has nothing to say cannot be told apart
	// from one that was never built.
	Account        Account           `json:"account"`
	Subagents      []SubagentSummary `json:"subagents"`
	SilentFailures SilentFailures    `json:"silent_failures"`

	// Families is which of this session's tool families were confirmed to
	// transit the proxy, derived from the join rather than from a probe.
	Families FamilyCoverage `json:"families"`
}

// Option configures Build.
//
// Variadic options rather than a wider signature, so a caller that does not
// know about the proxy store keeps compiling and keeps getting an honest
// "not observed" rather than being forced to pass a path it has no opinion
// about.
type Option func(*options)

type options struct {
	proxyStore string
}

// WithProxyStore names the proxy's causal store. An empty path means no store
// was configured, which the destinations section reports as not observed.
func WithProxyStore(path string) Option {
	return func(o *options) { o.proxyStore = path }
}

// Report is the rendered output.
type Report struct {
	GeneratedAtUnixMS int64 `json:"generated_at_unix_ms"`
	// Sessions always marshals as an array, never null.
	//
	// null and [] are the same absence to a reader and different values to a
	// consumer: the natural loop over sessions throws on one and is a no-op on
	// the other, and "no sessions yet" is the state every user is in exactly
	// once, before anything has been recorded.
	Sessions []Session `json:"sessions"`
}

// Empty is the report for a location that has recorded nothing, built without a
// store so that asking the question cannot create one. It is the same shape
// Build returns for a store with no runs.
func Empty(now time.Time) *Report {
	return &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}
}

// Build renders one session, or every session when sessionID is empty.
func Build(st *store.Store, sessionID string, now time.Time, opts ...Option) (*Report, error) {
	var cfg options
	for _, o := range opts {
		o(&cfg)
	}
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

	// Read once for the whole report: the forgotten set is a property of the
	// store, not of a session, and re-reading the gaps per session would be the
	// same answer at more cost.
	forgotten, err := st.ForgottenHost()
	if err != nil {
		return nil, err
	}

	// Sessions starts as an empty slice rather than nil, so a store with no runs
	// marshals the same array a store with runs does. See the field comment.
	rep := &Report{GeneratedAtUnixMS: now.UnixMilli(), Sessions: []Session{}}
	for _, name := range names {
		run, err := st.ReadRunDir(name)
		if err != nil {
			return nil, err
		}
		sess := build(run)
		// The destinations section is built per session, from the session's own
		// window. Reading the store once per session rather than once per
		// report is the cost of that: a window is a property of the run, and
		// sharing one observation across sessions would attribute each
		// session's destinations to all of them.
		sess.Destinations = buildDestinations(run, wire.Read(cfg.proxyStore, window(run)), st.Root(), forgotten)
		sess.Families = buildFamilies(run, observedHostSet(sess.Destinations),
			sess.Destinations.Observed, sess.Destinations.Reason)
		sess.Account = buildAccount(run)
		sess.Subagents = buildSubagents(run)
		sess.SilentFailures = buildSilentFailures(run, sess.Account)
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
			Recorded:         len(run.Declarations),
			Unterminated:     nonNil(run.Unterminated()),
			Dropped:          nonNil(run.Dropped()),
			WithoutExecution: []Unexecuted{},
			ByTool:           map[string]int{},
		},
		Executions:  Executions{Recorded: len(run.Executions)},
		Transcripts: []Transcript{},
		Coverage: Coverage{
			Reasons:          []string{},
			HookEntryAtStart: store.EntryUnknown,
			HookEntryAtEnd:   store.EntryUnknown,
		},
	}
	mode := map[string]string{}
	for _, d := range run.Declarations {
		sess.Declarations.ByTool[d.ToolName]++
		mode[d.ToolUseID] = d.PermissionMode
	}
	executed := map[string]bool{}
	for _, x := range run.Executions {
		executed[x.ToolUseID] = true
	}
	for _, id := range run.Unexecuted() {
		sess.Declarations.WithoutExecution = append(sess.Declarations.WithoutExecution,
			Unexecuted{ToolUseID: id, PermissionMode: mode[id]})
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

	// The accounting equation, once per transcript the run's declarations
	// named. A declaration naming none is counted rather than grouped.
	byPath := map[string]map[string]bool{}
	for _, d := range run.Declarations {
		if d.TranscriptPath == "" {
			sess.Declarations.WithoutTranscript++
			continue
		}
		ids := byPath[d.TranscriptPath]
		if ids == nil {
			ids = map[string]bool{}
			byPath[d.TranscriptPath] = ids
		}
		ids[d.ToolUseID] = true
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		t := accounting(path, byPath[path], executed)
		if t.Readable && (len(t.MissingFromStore) > 0 || len(t.MissingFromTranscript) > 0) {
			sess.Coverage.add(ReasonTranscriptMismatch)
		}
		// A result in the transcript with no execution record beside it is the
		// PostToolUse recorder having not fired or not landed. The reverse --
		// a declaration with no result -- is the ordinary shape of a denial
		// and says nothing about coverage.
		if t.Readable && len(t.ExecutedButUnrecorded) > 0 {
			sess.Coverage.add(ReasonExecutionMismatch)
		}
		sess.Transcripts = append(sess.Transcripts, t)
	}
	return sess
}

func accounting(path string, recorded, executed map[string]bool) Transcript {
	t := Transcript{Path: path, IDsRecorded: len(recorded)}
	for id := range recorded {
		if executed[id] {
			t.IDsExecuted++
		}
	}

	ids, results, denied, files, err := TranscriptIDs(path)
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
	nResults := len(results)
	t.ResultsInTranscript = &nResults

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
	t.ExecutedButUnrecorded = []string{}
	for id := range results {
		if !executed[id] {
			t.ExecutedButUnrecorded = append(t.ExecutedButUnrecorded, id)
		}
	}
	t.DeniedByUser = []string{}
	for id := range denied {
		t.DeniedByUser = append(t.DeniedByUser, id)
	}
	// What remains after the denials are named: failed, or the transcript has
	// not caught up. A denied call is in neither this list nor
	// ExecutedButUnrecorded -- it has its own, and the three are disjoint.
	t.DeclaredWithoutResult = []string{}
	for id := range recorded {
		if !results[id] && !denied[id] {
			t.DeclaredWithoutResult = append(t.DeclaredWithoutResult, id)
		}
	}
	sort.Strings(t.MissingFromStore)
	sort.Strings(t.MissingFromTranscript)
	sort.Strings(t.ExecutedButUnrecorded)
	sort.Strings(t.DeniedByUser)
	sort.Strings(t.DeclaredWithoutResult)
	return t
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
