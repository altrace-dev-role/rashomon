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
// keeps no nonce in what it prints or logs, names in each log the
// configuration whose login answered, refuses a configuration whose settings
// file declares hooks before claude runs, runs every session with
// --setting-sources project so no other settings file is read, and leaves the
// configuration directory as it found it.
//
// SessionStart is judged per source (startup, resume, compact, clear), each on
// its own nonce pair in its own log, since Claude Code delivers the two halves
// differently by source on one version and steer's floor is keyed by event and
// source.
//
// Break: the verdict reading one half only, an attachment of another event, or
// a hook that never ran as a failure; the exit code losing a failure or
// letting an inconclusive event outrank one; the refusal, the probe's own
// configuration directory, the command each event runs, or the transcript's
// removal dropped; one verdict for SessionStart's four sources.

// ptruStub is the stand-in claude. $PTRU_STUB_<event> picks what it records
// for the session of that event, and $PTRU_STUB_SessionStart_<source> for one
// SessionStart source: pass (both halves), noctx, nosm, otherevent (the
// additionalContext attachment under another event) or nohook (the hook never
// runs, as with no login). SessionStart's calls run as Claude Code's do:
// --session-id starts a session (startup); --resume of it fires resume, then
// compact for /compact or clear for /clear, whose attachments land in a new
// session's transcript.
const ptruStub = `#!/usr/bin/env python3
import json, os, subprocess, sys, uuid
args = sys.argv[1:]
with open(os.environ["PTRU_STUB_LOG"], "a") as f:
    f.write(json.dumps(args) + "\n")
if args[:1] == ["--version"]:
    print("0.0.0-stub (P-TRU-1)")
    sys.exit(0)
def flag(name):
    return args[args.index(name) + 1] if name in args else None
with open(flag("--settings")) as f:
    (event, groups), = json.load(f)["hooks"].items()
cfg = os.environ.get("CLAUDE_CONFIG_DIR") or os.path.join(os.environ["HOME"], ".claude")
proj = os.path.join(cfg, "projects", os.getcwd().replace("/", "-"))
os.makedirs(proj, exist_ok=True)
stream = []
def session(sid):
    os.makedirs(os.path.join(cfg, "session-env", sid), exist_ok=True)
    t = os.path.join(proj, sid + ".jsonl")
    open(t, "a").close()
    return t
def fire(group, mode, transcript):
    if mode == "nohook":
        return None
    out = json.loads(subprocess.run(group["hooks"][0]["command"], shell=True, input=b"{}", capture_output=True).stdout)
    ctx, sm = out["hookSpecificOutput"]["additionalContext"], out["systemMessage"]
    recs = []
    if mode != "noctx":
        on = "PostToolUse" if mode == "otherevent" else event
        recs.append({"type": "attachment", "attachment": {"type": "hook_additional_context", "content": [ctx], "hookEvent": on}})
    recs.append({"type": "attachment", "attachment": {"type": "hook_system_message", "content": sm, "hookEvent": event}})
    with open(transcript, "a") as f:
        for r in recs:
            f.write(json.dumps(r) + "\n")
    if mode != "nosm":
        stream.append({"type": "system", "subtype": "informational", "content": sm})
    return ctx
if event == "SessionStart":
    by = {g["matcher"]: g for g in groups}
    if flag("--session-id"):
        sid, fires = flag("--session-id"), ["startup"]
    else:
        sid, fires = flag("--resume"), ["resume"] + [c[1:] for c in ("/compact", "/clear") if c in args]
    t, said = session(sid), []
    for src in fires:
        if src == "clear":
            t = session(str(uuid.uuid4()))
        said.append(fire(by[src], os.environ.get("PTRU_STUB_SessionStart_" + src, "pass"), t))
else:
    said = [fire(groups[0], os.environ.get("PTRU_STUB_" + event, "pass"), session(flag("--session-id")))]
if said[0] is not None:
    stream.append({"type": "result", "subtype": "success", "result": " ".join(c for c in said if c)})
else:
    stream.append({"type": "result", "subtype": "success", "is_error": True, "result": "Not logged in"})
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
				assertHas(t, "a log", body, "config dir: ~/.claude-probe")
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

// TestVG1n_TheProbeRefusesASettingsFileWithHooks: a settings file in the
// probe's configuration that declares hooks, or that cannot be read, is
// refused before claude runs, so a user-level hook can never answer for the
// probe's; one that declares none (a model, a theme) is the configuration's
// own and is never read, as every session runs with --setting-sources
// project, and the run leaves its bytes as they were. An event the probe
// does not drive is refused too. Break: drop the refusal, refuse every
// settings file, or drop --setting-sources from a call, and this fails.
func TestVG1n_TheProbeRefusesASettingsFileWithHooks(t *testing.T) {
	for _, body := range []string{`{"hooks": {}}` + "\n", `{"model": "opus", "hooks": {"SessionStart": []}}` + "\n", "{\n"} {
		home := ptruHome(t)
		writeFile(t, filepath.Join(home, ".claude-probe", "settings.json"), body)
		res, asked, logs := ptruRun(t, home, nil)
		if res.exitCode != 2 || !strings.Contains(res.stderr, "refusing") {
			t.Errorf("settings %q: exit %d, stderr %q; want 2 and a refusal", body, res.exitCode, res.stderr)
		}
		if len(asked) != 0 || len(logs) != 0 {
			t.Errorf("settings %q: claude was asked %v and logs %v were written before the refusal", body, asked, logs)
		}
	}
	home := ptruHome(t)
	settings := filepath.Join(home, ".claude-probe", "settings.json")
	const plain = `{"model": "opus", "theme": "dark"}` + "\n"
	writeFile(t, settings, plain)
	res, asked, _ := ptruRun(t, home, nil, "PostToolUse")
	if res.exitCode != 0 {
		t.Errorf("a settings file with no hooks: exit %d, want 0:\n%s%s", res.exitCode, res.stdout, res.stderr)
	}
	sessions := 0
	for _, a := range asked {
		if !slices.Contains(a, "-p") {
			continue
		}
		sessions++
		if i := slices.Index(a, "--setting-sources"); i < 0 || i+1 >= len(a) || a[i+1] != "project" {
			t.Errorf("a session ran without --setting-sources project: %q", a)
		}
	}
	if sessions != 1 {
		t.Errorf("claude ran %d sessions, want 1", sessions)
	}
	if b, err := os.ReadFile(settings); err != nil || string(b) != plain {
		t.Errorf("the settings file holds %q after the run (%v), want it as it was", b, err)
	}
	res, asked, _ = ptruRun(t, ptruHome(t), nil, "Stop")
	if res.exitCode != 2 || len(asked) != 0 {
		t.Errorf("an event the probe does not drive: exit %d, claude asked %v; want 2 and nothing asked", res.exitCode, asked)
	}
}

// TestVG1n_SessionStartIsJudgedPerSource: SessionStart runs as one session
// through its four sources, startup with a new id and then resume, /compact
// and /clear each on --resume of that id, and each source is judged on its
// own nonce pair in its own log, so a pass at one source never stands for
// another. /clear's attachments land in the new session's transcript, which
// the verdict reads and the cleanup removes with its session-env directory.
// Break: one nonce pair or one verdict for the event, the clear judged on the
// old transcript, a source run without --setting-sources project, or the new
// session left in the configuration, and this fails.
func TestVG1n_SessionStartIsJudgedPerSource(t *testing.T) {
	const inconclusive = "inconclusive: no login, or the model did not run the command"
	sources := []string{"startup", "resume", "compact", "clear"}
	every := func(v string) map[string]string {
		return map[string]string{"startup": v, "resume": v, "compact": v, "clear": v}
	}
	for _, tc := range []struct {
		name  string
		modes map[string]string
		exit  int
		want  map[string]string
	}{
		{"every source lands both halves", nil, 0, every("pass")},
		{"the stream half at compact alone, as 2.1.280 and 2.1.316 give it",
			map[string]string{"SessionStart_startup": "nosm", "SessionStart_resume": "nosm", "SessionStart_clear": "nosm"}, 1,
			map[string]string{"startup": "fail", "resume": "fail", "compact": "pass", "clear": "fail"}},
		{"compact's attachment under another event", map[string]string{"SessionStart_compact": "otherevent"}, 1,
			map[string]string{"startup": "pass", "resume": "pass", "compact": "fail", "clear": "pass"}},
		{"no additionalContext at clear", map[string]string{"SessionStart_clear": "noctx"}, 1,
			map[string]string{"startup": "pass", "resume": "pass", "compact": "pass", "clear": "fail"}},
		{"no login", map[string]string{"SessionStart_startup": "nohook", "SessionStart_resume": "nohook",
			"SessionStart_compact": "nohook", "SessionStart_clear": "nohook"}, 3, every(inconclusive)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := ptruHome(t)
			cfg := filepath.Join(home, ".claude-probe")
			before := filesUnder(t, cfg)
			res, asked, logs := ptruRun(t, home, tc.modes, "SessionStart")
			if res.exitCode != tc.exit {
				t.Errorf("exit %d, want %d:\n%s%s", res.exitCode, tc.exit, res.stdout, res.stderr)
			}
			for _, src := range sources {
				if got := verdictOf(t, logs, "SessionStart-"+src); got != tc.want[src] {
					t.Errorf("SessionStart %s verdict %q, want %q", src, got, tc.want[src])
				}
			}
			if len(logs) != len(sources) {
				t.Errorf("%d logs, want one per source", len(logs))
			}
			all := res.stdout + res.stderr
			for _, body := range logs {
				all += body
				assertHas(t, "a log", body, "claude --version: 0.0.0-stub (P-TRU-1)")
				assertHas(t, "a log", body, "config dir: ~/.claude-probe")
			}
			if strings.Contains(all, "PTRU1-CTX-") || strings.Contains(all, "PTRU1-SM-") {
				t.Errorf("a nonce was printed or logged:\n%s", all)
			}
			var calls [][]string
			for _, a := range asked {
				if slices.Contains(a, "-p") {
					calls = append(calls, a)
				}
			}
			if len(calls) != len(sources) {
				t.Fatalf("claude ran %d sessions, want %d: %q", len(calls), len(sources), calls)
			}
			sid := ""
			if i := slices.Index(calls[0], "--session-id"); i >= 0 && i+1 < len(calls[0]) && !slices.Contains(calls[0], "--resume") {
				sid = calls[0][i+1]
			} else {
				t.Errorf("the first session %q does not start a new id", calls[0])
			}
			for n, want := range []string{"", "/compact", "/clear"} {
				a := calls[n+1]
				if i := slices.Index(a, "--resume"); i < 0 || i+1 >= len(a) || a[i+1] != sid {
					t.Errorf("session %d %q does not resume %q", n+2, a, sid)
				}
				for _, c := range []string{"/compact", "/clear"} {
					if slices.Contains(a, c) != (c == want) {
						t.Errorf("session %d %q: %s present %v, want %v", n+2, a, c, slices.Contains(a, c), c == want)
					}
				}
			}
			for _, a := range calls {
				if i := slices.Index(a, "--setting-sources"); i < 0 || i+1 >= len(a) || a[i+1] != "project" {
					t.Errorf("a session ran without --setting-sources project: %q", a)
				}
			}
			if after := filesUnder(t, cfg); !slices.Equal(after, before) {
				t.Errorf("the probe configuration holds %v after the run, want %v", after, before)
			}
		})
	}
}
