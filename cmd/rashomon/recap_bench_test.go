package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// The recap benchmarks read a session from payload.WriteSession, the same
// generator internal/digest's BenchmarkBuild reads.

// benchStopPayload is the Stop payload for the turn that ended with
// payload.FinalMessage.
func benchStopPayload(tb testing.TB) []byte {
	return benchJSON(tb, struct {
		benchHead
		StopHookActive       bool   `json:"stop_hook_active"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}{newBenchHead("Stop"), false, payload.FinalMessage})
}

// benchPromptPayload is the UserPromptSubmit payload of the next prompt.
func benchPromptPayload(tb testing.TB) []byte {
	return benchJSON(tb, struct {
		benchHead
		Prompt string `json:"prompt"`
	}{newBenchHead("UserPromptSubmit"), "Now add a test for a key that is too long."})
}

// benchRecapStore installs and writes a session of calls into a store at root.
func benchRecapStore(b *testing.B, root, cfg string, calls int) (string, payload.Turn) {
	b.Helper()
	installID := benchInstall(b, root, cfg)
	turn, err := payload.WriteSession(root, calls, payload.Prompts)
	if err != nil {
		b.Fatal(err)
	}
	return installID, turn
}

// benchCheckRecap is the recap work-done check: every call printed one line,
// and the last is the line for the generated turn -- the refused call, and on
// Stop the failed calls set against a final message that claims a pass.
func benchCheckRecap(b *testing.B, stdout, stderr *benchSink, pending bool, turn payload.Turn) {
	b.Helper()
	if stdout.writes != b.N || stderr.writes != 0 {
		b.Fatalf("%d lines printed in %d calls, stderr %q", stdout.writes, b.N, stderr.lastWrite())
	}
	var out struct {
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal(stdout.lastWrite(), &out); err != nil {
		b.Fatalf("recap printed %q: %v", stdout.lastWrite(), err)
	}
	want := []string{" rashomon: ", "1 declaration without recorded execution", "rashomon report --session " + benchSessionID}
	if pending {
		want[0] = " rashomon, previous turn: "
	} else {
		want = append(want, fmt.Sprintf("%d recorded failure", turn.Failed))
	}
	for _, w := range want {
		if !strings.Contains(out.SystemMessage, w) {
			b.Fatalf("recap line %q does not say %q", out.SystemMessage, w)
		}
	}
}

// benchRecapInput is the command line and payload of Stop's recap, or of the
// catch-up's, as a settings entry runs them.
func benchRecapInput(tb testing.TB, installID string, pending bool) ([]string, []byte) {
	tb.Helper()
	if pending {
		return []string{"pending", install.Marker, installID}, benchPromptPayload(tb)
	}
	return []string{install.Marker, installID}, benchStopPayload(tb)
}

// benchRecap runs recap on a session of calls, generated before the timer
// starts. recap.json is removed before every call, so each is the turn's
// first: a second Stop for the same turn prints nothing (H-101), and pending
// finds no turn left unchecked.
func benchRecap(b *testing.B, pending bool, calls int) {
	root, cfg := b.TempDir(), b.TempDir()
	benchSetenv(b, root, cfg)
	installID, turn := benchRecapStore(b, root, cfg, calls)
	args, input := benchRecapInput(b, installID, pending)
	state := filepath.Join(root, "recap.json")
	rd := bytes.NewReader(input)
	var stdout, stderr benchSink
	b.ReportAllocs()
	b.SetBytes(turn.Bytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.Remove(state); err != nil && !errors.Is(err, fs.ErrNotExist) {
			b.Fatal(err)
		}
		rd.Reset(input)
		b.StartTimer()
		if code := cmdRecap(args, rd, &stdout, &stderr); code != exitOK {
			b.Fatalf("recap exited %d", code)
		}
	}
	b.StopTimer()
	benchCheckRecap(b, &stdout, &stderr, pending, turn)
}

// BenchmarkRecap is Stop on a session of 10, 100 and 1,000 calls: the turn's
// digest, the line and recap.json. SetBytes is the session's store bytes.
func BenchmarkRecap(b *testing.B) {
	for _, calls := range []int{10, 100, 1000} {
		b.Run(strconv.Itoa(calls), func(b *testing.B) { benchRecap(b, false, calls) })
	}
}

// BenchmarkRecapPending is the UserPromptSubmit catch-up on a turn no Stop
// reached: the digest, the pending check and the previous-turn line. Its
// prompt names nothing, so the preflight stops at intent.Names. /preflight is
// the catch-up for a prompt that names a file of a project on disk:
// the spec walk, the index and steer-log reads, the resolve and its lookup,
// the note's render and its steer line, which is removed before each call so
// that every call gives the session's first note.
func BenchmarkRecapPending(b *testing.B) {
	for _, calls := range []int{10, 1000} {
		b.Run(strconv.Itoa(calls), func(b *testing.B) { benchRecap(b, true, calls) })
	}
	b.Run("preflight", benchRecapPreflight)
}

// benchRecapPreflight is BenchmarkRecapPending/preflight: the 10-call
// session, consented to notes, and a prompt naming internal/verify/stop.go
// of a project that holds it.
func benchRecapPreflight(b *testing.B) {
	root, cfg := b.TempDir(), b.TempDir()
	benchSetenv(b, root, cfg)
	installID, turn := benchRecapStore(b, root, cfg, 10)
	if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
		b.Fatal(err)
	}
	project := b.TempDir()
	for _, f := range []string{"go.mod", "internal/verify/stop.go"} {
		p := filepath.Join(project, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	st, err := store.Open(root)
	if err != nil {
		b.Fatal(err)
	}
	steerLog := filepath.Join(st.RunDir(benchSessionID), store.FileSteer)
	input := benchJSON(b, struct {
		benchHead
		Prompt   string `json:"prompt"`
		PromptID string `json:"prompt_id"`
	}{benchHead{benchSessionID, benchTranscript, project, "default", "UserPromptSubmit"},
		"Now fix the nil check in internal/verify/stop.go.", "prompt-bench-2"})
	args := []string{"pending", install.Marker, installID}
	state := filepath.Join(root, "recap.json")
	rd := bytes.NewReader(input)
	var stdout, stderr benchSink
	b.ReportAllocs()
	b.SetBytes(turn.Bytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for _, f := range []string{state, steerLog} {
			if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
				b.Fatal(err)
			}
		}
		rd.Reset(input)
		b.StartTimer()
		if code := cmdRecap(args, rd, &stdout, &stderr); code != exitOK {
			b.Fatalf("recap exited %d", code)
		}
	}
	b.StopTimer()
	benchCheckRecap(b, &stdout, &stderr, true, turn)
	var out struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.lastWrite(), &out); err != nil || !strings.Contains(out.HookSpecificOutput.AdditionalContext, "'internal/verify/stop.go'") {
		b.Fatalf("the catch-up printed no preflight note: %s", stdout.lastWrite())
	}
}
