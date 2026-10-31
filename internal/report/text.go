package report

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The words that stand in for a value this program does not have.
//
// This is the one thing a text renderer can get wrong that the JSON cannot. A
// null in the JSON is a fact that was not available -- the transcript could not
// be read, the run never said -- and rendering it as 0 or as an empty list
// turns "we do not know" into a measurement. Every null below renders as a
// word, and notRead says which of the two reasons applies to a transcript.
const (
	unknown = "unknown"
	notRead = "not read"
	none    = "none"
)

// TextOption configures the terminal rendering.
//
// Variadic for the same reason Build's options are: a caller that has no
// opinion about the chain view keeps compiling and keeps getting the summary.
type TextOption func(*textOptions)

type textOptions struct {
	chain    bool
	timeline bool
}

// WithChain expands the causal view from a count into the per-call listing.
//
// Off by default because it is the only section whose LENGTH GROWS WITH THE
// SESSION -- one line per tool call, so a long day's work buries a fixed-size
// report that a reader opens to see coverage and findings. The count is always
// shown, so the view can never be invisible; the flag decides whether it is
// expanded, not whether it exists. JSON always carries the whole structure,
// because that reader is a program and is not scrolling.
func WithChain() TextOption {
	return func(o *textOptions) { o.chain = true }
}

// WithTimeline adds the timeline: every call, main agent and subagents, on
// one list in seq order. Off by default for the reason WithChain is: it is one
// line per call. JSON always carries it.
func WithTimeline() TextOption {
	return func(o *textOptions) { o.timeline = true }
}

// Text renders a report for a terminal.
//
// No record has a field that carries a command line or a tool response, and
// nothing is printed beyond the store's fields except what is read from two
// other places when the report is rendered: the transcript -- tool-use ids and
// counts for the accounting, and the agent's final message, which --redact
// drops whole. Two things ARE taken from a command line: the program
// name -- the base name of the word in command position -- which `by program`
// prints for every session, and hostnames and ssh destinations (shape.Hosts),
// which the destinations section prints and --redact digests.
func Text(w io.Writer, rep *Report, opts ...TextOption) error {
	var cfg textOptions
	for _, o := range opts {
		o(&cfg)
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "rashomon report -- generated %s\n", stamp(rep.GeneratedAtUnixMS))

	// The legend goes in the REDACTED render only, and near the top, because
	// this is the copy that leaves the machine and its reader has no README.
	if rep.Redacted {
		fmt.Fprintln(&b, "hostnames are redacted: <digest>.<last label>, an HMAC under this "+
			"install's own key.")
		fmt.Fprintln(&b, "Equal hosts give equal digests here and DIFFERENT digests on another "+
			"machine, so a")
		fmt.Fprintln(&b, "reader without that key cannot test a guess. Eight hex characters is "+
			"32 bits: two")
		fmt.Fprintln(&b, "hosts can collide, the last label is kept in clear, and anyone who "+
			"can read this")
		fmt.Fprintln(&b, "install's store can compute these. The agent's summary is dropped "+
			"whole, not cleaned.")
	}
	if len(rep.Sessions) == 0 {
		writeHarnessUnknownIfSet(&b, rep.HarnessUnknownSessions)
		fmt.Fprintln(&b, "\nno sessions recorded")
		_, err := w.Write(b.Bytes())
		return err
	}
	writeStoreWide(&b, rep)
	for _, sess := range rep.Sessions {
		writeSession(&b, sess, cfg)
	}
	_, err := w.Write(b.Bytes())
	return err
}

func writeSession(b *bytes.Buffer, sess Session, cfg textOptions) {
	fmt.Fprintf(b, "\nsession %s\n", sess.SessionID)
	fmt.Fprintf(b, "  install: %s\n", orUnknown(sess.InstallID))
	// The account-versus-record block first, then destinations, then the
	// recorder's own accounting. The comparison is what the reader came for;
	// coverage and the declaration counts are how far it can be trusted, and
	// they read better after the finding than before it.
	writeAccount(b, sess.Account)
	writeSubagents(b, sess.Subagents)
	if sess.SessionID == store.UnattributedSession {
		// No call that arrives without a session id records an outcome
		// (hook.Post.Capture), so a failure count over this run is not zero:
		// it is unknown, and "0" here would be a clean zero over nothing.
		fmt.Fprintf(b, "  failed calls: %s (calls without a session id carry no outcome)\n", unknown)
	} else {
		writeSilentFailures(b, sess.SilentFailures)
	}
	if sess.MaskedRuns != nil {
		writeMaskedRuns(b, *sess.MaskedRuns)
	}
	writeTestRuns(b, sess.TestRuns, sess.SessionID)
	writeDestinations(b, sess.Destinations)
	writeFamilies(b, sess.Families)
	writeNono(b, sess.Nono)
	writeChains(b, sess.Chains, cfg.chain)
	if cfg.timeline {
		writeTimeline(b, sess.Timeline)
	}
	fmt.Fprintf(b, "  coverage: %s\n", sess.Coverage.State)
	writeReasons(b, sess.Coverage.Reasons)
	fmt.Fprintf(b, "  start recorded: %s\n", yesNo(sess.Coverage.StartRecorded))
	fmt.Fprintf(b, "  end recorded: %s\n", yesNo(sess.Coverage.EndRecorded))
	fmt.Fprintf(b, "  hook entry at start: %s\n", sess.Coverage.HookEntryAtStart)
	fmt.Fprintf(b, "  hook entry at end: %s\n", sess.Coverage.HookEntryAtEnd)

	d := sess.Declarations
	fmt.Fprintf(b, "  declarations recorded: %d\n", d.Recorded)
	fmt.Fprintf(b, "  declarations without a transcript path: %d\n", d.WithoutTranscript)
	fmt.Fprintf(b, "  by tool: %s\n", byName(d.ByTool))
	// What the session DID, as far as a recorder that keeps no content can
	// say: which programs ran and what kind of thing each call was. "by tool:
	// Bash 23" is the door, not the work.
	fmt.Fprintf(b, "  by verb: %s\n", byName(d.ByVerbClass))
	fmt.Fprintf(b, "  by program: %s\n", programs(d.ByProgram, d.ProgramsUnknown))
	fmt.Fprintf(b, "  by label: %s\n", byName(d.ByLabel))
	fmt.Fprintf(b, "  unterminated: %s\n", list(d.Unterminated))
	fmt.Fprintf(b, "  dropped: %s\n  possibly not durable: %s\n", list(d.Dropped), list(d.NotDurable))
	fmt.Fprintf(b, "  without execution: %s\n", unexecuted(d.WithoutExecution))
	fmt.Fprintf(b, "  executions recorded: %d\n", sess.Executions.Recorded)

	for _, t := range sess.Transcripts {
		fmt.Fprintf(b, "  transcript %s\n", t.Path)
		fmt.Fprintf(b, "    readable: %s\n", yesNo(t.Readable))
		fmt.Fprintf(b, "    status: %s\n", t.Status)
		fmt.Fprintf(b, "    files: %s\n", count(t.Files))
		fmt.Fprintf(b, "    lines too long to read: %s\n", count(t.LinesTooLong))
		fmt.Fprintf(b, "    ids in transcript: %s\n", count(t.IDsInTranscript))
		fmt.Fprintf(b, "    ids recorded: %d\n", t.IDsRecorded)
		fmt.Fprintf(b, "    results in transcript: %s\n", count(t.ResultsInTranscript))
		fmt.Fprintf(b, "    ids executed: %d\n", t.IDsExecuted)
		fmt.Fprintf(b, "    missing from store: %s\n", set(t.MissingFromStore))
		fmt.Fprintf(b, "    missing from transcript: %s\n", set(t.MissingFromTranscript))
		// Not a gap, and not dropped: refused before a hook could see it, and
		// named so the transcript's ids still add up against the store's.
		fmt.Fprintf(b, "    refused before any hook: %s\n", set(t.RefusedBeforeHooks))
		fmt.Fprintf(b, "    executed but unrecorded: %s\n",
			overlapping(t.ExecutedButUnrecorded, t.MissingFromStore, "missing from store"))
		// Between the two lists it sits between, and named rather than folded
		// into either: a denial is not a recording failure and not a call
		// waiting on its result. It is the permission prompt working.
		fmt.Fprintf(b, "    denied before running: %s\n", set(t.DeniedByUser))
		fmt.Fprintf(b, "    declared without result: %s\n", set(t.DeclaredWithoutResult))
	}

	if len(sess.Gaps) == 0 {
		fmt.Fprintf(b, "  gaps: %s\n", none)
	} else {
		fmt.Fprintf(b, "  gaps: %d\n", len(sess.Gaps))
		for _, g := range sess.Gaps {
			// A paused window removed nothing -- there was nothing to
			// remove -- so "N records removed" would be true and misleading
			// at once. It gets its own line: not a clean count, not an
			// absence, but a named, bounded reason nothing was recorded.
			if g.Reason == store.GapPaused {
				fmt.Fprintf(b, "    %s: not recorded, deliberately, %s to %s\n",
					g.Reason, stamp(g.FromUnixMS), stamp(g.ToUnixMS))
				continue
			}
			// Eviction removes a run it could not read by size, and then
			// does not know how many records went (store.RemovedUnknown).
			if g.RemovedRecords < 0 {
				fmt.Fprintf(b, "    %s: an unknown number of records removed, covering %s to %s\n",
					g.Reason, stamp(g.FromUnixMS), stamp(g.ToUnixMS))
				continue
			}
			fmt.Fprintf(b, "    %s: %d records removed, covering %s to %s\n",
				g.Reason, g.RemovedRecords, stamp(g.FromUnixMS), stamp(g.ToUnixMS))
		}
	}
	fmt.Fprintf(b, "  skipped records: %d\n", sess.SkippedRecords)
}

// plural is the "s" for a count, for the messages that interpolate a number.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// count renders a count that may not exist. A transcript that could not be read
// has no counts, and "not read" is the answer rather than a number.
func count(p *int) string {
	if p == nil {
		return notRead
	}
	return strconv.Itoa(*p)
}

// set renders a list that may not exist, which is a different thing from one
// that is empty: the first is a comparison that could not be made, the second
// is a comparison that found nothing.
func set(ids []string) string {
	if ids == nil {
		return unknown
	}
	return list(ids)
}

// reasonText explains a coverage reason in one line.
//
// The codes are the contract and stay on the line, because a reader who greps
// or a consumer reading JSON needs them. The sentence beside each is for the
// person who has never read this source.
//
// Reported from a real first run: a report carrying "probe_absent,
// run_not_closed, transcript_mismatch, execution_mismatch" and eight hundred
// tool-use ids, where all four codes had a single cause -- the recorder was
// installed in the middle of a session that had not finished -- and the
// report said none of it. A reader cannot act on a vocabulary they have to go
// and look up.
var reasonText = map[string]string{
	"probe_absent":           "a session-start record or marker is missing: the recorder was installed mid-session, its start hook did not run, failed or ran while paused, or the size cap evicted the run",
	"probe_unresolved":       "the session-start probe could not be read, so the start of this session is unaccounted for",
	"run_not_closed":         "no session-end was recorded: the session is still open, it ended without one, or its end hook ran while `rashomon pause` was in effect",
	"records_unreadable":     "some records could not be read (damaged, an unaccepted schema version, or an unknown type), so nothing they held is counted, or a coverage record was unverified with no reason or carried a state this build does not know",
	"transcript_mismatch":    "the transcript and this store disagree about which tool calls were made; see the two `missing from` lines below",
	"transcript_unreadable":  "a transcript this run's calls named exists but could not be read, so the check that every call in it was recorded did not run",
	"execution_mismatch":     "the transcript holds results for calls this store recorded no execution for",
	"gap":                    "a gap record marks a stretch this store does not hold: records removed by `forget` or the size cap, or a window `rashomon pause` left unrecorded",
	"recording_paused":       "a hook ran while `rashomon pause` was in effect, so what it would have recorded (a tool call, its result, or the session's start or end) was deliberately not recorded",
	"pause_state_unreadable": "a hook found the pause marker present but could not read it, so it did not record, but the gap is a filesystem fault rather than a deliberate `rashomon pause`",
	"duplicate_declarations": "more declarations than distinct tool calls: more than one recorder wrote into this run, and the counts here include every copy",
	"declaration_missing":    "a call's terminal record is here and its declaration is not, and the lock did not refuse it: the declaration was lost to a damaged line or a failed write",
	"internal_error":         "a hook invocation failed inside this program, so what it should have recorded is missing",
	"recovered_panic":        "a hook invocation hit a bug in this program and recovered from it, so what it should have recorded is missing, or a file label reads unknown",
	"lock_timeout":           "a hook could not take the store lock in time, so its record went to the spill file or was lost",
	"payload_too_large":      "a hook was handed a payload over the 8 MiB intake limit and refused it without parsing, so that call was not recorded",
	"record_oversize":        "a value in a hook's payload was over its size limit and was left out of the record, or the whole record was over the record limit and was not written, so what was recorded for that call is incomplete",
	"flush_failed":           "a hook could not flush its records to stable storage before it exited, so they are possibly not durable: a power loss or a crash of the machine could still remove them",
	"terminated_by_signal":   "a hook was killed by a signal before it finished",
	"unterminated_entry":     "a declaration was never closed, so the call's end was not observed",
	"hook_entry_absent":      "the settings file did not hold the recorder's entry for this event as watch installs it, and this hook was not running as a rashomon plugin that declares one",
	"hook_entry_unresolved":  "the settings file or the plugin could not be read, so whether the recorder was installed is unknown",

	// The record scope (1.2q): a fault that left content withheld, not a gap.
	"record_scope_unresolved": "the session start could not set the record scope from consent.json, so this session recorded without paths or command text whatever the consent on file says; `rashomon status` shows the record scope",
}

// writeReasons renders the coverage reasons, one per line with its meaning.
func writeReasons(b *bytes.Buffer, reasons []string) {
	if len(reasons) == 0 {
		fmt.Fprintf(b, "  reasons: %s\n", none)
		return
	}
	fmt.Fprintln(b, "  reasons:")
	for _, r := range reasons {
		if text, ok := reasonText[r]; ok {
			fmt.Fprintf(b, "    %-22s %s\n", r, text)
			continue
		}
		// A reason with no sentence still renders. The vocabulary cannot reach
		// this line -- TestReasonTextCoversTheVocabulary holds the map to it --
		// so what does is a reason read off disk exactly as stored: one written
		// by a newer or older build, or a hand-edited store. It is printed
		// verbatim, because build does not clamp it to the vocabulary.
		fmt.Fprintf(b, "    %s\n", r)
	}
}

// overlapping renders a list that may repeat one printed just above it.
//
// These two lists are the same set whenever the recorder was installed after
// the calls ran: every id in the transcript but not in the store also has a
// result the store recorded no execution for. In a real report that meant the
// same eight hundred and fifty-eight ids printed twice, four hundred
// characters for one fact, and a reader who has to compare two walls of
// opaque ids to notice they are identical.
//
// It names the set above, never says its ids are "listed" there: that list
// is shortened past a dozen ids, and "the same 858, listed under ... above"
// once sat beneath a line showing twelve of them.
//
// It states the overlap and NOT its cause. "The recorder was installed after
// these ran" is an inference; the reasons block above already carries
// probe_absent with its sentence, and this line is not the place to guess at
// a second explanation.
func overlapping(items, printed []string, where string) string {
	if len(items) == 0 || len(printed) == 0 {
		return set(items)
	}
	above := make(map[string]bool, len(printed))
	for _, id := range printed {
		above[id] = true
	}
	for _, id := range items {
		if !above[id] {
			// Not a subset: the reader needs the list itself.
			return set(items)
		}
	}
	if len(items) == len(printed) {
		return fmt.Sprintf("the same %d as %q above", len(items), where)
	}
	return fmt.Sprintf("%d, all of them among %q above", len(items), where)
}

// listWidth is how many characters of a list a terminal line will carry
// before it summarises, and listMax how many items.
//
// Two bounds because this function renders two very different things: a
// handful of short words, which should all fit, and hundreds of opaque
// tool-use ids, which must not. Bounding only the count would still emit a
// thousand characters of ids; bounding only the width would cut a list of
// short words mid-thought.
const (
	listWidth = 200
	listMax   = 12
)

// list renders a list for a terminal, summarising one too long to read.
//
// The summary names NO type. This function is called with ids, with tool
// families and with ordinary English words, and an earlier version that said
// "N ids" labelled all three as ids -- which was simply false for two of them.
//
// Nothing is dropped: the JSON carries every item and the line says so.
// Rendering a bare count instead would lose them, which is the thing the
// accounting equation exists to avoid.
//
// Reported from a real first run, where eight hundred and sixty ids joined
// into a single line of roughly twenty-five thousand characters and buried
// every other line in the report.
func list(items []string) string {
	if len(items) == 0 {
		return none
	}

	shown, width := 0, 0
	for _, it := range items {
		next := width + len(it) + 2
		if shown == listMax || (shown > 0 && next > listWidth) {
			break
		}
		width, shown = next, shown+1
	}
	if shown == len(items) {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more of %d (--json lists them all)",
		strings.Join(items[:shown], ", "), len(items)-shown, len(items))
}

// unexecuted renders the declarations no execution answers, each with the
// permission mode it was declared in -- through list(), for its bound. Every
// call that arrives without a session id lands here, because its execution
// is never recorded, so this is the line that grows with another harness's
// whole working day. The JSON carries every id.
func unexecuted(items []Unexecuted) string {
	if len(items) == 0 {
		return none
	}
	out := make([]string, len(items))
	for i, u := range items {
		out[i] = u.ToolUseID + " (" + orUnknown(u.PermissionMode) + ")"
	}
	return list(out)
}

func byName(counts map[string]int) string {
	if len(counts) == 0 {
		return none
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]string, len(names))
	for i, name := range names {
		// A tool name is the hook payload's, so it is made printable here as
		// on the timeline: a control byte in it would reach the terminal.
		out[i] = fmt.Sprintf("%s %d", printable(name), counts[name])
	}
	// Through list, for its bound: a session that ran sixty programs or
	// called forty MCP tools must not rebuild the line list was written to
	// retire. The JSON carries every name.
	return list(out)
}

// programs renders the per-program counts, with the shell calls whose
// program could not be told reported beside them rather than among them.
//
// Beside, not among: "unknown" is not a program, and a reader scanning the
// list for what ran must not meet it sitting between `git` and `go` as though
// it were one more command.
//
// untold counts the Bash calls whose program could not be found: a first word
// cut off by an unterminated quote, a line with no word in command position,
// one that begins with something the program search does not parse, or an
// input with no command string. So it is rendered as "could not be told"
// throughout, never as "named none", which is true of only some of those.
func programs(counts map[string]int, untold int) string {
	if len(counts) == 0 && untold == 0 {
		return none
	}
	if len(counts) == 0 {
		return fmt.Sprintf("%s (%d shell call(s) whose program could not be told)", none, untold)
	}
	out := byName(counts)
	if untold == 0 {
		return out
	}
	return fmt.Sprintf("%s (and %d shell call(s) whose program could not be told)", out, untold)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// orUnknown covers a field the records never supplied. A run with no coverage
// record names no install; the empty string is not an install id and must not
// render as one.
func orUnknown(s string) string {
	if s == "" {
		return unknown
	}
	return s
}

func stamp(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// writeAccount renders the agent's own summary.
//
// It is quoted, on its own line, and marked when truncated, so a reader can see
// that they are looking at the agent's words rather than the tool's. An
// unreadable transcript renders "unknown" and not an empty quotation: the
// second would read as an agent that said nothing.
func writeAccount(b *bytes.Buffer, a Account) {
	if !a.Available {
		fmt.Fprintf(b, "  the agent's account: %s (no assistant message could be read)\n", unknown)
		return
	}
	suffix := ""
	if a.Truncated {
		fmt.Fprintf(b, "  the agent's account (first %d characters):\n", accountLimit)
		suffix = "..."
	} else {
		fmt.Fprintln(b, "  the agent's account:")
	}
	// Newlines are collapsed so a multi-line summary cannot be mistaken for
	// the report's own structure.
	fmt.Fprintf(b, "    %q%s\n", collapse(a.Text), suffix)
}

// writeSubagents renders what the main transcript never shows.
func writeSubagents(b *bytes.Buffer, subs []SubagentSummary) {
	if len(subs) == 0 {
		fmt.Fprintln(b, "  subagents: none")
		return
	}
	fmt.Fprintf(b, "  subagents: %d\n", len(subs))
	for _, s := range subs {
		fmt.Fprintf(b, "    %s (%s): %d declarations, %d executions, %d Bash\n",
			s.AgentID, orUnknown(s.AgentType), s.Declarations, s.Executions, s.BashCalls)
	}
	fmt.Fprintln(b, "    (these calls do not appear in the main transcript)")
}

// writeSilentFailures renders the failure count beside the summary.
//
// The wording is a fact about text and stops there. It says how many calls
// failed and which acknowledgement words are absent; it does not say the agent
// concealed anything, because this program cannot know that. A test greps this
// package for the words that would cross that line.
func writeSilentFailures(b *bytes.Buffer, sf SilentFailures) {
	if sf.Unobserved > 0 {
		fmt.Fprintf(b, "  outcome unobserved: %d call(s) recorded no ending or were moved to the background before they ended\n", sf.Unobserved)
	}
	if sf.Failed == 0 {
		fmt.Fprintln(b, "  failed calls: 0")
		return
	}
	fmt.Fprintf(b, "  failed calls: %d\n", sf.Failed)
	if sf.FailedLookups > 0 {
		fmt.Fprintf(b, "    lookups among them: %d (%s, whatever the error: a directory, a missing file or folder, a bad pattern), not set against the final message\n", sf.FailedLookups, lookupToolNames)
	}
	if sf.Counted() == 0 {
		return
	}
	if !sf.FinalMessageAvailable {
		fmt.Fprintln(b, "    the final message could not be read, so it was not compared")
		return
	}
	if !sf.Fires {
		fmt.Fprintln(b, "    the final message uses at least one failure word")
		return
	}
	fmt.Fprintf(b, "    the final message contains none of these %d words: %s\n",
		len(sf.AbsentWords), list(sf.AbsentWords))
}

// writeMaskedRuns renders the build and test calls whose exit status their
// line masked and that no later plain run followed up, beside the failed
// calls: nothing when there were none. The count is of calls, and the line
// says what it rests on: the line's ok, not the runner's.
func writeMaskedRuns(b *bytes.Buffer, m MaskedRuns) {
	if m.Runs == 0 {
		return
	}
	fmt.Fprintf(b, "  masked exit status: %d build or test call(s) recorded ok while their line did not return the runner's exit status, and no later run matched to the same runner in the same directory returned it\n", m.Runs)
	if m.UnreadLocation > 0 {
		fmt.Fprintf(b, "    %d of these ran after a leading `cd` step the hook does not read (`cd DIR;`, a variable, quoting or `-`), so the directory they ran in is not known; a later run is matched only under the same unread step\n", m.UnreadLocation)
	}
	switch {
	case !m.FinalMessageAvailable:
		fmt.Fprintln(b, "    the final message could not be read, so it was not compared")
	case m.Fires:
		fmt.Fprintln(b, "    the final message claims a pass and uses no failure word")
	case !m.PassClaimed:
		fmt.Fprintln(b, "    the final message claims no pass")
	default:
		fmt.Fprintln(b, "    the final message uses at least one failure word")
	}
}

// writeTestRuns renders the session's test runs and the two test-bending
// patterns among them.
//
// Absent when the session recorded no test run, unlike the sections that are
// always present, and when its records predate the test class (t is nil):
// those say execute or package for the same commands, so a "test runs: 0"
// there would state a count the record never measured.
//
// The limit is printed wherever a pattern is: "no recorded file edit between"
// means no recorded call between the runs that may change files (mayEdit:
// every call but a read, a network call, a subagent launch or one of Claude
// Code's bookkeeping tools). A shell command classed read or network counts
// when its shape says it may write (`cat a > b`, `curl -o f`, `find
// -delete`, `grep | xargs sed -i`, rsync), and the block names what sets that;
// one that writes through an option the list does not name (`find -fprint
// f`, `curl -D f`, `curl -c f`) is not counted, and the block says so. Saying
// "nothing changed" would be the claim the record cannot make. Runs pair only
// within one directory (the declarations' cwd digests), and the block says
// so, since "the same command" alone would read as the same line anywhere.
// And a `cd DIR && go test` is a test run, since leaving `cd … &&` out would
// lose most real runs, so its outcome may be the cd's. A repeated relative
// `cd DIR && go test` needs no line: the second starts in DIR, another
// directory, and does not pair with the first. Each is a line of the block. A
// run moved to the background, on its timeout, by Ctrl+B or by
// run_in_background, is not a limit any more: its execution record says so
// (on Ctrl+B only if Claude Code marks it with backgroundTaskId or
// backgroundedByUser, which was not measured), and it is no run with a result
// (LinkOutcomeBackgrounded).
//
// A session holding a call whose declaration was lost has no pair looked
// for (detectTestBending), and the block says so on a line of its own: with
// the limit lines printed only beside a pair, the run count alone would read
// as "looked and found none", and a pair the end-of-turn line named earlier
// would be gone with no word of why.
//
// The pairs are seqs, and nothing in the default report maps a seq to its
// call, so the block's first limit line points at --timeline, which does,
// with the session's id so the command can be pasted: single-quoted
// (pasteArg) when the shell would split or unquote it.
func writeTestRuns(b *bytes.Buffer, t *TestRuns, sessionID string) {
	if t == nil || t.Runs == 0 {
		return
	}
	masked := ""
	if t.Masked != nil && *t.Masked > 0 {
		masked = fmt.Sprintf(", %d whose line did not return the test run's exit status, so neither", *t.Masked)
	}
	fmt.Fprintf(b, "  test runs: %d (%d ok, %d failed%s)\n", t.Runs, t.OK, t.Failed, masked)
	if t.Undeclared > 0 {
		fmt.Fprintf(b, "    no pair is looked for: %d call%s ran with no declaration recorded, and may have edited a file between any two runs\n",
			t.Undeclared, plural(t.Undeclared))
	}
	for _, p := range t.TestsOnlyThenGreen {
		fmt.Fprintf(b, "    failed, then the only recorded edits were to files named like tests, then the same command passed: %d → %d\n", p[0], p[1])
	}
	// Each pair with its runs' outcomes in declaration (start) order, which
	// is not always passed then failed, and is not always the order the two
	// finished in when parallel agents overlap.
	for _, p := range t.Flaky {
		first, second := p.Outcomes()
		fmt.Fprintf(b, "    same command had both outcomes with no recorded file edit between: %d %s, %d %s\n",
			p.Seqs[0], first, p.Seqs[1], second)
	}
	if len(t.TestsOnlyThenGreen)+len(t.Flaky) > 0 {
		fmt.Fprintf(b, "    the numbers are call seqs, and `rashomon report --session %s --timeline` shows these rows;\n", pasteArg(sessionID))
		fmt.Fprintln(b, "    a file edit here is any recorded call but a read, a web fetch, a subagent launch, or a task, todo, question, plan, background-shell, skill, search, message, cron or MCP-resource tool, even one that failed;")
		fmt.Fprintln(b, "    a shell read or fetch counts when its line may write: a redirect to a file, a download (curl -o, attached or not), a command or process substitution, find -delete or -exec, xargs, tee, rsync or scp, or a later stage that is not a read,")
		fmt.Fprintln(b, "    but one that writes through an option not on that list (find -fprint, curl -D or -c) is not counted;")
		fmt.Fprintln(b, "    runs pair only when the same command line started in the same directory: the reported cwd, or where its leading plain cd steps lead;")
		fmt.Fprintln(b, "    and a runner behind `cd DIR &&` is a test run, so a cd that failed reads as a failed run")
	}
}

// pasteArg is s as one shell word: bare when it is not empty and every byte
// is in [A-Za-z0-9._-], and otherwise single-quoted, with a quote inside
// closed, escaped and reopened, as internal/install's shellQuote does. Its
// safe set is narrower than shellQuote's on purpose, so the pasted id is a
// single word under any shell.
func pasteArg(s string) string {
	if s == "" {
		return "''"
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '.', c == '_', c == '-':
		default:
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// collapse turns a multi-line message into one line. The report's own
// structure is line-based, so an agent's newline would otherwise look like a
// field of the report.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// writeFamilies renders which network-capable tool families ran and how many
// hosts each named. The families that did not run are counted and named on
// one line rather than one line each: ten "not exercised" lines would bury
// the ones that ran.
func writeFamilies(b *bytes.Buffer, fc FamilyCoverage) {
	if len(fc.Families) == 0 {
		fmt.Fprintln(b, "  tool families: none of the known families ran in this session")
	} else {
		fmt.Fprintln(b, "  tool families:")
		for _, f := range fc.Families {
			fmt.Fprintf(b, "    %s (%s): %d call(s), %d declared host(s)\n",
				f.Name, strings.Join(f.Programs, " "), f.Calls, f.HostsDeclared)
		}
	}
	if len(fc.NotExercised) > 0 {
		fmt.Fprintf(b, "    not exercised: %s\n", list(fc.NotExercised))
	}
}

// writeRewritten names the calls behind the count above.
//
// THE WORDING DEPENDS ON THE TOOL, and that is correctness rather than style.
// shape.Derive digests the whole canonicalised tool_input for every tool
// EXCEPT Bash, where it digests the command. So for an Edit or a Write, a
// reworded `description` -- which changes nothing about what the call does to
// the file -- moves the digest and lands here. Calling that "the command
// changed" would be false twice: there is no command, and what changed may not
// affect the effect at all.
//
// Neither line says WHAT changed, because nothing here knows. Both inputs are
// gone; only their digests were ever kept.
func writeRewritten(b *bytes.Buffer, rows []Rewritten) {
	for _, r := range rows {
		what := "input changed"
		if r.ToolName == "Bash" {
			what = "command changed"
		}
		shape := r.VerbClass
		if r.Program != "" {
			shape = printable(r.Program) + ", " + r.VerbClass
		}
		// Every name here is the payload's: made printable, as on the
		// timeline, or an escape sequence in one reaches the terminal.
		fmt.Fprintf(b, "    %s  %s (%s): %s between declaration and execution\n",
			printable(r.ToolUseID), printable(r.ToolName), shape, what)
	}
}

// writeNono renders what the sandbox saw.
//
// ALWAYS PRINTS A LINE, including when no trail was configured. A section that
// vanishes when it has nothing to say cannot be told apart from one that was
// never built -- the rule this package states for family coverage and has now
// broken twice.
func writeNono(b *bytes.Buffer, n Nono) {
	if !n.Configured {
		// SILENT, not "not observed". A sandbox nobody asked for is not a
		// degraded observation -- it is a feature that was not in use, and
		// H-28 is right that printing a degradation marker on a healthy run
		// teaches readers to discount the word.
		return
	}
	if !n.Observed {
		// The skipped count is said here too, but ONLY when it is complete.
		// nono_audit_no_records means the reader reached end of file, so
		// Skipped counts the whole trail. nono_audit_unreadable means the
		// scan stopped at the line that broke it (an oversized line, a read
		// error): Skipped there counts only the torn lines BEFORE that
		// point, an undercount of however much of the trail follows. Two
		// torn lines followed by one line over the scanner's cap used to
		// print "2 trail records could not be read" regardless of how much
		// trail came after the point the read gave up.
		skipped := ""
		if n.Reason == nono.NotObservedNoRecords && n.Skipped > 0 {
			skipped = fmt.Sprintf("; %d trail record%s could not be read", n.Skipped, plural(n.Skipped))
		}
		fmt.Fprintf(b, "  sandbox (nono): not observed (%s%s)\n", orUnknown(n.Reason), skipped)
		return
	}
	if n.NetworkEventsInTrail == 0 {
		// Observed is true -- the trail held a session_started and a
		// session_ended record -- but the sandbox never wrote a single
		// network decision. That is a different fact from a session the
		// sandbox filtered and saw nothing on: it happens whenever the
		// active profile does not filter the network at all. Rendering it
		// as "0 allowed, 0 denied" reads as a quiet, filtered session when
		// no filtering ran -- silence read as zero, the failure this
		// package exists to prevent. Follow-up from #53, applied here.
		//
		// NOT A RETURN. This used to return right after the line below,
		// which hid the skipped/unparseable, sessions and inherited lines
		// below on exactly the trails where they are most informative: a
		// schema-drift record (an event type like log_denied) or a torn
		// line next to a session_started record both land in Skipped and
		// never touch NetworkEventsInTrail, so a reader got a confident
		// "the profile may not filter the network" on a trail this reader
		// actually failed to read. When Skipped > 0, unreadable records are
		// named as a possible cause alongside the profile, not displaced by
		// it.
		cause := "the active profile may not filter the network"
		if n.Skipped > 0 {
			cause = "the active profile may not filter the network, or the events could not be read"
		}
		fmt.Fprintf(b, "  sandbox (nono): the trail holds no network events (%s; this is "+
			"not the same as a filtered session that saw nothing)\n", cause)
	} else {
		// An open window is said ON THIS LINE: the two counts are only as
		// bounded as the window, and with no end record they run to the end
		// of the trail, where a later session's traffic on a shared trail
		// counts as this one's.
		open := ""
		if n.WindowOpen {
			open = " (no end record, so events up to the end of the trail are counted)"
		}
		fmt.Fprintf(b, "  sandbox (nono): %d allowed, %d denied in this session's window%s\n",
			len(n.Allowed), len(n.Denied), open)
		if len(n.ClientPlaneAllowed)+len(n.ClientPlaneDenied) > 0 {
			// Named, not dropped: the sandbox did allow or refuse these, and a
			// reader comparing against nono's own log should find every host and
			// what the sandbox did with it. Not counted above, because these are
			// hosts the client contacts on its own and no tool call of the session
			// named them; one that a call named is counted above, as the agent's.
			var said []string
			if len(n.ClientPlaneAllowed) > 0 {
				said = append(said, "allowed "+list(n.ClientPlaneAllowed))
			}
			if len(n.ClientPlaneDenied) > 0 {
				said = append(said, "refused "+list(n.ClientPlaneDenied))
			}
			fmt.Fprintf(b, "    the client's own traffic, not counted above: %s\n", strings.Join(said, "; "))
		}
	}
	// SAID SEPARATELY, because it is a LIFETIME count beside two windowed ones.
	// nono's session_started record carries an ISO string and no millisecond
	// instant, so there is nothing to window it on -- and joining all three in
	// one sentence rendered "0 allowed, 0 denied, across 1 session", which
	// reads as "a sandbox session ran and did nothing".
	fmt.Fprintf(b, "    the trail holds %d sandbox session%s in total (not "+
		"window-filtered: nono's session record carries no millisecond instant)\n",
		n.Sessions, plural(n.Sessions))
	if len(n.Denied) > 0 {
		// Named, because a denial is the sandbox doing its job and it is
		// evidence about the AGENT: it tried to go somewhere it could not.
		fmt.Fprintf(b, "    refused by the sandbox: %s\n", list(n.Denied))
	}
	if n.Skipped > 0 || n.UnparseableTargets > 0 {
		fmt.Fprintf(b, "    %d trail record%s could not be read, %d target%s could not be "+
			"parsed\n", n.Skipped, plural(n.Skipped),
			n.UnparseableTargets, plural(n.UnparseableTargets))
	}
	if n.UnknownModes > 0 {
		fmt.Fprintf(b, "    %d event%s carried a transport this reader does not know\n",
			n.UnknownModes, plural(n.UnknownModes))
	}
	if n.UnknownDecisions > 0 {
		verb := "were"
		if n.UnknownDecisions == 1 {
			verb = "was"
		}
		fmt.Fprintf(b, "    %d sandbox event%s carried a decision this reader does not "+
			"know, and %s counted in neither column\n",
			n.UnknownDecisions, plural(n.UnknownDecisions), verb)
	}
	if n.Inherited > 0 {
		fmt.Fprintf(b, "    %d sandbox event%s outside this session's window, excluded\n",
			n.Inherited, plural(n.Inherited))
	}
}
