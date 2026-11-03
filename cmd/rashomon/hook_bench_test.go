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

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/hook"
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

	// benchWarmupIndex is next's argument for benchToolCalls' one warm-up
	// call. A negative index produces an id no non-negative loop index 0..
	// b.N-1 can ever collide with -- next's digit loop writes one non-digit
	// byte ('/') for a negative n and '0' for every digit after, which
	// benchCheckCalls never names as a "last call" id, so it cannot be
	// mistaken for one of the timed loop's own ids.
	benchWarmupIndex = -1
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
	return benchToolCallIn(tb, benchCWD, event, tool, input, response)
}

// benchToolCallIn is benchToolCall with the payload's cwd given.
func benchToolCallIn(tb testing.TB, cwd, event, tool string, input, response []byte) []byte {
	tb.Helper()
	head := newBenchHead(event)
	head.CWD = cwd
	return benchJSON(tb, benchToolPayload{
		benchHead: head, PromptID: payload.PromptID(0), ToolName: tool,
		ToolInput: input, ToolUseID: benchToolUseID, ToolResponse: response,
	})
}

// benchProjectCWD is a PreToolUse payload's cwd: a real directory two levels
// below a .git marker, made fresh under tb's temporary directory. The hook
// resolves a declaration's cwd to its project key on every call (baseline.Key:
// symlinks resolved, then a walk up to the nearest .git), so a cwd that does
// not exist would measure a failed resolve and a walk to the filesystem root,
// which no session in the field takes. Only the PreToolUse payloads carry it:
// the post and the probes record their cwd as written and resolve nothing.
func benchProjectCWD(tb testing.TB) string {
	tb.Helper()
	project := filepath.Join(tb.TempDir(), "project")
	cwd := filepath.Join(project, "internal", "batch")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o755); err != nil {
		tb.Fatal(err)
	}
	return cwd
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
	return benchToolCallIn(tb, benchProjectCWD(tb), "PreToolUse", "Bash", benchBashInput(tb), nil)
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
	return benchToolCallIn(tb, benchProjectCWD(tb), "PreToolUse", "Edit", benchEditInput(tb, benchFiller(benchEditBytes)), nil)
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
	// CLAUDE_CODE_ENTRYPOINT=cli: these benchmarks run cmdHook/cmdPost in
	// process on payloads with no transcript_path under cfg, so without an
	// allowlisted entrypoint H1 (internal/hostguard, wired into all five
	// entry points, Technical spec 6.2.3) reads every one of them as the
	// unknown harness and stands down before the work this benchmark
	// measures.
	b.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
}

// benchInstall opens the store, with payload's fixed identity, and writes the
// settings entries watch writes, so coverage resolves the entry as present,
// and a consent naming record_scope v5, so the hook records the content
// fields: the healthy path. watch itself cannot run here, because
// install.Executable refuses a go-build binary. The consent's
// steer_disclosure is 0, below steer.DisclosureVersion, so no note is added
// and these rows measure the hook without steering.
func benchInstall(b testing.TB, root, cfg string) string {
	b.Helper()
	if err := payload.Pin(root); err != nil {
		b.Fatal(err)
	}
	st, err := store.Open(root)
	if err != nil {
		b.Fatal(err)
	}
	if err := consent.Append(root, consent.Record{
		V: consent.RecordVersion, ScopeVersion: disclose.ScopeVersion, RecordScope: consent.RecordScopeV5,
		Steer: consent.SteerOn, Via: consent.ViaWatchYes, At: 1760000000000,
	}); err != nil {
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
	if code := cmdProbe([]string{"start", install.Marker, installID}, bytes.NewReader(start), io.Discard, &stderr); code != exitOK || stderr.writes != 0 {
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

// benchNoNote leaves the note out of a post row's store: the fallback a
// turn pays when no note stands (bash_short_nonote).
var benchNoNote bool

// benchNoteNoSpec is the UserPromptSubmit preflight's note that the turn's
// directory holds no spec (hook.NoteSpecFound), for the post rows: the
// rules step of a turn whose prompt named a path finds it and skips the
// second discovery (card SPEC-RULES1, D210). A turn whose prompt named
// nothing runs no preflight discovery, writes no note, and pays the
// discovery in full; the rows measure the first case.
func benchNoteNoSpec(b *testing.B, root string, body []byte) {
	b.Helper()
	var head struct {
		benchHead
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		b.Fatal(err)
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		b.Fatal(err)
	}
	projectDir, managedDir := specDirs()
	hook.NoteSpecFound(st, head.SessionID, head.PromptID, head.CWD, projectDir, managedDir, nil)
}

// benchToolCalls runs a tool-call entry b.N times, each time as a new call,
// against a store this helper opens and warms once first.
//
// A store's first write costs more than every one after it: group commit
// (REC-34) creates its WAL and coverage files on that call, which every
// later one on the same store reuses. Measured 2026-10-17, before this
// warm-up call existed: BenchmarkRunHook/bash_short read about 51,250 B/op
// at the gate's `-benchtime 20x` and about 48,250 at `100x`, a 1/N curve
// (114,100 B/op at `1x`) that fits a fixed one-time cost of about 66,500 B
// on the first call, the rest flat -- a smaller fraction of B/op at a
// larger N, not a different cost. A 20-iteration row would then move
// whenever the gate's own iteration count did, which is a property of the
// harness, not of what P-12's RES R2 bounds. The call below pays that cost
// on the same store the timed loop then reuses, at an index
// (benchWarmupIndex) the timed loop never reaches, so the row is
// iteration-independent (archive 85.3.13, P-12).
func benchToolCalls(b *testing.B, entry func([]string, io.Reader, io.Writer) int, body []byte, declares bool) {
	root, installID := benchStarted(b)
	in := newBenchInput(b, body)
	args := []string{install.Marker, installID}
	var stderr benchSink
	if !declares && !benchNoNote {
		benchNoteNoSpec(b, root, body)
	}
	if code := entry(args, in.next(benchWarmupIndex), &stderr); code != exitOK {
		b.Fatalf("warm-up call exited %d", code)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code := entry(args, in.next(i), &stderr); code != exitOK {
			b.Fatalf("entry exited %d", code)
		}
	}
	b.StopTimer()
	// b.N+1: the warm-up call above wrote one record to the same store
	// before the timed loop wrote its own b.N.
	if declares {
		benchCheckCalls(b, root, b.N+1, 0, &stderr)
		return
	}
	benchCheckCalls(b, root, 0, b.N+1, &stderr)
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
		b.Run(c.name, func(b *testing.B) { benchToolCalls(b, postEntry, c.payload(b), false) })
	}
	// The post of a turn with no note: the discovery in full, which the
	// gate rows above skip (card SPEC-RULES1, D210). A provisional row,
	// measured and not gated.
	b.Run("bash_short_nonote", func(b *testing.B) {
		benchNoNote = true
		defer func() { benchNoNote = false }()
		benchToolCalls(b, postEntry, benchBashShortPost(b), false)
	})
}

// postEntry is cmdPost with its stdout discarded, in the entry shape cmdHook
// has: what the post prints is the steer step's, which these measures do not
// read. Both calls run the rules step's spec discovery, from a cwd two
// levels under a .git root with no spec, and stop there: neither names a
// path under .claude/rashomon nor fails a runner, so the step reads no run
// (card 2.3 R3 and P5B), and no rule raises (SPEC-RULES1).
func postEntry(args []string, stdin io.Reader, stderr io.Writer) int {
	return cmdPost(args, stdin, io.Discard, stderr)
}

// TestBenchWarmupIndexIsDistinctFromEveryLoopIndex: benchToolCalls' one
// warm-up call writes to the same store the timed loop then measures, so
// its id must never equal one the loop itself could write -- a collision
// would either double-count that index's record or make benchCheckCalls'
// "last id" read back the wrong call. A non-negative benchWarmupIndex is
// the whole regression: at 0 it is the loop's own first id.
func TestBenchWarmupIndexIsDistinctFromEveryLoopIndex(t *testing.T) {
	if benchWarmupIndex >= 0 {
		t.Fatalf("benchWarmupIndex = %d, want negative: a non-negative index can collide with the timed loop's own i", benchWarmupIndex)
	}
	in := newBenchInput(t, append([]byte(nil), benchToolUseID...))
	in.next(benchWarmupIndex)
	warmupID := string(append([]byte(nil), in.buf[in.at:in.at+benchIDDigits]...))
	for _, n := range []int{0, 1, 2, 100, 1 << 20} {
		in.next(n)
		if got := string(in.buf[in.at : in.at+benchIDDigits]); got == warmupID {
			t.Errorf("benchWarmupIndex produces the same id as loop index %d: %q", n, got)
		}
	}
}
