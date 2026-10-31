package report

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The first screen and the autosteer block (card REPORT1a). The report was
// the tables; a reader came for the story and had to find it in them. So a
// session opens with a few lines that say what the record shows of the last
// Stop, the calls, the notes the agent was given and the gates, each one the
// number or the sentence a table below gives, and then the tables. Every
// number here is a count of records with their seqs beside it, and a count
// the report did not take reads unknown or not read, never 0.

// maxStoryGates is how many gates the story names before `and <k> more`.
const maxStoryGates = 4

// maxListedSeqs is how many seqs a line lists before `and <k> more`; --json
// lists them all.
const maxListedSeqs = 6

// writeStory is the session's first screen: what it says of the last Stop,
// the record of calls, the AutoSteer counts, the gates, and the one finding
// of the account when the report sets the final message against a failure.
func writeStory(b *bytes.Buffer, sess Session) {
	if sess.SessionID == store.UnattributedSession {
		return
	}
	fmt.Fprintln(b, "  story")
	a := sess.Autosteer
	switch {
	case a == nil:
		fmt.Fprintf(b, "    last Stop: not read; `rashomon report --session %s` reads it\n", pasteArg(sess.SessionID))
	case a.Stop == nil:
		fmt.Fprintln(b, "    last Stop: none recorded for this session")
	case a.Stop.Sentence == "":
		fmt.Fprintf(b, "    last Stop: call %d; it printed no state line\n", a.Stop.Seq)
	default:
		fmt.Fprintf(b, "    last Stop: %s (call %d)\n", a.Stop.Sentence, a.Stop.Seq)
	}
	fmt.Fprintf(b, "    record: %d executions; %s\n", sess.Executions.Recorded, callOutcomes(sess))
	if a != nil && a.Stop != nil {
		if a.Clause == "" {
			fmt.Fprintln(b, "    autosteer: no notes, findings or checks to count")
		} else {
			fmt.Fprintf(b, "    autosteer: %s\n", a.Clause)
		}
		fmt.Fprintf(b, "    gates: %s\n", storyGates(a.Gates))
	}
	if sf := sess.SilentFailures; sf.Fires {
		fmt.Fprintf(b, "    account: the final message uses none of the %d failure words, and %d call%s failed\n",
			len(sf.AbsentWords), sf.Counted(), plural(sf.Counted()))
	}
	fmt.Fprintln(b, "    details below: the account, then the tables; the numbers are call seqs")
}

// callOutcomes is the failed-call count and the test runs of the session in
// one clause: the two numbers the tables below spell out.
func callOutcomes(sess Session) string {
	s := strconv.Itoa(sess.SilentFailures.Failed) + " failed call" + plural(sess.SilentFailures.Failed)
	if t := sess.TestRuns; t != nil && t.Runs > 0 {
		s += fmt.Sprintf("; %d test run%s (%d ok, %d failed)", t.Runs, plural(t.Runs), t.OK, t.Failed)
	}
	return s
}

// storyGates names each gate with its state and the seq that state rests on:
// the last green run for a green gate, the last run otherwise.
func storyGates(gates []AutosteerGate) string {
	if len(gates) == 0 {
		return "none known for this project"
	}
	var parts []string
	for i, g := range gates {
		if i == maxStoryGates {
			parts = append(parts, fmt.Sprintf("and %d more", len(gates)-maxStoryGates))
			break
		}
		s := g.Name + " " + strings.ReplaceAll(g.State, "_", " ")
		if at := gateSeq(g); at != nil {
			s += " at #" + strconv.FormatInt(*at, 10)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

func gateSeq(g AutosteerGate) *int64 {
	if g.State == "green" && g.LastGreenSeq != nil {
		return g.LastGreenSeq
	}
	return g.LastRunSeq
}

// writeAutosteer is the autosteer block, after the test runs: nothing when
// the report did not read it or the session has no Stop, and otherwise
// delivered, findings, checks, lessons and unknown, each with the seqs the
// counts rest on.
func writeAutosteer(b *bytes.Buffer, a *Autosteer) {
	if a == nil {
		return
	}
	if a.Stop == nil {
		// The story says so ("last Stop: none recorded"); a block of zeros
		// here would read as counts taken.
		return
	}
	fmt.Fprintln(b, "  autosteer")
	fmt.Fprintf(b, "    delivered: %s\n", deliveredText(a.Delivered))
	writeFindings(b, a.Findings)
	fmt.Fprintf(b, "    checks: %s\n", checksText(a.Checks))
	fmt.Fprintf(b, "    lessons: %s\n", lessonsText(a))
	fmt.Fprintf(b, "    unknown: %s\n", unknownText(a.Unknown))
}

// deliveredText is `3 notes (2 task checks, 1 known fix) at #3, #9 with
// prompts p1, p2`: the count, the titles by count in first-seen order, and
// the seqs and prompts the notes carry.
func deliveredText(notes []AutosteerNote) string {
	if len(notes) == 0 {
		return "none"
	}
	var titles []string
	by := map[string]int{}
	var seqs []int64
	var prompts []string
	for _, n := range notes {
		if by[n.Title] == 0 {
			titles = append(titles, n.Title)
		}
		by[n.Title]++
		if n.Seq > 0 {
			seqs = append(seqs, n.Seq)
		}
		if n.PromptID != "" && !slices.Contains(prompts, n.PromptID) {
			prompts = append(prompts, n.PromptID)
		}
	}
	kinds := make([]string, len(titles))
	for i, t := range titles {
		kinds[i] = strconv.Itoa(by[t]) + " " + pluralTitle(t, by[t])
	}
	s := fmt.Sprintf("%d note%s (%s)", len(notes), plural(len(notes)), strings.Join(kinds, ", "))
	if len(seqs) > 0 {
		s += " at " + seqList(seqs)
	}
	if len(prompts) > 0 {
		s += " with prompt" + plural(len(prompts)) + " " + strings.Join(prompts, ", ")
	}
	return s
}

// pluralTitle is a title in number: the titles are noun phrases, and `es`
// follows the ones that end in a sibilant (a known fix, a task match).
func pluralTitle(title string, n int) string {
	switch {
	case n == 1:
		return title
	case strings.HasSuffix(title, "x"), strings.HasSuffix(title, "s"), strings.HasSuffix(title, "ch"):
		return title + "es"
	}
	return title + "s"
}

// seqList is `#3, #9` and `and 2 more` past maxListedSeqs.
func seqList(seqs []int64) string {
	var parts []string
	for i, s := range seqs {
		if i == maxListedSeqs {
			parts = append(parts, fmt.Sprintf("and %d more", len(seqs)-maxListedSeqs))
			break
		}
		parts = append(parts, "#"+strconv.FormatInt(s, 10))
	}
	return strings.Join(parts, ", ")
}

// writeFindings is the findings line: with one finding the count and the
// finding on a line, and with more the count and a line each.
func writeFindings(b *bytes.Buffer, fs []AutosteerFinding) {
	switch len(fs) {
	case 0:
		fmt.Fprintln(b, "    findings: none")
	case 1:
		fmt.Fprintf(b, "    findings: 1 %s\n", findingText(fs[0]))
	default:
		fmt.Fprintf(b, "    findings: %d\n", len(fs))
		for _, f := range fs {
			fmt.Fprintf(b, "      %s\n", findingText(f))
		}
	}
}

// findingText is one finding: its title and seq, the gate it names, and
// whether a green run of that gate came after it.
func findingText(f AutosteerFinding) string {
	s := f.Title + " at #" + strconv.FormatInt(f.Seq, 10)
	if f.Gate != "" {
		s += " (" + f.Gate + ")"
	}
	switch f.State {
	case "resolved":
		return s + ", resolved by #" + strconv.FormatInt(f.ResolvedBy, 10) + " green"
	case "open":
		return s + ", not resolved: no green run of it after"
	}
	return s + ", no gate named, so no run settles it"
}

// checksText is each expected evidence row with its state and the seq or the
// reason behind it: met at the gate's green run, unmet in the record's words,
// unknown and open as they are.
func checksText(cs []AutosteerCheck) string {
	if len(cs) == 0 {
		return "none expected"
	}
	parts := make([]string, len(cs))
	for i, c := range cs {
		s := c.Kind + " " + c.State
		switch {
		case c.State == "met" && c.Seq > 0:
			s += " at #" + strconv.FormatInt(c.Seq, 10)
		case c.State == "unmet" && c.Reason != "":
			s += " (" + c.Reason + ")"
		case c.State == "unknown":
			s += " (no readable state)"
		}
		parts[i] = s
	}
	return strings.Join(parts, "; ")
}

// lessonsText is the ledger's counts by tier and the lessons delivered this
// session, or not read when the report opened no ledger. The ledger is opened
// read-only, so one that does not check is named, not quarantined.
func lessonsText(a *Autosteer) string {
	if a.Lessons == nil {
		return "not read here"
	}
	if a.Lessons.Unchecked {
		return fmt.Sprintf("the ledger did not check, so all %d read proposed (this project)", a.Lessons.Proposed)
	}
	s := fmt.Sprintf("%d approved, %d proposed, %d demoted (this project)", a.Lessons.Approved, a.Lessons.Proposed, a.Lessons.Demoted)
	var fired []int64
	for _, n := range a.Delivered {
		if n.Pattern == "learned-fact" {
			fired = append(fired, n.Seq)
		}
	}
	if len(fired) == 0 {
		return s + "; none fired this session"
	}
	return s + fmt.Sprintf("; %d fired this session", len(fired))
}

// unknownText is what the record cannot settle: masked runs with their calls
// and expected checks with no readable state.
func unknownText(us []AutosteerUnknown) string {
	if len(us) == 0 {
		return "none"
	}
	var masked []int64
	checks := 0
	for _, u := range us {
		if u.Kind == "masked run" {
			masked = append(masked, u.Seq)
		} else {
			checks++
		}
	}
	var parts []string
	if len(masked) > 0 {
		parts = append(parts, fmt.Sprintf("%d masked run%s (%s: exit status not read)", len(masked), plural(len(masked)), seqList(masked)))
	}
	if checks > 0 {
		parts = append(parts, fmt.Sprintf("%d expected check%s with no readable state", checks, plural(checks)))
	}
	return strings.Join(parts, "; ")
}
