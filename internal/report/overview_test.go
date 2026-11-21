package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The owner's layout of `rashomon report` (card RPT-1, the layout of
// 2026-11-21). Every expected text below is written by hand from that layout,
// with the rewordings the card's mapping table names, and never produced by
// the renderer: a test whose expected value comes from the function under
// test cannot fail.

func ovInt(n int) *int { return &n }

func ovSeq(n int64) *int64 { return &n }

func ovGenerated() int64 { return time.Date(2026, 11, 21, 8, 15, 44, 0, time.UTC).UnixMilli() }

func ovCoverage() Coverage {
	return Coverage{State: store.StateVerified, Reasons: []string{}, StartRecorded: true, EndRecorded: true,
		HookEntryAtStart: store.EntryPresentSettings, HookEntryAtEnd: store.EntryPresentSettings}
}

func ovTranscripts(n int) []Transcript {
	return []Transcript{{Path: "58014a24.jsonl", Readable: true, Status: TranscriptRead, Files: ovInt(1), LinesTooLong: ovInt(0),
		IDsInTranscript: ovInt(n), IDsRecorded: n, ResultsInTranscript: ovInt(n), IDsExecuted: n,
		MissingFromStore: []string{}, MissingFromTranscript: []string{}, RefusedBeforeHooks: []string{},
		ExecutedButUnrecorded: []string{}, DeniedByUser: []string{}, DeclaredWithoutResult: []string{}}}
}

func ovDeclarations(n int, byTool, byVerb map[string]int) Declarations {
	return Declarations{Recorded: n, ByTool: byTool, ByVerbClass: byVerb, ByProgram: map[string]int{}, ByLabel: map[string]int{},
		Unterminated: []string{}, Dropped: []string{}, NotDurable: []string{}, WithoutExecution: []Unexecuted{}}
}

func ovNote(pattern, title string, seq int64) AutosteerNote {
	return AutosteerNote{ID: "n" + title, Pattern: pattern, Title: title, Event: "PostToolUse", Seq: seq}
}

// ovA5 is the session of the owner's image (a5ae68a1, 118 calls, 13
// prompts, 5 notes, 4 findings that name no gate, 7 failed calls, 3 inputs
// changed), redacted as the image was.
func ovA5() *Report {
	unchecked := func(seq int64) AutosteerFinding {
		return AutosteerFinding{ID: "f", Pattern: "claim-vs-record", Title: "record check", Seq: seq, State: "unchecked"}
	}
	mistake := func(seq int64) AutosteerMistake { return AutosteerMistake{ID: "f", Seq: seq} }
	sess := Session{
		SessionID: "a5ae68a1-3e9b-4929-b581-79bc82026b30",
		Coverage:  ovCoverage(),
		Declarations: ovDeclarations(118,
			map[string]int{"Bash": 102, "Edit": 1, "ListAgents": 1, "Read": 2, "SendMessage": 3, "ToolSearch": 2, "Write": 7},
			map[string]int{"execute": 57, "read": 34, "unknown": 6, "vcs": 12, "write": 9}),
		Executions:  Executions{Recorded: 118},
		Transcripts: ovTranscripts(118),
		Destinations: Destinations{DeclaredHosts: []string{}, Rewritten: []Rewritten{
			{ToolUseID: "toolu_01KbMqTJ13ZuqgfdE7F2Ldpu", ToolName: "SendMessage"},
			{ToolUseID: "toolu_01KoizSHDfk3q8RSXp8HGdWY", ToolName: "SendMessage"},
			{ToolUseID: "toolu_01VHWkEeiP8K6QeRcvLNixJa", ToolName: "SendMessage"},
		}, Novelty: Novelty{Available: true, KnownHosts: 19, Hosts: []string{}}},
		Subagents:      []SubagentSummary{},
		SilentFailures: SilentFailures{Failed: 7, FinalMessageAvailable: true, AbsentWords: []string{}},
		Families: FamilyCoverage{Families: []Family{
			{Name: "gh", Programs: []string{"gh"}, Calls: 4},
			{Name: "git", Programs: []string{"git"}, Calls: 8},
			{Name: "python", Programs: []string{"python3"}, Calls: 11},
		}, NotExercised: []string{"aws", "go"}},
		Chains: Chains{Prompts: make([]Chain, 13), Unattributed: []Link{}, Dropped: []Link{}},
		Autosteer: &Autosteer{
			Stop:  &AutosteerStop{Seq: 366},
			Gates: []AutosteerGate{},
			Delivered: []AutosteerNote{ovNote("claim-vs-record", "record check", 60), ovNote("claim-vs-record", "record check", 112),
				ovNote("claim-vs-record", "record check", 260), ovNote("task-preflight", "preflight", 294),
				ovNote("claim-vs-record", "record check", 334)},
			Findings: []AutosteerFinding{unchecked(60), unchecked(112), unchecked(260), unchecked(334)},
			Checks:   []AutosteerCheck{},
			Lessons:  &AutosteerLessons{},
			Unknown:  []AutosteerUnknown{},
			Mistakes: &AutosteerMistakes{Caught: []AutosteerMistake{}, Reached: []AutosteerMistake{}, Missed: []AutosteerMistake{},
				Unsettled:       []AutosteerMistake{mistake(60), mistake(112), mistake(260), mistake(334)},
				FixedUnprompted: []AutosteerMistake{}},
		},
		Knowledge: &Knowledge{
			Project: "a78e7325",
			Baseline: &KnowledgeBaseline{SessionID: "c7ba1d62-48e3-4d11-9c8e-65a3654128ce",
				RecordedAtUnixMS: time.Date(2026, 11, 8, 13, 32, 0, 0, time.UTC).UnixMilli(), PointSeq: 9124},
			Retained:    Retained{RequiredFamilies: 2, Specs: []SpecHeld{}, Lessons: []LessonRef{}},
			Discovered:  []Change{{What: "gate family first seen in the project: lint", Source: "index"}},
			Invalidated: []Change{},
		},
	}
	return &Report{GeneratedAtUnixMS: ovGenerated(), Redacted: true, Sessions: []Session{sess}}
}

const ovA5Want = `rashomon · session a5ae68a1                                           generated 2026-11-21 08:15 UTC
project a78e7325 · 118 tool calls across 13 prompts · no subagents

┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ ✓ RECORDING COMPLETE                                                                             │
│ All 118 tool calls in the session transcript were recorded with their results. No recording gaps │
│ were detected.                                                                                   │
│                                                                                                  │
│ ! TASK COMPLETION NOT VERIFIED                                                                   │
│ Rashomon cannot confirm the final work: no verification gate (a test, build or lint command      │
│ whose result settles a check) is known for this project.                                         │
│                                                                                                  │
│ A complete recording shows the calls the agent made. It does not show that the work is correct.  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘

WHAT RASHOMON DID

Recorded the whole session, and AutoSteer sent the agent 5 notes: 4 record checks and 1 preflight.
None of the 4 findings was resolved.

Guidance delivered         ✓  5 notes: 4 record checks and 1 preflight.
                              Calls #60, #112, #260, #294, #334
Agent action observed      –  Not recorded. This report has no trace of how the agent responded to
                              the notes.
Outcome verified           !  0 of 4. No gate was named, so none of the 4 concerns could be settled.
Improvement demonstrated   ?  Unknown. 0 mistakes were recorded as caught, which does not mean none
                              occurred.

Rashomon was active, but the available evidence does not establish whether its guidance improved the
final result.

NEEDS ATTENTION

1   Task verification is incomplete
    AutoSteer recorded 4 findings. None was resolved: each names no gate, so no run settles it.
    Why it matters   Rashomon cannot determine whether these concerns were addressed in the final
                     code.
    Next step        Run the project's tests or build on the current code yourself; a finding that
                     names no gate is not settled by a later run.
    Evidence         Calls #60, #112, #260, #334

2   Seven tool calls failed
    Seven tool executions reported a failure during this session.
    Why it matters   Some may have been corrected later. This report does not establish whether a
                     later call corrected each one.
    Next step        Review the failures and confirm whether any affects the final result.
    Evidence         rashomon report --session a5ae68a1-3e9b-4929-b581-79bc82026b30 --chain shows
                     each call in its prompt context.
    Note             The agent's final message contains a failure-related word. That is a keyword
                     match, not proof the failures were acknowledged or resolved.

3   Three tool inputs differed from their declarations
    Three SendMessage calls ran with inputs that differed from what was first declared.
    Why it matters   This may be a normal transformation or an unexpected change. Which one is not
                     established.
    Next step        Read these calls in the session transcript; the record keeps a digest of each
                     input, never its text.
    Evidence         toolu_01KbMqTJ13ZuqgfdE7F2Ldpu
                     toolu_01KoizSHDfk3q8RSXp8HGdWY
                     toolu_01VHWkEeiP8K6QeRcvLNixJa

PROJECT KNOWLEDGE & LEARNING

Already known      2 gate families required (index) and 19 hosts in this project's network baseline.
                   Newest earlier checkpoint: session c7ba1d62, 11-08 13:32. Nothing known was
                   invalidated.
Newly observed     Gate family first seen in the project: lint (index).
Lessons            None proposed, approved or demoted. No lesson was sent to the agent, because none
                   is approved for this project.
Spec               No spec is bound in spec-trust.json, so no spec's rules were checked against the
                   work.

SESSION ACTIVITY

118 calls          execute 57 · read 34 · version control 12 · write 9 · unclassified 6
Tools              Bash 102 · Write 7 · SendMessage 3 · Read 2 · ToolSearch 2 · Edit 1 ·
                   ListAgents 1
Families           python 11 · git 8 · gh 4
Hosts              none named by any call
Observed           The last Stop (call 366) printed no state line. Its significance is not
                   established.

────────────────────────────────────────────────────────────────────────────────────────────────────
DETAILED EVIDENCE

rashomon report --session a5ae68a1-3e9b-4929-b581-79bc82026b30
    Every table of this session's record.
rashomon report --session a5ae68a1-3e9b-4929-b581-79bc82026b30 --chain
    Every call, grouped under the prompt that led to it.
rashomon report --session a5ae68a1-3e9b-4929-b581-79bc82026b30 --json
    The full structured record, for scripts and audits.

Recording integrity: hooks present at start and end · transcript readable · nothing dropped, denied,
unterminated or skipped.
Privacy: the agent's own summary is withheld, and hostnames are redacted under this install's key.
`

// ovNone is a session with nothing to flag: no notes, no failed call, no
// gate named, no input changed, not redacted.
func ovNone() *Report {
	sess := Session{
		SessionID:    "0b1c2d3e-0000-4000-8000-000000000001",
		Coverage:     ovCoverage(),
		Declarations: ovDeclarations(3, map[string]int{"Read": 3}, map[string]int{"read": 3}),
		Executions:   Executions{Recorded: 3},
		Transcripts:  ovTranscripts(3),
		Destinations: Destinations{DeclaredHosts: []string{}, Rewritten: []Rewritten{},
			Novelty: Novelty{Available: false, Reason: "no project root"}},
		Subagents:      []SubagentSummary{},
		SilentFailures: SilentFailures{FinalMessageAvailable: true, AbsentWords: []string{}},
		Families:       FamilyCoverage{Families: []Family{}, NotExercised: []string{"go"}},
		Chains:         Chains{Prompts: make([]Chain, 1), Unattributed: []Link{}, Dropped: []Link{}},
		Autosteer: &Autosteer{Stop: &AutosteerStop{Seq: 12}, Gates: []AutosteerGate{}, Delivered: []AutosteerNote{},
			Findings: []AutosteerFinding{}, Checks: []AutosteerCheck{}, Lessons: &AutosteerLessons{}, Unknown: []AutosteerUnknown{},
			Mistakes: &AutosteerMistakes{Caught: []AutosteerMistake{}, Reached: []AutosteerMistake{}, Missed: []AutosteerMistake{},
				Unsettled: []AutosteerMistake{}, FixedUnprompted: []AutosteerMistake{}}},
		Knowledge: &Knowledge{Project: "b2c3d4e5", Retained: Retained{Prior: true, Specs: []SpecHeld{}, Lessons: []LessonRef{}},
			Discovered: []Change{}, Invalidated: []Change{}},
	}
	return &Report{GeneratedAtUnixMS: ovGenerated(), Sessions: []Session{sess}}
}

const ovNoneWant = `rashomon · session 0b1c2d3e                                           generated 2026-11-21 08:15 UTC
project b2c3d4e5 · 3 tool calls across 1 prompt · no subagents

┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ ✓ RECORDING COMPLETE                                                                             │
│ All 3 tool calls in the session transcript were recorded with their results. No recording gaps   │
│ were detected.                                                                                   │
│                                                                                                  │
│ ! TASK COMPLETION NOT VERIFIED                                                                   │
│ Rashomon cannot confirm the final work: no verification gate (a test, build or lint command      │
│ whose result settles a check) is known for this project.                                         │
│                                                                                                  │
│ A complete recording shows the calls the agent made. It does not show that the work is correct.  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘

WHAT RASHOMON DID

Recorded the whole session, and AutoSteer sent the agent no notes.

Guidance delivered         –  None sent.
Outcome verified           –  No finding was recorded, so there was nothing to settle.
Improvement demonstrated   ?  Unknown. 0 mistakes were recorded as caught, which does not mean none
                              occurred.

NEEDS ATTENTION

None: no unresolved finding, no failed call, no changed input.

PROJECT KNOWLEDGE & LEARNING

Already known      No gate family established, so every gate is required (index). No earlier
                   checkpoint of this project. Nothing known was invalidated.
Newly observed     Nothing was discovered in this session.
Lessons            None proposed, approved or demoted. No lesson was sent to the agent, because none
                   is approved for this project.
Spec               No spec is bound in spec-trust.json, so no spec's rules were checked against the
                   work.

SESSION ACTIVITY

3 calls            read 3
Tools              Read 3
Families           none of the known families ran
Hosts              none named by any call
Observed           The last Stop (call 12) printed no state line. Its significance is not
                   established.

────────────────────────────────────────────────────────────────────────────────────────────────────
DETAILED EVIDENCE

rashomon report --session 0b1c2d3e-0000-4000-8000-000000000001
    Every table of this session's record.
rashomon report --session 0b1c2d3e-0000-4000-8000-000000000001 --chain
    Every call, grouped under the prompt that led to it.
rashomon report --session 0b1c2d3e-0000-4000-8000-000000000001 --json
    The full structured record, for scripts and audits.

Recording integrity: hooks present at start and end · transcript readable · nothing dropped, denied,
unterminated or skipped.
`

// ovPassed is a session whose gate was named and passed after the last edit,
// and whose one finding a green run of that gate resolved.
func ovPassed() *Report {
	sess := Session{
		SessionID:    "c3d4e5f6-0000-4000-8000-000000000002",
		Coverage:     ovCoverage(),
		Declarations: ovDeclarations(40, map[string]int{"Bash": 30, "Edit": 10}, map[string]int{"execute": 30, "write": 10}),
		Executions:   Executions{Recorded: 40},
		Transcripts:  ovTranscripts(40),
		Destinations: Destinations{DeclaredHosts: []string{"proxy.golang.org"}, Rewritten: []Rewritten{},
			Novelty: Novelty{Available: true, KnownHosts: 1, Hosts: []string{}}},
		Subagents:      []SubagentSummary{},
		SilentFailures: SilentFailures{FinalMessageAvailable: true, AbsentWords: []string{}},
		Families:       FamilyCoverage{Families: []Family{{Name: "go", Programs: []string{"go"}, Calls: 30}}, NotExercised: []string{}},
		Chains:         Chains{Prompts: make([]Chain, 2), Unattributed: []Link{}, Dropped: []Link{}},
		Autosteer: &Autosteer{
			Stop:      &AutosteerStop{Seq: 40, Row: "verified", Sentence: "go test passed after the last edit"},
			Gates:     []AutosteerGate{{Name: "go test", Kind: "test", State: "green", LastRunSeq: ovSeq(39), LastGreenSeq: ovSeq(39)}},
			Delivered: []AutosteerNote{ovNote("claim-vs-record", "record check", 20)},
			Findings: []AutosteerFinding{{ID: "f", Pattern: "claim-vs-record", Title: "record check", Gate: "go test", Seq: 20,
				State: "resolved", ResolvedBy: 39}},
			Checks:  []AutosteerCheck{},
			Lessons: &AutosteerLessons{Approved: 1},
			Unknown: []AutosteerUnknown{},
			Mistakes: &AutosteerMistakes{Caught: []AutosteerMistake{{ID: "f", Seq: 20, ResolvedBy: 39}}, Reached: []AutosteerMistake{},
				Missed: []AutosteerMistake{}, Unsettled: []AutosteerMistake{}, FixedUnprompted: []AutosteerMistake{}},
		},
		Knowledge: &Knowledge{
			Project: "d4e5f6a7",
			Baseline: &KnowledgeBaseline{SessionID: "c7ba1d62-48e3-4d11-9c8e-65a3654128ce",
				RecordedAtUnixMS: time.Date(2026, 11, 8, 13, 32, 0, 0, time.UTC).UnixMilli(), PointSeq: 9124},
			Retained:    Retained{RequiredFamilies: 1, Specs: []SpecHeld{}, Lessons: []LessonRef{}},
			Discovered:  []Change{},
			Invalidated: []Change{},
		},
	}
	return &Report{GeneratedAtUnixMS: ovGenerated(), Sessions: []Session{sess}}
}

const ovPassedWant = `rashomon · session c3d4e5f6                                           generated 2026-11-21 08:15 UTC
project d4e5f6a7 · 40 tool calls across 2 prompts · no subagents

┌──────────────────────────────────────────────────────────────────────────────────────────────────┐
│ ✓ RECORDING COMPLETE                                                                             │
│ All 40 tool calls in the session transcript were recorded with their results. No recording gaps  │
│ were detected.                                                                                   │
│                                                                                                  │
│ ✓ GATE PASSED AFTER THE LAST EDIT                                                                │
│ At the last Stop (call 40): go test passed after the last edit. Gates: go test green at #39.     │
│                                                                                                  │
│ A complete recording shows the calls the agent made. It does not show that the work is correct.  │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘

WHAT RASHOMON DID

Recorded the whole session, and AutoSteer sent the agent 1 note: 1 record check. The 1 finding was
resolved.

Guidance delivered         ✓  1 note: 1 record check.
                              Calls #20
Agent action observed      –  Not recorded. This report has no trace of how the agent responded to
                              the note.
Outcome verified           ✓  1 of 1 resolved by a green run of its gate.
Improvement demonstrated   ?  Unknown. 1 mistake was recorded as caught (call #20); no record
                              measures the final result.

Rashomon was active, but the available evidence does not establish whether its guidance improved the
final result.

NEEDS ATTENTION

None: no unresolved finding, no failed call, no changed input.

PROJECT KNOWLEDGE & LEARNING

Already known      1 gate family required (index) and 1 host in this project's network baseline.
                   Newest earlier checkpoint: session c7ba1d62, 11-08 13:32. Nothing known was
                   invalidated.
Newly observed     Nothing was discovered in this session.
Lessons            1 approved, 0 proposed, 0 demoted (this project). None was sent to the agent this
                   session.
Spec               No spec is bound in spec-trust.json, so no spec's rules were checked against the
                   work.

SESSION ACTIVITY

40 calls           execute 30 · write 10
Tools              Bash 30 · Edit 10
Families           go 30
Hosts              1 named by the calls
Observed           The last Stop (call 40) printed: go test passed after the last edit.

────────────────────────────────────────────────────────────────────────────────────────────────────
DETAILED EVIDENCE

rashomon report --session c3d4e5f6-0000-4000-8000-000000000002
    Every table of this session's record.
rashomon report --session c3d4e5f6-0000-4000-8000-000000000002 --chain
    Every call, grouped under the prompt that led to it.
rashomon report --session c3d4e5f6-0000-4000-8000-000000000002 --json
    The full structured record, for scripts and audits.

Recording integrity: hooks present at start and end · transcript readable · nothing dropped, denied,
unterminated or skipped.
`

// ovRender renders rep's overview without color and fails the test on an
// error.
func ovRender(t *testing.T, rep *Report, opts ...OverviewOption) string {
	t.Helper()
	var b bytes.Buffer
	if err := Overview(&b, rep, opts...); err != nil {
		t.Fatalf("Overview: %v", err)
	}
	return b.String()
}

// ovDiff names the first line where got and want part, so a failure points
// at the line and not at two 90-line blobs.
func ovDiff(t *testing.T, name, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Fatalf("%s: line %d differs\n got: %q\nwant: %q", name, i+1, g, w)
		}
	}
}

func TestRPT1a_OwnerLayoutOfTheImageSession(t *testing.T) {
	ovDiff(t, "a5ae68a1", ovRender(t, ovA5()), ovA5Want)
}

func TestRPT1b_NothingToFlag(t *testing.T) {
	ovDiff(t, "none", ovRender(t, ovNone()), ovNoneWant)
}

func TestRPT1c_GateNamedAndPassed(t *testing.T) {
	ovDiff(t, "passed", ovRender(t, ovPassed()), ovPassedWant)
}

// TestRPT1d_ColorOnlyWhenAsked: the plain render carries no escape byte, and
// the colored one paints the check green, the bang marks and item numbers
// amber, the next steps blue and the heads dim, with the words unchanged.
func TestRPT1d_ColorOnlyWhenAsked(t *testing.T) {
	plain := ovRender(t, ovA5())
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain render holds an escape sequence")
	}
	colored := ovRender(t, ovA5(), WithOverviewColor())
	for _, want := range []string{
		"\x1b[32m✓ RECORDING COMPLETE\x1b[0m",
		"\x1b[33m! TASK COMPLETION NOT VERIFIED\x1b[0m",
		"\x1b[33m1\x1b[0m   Task verification is incomplete",
		"\x1b[34mReview the failures and confirm whether any affects the final result.\x1b[0m",
		"\x1b[2mWHAT RASHOMON DID\x1b[0m",
		"\x1b[34mrashomon report --session a5ae68a1-3e9b-4929-b581-79bc82026b30 --json\x1b[0m",
	} {
		if !strings.Contains(colored, want) {
			t.Errorf("colored render lacks %q", want)
		}
	}
	stripped := ovStripANSI(colored)
	ovDiff(t, "colored, escapes stripped", stripped, plain)
}

// ovStripANSI drops every SGR sequence, the only escapes the view writes.
func ovStripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
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
	for _, want := range []string{
		"Outcome verified           !  1 of 4 resolved by a green run of its gate; 2 open: no green run of the\n" +
			"                              gate came after; 1 names no gate, so no run settles it.\n",
		"    AutoSteer recorded 4 findings. 3 were not resolved: 1 names no gate, so no run settles it; 2\n" +
			"    name a gate with no green run of it after.\n",
		"    Evidence         Calls #112, #260, #334\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q", want)
		}
	}
	if strings.Contains(out, "None of the 4 findings") {
		t.Errorf("a resolved finding still reads as none resolved")
	}
	if !strings.Contains(out, "\n3 of the 4 findings were not resolved.\n") && !strings.Contains(out, " 3 of the 4 findings were not\nresolved.\n") {
		t.Errorf("lead paragraph does not count the unresolved findings:\n%s", out)
	}

	for i := range a.Findings {
		a.Findings[i].State, a.Findings[i].Gate = "open", "go test"
	}
	out = ovRender(t, rep)
	if !strings.Contains(out, "    Next step        Run the gate each names on the current code and read its result.\n") {
		t.Errorf("an all-open item lacks the named-gate step:\n%s", out)
	}
}

// TestRPT1f_FailureNoteForms: the note under failed calls says what the
// comparison found, and nothing when the failures were lookups only.
func TestRPT1f_FailureNoteForms(t *testing.T) {
	cases := []struct {
		name string
		sf   SilentFailures
		want string
	}{
		{"fires", SilentFailures{Failed: 2, Fires: true, FinalMessageAvailable: true, AbsentWords: []string{"fail", "error"}},
			"    Note             The agent's final message contains none of the 2 failure words this report\n" +
				"                     looks for.\n"},
		{"unread", SilentFailures{Failed: 2, AbsentWords: []string{}},
			"    Note             The agent's final message could not be read, so it was not compared with the\n" +
				"                     failures.\n"},
		{"lookups only", SilentFailures{Failed: 2, FailedLookups: 2, FinalMessageAvailable: true, AbsentWords: []string{}}, ""},
	}
	for _, c := range cases {
		rep := ovA5()
		rep.Sessions[0].SilentFailures = c.sf
		out := ovRender(t, rep)
		if !strings.Contains(out, "2   Two tool calls failed\n    Two tool executions reported a failure during this session.\n") {
			t.Errorf("%s: the item does not count two failures:\n%s", c.name, out)
		}
		if c.want == "" {
			if strings.Contains(out, "    Note ") {
				t.Errorf("%s: a note printed for lookups only", c.name)
			}
			continue
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: lacks %q", c.name, c.want)
		}
	}
}

// TestRPT1g_RecordingAndStopHeadings: an unverified record and a session with
// no Stop each get their own heading, and the prompts clause names calls
// under no prompt.
func TestRPT1g_RecordingAndStopHeadings(t *testing.T) {
	rep := ovA5()
	s := &rep.Sessions[0]
	s.Coverage.State, s.Coverage.Reasons = store.StateUnverified, []string{ReasonGap}
	s.Chains.Unattributed = []Link{{}, {}}
	s.Autosteer.Stop = nil
	out := ovRender(t, rep)
	for _, want := range []string{
		"project a78e7325 · 118 tool calls across 13 prompts, 2 under no prompt · no subagents\n",
		"│ ! RECORDING INCOMPLETE ",
		"│ The record is not verified complete: gap. ",
		"│ ? NO STOP RECORDED ",
		"│ No Stop was recorded for this session, so no gate state was read. ",
		"Recorded the session, though the record is not verified complete, and AutoSteer sent the agent 5\n",
		"Observed           No Stop was recorded for this session.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q", want)
		}
	}
	if strings.Contains(out, "A complete recording shows") {
		t.Errorf("the complete-recording sentence printed over an unverified record")
	}
}
