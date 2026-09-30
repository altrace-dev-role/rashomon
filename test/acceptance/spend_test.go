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
		Store              string `json:"store"`
		Sessions           int    `json:"sessions"`
		CoveredSessions    int    `json:"covered_sessions"`
		NotCoveredSessions int    `json:"not_covered_sessions"`
		NotCoveredCost     struct {
			USD *float64 `json:"usd"`
		} `json:"not_covered_cost"`
		Turns int `json:"turns"`
		Cost  struct {
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
// end: the hooks record a turn with a failed call, the transcript's final
// reply mentions no failure, and spend prices the responses inside that
// turn's recorded span -- and names a session the hooks never saw as not
// covered, with its spend, rather than folding it in as zero.
func TestSpend_JoinsSilentlyFailedTurnsToTheHooksRecord(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	e.mustHook(p.build(t))
	time.Sleep(20 * time.Millisecond)
	e.mustPost(failurePayload(t, p.ToolUseID, "Exit code 1", false, 30))
	time.Sleep(20 * time.Millisecond)
	p2 := defaultPayload()
	p2.ToolUseID = "toolu_2"
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
	if !near(s.Cost.USD, 250_000*opus55Input) {
		t.Errorf("cost = %v, want $%v: msg_inside once, not msg_before or the final reply outside the span, "+
			"and not its second line", deref(s.Cost.USD), 250_000*opus55Input)
	}
	if s.CoveredSessions != 1 || s.NotCoveredSessions != 1 || s.Sessions != 2 {
		t.Errorf("covered %d, not covered %d of %d; want 1, 1 of 2", s.CoveredSessions, s.NotCoveredSessions, s.Sessions)
	}
	if !near(s.NotCoveredCost.USD, 500_000*opus55Input) {
		t.Errorf("not-covered cost = %v, want $%v", deref(s.NotCoveredCost.USD), 500_000*opus55Input)
	}

	txt := e.run("", nil, "spend")
	for _, want := range []string{
		"in turns that ended with a failure the summary never mentioned: at least $1.00 across 1 turn",
		"1 of 2 sessions was recorded, so this covers only those; $2.00 in the other 1 is not covered",
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
	if doc.Silent.Store != "none" || doc.Silent.NotCoveredSessions != 1 || doc.Silent.Turns != 0 {
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

// TestSpend_NoMessageTextReachesTheOutput: the canary is in every block and
// in cwd, and neither rendering carries it.
func TestSpend_NoMessageTextReachesTheOutput(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))
	now := time.Now()
	writeSessionTranscript(t, e.configDir, testSession,
		usageLine(t, testSession, "msg_1", now.Add(-time.Minute), 10, toolBlock("toolu_1")),
		usageLine(t, testSession, "msg_2", now, 10, textBlock("All done "+spendCanary)))
	for _, args := range [][]string{{"spend"}, {"spend", "--json"}} {
		res := e.run("", nil, args...)
		if res.exitCode != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, res.exitCode, res.stderr)
		}
		if strings.Contains(res.stdout+res.stderr, spendCanary) {
			t.Errorf("%v: message text reached the output:\n%s%s", args, res.stdout, res.stderr)
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
		{"spend", "--days"},
		{"spend", "--bogus"},
	} {
		res := e.run("", nil, args...)
		if res.exitCode != 1 || res.stdout != "" {
			t.Errorf("%v: exit %d, stdout %q; want 1 and nothing on stdout", args, res.exitCode, res.stdout)
		}
	}
	if res := e.run("", nil, "spend", "--days", "7"); res.exitCode != 0 || !strings.Contains(res.stdout, "last 7 days") {
		t.Errorf("--days 7: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	if res := e.run("", nil, "help"); !strings.Contains(res.stdout, "rashomon spend [--days N] [--json]") {
		t.Errorf("usage does not list spend:\n%s", res.stdout)
	}
}
