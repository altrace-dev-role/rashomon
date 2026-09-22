package report

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

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

type textOptions struct{ chain bool }

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

// Text renders a report for a terminal.
//
// No record has a field that carries a command line or a tool response, and
// nothing is printed beyond the store's fields except what is read from two
// other places when the report is rendered: the transcript -- tool-use ids and
// counts for the accounting, and the agent's final message, which --redact
// drops whole -- and, where an observing proxy's database is found, the hosts
// it recorded, which the destinations section prints and --redact digests. Two things ARE taken from a command line: the program
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
		fmt.Fprintln(&b, "\nno sessions recorded")
		_, err := w.Write(b.Bytes())
		return err
	}
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
	writeSilentFailures(b, sess.SilentFailures)
	writeDestinations(b, sess.Destinations)
	writeFamilies(b, sess.Families)
	writeNono(b, sess.Nono)
	writeChains(b, sess.Chains, cfg.chain)
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
	fmt.Fprintf(b, "  dropped: %s\n", list(d.Dropped))
	fmt.Fprintf(b, "  without execution: %s\n", unexecuted(d.WithoutExecution))
	fmt.Fprintf(b, "  executions recorded: %d\n", sess.Executions.Recorded)

	for _, t := range sess.Transcripts {
		fmt.Fprintf(b, "  transcript %s\n", t.Path)
		fmt.Fprintf(b, "    readable: %s\n", yesNo(t.Readable))
		fmt.Fprintf(b, "    files: %s\n", count(t.Files))
		fmt.Fprintf(b, "    ids in transcript: %s\n", count(t.IDsInTranscript))
		fmt.Fprintf(b, "    ids recorded: %d\n", t.IDsRecorded)
		fmt.Fprintf(b, "    results in transcript: %s\n", count(t.ResultsInTranscript))
		fmt.Fprintf(b, "    ids executed: %d\n", t.IDsExecuted)
		fmt.Fprintf(b, "    missing from store: %s\n", set(t.MissingFromStore))
		fmt.Fprintf(b, "    missing from transcript: %s\n", set(t.MissingFromTranscript))
		fmt.Fprintf(b, "    executed but unrecorded: %s\n",
			overlapping(t.ExecutedButUnrecorded, t.MissingFromStore, "missing from store"))
		// Between the two lists it sits between, and named rather than folded
		// into either: a denial is not a recording failure and not a call
		// waiting on its result. It is the permission prompt working.
		fmt.Fprintf(b, "    denied by user: %s\n", set(t.DeniedByUser))
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
			fmt.Fprintf(b, "    %s: %d records removed, covering %s to %s\n",
				g.Reason, g.RemovedRecords, stamp(g.FromUnixMS), stamp(g.ToUnixMS))
		}
	}
	fmt.Fprintf(b, "  skipped records: %d\n", sess.SkippedRecords)
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
	"records_unreadable":     "some records could not be read (damaged, an unaccepted schema version, or an unknown type), so nothing they held is counted",
	"transcript_mismatch":    "the transcript and this store disagree about which tool calls were made; see the two `missing from` lines below",
	"execution_mismatch":     "the transcript holds results for calls this store recorded no execution for",
	"gap":                    "a gap record marks a stretch this store does not hold: records removed by `forget` or the size cap, or a window `rashomon pause` left unrecorded",
	"recording_paused":       "a hook ran while `rashomon pause` was in effect, so what it would have recorded (a tool call, its result, or the session's start or end) was deliberately not recorded",
	"duplicate_declarations": "more declarations than distinct tool calls: more than one recorder wrote into this run, and the counts here include every copy",
	"internal_error":         "a hook invocation failed inside this program, so what it should have recorded is missing",
	"lock_timeout":           "a hook could not take the store lock in time, so its record went to the spill file or was lost",
	"terminated_by_signal":   "a hook was killed by a signal before it finished",
	"unterminated_entry":     "a declaration was never closed, so the call's end was not observed",
	"hook_entry_absent":      "neither the settings file nor the plugin declared the recorder's entry when the hook ran",
	"hook_entry_unresolved":  "the settings file or the plugin could not be read, so whether the recorder was installed is unknown",
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

func unexecuted(items []Unexecuted) string {
	if len(items) == 0 {
		return none
	}
	out := make([]string, len(items))
	for i, u := range items {
		out[i] = u.ToolUseID + " (" + orUnknown(u.PermissionMode) + ")"
	}
	return strings.Join(out, ", ")
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
		out[i] = fmt.Sprintf("%s %d", name, counts[name])
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

// writeDestinations renders what the wire saw beside what was declared.
//
// The degraded case prints a REASON and never an empty list. "destinations:
// not observed (no_proxy_store)" and "destinations: none" are different
// statements, and printing the second when the first is true is the single
// failure this whole product exists to avoid: silence read as zero.
func writeDestinations(b *bytes.Buffer, d Destinations) {
	// BEFORE the not-observed return, because this count compares two records
	// the recorder wrote itself and has nothing to do with the proxy. Printing
	// it only when the wire was readable would hide a rewritten call precisely
	// on the sessions where the proxy was not running.
	//
	// Always rendered, including the zero: "executed differently from declared:
	// 0" tells a reader the comparison ran, where a line that appeared only when
	// non-zero would leave them unable to tell a clean session from an unchecked
	// one -- the same silence-as-zero error in a different field.
	fmt.Fprintf(b, "  executed differently from declared: %d\n", d.ExecutedNotAsDeclared)
	writeRewritten(b, d.Rewritten)

	if !d.Observed {
		fmt.Fprintf(b, "  destinations: not observed (%s)\n", d.Reason)
		fmt.Fprintf(b, "  proxy on path: %s -- %s\n", d.ProxyOnPath, d.ProxyNote)
		// The novelty line prints here too. A section that vanishes when it has
		// nothing to say cannot be told apart from one that was never built --
		// the rule this package states for family coverage and broke for
		// novelty, on the most common path of all.
		writeNovelty(b, d.Novelty)
		return
	}

	fmt.Fprintf(b, "  destinations: %d distinct, %d attempt%s\n",
		d.DistinctHosts, d.Attempts, plural(d.Attempts))
	fmt.Fprintf(b, "  proxy on path: %s -- %s\n", d.ProxyOnPath, d.ProxyNote)
	writeJoin(b, d)
	if !d.WindowApplied {
		// Stated whenever it is true. A window that was not enforced means
		// another session's destinations may be in this list, and a reader who
		// is not told that will attribute them to this session.
		fmt.Fprintln(b, "  window: NOT applied (the proxy's timestamps could not be parsed);"+
			" rows from other sessions may be included")
	}
	if d.Suppressed > 0 {
		fmt.Fprintf(b, "  suppressed: %d destination(s) removed from this view by "+
			"forget --host (the proxy's own records are not deleted)\n", d.Suppressed)
	}
	if d.Inherited > 0 {
		// WHY they were excluded, not just that they were. This line used to
		// say "from outside this session's window" unconditionally, which the
		// token join made false: a row carrying another run's tag is excluded
		// while sitting squarely INSIDE the window, and telling a reader to go
		// looking outside it would send them after a session that does not
		// exist. The two reasons are different facts and now read differently.
		reason := "from outside this session's window"
		switch {
		case d.OtherToken > 0 && d.OtherToken == d.Inherited:
			reason = "carrying another session's token"
		case d.OtherToken > 0:
			reason = fmt.Sprintf("from outside this session's window or carrying "+
				"another session's token (%d of them)", d.OtherToken)
		}
		fmt.Fprintf(b, "  inherited: %d attempt%s %s, excluded from the counts above\n",
			d.Inherited, plural(d.Inherited), reason)
		if d.InheritedAllClientPlane {
			// Says which previous session to go looking for: none. This is the
			// client's own tunnel, opened before the first hook ran.
			fmt.Fprintln(b, "    (client-plane traffic before the session's first hook)")
		}
	}

	// The finding.
	if len(d.WireOnly) == 0 {
		fmt.Fprintln(b, "  reached but never named: none")
	} else {
		fmt.Fprintf(b, "  reached but never named: %s\n", list(d.WireOnly))
		fmt.Fprintln(b, "    (these hosts appear in no tool call's declared hosts;"+
			" no transcript or hook log can produce this line)")
	}

	if len(d.ClientPlane) > 0 {
		fmt.Fprintf(b, "  client plane: %s\n", list(d.ClientPlane))
		fmt.Fprintln(b, "    (the client's own model traffic transits the same proxy;"+
			" an agent request to the same host is indistinguishable)")
	}

	// The comparison's other direction, rendered with its own weakness stated.
	// Naming the four causes is not hedging: without them a reader takes the
	// line as "the agent claimed a host it never used", which is one of four
	// possibilities and the only accusatory one.
	if len(d.DeclaredNotObserved) == 0 {
		fmt.Fprintln(b, "  declared but not observed: none")
	} else {
		fmt.Fprintf(b, "  declared but not observed: %s\n", list(d.DeclaredNotObserved))
		fmt.Fprintln(b, "    (a call that was denied, that failed before connecting,"+
			" that was served from a cache, or a host the proxy did not see)")
	}

	writeNovelty(b, d.Novelty)

	// Rendered under the destinations block rather than beside the findings,
	// because these are limitations of the instrument and not facts about the
	// session. A reader scanning for what the agent did should not meet them
	// among the findings at all.
	if len(d.NotObservable) > 0 {
		fmt.Fprintf(b, "  not observable (ssh/git, outside what a CONNECT proxy sees): %s\n",
			list(d.NotObservable))
	}

	for _, h := range d.Hosts {
		suffix := ""
		if h.Inherited {
			suffix = " [inherited]"
		}
		if h.Unreached {
			suffix += " [allowed, never reached]"
		}
		fmt.Fprintf(b, "    %s: %d attempt(s)%s\n", h.Host, h.Attempts, suffix)
	}
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
		fmt.Fprintf(b, "  outcome unobserved: %d call(s) recorded no ending\n", sf.Unobserved)
	}
	if sf.Failed == 0 {
		fmt.Fprintln(b, "  failed calls: 0")
		return
	}
	fmt.Fprintf(b, "  failed calls: %d\n", sf.Failed)
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

// collapse turns a multi-line message into one line. The report's own
// structure is line-based, so an agent's newline would otherwise look like a
// field of the report.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// writeNovelty renders the per-project first-seen line.
//
// Three distinct states, and collapsing any two of them would make the line
// worthless: the baseline was just created (nothing to be novel against), the
// baseline exists and nothing was new, or the baseline could not be read. Only
// the middle one is "no new hosts"; printing that for either of the others is
// the silence-as-zero error again, in the field most likely to be skimmed.
func writeNovelty(b *bytes.Buffer, n Novelty) {
	switch {
	case !n.Available:
		fmt.Fprintf(b, "  new for this project: %s (%s)\n", unknown, n.Reason)
	case n.Established:
		// The count says which hosts it counted. The destinations are listed
		// below this line, and the client plane is excluded from novelty on
		// purpose, so a bare count reads as disagreeing with the list.
		fmt.Fprintf(b, "  new for this project: baseline established "+
			"(%d host%s, client plane excluded)\n", n.KnownHosts, plural(n.KnownHosts))
	case len(n.Hosts) == 0:
		fmt.Fprintf(b, "  new for this project: none (%d known)\n", n.KnownHosts)
	default:
		fmt.Fprintf(b, "  new for this project: %s\n", list(n.Hosts))
		fmt.Fprintf(b, "    (first seen in this project by this session; %d hosts known)\n", n.KnownHosts)
	}
}

// writeFamilies renders which tool families were confirmed to transit.
//
// The not-observable list prints on EVERY report, including a completely
// healthy one. These are properties of the instrument rather than of the
// session, and a reader told only what was observed will read the rest as an
// absence of traffic rather than an absence of observation.
func writeFamilies(b *bytes.Buffer, fc FamilyCoverage) {
	if !fc.Available {
		fmt.Fprintf(b, "  tool families: %s (%s)\n", unknown, fc.Reason)
	} else if len(fc.Families) == 0 {
		fmt.Fprintln(b, "  tool families: none of the known families ran in this session")
	} else {
		fmt.Fprintln(b, "  tool families:")
		for _, f := range fc.Families {
			fmt.Fprintf(b, "    %s (%s): %d call(s), %d/%d declared hosts observed -- %s\n",
				f.Name, strings.Join(f.Programs, " "), f.Calls,
				f.HostsObserved, f.HostsDeclared, f.Status)
		}
	}
	if len(fc.NotExercised) > 0 {
		// Counted and named on one line rather than one line each: ten
		// "not exercised" lines would bury the families that did run.
		fmt.Fprintf(b, "    not exercised: %s\n", list(fc.NotExercised))
	}
	fmt.Fprintln(b, "  not observable, whatever the session did:")
	for _, n := range fc.NotObservable {
		fmt.Fprintf(b, "    %s\n", n)
	}
}

// writeChains renders the causal spine: which prompt produced which calls.
//
// THE LEGEND IS NOT DECORATION. A host on a link reads as though that call
// reached it, and it does not mean that: the proxy's store carries no
// tool_use_id, so no wire row can be attributed to an individual call. The
// state is that host's state across the session's window, sitting next to the
// call that named it. That is a genuinely useful join and a genuinely easy
// misreading, and the misreading overstates what is known -- so the line saying
// so is printed every time the section is, not once in the documentation.
func writeChains(b *bytes.Buffer, c Chains, expand bool) {
	extra := len(c.Unattributed) + len(c.Dropped)
	if len(c.Prompts) == 0 && extra == 0 {
		return
	}
	fmt.Fprintf(b, "  chains: %d\n", len(c.Prompts))

	// The COUNT is unconditional and the listing is not. This section is the
	// only one whose length grows with the session -- one line per tool call --
	// so on a long day it buries a report whose other sections are fixed size
	// and which a reader opens for coverage and findings. Hiding it entirely
	// behind a flag would be the opposite error: a view nobody knows exists is
	// the same as one that was never built.
	if !expand {
		if len(c.Prompts) > 0 {
			fmt.Fprintf(b, "    --chain lists the calls under each prompt\n")
		}
		writeChainTail(b, c, false)
		return
	}

	if len(c.Prompts) > 0 {
		fmt.Fprintf(b, "    a host's state is that host's across this session, "+
			"not proof this call reached it\n")
	}

	var lastPath string
	for _, ch := range c.Prompts {
		if ch.TranscriptPath != lastPath {
			fmt.Fprintf(b, "    %s\n", ch.TranscriptPath)
			lastPath = ch.TranscriptPath
		}
		fmt.Fprintf(b, "    prompt %s\n", ch.PromptID)
		for _, l := range ch.Links {
			writeLink(b, l)
		}
	}
	writeChainTail(b, c, true)
}

// writeChainTail renders the two groups that belong to no prompt.
//
// Both are announced whether or not the listing is expanded, because both are
// statements about COMPLETENESS -- how much of the session the chains above do
// not account for -- and a reader deciding whether to trust the view needs that
// without having to ask for more output.
func writeChainTail(b *bytes.Buffer, c Chains, expand bool) {
	if n := len(c.Unattributed); n > 0 {
		fmt.Fprintf(b, "    %d call%s could not be placed under a prompt "+
			"(prompt not recorded)\n", n, plural(n))
		if expand {
			for _, l := range c.Unattributed {
				writeLink(b, l)
			}
		}
	}
	if n := len(c.Dropped); n > 0 {
		// The declaration never landed, so the id is the whole of what is
		// known. Named anyway: this is a call the session made and cannot
		// describe, which is worth more to a reader than a tidy omission.
		fmt.Fprintf(b, "    %d call%s ran with no declaration recorded\n", n, plural(n))
		if expand {
			for _, l := range c.Dropped {
				fmt.Fprintf(b, "      %s  everything but the id is unknown\n", l.ToolUseID)
			}
		}
	}
}

func writeLink(b *bytes.Buffer, l Link) {
	shape := l.VerbClass
	if l.Program != "" {
		shape = l.Program + ", " + l.VerbClass
	}
	outcome := l.Outcome
	// Two post records for one id. The headline is the higher-seq one and this
	// says the other existed, because a link that showed only the winner would
	// hide precisely the disagreement worth seeing.
	if l.ExecutionRecords > 1 {
		outcome = fmt.Sprintf("%s (%d records: %s)", l.Outcome,
			l.ExecutionRecords, strings.Join(l.Outcomes, ", "))
	}
	fmt.Fprintf(b, "      %d  %s (%s)  %s%s%s\n",
		l.Seq, l.ToolName, shape, outcome, linkHosts(l.Hosts), linkSSH(l.SSHHosts))
}

// linkSSH renders the ssh hosts a call named, kept apart from the observable
// ones and carrying no state: the proxy cannot see ssh, so there is nothing to
// report about them and a verdict column would be answering a question the wire
// was never able to be asked.
func linkSSH(hosts []string) string {
	if len(hosts) == 0 {
		return ""
	}
	return "  ssh: " + strings.Join(hosts, ", ") + " (not observable)"
}

// linkHosts renders the hosts a call named, or nothing at all when it named
// none -- which is most calls, and a trailing "->" on every one of them would
// bury the ones that matter.
func linkHosts(hosts []LinkHost) string {
	if len(hosts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(hosts))
	for _, h := range hosts {
		parts = append(parts, h.Host+" "+h.State)
	}
	return "  -> " + strings.Join(parts, ", ")
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
			shape = r.Program + ", " + r.VerbClass
		}
		fmt.Fprintf(b, "    %s  %s (%s): %s between declaration and execution\n",
			r.ToolUseID, r.ToolName, shape, what)
	}
}

// writeJoin says how this session's rows were attributed, and never as one
// word.
//
// "join: token" over a set that is half window-matched is a claim of precision
// the data does not support, and the mixture is not the exception -- it is what
// every real tokened session looks like, because not every client carries the
// credential.
//
// THE CLIENT FAMILIES ARE NAMED, with the measurement, because a reader who
// sees some rows joined by the clock will reasonably wonder whether the token
// is broken. It is not: git strips proxy userinfo and expects a credential
// helper. Saying which client and how it was established turns a suspicious
// number into a known limit.
func writeJoin(b *bytes.Buffer, d Destinations) {
	if d.TokenMatched == 0 && d.WindowMatched == 0 && d.OtherToken == 0 {
		return
	}
	switch {
	case d.TokenMatched == 0 && !d.TokenRequested:
		// No tag was in play. Said plainly rather than dressed up as a join.
		fmt.Fprintf(b, "  join: window (%d request%s); no session token was in use\n",
			d.WindowMatched, plural(d.WindowMatched))
	case d.TokenMatched == 0:
		// A tag WAS in play and no row carried it. This is a diagnostic, not a
		// quiet fallback: either the proxy is not writing run_id, or every
		// client stripped the credential. Inferring "no token was in use" from
		// the zero -- as this did -- reported the failure as its own opposite.
		fmt.Fprintf(b, "  join: window (%d request%s); a session token was in use and NO row "+
			"carried it -- the proxy may not be recording it\n",
			d.WindowMatched, plural(d.WindowMatched))
	default:
		fmt.Fprintf(b, "  join: token (%d request%s) + window (%d request%s)\n",
			d.TokenMatched, plural(d.TokenMatched),
			d.WindowMatched, plural(d.WindowMatched))
		if d.WindowMatched > 0 {
			fmt.Fprintln(b, "    some clients do not send the proxy credential and are "+
				"attributed by time alone:")
			fmt.Fprintln(b, "    git (measured: no Proxy-Authorization on CONNECT, via "+
				"HTTPS_PROXY or git -c http.proxy)")
		}
	}
	if d.OtherToken > 0 {
		// The overlap is visible WITHOUT those rows entering this session's
		// numbers. Before tokens an operator could not know another session was
		// active at all; now the report can say so and still refuse to count it.
		fmt.Fprintf(b, "  other sessions in this window: %d request%s, "+
			"identified by their own token and excluded from the counts above\n",
			d.OtherToken, plural(d.OtherToken))
	}
}

// writeNono renders what the sandbox saw, and what it disagrees with.
//
// ALWAYS PRINTS A LINE, including when no trail was configured. A section that
// vanishes when it has nothing to say cannot be told apart from one that was
// never built -- the rule this package states for family coverage and has now
// broken twice.
//
// THE DISAGREEMENT IS NOT PRESENTED AS A FAULT. A host the sandbox saw and the
// proxy did not is usually plain HTTP, which rashomon structurally cannot see
// because it exports no HTTP_PROXY. Rendering that as "the proxy missed it"
// would turn a documented boundary into an accusation against the recorder --
// the same error as calling loopback "not observed".
func writeNono(b *bytes.Buffer, n Nono) {
	if !n.Configured {
		// SILENT, not "not observed". A sandbox nobody asked for is not a
		// degraded observation -- it is a feature that was not in use, and
		// H-28 is right that printing a degradation marker on a healthy run
		// teaches readers to discount the word.
		return
	}
	if !n.Observed {
		fmt.Fprintf(b, "  sandbox (nono): not observed (%s)\n", orUnknown(n.Reason))
		return
	}
	fmt.Fprintf(b, "  sandbox (nono): %d allowed, %d denied in this session's window\n",
		len(n.Allowed), len(n.Denied))
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
	if len(n.SawWhatTheProxyDidNot) > 0 {
		fmt.Fprintf(b, "    seen by the sandbox and not on the wire: %s\n",
			list(n.SawWhatTheProxyDidNot))
		if len(n.PlainHTTP) > 0 {
			// NAMED, not counted. A bare count beside a list invites the reader
			// to conclude the whole list is benign when the numbers happen to
			// match, and leaves the unexplained host -- the only one that
			// mattered -- unidentifiable.
			fmt.Fprintf(b, "      plain HTTP, which this proxy does not observe (a known "+
				"boundary, not a recording gap): %s\n", list(n.PlainHTTP))
		}
	}
	if len(n.ProxySawWhatItDidNot) > 0 {
		fmt.Fprintf(b, "    on the wire and not in the sandbox's trail: %s\n",
			list(n.ProxySawWhatItDidNot))
	}
	if len(n.DeniedButReached) > 0 {
		// LOUDEST LINE IN THE SECTION. The sandbox refused it and the wire
		// recorded reaching it anyway: traffic that escaped the sandbox.
		fmt.Fprintf(b, "    REFUSED BY THE SANDBOX AND REACHED ANYWAY: %s\n",
			list(n.DeniedButReached))
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
		fmt.Fprintf(b, "    %d sandbox event%s carried a decision this reader does not "+
			"know, and were counted in neither column\n",
			n.UnknownDecisions, plural(n.UnknownDecisions))
	}
	if n.Inherited > 0 {
		fmt.Fprintf(b, "    %d sandbox event%s outside this session's window, excluded\n",
			n.Inherited, plural(n.Inherited))
	}
}
