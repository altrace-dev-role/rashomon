package report

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The focused cases of the overview (card RPT-1). Each expected string is a
// hand-written sentence laid out by the same separate script as the goldens
// in overview_test.go; none comes from the renderer.

// ovHas fails the test for each want that out does not hold.
func ovHas(t *testing.T, name, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("%s: render lacks %q\n%s", name, w, out)
		}
	}
}

// ovLacks fails the test for each bad that out holds.
func ovLacks(t *testing.T, name, out string, bads ...string) {
	t.Helper()
	for _, b := range bads {
		if strings.Contains(out, b) {
			t.Errorf("%s: render holds %q\n%s", name, b, out)
		}
	}
}

// TestRPT1e_FindingsMixAndOpenGate: findings in all three states print each
// count, and the attention item names both causes and the step for a gate
// that was named.
func TestRPT1e_FindingsMixAndOpenGate(t *testing.T) {
	rep := ovA5()
	a := rep.Sessions[0].Autosteer
	a.Findings[0].State, a.Findings[0].ResolvedBy, a.Findings[0].Gate = "resolved", 70, "go test"
	a.Findings[1].State, a.Findings[1].Gate = "open", "go test"
	a.Findings[2].State, a.Findings[2].Gate = "open", "go test"
	out := ovRender(t, rep)
	ovHas(t, "mixed", out,
		"\nOutcome verified           !  1 of 4 resolved by a green run of its gate; 2 open: no green run of\n"+
			"                              the gate came after; 1 names no gate, so no run settles it.\n",
		"\n    AutoSteer recorded 4 findings. 3 were not resolved: 1 names no gate, so no run settles it; 2\n"+
			"    name a gate with no green run of it after.\n",
		"\n    Evidence         Calls #112, #260, #334\n",
		"\nRecorded the whole session, and AutoSteer sent the agent 5 notes: 4 record checks and 1 preflight. 3\n"+
			"of the 4 findings were not resolved.\n",
	)
	ovLacks(t, "mixed", out, "None of the 4 findings")

	for i := range a.Findings {
		a.Findings[i].State, a.Findings[i].Gate = "open", "go test"
	}
	ovHas(t, "all open", ovRender(t, rep),
		"\n    Next step        Run the gate each names on the current code and read its result.\n")
}

// TestRPT1f_FailureItemRows: the failure item lists the lookups and the
// missing programs among its failures, after its body and before why it
// matters, and its note follows SilentFailures: nothing when every failure was
// a lookup or a missing program, then unread, then fired, then a word found.
func TestRPT1f_FailureItemRows(t *testing.T) {
	const two = "\n2   Two tool calls failed\n    Two tool executions reported a failure during this session.\n"
	lookups := "    Lookups          2 among them (Read, Glob, Grep or NotebookRead, whatever the error: a\n" +
		"                     directory, a missing file or folder, a bad pattern), not set against the final\n" +
		"                     message.\n"
	missing := "    Missing programs 1 among them (a which, type or command -v probe that found nothing, or a\n" +
		"                     command the shell could not find, exit 127), not set against the final message.\n"
	word := "    Note             The agent's final message contains a failure-related word. That is a keyword\n" +
		"                     match, not proof the failures were acknowledged or resolved.\n"
	cases := []struct {
		name  string
		sf    SilentFailures
		wants []string
		note  bool
	}{
		{"fires", SilentFailures{Failed: 2, Fires: true, FinalMessageAvailable: true, AbsentWords: []string{"fail", "error"}},
			[]string{two, "    Note             The agent's final message contains none of the 2 failure words this report\n" +
				"                     looks for.\n"}, true},
		{"unread", SilentFailures{Failed: 2, AbsentWords: []string{}},
			[]string{two, "    Note             The agent's final message could not be read, so it was not compared with the\n" +
				"                     failures.\n"}, true},
		{"lookups only", SilentFailures{Failed: 2, FailedLookups: 2, FinalMessageAvailable: true, AbsentWords: []string{}},
			[]string{two + lookups + "    Why it matters   "}, false},
		{"a missing program", SilentFailures{Failed: 3, MissingPrograms: 1, FinalMessageAvailable: true, AbsentWords: []string{}},
			[]string{"\n2   Three tool calls failed\n    Three tool executions reported a failure during this session.\n" +
				missing + "    Why it matters   ", word}, true},
	}
	for _, c := range cases {
		rep := ovA5()
		rep.Sessions[0].SilentFailures = c.sf
		out := ovRender(t, rep)
		ovHas(t, c.name, out, c.wants...)
		if !c.note {
			ovLacks(t, c.name, out, "    Note ")
		}
	}
}

// TestRPT1g_RecordingHeadingTruthTable: the recording heading is a check only
// when every clause of ovRecordingOf holds, and each row below makes one
// clause false. A row that leaves State verified while a list is not empty
// pins the overview to read the list itself, not the state alone.
func TestRPT1g_RecordingHeadingTruthTable(t *testing.T) {
	const (
		ok       = "│ ✓ RECORDING COMPLETE "
		bang     = "│ ! RECORDING INCOMPLETE "
		unk      = "│ ? RECORDING NOT CHECKED AGAINST A TRANSCRIPT "
		not      = "The record is not verified complete: "
		noneRd   = "118 tool calls were recorded. No transcript was read, so the record was not compared with one."
		disagree = not + "the transcript and this store disagree about which tool calls were made."
	)
	missing := func(s *Session) {
		tr := &s.Transcripts[0]
		tr.Status, tr.Readable, tr.IDsInTranscript, tr.ResultsInTranscript = TranscriptMissing, false, nil, nil
	}
	cases := []struct {
		name, heading, body string
		edit                func(s *Session)
	}{
		{"every clause holds", ok,
			"All 118 tool calls in the session transcript were recorded with their results. No recording gaps were detected.",
			func(*Session) {}},
		{"coverage unverified", bang, not + "a gap record marks a stretch this store does not hold:",
			func(s *Session) { s.Coverage.State, s.Coverage.Reasons = store.StateUnverified, []string{ReasonGap} }},
		{"two reasons", bang, "or its end hook ran while `rashomon pause` was in effect; a gap record marks",
			func(s *Session) {
				s.Coverage.State, s.Coverage.Reasons = store.StateUnverified, []string{ReasonRunNotClosed, ReasonGap}
			}},
		{"unknown reason code", bang, not + "brand_new_code.",
			func(s *Session) {
				s.Coverage.State, s.Coverage.Reasons = store.StateUnverified, []string{"brand_new_code"}
			}},
		{"unverified with no reason", bang, "The record is not verified complete.",
			func(s *Session) { s.Coverage.State = store.StateUnverified }},
		{"no transcript", unk, noneRd, func(s *Session) { s.Transcripts = []Transcript{} }},
		{"transcript missing", unk, noneRd, missing},
		{"missing from store", bang, disagree,
			func(s *Session) { s.Transcripts[0].MissingFromStore = []string{"toolu_a"} }},
		{"missing from transcript", bang, disagree,
			func(s *Session) { s.Transcripts[0].MissingFromTranscript = []string{"toolu_b"} }},
		{"executed but unrecorded", bang,
			not + "the transcript holds results for calls this store recorded no execution for.",
			func(s *Session) { s.Transcripts[0].ExecutedButUnrecorded = []string{"toolu_c"} }},
		{"gap record", bang, not + "a gap record marks a stretch this store does not hold:",
			func(s *Session) { s.Gaps = []store.Gap{{Reason: "forget"}} }},
	}
	for _, c := range cases {
		rep := ovA5()
		c.edit(&rep.Sessions[0])
		out := ovRender(t, rep)
		ovHas(t, c.name, out, c.heading)
		ovHas(t, c.name, ovBoxText(out), c.body)
		for _, h := range []string{ok, bang, unk} {
			if h != c.heading {
				ovLacks(t, c.name, out, h)
			}
		}
		// The overview names no line it does not print: the long view's
		// pointer to its `missing from` lines is cut.
		ovLacks(t, c.name, out, "lines below")
		if c.heading == ok {
			ovHas(t, c.name, out, "\nRecorded the whole session, ", "A complete recording shows")
			continue
		}
		ovHas(t, c.name, out, "\nRecorded 118 calls, ")
		ovLacks(t, c.name, out, "Recorded the whole session", "A complete recording shows")
	}

	rep := ovA5()
	s := &rep.Sessions[0]
	s.Coverage.State, s.Coverage.Reasons, s.Gaps = store.StateUnverified, []string{ReasonGap}, []store.Gap{{Reason: "forget"}}
	if n := strings.Count(ovBoxText(ovRender(t, rep)), "a gap record marks"); n != 1 {
		t.Errorf("a gap both coded and recorded prints %d times, want once", n)
	}
}

// TestRPT1i_RefusedBeforeHooks: the box counts the calls that reached a hook
// (the ids of every read transcript less those refused before any hook ran),
// and says when results are fewer than calls.
func TestRPT1i_RefusedBeforeHooks(t *testing.T) {
	rep := ovA5()
	tr := &rep.Sessions[0].Transcripts[0]
	tr.IDsInTranscript, tr.ResultsInTranscript, tr.RefusedBeforeHooks = ovInt(120), ovInt(120), []string{"toolu_r1", "toolu_r2"}
	out := ovRender(t, rep)
	ovHas(t, "refused", out, "│ ✓ RECORDING COMPLETE ")
	ovHas(t, "refused", ovBoxText(out), "All 118 tool calls that reached a hook were recorded with their results; "+
		"2 more were refused before any hook ran. No recording gaps were detected.")

	rep = ovA5()
	rep.Sessions[0].Transcripts[0].ResultsInTranscript = ovInt(116)
	ovHas(t, "fewer results", ovBoxText(ovRender(t, rep)), "All 118 tool calls in the session transcript were recorded, "+
		"and every result in the transcript has its execution recorded. No recording gaps were detected.")

	rep = ovA5()
	two := append(ovTranscripts(100), ovTranscripts(18)...)
	rep.Sessions[0].Transcripts = two
	ovHas(t, "two transcripts", ovBoxText(ovRender(t, rep)),
		"All 118 tool calls in the session transcript were recorded with their results.")
}

// TestRPT1j_SourcesNotRead: a part of the record this report did not read
// says so, and never reads as none or zero.
func TestRPT1j_SourcesNotRead(t *testing.T) {
	rep := ovA5()
	rep.Sessions[0].Autosteer = nil
	out := ovRender(t, rep)
	ovHas(t, "autosteer", ovBoxText(out), "? LAST STOP NOT READ The last Stop was not read here, so no gate state is shown.")
	ovHas(t, "autosteer", out,
		"\nRecorded the whole session; AutoSteer's record of it was not read here.\n",
		"\n1   Seven tool calls failed\n",
		"\nAutoSteer's findings were not read here, so none is listed.\n\nPROJECT KNOWLEDGE & LEARNING\n",
		"\nLessons            Not read here.\n",
		"\nObserved           The last Stop was not read here.\n")
	ovLacks(t, "autosteer", out, "None sent.", "0 of ", "0 mistakes", "Task verification is incomplete")

	rep = ovA5()
	// With no checkpoint, autosteerOf (cmd/rashomon/report_impact.go) returns
	// empty lists, no lessons and no mistakes: the lists were not read, so
	// none of them reads as none sent or zero settled.
	rep.Sessions[0].Autosteer = &Autosteer{Gates: []AutosteerGate{}, Delivered: []AutosteerNote{},
		Findings: []AutosteerFinding{}, Checks: []AutosteerCheck{}, Unknown: []AutosteerUnknown{}}
	out = ovRender(t, rep)
	ovHas(t, "stop", ovBoxText(out), "? NO STOP RECORDED No Stop was recorded for this session, so no gate state was read.")
	ovHas(t, "stop", out,
		"\nRecorded the whole session; no Stop was recorded, so AutoSteer's record of it was not read.\n",
		"\nNo Stop was recorded, so AutoSteer's findings were not read and none is listed.\n\nPROJECT KNOWLEDGE & LEARNING\n",
		"\nLessons            Not read here.\n",
		"\nObserved           No Stop was recorded for this session.\n")
	ovLacks(t, "stop", out, "None sent.", "0 of ", "0 mistakes", "nothing to settle")

	rep = ovA5()
	rep.Sessions[0].Autosteer.Lessons = nil
	ovHas(t, "lessons", ovRender(t, rep), "\nLessons            Not read here.\n")

	rep = ovA5()
	rep.Sessions[0].Knowledge = nil
	out = ovRender(t, rep)
	ovHas(t, "knowledge", out,
		"\n118 tool calls across 13 prompts · no subagents\n",
		"\nKnown now          Not read here.\n",
		"\nNewly observed     Not read here.\n",
		"\nSpec               Not read here.\n")
	ovLacks(t, "knowledge", out, "project ")

	rep = ovA5()
	rep.Sessions[0].Autosteer.Mistakes = nil
	ovHas(t, "mistakes", ovRender(t, rep),
		"\nImprovement demonstrated   ?  Unknown. The mistake counts were not read here.\n")
}

// TestRPT1k_NumberWordsEdge: item titles and bodies spell numbers up to
// twelve and use digits above it.
func TestRPT1k_NumberWordsEdge(t *testing.T) {
	for _, c := range []struct {
		n           int
		title, body string
	}{
		{1, "One tool call failed", "One tool execution reported a failure during this session."},
		{12, "Twelve tool calls failed", "Twelve tool executions reported a failure during this session."},
		{13, "13 tool calls failed", "13 tool executions reported a failure during this session."},
	} {
		rep := ovA5()
		rep.Sessions[0].SilentFailures = SilentFailures{Failed: c.n, FinalMessageAvailable: true, AbsentWords: []string{}}
		ovHas(t, c.title, ovRender(t, rep), "\n2   "+c.title+"\n    "+c.body+"\n")
	}
}

// TestRPT1l_ImprovementForms: the improvement row is always a question mark,
// counts what was caught and what was found later when each was read, and
// never turns an unread count into zero.
func TestRPT1l_ImprovementForms(t *testing.T) {
	empty := func() *AutosteerMistakes {
		return &AutosteerMistakes{Caught: []AutosteerMistake{}, Reached: []AutosteerMistake{}, Missed: []AutosteerMistake{},
			Unsettled: []AutosteerMistake{}, FixedUnprompted: []AutosteerMistake{}}
	}
	both := empty()
	both.Caught = []AutosteerMistake{{ID: "a", Seq: 20, ResolvedBy: 25}, {ID: "b", Seq: 30, ResolvedBy: 35}}
	both.Later = []AutosteerMistake{{ID: "c", Seq: 50}}
	zero := empty()
	zero.Later = []AutosteerMistake{}
	for _, c := range []struct {
		name string
		m    *AutosteerMistakes
		want string
	}{
		{"caught and later", both,
			"\nImprovement demonstrated   ?  Unknown. 2 mistakes were recorded as caught (calls #20, #30); 1 was\n" +
				"                              found later (call #50); no record measures the final result.\n"},
		{"none caught, none later", zero,
			"\nImprovement demonstrated   ?  Unknown. 0 mistakes were recorded as caught, which does not mean none\n" +
				"                              occurred; 0 were found later.\n"},
		{"not read", nil, "\nImprovement demonstrated   ?  Unknown. The mistake counts were not read here.\n"},
	} {
		rep := ovA5()
		rep.Sessions[0].Autosteer.Mistakes = c.m
		out := ovRender(t, rep)
		ovHas(t, c.name, out, c.want)
		ovLacks(t, c.name, out, "Improvement demonstrated   ✓", "Improvement demonstrated   !")
	}
}

// TestRPT1m_UnderNoPrompt: the header counts the calls under no prompt.
func TestRPT1m_UnderNoPrompt(t *testing.T) {
	rep := ovA5()
	rep.Sessions[0].Chains.Unattributed = []Link{{}, {}}
	ovHas(t, "unattributed", ovRender(t, rep),
		"\nproject a78e7325 · 118 tool calls across 13 prompts, 2 under no prompt · no subagents\n")
}

// TestRPT1o_PayloadNamesPrintable: a tool name and a tool-use id come from
// the hook payload, so the overview prints them as today's text does
// (printable): a control byte in one never reaches the terminal, color or not.
func TestRPT1o_PayloadNamesPrintable(t *testing.T) {
	rep := ovA5()
	s := &rep.Sessions[0]
	s.Declarations.ByTool["evil\x1b[31mX"] = 1
	s.Destinations.Rewritten[0].ToolName = "Send\x1b[2JMessage"
	s.Destinations.Rewritten[1].ToolUseID = "toolu_\x07bell"
	out := ovRender(t, rep)
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("a control byte from the payload reached the render:\n%q", out)
	}
	ovHas(t, "printable", out, "evil[31mX 1", "toolu_bell")
}

// TestRPT1n_KnowledgeBranches: where today's knowledge block hedges (a
// limited walk for the baseline, an unsigned spec-trust.json, a ledger that
// did not check), the overview hedges in the same words and never says
// newest, unbound or none approved.
func TestRPT1n_KnowledgeBranches(t *testing.T) {
	rep := ovA5()
	k := rep.Sessions[0].Knowledge
	k.Baseline, k.BaselineWalkCap = nil, 50
	out := ovRender(t, rep)
	ovHas(t, "walk limited, no baseline", out,
		"\nKnown now          2 gate families required (index) and 19 hosts in this project's network baseline.\n"+
			"                   No earlier checkpoint was found in the first 50 session directories walked.\n"+
			"                   Nothing known was invalidated.\n")
	ovLacks(t, "walk limited, no baseline", out, "No earlier checkpoint of this project")

	rep = ovA5()
	rep.Sessions[0].Knowledge.BaselineWalkCap = 50
	ovHas(t, "walk limited, baseline found", ovRender(t, rep),
		"\nKnown now          2 gate families required (index) and 19 hosts in this project's network baseline.\n"+
			"                   Newest earlier checkpoint among the first 50 session directories walked: session\n"+
			"                   c7ba1d62, 11-08 13:32; a newer one may exist. Nothing known was invalidated.\n")

	rep = ovA5()
	rep.Sessions[0].Knowledge.Retained.BindingsUnsigned = true
	out = ovRender(t, rep)
	ovHas(t, "unsigned", out,
		"\nSpec               spec-trust.json does not carry this store's MAC, so no binding was read and no\n"+
			"                   spec's rules were checked against the work.\n")
	ovLacks(t, "unsigned", out, "No spec is bound")

	rep = ovA5()
	rep.Sessions[0].Knowledge.Retained.Specs = []SpecHeld{{Path: "docs/spec.md", FileDigest: "sha256:ab",
		BoundAtMS: rep.Sessions[0].Knowledge.Baseline.RecordedAtUnixMS}}
	ovHas(t, "bound", ovRender(t, rep), "\nSpec               Spec docs/spec.md is bound in spec-trust.json (bound 11-08 13:32).\n")

	rep = ovA5()
	rep.Sessions[0].Autosteer.Lessons = &AutosteerLessons{Proposed: 2, Unchecked: true}
	out = ovRender(t, rep)
	ovHas(t, "ledger unchecked", out,
		"\nLessons            The ledger did not check, so all 2 read proposed (this project). None was sent to\n"+
			"                   the agent this session.\n")
	ovLacks(t, "ledger unchecked", out, "because none is approved")
}

// TestRPT1p_KnownNowCountsWhatIsKnownNow: the Known now row prints the counts
// the index and the network baseline hold when the report runs. A gate family
// first seen in this session is listed under Newly observed and may also be
// counted under Known now; the hosts the session named do not hide the
// baseline's count; where Novelty is not available, for a baseline read and
// its update refused as for one never located, one fixed sentence says the
// count is not known, with no Reason and never zero hosts.
func TestRPT1p_KnownNowCountsWhatIsKnownNow(t *testing.T) {
	const known = "\nKnown now          2 gate families required (index) and 19 hosts in this project's network baseline.\n"
	out := ovRender(t, ovA5())
	ovHas(t, "family in both", out, known,
		"\nNewly observed     Gate family first seen in the project: lint (index).\n")
	ovLacks(t, "family in both", out, "Already known")

	rep := ovA5()
	rep.Sessions[0].Destinations.DeclaredHosts = []string{"example.test"}
	rep.Sessions[0].Destinations.SSHHosts = []string{"git.example.test"}
	ovHas(t, "hosts named", ovRender(t, rep), known, "\nHosts              2 named by the calls\n")

	// The second reason is gather's for baseline.ErrTooManyHosts
	// (baselineErrorReason): the baseline was read and its update refused.
	for _, reason := range []string{
		"no store root, so the baseline could not be located",
		"this session named more hosts than a project baseline can hold, so novelty could not be updated this time",
	} {
		rep = ovA5()
		rep.Sessions[0].Destinations.Novelty = Novelty{Available: false, Reason: reason}
		out = ovRender(t, rep)
		ovHas(t, reason, out,
			"\nKnown now          2 gate families required (index). The host count of the network baseline is not\n"+
				"                   known. Newest earlier checkpoint: session c7ba1d62, 11-08 13:32. Nothing known\n"+
				"                   was invalidated.\n")
		ovLacks(t, reason, out, reason, "hosts in this project's network baseline", "0 hosts", "was not read")
	}
}

// TestRPT1t_BranchesNoGoldenReaches: a project with no gate family established
// still counts the hosts its baseline holds, in the singular for one; one
// finding that names no gate reads in the singular; a finding in a state recap
// does not write is named and never counted as resolved; a lesson delivered
// this session is counted as sent.
func TestRPT1t_BranchesNoGoldenReaches(t *testing.T) {
	for _, c := range []struct {
		hosts int
		want  string
	}{
		{19, "\nKnown now          No gate family established, so every gate is required (index). 19 hosts are in\n" +
			"                   this project's network baseline. Newest earlier checkpoint: session c7ba1d62,\n" +
			"                   11-08 13:32. Nothing known was invalidated.\n"},
		{1, "\nKnown now          No gate family established, so every gate is required (index). 1 host is in this\n" +
			"                   project's network baseline. Newest earlier checkpoint: session c7ba1d62, 11-08\n" +
			"                   13:32. Nothing known was invalidated.\n"},
	} {
		rep := ovA5()
		rep.Sessions[0].Knowledge.Retained.Prior = true
		rep.Sessions[0].Destinations.Novelty.KnownHosts = c.hosts
		ovHas(t, "no family, baseline read", ovRender(t, rep), c.want)
	}

	rep := ovA5()
	a := rep.Sessions[0].Autosteer
	a.Findings = a.Findings[:1]
	out := ovRender(t, rep)
	ovHas(t, "one finding, no gate", out,
		"\nOutcome verified           !  0 of 1. The 1 finding names no gate, so no run settles it.\n")
	ovLacks(t, "one finding, no gate", out, "None of the 1 findings")

	rep = ovA5()
	a = rep.Sessions[0].Autosteer
	a.Findings[0].State, a.Findings[0].ResolvedBy, a.Findings[0].Gate = "resolved", 70, "go test"
	a.Findings[1].State = "withdrawn"
	flat := strings.Join(strings.Fields(ovRender(t, rep)), " ")
	ovHas(t, "a state recap does not write", flat, "Outcome verified ! 1 of 4 resolved by a green run of its gate; "+
		"2 name no gate, so no run settles them; 1 in a state this build does not read.")
	ovLacks(t, "a state recap does not write", flat, "2 of 4 resolved")

	rep = ovA5()
	a = rep.Sessions[0].Autosteer
	a.Lessons = &AutosteerLessons{Approved: 1}
	a.Delivered = append(a.Delivered, ovNote("learned-fact", "lesson", 70))
	out = ovRender(t, rep)
	ovHas(t, "lesson sent", out,
		"\nLessons            1 approved, 0 proposed, 0 demoted (this project). 1 was sent to the agent this\n"+
			"                   session.\n")
	ovLacks(t, "lesson sent", out, "None was sent", "No lesson was sent")
}
