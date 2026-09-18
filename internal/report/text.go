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
	// Destinations first, before the recorder's own accounting. The comparison
	// is what the reader came for; the coverage and declaration counts are how
	// far it can be trusted, and they read better after the finding than
	// before it.
	writeDestinations(b, sess.Destinations)
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
	if !d.Observed {
		fmt.Fprintf(b, "  destinations: not observed (%s)\n", d.Reason)
		fmt.Fprintf(b, "  proxy on path: %s -- %s\n", d.ProxyOnPath, d.ProxyNote)
		return
	}

	fmt.Fprintf(b, "  destinations: %d distinct, %d attempts\n", d.DistinctHosts, d.Attempts)
	fmt.Fprintf(b, "  proxy on path: %s -- %s\n", d.ProxyOnPath, d.ProxyNote)
	if !d.WindowApplied {
		// Stated whenever it is true. A window that was not enforced means
		// another session's destinations may be in this list, and a reader who
		// is not told that will attribute them to this session.
		fmt.Fprintln(b, "  window: NOT applied (the proxy's timestamps could not be parsed);"+
			" rows from other sessions may be included")
	}
	if d.Inherited > 0 {
		fmt.Fprintf(b, "  inherited: %d attempts from outside this session's window, excluded from the counts above\n", d.Inherited)
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
