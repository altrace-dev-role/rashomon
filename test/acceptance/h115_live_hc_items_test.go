package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// H-115 -- the live hook-contract checks keep and print shapes, never values,
// and leave nothing behind.
//
// test/live/hc-items.sh runs real Claude Code sessions, so what it observes is
// read by a person and asserted by nothing, as with test/live/l-items.sh. What
// can be held down without a real session is what it does with what it
// captures. The payloads its hooks receive carry prompts, commands, tool
// output and the paths of the machine they ran on, and the streams and logs
// Claude Code writes beside them carry the account's plugins and settings. A
// check that printed or kept one would publish it the first time its output
// was pasted into a pull request, and a clean secret scan would not notice.
//
// Every payload, stream and log the script produces goes through
// test/live/hc_shape.py. This item feeds that file inputs whose every value
// carries a canary and fails on a canary in anything it keeps or prints; the
// positive twin in each case is the shape it was supposed to print, so a
// reader that printed nothing at all does not pass. The script itself runs
// against a stand-in claude: it must refuse a machine with a user settings
// file before touching anything, record the version before any check, and
// when it is done leave TMPDIR and HOME as it found them -- its scratch gone,
// and with it what each session wrote outside the scratch (a transcript, a
// session-env directory, files under $TMPDIR/claude-<uid> that name the
// session's model and tools).
//
// Break: the recorder keeping a raw prompt_id, a failure's error text or a
// key name it does not recognise; the hook summary printing a hook's stdout
// instead of its size; the script dropping its refusal, its version line, the
// removal of its scratch, of a session's transcript or session-env directory,
// or the TMPDIR it points into the scratch.

// leak is the canary. Upper case on purpose: a key name the recorder keeps
// must look like Claude Code's own, lower case and underscored, so a key made
// of this is one it has to mask.
const leak = "LEAKCANARY"

func hcShapePath() string { return filepath.Join(moduleRoot, "test", "live", "hc_shape.py") }

func needTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available", tool)
		}
	}
}

// hcShape runs one hc_shape.py subcommand the way the script does.
func hcShape(t *testing.T, stdin string, extraEnv []string, args ...string) result {
	t.Helper()
	needTools(t, "python3")
	e := &env{t: t}
	cmd := exec.Command("python3", append([]string{"-I", hcShapePath()}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), extraEnv...)
	return e.wait(cmd)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertNoLeak(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, leak) {
		t.Errorf("%s carries a value it was handed:\n%s", what, s)
	}
}

func assertHas(t *testing.T, what, s string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("%s does not say %q:\n%s", what, w, s)
		}
	}
}

// hcTurn is one hook payload of a session the summaries are asked about. Every
// value it can carry is a canary or one of Claude Code's closed vocabularies.
type hcTurn struct {
	run     string
	payload map[string]any
}

func hcPayload(event string, fields map[string]any) map[string]any {
	p := map[string]any{
		"hook_event_name": event,
		"session_id":      "sess-" + leak,
		"transcript_path": "/home/" + leak + "/t.jsonl",
		"cwd":             "/home/" + leak,
		"permission_mode": "default",
		"Key" + leak:      "x",
	}
	for k, v := range fields {
		p[k] = v
	}
	return p
}

// hcSession is two turns and a manual compaction, as the script's HC-2 run
// produces them, plus the three failures HC-3 asks for.
func hcSession() []hcTurn {
	pid1, pid2, pid3 := "pid-1-"+leak, "pid-2-"+leak, "pid-3-"+leak
	bash := func(id, cmd string) map[string]any {
		return map[string]any{"tool_name": "Bash", "tool_use_id": id, "tool_input": map[string]any{"command": cmd, "description": leak}}
	}
	with := func(base map[string]any, more map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range more {
			out[k] = v
		}
		return out
	}
	call1, call2 := bash("toolu-1-"+leak, "echo "+leak), bash("toolu-2-"+leak, "echo two "+leak)
	call3 := bash("toolu-3-"+leak, "echo three "+leak)
	return []hcTurn{
		{"1", hcPayload("SessionStart", map[string]any{"source": "startup"})},
		{"1", hcPayload("UserPromptSubmit", map[string]any{"prompt": leak, "prompt_id": pid1})},
		{"1", hcPayload("PreToolUse", with(call1, map[string]any{"prompt_id": pid1}))},
		{"1", hcPayload("PostToolUse", with(call1, map[string]any{"prompt_id": pid1, "tool_response": map[string]any{"stdout": leak}}))},
		{"1", hcPayload("PreToolUse", with(call2, map[string]any{"prompt_id": pid1}))},
		{"1", hcPayload("PostToolUse", with(call2, map[string]any{"prompt_id": pid1, "tool_response": map[string]any{"stdout": leak}}))},
		{"1", hcPayload("Stop", map[string]any{"prompt_id": pid1, "last_assistant_message": leak, "stop_hook_active": false})},
		{"1", hcPayload("SessionEnd", map[string]any{"reason": leak})},
		{"2", hcPayload("SessionStart", map[string]any{"source": "resume"})},
		{"2", hcPayload("UserPromptSubmit", map[string]any{"prompt": leak, "prompt_id": pid2})},
		{"2", hcPayload("PreToolUse", with(call3, map[string]any{"prompt_id": pid2}))},
		{"2", hcPayload("PostToolUse", with(call3, map[string]any{"prompt_id": pid2, "tool_response": map[string]any{"stdout": leak}}))},
		{"2", hcPayload("Stop", map[string]any{"prompt_id": pid2, "last_assistant_message": leak, "stop_hook_active": false})},
		{"3", hcPayload("PreCompact", map[string]any{"trigger": "manual", "custom_instructions": leak, "prompt_id": pid3})},
		{"3", hcPayload("SessionStart", map[string]any{"source": "compact", "prompt_id": pid3})},
		{"3", hcPayload("PostCompact", map[string]any{"trigger": "manual", "compact_summary": leak, "prompt_id": pid3})},
		{"3", hcPayload("SessionEnd", map[string]any{"reason": leak, "source": leak})},
		{"4", hcPayload("PostToolUseFailure", with(bash("toolu-4-"+leak, "git commit --allow-empty -m '"+leak+"' 2>&1"),
			map[string]any{"error": "Exit code 1\n" + leak + "-rejected", "is_interrupt": false}))},
		{"4", hcPayload("PostToolUseFailure", with(bash("toolu-5-"+leak, "false"), map[string]any{"error": "Exit code 1", "is_interrupt": false}))},
		{"4", hcPayload("PostToolUseFailure", with(bash("toolu-6-"+leak, "exit 3"), map[string]any{"error": "Exit code 3", "is_interrupt": false}))},
	}
}

// recordSession hands every payload to the recorder, as each installed hook
// does, and returns the log it wrote.
func recordSession(t *testing.T, dir string) string {
	t.Helper()
	log := filepath.Join(dir, "events.ndjson")
	for _, turn := range hcSession() {
		res := hcShape(t, mustJSON(t, turn.payload), []string{"HC_RUN=" + turn.run, "HC_MARKER=" + leak + "-rejected"}, "record", log, "hc2")
		if res.exitCode != 0 || res.stdout != "" {
			t.Fatalf("record: exit %d, stdout %q; a hook must exit 0 and print nothing", res.exitCode, res.stdout)
		}
	}
	return log
}

func TestH115_TheRecorderKeepsNoValue(t *testing.T) {
	dir := t.TempDir()
	log := recordSession(t, dir)
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	assertNoLeak(t, "the recorder's log", string(b))

	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	session := hcSession()
	if len(lines) != len(session) {
		t.Fatalf("the recorder wrote %d lines for %d payloads", len(lines), len(session))
	}
	for i, ln := range lines {
		var rec struct {
			Event string   `json:"event"`
			Keys  []string `json:"keys"`
		}
		if err := json.Unmarshal([]byte(ln), &rec); err != nil {
			t.Fatalf("line %d is not JSON: %v", i+1, err)
		}
		if want := session[i].payload["hook_event_name"]; rec.Event != want {
			t.Errorf("line %d names event %q, want %q", i+1, rec.Event, want)
		}
		if !strings.Contains(strings.Join(rec.Keys, ","), "<other>") {
			t.Errorf("line %d keys %v: the key made of the canary is not masked as <other>", i+1, rec.Keys)
		}
	}

	// A payload it cannot read is a line saying so, and still exit 0 and
	// nothing on stdout: these run as hooks.
	res := hcShape(t, "not json "+leak, nil, "record", log, "hc2")
	if res.exitCode != 0 || res.stdout != "" {
		t.Errorf("record on garbage: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	b, _ = os.ReadFile(log)
	assertNoLeak(t, "the recorder's log after garbage", string(b))
	assertHas(t, "the recorder's log after garbage", string(b), "record-failed")
}

func TestH115_TheSessionSummariesPrintNoValue(t *testing.T) {
	dir := t.TempDir()
	log := recordSession(t, dir)

	for _, c := range []struct {
		sub   string
		wants []string
	}{
		{"hc2", []string{
			"run 1: UserPromptSubmit 1, PreToolUse 2, PostToolUse 2, Stop 1; distinct tool_use_id over PreToolUse 2",
			"PreToolUse 2/2, PostToolUse 2/2, Stop 1/1",
			"differs between run 1 and run 2: yes",
			"keys UserPromptSubmit: ", "prompt_id",
		}},
		{"fm12", []string{"Stop payloads 2, carrying prompt_id 2, equal to their turn's UserPromptSubmit prompt_id 2"}},
		{"fm13", []string{
			"run 1 startup, run 2 resume, run 3 compact",
			"PreCompact(trigger manual), SessionStart(source compact), PostCompact(trigger manual)",
			"compact-source SessionStart carries prompt_id yes; equal to PreCompact's yes",
		}},
		{"hc3", []string{
			"hook-rejected git commit: PreToolUse 0, PostToolUseFailure 1, PostToolUse 0",
			"N=1, further lines 1, carries the commit-msg hook's line yes",
			"N=3, further lines 0",
			"tool_response key no",
		}},
	} {
		res := hcShape(t, "", nil, c.sub, log)
		if res.exitCode != 0 {
			t.Errorf("%s: exit %d, stderr %q", c.sub, res.exitCode, res.stderr)
		}
		assertNoLeak(t, c.sub, res.stdout+res.stderr)
		assertHas(t, c.sub, res.stdout, c.wants...)
	}
}

// hcStream is a stream-json run as Claude Code writes one: hook responses that
// echo the hook's output, an init event naming the account's tools and
// plugins, and a reply.
func hcStream(t *testing.T, sessionID, reply string, tokens ...string) string {
	t.Helper()
	ev := []map[string]any{
		{"type": "system", "subtype": "hook_response", "hook_name": "SessionStart:startup", "hook_event": "SessionStart",
			"output": leak, "stdout": leak + "-out", "stderr": "", "exit_code": 0, "outcome": "success", "session_id": sessionID},
		{"type": "system", "subtype": "hook_response", "hook_name": leak, "hook_event": leak,
			"output": "", "stdout": "", "stderr": leak, "exit_code": 1, "outcome": leak, "session_id": sessionID},
		{"type": "system", "subtype": "init", "session_id": sessionID, "cwd": "/" + leak, "tools": []string{},
			"mcp_servers": []map[string]any{{"name": leak, "status": "connected"}}, "plugins": []map[string]any{{"name": leak}}, "model": leak},
		{"type": "system", "subtype": "informational", "content": "UserPromptSubmit says: " + strings.Join(tokens, " ") + " " + leak},
		{"type": "result", "subtype": "success", "is_error": false, "num_turns": 1, "result": reply, "session_id": sessionID},
	}
	var b strings.Builder
	for _, e := range ev {
		b.WriteString(mustJSON(t, e) + "\n")
	}
	return b.String()
}

func TestH115_TheStreamSummariesPrintNoValue(t *testing.T) {
	dir := t.TempDir()
	ctxStart, smStart, ctxUps, smUps := "HC1-CTX-START-"+leak, "HC1-SM-START-"+leak, "HC1-CTX-UPS-"+leak, "HC1-SM-UPS-"+leak

	t.Run("hooks and hc1", func(t *testing.T) {
		stream := filepath.Join(dir, "hc1.ndjson")
		writeFile(t, stream, hcStream(t, "s1", ctxStart+"\n"+ctxUps+"\n"+leak, smUps))
		transcript := filepath.Join(dir, "s1.jsonl")
		writeFile(t, transcript, strings.Join([]string{
			mustJSON(t, map[string]any{"type": "attachment", "attachment": map[string]any{"type": "hook_additional_context", "hookEvent": "SessionStart", "content": []string{ctxStart}}}),
			mustJSON(t, map[string]any{"type": "attachment", "attachment": map[string]any{"type": "hook_system_message", "hookEvent": "UserPromptSubmit", "content": smUps}}),
			mustJSON(t, map[string]any{"type": "attachment", "attachment": map[string]any{"type": "credential_org", "organizationUuid": leak}}),
			mustJSON(t, map[string]any{"type": "user", "message": map[string]any{"content": leak}}),
		}, "\n")+"\n")

		res := hcShape(t, "", nil, "hooks", stream)
		assertNoLeak(t, "hooks", res.stdout+res.stderr)
		assertHas(t, "hooks", res.stdout,
			"hook_response SessionStart:startup: exit_code 0, outcome success, stdout 14 bytes, stderr 0 bytes",
			"hook_response other: exit_code 1, outcome other")

		res = hcShape(t, "", nil, "hc1", stream, transcript, ctxStart, smStart, ctxUps, smUps)
		assertNoLeak(t, "hc1", res.stdout+res.stderr)
		assertHas(t, "hc1", res.stdout,
			"SessionStart additionalContext yes, SessionStart systemMessage no, UserPromptSubmit additionalContext yes, UserPromptSubmit systemMessage no",
			"stream system/informational events: 1; carrying SessionStart's systemMessage no, UserPromptSubmit's yes",
			"hook_additional_context SessionStart 1 UserPromptSubmit 0; hook_system_message SessionStart 0 UserPromptSubmit 1")
	})

	t.Run("hc5 and hc6", func(t *testing.T) {
		d := filepath.Join(dir, "hc5")
		cfg := filepath.Join(dir, "cfg")
		nestedCwd := filepath.Join(d, "nested")
		n1, n2 := "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
		writeFile(t, filepath.Join(d, "n1.exit"), "0\n")
		writeFile(t, filepath.Join(d, "n2.exit"), "0\n")
		writeFile(t, filepath.Join(d, "n1.stream"), hcStream(t, n1, "PONG"))
		writeFile(t, filepath.Join(d, "n2.json"), mustJSON(t, map[string]any{"type": "result", "is_error": false, "result": "PONG", "session_id": n2})+"\n")
		writeFile(t, filepath.Join(d, "n1.debug"), strings.Join([]string{
			"2026-10-20T00:00:00.000Z [DEBUG] Broken symlink or missing file encountered for settings.json at path: " + filepath.Join(cfg, "settings.json"),
			"2026-10-20T00:00:00.000Z [DEBUG] Broken symlink or missing file encountered for settings.json at path: /etc/claude-code/managed-settings.json",
			"2026-10-20T00:00:00.000Z [DEBUG] Broken symlink or missing file encountered for settings.json at path: " + filepath.Join(nestedCwd, ".claude", "settings.json"),
			"2026-10-20T00:00:00.000Z [DEBUG] Broken symlink or missing file encountered for settings.json at path: /" + leak + "/settings.json",
			"2026-10-20T00:00:00.000Z [DEBUG] Replacing all allow rules for destination 'flagSettings' with 0 rule(s): []",
			"2026-10-20T00:00:00.000Z [DEBUG] Replacing all allow rules for destination '" + leak + "' with 0 rule(s): []",
			"2026-10-20T00:00:00.000Z [DEBUG] Remote settings: No settings found (404)",
			"2026-10-20T00:00:00.000Z [DEBUG] hooks module " + leak + "@builtin loaded",
		}, "\n")+"\n")
		writeFile(t, filepath.Join(d, "window"), "100.0\n200.0\n")
		// The recorder keeps a session id only as this digest; the second
		// nested run fires the same hooks under its own id, and an outer
		// Stop inside the window is the hook the nested runs are part of.
		digest := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:])[:16] }
		var events strings.Builder
		for _, r := range []map[string]any{
			{"tag": "outer", "t": 99.0, "event": "UserPromptSubmit"},
			{"tag": "outer", "t": 100.5, "event": "Stop"},
			{"tag": "nested", "t": 150.0, "event": "SessionStart", "session": digest(n1)},
			{"tag": "nested", "t": 151.0, "event": "UserPromptSubmit", "session": digest(n1)},
			{"tag": "outer", "t": 152.0, "event": "PreToolUse"},
			{"tag": "nested", "t": 160.0, "event": "SessionStart", "session": digest(n2)},
			{"tag": "outer", "t": 300.0, "event": "SessionEnd"},
		} {
			events.WriteString(mustJSON(t, r) + "\n")
		}
		writeFile(t, filepath.Join(d, "events.ndjson"), events.String())
		since := filepath.Join(d, "since")
		writeFile(t, since, "")
		writeFile(t, filepath.Join(cfg, "projects", "-"+leak, n1+".jsonl"), "c1-"+leak+"\n")

		res := hcShape(t, "", nil, "hc5", d, cfg, nestedCwd, n1)
		assertNoLeak(t, "hc5", res.stdout+res.stderr)
		assertHas(t, "hc5", res.stdout,
			"exit 0; result success, is_error no, num_turns 1; replied PONG yes; session id the one it was given yes",
			"tools 0, mcp servers listed yes",
			"SessionStart 1, UserPromptSubmit 1, Stop 0",
			"outer-session hooks other than its own Stop that fired while it ran: 1",
			"found missing: user, managed, project; other paths 1",
			"applied rules for: flagSettings; other 1",
			"remote settings consulted: yes")

		res = hcShape(t, "", nil, "hc6", d, cfg, n1, n2, since, "c1-"+leak, "c2-"+leak)
		assertNoLeak(t, "hc6", res.stdout+res.stderr)
		assertHas(t, "hc6", res.stdout,
			"exit 0; is_error no; replied PONG yes; session id the one it was given yes",
			"transcripts named for it under the projects directory: 0; for the run before it, which kept its session: 1",
			"holding its prompt's marker: 0; holding the earlier run's marker: 1")
	})

	t.Run("fm11", func(t *testing.T) {
		d := filepath.Join(dir, "fm11")
		sid := "33333333-3333-4333-8333-333333333333"
		writeFile(t, filepath.Join(d, "probe.log"), "1 start 0 0 0\n1 end 0 0 0\n2 start 0 0 0\n")
		writeFile(t, filepath.Join(d, "store", "runs", sid, "coverage.ndjson"), strings.Join([]string{
			mustJSON(t, map[string]any{"type": "coverage", "phase": "start", "cwd": "/" + leak}),
			mustJSON(t, map[string]any{"type": "coverage", "phase": "start", "cwd": "/" + leak}),
			mustJSON(t, map[string]any{"type": "coverage", "phase": "end", "cwd": "/" + leak}),
		}, "\n")+"\n")
		res := hcShape(t, "", nil, "fm11", filepath.Join(d, "probe.log"), filepath.Join(d, "store"), sid)
		assertNoLeak(t, "fm11", res.stdout+res.stderr)
		assertHas(t, "fm11", res.stdout,
			"probe start: invocations 2 (runs 1, 2), exit codes 0, stdout 0 bytes, stderr 0 bytes",
			"probe end: invocations 1 (runs 1), exit codes 0, stdout 0 bytes, stderr 0 bytes",
			"coverage records the probes wrote for this session: start 2, end 1")
	})
}

// hcStub is the stand-in claude. Every session it is given an id for leaves
// what a real one left outside the script's scratch when measured on 2.1.305:
// a cache-break-state file under $TMPDIR/claude-<uid> (the real one holds the
// account's model, API betas and tool names), an empty session-env directory
// and a transcript under the config directory. An earlier stand-in wrote
// nothing, so "leaves nothing behind" held here while a real run left all
// three. Each path it writes goes to $HC_STUB_WROTE, so the test can tell a
// cleanup that removed them from a stand-in that never wrote them. With
// $HC_STUB_HANG set, a session marks that file once its files are written and
// then takes three seconds, the window a hangup arrives in.
const hcStub = `#!/bin/sh
printf '%s\n' "$*" >> "$HC_STUB_LOG"
if [ "$1" = --version ]; then echo '0.0.0-stub (H-115)'; exit 0; fi
sid= prev=
for a in "$@"; do
  if [ "$prev" = --session-id ]; then sid=$a; fi
  prev=$a
done
if [ -n "$sid" ]; then
  cfg=${CLAUDE_CONFIG_DIR:-$HOME/.claude}
  tmpd=${TMPDIR:-/tmp}/claude-$(id -u)
  slug=$(pwd | tr -c 'A-Za-z0-9\n' '-')
  mkdir -p "$tmpd" "$cfg/session-env/$sid" "$cfg/projects/$slug"
  echo '{}' > "$tmpd/cache-break-state-$sid.json"
  : > "$cfg/projects/$slug/$sid.jsonl"
  printf '%s\n' "$tmpd/cache-break-state-$sid.json" "$cfg/session-env/$sid" "$cfg/projects/$slug/$sid.jsonl" >> "$HC_STUB_WROTE"
  if [ -n "${HC_STUB_HANG:-}" ]; then : > "$HC_STUB_HANG"; sleep 3; fi
fi
exit 1
`

// hcScript runs test/live/hc-items.sh against a stand-in claude, with a home
// and a temporary directory of its own, and returns what it printed, what the
// stand-in was asked and the paths the stand-in wrote.
func hcScript(t *testing.T, home string) (res result, asked []string, tmp string, wrote []string) {
	t.Helper()
	needTools(t, "sh", "python3", "git")
	stub := t.TempDir()
	stubLog := filepath.Join(t.TempDir(), "asked")
	stubWrote := filepath.Join(t.TempDir(), "wrote")
	writeFile(t, filepath.Join(stub, "claude"), hcStub)
	if err := os.Chmod(filepath.Join(stub, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp = t.TempDir()
	cmd := exec.Command("sh", filepath.Join(moduleRoot, "test", "live", "hc-items.sh"))
	cmd.Dir = moduleRoot
	cmd.Env = hcEnviron(home, stub, tmp, "HC_STUB_LOG="+stubLog, "HC_STUB_WROTE="+stubWrote)
	e := &env{t: t}
	res = e.wait(cmd)
	if b, err := os.ReadFile(stubLog); err == nil {
		asked = strings.Split(strings.TrimSpace(string(b)), "\n")
	}
	if b, err := os.ReadFile(stubWrote); err == nil {
		wrote = strings.Split(strings.TrimSpace(string(b)), "\n")
	}
	return res, asked, tmp, wrote
}

// hcEnviron is this process's environment with the home, PATH and TMPDIR a
// run of hc-items.sh against the stand-in in stub gets, and nothing of the
// caller's claude or rashomon configuration.
func hcEnviron(home, stub, tmp string, extra ...string) []string {
	var environ []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "HOME", "PATH", "TMPDIR", "CLAUDE_CONFIG_DIR", "RASHOMON_HOME", "RASHOMON_BIN":
			continue
		}
		environ = append(environ, kv)
	}
	environ = append(environ,
		"HOME="+home,
		"PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+tmp,
		"RASHOMON_BIN="+rashomonBin,
	)
	return append(environ, extra...)
}

func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && p != root {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestH115_RefusesAMachineWithAUserSettingsFile(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	const body = `{"hooks": {"SessionStart": []}}` + "\n"
	writeFile(t, settings, body)

	res, asked, tmp, _ := hcScript(t, home)
	if res.exitCode == 0 {
		t.Errorf("the script ran over an existing user settings file:\n%s", res.stdout)
	}
	if !strings.Contains(res.stderr, "refusing") {
		t.Errorf("stderr %q does not say it is refusing", res.stderr)
	}
	if len(asked) != 0 {
		t.Errorf("claude was invoked %v before the refusal; nothing should run first", asked)
	}
	if b, _ := os.ReadFile(settings); string(b) != body {
		t.Errorf("the user settings file changed: %q", b)
	}
	if left := filesUnder(t, tmp); len(left) != 0 {
		t.Errorf("a refused run left %v", left)
	}
}

func TestH115_RecordsTheVersionAndLeavesNothingBehind(t *testing.T) {
	// A home claude has run in before: the directories its sessions write
	// into are already there, and they are not the script's to remove.
	home := t.TempDir()
	for _, d := range []string{"projects", "session-env"} {
		if err := os.MkdirAll(filepath.Join(home, ".claude", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := filesUnder(t, home)
	res, asked, tmp, wrote := hcScript(t, home)
	t.Logf("what the script printed with every session failing:\n%s", res.stdout)
	if res.exitCode != 0 {
		t.Fatalf("exit %d with a claude whose every session fails; each check should say what it saw and go on\nstdout:\n%s\nstderr:\n%s", res.exitCode, res.stdout, res.stderr)
	}
	if len(asked) == 0 || asked[0] != "--version" {
		t.Errorf("the first thing asked of claude was %v, want --version", asked)
	}
	ran := 0
	for _, a := range asked {
		if strings.HasPrefix(a, "-p ") {
			ran++
		}
	}
	if ran == 0 {
		t.Errorf("no check asked claude for a session; the refusal is not what let this pass: %v", asked)
	}
	v := strings.Index(res.stdout, "0.0.0-stub (H-115)")
	first := strings.Index(res.stdout, "HC-1")
	if v < 0 || first < 0 || v > first {
		t.Errorf("the version is not recorded before the first check:\n%s", res.stdout)
	}
	// The positive twin: the stand-in did leave each kind of file a real
	// session leaves, so the checks below are about a cleanup and not about
	// a stand-in that wrote nothing.
	for _, kind := range []string{"/claude-", "/session-env/", "/projects/"} {
		n := 0
		for _, w := range wrote {
			if strings.Contains(w, kind) {
				n++
			}
		}
		if n == 0 {
			t.Errorf("the stand-in wrote nothing under %s, so its removal is not tested: %v", kind, wrote)
		}
	}
	for _, w := range wrote {
		if _, err := os.Lstat(w); !os.IsNotExist(err) {
			t.Errorf("a session's %s is still there after the run", w)
		}
	}
	if left := filesUnder(t, tmp); len(left) != 0 {
		t.Errorf("the run left its scratch, or Claude Code's temporary files for its sessions, in TMPDIR: %v", left)
	}
	if after := filesUnder(t, home); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Errorf("HOME held %v before the run and %v after it", before, after)
	}
	assertNoLeak(t, "the script's output", res.stdout+res.stderr)
}
