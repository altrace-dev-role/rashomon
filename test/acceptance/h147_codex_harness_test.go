package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-147 -- a run records which harness wrote it, and the report reads the
// run's transcript in that harness's format, or not at all.
//
// The Claude Code check, run on a real Codex rollout in phase 4, found no tool
// ids in it and reported every call "missing from transcript" and the run
// transcript_mismatch, though every call was in the rollout. A report that
// asserts a disagreement that does not exist is the thing this item holds
// down. So the Codex run below has a readable rollout at its transcript_path
// holding every id the store holds, in Codex's own shape: read as Claude
// Code's, it disagrees completely; read as Codex's, it agrees. A harness this
// build does not know gets no check, and says so.
//
// The twin is a Claude Code run with a transcript that really does disagree:
// it must still say so, so the check is skipped by harness, not switched off.
// Its coverage records carry no harness key, which is how H-144's bytes stay
// what they were.
func TestH147_TheTranscriptIsReadInItsHarnessFormat(t *testing.T) {
	const (
		codexSession = "01a15f65-82dd-7020-afc6-a28414a29cad"
		passed       = "exec-3517123b-6ae8-4db9-af45-24db0b10d8a6"
		failed       = "exec-8d96742b-8c96-4f9f-9be0-4900185974e5"
	)
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// The rollout, in the shape phase 4 measured: each command's completion
	// event carries the record's tool_use_id as item.id.
	rollout := filepath.Join(t.TempDir(), "rollout-"+codexSession+".jsonl")
	body := `{"type":"session_meta","payload":{"id":"` + codexSession + `","cwd":"/home/user/hook-test"}}` + "\n"
	for id, status := range map[string]string{passed: "completed", failed: "failed"} {
		body += `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"` +
			id + `","status":"` + status + `"}}}` + "\n"
	}
	if err := os.WriteFile(rollout, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	codex := append([]string{"--harness", "codex"}, e.installArgs()...)
	for _, f := range []struct{ fixture, sub string }{
		{"SessionStart-04.json", "probe start"},
		{"PreToolUse-06.json", "hook"},
		{"PostToolUse-07.json", "post"},
		{"PreToolUse-16.json", "hook"},
		{"PostToolUse-17.json", "post"},
	} {
		var payload map[string]any
		if err := json.Unmarshal(mustRead(t, filepath.Join(moduleRoot, "test", "fixtures", "codex", "payloads", f.fixture)), &payload); err != nil {
			t.Fatal(err)
		}
		payload["transcript_path"] = rollout
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		res := e.run(string(b), nil, append(strings.Fields(f.sub), codex...)...)
		if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
			t.Fatalf("%s: exit %d, stdout %q, stderr %q", f.fixture, res.exitCode, res.stdout, res.stderr)
		}
	}

	cov := e.read(codexSession, "coverage.ndjson")
	if len(cov) != 5 {
		t.Fatalf("%d coverage records, want one per invocation (5)", len(cov))
	}
	for _, c := range cov {
		if c.str("harness") != "codex" {
			t.Errorf("coverage %s: harness %v, want \"codex\"", c.str("phase"), c.fields["harness"])
		}
		assertKeySet(t, c, append(append([]string{}, coverageKeys...), coverageOptionalKeys...))
	}

	type transcript struct {
		IDsInTranscript       *int     `json:"ids_in_transcript"`
		MissingFromStore      []string `json:"missing_from_store"`
		MissingFromTranscript []string `json:"missing_from_transcript"`
	}
	type session struct {
		Harness         *string      `json:"harness"`
		TranscriptCheck *string      `json:"transcript_check"`
		Transcripts     []transcript `json:"transcripts"`
		Coverage        struct {
			Reasons []string `json:"reasons"`
		} `json:"coverage"`
	}
	reportOf := func() session {
		t.Helper()
		var rep struct {
			Sessions []session `json:"sessions"`
		}
		res := e.run("", nil, "report", "--json", "--session", codexSession)
		if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
			t.Fatalf("report: exit %d, %v\n%s", res.exitCode, err, res.stdout)
		}
		return rep.Sessions[0]
	}

	// Read as Codex's, the rollout agrees with the store.
	s := reportOf()
	if s.Harness == nil || *s.Harness != "codex" {
		t.Errorf("report harness %v, want \"codex\"", s.Harness)
	}
	if s.TranscriptCheck != nil {
		t.Errorf("report transcript_check %q on a Codex run, whose rollout this build reads", *s.TranscriptCheck)
	}
	if len(s.Transcripts) != 1 || s.Transcripts[0].IDsInTranscript == nil || *s.Transcripts[0].IDsInTranscript != 2 ||
		len(s.Transcripts[0].MissingFromStore) != 0 || len(s.Transcripts[0].MissingFromTranscript) != 0 {
		t.Errorf("report transcripts %+v; want the rollout read, 2 ids, nothing missing either way", s.Transcripts)
	}
	for _, r := range s.Coverage.Reasons {
		if r == "transcript_mismatch" || r == "execution_mismatch" {
			t.Errorf("a Codex run whose rollout holds every call carries %s", r)
		}
	}
	text := e.run("", nil, "report", "--session", codexSession).stdout
	for _, want := range []string{"\n    ids in transcript: 2\n", "\n    missing from transcript: none\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("text report does not say %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "transcript check:") {
		t.Errorf("text report says the check was not run:\n%s", text)
	}

	// A harness this build does not know: the check is not run, and the
	// stored name is not printed.
	covPath := filepath.Join(e.home, "runs", codexSession, "coverage.ndjson")
	stored := mustRead(t, covPath)
	if err := os.WriteFile(covPath, []byte(strings.ReplaceAll(string(stored), `"harness":"codex"`, `"harness":"cursor"`)), 0o600); err != nil {
		t.Fatal(err)
	}
	s = reportOf()
	if s.Harness == nil || *s.Harness != "unknown" || s.TranscriptCheck == nil || *s.TranscriptCheck != "not_supported" || len(s.Transcripts) != 0 {
		t.Errorf("unknown harness: harness %v, transcript_check %v, %d transcripts; want unknown, not_supported, 0",
			s.Harness, s.TranscriptCheck, len(s.Transcripts))
	}
	text = e.run("", nil, "report", "--session", codexSession).stdout
	if !strings.Contains(text, "\n  transcript check: not supported for this harness (unknown)\n") || strings.Contains(text, "cursor") {
		t.Errorf("text report for an unknown harness:\n%s", text)
	}
	if err := os.WriteFile(covPath, stored, 0o600); err != nil {
		t.Fatal(err)
	}

	// The twin: Claude Code, a call the transcript does not hold.
	e.watched(testSession)
	call := defaultPayload()
	call.TranscriptPath = writeTranscript(t, t.TempDir(), testSession, []string{"toolu_not_in_store"}, nil, nil)
	e.mustHook(call.build(t))
	for _, c := range e.read(testSession, "coverage.ndjson") {
		assertKeySet(t, c, coverageKeys)
	}
	claude := e.report(testSession)
	if !e.hasReason(claude, "transcript_mismatch") {
		t.Errorf("a Claude Code run whose transcript disagrees lost transcript_mismatch: %v", claude.Coverage.Reasons)
	}
	raw := e.run("", nil, "report", "--json", "--session", testSession).stdout
	for _, key := range []string{`"harness"`, `"transcript_check"`} {
		if strings.Contains(raw, key) {
			t.Errorf("a Claude Code report carries %s:\n%s", key, raw)
		}
	}
	if text := e.run("", nil, "report", "--session", testSession).stdout; strings.Contains(text, "transcript check:") ||
		!strings.Contains(text, "missing from transcript: "+testToolUseID) {
		t.Errorf("Claude Code text report:\n%s", text)
	}
}
