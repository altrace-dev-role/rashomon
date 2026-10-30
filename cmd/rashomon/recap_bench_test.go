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

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/learn"
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
// that every call gives the session's first note. /no_ledger, /lessons_3 and
// /lessons_caps are the common path, a prompt after a turn its Stop reached,
// which the catch-up leaves at recap.Pending, with the lesson read every
// prompt makes (LEARN-DELIVER1, D171 LD9): no ledger, so baseline.Key's walk
// and one Lstat; three approved lessons, the first given; and the same with
// lessons.json and labels.ndjson each at the 4 MiB the prompt path hashes
// (LD8).
func BenchmarkRecapPending(b *testing.B) {
	for _, calls := range []int{10, 1000} {
		b.Run(strconv.Itoa(calls), func(b *testing.B) { benchRecap(b, true, calls) })
	}
	b.Run("preflight", benchRecapPreflight)
	b.Run("no_ledger", func(b *testing.B) { benchRecapLessons(b, 0, false) })
	b.Run("lessons_3", func(b *testing.B) { benchRecapLessons(b, 3, false) })
	b.Run("lessons_caps", func(b *testing.B) { benchRecapLessons(b, 3, true) })
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

// benchLessons are the lesson rows benchRecapLessons approves, in import
// order, so go-test-cwd is the one given.
var benchLessons = [][2]string{
	{"go-test-cwd", "run go test from the module root"},
	{"npm-ci", "npm ci before npm test"},
	{"tidy", "run go mod tidy after a dependency change"},
}

// benchRecapLessons is BenchmarkRecapPending's lesson rows: the 10-call
// session, consented to notes, its turn claimed by its Stop, and a prompt
// naming nothing from a project on disk whose ledger approves the first n of
// benchLessons. With caps, lessons.json and labels.ndjson are padded to
// learn.StateMaxBytes each and the state signed again. The session's steer
// log is removed before each call, so every call gives the first lesson.
func benchRecapLessons(b *testing.B, n int, caps bool) {
	root, cfg := b.TempDir(), b.TempDir()
	benchSetenv(b, root, cfg)
	installID, turn := benchRecapStore(b, root, cfg, 10)
	if err := store.WriteSteerDisclosureMarker(root, steer.DisclosureVersion); err != nil {
		b.Fatal(err)
	}
	cwd := b.TempDir()
	project := strings.TrimSuffix(baseline.FileName(baseline.Key(cwd)), baseline.FileExt)
	if n > 0 {
		var ids []string
		for _, r := range benchLessons[:n] {
			ids = append(ids, r[0])
		}
		lessonsInto(b, root, project, benchLessons[:n], ids...)
	}
	if caps {
		benchPadLearnState(b, root, project)
	}
	stopArgs, stopInput := benchRecapInput(b, installID, false)
	var stopOut benchSink
	if code := cmdRecap(stopArgs, bytes.NewReader(stopInput), &stopOut, &stopOut); code != exitOK || stopOut.writes != 1 {
		b.Fatalf("the Stop that claims the turn: exit %d, %d writes", code, stopOut.writes)
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
	}{benchHead{benchSessionID, benchTranscript, cwd, "default", "UserPromptSubmit"},
		"Now add a test for a key that is too long.", "prompt-bench-2"})
	args := []string{"pending", install.Marker, installID}
	rd := bytes.NewReader(input)
	var stdout, stderr benchSink
	b.ReportAllocs()
	b.SetBytes(turn.Bytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.Remove(steerLog); err != nil && !errors.Is(err, fs.ErrNotExist) {
			b.Fatal(err)
		}
		rd.Reset(input)
		b.StartTimer()
		if code := cmdRecap(args, rd, &stdout, &stderr); code != exitOK {
			b.Fatalf("recap exited %d", code)
		}
	}
	b.StopTimer()
	benchCheckLessons(b, &stdout, &stderr, n)
}

// benchPadLearnState pads project's lessons.json (JSON whitespace) and
// labels.ndjson (one line no label parses from) to learn.StateMaxBytes each,
// the most the prompt path hashes, and signs the state again.
func benchPadLearnState(b *testing.B, root, project string) {
	b.Helper()
	dir := filepath.Join(root, learn.DirLearn, project)
	lessons, err := os.ReadFile(filepath.Join(dir, learn.FileLessons))
	if err != nil {
		b.Fatal(err)
	}
	lessons = bytes.TrimRight(lessons, "\n ")
	pad := bytes.Repeat([]byte(" "), learn.StateMaxBytes-len(lessons)-1)
	for name, body := range map[string][]byte{
		learn.FileLessons: append(append(lessons, pad...), '\n'),
		learn.FileLabels:  append(bytes.Repeat([]byte("x"), learn.StateMaxBytes-1), '\n'),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			b.Fatal(err)
		}
	}
	signLearnState(b, root, project)
}

// benchCheckLessons is the lesson rows' work-done check: with no lesson,
// nothing printed; with lessons, every call printed the first lesson's note
// as additionalContext, and no previous-turn line, since the Stop reached
// the turn.
func benchCheckLessons(b *testing.B, stdout, stderr *benchSink, n int) {
	b.Helper()
	if stderr.writes != 0 {
		b.Fatalf("stderr %q", stderr.lastWrite())
	}
	if n == 0 {
		if stdout.writes != 0 {
			b.Fatalf("no ledger: %d writes, last %q; want none", stdout.writes, stdout.lastWrite())
		}
		return
	}
	if stdout.writes != b.N {
		b.Fatalf("%d writes in %d calls", stdout.writes, b.N)
	}
	var out struct {
		SystemMessage      string `json:"systemMessage"`
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.lastWrite(), &out); err != nil ||
		!strings.Contains(out.HookSpecificOutput.AdditionalContext, "lesson go-test-cwd, approved") ||
		strings.Contains(out.SystemMessage, "previous turn") {
		b.Fatalf("the catch-up printed %s; want the first lesson and no catch-up line", stdout.lastWrite())
	}
}
