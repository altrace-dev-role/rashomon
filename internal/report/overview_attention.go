package report

import (
	"fmt"
	"strconv"
	"strings"
)

// The finding states recap writes (recap.FindingResolved, FindingOpen and
// FindingUnchecked); report does not import recap, which imports report.
const (
	ovResolved  = "resolved"
	ovOpen      = "open"
	ovUnchecked = "unchecked"
)

// ovFindings is the findings by state, with the seqs of those not resolved.
// A state recap does not write counts as other, never as resolved.
type ovFindings struct {
	n, resolved, open, unchecked, other int
	unresolved                          []int64
}

func ovCountFindings(fs []AutosteerFinding) ovFindings {
	f := ovFindings{n: len(fs)}
	for _, x := range fs {
		switch x.State {
		case ovResolved:
			f.resolved++
			continue
		case ovOpen:
			f.open++
		case ovUnchecked:
			f.unchecked++
		default:
			f.other++
		}
		if x.Seq > 0 {
			f.unresolved = append(f.unresolved, x.Seq)
		}
	}
	return f
}

// lead is the sentence on the findings after the notes, or "".
func (f ovFindings) lead() string {
	switch un := f.n - f.resolved; {
	case f.n == 0:
		return ""
	case un == 0 && f.n == 1:
		return " The 1 finding was resolved."
	case un == 0:
		return fmt.Sprintf(" The %d findings were resolved.", f.n)
	case f.n == 1:
		return " The 1 finding was not resolved."
	case f.resolved == 0:
		return fmt.Sprintf(" None of the %d findings was resolved.", f.n)
	default:
		return fmt.Sprintf(" %d of the %d findings %s not resolved.", un, f.n, wasWere(un))
	}
}

// outcome is the mark and the words of the Outcome verified row.
func (f ovFindings) outcome() (string, string) {
	switch {
	case f.n == 0:
		return "–", "No finding was recorded, so there was nothing to settle."
	case f.resolved == f.n:
		return "✓", fmt.Sprintf("%d of %d resolved by a green run of its gate.", f.n, f.n)
	case f.unchecked == f.n && f.n == 1:
		return "!", "0 of 1. The 1 finding names no gate, so no run settles it."
	case f.unchecked == f.n:
		return "!", fmt.Sprintf("0 of %d. None of the %d findings names a gate, so no run settles them.", f.n, f.n)
	}
	// A result row: what was settled, what is open, what no run can settle.
	parts := []string{fmt.Sprintf("%d of %d resolved by a green run of its gate", f.resolved, f.n)}
	if f.open > 0 {
		parts = append(parts, fmt.Sprintf("%d open: no green run of the gate came after", f.open))
	}
	if f.unchecked > 0 {
		parts = append(parts, f.noGate())
	}
	if f.other > 0 {
		parts = append(parts, f.otherState())
	}
	return "!", strings.Join(parts, "; ") + "."
}

func (f ovFindings) noGate() string {
	return fmt.Sprintf("%d %s no gate, so no run settles %s", f.unchecked, namesName(f.unchecked), itThem(f.unchecked))
}

func (f ovFindings) otherState() string {
	return fmt.Sprintf("%d in a state this build does not read", f.other)
}

// causes is each cause of a finding not resolved with its count, for the
// item's body: the findings no run can settle first, since their next step
// differs, then the open ones, then any state this build does not read.
func (f ovFindings) causes() []string {
	var cs []string
	if f.unchecked > 0 {
		cs = append(cs, f.noGate())
	}
	if f.open > 0 {
		cs = append(cs, fmt.Sprintf("%d %s a gate with no green run of it after", f.open, namesName(f.open)))
	}
	if f.other > 0 {
		cs = append(cs, f.otherState())
	}
	return cs
}

// ovItem is one item of NEEDS ATTENTION: its title and body, the rows after
// the body (the failures among the failed calls), and what to do.
type ovItem struct {
	title, body string
	rows        [][2]string
	why, next   string
	evidence    []string
	note        string
}

// attention is NEEDS ATTENTION: the findings not resolved, the failed calls
// and the changed inputs, numbered, or a None line; and, when the findings
// were not read, a line that says so.
func (v *ovView) attention(s *Session) {
	v.section("NEEDS ATTENTION")
	a := s.Autosteer
	read := a != nil && a.Stop != nil
	var items []ovItem
	if read {
		if it, ok := ovFindingsItem(a); ok {
			items = append(items, it)
		}
	}
	if it, ok := ovFailuresItem(s); ok {
		items = append(items, it)
	}
	if it, ok := ovRewrittenItem(s); ok {
		items = append(items, it)
	}
	for i, it := range items {
		if i > 0 {
			v.blank()
		}
		v.item(i+1, it)
	}
	switch {
	case len(items) == 0 && read:
		v.para("None: no unresolved finding, no failed call, no changed input.")
	case len(items) == 0:
		v.para("None: no failed call, no changed input.")
	}
	switch {
	case a == nil:
		v.blank()
		v.para("AutoSteer's findings were not read here, so none is listed.")
	case a.Stop == nil:
		v.blank()
		v.para("No Stop was recorded, so AutoSteer's findings were not read and none is listed.")
	}
}

// item writes one item: its number painted, the title, the body under four
// columns, then each labeled row with its label padded to column 21.
func (v *ovView) item(n int, it ovItem) {
	num := strconv.Itoa(n)
	v.line(v.paint(ovAmber, num) + ovPad(4-len(num)) + it.title)
	for _, l := range ovWrap(it.body, ovWidth-4) {
		v.line("    " + l)
	}
	for _, r := range it.rows {
		v.sub(r[0], "", ovWrap(r[1], ovWidth-21))
	}
	v.sub("Why it matters", "", ovWrap(it.why, ovWidth-21))
	v.sub("Next step", ovBlue, ovWrap(it.next, ovWidth-21))
	if len(it.evidence) > 0 {
		v.sub("Evidence", "", it.evidence)
	}
	if it.note != "" {
		v.sub("Note", "", ovWrap(it.note, ovWidth-21))
	}
}

// sub is one labeled row of an item: four spaces, the label padded to column
// 21, and its lines, each painted with sgr.
func (v *ovView) sub(label, sgr string, lines []string) {
	for i, l := range lines {
		if i == 0 {
			v.line("    " + v.paint(ovDim, label) + ovPad(17-ovLen(label)) + v.paint(sgr, l))
			continue
		}
		v.line(ovPad(21) + v.paint(sgr, l))
	}
}

// ovFindingsItem is the item for the findings not resolved.
func ovFindingsItem(a *Autosteer) (ovItem, bool) {
	f := ovCountFindings(a.Findings)
	un := f.n - f.resolved
	if un == 0 {
		return ovItem{}, false
	}
	it := ovItem{title: "Task verification is incomplete",
		why: "Rashomon cannot determine whether these findings were addressed in the final code."}
	body := fmt.Sprintf("AutoSteer recorded %d finding%s. ", f.n, plural(f.n))
	switch {
	case f.n == 1 && f.unchecked == 1:
		body += "It was not resolved: it names no gate, so no run settles it."
	case f.n == 1 && f.open == 1:
		body += "It was not resolved: it names a gate with no green run of it after."
	case f.resolved == 0 && f.unchecked == f.n:
		body += "None was resolved: each names no gate, so no run settles it."
	case f.resolved == 0 && f.open == f.n:
		body += "None was resolved: each names a gate with no green run of it after."
	default:
		body += fmt.Sprintf("%d %s not resolved: %s.", un, wasWere(un), strings.Join(f.causes(), "; "))
	}
	it.body = body
	if un == 1 {
		it.why = "Rashomon cannot determine whether this finding was addressed in the final code."
	}
	switch {
	case f.open > 0 && f.unchecked == 0 && f.other == 0:
		it.next = "Run the gate each names on the current code and read its result."
	case f.open > 0:
		it.next = "Run the gate each open finding names on the current code and read its result; a finding that names no gate is not settled by a later run."
	default:
		it.next = "Run the project's tests or build on the current code yourself; a finding that names no gate is not settled by a later run."
	}
	if len(f.unresolved) > 0 {
		it.evidence = ovWrap("Calls "+ovSeqs(f.unresolved), ovWidth-21)
	}
	return it, true
}

// ovFailuresItem is the item for the failed calls, in the arithmetic of
// today's failure section: the lookups and the missing programs among them
// are named with their counts when not zero, and the note follows the order
// of SilentFailures (nothing to compare, unread, fired, a word found).
func ovFailuresItem(s *Session) (ovItem, bool) {
	sf := s.SilentFailures
	n := sf.Failed
	if n <= 0 {
		return ovItem{}, false
	}
	w := ovCap(ovWord(n))
	it := ovItem{
		title: fmt.Sprintf("%s tool call%s failed", w, plural(n)),
		body:  fmt.Sprintf("%s tool execution%s reported a failure during this session.", w, plural(n)),
		why:   "Some may have been corrected later. This report does not establish whether a later call corrected each one.",
		next:  "Review the failures and confirm whether any affects the final result.",
		evidence: ovWrap("rashomon report --session "+pasteArg(printable(s.SessionID))+" --chain shows each call in its prompt context.",
			ovWidth-21),
	}
	if n == 1 {
		it.why = "It may have been corrected later. This report does not establish whether a later call corrected it."
		it.next = "Review the failure and confirm whether it affects the final result."
	}
	if sf.FailedLookups > 0 {
		it.rows = append(it.rows, [2]string{"Lookups", fmt.Sprintf("%d among them (%s, whatever the error: a directory, a missing file or folder, a bad pattern), not set against the final message.", sf.FailedLookups, lookupToolNames)})
	}
	if sf.MissingPrograms > 0 {
		it.rows = append(it.rows, [2]string{"Missing programs", fmt.Sprintf("%d among them (a which, type or command -v probe that found nothing, or a command the shell could not find, exit 127), not set against the final message.", sf.MissingPrograms)})
	}
	switch {
	case sf.Counted() <= 0:
	case !sf.FinalMessageAvailable:
		it.note = "The agent's final message could not be read, so it was not compared with the failures."
	case sf.Fires:
		it.note = fmt.Sprintf("The agent's final message contains none of the %d failure words this report looks for.", len(sf.AbsentWords))
	default:
		it.note = "The agent's final message contains a failure-related word. That is a keyword match, not proof the failures were acknowledged or resolved."
	}
	return it, true
}

// ovRewrittenItem is the item for the calls that ran with an input other
// than the one first declared, each named by its tool-use id.
func ovRewrittenItem(s *Session) (ovItem, bool) {
	rw := s.Destinations.Rewritten
	n := len(rw)
	if n == 0 {
		return ovItem{}, false
	}
	w := ovCap(ovWord(n))
	it := ovItem{
		title: fmt.Sprintf("%s tool inputs differed from their declarations", w),
		why:   "This may be a normal transformation or an unexpected change. Which one is not established.",
		next:  "Read these calls in the session transcript; the record keeps a digest of each input, never its text.",
	}
	raw := map[string]int{}
	for _, r := range rw {
		raw[r.ToolName]++
	}
	tools := ovSorted(raw, printable)
	switch {
	case n == 1:
		it.title = "One tool input differed from its declaration"
		it.body = fmt.Sprintf("One %s call ran with an input that differed from what was first declared.", tools[0].name)
		it.next = "Read this call in the session transcript; the record keeps a digest of its input, never its text."
	case len(tools) == 1:
		it.body = fmt.Sprintf("%s %s calls ran with inputs that differed from what was first declared.", w, tools[0].name)
	default:
		parts := make([]string, len(tools))
		for i, t := range tools {
			parts[i] = fmt.Sprintf("%d %s", t.n, t.name)
		}
		it.body = fmt.Sprintf("%s tool calls ran with inputs that differed from what was first declared: %s.", w, strings.Join(parts, ", "))
	}
	for i, r := range rw {
		if i == ovMaxList {
			it.evidence = append(it.evidence, fmt.Sprintf("and %d more", n-ovMaxList))
			break
		}
		it.evidence = append(it.evidence, printable(r.ToolUseID))
	}
	return it, true
}
