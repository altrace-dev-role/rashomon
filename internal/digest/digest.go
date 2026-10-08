// Package digest projects what ONE TURN of a session recorded: a small JSON
// document, read only from the store's own NDJSON files.
//
// A Stop hook fires per turn, and a session-scoped summary rendered there
// would raise a finding from an early turn against an unrelated later one --
// report.Session answers "what has this session recorded", and that is the
// wrong shape for "what did THIS reply produce". A turn is scoped by
// prompt_id: chains.go groups a report's causal view by (transcript, prompt),
// but a subagent's declarations carry the PARENT's prompt_id under the
// subagent's OWN transcript path (record.go's TranscriptPath doc), so keying
// on both would put a subagent's calls in a different bucket from the turn
// that spawned it and silently under-count it. This package keys on
// prompt_id ALONE, and counts subagent calls separately rather than folding
// them in unmarked -- see Subagents.
//
// It is read-only in a way report.Build is not: it opens the store with
// store.OpenExisting, never store.Open, so asking for a digest cannot mint an
// install identity on a machine that has recorded nothing; it never reads the
// proxy database, never globs a subagent's transcript file, and never writes
// baseline/. It also never opens the SESSION's own transcript file --
// report's Account and buildSilentFailures read that for the final assistant
// message, and that read is both session-wide (wrong scope for a turn) and
// racy at Stop (the file may not hold the final line yet). A digest's final
// message, when it has one, is handed in by its caller -- see
// report.AccountFromMessage.
//
// It carries no new content. Every field here is a projection of something
// the recorder already wrote; see the spec's Non-goals.
package digest

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// SchemaVersion is this document's own version, independent of the store's.
// It is not written to the store and starts at 1.
const SchemaVersion = 1

// CapBytes is the ceiling on the WHOLE marshalled document, not a per-field
// cap. WithoutExecution, Unterminated and Dropped grow with the turn; ByTool
// and ByLabel grow with the number of distinct tools and labels the turn
// touched; TestBending's two lists grow with the turn's test runs.
// truncate() trims all seven, in a fixed order, before this is
// reached, rather than capping each independently -- a turn wide in several
// of them at once could exceed the total while every field stayed within an
// independent cap of its own.
const CapBytes = 8 << 10

// Reasons this package adds to the vocabulary store.Reasons() and
// report.Reasons() already define. Together the three lists are the whole
// vocabulary a reader of a digest can meet.
const (
	// ReasonNoStore: asked about a location where nothing has ever been
	// recorded. Not an error -- see Build's caller -- but not "verified"
	// either: nothing was checked because there was nothing to check.
	ReasonNoStore = "no_store"
	// ReasonRecordsSkipped: this read encountered at least one NDJSON line
	// that did not parse (store.Run.Skipped). It is not attributed to a
	// specific turn -- a line that failed to parse carries no recoverable
	// prompt_id -- so it is raised against every digest read from this
	// session rather than risk the alternative: a line lost to exactly the
	// race a concurrent writer can cause at Stop (see
	// store.Store.ReadRunConsistent) silently undercounting the turn that
	// was actually affected.
	ReasonRecordsSkipped = "records_skipped"
)

// TurnCoverage is turn-scoped coverage: the probe fired at start, the entry
// was present for the calls in this turn, and no gap intersects it.
//
// It is deliberately NOT report.Coverage. report.go:467 adds run_not_closed
// whenever EndRecorded is false, and EndRecorded is false for every turn read
// at Stop by construction -- the session is still open. Projecting session
// coverage onto a turn would mark every healthy turn unverified for a fact
// that is true of all of them and a finding against none. TurnCoverage's
// clean state is reachable mid-session; run_not_closed is not in its
// vocabulary at all.
type TurnCoverage struct {
	State   string   `json:"state"`
	Reasons []string `json:"reasons"`
	// StartRecorded and HookEntryAtStart are session-wide facts, carried here
	// because they still bear on this turn: there is exactly one SessionStart
	// per session and it precedes every turn's own window, so what it says is
	// true for all of them.
	StartRecorded    bool   `json:"start_recorded"`
	HookEntryAtStart string `json:"hook_entry_at_start"`
}

// Unexecuted is report.Unexecuted, reused rather than redeclared so the two
// packages cannot describe the same fact with two different shapes.
type Unexecuted = report.Unexecuted

// Declarations is this turn's own tally, built by the SAME counting loop
// report.build uses for a whole session (report.CountDeclarations), given
// this turn's declarations and the executions that answer them.
//
// Recorded is a count of this turn's own prompt_id-tagged records, and by
// itself it is ambiguous: a turn that made no tool calls and a turn nothing
// was recorded for both show Recorded == 0. Digest.Unknown resolves that
// using TurnCoverage -- see its doc -- so Recorded is NEVER rendered as a bare
// zero standing in for "nothing is known".
type Declarations struct {
	Recorded int `json:"recorded"`

	WithoutExecution        []Unexecuted `json:"without_execution"`
	WithoutExecutionOmitted int          `json:"without_execution_omitted"`
	Unterminated            []string     `json:"unterminated"`
	UnterminatedOmitted     int          `json:"unterminated_omitted"`
	// Dropped is windowed against this turn's own span by RECORDED TIME, not
	// by prompt_id: a dropped terminal's declaration never landed, so it
	// never carried a prompt_id to key on in the first place, and this is
	// the best attribution the records support -- see selectTurn's doc on
	// turnWindow. Empty whenever the turn's own window could not be
	// established (Recorded == 0), for the same reason.
	Dropped        []string `json:"dropped"`
	DroppedOmitted int      `json:"dropped_omitted"`

	ByTool        map[string]int `json:"by_tool"`
	ByToolOmitted int            `json:"by_tool_omitted"`
	// ByLabel is empty for a declaration whose tool names no file, exactly as
	// report.Declarations.ByLabel is.
	ByLabel        map[string]int `json:"by_label"`
	ByLabelOmitted int            `json:"by_label_omitted"`
}

// SubagentCounts is what a subagent contributed to THIS turn, named apart
// from Declarations.Recorded/Executions.Recorded so a reader can see the
// composition rather than a total that does not say who did the work --
// measured at a median 51% of a session's calls in the sessions that use
// subagents (account.go's SubagentSummary doc).
//
// Both counts are already included in Declarations.Recorded and
// Executions.Recorded: this is a breakdown of that total, not an addition to
// it, because a subagent's PreToolUse payload carries the PARENT's prompt_id
// (spec-chain-and-scope.md:129) -- that is what the records say a subagent
// call belongs to, and there is no second, disjoint pool for it to come from.
type SubagentCounts struct {
	Declarations int `json:"declarations"`
	Executions   int `json:"executions"`
}

// TestBending is report.TestBending for one turn, with how much of each list
// truncate cut. The lists grow with the turn, so they are growable fields like
// the five in Declarations; the omitted counts are what keep a cut list's
// finding alive, since recap counts len + omitted and not the list alone.
type TestBending struct {
	TestsOnlyThenGreen        []report.SeqPair   `json:"tests_only_then_green"`
	TestsOnlyThenGreenOmitted int                `json:"tests_only_then_green_omitted"`
	Flaky                     []report.FlakyPair `json:"flaky"`
	FlakyOmitted              int                `json:"flaky_omitted"`
}

// Digest is one turn's projection.
type Digest struct {
	SchemaVersion     int    `json:"schema_version"`
	GeneratedAtUnixMS int64  `json:"generated_at_unix_ms"`
	SessionID         string `json:"session_id"`
	PromptID          string `json:"prompt_id"`
	InstallID         string `json:"install_id"`

	Coverage     TurnCoverage `json:"coverage"`
	Declarations Declarations `json:"declarations"`

	// Unknown is true exactly when Declarations.Recorded == 0 AND Coverage
	// is not verified: the two facts together are what "the agent used no
	// tools" and "nothing was recorded" have in common on disk, and Unknown
	// is how the digest tells them apart rather than rendering the second as
	// a clean zero. A non-zero Recorded is never marked Unknown, whatever
	// Coverage says -- the count is still a real fact about this store, and a
	// coverage problem beside it is a coverage finding, not an unknown count.
	Unknown bool `json:"unknown"`

	Executions     report.Executions     `json:"executions"`
	SilentFailures report.SilentFailures `json:"silent_failures"`
	// MaskedRuns is this turn's build and test calls whose exit status their
	// line masked and that no later plain run in this turn followed up, set
	// against this turn's final message (report.BuildMaskedRuns over the
	// same turn-scoped run SilentFailures reads).
	MaskedRuns report.MaskedRuns `json:"masked_runs"`
	Subagents  SubagentCounts    `json:"subagents"`

	// TestBending is the pairs report.DetectTestBending finds over the whole
	// session, cut to those whose two runs are both this turn's calls, main
	// agent and subagents together (turnPairs): seq pairs, and which run of a
	// flaky pair failed, and nothing else. See report.TestBending's doc for
	// the two patterns, and this package's TestBending for why each list has
	// an omitted count.
	TestBending TestBending `json:"test_bending"`

	// Gaps intersecting this turn's window. Rendered rather than dropped, for
	// the same reason report never drops them: a forget that vanished from
	// view would read as undone.
	Gaps []store.Gap `json:"gaps"`

	// SkippedRecords is store.Run.Skipped for this session's whole read: a
	// line that did not parse, whatever turn it belonged to. It is not
	// turn-scoped -- a line that failed to parse carries no recoverable
	// prompt_id to scope it BY -- and its presence also raises
	// ReasonRecordsSkipped in Coverage.Reasons.
	SkippedRecords int `json:"skipped_records"`

	// Truncated is set when any of the seven growable fields -- five in
	// Declarations, two in TestBending -- was cut to hold the whole document
	// under CapBytes. See truncate.go.
	Truncated bool `json:"truncated"`
}

// readLockBudget bounds how long ReadRunConsistent may wait for a writer's
// lock, per file. It is NOT store.lockBudget (2s): that number is what a
// WRITER may cost another writer, and a reader borrowing it could spend
// three-quarters of the whole 50ms budget this command owes waiting on a lock
// for a single file. lock_unix.go's own comment measures a real append at
// sub-millisecond; 5ms is generous headroom above that, not a real expected
// wait.
const readLockBudget = 5 * time.Millisecond

// Empty is the digest for a location that has recorded nothing, built
// without a store so that asking the question cannot create one -- the same
// rule report.Empty follows, for the same reason: OpenExisting must be able
// to answer "there is nothing here" without OpenExisting's caller having
// opened anything.
func Empty(now time.Time, sessionID, promptID string) *Digest {
	d := &Digest{
		SchemaVersion:     SchemaVersion,
		GeneratedAtUnixMS: now.UnixMilli(),
		SessionID:         sessionID,
		PromptID:          promptID,
		Coverage: TurnCoverage{
			State:            store.StateUnverified,
			Reasons:          []string{ReasonNoStore},
			HookEntryAtStart: store.EntryUnknown,
		},
		Declarations: Declarations{
			WithoutExecution: []Unexecuted{},
			Unterminated:     []string{},
			Dropped:          []string{},
			ByTool:           map[string]int{},
			ByLabel:          map[string]int{},
		},
		Unknown:        true,
		SilentFailures: report.SilentFailures{AbsentWords: []string{}},
		TestBending:    TestBending{TestsOnlyThenGreen: []report.SeqPair{}, Flaky: []report.FlakyPair{}},
		Gaps:           []store.Gap{},
	}
	return d
}

// Build projects one turn.
//
// st MUST come from store.OpenExisting, never store.Open: asking for a
// digest is a question, and a command that only asks a question must not
// mint an install identity and an HMAC key as a side effect of being asked --
// see cmd/rashomon's openStoreForRead, which every other read-only command
// already follows for the same reason, and H-87.
//
// lastAssistantMessage is the text a caller already has -- a future Stop
// hook's own payload -- never a path this function reads for itself. See the
// package doc and report.AccountFromMessage.
func Build(st *store.Store, sessionID, promptID, lastAssistantMessage string, now time.Time) (*Digest, error) {
	run, err := st.ReadRunConsistent(sessionID, readLockBudget)
	if err != nil {
		return nil, err
	}

	allGaps, err := st.ReadGaps()
	if err != nil {
		return nil, err
	}
	dir := st.DirName(sessionID)
	var gaps []store.Gap
	for _, g := range allGaps {
		if st.DirName(g.SessionID) == dir {
			gaps = append(gaps, g)
		}
	}

	d := build(run, gaps, promptID, lastAssistantMessage, now)
	if d.SessionID == "" {
		d.SessionID = sessionID
	}
	truncate(d)
	return d, nil
}

func build(run *store.Run, gaps []store.Gap, promptID, lastAssistantMessage string, now time.Time) *Digest {
	// Masking is judged on the command that ran.
	report.MaskingAsRan(run)
	w := selectTurn(run, promptID)

	d := &Digest{
		SchemaVersion:     SchemaVersion,
		GeneratedAtUnixMS: now.UnixMilli(),
		SessionID:         run.SessionID(),
		PromptID:          w.promptID,
	}

	turnExecs := filterExecutions(run.Executions, turnToolUseIDs(w))
	// One small Run -- this turn's own declarations and executions, nothing
	// else -- built once and shared by CountDeclarations and
	// BuildSilentFailures below, the same currency both already take.
	turnRun := &store.Run{Declarations: w.declarations, Executions: turnExecs}
	byTool, byLabel, withoutExec, _ := report.CountDeclarations(turnRun)
	d.Declarations = Declarations{
		Recorded:         len(w.declarations),
		WithoutExecution: withoutExec,
		Unterminated:     turnUnterminated(w, run.Terminals),
		Dropped:          turnDropped(run, w),
		ByTool:           byTool,
		ByLabel:          byLabel,
	}
	if d.Declarations.Unterminated == nil {
		d.Declarations.Unterminated = []string{}
	}
	if d.Declarations.Dropped == nil {
		d.Declarations.Dropped = []string{}
	}

	d.Executions = report.Executions{Recorded: len(turnExecs)}
	d.Subagents = subagentCounts(w, turnExecs)
	// No denied set: a digest reads no transcript, so a denied edit reads as
	// "no execution record", which stops a pattern rather than completing
	// one. The under-claim is the direction this line can afford to be wrong.
	// Over the session's run, not turnRun: a call between two of this turn's
	// runs may carry another turn's prompt_id (a background subagent launched
	// earlier) or have lost its declaration, and either may have been an
	// edit. Only the pairs whose two runs are both this turn's are kept.
	d.TestBending = turnPairs(report.DetectTestBending(run, nil), w)

	tc, global := buildTurnCoverage(run, gaps, w)
	if run.Skipped > 0 {
		tc.add(ReasonRecordsSkipped)
		tc.State = store.StateUnverified
	}
	d.Coverage = tc
	d.InstallID = global.InstallID
	d.SkippedRecords = run.Skipped

	acct := report.AccountFromMessage(lastAssistantMessage)
	d.SilentFailures = report.BuildSilentFailures(turnRun, acct)
	d.MaskedRuns = report.BuildMaskedRuns(turnRun, acct)

	d.Gaps = gapsInWindow(gaps, w)
	if d.Gaps == nil {
		d.Gaps = []store.Gap{}
	}

	d.Unknown = d.Declarations.Recorded == 0 && d.Coverage.State != store.StateVerified
	return d
}

// turnPairs is tb, found over the session, cut to the pairs whose two runs are
// both among w's own declarations: a pair across two turns is the report's to
// name, and an earlier turn's pair raised again on a later one would point at
// work this reply did not do.
func turnPairs(tb report.TestBending, w turnWindow) TestBending {
	seqs := make(map[int64]bool, len(w.declarations))
	for _, d := range w.declarations {
		seqs[d.Seq] = true
	}
	out := TestBending{TestsOnlyThenGreen: []report.SeqPair{}, Flaky: []report.FlakyPair{}}
	for _, p := range tb.TestsOnlyThenGreen {
		if seqs[p[0]] && seqs[p[1]] {
			out.TestsOnlyThenGreen = append(out.TestsOnlyThenGreen, p)
		}
	}
	for _, p := range tb.Flaky {
		if seqs[p.Seqs[0]] && seqs[p.Seqs[1]] {
			out.Flaky = append(out.Flaky, p)
		}
	}
	return out
}
