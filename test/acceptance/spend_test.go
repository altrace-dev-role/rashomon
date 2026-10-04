package acceptance

// `rashomon spend` -- the last N days of Claude Code usage priced at API list
// rates, read from Claude Code's own transcripts.
//
// These drive the binary against a fake configuration directory: the
// transcripts are fixtures written here, and the store, where one exists, is
// the one the real hooks wrote. What is asserted is the command's contract
// with a machine: where it reads, that it writes nothing and creates no
// store, that no message text reaches its output, and that its one
// rashomon-only line -- spend inside a turn whose recorded failures the
// summary never mentioned -- is joined to what the hooks recorded.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// spendCanary sits in every fixture's message content and cwd.
const spendCanary = "SPEND-ACCEPTANCE-CANARY-41c9"

// usageLine is one transcript line of one API response, in the shape Claude
// Code 2.1.285 writes: one line per content block, each with the full usage.
func usageLine(t *testing.T, session, id string, at time.Time, inputTokens int64, block map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":        "assistant",
		"timestamp":   at.UTC().Format(time.RFC3339Nano),
		"sessionId":   session,
		"isSidechain": false,
		"cwd":         "/work/" + spendCanary,
		"message": map[string]any{
			"id":          id,
			"model":       "claude-opus-5-5",
			"role":        "assistant",
			"stop_reason": "end_turn",
			"content":     []map[string]any{block},
			"usage": map[string]any{
				"input_tokens":                inputTokens,
				"output_tokens":               0,
				"cache_read_input_tokens":     0,
				"cache_creation_input_tokens": 0,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// promptLine is the user line that opens a turn, as Claude Code 2.1.285
// writes it: carrying the turn's promptId, which is the prompt_id the hooks
// record, and which ties the assistant lines after it to that turn.
func promptLine(t *testing.T, session, promptID string, at time.Time) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type":        "user",
		"timestamp":   at.UTC().Format(time.RFC3339Nano),
		"sessionId":   session,
		"isSidechain": false,
		"promptId":    promptID,
		"message":     map[string]any{"role": "user", "content": "a prompt " + spendCanary},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func textBlock(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

func toolBlock(id string) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": "Bash",
		"input": map[string]any{"command": "echo " + spendCanary}}
}

// writeSessionTranscript writes projects/<project>/<session>.jsonl under
// dir, which is a Claude Code configuration directory.
func writeSessionTranscript(t *testing.T, dir, session string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, "projects", "-work-project", session+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// opus55Input is Opus 5.5's input rate, $4/MTok, in dollars per token.
const opus55Input = 4.0 / 1e6

type spendDoc struct {
	Sessions int `json:"sessions"`
	Total    struct {
		USD *float64 `json:"usd"`
	} `json:"total"`
	Silent struct {
		Store                 string `json:"store"`
		Transcripts           int    `json:"transcripts"`
		CoveredTranscripts    int    `json:"covered_transcripts"`
		NotCoveredTranscripts int    `json:"not_covered_transcripts"`
		NotCoveredCost        struct {
			USD *float64 `json:"usd"`
		} `json:"not_covered_cost"`
		NotCoveredSessions []string `json:"not_covered_sessions"`
		Turns              int      `json:"turns"`
		Cost               struct {
			USD *float64 `json:"usd"`
		} `json:"cost"`
	} `json:"silent_failure_turns"`
}

func (e *env) spend(extraEnv []string, args ...string) (result, spendDoc) {
	e.t.Helper()
	res := e.run("", extraEnv, append([]string{"spend", "--json"}, args...)...)
	if res.exitCode != 0 {
		e.t.Fatalf("spend: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var doc spendDoc
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		e.t.Fatalf("spend --json is not JSON: %v\n%s", err, res.stdout)
	}
	return res, doc
}

func near(got *float64, want float64) bool {
	return got != nil && *got > want-1e-12 && *got < want+1e-12
}

// TestSpend_JoinsSilentlyFailedTurnsToTheHooksRecord runs the join end to
// end to end: the hooks record a turn with a failed call, the transcript's final
// reply mentions no failure, and spend prices every response the transcript
// ties to that turn's prompt -- and names a transcript the hooks never saw as
// not covered, with its spend, rather than folding it in as zero. The hooks'
// transcript_path names the transcript, which is what makes it covered.
func TestSpend_JoinsSilentlyFailedTurnsToTheHooksRecord(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := filepath.Join(e.configDir, "projects", "-work-project", testSession+".jsonl")

	p := defaultPayload()
	p.TranscriptPath = transcript
	e.mustHook(p.build(t))
	time.Sleep(20 * time.Millisecond)
	e.mustPost(failurePayload(t, p.ToolUseID, "Exit code 1", false, 30))
	time.Sleep(20 * time.Millisecond)
	p2 := defaultPayload()
	p2.ToolUseID = "toolu_2"
	p2.TranscriptPath = transcript
	e.mustHook(p2.build(t))
	time.Sleep(20 * time.Millisecond)
	post := defaultPost()
	post.ToolUseID = "toolu_2"
	e.mustPost(post.build(t))

	// The turn's span, as the hooks recorded it.
	var first, last int64
	for _, r := range e.records(testSession) {
		ms := int64(r.fields["recorded_at_unix_ms"].(float64))
		if first == 0 || ms < first {
			first = ms
		}
		if ms > last {
			last = ms
		}
	}
	if last-first < 40 {
		t.Fatalf("premise: the turn spans %dms; the fixture needs room inside it", last-first)
	}
	at := func(ms int64) time.Time { return time.UnixMilli(ms) }

	writeSessionTranscript(t, e.configDir, testSession,
		promptLine(t, testSession, p.PromptID, at(first-1000)),
		usageLine(t, testSession, "msg_before", at(first-500), 1_000, toolBlock("toolu_1")),
		usageLine(t, testSession, "msg_inside", at((first+last)/2), 250_000, toolBlock("toolu_2")),
		usageLine(t, testSession, "msg_inside", at((first+last)/2+1), 250_000, textBlock("thinking about it")),
		usageLine(t, testSession, "msg_final", at(last+500), 3_000, textBlock("Ran the command as requested.")))
	writeSessionTranscript(t, e.configDir, "sess-unrecorded",
		usageLine(t, "sess-unrecorded", "msg_other", time.Now().Add(-time.Hour), 500_000, textBlock("ok")))

	res, doc := e.spend(nil)
	s := doc.Silent
	if s.Store != "read" || s.Turns != 1 {
		t.Fatalf("silent_failure_turns = %+v, want the store read and one turn\n%s", s, res.stdout)
	}
	if !near(s.Cost.USD, 254_000*opus55Input) {
		t.Errorf("cost = %v, want $%v: every response tied to the turn's prompt -- msg_before, msg_inside once "+
			"(not its second line) and the final reply", deref(s.Cost.USD), 254_000*opus55Input)
	}
	if s.CoveredTranscripts != 1 || s.NotCoveredTranscripts != 1 || s.Transcripts != 2 {
		t.Errorf("covered %d, not covered %d of %d; want 1, 1 of 2", s.CoveredTranscripts, s.NotCoveredTranscripts, s.Transcripts)
	}
	if !reflect.DeepEqual(s.NotCoveredSessions, []string{"sess-unrecorded"}) {
		t.Errorf("not-covered sessions = %v, want the unrecorded one named", s.NotCoveredSessions)
	}
	if !near(s.NotCoveredCost.USD, 500_000*opus55Input) {
		t.Errorf("not-covered cost = %v, want $%v", deref(s.NotCoveredCost.USD), 500_000*opus55Input)
	}

	txt := e.run("", nil, "spend")
	for _, want := range []string{
		"in turns with a failed call the summary never mentioned: at least $1.02 across 1 turn",
		"1 of 2 transcripts was recorded, so this covers only those; $2.00 in the other 1 is not covered",
		"(not-covered spend is in: session sess-unrecorded)",
	} {
		if !strings.Contains(txt.stdout, want) {
			t.Errorf("text lacks %q:\n%s", want, txt.stdout)
		}
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// TestSpend_OpensNoStore is H-87 for spend: on a machine that has recorded
// nothing, asking what was spent must not mint an install identity, and the
// silent-failure line says it is unknown rather than zero.
func TestSpend_OpensNoStore(t *testing.T) {
	e := newEnv(t)
	writeSessionTranscript(t, e.configDir, "sess-a",
		usageLine(t, "sess-a", "msg_1", time.Now().Add(-time.Hour), 1_000_000, textBlock("hello")))

	_, doc := e.spend(nil)
	txt := e.run("", nil, "spend")

	entries, err := os.ReadDir(e.home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the store root is not empty after `rashomon spend` on a machine with no store: %v", entries)
	}
	if doc.Silent.Store != "none" || doc.Silent.NotCoveredTranscripts != 1 || doc.Silent.Turns != 0 {
		t.Errorf("silent_failure_turns = %+v, want store none and the one session not covered", doc.Silent)
	}
	if !strings.Contains(txt.stdout, "never mentioned: unknown") ||
		!strings.Contains(txt.stdout, "rashomon has recorded nothing on this machine") {
		t.Errorf("the text does not say the line is unknown for want of a record:\n%s", txt.stdout)
	}
	if !near(doc.Total.USD, 4.0) {
		t.Errorf("total = %v, want $4.00 for 1 MTok of Opus 5.5 input", deref(doc.Total.USD))
	}
}

// TestSpend_WritesNothing: the store and the transcripts are byte-for-byte
// and mtime-for-mtime what they were.
func TestSpend_WritesNothing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))
	tp := writeSessionTranscript(t, e.configDir, testSession,
		usageLine(t, testSession, "msg_1", time.Now().Add(-time.Minute), 10, textBlock("Ran it.")))
	before := walkStore(t, e.home)
	tBefore, err := os.Stat(tp)
	if err != nil {
		t.Fatal(err)
	}

	e.spend(nil)
	e.run("", nil, "spend")

	if after := walkStore(t, e.home); !reflect.DeepEqual(before, after) {
		t.Errorf("the store changed after `rashomon spend`")
	}
	tAfter, err := os.Stat(tp)
	if err != nil {
		t.Fatal(err)
	}
	if !tAfter.ModTime().Equal(tBefore.ModTime()) || tAfter.Size() != tBefore.Size() {
		t.Errorf("the transcript changed after `rashomon spend`")
	}
}

// TestSpend_ReadsWhereClaudeCodeKeepsItsTranscripts: CLAUDE_CONFIG_DIR
// relocates the whole directory, projects/ included, and only when it is
// unset is ~/.claude the place to look -- the rule settings.ConfigDir states
// for settings.json and plugins/.
func TestSpend_ReadsWhereClaudeCodeKeepsItsTranscripts(t *testing.T) {
	e := newEnv(t)
	home := t.TempDir()
	writeSessionTranscript(t, e.configDir, "sess-cfg",
		usageLine(t, "sess-cfg", "msg_cfg", time.Now().Add(-time.Hour), 1_000_000, textBlock("a")))
	writeSessionTranscript(t, filepath.Join(home, ".claude"), "sess-home",
		usageLine(t, "sess-home", "msg_home", time.Now().Add(-time.Hour), 2_000_000, textBlock("b")))

	_, doc := e.spend([]string{"HOME=" + home})
	if !near(doc.Total.USD, 4.0) || doc.Sessions != 1 {
		t.Errorf("with CLAUDE_CONFIG_DIR set: total %v over %d sessions, want $4.00 over 1 (that directory only)",
			deref(doc.Total.USD), doc.Sessions)
	}

	cmd := e.command("", []string{"HOME=" + home}, "spend", "--json")
	var env []string
	for _, kv := range cmd.Env {
		if !strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
			env = append(env, kv)
		}
	}
	cmd.Env = env
	res := e.wait(cmd)
	var unset spendDoc
	if err := json.Unmarshal([]byte(res.stdout), &unset); err != nil {
		t.Fatalf("spend --json: %v\n%s%s", err, res.stdout, res.stderr)
	}
	if !near(unset.Total.USD, 8.0) || unset.Sessions != 1 {
		t.Errorf("with CLAUDE_CONFIG_DIR unset: total %v over %d sessions, want $8.00 over 1 (~/.claude only)",
			deref(unset.Total.USD), unset.Sessions)
	}
}

// TestSpend_OldTranscriptsAreSkippedAndSaid: cmdSpend hands Discover the
// window, so a transcript last written before it is not read at all -- and is
// counted and said, not reported as "no transcripts found". Without the
// window, a regression reading every transcript ever written went unnoticed.
func TestSpend_OldTranscriptsAreSkippedAndSaid(t *testing.T) {
	e := newEnv(t)
	old := time.Now().Add(-40 * 24 * time.Hour)
	p := writeSessionTranscript(t, e.configDir, "sess-old",
		usageLine(t, "sess-old", "msg_old", old, 1_000, textBlock("a")))
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	res := e.run("", nil, "spend", "--json")
	var doc struct {
		Read struct {
			Files             int `json:"files"`
			FilesBeforeWindow int `json:"files_before_window"`
		} `json:"read"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		t.Fatalf("spend --json: %v\n%s%s", err, res.stdout, res.stderr)
	}
	if doc.Read.Files != 0 || doc.Read.FilesBeforeWindow != 1 {
		t.Errorf("read.files = %d, read.files_before_window = %d; want 0 and 1: the old transcript was read", doc.Read.Files, doc.Read.FilesBeforeWindow)
	}
	// And the window is the one asked for: over 60 days the same transcript
	// is read.
	res = e.run("", nil, "spend", "--days", "60", "--json")
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		t.Fatalf("spend --days 60 --json: %v\n%s%s", err, res.stdout, res.stderr)
	}
	if doc.Read.Files != 1 {
		t.Errorf("--days 60: read.files = %d, want 1: the window passed to Discover is not the one asked for", doc.Read.Files)
	}
	txt := e.run("", nil, "spend")
	if !strings.Contains(txt.stdout, "(1 older transcript last written before that was not read)") {
		t.Errorf("the text does not say the older transcript was skipped:\n%s", txt.stdout)
	}
}

// classifierLine is a refused response a fallback served, carrying the
// canary in the fields spend decodes and prints only as closed words -- the
// refusal's category and an iteration entry's type and model -- and in the
// refusal's explanation, which it does not read.
func classifierLine(t *testing.T, session, id string, at time.Time) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "assistant", "timestamp": at.UTC().Format(time.RFC3339Nano),
		"sessionId": session, "isSidechain": false,
		"message": map[string]any{
			"id": id, "model": "claude-opus-5-5", "role": "assistant", "content": []any{},
			"stop_reason":  "refusal",
			"stop_details": map[string]any{"type": "refusal", "category": spendCanary, "explanation": spendCanary},
			"usage": map[string]any{
				"input_tokens": 10, "output_tokens": 0,
				"iterations": []map[string]any{
					{"type": "message", "model": "arn:aws:bedrock:" + spendCanary, "input_tokens": 10, "output_tokens": 3},
					{"type": spendCanary, "model": "claude-" + spendCanary, "input_tokens": 10, "output_tokens": 3},
					{"type": "fallback_message", "model": "claude-opus-5-5", "input_tokens": 10, "output_tokens": 0},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSpend_NoMessageTextReachesTheOutput: the canary is in every block and
// in cwd -- and in the final words of a turn that fires, so the one read of
// message content (the verdict's) is reached end to end: the hooks record a
// failed call whose transcript_path names a discovered transcript, and the
// transcript's user lines carry the recorded prompt_id -- and in a refusal's
// category and a fallback's iteration entries (classifierLine). Neither rendering,
// on stdout or stderr, carries a byte of it. The earlier fixture recorded a
// path spend never discovers, so the content read never ran and a mutant
// that printed the final message to stderr passed. A second recorded turn
// with a failed call ends in words that name the failure, canary included:
// it does not fire, and its words are read too.
func TestSpend_NoMessageTextReachesTheOutput(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := filepath.Join(e.configDir, "projects", "-work-project", testSession+".jsonl")
	honest := defaultPayload()
	honest.PromptID, honest.ToolUseID, honest.TranscriptPath = "prompt-honest", "toolu_h", transcript
	e.mustHook(honest.build(t))
	e.mustPost(failurePayload(t, honest.ToolUseID, "Exit code 1", false, 30))
	p := defaultPayload()
	p.TranscriptPath = transcript
	e.mustHook(p.build(t))
	e.mustPost(failurePayload(t, p.ToolUseID, "Exit code 1", false, 30))
	now := time.Now()
	thinking := map[string]any{"type": "thinking", "thinking": "thinking " + spendCanary}
	writeSessionTranscript(t, e.configDir, testSession,
		promptLine(t, testSession, honest.PromptID, now.Add(-3*time.Minute)),
		usageLine(t, testSession, "msg_h", now.Add(-150*time.Second), 10, textBlock("The command failed with exit 1. "+spendCanary)),
		promptLine(t, testSession, p.PromptID, now.Add(-2*time.Minute)),
		usageLine(t, testSession, "msg_1", now.Add(-time.Minute), 10, thinking),
		usageLine(t, testSession, "msg_1", now.Add(-time.Minute), 10, toolBlock("toolu_1")),
		usageLine(t, testSession, "msg_2", now.Add(-30*time.Second), 10,
			map[string]any{"type": "tool_use", "id": "toolu_w", "name": "Write", "input": map[string]any{"text": spendCanary}}),
		classifierLine(t, testSession, "msg_r", now.Add(-20*time.Second)),
		usageLine(t, testSession, "msg_3", now, 10, textBlock("All done "+spendCanary)))

	_, doc := e.spend(nil)
	if doc.Silent.Turns != 1 {
		t.Fatalf("premise: silent_failure_turns.turns = %d, want 1 -- the turn's final words were not read, so this proves nothing",
			doc.Silent.Turns)
	}
	for _, args := range [][]string{{"spend"}, {"spend", "--json"}} {
		res := e.run("", nil, args...)
		if res.exitCode != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, res.exitCode, res.stderr)
		}
		if strings.Contains(res.stdout, spendCanary) || strings.Contains(res.stderr, spendCanary) {
			t.Errorf("%v: message text reached the output:\nstdout:\n%s\nstderr:\n%s", args, res.stdout, res.stderr)
		}
	}
}

// TestSpend_RefusesAWindowThatIsNotADayCount: zero, negative or non-numeric
// --days names no window, and answering it would print a confident $0.00.
func TestSpend_RefusesAWindowThatIsNotADayCount(t *testing.T) {
	e := newEnv(t)
	for _, args := range [][]string{
		{"spend", "--days", "0"},
		{"spend", "--days", "-3"},
		{"spend", "--days", "week"},
		// Past the century MaxDays allows: the overflow that once printed a
		// confident $0.00 is refused before any window is computed.
		{"spend", "--days", "106752"},
		{"spend", "--days", "36501"},
		{"spend", "--days"},
		{"spend", "--bogus"},
	} {
		res := e.run("", nil, args...)
		if res.exitCode != 1 || res.stdout != "" {
			t.Errorf("%v: exit %d, stdout %q; want 1 and nothing on stdout", args, res.exitCode, res.stdout)
		}
	}
	for _, d := range []string{"7", "36500"} {
		if res := e.run("", nil, "spend", "--days", d); res.exitCode != 0 || !strings.Contains(res.stdout, "last "+d+" days") {
			t.Errorf("--days %s: exit %d, stdout %q", d, res.exitCode, res.stdout)
		}
	}
	if res := e.run("", nil, "help"); !strings.Contains(res.stdout, "rashomon spend [--days N] [--json]") {
		t.Errorf("usage does not list spend:\n%s", res.stdout)
	}
}

// TestSpend_AWatchedSessionWithNoCallIsRecorded drives the hooks as a session
// with no tool call fires them -- measured on Claude Code 2.1.280 for `claude
// -p "Reply with exactly: ok"`, a resume of it, and an interactive session
// answered without a tool: SessionStart, then SessionEnd, nothing between,
// and the transcript's first lines stamped a few milliseconds before the
// start record. No record names the transcript, and spend read it "not
// recorded by rashomon" and the silent-failure line "unknown". Such a session
// had no call to miss, so its transcript is covered. A session still open (a
// start and no end yet), a paused one, and one a resume without the hooks
// made a call in between two watched runs are not -- measured live, that
// resume read recorded under a rule that took one stretch from the first
// start to the last end.
func TestSpend_AWatchedSessionWithNoCallIsRecorded(t *testing.T) {
	const id = "sess-watched"
	// run is one Claude Code process on the session: SessionStart, a prompt
	// and its answer, and SessionEnd when end is set.
	run := func(t *testing.T, e *env, prompt, msg string, end bool) []string {
		t.Helper()
		if res := e.probe("start", id); res.exitCode != 0 {
			t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		starts := e.coverage(id, "start")
		startMS := int64(starts[len(starts)-1].fields["recorded_at_unix_ms"].(float64))
		time.Sleep(20 * time.Millisecond)
		answered := time.Now()
		time.Sleep(20 * time.Millisecond)
		if end {
			if res := e.probe("end", id); res.exitCode != 0 {
				t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
			}
		}
		return []string{
			promptLine(t, id, prompt, time.UnixMilli(startMS-3)),
			usageLine(t, id, msg, answered, 100_000, textBlock("ok")),
		}
	}
	for _, tc := range []struct {
		name    string
		session func(t *testing.T, e *env) []string
		covered bool
	}{
		{"start and end", func(t *testing.T, e *env) []string {
			return run(t, e, "p1", "msg_1", true)
		}, true},
		{"a session still open", func(t *testing.T, e *env) []string {
			return run(t, e, "p1", "msg_1", false)
		}, false},
		{"resumed, every answer watched", func(t *testing.T, e *env) []string {
			return append(run(t, e, "p1", "msg_1", true), run(t, e, "p2", "msg_2", true)...)
		}, true},
		{"a resume without the hooks made a call in between", func(t *testing.T, e *env) []string {
			lines := run(t, e, "p1", "msg_1", true)
			time.Sleep(20 * time.Millisecond)
			unwatched := time.Now()
			lines = append(lines,
				promptLine(t, id, "p2", unwatched),
				strings.Replace(usageLine(t, id, "msg_2", unwatched.Add(time.Millisecond), 100_000, toolBlock("toolu_gap")),
					`"stop_reason":"end_turn"`, `"stop_reason":"tool_use"`, 1),
				usageLine(t, id, "msg_3", unwatched.Add(2*time.Millisecond), 100_000, textBlock("gap")))
			time.Sleep(20 * time.Millisecond)
			return append(lines, run(t, e, "p3", "msg_4", true)...)
		}, false},
		{"a call made while paused", func(t *testing.T, e *env) []string {
			if res := e.probe("start", id); res.exitCode != 0 {
				t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			time.Sleep(20 * time.Millisecond)
			answered := time.Now()
			if res := e.pause(); res.exitCode != 0 {
				t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			p := defaultPayload()
			p.SessionID = id
			p.TranscriptPath = filepath.Join(e.configDir, "projects", "-work-project", id+".jsonl")
			e.mustHook(p.build(t))
			if res := e.resume(); res.exitCode != 0 {
				t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			time.Sleep(20 * time.Millisecond)
			if res := e.probe("end", id); res.exitCode != 0 {
				t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			return []string{usageLine(t, id, "msg_1", answered, 100_000, textBlock("ok"))}
		}, false},
		// forget removes a call's declaration, execution and terminal and
		// keeps its coverage records: the run still holds a call.
		{"a call that was forgotten", func(t *testing.T, e *env) []string {
			if res := e.probe("start", id); res.exitCode != 0 {
				t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			time.Sleep(20 * time.Millisecond)
			answered := time.Now()
			p := defaultPayload()
			p.SessionID = id
			p.TranscriptPath = filepath.Join(e.configDir, "projects", "-work-project", id+".jsonl")
			e.mustHook(p.build(t))
			post := defaultPost()
			post.SessionID, post.TranscriptPath = id, p.TranscriptPath
			e.mustPost(post.build(t))
			if res := e.forget("1h"); res.exitCode != 0 || len(e.declarations(id)) != 0 {
				t.Fatalf("forget: exit %d, stderr %q, %d declarations left", res.exitCode, res.stderr, len(e.declarations(id)))
			}
			time.Sleep(20 * time.Millisecond)
			if res := e.probe("end", id); res.exitCode != 0 {
				t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			return []string{usageLine(t, id, "msg_1", answered, 100_000, textBlock("ok"))}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if res := e.watch(); res.exitCode != 0 {
				t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			writeSessionTranscript(t, e.configDir, id, tc.session(t, e)...)
			res, doc := e.spend(nil)
			want := 0
			if tc.covered {
				want = 1
			}
			if s := doc.Silent; s.Store != "read" || s.Transcripts != 1 || s.CoveredTranscripts != want {
				t.Errorf("silent_failure_turns = %+v, want %d of 1 transcript covered\n%s", s, want, res.stdout)
			}
			txt := e.run("", nil, "spend").stdout
			if tc.covered && (!strings.Contains(txt, "never mentioned: none found (no recorded turn with a failed call ended in a summary that left it out)\n") ||
				!strings.Contains(txt, "(from rashomon's record; 1 of 1 transcript was recorded, so this covers only those)\n") ||
				strings.Contains(txt, "not recorded by rashomon")) {
				t.Errorf("a watched session with no call is not shown recorded:\n%s", txt)
			}
			if !tc.covered && (!strings.Contains(txt, "never mentioned: unknown\n  (rashomon recorded none of the 1 transcript, so none is covered)\n") ||
				!strings.Contains(txt, id+" $") || !strings.Contains(txt, ", not recorded by rashomon\n")) {
				t.Errorf("the session is not shown not recorded:\n%s", txt)
			}
		})
	}
}
