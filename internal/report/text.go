package report

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
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

// Text renders a report for a terminal.
//
// Nothing beyond the store's own fields is printed: ids, tool names, transcript
// paths, counts and reason codes. There is no field here that could carry a
// command line or a tool response, because there is no such field in the
// records this reads.
func Text(w io.Writer, rep *Report) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "rashomon report -- generated %s\n", stamp(rep.GeneratedAtUnixMS))
	if len(rep.Sessions) == 0 {
		fmt.Fprintln(&b, "\nno sessions recorded")
		_, err := w.Write(b.Bytes())
		return err
	}
	for _, sess := range rep.Sessions {
		writeSession(&b, sess)
	}
	_, err := w.Write(b.Bytes())
	return err
}

func writeSession(b *bytes.Buffer, sess Session) {
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
	fmt.Fprintf(b, "  coverage: %s\n", sess.Coverage.State)
	fmt.Fprintf(b, "  reasons: %s\n", list(sess.Coverage.Reasons))
	fmt.Fprintf(b, "  start recorded: %s\n", yesNo(sess.Coverage.StartRecorded))
	fmt.Fprintf(b, "  end recorded: %s\n", yesNo(sess.Coverage.EndRecorded))
	fmt.Fprintf(b, "  hook entry at start: %s\n", sess.Coverage.HookEntryAtStart)
	fmt.Fprintf(b, "  hook entry at end: %s\n", sess.Coverage.HookEntryAtEnd)

	d := sess.Declarations
	fmt.Fprintf(b, "  declarations recorded: %d\n", d.Recorded)
	fmt.Fprintf(b, "  declarations without a transcript path: %d\n", d.WithoutTranscript)
	fmt.Fprintf(b, "  by tool: %s\n", byTool(d.ByTool))
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
		fmt.Fprintf(b, "    executed but unrecorded: %s\n", set(t.ExecutedButUnrecorded))
		fmt.Fprintf(b, "    declared without result: %s\n", set(t.DeclaredWithoutResult))
	}

	if len(sess.Gaps) == 0 {
		fmt.Fprintf(b, "  gaps: %s\n", none)
	} else {
		fmt.Fprintf(b, "  gaps: %d\n", len(sess.Gaps))
		for _, g := range sess.Gaps {
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

func list(items []string) string {
	if len(items) == 0 {
		return none
	}
	return strings.Join(items, ", ")
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

func byTool(counts map[string]int) string {
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
	return strings.Join(out, ", ")
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
		fmt.Fprintf(b, "  inherited: %d attempt%s from outside this session's window, "+
			"excluded from the counts above\n", d.Inherited, plural(d.Inherited))
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
