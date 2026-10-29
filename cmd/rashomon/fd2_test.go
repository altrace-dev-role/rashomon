package main

import (
	"bytes"
	"io"
	"runtime"
	"runtime/metrics"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// fd2Created is the runtime's count of goroutines created since the process
// started (runtime/metrics, go1.26 on; CI and the lane runs are go1.27.3).
const fd2Created = "/sched/goroutines-created:goroutines"

// fd2Spawned is how many goroutines fn started. By the creation count where
// the toolchain has it, so one that starts and ends inside fn still counts;
// else by the live count before and after, which sees only one still running,
// and the log says so.
func fd2Spawned(t *testing.T, fn func()) int64 {
	t.Helper()
	s := []metrics.Sample{{Name: fd2Created}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		t.Logf("%s is not in this toolchain; counting live goroutines instead", fd2Created)
		before := runtime.NumGoroutine()
		fn()
		return int64(runtime.NumGoroutine() - before)
	}
	before := s[0].Value.Uint64()
	fn()
	metrics.Read(s)
	return int64(s[0].Value.Uint64() - before)
}

// fd2Failure is a PostToolUseFailure payload: the tool payload and the
// failure's own fields.
type fd2Failure struct {
	benchToolPayload
	Error       string `json:"error"`
	IsInterrupt bool   `json:"is_interrupt"`
}

// TestFD2a_NoSecondGoroutineOnTheCallPath (PRD PF-FD2; card P5B, 5.4, D-B2):
// a hook entry runs on the goroutine Claude Code's call started, start to
// end: PreToolUse, PostToolUse at success, PostToolUseFailure with a runner
// (the steer step), the store write that 5.7's trigger steps on, and
// SessionStart's probe start no goroutine. Each row runs once to warm up
// first (os/signal's loop starts on the process's first Notify, the GC's
// workers on its first cycle), then three times measured; the fewest started
// must be none, so a stray timer elsewhere in the process cannot fail a row.
func TestFD2a_NoSecondGoroutineOnTheCallPath(t *testing.T) {
	root, installID := benchStarted(t)
	// The bench consent names record_scope v5, but in process nothing syncs
	// the marker the hook reads (wireConsentViews is main's), so the calls
	// would record at v4, which withholds file_path: the store write would
	// leave no store_touched entry to raise. The binary's probe writes it.
	if err := store.WriteRecordScopeMarker(root, store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	args := []string{install.Marker, installID}
	var stderr benchSink
	var out bytes.Buffer
	call := func(name string, entry func([]string, io.Reader, io.Writer) int, in io.Reader) {
		if code := entry(args, in, &stderr); code != exitOK || stderr.writes != 0 {
			t.Fatalf("%s: exit %d, stderr %q", name, code, stderr.lastWrite())
		}
	}
	post := func(a []string, in io.Reader, e io.Writer) int { return cmdPost(a, in, &out, e) }

	storePath := benchCWD + "/.claude/rashomon/task/current.json"
	storeInput := benchJSON(t, map[string]string{"file_path": storePath, "content": "{}"})
	storePre := newBenchInput(t, benchToolCall(t, "PreToolUse", "Write", storeInput, nil))
	storePost := newBenchInput(t, benchToolCall(t, "PostToolUse", "Write", storeInput,
		benchJSON(t, map[string]string{"type": "create", "filePath": storePath})))
	goTest := benchJSON(t, map[string]string{"command": "go test ./..."})
	failurePre := newBenchInput(t, benchToolCall(t, "PreToolUse", "Bash", goTest, nil))
	head := newBenchHead(hook.FailureEvent)
	failure := newBenchInput(t, benchJSON(t, fd2Failure{benchToolPayload: benchToolPayload{
		benchHead: head, PromptID: "p", ToolName: "Bash", ToolInput: goTest,
		ToolUseID: benchToolUseID}, Error: "Exit code 1"}))
	hookIn := newBenchInput(t, benchBashShortPre(t))
	postIn := newBenchInput(t, benchBashShortPost(t))
	start := benchJSON(t, struct {
		benchHead
		Source string `json:"source"`
	}{newBenchHead("SessionStart"), "startup"})

	// Each row's ids are its own (row*10 + k), so no row's post closes
	// another row's declaration. A post row's pre lands first, in setup,
	// outside the count: a post with no declaration is an orphan, which reads
	// the recording incomplete, and verify then stops before the steer step's
	// judgment for every later call of the session.
	rows := []struct {
		name  string
		setup func(k int)
		run   func(k int)
	}{
		{"PreToolUse bash_short", nil, func(k int) { call("pre", cmdHook, hookIn.next(k)) }},
		{"PostToolUse bash_short", func(k int) { call("pre", cmdHook, hookIn.next(10+k)) },
			func(k int) { call("post", postEntry, postIn.next(10+k)) }},
		{"PostToolUseFailure go test", func(k int) { call("failure pre", cmdHook, failurePre.next(20+k)) },
			func(k int) { call("failure", postEntry, failure.next(20+k)) }},
		{"store write, pre and post", nil, func(k int) {
			call("store pre", cmdHook, storePre.next(30+k))
			out.Reset()
			call("store post", post, storePost.next(30+k))
		}},
		{"probe start", nil, func(int) {
			call("probe start", func(_ []string, in io.Reader, e io.Writer) int {
				return cmdProbe([]string{"start", install.Marker, installID}, in, e)
			}, bytes.NewReader(start))
		}},
	}
	prepare := func(i, k int) {
		if rows[i].setup != nil {
			rows[i].setup(k)
		}
	}
	for i, r := range rows {
		prepare(i, 0)
		r.run(0)
	}
	if !strings.Contains(out.String(), `"systemMessage"`) {
		t.Fatalf("premise: the store write's post printed %q, want its integrity flag (the step ran and fired)", out.String())
	}
	runtime.GC()
	for i, r := range rows {
		least := int64(-1)
		for k := 1; k <= 3; k++ {
			prepare(i, k)
			if n := fd2Spawned(t, func() { r.run(k) }); least < 0 || n < least {
				least = n
			}
		}
		if least > 0 {
			t.Errorf("%s: started %d goroutines at the fewest over three calls, want none (PF-FD2: no second goroutine)", r.name, least)
		}
	}
	// The measured store posts took the whole steer path too, not an early
	// exit: the last one printed its flag.
	if !strings.Contains(out.String(), `"systemMessage"`) {
		t.Errorf("the last measured store post printed %q, want its integrity flag", out.String())
	}
}

// TestFD2c_InputIsCappedBeforeParse (PRD PF-FD2; card P5B, 5.4, D-B3): a
// stdin that never ends costs PreToolUse and PostToolUse at most
// MaxPayloadBytes+1 bytes read, and the entry exits 0. The cap is
// hookio.Read's, the one read every entry makes (claudeHostGuard), before
// any parse.
func TestFD2c_InputIsCappedBeforeParse(t *testing.T) {
	_, installID := benchStarted(t)
	args := []string{install.Marker, installID}
	for _, c := range []struct {
		name  string
		entry func([]string, io.Reader, io.Writer) int
	}{{"PreToolUse", cmdHook}, {"PostToolUse", postEntry}} {
		r := &repeatReader{prefix: `{"session_id":"`, c: 'a', n: -1}
		var stderr bytes.Buffer
		if code := c.entry(args, r, &stderr); code != exitOK {
			t.Errorf("%s: exit %d, want 0; stderr %q", c.name, code, stderr.String())
		}
		if r.read > hookio.MaxPayloadBytes+1 {
			t.Errorf("%s: read %d bytes of an endless stdin, want at most %d", c.name, r.read, hookio.MaxPayloadBytes+1)
		}
	}
}
