package recap

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The AutoSteer clause (card REPORT1a): one line under the Stop's state
// sentence counting what the record shows of the session's notes, findings
// and checks. Every slot is a count or a seq from the record's own lists
// (Impact); the words are fixed, and none is one of FUN-15's (TestFUN15b
// scans this file with value.go and impact.go).

// maxClauseSeqs is how many resolving seqs the clause names before it counts
// the rest; the report lists them all.
const maxClauseSeqs = 2

// clause is the AutoSteer clause for im, "" with nothing to count. It takes
// the fullest form that fits the continuation line under lineColumns, and
// the shortest when none does: detail goes before a count does.
func clause(im Impact) string {
	if im.Empty() {
		return ""
	}
	var last string
	for level := 0; level <= 3; level++ {
		last = "AutoSteer: " + clauseAt(im, level) + "."
		if columns(continuation()+last) <= lineColumns {
			return last
		}
	}
	return last
}

// clauseAt is the clause's body at a level of detail: 0 names the one note's title,
// its <when> and the resolving seqs, 1 drops the title and <when>, 2 drops "given", 3 drops
// the seqs too.
func clauseAt(im Impact, level int) string {
	var parts []string
	if n := len(im.Delivered); n > 0 {
		parts = append(parts, notesPart(im.Delivered, level))
	}
	if n := len(im.Findings); n > 0 {
		parts = append(parts, findingsPart(im, level))
	}
	if n := len(im.Checks); n > 0 {
		met, unknown := im.Met()
		s := "checks " + strconv.Itoa(met) + " of " + strconv.Itoa(n) + " met"
		if unknown > 0 {
			s += ", " + strconv.Itoa(unknown) + " unknown"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// ClauseBody is the clause's counts at the fullest detail, without its lead
// and full stop, "" with nothing to count: the report's first screen says it
// in the clause's words, so the two agree.
func ClauseBody(im Impact) string {
	if im.Empty() {
		return ""
	}
	return clauseAt(im, 0)
}

// TitleOf is the title a delivered note of pattern carries in the clause,
// "note" for a pattern with none.
func TitleOf(pattern string) string {
	if t := titles()[pattern]; t != "" {
		return t
	}
	return "note"
}

// StateSentence is the Stop view's state sentence as the value line prints
// it, "" for a view that prints nothing: the report's last-Stop line.
func StateSentence(v verify.StopView) string {
	if v.Summary.Row == verify.RowNone {
		return ""
	}
	return fitted(v, prefix)
}

// notesPart is the delivered count: with one note at level 0 its title, with
// every note at one event that event's <when> (D78, D87), which the events
// with no phrase (a session start) leave out.
func notesPart(notes []NoteRef, level int) string {
	n := len(notes)
	if level >= 2 {
		return strconv.Itoa(n) + " " + noun(n, "note", "notes")
	}
	s := strconv.Itoa(n) + " " + noun(n, "note", "notes") + " given"
	if level == 1 {
		return s
	}
	if n == 1 {
		title := titles()[notes[0].Pattern]
		if title == "" {
			title = "note"
		}
		s = "1 " + title + " given"
	}
	same := true
	for _, x := range notes {
		same = same && x.Event == notes[0].Event
	}
	if when := whens()[notes[0].Event]; same && when != "" {
		s += " " + when
	}
	return s
}

// findingsPart is the findings count with the resolved ones' seqs, and the
// open ones counted: a finding whose line names no gate is counted and in
// neither list, since no run can settle it.
func findingsPart(im Impact, level int) string {
	n := len(im.Findings)
	s := strconv.Itoa(n) + " " + noun(n, "finding", "findings")
	resolved, open := im.Resolved(), 0
	for _, f := range im.Findings {
		if f.State == FindingOpen {
			open++
		}
	}
	if len(resolved) > 0 {
		s += ", " + resolvedWords(resolved, n == 1, level)
	}
	if open > 0 {
		s += ", " + strconv.Itoa(open) + " not resolved"
	}
	return s
}

// resolvedWords is `resolved at #41`, or for several `2 resolved at #41, #45`
// and `and 1 more` past maxClauseSeqs; without seqs at level 2 the count.
func resolvedWords(resolved []FindingRef, only bool, level int) string {
	lead := strconv.Itoa(len(resolved)) + " resolved"
	if only {
		lead = "resolved"
	}
	if level >= 3 {
		return lead
	}
	var at []string
	for i, f := range resolved {
		if i == maxClauseSeqs {
			at = append(at, "and "+strconv.Itoa(len(resolved)-maxClauseSeqs)+" more")
			break
		}
		at = append(at, "#"+strconv.FormatInt(f.ResolvedBy, 10))
	}
	return lead + " at " + strings.Join(at, ", ")
}

func noun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
