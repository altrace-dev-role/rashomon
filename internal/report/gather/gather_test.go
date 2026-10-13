package gather

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

const t0ms = int64(1789000000000)

// openWindowCaveat is the sandbox line's words for a window with no end.
const openWindowCaveat = " (no end record, so events up to the end of the trail are counted)"

// locate returns the text surrounding the first hit, so a failure names where
// the leak is rather than only that there is one.
func locate(hay, needle string) string {
	i := strings.Index(hay, needle)
	if i < 0 {
		return ""
	}
	return "..." + hay[max(0, i-120):min(len(hay), i+len(needle)+120)] + "..."
}

// writeLines writes a transcript file, creating its directory.
func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRubricHLD4_BuildReadsTheTranscriptsTheRunNames is the read step end to
// end: Build opens the transcript a session's declarations name and the
// subagent transcript beside it, and the report's accounting, account and
// silent-failure line all carry what was read. report.Assemble opens nothing,
// so a read step that stopped reading would leave the transcript unreadable
// and the account unavailable.
func TestRubricHLD4_BuildReadsTheTranscriptsTheRunNames(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s.jsonl")
	writeLines(t, main,
		`{"message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_main"}]}}`,
		`{"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_main","is_error":true,"content":"exit 1"}]}}`,
		`{"message":{"role":"assistant","content":[{"type":"text","text":"All done."}]}}`)
	writeLines(t, filepath.Join(dir, "s", "subagents", "agent-a.jsonl"),
		`{"message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_sub"}]}}`)

	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "hld4-session"
	if err := st.AppendCoverage(store.Coverage{
		Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: store.PhaseStart,
		RecordedAtMS: t0ms - 1, CWD: t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"toolu_main", "toolu_sub"} {
		if err := st.AppendDeclaration(store.Declaration{
			Type: "declaration", SchemaVersion: 2, SessionID: id, ToolUseID: d, ToolName: "Bash",
			TranscriptPath: main, RecordedAtMS: t0ms, Hosts: []string{"a.example"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.AppendExecution(store.Execution{
		Type: "execution", SchemaVersion: 2, SessionID: id, ToolUseID: "toolu_main",
		Outcome: store.ExecFailed, RecordedAtMS: t0ms + 1,
	}); err != nil {
		t.Fatal(err)
	}

	rep, err := Build(st, id, time.UnixMilli(t0ms+10))
	if err != nil {
		t.Fatal(err)
	}
	s := rep.Sessions[0]
	if len(s.Transcripts) != 1 || !s.Transcripts[0].Readable {
		t.Fatalf("transcripts = %+v; the read step did not read %s", s.Transcripts, main)
	}
	tr := s.Transcripts[0]
	if tr.Files == nil || *tr.Files != 2 || tr.IDsInTranscript == nil || *tr.IDsInTranscript != 2 {
		t.Errorf("files = %v, ids = %v; want the main transcript and its subagent, two ids", tr.Files, tr.IDsInTranscript)
	}
	if len(tr.MissingFromStore)+len(tr.MissingFromTranscript) != 0 {
		t.Errorf("missing from store %v, from transcript %v; want none", tr.MissingFromStore, tr.MissingFromTranscript)
	}
	if !s.Account.Available || s.Account.Text != "All done." {
		t.Errorf("account = %+v; the final message was not read", s.Account)
	}
	if !s.SilentFailures.Fires {
		t.Error("a failed call and a summary acknowledging nothing did not fire the silent-failure line")
	}
	if n := s.Destinations.Novelty; !n.Available || !n.Established {
		t.Errorf("novelty = %+v; the read step did not ask the project baseline", n)
	}
}

// TestRubricHLD4_AnUnreadableSubagentCostsTheCountNotTheAccount: a subagent
// file that cannot be read makes the session's id count unknown, since a count
// over some of its files is not its count, and leaves the main transcript's
// final message readable.
func TestRubricHLD4_AnUnreadableSubagentCostsTheCountNotTheAccount(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s.jsonl")
	writeLines(t, main, `{"message":{"role":"assistant","content":[{"type":"text","text":"Finished."}]}}`)
	// A dangling link: listed like any agent file, and not there to open.
	sub := filepath.Join(dir, "s", "subagents", "agent-a.jsonl")
	if err := os.MkdirAll(filepath.Dir(sub), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "gone.jsonl"), sub); err != nil {
		t.Fatal(err)
	}
	read := readTranscript(main)
	if read.Err == nil || read.Set != nil {
		t.Errorf("read = %+v; want the subagent's error and no set", read)
	}
	if !read.FinalRead || read.Final != "Finished." {
		t.Errorf("final = %q (%v); the account must survive a subagent the ids could not read", read.Final, read.FinalRead)
	}
}

// TestNono_ASessionWithNoEndRecordReportsAnOpenWindow drives Build: the
// window comes from the run's coverage records, and a run with a start and
// no end must reach the sandbox line as open.
func TestNono_ASessionWithNoEndRecordReportsAnOpenWindow(t *testing.T) {
	at := firstNonoEvent(t)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const id = "no-end-session"
	if err := st.AppendCoverage(store.Coverage{
		Type: "coverage", SchemaVersion: 2, SessionID: id, Phase: store.PhaseStart,
		RecordedAtMS: at.Add(-time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(st, id, at.Add(time.Hour), WithNonoTrail(nonoFixture))
	if err != nil {
		t.Fatal(err)
	}
	n := rep.Sessions[0].Nono
	if !n.Observed || !n.WindowOpen {
		t.Errorf("observed=%v window_open=%v, want true, true (%s)", n.Observed, n.WindowOpen, n.Reason)
	}
	var b strings.Builder
	if err := report.Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), openWindowCaveat) {
		t.Errorf("the rendered report does not say the window has no end:\n%s", b.String())
	}
}

// TestNovelty_UsesTheRunsOwnCWDNotTheReadersCWD. A report run from a different
// directory must not key a past session to the reader's project — that would
// make novelty depend on where somebody happened to stand when they read it.
func TestNovelty_UsesTheRunsOwnCWDNotTheReadersCWD(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	run := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t1", ToolName: "Bash", SessionID: "sess-1", Hosts: []string{"a.example"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms, CWD: project, SessionID: "sess-1",
		}},
	}

	first := novelty(run, root, nil)
	if !first.Available {
		t.Fatalf("novelty unavailable: %s", first.Reason)
	}
	if !first.Established {
		t.Error("the first session in a project should establish the baseline")
	}

	// A different session in the SAME project, later, naming a new host.
	run2 := &store.Run{
		Declarations: []store.Declaration{{
			ToolUseID: "t2", ToolName: "Bash", SessionID: "sess-2",
			Hosts: []string{"a.example", "b.example"},
		}},
		Coverage: []store.Coverage{{
			Phase: store.PhaseStart, RecordedAtMS: t0ms + 3600000, CWD: project, SessionID: "sess-2",
		}},
	}
	second := novelty(run2, root, nil)

	if second.Established {
		t.Error("the second session established the baseline again")
	}
	if len(second.Hosts) != 1 || second.Hosts[0] != "b.example" {
		t.Errorf("novelty hosts = %v, want [b.example]", second.Hosts)
	}
}

// TestNovelty_ClientPlaneIsNeverNovel. Folding the client's own control plane
// into a project baseline would report api.anthropic.com as a new destination
// on the first session of every project — the novelty line's most obvious way
// to make itself worthless.
func TestNovelty_ClientPlaneIsNeverNovel(t *testing.T) {
	project := t.TempDir()
	root := t.TempDir()
	mk := func(id string, at int64, hosts ...string) *store.Run {
		return &store.Run{
			Declarations: []store.Declaration{{
				ToolUseID: "t", ToolName: "Bash", SessionID: id, Hosts: hosts,
			}},
			Coverage: []store.Coverage{{
				Phase: store.PhaseStart, RecordedAtMS: at, CWD: project, SessionID: id,
			}},
		}
	}
	// Establish, then a later session whose only new host is client plane.
	novelty(mk("sess-1", t0ms, "a.example"), root, nil)
	n := novelty(mk("sess-2", t0ms+3600000, "a.example", "api.anthropic.com", "127.0.0.1"), root, nil)

	if len(n.Hosts) != 0 {
		t.Errorf("novelty hosts = %v, want empty: the client plane and loopback are not "+
			"new destinations for a project", n.Hosts)
	}
}

// TestTranscript_WorkflowSubagentTranscriptsAreRead: a workflow's subagents
// write <session>/subagents/workflows/<run>/agent-*.jsonl, two levels below the
// plain subagent files. A one-level glob read 35 of a real session's 407
// transcripts, and the 372 it skipped made every call they held read as
// recorded-but-not-in-the-transcript.
func TestTranscript_WorkflowSubagentTranscriptsAreRead(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s.jsonl")
	write := func(path, id string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		line := `{"message":{"role":"assistant","content":[{"type":"tool_use","id":"` + id + `"}]}}` + "\n"
		if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(main, "toolu_main")
	write(filepath.Join(dir, "s", "subagents", "agent-plain.jsonl"), "toolu_plain")
	write(filepath.Join(dir, "s", "subagents", "workflows", "run1", "agent-deep.jsonl"), "toolu_deep")

	set, err := readIDSet(main)
	if err != nil {
		t.Fatal(err)
	}
	ids, files := set.IDs, set.Files
	if files != 3 {
		t.Errorf("read %d transcripts, want 3: the main one, a plain subagent, a workflow subagent", files)
	}
	for _, id := range []string{"toolu_main", "toolu_plain", "toolu_deep"} {
		if !ids[id] {
			t.Errorf("%s missing from the transcript ids", id)
		}
	}
}
