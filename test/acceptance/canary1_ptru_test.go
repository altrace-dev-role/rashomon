package acceptance

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// CANARY1 -- P-TRU-1 reads a hook's delivery to the model from what the
// client gave the model, per event, and its script runs two sessions against
// a scratch configuration and nothing else: a claude -p control, then an
// interactive session the owner starts with the one command it prints.
//
// test/live/p-tru-1.sh runs real Claude Code sessions, so its verdict is read
// by a person, and internal/steer/client.go's verified list grows only from
// the artifacts it writes. What can be held down without a session is the
// reader (test/live/p_tru_1.py) and the script's refusals and bookkeeping. The
// reader is fed transcripts in the shape 2.1.280 writes for a hook: a
// hook_success attachment holding the hook's stdout, which was given to no
// one; a hook_system_message attachment holding the line the user is shown;
// and a hook_additional_context attachment whose "rendered" is what the model
// was given. Delivered means the context token in "rendered", and nowhere
// else: a token in the hook's own record, in the user's line or carrying
// another run's nonce counts for nothing. The script runs against a stand-in
// claude that runs the probe hook and writes that shape, and the test plays
// the owner: it runs the printed command from a terminal of its own.
//
// Break: the reader taking the hook's record for delivery, matching a token
// of another nonce, passing a run whose control was not repeated, writing
// over an artifact, guessing a version from a mixed transcript, or misjudging
// the scratch settings; the script running at ~/.claude, over hooks or an
// enabled plugin, over an existing artifact, past a failed control, with the
// interactive session outside the scratch config, waiting without a limit,
// leaving its scratch behind, not recording the version, or swallowing the
// reader's verdict.

const (
	ptruNonce  = "0a1b2c3d4e5f6a7b"
	ptruOK     = "PostToolUse"
	ptruFail   = "PostToolUseFailure"
	ptruPrompt = "This session checks hook delivery. Use the Bash tool to run exactly: true\n" +
		"Then use the Bash tool to run exactly: false\n" +
		"Then reply with every line beginning PTRU- that you were shown after either call, verbatim, one per line, and nothing else."
)

func ptruReaderPath() string { return filepath.Join(moduleRoot, "test", "live", "p_tru_1.py") }

func ptruContext(event, nonce string) string { return "PTRU-CONTEXT-" + event + "-" + nonce }

func ptruMirror(event, nonce string) string { return "PTRU-MIRROR-" + event + "-" + nonce }

// ptruAttachment is one attachment entry as 2.1.280 writes it, with what the
// model was given when rendered is not empty.
func ptruAttachment(a map[string]any, rendered ...string) map[string]any {
	e := map[string]any{"type": "attachment", "version": "2.1.280", "entrypoint": "sdk-cli", "attachment": a}
	if len(rendered) > 0 {
		var r []map[string]string
		for _, s := range rendered {
			r = append(r, map[string]string{"content": s})
		}
		e["rendered"] = r
	}
	return e
}

// ptruHook is what one run of the probe hook for event leaves: its stdout,
// which holds both tokens; the user's line; and, when delivered, the context
// with what the model was given.
func ptruHook(event, nonce string, delivered bool) []map[string]any {
	ctx, mirror := ptruContext(event, nonce), ptruMirror(event, nonce)
	stdout := `{"systemMessage": "` + mirror + `", "hookSpecificOutput": {"hookEventName": "` + event +
		`", "additionalContext": "` + ctx + `"}}`
	out := []map[string]any{
		ptruAttachment(map[string]any{"type": "hook_success", "hookEvent": event, "stdout": stdout}),
		ptruAttachment(map[string]any{"type": "hook_system_message", "hookEvent": event, "content": mirror}),
	}
	if delivered {
		out = append(out, ptruAttachment(
			map[string]any{"type": "hook_additional_context", "hookEvent": event, "content": []string{ctx}},
			"<system-reminder>\n"+event+" hook additional context: "+ctx+"\n</system-reminder>"))
	}
	return out
}

// ptruSaid is the assistant's reply, with the canary on a line of its own:
// message text the artifact must not keep.
func ptruSaid(lines ...string) map[string]any {
	text := strings.Join(append(lines, leak), "\n")
	return map[string]any{"type": "assistant", "version": "2.1.280", "entrypoint": "sdk-cli",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}}}
}

// ptruDelivered is a session whose two hooks were both delivered and repeated.
func ptruDelivered() []map[string]any {
	return append(append(ptruHook(ptruOK, ptruNonce, true), ptruHook(ptruFail, ptruNonce, true)...),
		ptruSaid(ptruContext(ptruOK, ptruNonce), ptruContext(ptruFail, ptruNonce)))
}

type ptruEvent struct {
	Event       string `json:"event"`
	Fired       bool   `json:"fired"`
	Delivered   bool   `json:"delivered"`
	Echoed      bool   `json:"echoed"`
	Mirror      bool   `json:"mirror"`
	MirrorModel bool   `json:"mirror_reached_model"`
}

type ptruArtifact struct {
	Probe         string      `json:"probe"`
	Version       *string     `json:"version"`
	VersionSource *string     `json:"version_source"`
	Versions      []string    `json:"transcript_versions"`
	Entrypoints   []string    `json:"entrypoints"`
	SettingsKeys  *[]string   `json:"settings_keys"`
	Nonce         string      `json:"nonce"`
	Transcript    string      `json:"transcript"`
	SHA256        string      `json:"transcript_sha256"`
	ControlOK     bool        `json:"control_ok"`
	Events        []ptruEvent `json:"events"`
}

// ptruFixture writes lines as a session transcript, beside a stream holding
// the init event, and returns both paths.
func ptruFixture(t *testing.T, lines []map[string]any) (stream, transcript string) {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(mustJSON(t, l) + "\n")
	}
	transcript = filepath.Join(dir, "session.jsonl")
	writeFile(t, transcript, b.String())
	stream = filepath.Join(dir, "stream.ndjson")
	writeFile(t, stream, `{"type": "system", "subtype": "init", "claude_code_version": "2.1.280", "session_id": "s"}`+
		"\n"+`{"type": "result", "subtype": "success"}`+"\n")
	return stream, transcript
}

// ptruReport runs the reader's report over transcript, with the stream when
// it is not empty, and keys as the scratch settings' key names.
func ptruReport(t *testing.T, stream, transcript, nonce, keys, out string) result {
	t.Helper()
	needTools(t, "python3")
	args := []string{"-I", ptruReaderPath(), "report"}
	if stream != "" {
		args = append(args, "--stream", stream)
	}
	args = append(args, "--transcript", transcript, "--nonce", nonce, "--settings-keys", keys, "--out", out)
	return (&env{t: t}).wait(exec.Command("python3", args...))
}

func ptruLoad(t *testing.T, path string) (ptruArtifact, []byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no artifact: %v", err)
	}
	var art ptruArtifact
	if err := json.Unmarshal(b, &art); err != nil {
		t.Fatalf("the artifact is not JSON: %v\n%s", err, b)
	}
	return art, b
}

func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestCANARY1_TheReaderReadsDeliveryFromWhatTheModelWasGiven(t *testing.T) {
	const other = "ffffeeeeddddcccc"
	okCtx, failCtx := ptruContext(ptruOK, ptruNonce), ptruContext(ptruFail, ptruNonce)
	failAttach := func(kind string, content any, rendered ...string) []map[string]any {
		return []map[string]any{ptruAttachment(map[string]any{"type": kind, "hookEvent": ptruFail, "content": content}, rendered...)}
	}
	session := func(control bool, fail []map[string]any, said ...string) []map[string]any {
		return append(append(ptruHook(ptruOK, ptruNonce, control), fail...), ptruSaid(said...))
	}
	all := ptruEvent{Fired: true, Delivered: true, Echoed: true, Mirror: true}
	for _, tc := range []struct {
		name    string
		lines   []map[string]any
		control bool
		fail    ptruEvent
	}{
		{"both delivered and repeated", ptruDelivered(), true, all},
		{"the failure's context not given to the model", session(true, ptruHook(ptruFail, ptruNonce, false), okCtx), true,
			ptruEvent{Fired: true, Mirror: true}},
		{"the failure hook never ran", session(true, nil, okCtx), true, ptruEvent{}},
		{"the failure's context given and not repeated", session(true, ptruHook(ptruFail, ptruNonce, true), okCtx), true,
			ptruEvent{Fired: true, Delivered: true, Mirror: true}},
		{"another run's tokens", session(true, ptruHook(ptruFail, other, true), okCtx, ptruContext(ptruFail, other)), true,
			ptruEvent{Fired: true}},
		{"the context in the hook's record and not rendered", session(true, failAttach("hook_additional_context", []string{failCtx}), okCtx), true,
			ptruEvent{Fired: true}},
		{"the context in the user's line", session(true, failAttach("hook_system_message", failCtx), okCtx), true,
			ptruEvent{Fired: true}},
		{"the mirror given to the model", session(true, failAttach("hook_additional_context", []string{failCtx},
			failCtx, ptruMirror(ptruFail, ptruNonce)), okCtx, failCtx), true,
			ptruEvent{Fired: true, Delivered: true, Echoed: true, MirrorModel: true}},
		{"the control not given to the model", session(false, ptruHook(ptruFail, ptruNonce, true), failCtx), false, all},
		{"the control given and not repeated", session(true, ptruHook(ptruFail, ptruNonce, true), failCtx), false, all},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream, transcript := ptruFixture(t, tc.lines)
			out := filepath.Join(t.TempDir(), "artifact.json")
			res := ptruReport(t, stream, transcript, ptruNonce, "null", out)
			exit := 0
			if !tc.control {
				exit = 2
			}
			if res.exitCode != exit {
				t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", res.exitCode, exit, res.stdout, res.stderr)
			}
			art, b := ptruLoad(t, out)
			if art.ControlOK != tc.control {
				t.Errorf("control_ok = %v, want %v", art.ControlOK, tc.control)
			}
			want := tc.fail
			want.Event = ptruFail
			if len(art.Events) != 2 || art.Events[1] != want {
				t.Errorf("events = %+v, want the failure read as %+v", art.Events, want)
			}
			if okWant := (ptruEvent{Event: ptruOK, Fired: true, Delivered: true, Echoed: true, Mirror: true}); tc.control &&
				(len(art.Events) == 0 || art.Events[0] != okWant) {
				t.Errorf("events = %+v, want the control read as %+v", art.Events, okWant)
			}
			if art.Version == nil || *art.Version != "2.1.280" || art.VersionSource == nil || *art.VersionSource != "init" ||
				art.Nonce != ptruNonce || art.Transcript != transcript || art.SettingsKeys != nil {
				t.Errorf("version %v from %v, nonce %q, transcript %q, settings keys %v; want 2.1.280 from init, %q, %q, null",
					art.Version, art.VersionSource, art.Nonce, art.Transcript, art.SettingsKeys, ptruNonce, transcript)
			}
			if got := sha256Hex(t, transcript); art.SHA256 != got {
				t.Errorf("transcript_sha256 = %q, the transcript's is %q", art.SHA256, got)
			}
			assertNoLeak(t, "the artifact", string(b))
			assertNoLeak(t, "what the reader printed", res.stdout+res.stderr)
			if strings.Contains(string(b), "system-reminder") {
				t.Errorf("the artifact keeps what the model was given:\n%s", b)
			}
		})
	}
}

// TestCANARY1_TheReaderTakesAStreamlessVersionFromTheTranscript: an
// interactive session has no stream, so its version is the one the
// transcript records, and none when it records two; the scratch settings'
// key names are kept as given.
func TestCANARY1_TheReaderTakesAStreamlessVersionFromTheTranscript(t *testing.T) {
	mixed := append(ptruDelivered(), map[string]any{"type": "system", "version": "2.1.281"})
	for _, tc := range []struct {
		name    string
		lines   []map[string]any
		version string // "": null
	}{
		{"one version", ptruDelivered(), "2.1.280"},
		{"two versions", mixed, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, transcript := ptruFixture(t, tc.lines)
			out := filepath.Join(t.TempDir(), "artifact.json")
			if res := ptruReport(t, "", transcript, ptruNonce, `["model", "theme"]`, out); res.exitCode != 0 {
				t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", res.exitCode, res.stdout, res.stderr)
			}
			art, _ := ptruLoad(t, out)
			switch {
			case tc.version == "" && (art.Version != nil || art.VersionSource != nil):
				t.Errorf("version %v from %v over %v, want null", art.Version, art.VersionSource, art.Versions)
			case tc.version != "" && (art.Version == nil || *art.Version != tc.version ||
				art.VersionSource == nil || *art.VersionSource != "transcript"):
				t.Errorf("version %v from %v, want %s from transcript", art.Version, art.VersionSource, tc.version)
			}
			if art.SettingsKeys == nil || !slices.Equal(*art.SettingsKeys, []string{"model", "theme"}) {
				t.Errorf("settings keys %v, want [model theme]", art.SettingsKeys)
			}
		})
	}
	_, transcript := ptruFixture(t, ptruDelivered())
	out := filepath.Join(t.TempDir(), "artifact.json")
	if res := ptruReport(t, "", transcript, ptruNonce, `{"theme": "dark"}`, out); res.exitCode == 0 {
		t.Errorf("the reader took settings values for key names")
	}
}

func TestCANARY1_TheReaderNeverWritesOverAnArtifact(t *testing.T) {
	stream, transcript := ptruFixture(t, ptruDelivered())
	dir := t.TempDir()
	kept := filepath.Join(dir, "kept.json")
	writeFile(t, kept, "kept\n")
	if res := ptruReport(t, stream, transcript, ptruNonce, "null", kept); res.exitCode == 0 {
		t.Errorf("the reader wrote over an existing artifact:\n%s", res.stdout)
	}
	if b, _ := os.ReadFile(kept); string(b) != "kept\n" {
		t.Errorf("the existing artifact changed: %q", b)
	}
	// The positive twin: the same session to a new path is an artifact, so
	// the refusal above is about the path and not the session.
	fresh := filepath.Join(dir, "fresh.json")
	if res := ptruReport(t, stream, transcript, ptruNonce, "null", fresh); res.exitCode != 0 {
		t.Fatalf("exit %d on a fresh path\nstdout:\n%s\nstderr:\n%s", res.exitCode, res.stdout, res.stderr)
	}
	odd := filepath.Join(dir, "odd.json")
	if res := ptruReport(t, stream, transcript, "0a1b.*", "null", odd); res.exitCode == 0 {
		t.Errorf("the reader took a nonce that is not letters and digits")
	}
	if _, err := os.Stat(odd); !os.IsNotExist(err) {
		t.Errorf("a refused nonce left an artifact: %v", err)
	}
}

// ptruStub is a stand-in claude for p-tru-1.sh. It answers --version as
// 2.1.280 does. Given a session it logs its entrypoint (sdk-cli under -p, cli
// otherwise), the config directory and prompt it was given, its flags and its
// --settings hooks; runs the PostToolUse hook for a Bash call that succeeds
// and the PostToolUseFailure hook for one that fails; and writes what 2.1.280
// writes for them under the config directory's projects, $HOME/.claude when
// CLAUDE_CONFIG_DIR is unset: each hook's stdout, the user's line and, unless
// $PTRU_STUB_DROP lists "<entrypoint>:<event>", the context with what the
// model was given; then a reply repeating every context it was given. Under
// -p it prints the stream's init and result events.
const ptruStub = `#!/usr/bin/env python3
import json, os, re, subprocess, sys
a = sys.argv[1:]
log = open(os.environ["PTRU_STUB_LOG"], "a")
if a == ["--version"]:
    log.write(json.dumps({"version": True}) + "\n")
    print("2.1.280 (Claude Code)")
    sys.exit(0)
entry = "sdk-cli" if "-p" in a else "cli"
cfg = os.environ.get("CLAUDE_CONFIG_DIR") or os.path.join(os.environ["HOME"], ".claude")
hooks = json.load(open(a[a.index("--settings") + 1]))["hooks"]
log.write(json.dumps({"entrypoint": entry, "config": cfg, "prompt": a[a.index("-p") + 1] if entry == "sdk-cli" else a[-1],
                      "flags": [x for x in a if x.startswith("-")], "hooks": sorted(hooks),
                      "commands": [h["command"] for ev in sorted(hooks) for m in hooks[ev] for h in m["hooks"]]}) + "\n")
log.flush()
sid = a[a.index("--session-id") + 1]
d = os.path.join(cfg, "projects", re.sub("[^A-Za-z0-9]", "-", os.getcwd()))
os.makedirs(d, exist_ok=True)
drop = os.environ.get("PTRU_STUB_DROP", "").split()
base = {"type": "attachment", "version": "2.1.280", "entrypoint": entry, "sessionId": sid}
out, said = [], []
for ev, cmd in (("PostToolUse", "true"), ("PostToolUseFailure", "false")):
    payload = json.dumps({"hook_event_name": ev, "tool_name": "Bash", "tool_input": {"command": cmd}})
    r = subprocess.run(hooks[ev][0]["hooks"][0]["command"], shell=True, input=payload, capture_output=True, text=True)
    o = json.loads(r.stdout)
    out.append(dict(base, attachment={"type": "hook_success", "hookEvent": ev, "stdout": r.stdout}))
    out.append(dict(base, attachment={"type": "hook_system_message", "hookEvent": ev, "content": o["systemMessage"]}))
    if entry + ":" + ev not in drop:
        ctx = o["hookSpecificOutput"]["additionalContext"]
        rendered = "<system-reminder>\n" + ev + " hook additional context: " + ctx + "\n</system-reminder>"
        out.append(dict(base, attachment={"type": "hook_additional_context", "hookEvent": ev, "content": [ctx]},
                        rendered=[{"content": rendered}]))
        said.append(ctx)
out.append(dict(base, type="assistant", message={"role": "assistant", "content": [{"type": "text", "text": "\n".join(said) or "none"}]}))
with open(os.path.join(d, sid + ".jsonl"), "w") as f:
    f.writelines(json.dumps(e) + "\n" for e in out)
if entry == "sdk-cli":
    print(json.dumps({"type": "system", "subtype": "init", "claude_code_version": "2.1.280", "session_id": sid}))
    print(json.dumps({"type": "result", "subtype": "success"}))
`

// ptruAsk is one call the stand-in logged: --version, or a session.
type ptruAsk struct {
	Version    bool     `json:"version"`
	Entrypoint string   `json:"entrypoint"`
	Config     string   `json:"config"`
	Prompt     string   `json:"prompt"`
	Flags      []string `json:"flags"`
	Hooks      []string `json:"hooks"`
	Commands   []string `json:"commands"`
}

// ptruRun is one run of p-tru-1.sh against the stand-in.
type ptruRun struct {
	res   result
	asked []ptruAsk
	tmp   string
	ran   bool // the owner ran the command the script printed
}

// ptruScript runs test/live/p-tru-1.sh from dir against the stand-in, with a
// home and a temporary directory of its own; cfg, when not empty, is its
// CLAUDE_CONFIG_DIR. When owner is set the test plays the owner: the command
// the script prints is run from a terminal of its own, whose environment has
// no CLAUDE_CONFIG_DIR. A script still running after 30 s is killed.
func ptruScript(t *testing.T, home, cfg, dir string, owner bool, args []string, extra ...string) ptruRun {
	t.Helper()
	needTools(t, "sh", "python3")
	stub := t.TempDir()
	stubLog := filepath.Join(t.TempDir(), "asked")
	writeFile(t, filepath.Join(stub, "claude"), ptruStub)
	if err := os.Chmod(filepath.Join(stub, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := ptruRun{tmp: t.TempDir()}
	extra = append(extra, "PTRU_STUB_LOG="+stubLog)
	// PWD as a terminal sets it, so a relative CLAUDE_CONFIG_DIR resolves
	// against dir as given and not through the symlinks under it.
	scriptEnv := append(slices.Clone(extra), "PWD="+dir)
	if cfg != "" {
		scriptEnv = append(scriptEnv, "CLAUDE_CONFIG_DIR="+cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", append([]string{filepath.Join(moduleRoot, "test", "live", "p-tru-1.sh")}, args...)...)
	cmd.Dir = dir
	cmd.Env = hcEnviron(home, stub, r.tmp, scriptEnv...)
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	for sc := bufio.NewScanner(pipe); sc.Scan(); {
		line := sc.Text()
		stdout.WriteString(line + "\n")
		if path, ok := strings.CutPrefix(line, "  sh "); ok && owner && strings.HasSuffix(path, "/run.sh") {
			term := exec.Command("sh", path)
			term.Dir = t.TempDir()
			term.Env = hcEnviron(home, stub, r.tmp, extra...)
			if out, err := term.CombinedOutput(); err != nil {
				t.Errorf("the printed command failed: %v\n%s", err, out)
			}
			r.ran = true
		}
	}
	_ = cmd.Wait()
	if ctx.Err() != nil {
		t.Errorf("the script was still running after 30 s and was killed")
	}
	r.res = result{exitCode: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	if b, err := os.ReadFile(stubLog); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var a ptruAsk
			if json.Unmarshal([]byte(l), &a) == nil {
				r.asked = append(r.asked, a)
			}
		}
	}
	return r
}

func TestCANARY1_TheScriptRefusesBeforeAnySession(t *testing.T) {
	scratch := func(t *testing.T, home string) string {
		cfg := filepath.Join(home, ".claude-ptru")
		if err := os.MkdirAll(cfg, 0o755); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	userDir := func(t *testing.T, home string) string {
		d := filepath.Join(home, ".claude")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	settings := func(body string) func(t *testing.T, home, art string) (string, []string) {
		return func(t *testing.T, home, art string) (string, []string) {
			cfg := scratch(t, home)
			writeFile(t, filepath.Join(cfg, "settings.json"), body)
			return cfg, []string{art}
		}
	}
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, home, art string) (cfg string, args []string)
	}{
		{"no artifact directory named", func(t *testing.T, home, art string) (string, []string) { return scratch(t, home), nil }},
		{"CLAUDE_CONFIG_DIR unset", func(t *testing.T, home, art string) (string, []string) { return "", []string{art} }},
		{"CLAUDE_CONFIG_DIR not a directory", func(t *testing.T, home, art string) (string, []string) {
			return filepath.Join(home, ".claude-ptru"), []string{art}
		}},
		{"CLAUDE_CONFIG_DIR is ~/.claude", func(t *testing.T, home, art string) (string, []string) {
			return userDir(t, home), []string{art}
		}},
		{"CLAUDE_CONFIG_DIR is ~/.claude by another name", func(t *testing.T, home, art string) (string, []string) {
			alias := filepath.Join(home, "alias")
			if err := os.Symlink(userDir(t, home), alias); err != nil {
				t.Fatal(err)
			}
			return alias, []string{art}
		}},
		{"hooks in the scratch settings", settings(`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "true"}]}]}}`)},
		{"an enabled plugin in the scratch settings", settings(`{"enabledPlugins": {"p@m": true}}`)},
		{"scratch settings that are not JSON", settings(`{`)},
		{"an artifact in the directory", func(t *testing.T, home, art string) (string, []string) {
			writeFile(t, filepath.Join(art, "cli.json"), "kept\n")
			return scratch(t, home), []string{art}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			art := filepath.Join(t.TempDir(), "ptru")
			cfg, args := tc.setup(t, home, art)
			before, artBefore := filesUnder(t, home), filesUnder(t, filepath.Dir(art))
			r := ptruScript(t, home, cfg, t.TempDir(), true, args)
			if r.res.exitCode != 1 || r.res.stderr == "" {
				t.Errorf("exit %d, stderr %q; want 1 and a refusal\nstdout:\n%s", r.res.exitCode, r.res.stderr, r.res.stdout)
			}
			if len(r.asked) != 0 || r.ran {
				t.Errorf("claude was asked %+v (the owner's command run: %v) before the refusal; nothing should run first", r.asked, r.ran)
			}
			if after := filesUnder(t, home); !slices.Equal(before, after) {
				t.Errorf("the home changed from %v to %v", before, after)
			}
			if after := filesUnder(t, filepath.Dir(art)); !slices.Equal(artBefore, after) {
				t.Errorf("the artifact directory changed from %v to %v", artBefore, after)
			}
			if b, err := os.ReadFile(filepath.Join(art, "cli.json")); err == nil && string(b) != "kept\n" {
				t.Errorf("an existing artifact changed: %q", b)
			}
			if left := filesUnder(t, r.tmp); len(left) != 0 {
				t.Errorf("a refused run left %v", left)
			}
		})
	}
}

// ptruSurface is what one session's artifact should say.
type ptruSurface struct {
	written, control, delivered bool
}

func TestCANARY1_TheScriptRunsTheControlThenTheOwnersSession(t *testing.T) {
	keys := []string{"enabledPlugins", "hooks", "theme"}
	for _, tc := range []struct {
		name, drop string
		owner      bool
		relCfg     bool // CLAUDE_CONFIG_DIR given relative to where the script runs
		exit       int
		ran        bool
		sdk, cli   ptruSurface
	}{
		{"both surfaces delivered", "", true, false, 0, true, ptruSurface{true, true, true}, ptruSurface{true, true, true}},
		{"the failure dropped on the cli surface", "cli:" + ptruFail, true, true, 0, true,
			ptruSurface{true, true, true}, ptruSurface{true, true, false}},
		{"the failure dropped under -p alone", "sdk-cli:" + ptruFail, true, false, 0, true,
			ptruSurface{true, true, false}, ptruSurface{true, true, true}},
		{"the -p control dropped", "sdk-cli:" + ptruOK, true, false, 2, false,
			ptruSurface{true, false, true}, ptruSurface{}},
		{"the cli control dropped", "cli:" + ptruOK, true, false, 2, true,
			ptruSurface{true, true, true}, ptruSurface{true, false, true}},
		{"the owner never runs it", "", false, false, 1, false, ptruSurface{true, true, true}, ptruSurface{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, dir := t.TempDir(), t.TempDir()
			cfg := filepath.Join(home, ".claude-ptru")
			writeFile(t, filepath.Join(cfg, "settings.json"), `{"theme": "dark", "hooks": {}, "enabledPlugins": {"p@m": false}}`)
			given := cfg
			if tc.relCfg {
				rel, err := filepath.Rel(dir, cfg)
				if err != nil {
					t.Fatal(err)
				}
				given = rel
			}
			r := ptruScript(t, home, given, dir, tc.owner, []string{"ptru"}, "PTRU_STUB_DROP="+tc.drop, "PTRU_WAIT=3")
			if r.res.exitCode != tc.exit || r.ran != tc.ran {
				t.Fatalf("exit %d with the owner's command run %v, want %d and %v\nstdout:\n%s\nstderr:\n%s",
					r.res.exitCode, r.ran, tc.exit, tc.ran, r.res.stdout, r.res.stderr)
			}
			assertHas(t, "what the script printed", r.res.stdout, "claude --version: 2.1.280 (Claude Code)")
			ptruCheckAsked(t, r.asked, cfg, tc.ran)
			art := filepath.Join(dir, "ptru")
			sdk := ptruCheckSurface(t, filepath.Join(art, "sdk-cli.json"), cfg, "sdk-cli", tc.sdk, keys)
			cli := ptruCheckSurface(t, filepath.Join(art, "cli.json"), cfg, "cli", tc.cli, keys)
			if tc.cli.written && sdk.Nonce == cli.Nonce {
				t.Errorf("both sessions ran with nonce %q; each needs its own", sdk.Nonce)
			}
			if left := filesUnder(t, r.tmp); len(left) != 0 {
				t.Errorf("the run left %v in TMPDIR", left)
			}
			for _, p := range filesUnder(t, home) {
				if p != cfg && !strings.HasPrefix(p, cfg+string(os.PathSeparator)) {
					t.Errorf("the run wrote %s outside the scratch config", p)
				}
			}
		})
	}
}

// ptruCheckAsked checks what the stand-in was asked: the version first, then
// a claude -p session, then, when the owner ran the printed command, an
// interactive one; each in the scratch config, with the frozen prompt and
// the probe on the two events alone.
func ptruCheckAsked(t *testing.T, asked []ptruAsk, cfg string, ran bool) {
	t.Helper()
	want := []string{"", "sdk-cli"}
	if ran {
		want = append(want, "cli")
	}
	if len(asked) != len(want) || !asked[0].Version {
		t.Fatalf("the stand-in was asked %+v, want --version and then sessions %v", asked, want[1:])
	}
	for i, s := range asked[1:] {
		if s.Entrypoint != want[i+1] || slices.Contains(s.Flags, "-p") != (s.Entrypoint == "sdk-cli") {
			t.Errorf("session %d ran as %s with flags %v, want %s", i+1, s.Entrypoint, s.Flags, want[i+1])
		}
		if s.Config != cfg || s.Prompt != ptruPrompt || !slices.Contains(s.Flags, "--session-id") {
			t.Errorf("session %d ran in %q with prompt %q and flags %v, want the scratch config %q, the frozen prompt and a session id",
				i+1, s.Config, s.Prompt, s.Flags, cfg)
		}
		if !slices.Equal(s.Hooks, []string{ptruOK, ptruFail}) || len(s.Commands) != 2 {
			t.Errorf("session %d's hooks are %v with commands %v, want the probe on %s and %s alone",
				i+1, s.Hooks, s.Commands, ptruOK, ptruFail)
		}
		for _, c := range s.Commands {
			if strings.Contains(c, "rashomon") {
				t.Errorf("a hook command names rashomon: %q", c)
			}
		}
	}
}

// ptruCheckSurface reads the artifact a session wrote at path and checks it
// against want and the transcript the run kept under cfg; an artifact that
// should not exist must not.
func ptruCheckSurface(t *testing.T, path, cfg, entrypoint string, want ptruSurface, keys []string) ptruArtifact {
	t.Helper()
	if !want.written {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s exists, want no %s artifact: %v", path, entrypoint, err)
		}
		return ptruArtifact{}
	}
	art, _ := ptruLoad(t, path)
	source := map[string]string{"sdk-cli": "init", "cli": "transcript"}[entrypoint]
	if art.Version == nil || *art.Version != "2.1.280" || art.VersionSource == nil || *art.VersionSource != source ||
		!slices.Equal(art.Entrypoints, []string{entrypoint}) || art.ControlOK != want.control {
		t.Errorf("%s: version %v from %v, entrypoints %v, control_ok %v; want 2.1.280 from %s, [%s], %v",
			entrypoint, art.Version, art.VersionSource, art.Entrypoints, art.ControlOK, source, entrypoint, want.control)
	}
	if len(art.Events) != 2 || !art.Events[1].Fired || art.Events[1].Delivered != want.delivered {
		t.Errorf("%s: events = %+v, want %s fired and delivered %v", entrypoint, art.Events, ptruFail, want.delivered)
	}
	if art.SettingsKeys == nil || !slices.Equal(*art.SettingsKeys, keys) {
		t.Errorf("%s: settings keys %v, want %v", entrypoint, art.SettingsKeys, keys)
	}
	if rel, err := filepath.Rel(filepath.Join(cfg, "projects"), art.Transcript); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("%s: the transcript %q is not under the scratch config's projects", entrypoint, art.Transcript)
	}
	if got := sha256Hex(t, art.Transcript); art.SHA256 != got {
		t.Errorf("%s: transcript_sha256 = %q, the kept transcript's is %q", entrypoint, art.SHA256, got)
	}
	return art
}
