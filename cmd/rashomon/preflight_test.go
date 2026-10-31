package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// promptPayload is a UserPromptSubmit payload carrying prompt.
func promptPayload(t *testing.T, prompt string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"session_id": "s-pf", "hook_event_name": "UserPromptSubmit", "transcript_path": "/tmp/t.jsonl",
		"cwd": "/tmp/project", "permission_mode": "default", "prompt": prompt, "prompt_id": "prompt-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPF1j_ReaderCutsThePromptAt16KiB: the catch-up's read takes the
// prompt's first 16 KiB, never splitting a character, says it cut the rest,
// and takes the new prompt's id; a short prompt arrives whole and uncut.
func TestPF1j_ReaderCutsThePromptAt16KiB(t *testing.T) {
	long := strings.Repeat("é", intent.MaxPromptBytes)
	got := readPendingPayload(strings.NewReader(promptPayload(t, long)), time.Second)
	if !got.OK || got.SessionID != "s-pf" || got.PromptID != "prompt-2" || got.PermissionMode != "default" {
		t.Fatalf("payload = ok %v, session %q, prompt id %q, mode %q", got.OK, got.SessionID, got.PromptID, got.PermissionMode)
	}
	if len(got.Prompt) == 0 || len(got.Prompt) > intent.MaxPromptBytes || !utf8.Valid(got.Prompt) || !got.PromptCut {
		t.Errorf("prompt = %d bytes, valid %v, cut %v; want at most %d whole characters, cut",
			len(got.Prompt), utf8.Valid(got.Prompt), got.PromptCut, intent.MaxPromptBytes)
	}
	short := readPendingPayload(strings.NewReader(promptPayload(t, "fix internal/a.go")), time.Second)
	if string(short.Prompt) != "fix internal/a.go" || short.PromptCut {
		t.Errorf("prompt = %q, cut %v; want the whole prompt, uncut", short.Prompt, short.PromptCut)
	}
}

// TestPF1k_OnlyThePendingReadTakesThePrompt: the Stop path's read never
// holds the prompt (the payload type has no field for it), and the catch-up's
// read holds it only in a buffer clearPrompt zeroes.
func TestPF1k_OnlyThePendingReadTakesThePrompt(t *testing.T) {
	in := promptPayload(t, "zebracornflake in internal/a.go")
	if got := readRecapPayload(strings.NewReader(in), time.Second); strings.Contains(fmt.Sprintf("%+v", got), "zebracornflake") {
		t.Errorf("the Stop path's payload holds the prompt: %+v", got)
	}
	got := readPendingPayload(strings.NewReader(in), time.Second)
	if !strings.Contains(string(got.Prompt), "zebracornflake") {
		t.Fatalf("premise: the catch-up read the prompt, got %q", got.Prompt)
	}
	buf := got.Prompt
	clearPrompt(&got)
	if got.Prompt != nil || strings.Trim(string(buf), "\x00") != "" {
		t.Errorf("after clearPrompt the buffer holds %q", buf)
	}
}

// TestPF1u_APromptWithNoSessionDoesNotPanic: a UserPromptSubmit payload with
// no session_id has no run (recapTurn leaves it nil, H-107), so the
// preflight must not ask it for a last seq. recapTurn returns without a
// panic and prints nothing: no note, since there is no session to log it
// under, and no catch-up, since recap.Pending answers false without one.
// cmdRecap's guard would swallow a panic and print the same nothing, so this
// calls recapTurn itself. A guard more than a red: it passes before the
// preflight is wired, and its break is the wired step calling LastSeq on the
// nil run.
func TestPF1u_APromptWithNoSessionDoesNotPanic(t *testing.T) {
	_, installID := benchStarted(t)
	project := t.TempDir()
	stop := filepath.Join(project, "internal", "verify", "stop.go")
	if err := os.MkdirAll(filepath.Dir(stop), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stop, []byte("package verify\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(map[string]any{
		"hook_event_name": "UserPromptSubmit", "transcript_path": "/tmp/t.jsonl", "cwd": project,
		"permission_mode": "default", "prompt": "fix internal/verify/stop.go", "prompt_id": "prompt-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	var rerr error
	panicked := func() (p any) {
		defer func() { p = recover() }()
		rerr = recapTurn([]string{"pending", install.Marker, installID}, bytes.NewReader(in), &stdout, &stderr)
		return nil
	}()
	if panicked != nil || rerr != nil || stdout.Len() != 0 {
		t.Errorf("recap pending with no session_id: panic %v, error %v, stdout %q; want none of them", panicked, rerr, stdout.String())
	}
}

// TestPF1v_ARequestNamingNothingBuildsNoSteerEnv: a request that names no
// path and no identifier, with no lesson to give, stops at intent.Names,
// before any read and before the steer environment is built, so the
// catch-up's preflight costs it what Names costs and nothing more (the
// project read is recapTurn's, readPromptProject, measured on its own).
// Break: build the environment ahead of the guard (harness.Caps' map,
// specDirs, the Getenv reads) and every catch-up pays for it.
func TestPF1v_ARequestNamingNothingBuildsNoSteerEnv(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector's own allocations are counted")
	}
	pp := pendingPayload{recapPayload: recapPayload{CWD: t.TempDir()}, Prompt: []byte("Now add a test for a key that is too long.")}
	if intent.Names(pp.Prompt, false) {
		t.Fatal("premise: the request names a path or an identifier")
	}
	id, now := harness.Identity{ID: harness.ClaudeCode}, time.Now()
	names := testing.AllocsPerRun(100, func() { intent.Names(pp.Prompt, false) })
	note := testing.AllocsPerRun(100, func() { preflightNote(nil, &store.Run{}, &pp, promptProject{}, id, now) })
	if note != names {
		t.Errorf("preflightNote on a request naming nothing: %v allocations a call, want intent.Names' %v", note, names)
	}
}
