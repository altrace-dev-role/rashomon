package acceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// VERSION-GATE1 (n) -- test/live/p-tru-1.sh, the probe a floor in
// internal/steer/client.go stands on, passes an event only when both halves
// of the one hook output land.
//
// The script runs a real Claude Code session per event, so what it sees is
// Claude Code's; what can be held down without one is how it judges what it
// sees. It runs here against a stand-in claude that runs the script's own hook
// and records each half where the real one would, or leaves one out: the
// additionalContext in a hook_additional_context attachment of the session's
// transcript, the systemMessage in a system/informational event on the
// stream. The verdict passes an event only on both, under that event; fails it
// on a missing half; and calls it inconclusive when the hook never ran (no
// login), which must never read as a pass or as a failure of the channel. It
// keeps no nonce in what it prints or logs, refuses a configuration with a
// settings file before claude runs, and leaves the configuration directory as
// it found it.
//
// Break: the verdict reading one half only, an attachment of another event, or
// a hook that never ran as a failure; the exit code losing a failure or
// letting an inconclusive event outrank one; the refusal, the probe's own
// configuration directory, the command each event runs, or the transcript's
// removal dropped.

// ptruStub is the stand-in claude. $PTRU_STUB_<event> picks what it records
// for the session of that event: pass (both halves), noctx, nosm, otherevent
// (the additionalContext attachment under the other event) or nohook (the hook
// never runs, as with no login).
const ptruStub = `#!/usr/bin/env python3
import json, os, subprocess, sys
args = sys.argv[1:]
with open(os.environ["PTRU_STUB_LOG"], "a") as f:
    f.write(json.dumps(args) + "\n")
if args[:1] == ["--version"]:
    print("0.0.0-stub (P-TRU-1)")
    sys.exit(0)
def flag(name):
    return args[args.index(name) + 1]
sid = flag("--session-id")
with open(flag("--settings")) as f:
    (event, groups), = json.load(f)["hooks"].items()
cfg = os.environ.get("CLAUDE_CONFIG_DIR") or os.path.join(os.environ["HOME"], ".claude")
proj = os.path.join(cfg, "projects", os.getcwd().replace("/", "-"))
os.makedirs(proj, exist_ok=True)
os.makedirs(os.path.join(cfg, "session-env", sid), exist_ok=True)
mode = os.environ.get("PTRU_STUB_" + event, "pass")
stream, recs = [], []
if mode == "nohook":
    stream.append({"type": "result", "subtype": "success", "is_error": True, "result": "Not logged in"})
else:
    cmd = groups[0]["hooks"][0]["command"]
    out = json.loads(subprocess.run(cmd, shell=True, input=b"{}", capture_output=True).stdout)
    ctx, sm = out["hookSpecificOutput"]["additionalContext"], out["systemMessage"]
    if mode != "noctx":
        on = "PostToolUse" if mode == "otherevent" else event
        recs.append({"type": "attachment", "attachment": {"type": "hook_additional_context", "content": [ctx], "hookEvent": on}})
    recs.append({"type": "attachment", "attachment": {"type": "hook_system_message", "content": sm, "hookEvent": event}})
    if mode != "nosm":
        stream.append({"type": "system", "subtype": "informational", "content": sm})
    stream.append({"type": "result", "subtype": "success", "result": ctx})
with open(os.path.join(proj, sid + ".jsonl"), "w") as f:
    for r in recs:
        f.write(json.dumps(r) + "\n")
for e in stream:
    print(json.dumps(e))
`

// ptruRun is one run of p-tru-1.sh against the stand-in, with modes the
// stand-in's per-event modes and args the script's: what it printed, the
// arguments of each claude call, and the logs it wrote.
func ptruRun(t *testing.T, home string, modes map[string]string, args ...string) (res result, asked [][]string, logs map[string]string) {
	t.Helper()
	needTools(t, "sh", "python3")
	stub := t.TempDir()
	stubLog := filepath.Join(t.TempDir(), "asked")
	writeFile(t, filepath.Join(stub, "claude"), ptruStub)
	if err := os.Chmod(filepath.Join(stub, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(t.TempDir(), "logs")
	extra := []string{"PTRU_STUB_LOG=" + stubLog, "PTRU_LOGS=" + logDir}
	for ev, m := range modes {
		extra = append(extra, "PTRU_STUB_"+ev+"="+m)
	}
	cmd := exec.Command("sh", append([]string{filepath.Join(moduleRoot, "test", "live", "p-tru-1.sh")}, args...)...)
	cmd.Dir = moduleRoot
	cmd.Env = hcEnviron(home, stub, t.TempDir(), extra...)
	e := &env{t: t}
	res = e.wait(cmd)
	if b, err := os.ReadFile(stubLog); err == nil {
		for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var a []string
			if err := json.Unmarshal([]byte(ln), &a); err != nil {
				t.Fatalf("the stand-in's log line %q: %v", ln, err)
			}
			asked = append(asked, a)
		}
	}
	logs = map[string]string{}
	entries, _ := os.ReadDir(logDir)
	for _, de := range entries {
		b, err := os.ReadFile(filepath.Join(logDir, de.Name()))
		if err != nil {
			t.Fatal(err)
		}
		logs[de.Name()] = string(b)
	}
	return res, asked, logs
}

// ptruHome is a home whose probe configuration directory, .claude-probe,
// claude has run in before: its projects and session-env directories exist
// and are not the script's to remove.
func ptruHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{"projects", "session-env"} {
		if err := os.MkdirAll(filepath.Join(home, ".claude-probe", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// verdictOf is the verdict line of the one log of event, "" with none.
func verdictOf(t *testing.T, logs map[string]string, event string) string {
	t.Helper()
	var got []string
	for name, body := range logs {
		if strings.HasPrefix(name, "p-tru-1-"+event+"-") {
			for _, ln := range strings.Split(body, "\n") {
				if v, ok := strings.CutPrefix(ln, "  verdict: "); ok {
					got = append(got, v)
				}
			}
		}
	}
	if len(got) != 1 {
		t.Errorf("%s: verdicts %q in the logs, want one", event, got)
		return ""
	}
	return got[0]
}

func TestVG1n_TheProbePassesOnlyWhenBothHalvesLand(t *testing.T) {
	const inconclusive = "inconclusive: no login, or the model did not run the command"
	for _, tc := range []struct {
		name         string
		modes        map[string]string
		exit         int
		post, failed string
	}{
		{"both halves on both events", nil, 0, "pass", "pass"},
		{"no systemMessage on the stream", map[string]string{"PostToolUse": "nosm"}, 1, "fail", "pass"},
		{"no additionalContext attachment", map[string]string{"PostToolUseFailure": "noctx"}, 1, "pass", "fail"},
		{"the attachment under another event", map[string]string{"PostToolUseFailure": "otherevent"}, 1, "pass", "fail"},
		{"the hook never ran", map[string]string{"PostToolUse": "nohook", "PostToolUseFailure": "nohook"}, 3, inconclusive, inconclusive},
		{"a failure outranks an inconclusive event", map[string]string{"PostToolUse": "nosm", "PostToolUseFailure": "nohook"}, 1, "fail", inconclusive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := ptruHome(t)
			cfg := filepath.Join(home, ".claude-probe")
			before := filesUnder(t, cfg)
			res, asked, logs := ptruRun(t, home, tc.modes)
			if res.exitCode != tc.exit {
				t.Errorf("exit %d, want %d:\n%s%s", res.exitCode, tc.exit, res.stdout, res.stderr)
			}
			if got := verdictOf(t, logs, "PostToolUse"); got != tc.post {
				t.Errorf("PostToolUse verdict %q, want %q", got, tc.post)
			}
			if got := verdictOf(t, logs, "PostToolUseFailure"); got != tc.failed {
				t.Errorf("PostToolUseFailure verdict %q, want %q", got, tc.failed)
			}
			all := res.stdout + res.stderr
			for _, body := range logs {
				all += body
				assertHas(t, "a log", body, "claude --version: 0.0.0-stub (P-TRU-1)")
			}
			if strings.Contains(all, "PTRU1-CTX-") || strings.Contains(all, "PTRU1-SM-") {
				t.Errorf("a nonce was printed or logged:\n%s", all)
			}
			var runs []string
			for _, a := range asked {
				if i := slices.Index(a, "--allowedTools"); i >= 0 && i+1 < len(a) {
					runs = append(runs, a[i+1])
				}
			}
			if want := []string{"Bash(true)", "Bash(false)"}; !slices.Equal(runs, want) {
				t.Errorf("the sessions allowed %q, want %q: PostToolUse runs true, PostToolUseFailure false", runs, want)
			}
			// The configuration directory, not the home: python3 under
			// /usr/bin on macOS caches bytecode in $HOME/Library.
			if after := filesUnder(t, cfg); !slices.Equal(after, before) {
				t.Errorf("the probe configuration holds %v after the run, want %v", after, before)
			}
		})
	}
}

func TestVG1n_TheProbeRefusesASettingsFile(t *testing.T) {
	home := ptruHome(t)
	settings := filepath.Join(home, ".claude-probe", "settings.json")
	writeFile(t, settings, `{"hooks": {}}`+"\n")
	res, asked, logs := ptruRun(t, home, nil)
	if res.exitCode != 2 || !strings.Contains(res.stderr, "refusing") {
		t.Errorf("exit %d, stderr %q; want 2 and a refusal", res.exitCode, res.stderr)
	}
	if len(asked) != 0 || len(logs) != 0 {
		t.Errorf("claude was asked %v and logs %v were written before the refusal", asked, logs)
	}
	res, asked, _ = ptruRun(t, ptruHome(t), nil, "SessionStart")
	if res.exitCode != 2 || len(asked) != 0 {
		t.Errorf("an event the probe does not drive: exit %d, claude asked %v; want 2 and nothing asked", res.exitCode, asked)
	}
}
