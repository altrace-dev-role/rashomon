package report

import (
	"fmt"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// knowledge is PROJECT KNOWLEDGE & LEARNING. It hedges where today's
// knowledge block hedges (text_knowledge.go): a baseline walk that stopped at
// its limit, an unsigned spec-trust.json, a lesson ledger that did not check.
func (v *ovView) knowledge(s *Session) {
	v.section("PROJECT KNOWLEDGE & LEARNING")
	k := s.Knowledge
	v.krow("Known now", ovKnown(s))
	newly := "Not read here."
	if k != nil {
		newly = "Nothing was discovered in this session."
		if len(k.Discovered) > 0 {
			newly = ovCap(printable(changesText(k.Discovered))) + "."
		}
	}
	v.krow("Newly observed", newly)
	v.krow("Lessons", ovLessons(s.Autosteer))
	v.krow("Spec", ovSpec(k))
}

// ovKnown is what the project holds when the report runs: the gate families
// the index requires and the hosts in the network baseline, both as read
// now, so a family or a host first seen in this session may be counted here
// and listed under Newly observed as well. When Novelty is not available,
// whatever the Reason (the baseline may have been read and its update
// refused), one fixed sentence says the host count is not known; it never
// prints the Reason and never reads as zero hosts. Then the earlier
// checkpoint and what this session invalidated.
func ovKnown(s *Session) string {
	k := s.Knowledge
	if k == nil {
		return "Not read here."
	}
	nv := s.Destinations.Novelty
	n, f := nv.KnownHosts, k.Retained.RequiredFamilies
	var b strings.Builder
	switch {
	case k.Retained.Prior && nv.Available:
		fmt.Fprintf(&b, "No gate family established, so every gate is required (index). %d host%s %s in this project's network baseline.",
			n, plural(n), isAre(n))
	case k.Retained.Prior:
		b.WriteString("No gate family established, so every gate is required (index). The host count of the network baseline is not known.")
	case nv.Available:
		fmt.Fprintf(&b, "%d gate famil%s required (index) and %d host%s in this project's network baseline.", f, yIes(f), n, plural(n))
	default:
		fmt.Fprintf(&b, "%d gate famil%s required (index). The host count of the network baseline is not known.", f, yIes(f))
	}
	b.WriteString(" " + ovBaseline(k))
	if len(k.Invalidated) == 0 {
		b.WriteString(" Nothing known was invalidated.")
	} else {
		b.WriteString(" Invalidated in this session: " + printable(changesText(k.Invalidated)) + ".")
	}
	return b.String()
}

// ovBaseline is the earlier checkpoint, hedged when the walk stopped at its
// limit.
func ovBaseline(k *Knowledge) string {
	bl := k.Baseline
	switch {
	case bl == nil && k.BaselineWalkCap > 0:
		return fmt.Sprintf("No earlier checkpoint was found in the first %d session directories walked.", k.BaselineWalkCap)
	case bl == nil:
		return "No earlier checkpoint of this project."
	case k.BaselineWalkCap > 0:
		return fmt.Sprintf("Newest earlier checkpoint among the first %d session directories walked: session %s, %s; a newer one may exist.",
			k.BaselineWalkCap, ovShort(bl.SessionID), knowledgeTime(bl.RecordedAtUnixMS))
	}
	return fmt.Sprintf("Newest earlier checkpoint: session %s, %s.", ovShort(bl.SessionID), knowledgeTime(bl.RecordedAtUnixMS))
}

// ovLessons is the lesson ledger's counts and how many lessons were sent;
// a lesson reaches the agent as a learned-fact note.
func ovLessons(a *Autosteer) string {
	if a == nil || a.Lessons == nil {
		return "Not read here."
	}
	l := a.Lessons
	var s string
	switch {
	case l.Unchecked:
		s = fmt.Sprintf("The ledger did not check, so all %d read proposed (this project).", l.Proposed)
	case l.Approved == 0 && l.Proposed == 0 && l.Demoted == 0:
		s = "None proposed, approved or demoted."
	default:
		s = fmt.Sprintf("%d approved, %d proposed, %d demoted (this project).", l.Approved, l.Proposed, l.Demoted)
	}
	sent := 0
	for _, d := range a.Delivered {
		if d.Pattern == string(pattern.LearnedFact) {
			sent++
		}
	}
	switch {
	case sent > 0:
		return s + fmt.Sprintf(" %d %s sent to the agent this session.", sent, wasWere(sent))
	case l.Approved == 0 && !l.Unchecked:
		return s + " No lesson was sent to the agent, because none is approved for this project."
	}
	return s + " None was sent to the agent this session."
}

// ovSpec is the spec bindings, or why none was read.
func ovSpec(k *Knowledge) string {
	if k == nil {
		return "Not read here."
	}
	r := k.Retained
	var s string
	switch {
	case r.BindingsUnsigned:
		s = "spec-trust.json does not carry this store's MAC, so no binding was read and no spec's rules were checked against the work."
	case len(r.Specs) == 0:
		s = "No spec is bound in spec-trust.json, so no spec's rules were checked against the work."
	default:
		parts := make([]string, len(r.Specs))
		for i, sp := range r.Specs {
			parts[i] = fmt.Sprintf("Spec %s is bound in spec-trust.json (bound %s).", printable(sp.Path), knowledgeTime(sp.BoundAtMS))
		}
		s = strings.Join(parts, " ")
	}
	if st := r.SpecAtLastStop; st != nil {
		s += " The spec read " + printable(st.Trust) + " at the last Stop."
	}
	return s
}

// ovVerbNames are the verb classes whose code is not a word a reader knows.
var ovVerbNames = map[string]string{"vcs": "version control", "unknown": "unclassified"}

func ovVerb(c string) string {
	if n, ok := ovVerbNames[c]; ok {
		return n
	}
	return printable(strings.ReplaceAll(c, "_", " "))
}

// activity is SESSION ACTIVITY: the calls by verb class, the tools, the
// known program families that ran, the hosts and what the last Stop printed.
func (v *ovView) activity(s *Session) {
	v.section("SESSION ACTIVITY")
	d := s.Declarations
	v.krows(fmt.Sprintf("%d call%s", d.Recorded, plural(d.Recorded)), ovNameCounts(ovSorted(d.ByVerbClass, ovVerb)))
	v.krows("Tools", ovNameCounts(ovSorted(d.ByTool, printable)))
	v.krows("Programs", ovPrograms(s.Families))
	v.krow("Hosts", ovHosts(s.Destinations))
	v.krow("Observed", ovObserved(s.Autosteer))
}

// ovPrograms is the known families that ran with their calls; a program in
// no known family is not counted here, so the row says so.
func ovPrograms(fc FamilyCoverage) []string {
	m := map[string]int{}
	for _, f := range fc.Families {
		if f.Calls > 0 {
			m[f.Name] += f.Calls
		}
	}
	items := ovNameCounts(ovSorted(m, printable))
	if len(items) == 0 {
		return []string{"none of the known families ran"}
	}
	items[0] = "known families only: " + items[0]
	return items
}

// ovHosts is how many distinct hosts the calls named, https and ssh alike.
func ovHosts(d Destinations) string {
	set := map[string]bool{}
	for _, h := range d.DeclaredHosts {
		set[h] = true
	}
	for _, h := range d.SSHHosts {
		set[h] = true
	}
	if len(set) == 0 {
		return "none named by any call"
	}
	return fmt.Sprintf("%d named by the calls", len(set))
}

// ovObserved is the state line the last Stop printed, or why there is none.
func ovObserved(a *Autosteer) string {
	switch {
	case a == nil:
		return "The last Stop was not read here."
	case a.Stop == nil:
		return "No Stop was recorded for this session."
	}
	if s := strings.TrimSuffix(strings.TrimSpace(printable(a.Stop.Sentence)), "."); s != "" {
		return fmt.Sprintf("The last Stop (call %d) printed: %s.", a.Stop.Seq, s)
	}
	return fmt.Sprintf("The last Stop (call %d) printed no state line. Its significance is not established.", a.Stop.Seq)
}

// evidence is DETAILED EVIDENCE: the commands that print the long view, the
// recording's integrity in one line and, when redacted, what was withheld.
func (v *ovView) evidence(rep *Report, s *Session) {
	v.blank()
	v.line(strings.Repeat("─", ovWidth))
	v.line(v.paint(ovDim, "DETAILED EVIDENCE"))
	v.blank()
	cmd := "rashomon report --session " + pasteArg(printable(s.SessionID))
	for _, e := range [...][2]string{
		{"--full", "Every table of this session's record."},
		{"--chain", "Every call, grouped under the prompt that led to it."},
		{"--json", "The full structured record, for scripts and audits."},
	} {
		v.line(v.paint(ovBlue, cmd+" "+e[0]))
		v.line("    " + e[1])
	}
	v.blank()
	v.para("Recording integrity: " + strings.Join(ovIntegrity(s), " · ") + ".")
	if rep.Redacted {
		v.para("Privacy: the agent's own summary is withheld, and hostnames are redacted under this install's key.")
	}
}

// ovIntegrity is the hook entries, the transcripts read, and each count of
// calls the record holds incompletely, or that there is none.
func ovIntegrity(s *Session) []string {
	c, d := s.Coverage, s.Declarations
	parts := []string{ovHooks(c.HookEntryAtStart, c.HookEntryAtEnd), ovTranscriptsRead(s.Transcripts)}
	denied := 0
	for _, t := range s.Transcripts {
		denied += len(t.DeniedByUser)
	}
	var faults []string
	if n := len(d.Dropped); n > 0 {
		faults = append(faults, fmt.Sprintf("%d dropped", n))
	}
	if denied > 0 {
		faults = append(faults, fmt.Sprintf("%d denied by the user", denied))
	}
	if n := len(d.Unterminated); n > 0 {
		faults = append(faults, fmt.Sprintf("%d unterminated (%s)", n, ovIDs(d.Unterminated)))
	}
	if n := len(d.NotDurable); n > 0 {
		faults = append(faults, fmt.Sprintf("%d possibly not durable", n))
	}
	if n := s.SkippedRecords; n > 0 {
		faults = append(faults, fmt.Sprintf("%d record%s skipped", n, plural(n)))
	}
	if len(faults) == 0 {
		faults = []string{"nothing dropped, denied, unterminated or skipped"}
	}
	return append(parts, faults...)
}

func ovHooks(start, end string) string {
	if ovHookPresent(start) && ovHookPresent(end) {
		return "hooks present at start and end"
	}
	return "hook entry at start " + ovEntry(start) + ", at end " + ovEntry(end)
}

func ovHookPresent(e string) bool {
	return e == store.EntryPresentSettings || e == store.EntryPresentPlugin || e == store.EntryPresent
}

func ovEntry(e string) string {
	if e == "" {
		return "not recorded"
	}
	return printable(strings.ReplaceAll(e, "_", " "))
}

func ovTranscriptsRead(ts []Transcript) string {
	read := 0
	for _, t := range ts {
		if t.Status == TranscriptRead {
			read++
		}
	}
	switch {
	case len(ts) == 0:
		return "no transcript read"
	case read == len(ts) && read == 1:
		return "transcript readable"
	case read == len(ts):
		return fmt.Sprintf("%d transcripts readable", read)
	}
	return fmt.Sprintf("%d of %d transcripts readable", read, len(ts))
}
