package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// BRD-4 (Technical spec 6.2.3): every hook entry this install owns compares
// its installed-for harness with internal/harness.Detect before it opens
// the store. A mismatch -- foreign (Cursor, Codex) or Unknown -- means: no
// record, no run directory, no output, exit 0, and
// harness_counts.json's "<installed_for>@<detected>" counter moves.
//
// These tests exercise the compiled binary end to end (every acceptance
// test's rationale: a function call cannot observe an exit code), proving
// H1's wiring into all four entry points the harness exercises directly --
// hook, post, probe and recap -- rather than internal/hostguard's and
// internal/store's own unit tests, which prove the decision and the
// counter write in isolation but never through cmd/rashomon's dispatch.

// cursorPayload is a PreToolUse/PostToolUse-shaped payload carrying
// cursor_version: internal/harness.Detect's step 1 fires on this alone,
// REGARDLESS of CLAUDE_CODE_ENTRYPOINT, because the fixture's
// transcript_path (sess-1.jsonl, never under e.configDir) never satisfies
// claudeOverride's "entrypoint AND transcript both Claude Code" exception.
func cursorPayload(t *testing.T, sessionID, event, toolUseID string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hook_event_name": event,
		"session_id":      sessionID,
		"transcript_path": "/tmp/transcripts/" + sessionID + ".jsonl",
		"cwd":             "/tmp/project",
		"permission_mode": "default",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "echo hi"},
		"tool_use_id":     toolUseID,
		"cursor_version":  "1.2.3",
	})
	if err != nil {
		t.Fatalf("building cursor payload: %v", err)
	}
	return string(b)
}

// cursorPayloadByConversation is a real Cursor shape: conversation_id, not
// session_id, is the field Cursor keys its own sessions by (Technical spec
// 6.2.5: "runs/<cursor-+conversation_id>"), so session_id is omitted here
// rather than filled with a placeholder -- a fixture that always carried
// one would never exercise the dedup key a session-less Cursor payload
// actually hits.
func cursorPayloadByConversation(t *testing.T, conversationID, event, toolUseID string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hook_event_name": event,
		"conversation_id": conversationID,
		"generation_id":   "gen-1",
		"transcript_path": "/tmp/transcripts/" + conversationID + ".jsonl",
		"cwd":             "/tmp/project",
		"permission_mode": "default",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "echo hi"},
		"tool_use_id":     toolUseID,
		"cursor_version":  "1.2.3",
	})
	if err != nil {
		t.Fatalf("building cursor payload: %v", err)
	}
	return string(b)
}

func runDirExists(e *env, sessionID string) bool {
	_, err := os.Stat(filepath.Join(e.home, "runs", sessionID))
	return err == nil
}

func harnessCountsPath(e *env) string { return filepath.Join(e.home, "harness_counts.json") }

// harnessCountsFile reads harness_counts.json back with the same shape
// store.RecordHarnessMismatch writes, for an assertion on its content --
// the file's own json field names (v, counts, counts[key].days, sessions)
// are store.harnessCounts' contract, not exported, so this is this test's
// own mirror of it, the same way reportSession mirrors report's.
type harnessCountsFile struct {
	V      int `json:"v"`
	Counts map[string]struct {
		Days map[string]struct {
			Seen      int `json:"seen"`
			StoodDown int `json:"stood_down"`
		} `json:"days"`
		LastStoodDown string   `json:"last_stood_down"`
		LastEvidence  []string `json:"last_evidence"`
	} `json:"counts"`
	Sessions []string `json:"sessions"`
}

func readHarnessCountsFile(t *testing.T, e *env) harnessCountsFile {
	t.Helper()
	b, err := os.ReadFile(harnessCountsPath(e))
	if err != nil {
		t.Fatalf("reading harness_counts.json: %v", err)
	}
	var hc harnessCountsFile
	if err := json.Unmarshal(b, &hc); err != nil {
		t.Fatalf("harness_counts.json is not valid JSON: %v\n%s", err, b)
	}
	return hc
}

// TestBRD4_HookStandsDownOnUnknownHarness: an entrypoint outside the
// 2.1.280 allowlist, with the fixture's own transcript_path (never under
// e.configDir), satisfies none of Detect's five steps but the last --
// Unknown -- and BRD-4's own text says to stand down on it, same as a
// named foreign harness: "stand down on foreign OR UNKNOWN harnesses".
func TestBRD4_HookStandsDownOnUnknownHarness(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	res := e.hook(defaultPayload().build(t), "CLAUDE_CODE_ENTRYPOINT=")
	if res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("stood-down hook wrote stdout %q, want none", res.stdout)
	}
	if len(e.declarations(testSession)) != 0 {
		t.Errorf("stood-down hook recorded a declaration")
	}

	hc := readHarnessCountsFile(t, e)
	entry, ok := hc.Counts["claude@unknown"]
	if !ok {
		t.Fatalf("harness_counts.json has no claude@unknown entry: %+v", hc.Counts)
	}
	var total int
	for _, d := range entry.Days {
		total += d.StoodDown
	}
	if total != 1 {
		t.Errorf("claude@unknown stood_down total = %d, want 1", total)
	}
}

// TestBRD4_HookStandsDownOnCursorMarker: Detect's step 1 fires on
// cursor_version alone, independent of CLAUDE_CODE_ENTRYPOINT, because
// this payload's transcript_path never resolves under e.configDir (see
// cursorPayload's own doc on claudeOverride).
func TestBRD4_HookStandsDownOnCursorMarker(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	// watched's own probe already created a run directory for testSession;
	// this test needs that absent, so it uses a session id of its own.
	const sessionID = "sess-cursor"

	res := e.hook(cursorPayload(t, sessionID, "PreToolUse", "toolu_cursor"))
	if res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if runDirExists(e, sessionID) {
		t.Errorf("stood-down hook created a run directory for %s", sessionID)
	}
	if len(e.declarations(sessionID)) != 0 {
		t.Errorf("stood-down hook recorded a declaration for %s", sessionID)
	}

	hc := readHarnessCountsFile(t, e)
	entry, ok := hc.Counts["claude@cursor"]
	if !ok {
		t.Fatalf("harness_counts.json has no claude@cursor entry: %+v", hc.Counts)
	}
	found := false
	for _, ev := range entry.LastEvidence {
		if ev == "cursor_version" {
			found = true
		}
	}
	if !found {
		t.Errorf("claude@cursor entry's last_evidence = %v, want cursor_version", entry.LastEvidence)
	}
}

// TestBRD4_SessionlessCursorStandDownsAreCountedPerConversation: a real
// Cursor payload carries no session_id at all (Cursor keys its own runs by
// conversation_id, 6.2.5), so the dedup key RecordHarnessMismatch digests
// must come from conversation_id for a Cursor-detected stand-down, not from
// an absent session_id. Two stand-downs with DIFFERENT conversation_ids
// must both count: digesting the empty string for both (the bug this pins)
// made the second always read as "already counted", so the total stuck at
// 1 no matter how many session-less Cursor calls followed the first.
func TestBRD4_SessionlessCursorStandDownsAreCountedPerConversation(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	for i, conv := range []string{"conv-aaa", "conv-bbb"} {
		res := e.hook(cursorPayloadByConversation(t, conv, "PreToolUse", "toolu_"+conv))
		if res.exitCode != 0 {
			t.Fatalf("hook %d: exit %d, stderr %q", i, res.exitCode, res.stderr)
		}
	}

	hc := readHarnessCountsFile(t, e)
	entry, ok := hc.Counts["claude@cursor"]
	if !ok {
		t.Fatalf("harness_counts.json has no claude@cursor entry: %+v", hc.Counts)
	}
	var total int
	for _, d := range entry.Days {
		total += d.StoodDown
	}
	if total != 2 {
		t.Errorf("claude@cursor stood_down total = %d, want 2 (one per conversation_id)", total)
	}
}

// TestBRD4_SessionlessCursorStandDownsDedupeBySameConversation is the
// companion case: the SAME conversation_id, sent twice, must still
// dedupe to one count -- the fix for the case above must not simply stop
// deduping session-less Cursor calls altogether.
func TestBRD4_SessionlessCursorStandDownsDedupeBySameConversation(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	for i := 0; i < 2; i++ {
		res := e.hook(cursorPayloadByConversation(t, "conv-same", "PreToolUse", "toolu_same"))
		if res.exitCode != 0 {
			t.Fatalf("hook %d: exit %d, stderr %q", i, res.exitCode, res.stderr)
		}
	}

	hc := readHarnessCountsFile(t, e)
	entry, ok := hc.Counts["claude@cursor"]
	if !ok {
		t.Fatalf("harness_counts.json has no claude@cursor entry: %+v", hc.Counts)
	}
	var total int
	for _, d := range entry.Days {
		total += d.StoodDown
	}
	if total != 1 {
		t.Errorf("claude@cursor stood_down total = %d, want 1 (same conversation_id deduped)", total)
	}
}

// TestBRD4_PostStandsDownOnUnknownHarness mirrors the hook case for
// PostToolUse: no execution recorded, same counter key.
func TestBRD4_PostStandsDownOnUnknownHarness(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	res := e.post(defaultPost().build(t), "CLAUDE_CODE_ENTRYPOINT=")
	if res.exitCode != 0 {
		t.Fatalf("post: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if len(e.executions(testSession)) != 0 {
		t.Errorf("stood-down post recorded an execution")
	}
	if _, ok := readHarnessCountsFile(t, e).Counts["claude@unknown"]; !ok {
		t.Errorf("post's stand-down did not move claude@unknown")
	}
}

// TestBRD4_ProbeStandsDownOnUnknownHarness: probe's SessionStart/SessionEnd
// phases are the one entry point whose payload never carries a tool call,
// so the only observable is coverage.ndjson staying empty and the exit
// code -- there is no declaration or execution to check.
func TestBRD4_ProbeStandsDownOnUnknownHarness(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	const sessionID = "sess-probe-unknown"
	res := e.probe("start", sessionID, "CLAUDE_CODE_ENTRYPOINT=")
	if res.exitCode != 0 {
		t.Fatalf("probe: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if runDirExists(e, sessionID) {
		t.Errorf("stood-down probe created a run directory for %s", sessionID)
	}
	if _, ok := readHarnessCountsFile(t, e).Counts["claude@unknown"]; !ok {
		t.Errorf("probe's stand-down did not move claude@unknown")
	}
}

// TestBRD4_RecapStandsDownOnCursorMarker: recap reads its harness signal
// from the same single stdin stream as last_assistant_message
// (readFinalMessage's withHarness paths, cmd/rashomon's recapHostGuard),
// never a second read -- this is the one acceptance proof that path
// actually decides anything, rather than only the unit tests on
// syntheticHarnessPayload and recapHostGuard in cmd/rashomon's own package.
func TestBRD4_RecapStandsDownOnCursorMarker(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	b, err := json.Marshal(map[string]any{
		"hook_event_name":        "Stop",
		"session_id":             testSession,
		"transcript_path":        "/tmp/transcripts/" + testSession + ".jsonl",
		"last_assistant_message": "done",
		"cursor_version":         "1.2.3",
	})
	if err != nil {
		t.Fatalf("building recap payload: %v", err)
	}

	line, wrote := e.recapLine(string(b))
	if wrote {
		t.Errorf("stood-down recap wrote a line: %q", line)
	}

	hc := readHarnessCountsFile(t, e)
	if _, ok := hc.Counts["claude@cursor"]; !ok {
		t.Errorf("recap's stand-down did not move claude@cursor: %+v", hc.Counts)
	}
}

// TestBRD4_HarnessCountsFileIsPrivate: 6.2.1's own "0600" and "at most 64
// KiB" bounds, read back from the file a stand-down actually wrote --
// internal/store's own unit tests already cover the cap under contention
// and eviction; this is the one acceptance-level proof the file this
// install's hooks actually produce is as private as the spec requires.
func TestBRD4_HarnessCountsFileIsPrivate(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	if res := e.hook(defaultPayload().build(t), "CLAUDE_CODE_ENTRYPOINT="); res.exitCode != 0 {
		t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	info, err := os.Stat(harnessCountsPath(e))
	if err != nil {
		t.Fatalf("statting harness_counts.json: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("harness_counts.json mode = %o, want 0600", info.Mode().Perm())
	}
	if info.Size() > 64<<10 {
		t.Errorf("harness_counts.json is %d bytes, over the 64 KiB cap", info.Size())
	}
}

// TestBRD4_ClaudeEntryNeverStandsDown: the control. Every other acceptance
// test in this package already exercises this path on every one of its
// hook/post/probe/recap calls (environ's own CLAUDE_CODE_ENTRYPOINT=cli),
// so this is a single, explicit statement of the invariant their mass
// coverage leans on, not the only proof of it.
func TestBRD4_ClaudeEntryNeverStandsDown(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))

	if len(e.declarations(testSession)) != 1 {
		t.Fatalf("got %d declarations, want 1 (hook must not have stood down)", len(e.declarations(testSession)))
	}
	if _, err := os.Stat(harnessCountsPath(e)); !os.IsNotExist(err) {
		t.Errorf("harness_counts.json exists for an install that never stood down: %v", err)
	}
}
