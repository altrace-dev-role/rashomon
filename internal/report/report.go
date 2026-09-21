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

	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// Coverage reasons a report can add beyond those the run recorded.
//
// ReasonDuplicateDeclarations is a statement about records, not about why
// there are two: more declaration records than distinct tool_use_ids means
// more than one recorder wrote into this run, whatever put it there -- a
// plugin and a settings install both live, a future third origin, a
// duplicated settings entry. No coverage record can know this at write time
// -- each hook invocation sees exactly one call and has no visibility into
// whether another origin also recorded it -- so it is derived here, once,
// over the whole run's declarations, the same way ReasonGap is derived from
// the whole run's gap records rather than carried by any single one.
//
// Deliberately never applied to executions: chains.go documents that one
// tool_use_id legitimately carries two of those (PostToolUse and
// PostToolUseFailure), so the same check there would misfire on every
// healthy failed call.
//
// ReasonRecordsUnreadable belongs here and not in store.Reasons(), even
// though it is about the records: it is derived from run.Skipped while
// reading the whole run, the way ReasonRunNotClosed is derived from the
// absence of an end-phase coverage record. No coverage record a hook writes
// ever carries it -- a hook has no way to know that some OTHER line in the
// file failed to parse -- so publishing it in the coverage record's own
// reason enum would be a schema promising a record shape this program never
// writes, the same inverse drift TestStoreSchemaMatchesTheAllowlists exists
// to catch on every other field.
const (
	ReasonRunNotClosed          = "run_not_closed"
	ReasonRecordsUnreadable     = "records_unreadable"
	ReasonTranscriptMismatch    = "transcript_mismatch"
	ReasonExecutionMismatch     = "execution_mismatch"
	ReasonGap                   = "gap"
	ReasonDuplicateDeclarations = "duplicate_declarations"
)

// knownLabel clamps a stored file_label to the vocabulary shape.Labels()
// defines, mapping anything else to unknown.
//
// The value comes off disk, and this map's KEYS are rendered verbatim in both
// the text report and the JSON one. A record written by an older build, a
// newer one, or a hand-edited file could carry any string at all, and without
// this the report would print it -- which is the one thing every other line of
// this program is arranged to prevent. Counting it as unknown keeps the total
// honest and the rendered vocabulary closed; dropping it would lose a
// declaration the run really made.
func knownLabel(v string) string {
	for _, l := range shape.Labels() {
		if v == l {
			return v
		}
	}
	return shape.LabelUnknown
}

// Reasons lists the coverage reason codes this package derives, as
// store.Reasons lists the ones a record carries. Together they are the whole
// vocabulary this build writes or derives. A reason read off disk is printed
// as stored, so a record from another build or a hand-edited store can still
// show a code outside it.
func Reasons() []string {
	return []string{
		ReasonRunNotClosed,
		ReasonRecordsUnreadable,
		ReasonTranscriptMismatch,
		ReasonExecutionMismatch,
		ReasonGap,
		ReasonDuplicateDeclarations,
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

	// ByLabel counts declarations per file_label (v3). A declaration whose
	// tool names no file is not counted at all, so the total here is the
	// number of calls that named one, not the number of declarations.
	//
	// Empty until schema 3 carries the field, because a label the hook path
	// dropped is a label this count never sees.
	ByLabel map[string]int `json:"by_label"`

	// ByProgram counts Bash calls per program, and ByVerbClass counts every
	// declaration by what it turned out to be.
	//
	// They exist because "by tool: Bash 23" answers nothing. Bash is not a
	// thing anyone did; it is the door every shell command comes through, and
	// a reader who asks what a session did and is told "Bash 23" has learned
	// only that they used a terminal. The program and the verb class are
	// already on every record -- shape.Derive puts them there -- and until
	// now they were reachable from `--chain` alone, one call at a time.
	//
	// ProgramsUnknown counts the Bash calls whose program could not be told:
	// a first word cut off by an unterminated quote, a line with no word in
	// command position, one beginning with something shape does not parse
	// (a here-document, arithmetic, an array), or an input with no command
	// string. A line that fails to tokenize later still names its program. It is
	// a count and not a bucket in ByProgram, because "unknown" is not a
	// program and a reader scanning the list must not find it sitting among
	// real ones.
	ByProgram       map[string]int `json:"by_program"`
	ByVerbClass     map[string]int `json:"by_verb_class"`
	ProgramsUnknown int            `json:"programs_unknown"`
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

	// Nono is what a sandbox saw, when one was supervising. A fourth evidence
	// source, carried because it DISAGREES with the other three in ways that
	// are informative rather than alarming: it sees plain HTTP, which this
	// proxy structurally cannot.
	Nono Nono `json:"nono"`

	// Chains is the causal view: which prompt produced which calls. Every other
	// section here is a set, and a set is exactly the structure that discards
	// the edge between a request and its consequences.
	Chains Chains `json:"chains"`
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
	nonoTrail  string
	runToken   string
	// tokenRequested records that a tag WAS in play, separately from whether
	// any row carried one. Without it the renderer inferred "no token was in
	// use" from a zero match count, which is a statement about the reporting
	// process printed as a statement about the session -- and it was false in
	// the two most interesting cases: a re-render, and a tokened run whose
	// clients all stripped the credential.
	tokenRequested bool
	isOurs         func(string) bool
}

// WithNonoTrail names a nono audit-events.ndjson to reconcile against.
//
// An empty path means none was configured, which the report states as a
// coverage line rather than omitting -- a section that vanishes when it has
// nothing to say cannot be told apart from one that was never built.
func WithNonoTrail(path string) Option {
	return func(o *options) { o.nonoTrail = path }
}

// WithRunToken supplies the session tag this run handed to the proxy.
//
// IN MEMORY ONLY, and deliberately not read from disk. The join needs the RAW
// token -- a digest cannot be compared against the proxy's run_id column -- so
// retaining it anywhere would mean storing a value that identifies a session's
// traffic. It is available exactly while the process that minted it is alive,
// which is when `run` renders its automatic report; a later `rashomon report`
// has no token and falls back to the window, and says which it used.
func WithRunToken(token string) Option {
	return func(o *options) {
		o.runToken = token
		o.tokenRequested = token != ""
	}
}

// WithTokenVerifier supplies the test for whether a FOREIGN run_id is a tag
// this install issued.
//
// Only a tag that verifies may be treated as another session's and excluded.
// Anything else is not evidence about anybody and falls back to the clock --
// see internal/wire's joinOf for the two ways the alternative failed.
func WithTokenVerifier(isOurs func(string) bool) Option {
	return func(o *options) { o.isOurs = isOurs }
}

// WithProxyStore names the proxy's causal store. An empty path means no store
// was configured, which the destinations section reports as not observed.
func WithProxyStore(path string) Option {
	return func(o *options) { o.proxyStore = path }
}

// Report is the rendered output.
type Report struct {
	GeneratedAtUnixMS int64 `json:"generated_at_unix_ms"`
	// Redacted says the hostnames in this report are keyed digests rather than
	// names. Carried on the report itself so the renderer can print the legend
	// and a JSON consumer does not have to infer it from the shape of a string.
	Redacted bool `json:"redacted"`
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
		w := window(run)
		w.RunID = cfg.runToken
		w.IsOurs = cfg.isOurs
		sess.Destinations = buildDestinations(run, wire.Read(cfg.proxyStore, w), st.Root(), forgotten)
		// Set here rather than threaded through buildDestinations: it is a fact
		// about THIS RENDER, not about the observation, and widening that
		// function's signature would have touched every existing caller to say
		// "false" -- churn that hides the one call site that matters.
		sess.Destinations.TokenRequested = cfg.tokenRequested
		sess.Families = buildFamilies(run, observedHostSet(sess.Destinations),
			sess.Destinations.Observed, sess.Destinations.Reason)
		// After Destinations, and reading it rather than the observation: the
		// chain's host states must be the ones the destinations section already
		// suppressed and accounted for, or a forgotten host returns in a
		// different section under a different name for the same row.
		// After Destinations, and reading it rather than the raw observation:
		// the comparison must use the view that has already suppressed
		// forgotten hosts, or a forgotten host returns here under a different
		// heading.
		sess.Nono = buildNono(
			nono.Read(cfg.nonoTrail, nono.Window{Start: w.Start, End: w.End}),
			sess.Destinations, cfg.nonoTrail != "", forgotten)
		sess.Chains = buildChains(run, sess.Destinations, deniedSet(sess.Transcripts), forgotten)
		sess.Account = buildAccount(run)
		sess.Subagents = buildSubagents(run)
		sess.SilentFailures = BuildSilentFailures(run, sess.Account)
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
			ByLabel:          map[string]int{},
			ByProgram:        map[string]int{},
			ByVerbClass:      map[string]int{},
		},
		Executions:  Executions{Recorded: len(run.Executions)},
		Transcripts: []Transcript{},
		Coverage: Coverage{
			Reasons:          []string{},
			HookEntryAtStart: store.EntryUnknown,
			HookEntryAtEnd:   store.EntryUnknown,
		},
	}
	byTool, byLabel, withoutExecution, executed := CountDeclarations(run.Declarations, run.Executions)
	sess.Declarations.ByTool = byTool
	sess.Declarations.ByLabel = byLabel
	sess.Declarations.WithoutExecution = withoutExecution
	// Report-only tallies, beside the shared ones rather than inside them:
	// the turn digest counts with CountDeclarations and needs none of these,
	// so they stay out of the rule the two share.
	declByID := map[string]int{}
	for _, d := range run.Declarations {
		if d.Shape.VerbClass != "" {
			sess.Declarations.ByVerbClass[d.Shape.VerbClass]++
		}
		switch {
		case d.Shape.Program != nil:
			sess.Declarations.ByProgram[*d.Shape.Program]++
		case d.ToolName == "Bash":
			// Only a shell tool has a program to miss. Every other tool
			// legitimately has none, and counting those here would report a
			// gap where there is nothing to know.
			sess.Declarations.ProgramsUnknown++
		}
		// Empty is excluded, the same way WithoutTranscript counts a missing
		// path rather than grouping every such declaration under one key: an
		// id this program never received is not evidence that two records
		// share an identity, only that neither carries one.
		if d.ToolUseID != "" {
			declByID[d.ToolUseID]++
		}
	}

	// What the run said about itself, phase by phase.
	facts := RollupCoverage(run.Coverage)
	sess.InstallID = facts.InstallID
	sess.Coverage.StartRecorded = facts.StartRecorded
	sess.Coverage.EndRecorded = facts.EndRecorded
	sess.Coverage.HookEntryAtStart = facts.HookEntryAtStart
	sess.Coverage.HookEntryAtEnd = facts.HookEntryAtEnd
	for _, r := range facts.Reasons {
		sess.Coverage.add(r)
	}
	if !sess.Coverage.StartRecorded {
		sess.Coverage.add(store.ReasonProbeAbsent)
	}
	if !sess.Coverage.EndRecorded {
		sess.Coverage.add(ReasonRunNotClosed)
	}
	// SkippedRecords used to reach only the rendered number at text.go's
	// "skipped records" line, tied to no reason. A store this binary could
	// not fully read was therefore free to render `verified`, over evidence
	// it never saw -- the exact violation of "never render zero when we mean
	// unknown" this package exists to prevent.
	if sess.SkippedRecords > 0 {
		sess.Coverage.add(ReasonRecordsUnreadable)
	}

	// What the records show, independent of what the run said.
	if len(sess.Declarations.Unterminated) > 0 {
		sess.Coverage.add(store.ReasonUnterminatedEntry)
	}
	if len(sess.Declarations.Dropped) > 0 {
		sess.Coverage.add(store.ReasonLockTimeout)
	}
	for _, n := range declByID {
		if n > 1 {
			sess.Coverage.add(ReasonDuplicateDeclarations)
			break
		}
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

// CountDeclarations tallies a set of declarations against a set of
// executions: per-tool and per-label counts, the executed set keyed by
// tool_use_id, and the declarations no execution answers, each carrying the
// permission mode it was declared under.
//
// It takes slices rather than a *store.Run so a caller scoped to less than a
// whole run counts its own subset under the exact same rule report uses for a
// whole session. digest is that caller: it hands this the declarations and
// executions of ONE TURN, and gets back the turn's own tallies rather than a
// second implementation that could drift from this one.
func CountDeclarations(decls []store.Declaration, execs []store.Execution) (
	byTool, byLabel map[string]int, withoutExecution []Unexecuted, executed map[string]bool,
) {
	byTool = map[string]int{}
	byLabel = map[string]int{}
	mode := map[string]string{}
	for _, d := range decls {
		byTool[d.ToolName]++
		if d.FileLabel != nil {
			byLabel[knownLabel(*d.FileLabel)]++
		}
		mode[d.ToolUseID] = d.PermissionMode
	}
	executed = map[string]bool{}
	for _, x := range execs {
		executed[x.ToolUseID] = true
	}
	withoutExecution = []Unexecuted{}
	for _, d := range decls {
		if !executed[d.ToolUseID] {
			id := d.ToolUseID
			withoutExecution = append(withoutExecution,
				Unexecuted{ToolUseID: id, PermissionMode: mode[id]})
		}
	}
	return byTool, byLabel, withoutExecution, executed
}

// CoverageFacts is the per-invocation coverage evidence common to a
// session-wide report and a turn-scoped digest: which phases were recorded,
// what hook entry state they carried, and which reasons the records
// themselves declared unverified.
//
// It stops short of the two reasons that depend on the CALLER's own scope.
// run_not_closed only means something against a whole run -- a turn digest
// read mid-session must not add it, by H-84 -- and whether a missing start or
// end phase is even meaningful depends on whether the caller is looking at
// the whole run or a slice of it. Each caller adds those itself, against the
// scope it actually has.
type CoverageFacts struct {
	InstallID        string
	StartRecorded    bool
	EndRecorded      bool
	HookEntryAtStart string
	HookEntryAtEnd   string
	// Reasons is de-duplicated, in first-seen order, from every record whose
	// State is Unverified.
	Reasons []string
}

// RollupCoverage reduces a set of coverage records to CoverageFacts. covs may
// be a whole run's records, as report uses it, or a caller-defined subset --
// digest filters to the records that fall inside one turn's window before
// calling this, because a coverage record carries no tool_use_id or prompt_id
// of its own and a time window is the only join key available for one.
func RollupCoverage(covs []store.Coverage) CoverageFacts {
	f := CoverageFacts{HookEntryAtStart: store.EntryUnknown, HookEntryAtEnd: store.EntryUnknown}
	seen := map[string]bool{}
	for _, c := range covs {
		if f.InstallID == "" {
			f.InstallID = c.InstallID
		}
		// A start or end probe that found recording paused still writes a
		// record of that phase, so the skip shows up in this session's own
		// report. That record says the probe did NOT run, so it must not count
		// as the start or end having been recorded, and the hook entry it
		// carries was never resolved by a probe. Counting it once rendered
		// "start recorded: yes" beside probe_absent's "no session start was
		// recorded". Its reason is still added below.
		paused := c.Reason != nil && *c.Reason == store.ReasonRecordingPaused
		switch {
		case paused:
		case c.Phase == store.PhaseStart:
			f.StartRecorded = true
			f.HookEntryAtStart = c.HookEntry
		case c.Phase == store.PhaseEnd:
			f.EndRecorded = true
			f.HookEntryAtEnd = c.HookEntry
		}
		if c.State == store.StateUnverified && c.Reason != nil && !seen[*c.Reason] {
			seen[*c.Reason] = true
			f.Reasons = append(f.Reasons, *c.Reason)
		}
	}
	return f
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
