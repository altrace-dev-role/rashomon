package report

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRubricHLD4_AssembleTakesWhatTheReadStepRead is the pure half of the
// split: Assemble renders a session from what the read step handed it and
// opens nothing. The transcript path below names no file on any machine, so
// a report that shows its ids, its result and its final message can only
// have taken them from the input -- and one that went looking on disk would
// render the transcript unreadable and the account unavailable.
func TestRubricHLD4_AssembleTakesWhatTheReadStepRead(t *testing.T) {
	const path = "/nonexistent/rashomon-hld4/session.jsonl"
	run := &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "toolu_1", ToolName: "Bash", TranscriptPath: path}},
		Executions:   []store.Execution{{ToolUseID: "toolu_1", Outcome: store.ExecFailed}},
	}
	set := NewTranscriptSet()
	if err := TranscriptIDs(strings.NewReader(
		`{"message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}`+"\n"+
			`{"message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}`+"\n"), set); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	rep := Assemble(Input{Sessions: []SessionInput{{
		Dir:         "hld4",
		Run:         run,
		Transcripts: map[string]TranscriptRead{path: {Set: set, Final: "All done.", FinalRead: true}},
		Novelty:     Novelty{Reason: "not asked"},
	}}}, now)

	if len(rep.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(rep.Sessions))
	}
	s := rep.Sessions[0]
	if len(s.Transcripts) != 1 || !s.Transcripts[0].Readable {
		t.Fatalf("transcripts = %+v; the read step's set for %s did not reach the accounting", s.Transcripts, path)
	}
	tr := s.Transcripts[0]
	if tr.IDsInTranscript == nil || *tr.IDsInTranscript != 1 || tr.ResultsInTranscript == nil || *tr.ResultsInTranscript != 1 {
		t.Errorf("ids_in_transcript = %v, results_in_transcript = %v; want 1 and 1 from the input", tr.IDsInTranscript, tr.ResultsInTranscript)
	}
	if tr.Files == nil || *tr.Files != 1 {
		t.Errorf("files = %v, want the one file TranscriptIDs read", tr.Files)
	}
	if !s.Account.Available || s.Account.Text != "All done." {
		t.Errorf("account = %+v; the read step's final message did not reach it", s.Account)
	}
	if !s.SilentFailures.Fires {
		t.Error("a failed call and a final message acknowledging nothing did not fire the silent-failure line")
	}
	if s.Destinations.Novelty.Reason != "not asked" {
		t.Errorf("novelty = %+v; the read step's answer did not reach the destinations", s.Destinations.Novelty)
	}
	if rep.GeneratedAtUnixMS != now.UnixMilli() || rep.ReportVersion != CurrentReportVersion {
		t.Errorf("generated_at = %d, report_version = %d", rep.GeneratedAtUnixMS, rep.ReportVersion)
	}
}

// TestRubricHLD4_AMissingTranscriptIsAbsentAndAnUnreadOneIsUnreadable: the
// read step says why a transcript has no set. A file that does not exist and
// one that could not be read both render unreadable, with every count null,
// and a path the read step never reached is not taken to be readable.
func TestRubricHLD4_AMissingTranscriptIsAbsentAndAnUnreadOneIsUnreadable(t *testing.T) {
	for name, read := range map[string]TranscriptRead{
		"absent":   {Err: fs.ErrNotExist},
		"unread":   {Err: fs.ErrPermission},
		"no entry": {},
	} {
		run := &store.Run{Declarations: []store.Declaration{{ToolUseID: "toolu_1", ToolName: "Bash", TranscriptPath: "/x.jsonl"}}}
		reads := map[string]TranscriptRead{"/x.jsonl": read}
		if name == "no entry" {
			reads = nil
		}
		s := build(run, reads)
		if len(s.Transcripts) != 1 || s.Transcripts[0].Readable || s.Transcripts[0].IDsInTranscript != nil {
			t.Errorf("%s: transcripts = %+v, want one unreadable group with null counts", name, s.Transcripts)
		}
		if acct := buildAccount(run, reads); acct.Available {
			t.Errorf("%s: account available from a transcript that was not read", name)
		}
	}
}

// TestRubricHLD4_TranscriptIDsReadsOneReader: the reader seam counts the
// file it was handed and gathers into the set it was handed, so the read
// step can add a session's subagent files one at a time.
func TestRubricHLD4_TranscriptIDsReadsOneReader(t *testing.T) {
	set := NewTranscriptSet()
	for _, body := range []string{
		`{"message":{"content":[{"type":"tool_use","id":"toolu_a"}]}}` + "\n",
		`{"message":{"content":[{"type":"tool_use","id":"toolu_b"}]}}` + "\n",
	} {
		if err := TranscriptIDs(strings.NewReader(body), set); err != nil {
			t.Fatal(err)
		}
	}
	if set.Files != 2 || !set.IDs["toolu_a"] || !set.IDs["toolu_b"] {
		t.Errorf("set = %+v, want two files and both ids", set)
	}
}

// TestRubricHLD4_TranscriptPathsAreTheDistinctNamedPathsSorted is the list
// the read step reads, which must be the list the accounting groups by.
func TestRubricHLD4_TranscriptPathsAreTheDistinctNamedPathsSorted(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		{ToolUseID: "1", TranscriptPath: "/b.jsonl"},
		{ToolUseID: "2", TranscriptPath: ""},
		{ToolUseID: "3", TranscriptPath: "/a.jsonl"},
		{ToolUseID: "4", TranscriptPath: "/b.jsonl"},
	}}
	got := TranscriptPaths(run)
	if strings.Join(got, ",") != "/a.jsonl,/b.jsonl" {
		t.Errorf("TranscriptPaths = %v, want [/a.jsonl /b.jsonl]", got)
	}
}

// TestRubricHLD4_TheBaselineQuestionLeavesOutTheClientPlane: the read step
// asks the project baseline the question NoveltyQueryOf decides, so the rule
// that the client's own plane is never a project's new destination has to
// live in that question, not in the read step that carries it.
func TestRubricHLD4_TheBaselineQuestionLeavesOutTheClientPlane(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "toolu_1", ToolName: "Bash", SessionID: "hld4",
			Hosts: []string{"a.example", "api.anthropic.com"},
		}},
		Coverage: []store.Coverage{{Phase: store.PhaseStart, RecordedAtMS: 1, CWD: "/project", SessionID: "hld4"}},
	}
	q, unavailable := NoveltyQueryOf(run, "/root", nil)
	if unavailable != nil {
		t.Fatalf("no query: %s", unavailable.Reason)
	}
	if strings.Join(q.Hosts, ",") != "a.example" {
		t.Errorf("query hosts = %v, want [a.example]: the client plane is not asked about", q.Hosts)
	}
}

// TestRubricHLD4_TheBaselinesAnswerCarriesNoErrorText: the read step hands
// Assemble the baseline's error as it came, path and all; the pure half
// decides what a reader sees, and that is a reason, never the error's text.
func TestRubricHLD4_TheBaselinesAnswerCarriesNoErrorText(t *testing.T) {
	const text = "open /home/someone/baseline.json: permission denied"
	n := AnsweredNovelty(false, 0, nil, errors.New(text))
	if n.Available || n.Reason == "" || strings.Contains(n.Reason, "someone") || strings.Contains(n.Reason, "denied") {
		t.Errorf("novelty = %+v, want unavailable with a reason that is not %q", n, text)
	}
}
