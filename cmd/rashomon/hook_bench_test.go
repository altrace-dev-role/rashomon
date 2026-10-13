package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// The entry points run in process, each against a scratch store and a scratch
// Claude Code configuration (benchSetenv), on payloads generated in code: the
// 50k and 314k cases are exact on the field that carries them, new_string and
// tool_response.

const (
	benchSessionID  = payload.Session
	benchTranscript = "/Users/dev/.claude/projects/-Users-dev-src-project/" + benchSessionID + ".jsonl"
	benchCWD        = "/Users/dev/src/project"

	benchShortCommand = "git status --short && git diff --stat"
	benchEditBytes    = 50 << 10
	benchResponseSize = 314000

	// benchToolUseID is the payloads' tool_use_id. Its last benchIDDigits
	// digits are a counter, rewritten before each call (benchInput).
	benchToolUseID = "toolu_010000000000000000000000"
	benchIDDigits  = 22
)

// benchHead is the head every hook payload carries, in Claude Code's order.
type benchHead struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	HookEventName  string `json:"hook_event_name"`
}

func newBenchHead(event string) benchHead {
	return benchHead{benchSessionID, benchTranscript, benchCWD, "default", event}
}

type benchToolPayload struct {
	benchHead
	PromptID     string          `json:"prompt_id"`
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolUseID    string          `json:"tool_use_id"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
}

// benchJSON encodes v as Claude Code writes a payload: JavaScript's
// JSON.stringify leaves & < > alone, where json.Marshal would escape them.
func benchJSON(tb testing.TB, v any) []byte {
	tb.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		tb.Fatal(err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// benchFiller is n bytes of Go-looking text holding no byte JSON escapes, so
// it encodes to n bytes plus its quotes.
func benchFiller(n int) string {
	const line = "    for i := range batch { if err := handle(batch[i]); err != nil { return err } } "
	return strings.Repeat(line, n/len(line)+1)[:n]
}

// benchToolCall is a PreToolUse or PostToolUse payload for one call; a nil
// response leaves tool_response out.
func benchToolCall(tb testing.TB, event, tool string, input, response []byte) []byte {
	tb.Helper()
	return benchJSON(tb, benchToolPayload{
		benchHead: newBenchHead(event), PromptID: payload.PromptID(0), ToolName: tool,
		ToolInput: input, ToolUseID: benchToolUseID, ToolResponse: response,
	})
}

// benchBashInput is bash_short's tool_input.
func benchBashInput(tb testing.TB) []byte {
	tb.Helper()
	return benchJSON(tb, struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}{benchShortCommand, "Show working tree status"})
}

// benchEditInput is an Edit's tool_input with the given new_string.
func benchEditInput(tb testing.TB, newString string) []byte {
	tb.Helper()
	return benchJSON(tb, struct {
		FilePath   string `json:"file_path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}{benchCWD + "/internal/batch/handle.go", "// handle processes one batch of records\n", newString, false})
}

// benchBashShortPre and benchBashShortPost are the two hooks' payloads for
// bash_short; the post carries the command's short tool_response.
func benchBashShortPre(tb testing.TB) []byte {
	return benchToolCall(tb, "PreToolUse", "Bash", benchBashInput(tb), nil)
}

func benchBashShortPost(tb testing.TB) []byte {
	return benchToolCall(tb, "PostToolUse", "Bash", benchBashInput(tb), benchJSON(tb, struct {
		Stdout      string `json:"stdout"`
		Stderr      string `json:"stderr"`
		Interrupted bool   `json:"interrupted"`
		IsImage     bool   `json:"isImage"`
	}{" M internal/store/store.go\n", "", false, false}))
}

// benchEdit50kPre is an Edit whose new_string is exactly benchEditBytes.
func benchEdit50kPre(tb testing.TB) []byte {
	return benchToolCall(tb, "PreToolUse", "Edit", benchEditInput(tb, benchFiller(benchEditBytes)), nil)
}

// benchEdit314kPost is edit_50k's PostToolUse, as v1.1.0's 314 KB payload
// was: tool_input carries the 50 KiB edit, and tool_response -- the edit and
// the file's content in originalFile -- is exactly benchResponseSize bytes.
func benchEdit314kPost(tb testing.TB) []byte {
	tb.Helper()
	newString := benchFiller(benchEditBytes)
	response := func(original string) []byte {
		return benchJSON(tb, struct {
			FilePath     string `json:"filePath"`
			OldString    string `json:"oldString"`
			NewString    string `json:"newString"`
			OriginalFile string `json:"originalFile"`
			UserModified bool   `json:"userModified"`
			ReplaceAll   bool   `json:"replaceAll"`
		}{benchCWD + "/internal/batch/handle.go", "// handle processes one batch of records\n", newString, original, false, false})
	}
	resp := response(benchFiller(benchResponseSize - len(response(""))))
	if len(resp) != benchResponseSize {
		tb.Fatalf("tool_response is %d bytes, want %d", len(resp), benchResponseSize)
	}
	return benchToolCall(tb, "PostToolUse", "Edit", benchEditInput(tb, newString), resp)
}

// benchInput is a payload whose tool_use_id ends in a counter. next rewrites
// the counter in place, so every call is a distinct one and no payload is
// allocated per iteration.
type benchInput struct {
	buf []byte
	at  int
	rd  bytes.Reader
}

func newBenchInput(tb testing.TB, body []byte) *benchInput {
	tb.Helper()
	at := bytes.Index(body, []byte(benchToolUseID))
	if at < 0 || bytes.Count(body, []byte(benchToolUseID)) != 1 {
		tb.Fatal("payload does not name its tool_use_id exactly once")
	}
	return &benchInput{buf: body, at: at + len(benchToolUseID) - benchIDDigits}
}

func (p *benchInput) next(n int) io.Reader {
	for j := p.at + benchIDDigits - 1; j >= p.at; j-- {
		p.buf[j] = byte('0' + n%10)
		n /= 10
	}
	p.rd.Reset(p.buf)
	return &p.rd
}

// benchID is the tool_use_id of call n.
func benchID(n int) string {
	return fmt.Sprintf("toolu_01%0*d", benchIDDigits, n)
}

// benchSink counts writes and keeps the last one, without allocating, so it
// adds nothing to B/op.
type benchSink struct {
	writes int
	n      int
	last   [8 << 10]byte
}

func (s *benchSink) Write(p []byte) (int, error) {
	s.writes++
	s.n = copy(s.last[:], p)
	return len(p), nil
}

func (s *benchSink) lastWrite() []byte { return s.last[:s.n] }

// benchSetenv points the store and Claude Code's configuration at scratch
// directories, so nothing here reads or writes the real ones.
func benchSetenv(b testing.TB, root, cfg string) {
	b.Setenv("RASHOMON_HOME", root)
	b.Setenv("CLAUDE_CONFIG_DIR", cfg)
	b.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(cfg, "absent"))
}

// benchInstall opens the store, with payload's fixed identity, and writes the
// settings entries watch writes, so coverage resolves the entry as present:
// the healthy path. watch itself cannot run here, because install.Executable
// refuses a go-build binary.
func benchInstall(b testing.TB, root, cfg string) string {
	b.Helper()
	if err := payload.Pin(root); err != nil {
		b.Fatal(err)
	}
	st, err := store.Open(root)
	if err != nil {
		b.Fatal(err)
	}
	doc, err := settings.Parse(nil)
	if err != nil {
		b.Fatal(err)
	}
	changed, err := install.Apply(doc, install.Spec{Executable: "/usr/local/bin/rashomon", InstallID: st.InstallID()})
	if err != nil || !changed {
		b.Fatalf("installing the settings entries: changed %v, err %v", changed, err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "settings.json"), doc.Bytes(), 0o600); err != nil {
		b.Fatal(err)
	}
	return st.InstallID()
}

// benchStarted is a store with the session started as SessionStart starts it.
func benchStarted(b testing.TB) (root, installID string) {
	b.Helper()
	root, cfg := b.TempDir(), b.TempDir()
	benchSetenv(b, root, cfg)
	installID = benchInstall(b, root, cfg)
	start := benchJSON(b, struct {
		benchHead
		Source string `json:"source"`
	}{newBenchHead("SessionStart"), "startup"})
	var stderr benchSink
	if code := cmdProbe([]string{"start", install.Marker, installID}, bytes.NewReader(start), &stderr); code != exitOK || stderr.writes != 0 {
		b.Fatalf("probe start: exit %d, stderr %q", code, stderr.lastWrite())
	}
	return root, installID
}

// benchCounts is what a run holds: declarations, terminals, executions,
// verified call and post coverage records, and skipped lines.
func benchCounts(run *store.Run) [5]int {
	verified := 0
	for _, c := range run.Coverage {
		if c.State == store.StateVerified && (c.Phase == store.PhaseCall || c.Phase == store.PhasePost) {
			verified++
		}
	}
	return [5]int{len(run.Declarations), len(run.Terminals), len(run.Executions), verified, run.Skipped}
}

// benchCheckCalls is the hook and post work-done check: one declaration and
// terminal per hook call or one execution per post call, the last naming the
// last call's id, each with a verified coverage record, and nothing on stderr.
func benchCheckCalls(b *testing.B, root string, declarations, executions int, stderr *benchSink) {
	b.Helper()
	if stderr.writes != 0 {
		b.Fatalf("stderr: %q", stderr.lastWrite())
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		b.Fatal(err)
	}
	run, err := st.ReadRun(benchSessionID)
	if err != nil {
		b.Fatal(err)
	}
	want := [5]int{declarations, declarations, executions, declarations + executions, 0}
	if got := benchCounts(run); got != want {
		b.Fatalf("store holds %v declarations, terminals, executions, verified coverage and skipped lines; want %v", got, want)
	}
	var last string
	if n := len(run.Declarations); n > 0 {
		last = run.Declarations[n-1].ToolUseID
	} else if n := len(run.Executions); n > 0 {
		last = run.Executions[n-1].ToolUseID
	}
	if want := benchID(b.N - 1); last != want {
		b.Fatalf("last call recorded as %q, want %q", last, want)
	}
}

// benchToolCalls runs a tool-call entry b.N times, each time as a new call.
func benchToolCalls(b *testing.B, entry func([]string, io.Reader, io.Writer) int, body []byte, declares bool) {
	root, installID := benchStarted(b)
	in := newBenchInput(b, body)
	args := []string{install.Marker, installID}
	var stderr benchSink
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := entry(args, in.next(i), &stderr); code != exitOK {
			b.Fatalf("entry exited %d", code)
		}
	}
	b.StopTimer()
	if declares {
		benchCheckCalls(b, root, b.N, 0, &stderr)
		return
	}
	benchCheckCalls(b, root, 0, b.N, &stderr)
}

// BenchmarkRunHook is one PreToolUse, in process: payload read, shape,
// declaration, terminal and coverage, each append synced as in production.
func BenchmarkRunHook(b *testing.B) {
	for _, c := range []struct {
		name    string
		payload func(testing.TB) []byte
	}{{"bash_short", benchBashShortPre}, {"edit_50k", benchEdit50kPre}} {
		b.Run(c.name, func(b *testing.B) { benchToolCalls(b, cmdHook, c.payload(b), true) })
	}
}

// BenchmarkRunPost is one PostToolUse, in process: payload read, the
// executed shape, execution and coverage.
func BenchmarkRunPost(b *testing.B) {
	for _, c := range []struct {
		name    string
		payload func(testing.TB) []byte
	}{{"bash_short", benchBashShortPost}, {"edit_314k", benchEdit314kPost}} {
		b.Run(c.name, func(b *testing.B) { benchToolCalls(b, cmdPost, c.payload(b), false) })
	}
}
