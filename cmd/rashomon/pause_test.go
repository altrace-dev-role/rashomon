package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestH82_RecordingStateDistinguishesPausedAbsentAndUnknown calls
// RecordingState directly, in process, exactly as H-82 in
// docs/spec-claude-code-integration.md asks: status is not edited by this
// change, so the exported function is what a reader must be able to trust,
// and this test is what stands in for status calling it.
//
// Break (to see this fail): collapse RecordingPausedNoStore into
// RecordingActive in RecordingState, i.e. return RecordingActive whenever
// store.Paused reports false OR whenever no store exists, regardless of the
// pause marker. That is exactly the collapse H-82 forbids: "I turned it off"
// (paused, no store yet) would read the same as "it was never installed"
// (never paused, no store).
func TestH82_RecordingStateDistinguishesPausedAbsentAndUnknown(t *testing.T) {
	t.Run("active: nothing paused, no store", func(t *testing.T) {
		t.Setenv("RASHOMON_HOME", t.TempDir())
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingActive {
			t.Errorf("state is %q, want %q", state, RecordingActive)
		}
		if !since.IsZero() {
			t.Errorf("since is %v, want zero", since)
		}
	})

	t.Run("paused, no store recorded here yet", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		now := time.Now()
		if _, _, err := store.Pause(root, now); err != nil {
			t.Fatalf("store.Pause: %v", err)
		}
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingPausedNoStore {
			t.Errorf("state is %q, want %q", state, RecordingPausedNoStore)
		}
		if since.UnixMilli() != now.UnixMilli() {
			t.Errorf("since is %v, want %v", since, now)
		}
		// The premise this whole state exists for: pausing before ever
		// recording must not have minted an install identity.
		if _, err := os.Stat(filepath.Join(root, "install.json")); err == nil {
			t.Errorf("a store now exists, which RecordingPausedNoStore promises did not happen")
		}
	})

	t.Run("paused, with a store", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		// store.Open, not `watch`: watch refuses to install a go-test binary,
		// and all this state needs is install.json to exist, which is what
		// storeExistsAt actually checks.
		if _, err := store.Open(root); err != nil {
			t.Fatalf("store.Open: %v", err)
		}
		if _, _, err := store.Pause(root, time.Now()); err != nil {
			t.Fatalf("store.Pause: %v", err)
		}
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingPaused {
			t.Errorf("state is %q, want %q", state, RecordingPaused)
		}
		if since.IsZero() {
			t.Errorf("since is zero, want the pause instant")
		}
	})

	t.Run("unknown: the marker exists but will not parse", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("RASHOMON_HOME", root)
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "pause"), []byte("not a number\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// A marker that cannot be parsed still reads as paused, with an
		// unknown start rather than a guessed one -- RecordingState does not
		// surface a Go error for this shape, since the file is readable and
		// the answer ("paused") is not in doubt, only its start time.
		state, since, err := RecordingState()
		if err != nil {
			t.Fatalf("RecordingState: %v", err)
		}
		if state != RecordingPausedNoStore {
			t.Errorf("state is %q, want %q", state, RecordingPausedNoStore)
		}
		if !since.IsZero() {
			t.Errorf("since is %v, want zero for an unparseable marker", since)
		}
	})
}

// TestH78_CheckPausedReReadsOnEveryCallInOneProcess is H-78 at the level its
// break actually threatens: not across the separate OS processes Claude Code
// spawns per hook call (a cache could never survive exec anyway, since each
// invocation starts a fresh binary with fresh package state), but within one
// process making several calls to checkPaused/run in a row, as this test
// harness -- and a future daemon-shaped caller -- would.
//
// Break (to see this fail): add a package-level `sync.Once`-guarded cache in
// checkPaused, e.g. `once.Do(func() { cachedPaused, _, _ = store.Paused(root)
// })` in place of the direct store.Paused(root) call. Within a single
// process, pause taken effect mid-sequence would then never be seen: exactly
// "a pause mid-session does nothing until restart."
func TestH78_CheckPausedReReadsOnEveryCallInOneProcess(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	if _, err := store.Open(root); err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	call := func(id string) (declared bool) {
		payload := `{"session_id":"sess-78","hook_event_name":"PreToolUse","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_use_id":"` + id + `"}`
		var stdout, stderr bytes.Buffer
		if res := run([]string{"hook"}, strings.NewReader(payload), &stdout, &stderr); res != exitOK {
			t.Fatalf("hook %s: exit %d, stderr %s", id, res, stderr.String())
		}
		b, err := os.ReadFile(filepath.Join(root, "runs", "sess-78", "records.ndjson"))
		if err != nil {
			return false
		}
		return bytes.Contains(b, []byte(`"tool_use_id":"`+id+`"`))
	}

	if !call("toolu_1") {
		t.Fatalf("first call (unpaused) was not recorded")
	}
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}
	if call("toolu_2") {
		t.Errorf("second call recorded although pause had already taken effect -- " +
			"the check is reading a cached answer rather than the marker file")
	}
	if _, _, err := store.Resume(root, time.Now()); err != nil {
		t.Fatalf("store.Resume: %v", err)
	}
	if !call("toolu_3") {
		t.Errorf("third call (after resume, same process) was not recorded")
	}
}

// TestCheckPaused_H80WritesRecordingPausedWithAStore covers H-80 at the
// function level, in process: a paused invocation against a root that
// already holds a store writes exactly one coverage record carrying
// store.ReasonRecordingPaused, and no declaration.
//
// Break: return early inside checkPaused before the AppendCoverage call (or
// equivalently, delete the hook.RecordPaused line). The gap then renders
// identically to a crash, which is H-80's own break.
func TestCheckPaused_H80WritesRecordingPausedWithAStore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	// store.Open, not `watch`: watch refuses to install a go-test binary, and
	// this test needs a store to exist, not a settings.json entry.
	if _, err := store.Open(root); err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	if _, _, err := store.Pause(root, time.Now()); err != nil {
		t.Fatalf("store.Pause: %v", err)
	}

	payload := `{"session_id":"sess-check","hook_event_name":"PreToolUse","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_use_id":"toolu_check"}`
	var stdout, stderr bytes.Buffer
	if res := run([]string{"hook"}, strings.NewReader(payload), &stdout, &stderr); res != exitOK {
		t.Fatalf("hook: exit %d, stderr %s", res, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("hook wrote to stdout while paused: %q", stdout.String())
	}

	recPath := filepath.Join(root, "runs", "sess-check", "records.ndjson")
	if b, err := os.ReadFile(recPath); err == nil && len(bytes.TrimSpace(b)) != 0 {
		t.Errorf("a declaration was written while paused:\n%s", b)
	}

	covPath := filepath.Join(root, "runs", "sess-check", "coverage.ndjson")
	body, err := os.ReadFile(covPath)
	if err != nil {
		t.Fatalf("reading coverage.ndjson: %v", err)
	}
	if !bytes.Contains(body, []byte(`"reason":"recording_paused"`)) {
		t.Errorf("coverage.ndjson does not carry recording_paused:\n%s", body)
	}
	if !bytes.Contains(body, []byte(`"state":"unverified"`)) {
		t.Errorf("coverage.ndjson does not carry state unverified:\n%s", body)
	}
}
