package acceptance

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// The driver's tests (card MEASURE1, PR B): scripts/measure1.sh against the
// suite's binary, with claude, tmux and go stubbed from testdata/measure1.
// The claude stub runs the hook commands `rashomon watch` installed, with
// Claude Code's payload shapes and hook environment, so every record, steer
// line and checkpoint the driver reads is this binary's own. What the
// stubs cannot show, a real tmux server's environment, its default shell and
// paste timing, is G1a's and G1c's to see on the real claude.
//
// G1c's positive control is a second probe per arm on a fixed prompt that
// names go.mod (scripts/measure1_control_prompt.txt), whose task-preflight
// note at UserPromptSubmit this binary delivers in the steer arm and withholds
// in the off arm, whatever the model does. The stub's default run therefore
// passes G1c and goes on through G6; the control_blind knob takes the note
// from one arm to show G1c stopping.

// The task's base: Add subtracts, so the fix's TestAdd fails on its own
// assertion. The go stub passes it when calc.go says FIXED and fails the
// build when it says BROKEN.
const (
	m1Red     = "func Add(a, b int) int { return a - b }"
	m1Green   = "// FIXED\nfunc Add(a, b int) int { return a + b }"
	m1Unbuilt = "// BROKEN\nfunc Add(a, b int) int { return a - b }"
)

// m1Rig is one driver run's machine: stubs, a task repository, and M after
// prepare and the owner's act. Its root is short because the tmux stub binds
// a unix socket under every arm, and a socket path is capped near 104 bytes,
// which t.TempDir() can pass on macOS.
type m1Rig struct {
	t                          *testing.T
	root, stubs, m, task, home string
}

func newM1Rig(t *testing.T, knobs map[string]any, base string) *m1Rig {
	t.Helper()
	return newM1RigWith(t, knobs, func(r *m1Rig) { r.buildTask(base) })
}

// newM1RigWith is newM1Rig with the task built by build.
func newM1RigWith(t *testing.T, knobs map[string]any, build func(*m1Rig)) *m1Rig {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("measure1.sh is a bash script for macOS and Linux")
	}
	for _, tool := range []string{"bash", "python3", "git", "tar", "shasum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("premise: the driver needs %s on PATH: %v", tool, err)
		}
	}
	root, err := os.MkdirTemp("/tmp", "m1-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m1RemoveAll(root) })
	r := &m1Rig{t: t, root: root, stubs: filepath.Join(root, "stubs"), m: filepath.Join(root, "M"),
		task: filepath.Join(root, "task"), home: filepath.Join(root, "home")}
	for _, d := range []string{r.stubs, r.home, filepath.Join(root, "t"), filepath.Join(root, "nopolicy")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for src, dst := range map[string]string{"claude.py": "claude.py", "tmux.py": "tmux.py", "go.py": "go",
		"init-2.1.280.jsonl": "init-2.1.280.jsonl"} {
		b, err := os.ReadFile(filepath.Join("testdata", "measure1", src))
		if err != nil {
			t.Fatal(err)
		}
		r.write(filepath.Join(r.stubs, dst), string(b), 0o755)
	}
	if knobs != nil {
		b, err := json.Marshal(knobs)
		if err != nil {
			t.Fatal(err)
		}
		r.write(filepath.Join(r.stubs, "stub.json"), string(b), 0o600)
	}
	build(r)
	r.prepare()
	return r
}

// m1RemoveAll removes a rig's root; G2 leaves the module cache read-only.
func m1RemoveAll(root string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(root)
}

func (r *m1Rig) write(path, body string, mode os.FileMode) {
	r.t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		r.t.Fatal(err)
	}
}

// env is the driver's own environment: the runner's, less anything Claude
// Code, rashomon or a proxy set (the arms start from env -i in any case),
// with the stubs first on PATH and a home of the rig's own.
func (r *m1Rig) env(extra ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(name)
		switch {
		case strings.HasPrefix(up, "CLAUDE"), strings.HasPrefix(up, "RASHOMON_"), strings.HasPrefix(up, "ANTHROPIC_"),
			strings.HasPrefix(up, "MEASURE1_"), strings.HasSuffix(up, "_PROXY"),
			up == "AI_AGENT", up == "PATH", up == "HOME", up == "TMPDIR":
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "PATH="+r.stubs+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HOME="+r.home, "TMPDIR="+filepath.Join(r.root, "t"),
		"MEASURE1_CLAUDE="+filepath.Join(r.stubs, "claude.py"),
		"MEASURE1_TMUX="+filepath.Join(r.stubs, "tmux.py"),
		"MEASURE1_POLICY_DIR="+filepath.Join(r.root, "nopolicy"))
	return append(out, extra...)
}

// m1Result is one script run: its exit code and its stdout and stderr
// interleaved, as a terminal shows them.
type m1Result struct {
	code int
	out  string
}

func (r *m1Rig) script(extraEnv []string, args ...string) m1Result {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{filepath.Join(moduleRoot, "scripts", "measure1.sh")}, args...)...)
	cmd.Dir = r.root
	cmd.Env = r.env(extraEnv...)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if ctx.Err() != nil {
		r.t.Fatalf("measure1.sh %v: no exit in 5 minutes\n%s", args, buf.String())
	}
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return m1Result{ee.ExitCode(), buf.String()}
	case err != nil:
		r.t.Fatalf("measure1.sh %v: %v", args, err)
	}
	return m1Result{0, buf.String()}
}

// run is the paired run over the rig's task, with the watchdog at limit
// seconds.
func (r *m1Rig) run(limit string, extraEnv ...string) m1Result {
	r.t.Helper()
	return r.script(extraEnv, "run", r.m, "--task", r.task, "--limit", limit)
}

// taskRepo makes the task's repo/ directory and returns its git and commit
// helpers (a commit writes its files, adds everything and returns the id).
func (r *m1Rig) taskRepo() (func(args ...string) string, func(msg string, files map[string]string) string) {
	r.t.Helper()
	repo := filepath.Join(r.task, "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		r.t.Fatal(err)
	}
	git := func(args ...string) string {
		r.t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=measure1",
			"-c", "user.email=measure1@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = r.env("GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			r.t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(msg string, files map[string]string) string {
		r.t.Helper()
		for name, body := range files {
			path := filepath.Join(repo, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				r.t.Fatal(err)
			}
			r.write(path, body, 0o600)
		}
		git("add", "-A")
		git("commit", "-q", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	return git, commit
}

// buildTask makes the task directory: a repository whose base holds base as
// calc.go and whose fix commit makes Add add and adds TestAdd.
func (r *m1Rig) buildTask(base string) {
	r.t.Helper()
	git, commit := r.taskRepo()
	git("init", "-q")
	baseID := commit("base", map[string]string{"go.mod": "module example.invalid/task\n\ngo 1.25\n",
		"calc.go": "package task\n\n" + base + "\n"})
	fixID := commit("fix", map[string]string{"calc.go": "package task\n\n" + m1Green + "\n",
		"calc_test.go": "package task\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"Add\")\n\t}\n}\n"})
	for name, body := range map[string]string{"base": baseID + "\n", "fix": fixID + "\n",
		"issue.txt": "Add subtracts.\n", "FAIL_TO_PASS": ". TestAdd\n", "PASS_TO_PASS": ".\n"} {
		r.write(filepath.Join(r.task, name), body, 0o600)
	}
}

// prepare runs the script's prepare, then the owner's act as a terminal
// would: watch into the template's configuration and store, then consent to
// steering at this binary's disclosure version (the marker is the hook
// path's view of it).
func (r *m1Rig) prepare() {
	r.t.Helper()
	if res := r.script(nil, "prepare", rashomonBin, r.m); res.code != 0 {
		r.t.Fatalf("prepare: exit %d\n%s", res.code, res.out)
	}
	tpl := filepath.Join(r.m, "tpl", "store")
	cmd := exec.Command(filepath.Join(r.m, "bin", "rashomon"), "watch", "--yes")
	cmd.Env = r.env("CLAUDE_CONFIG_DIR="+filepath.Join(r.m, "tpl", "cc"), "RASHOMON_HOME="+tpl)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("the owner's watch: %v\n%s", err, out)
	}
	consent := filepath.Join(tpl, "consent.json")
	b, err := os.ReadFile(consent)
	if err != nil {
		r.t.Fatal(err)
	}
	v := strconv.Itoa(steer.DisclosureVersion)
	raised := strings.Replace(string(b), `"steer_disclosure":0`, `"steer_disclosure":`+v, 1)
	if raised == string(b) {
		r.t.Fatal("premise: watch's consent record does not carry steer_disclosure 0")
	}
	r.write(consent, raised, 0o600)
	r.write(filepath.Join(tpl, "steer_disclosure"), v+"\n", 0o600)
}

// out reads a file of M/out, "" when it is absent.
func (r *m1Rig) out(name string) string {
	b, err := os.ReadFile(filepath.Join(r.m, "out", name))
	if err != nil {
		return ""
	}
	return string(b)
}

// ran reports whether any M/out file matches pattern.
func (r *m1Rig) ran(pattern string) bool {
	m, _ := filepath.Glob(filepath.Join(r.m, "out", pattern))
	return len(m) > 0
}

func (r *m1Rig) want(res m1Result, lines ...string) {
	r.t.Helper()
	for _, l := range lines {
		if !strings.Contains(res.out, l) {
			r.t.Errorf("the driver's output lacks %q", l)
		}
	}
	if r.t.Failed() {
		r.t.Logf("exit %d; the driver's output:\n%s", res.code, res.out)
	}
}

// TestMS1x_DriverStopsWithNoModelChannel: on a tree whose steer arm delivers
// nothing at nudge (the stub's steer arm sees a control prompt that names no
// path), a run passes G0, G2, G1b and G1a and stops at G1c's positive
// control, before any arm runs. Each probe's turn ends on this binary's own
// stop checkpoint, the end the driver reads, and the probes' usage is
// projected over the arms before any arm spends (the stub's one call per
// turn: 10 tokens in, 5 out), from prompt 1's probe alone.
func TestMS1x_DriverStopsWithNoModelChannel(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"control_blind": []string{"steer"}}, m1Red)
	res := r.run("20")
	if res.code == 0 {
		t.Errorf("exit 0, want the stop at G1c")
	}
	r.want(res, "measure1: G0 claude 2.1.280 at ",
		"measure1: oracle FAIL_TO_PASS 0/1 passed, 1 failed on their assertions",
		"measure1: oracle FAIL_TO_PASS 1/1 passed, 0 failed on their assertions",
		"measure1: G1b init events equal in 14 keys",
		"equal but for RASHOMON_STEER_ARM=off and RASHOMON_STEER_ARM=steer",
		"measure1: G1c prompt 1 per probe: 1 request(s), 10 context tokens (0% cache reads), 5 output; "+
			"projected arms (x3 prompts x2 arms): 6 requests, 60 context tokens, 30 output",
		"measure1: G1c: no model channel on this tree: the steer arm delivered no nudge with model text (0 steer lines logged)",
		"measure1: G1c: the positive control failed")
	for _, name := range []string{"g1c-off", "g1c-steer", "g1c-control-off", "g1c-control-steer"} {
		if got := r.out(name + ".turns"); got != "prompt 1: stop\n" {
			t.Errorf("%s's turns read %q, want one turn ended by its stop checkpoint", name, got)
		}
	}
	if r.ran("arm-[0-9]*") {
		t.Errorf("an arm ran past G1c's stop")
	}
}

// TestMS1i_DriverArmsDifferOnlyInTheArm: G1b's init events and G1a's
// hook-side dumps agree but for RASHOMON_STEER_ARM; each dump is the hook's
// own environment, Claude Code's host signals in it, and a variable the
// driver's caller set reaches neither arm (env -i).
func TestMS1i_DriverArmsDifferOnlyInTheArm(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, nil, m1Red)
	res := r.run("20", "M1_SENTINEL=leaked")
	r.want(res, "measure1: G1b init events equal in 14 keys",
		"equal but for RASHOMON_STEER_ARM=off and RASHOMON_STEER_ARM=steer")
	for _, arm := range []string{"off", "steer"} {
		dump := "\n" + r.out("g1a-"+arm+".env")
		for _, l := range []string{"RASHOMON_STEER_ARM=" + arm, "CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDECODE=1",
			"AI_AGENT=claude-code_2-1-280_harness", "GOENV=off", "GIT_CONFIG_NOSYSTEM=1"} {
			if !strings.Contains(dump, "\n"+l+"\n") {
				t.Errorf("the %s arm's hook-side dump lacks the line %s", arm, l)
			}
		}
		if strings.Contains(dump, "M1_SENTINEL") {
			t.Errorf("the caller's M1_SENTINEL reached the %s arm's hook", arm)
		}
	}
}

// TestMS1j_DriverStopsOnAGreenBase: G2 rejects a task whose FAIL_TO_PASS
// tests do not fail on base on their own assertions, a build failure named
// as one, before any session spends a token.
func TestMS1j_DriverStopsOnAGreenBase(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, base, want string }{
		{"green", m1Green, "G2: on base 0 of 1 FAIL_TO_PASS tests fail on their own assertions; the task is rejected"},
		{"unbuilt", m1Unbuilt, "example.invalid/task did not build, so its tests never ran; the task is rejected"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newM1Rig(t, nil, c.base)
			res := r.run("20")
			if res.code == 0 {
				t.Errorf("exit 0, want G2's rejection")
			}
			r.want(res, c.want)
			if r.ran("g1b-*") {
				t.Errorf("a session started before G2 rejected the task")
			}
		})
	}
}

// TestMS1k_DriverStopsOnVersionDrift: G0 refuses a claude that is not the
// pinned version. The binary's sha256, rechecked after the arms, is G3's.
func TestMS1k_DriverStopsOnVersionDrift(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"version": "2.1.281"}, m1Red)
	res := r.run("20")
	if res.code == 0 {
		t.Errorf("exit 0, want G0's refusal")
	}
	r.want(res, "G0: claude is 2.1.281 at ", "not the pinned 2.1.280")
	if r.ran("g1b-*") || r.ran("red.json") {
		t.Errorf("the run went past G0")
	}
}

// TestMS1m_DriverRefusesWithAMessage: the machine facts G0 meets, each a
// one-line refusal before anything starts and never a hang: no tmux, a
// claude path that is not a file, a version that drifted (with the pin to
// set), a pin below the verified floor.
func TestMS1m_DriverRefusesWithAMessage(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"version": "2.1.308"}, m1Red)
	for _, c := range []struct {
		name string
		env  []string
		want string
	}{
		{"no tmux", []string{"MEASURE1_TMUX=/nonexistent/tmux"}, "G0: no tmux on PATH: install it (brew install tmux)"},
		{"no claude", []string{"MEASURE1_CLAUDE=/nonexistent/claude"}, "G0: /nonexistent/claude is not an executable file"},
		{"drift names the pin", nil, "(to run on 2.1.308, set MEASURE1_PIN=2.1.308)"},
		{"pin below the floor", []string{"MEASURE1_PIN=2.1.279"}, "G0: MEASURE1_PIN=2.1.279 is below the verified floor 2.1.280"},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := r.script(c.env, "run", r.m, "--task", r.task, "--limit", "20")
			if res.code == 0 {
				t.Errorf("exit 0, want G0's refusal")
			}
			r.want(res, c.want)
			if r.ran("g1b-*") || r.ran("red.json") {
				t.Errorf("the run went past G0")
			}
		})
	}
}

// TestMS1l_DriverStopsOnAnInputDiff: an init event that differs between the
// arms in anything but the arm's own names stops the run at G1b.
func TestMS1l_DriverStopsOnAnInputDiff(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"extra_tool_for_arm": "steer"}, m1Red)
	res := r.run("20")
	if res.code == 0 {
		t.Errorf("exit 0, want G1b's stop")
	}
	r.want(res, "G1b: the arms' init events differ in tools")
	if r.ran("g1c-*") {
		t.Errorf("a probe session ran past G1b")
	}
}

// TestMS1t_DriverMarksABlockedPrompt: a turn whose call never returns is
// marked blocked by the watchdog, naming the pending call (the hook's own
// terminal is not the call's return), and the pane is captured.
func TestMS1t_DriverMarksABlockedPrompt(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"hang_at": 1, "control_blind": []string{"off", "steer"}}, m1Red)
	r.run("3")
	if got := r.out("g1c-off.turns"); got != "prompt 1: blocked (call pending: permission prompt or long call)\n" {
		t.Errorf("the off probe's turns read %q, want the pending call named", got)
	}
	if !r.ran("g1c-off.pane-1.txt") {
		t.Errorf("no pane capture for the blocked prompt")
	}
}

// TestMS1u_DriverKeepsTranscriptsOnFailure: a client that dies mid-turn is
// named as exited, and its arm's transcript and store are kept in an
// aborted tar named by the arm's label.
func TestMS1u_DriverKeepsTranscriptsOnFailure(t *testing.T) {
	t.Parallel()
	// The stub dies after the prompt's hook has run, so the control note is
	// still logged; blinding both arms stops the run at G1c, where the
	// driver's cleanup keeps the probes.
	r := newM1Rig(t, map[string]any{"die_at": 1, "control_blind": []string{"off", "steer"}}, m1Red)
	r.run("20")
	if got := r.out("g1c-off.turns"); got != "prompt 1: blocked (client exited)\n" {
		t.Errorf("the off probe's turns read %q, want the exit named", got)
	}
	sid := strings.TrimSpace(r.out("g1c-off.session"))
	members := m1TarMembers(t, filepath.Join(r.m, "out", "aborted-g1c-off.tar"))
	for _, suffix := range []string{"/" + sid + ".jsonl", "/runs/" + sid + "/coverage.ndjson"} {
		found := false
		for _, m := range members {
			found = found || strings.HasSuffix(m, suffix)
		}
		if !found {
			t.Errorf("aborted-g1c-off.tar holds no member ending %s; it holds %v", suffix, members)
		}
	}
}

func m1TarMembers(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("the aborted arm's tar: %v", err)
	}
	defer f.Close()
	var names []string
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
}

// TestMS1w_DriverEndsATurnByOffset: a prompt the client dropped is resent
// once and logged as a deviation, then stops as not submitted if dropped
// again; and the turn's end is the first complete stop checkpoint past the
// offset taken before the prompt was sent, an earlier one or a torn one not
// counting.
func TestMS1w_DriverEndsATurnByOffset(t *testing.T) {
	t.Parallel()
	t.Run("offset", m1TurnEndByOffset)
	for _, c := range []struct {
		name  string
		drops int
		want  string
	}{{"resent", 1, "prompt 1: stop\n"}, {"never submitted", 2, "prompt 1: blocked (not submitted)\n"}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newM1Rig(t, map[string]any{"ignore_pastes": c.drops}, m1Red)
			r.run("20")
			if got := r.out("g1c-off.turns"); got != c.want {
				t.Errorf("the off probe's turns read %q, want %q", got, c.want)
			}
			if !strings.Contains(r.out("deviations.log"), "not seen in the transcript after 30 s; sent once more") {
				t.Errorf("no resend deviation logged; the log reads %q", r.out("deviations.log"))
			}
		})
	}
}

func m1TurnEndByOffset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec, tr := filepath.Join(dir, "records.ndjson"), filepath.Join(dir, "t.jsonl")
	old := `{"type":"verify","point":"stop","prompt_id":"p1"}` + "\n"
	if err := os.WriteFile(rec, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tr, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	turnend := func() (int, string) {
		cmd := exec.Command("python3", filepath.Join(moduleRoot, "scripts", "measure1_check.py"), "turnend",
			rec, strconv.Itoa(len(old)), tr, "0")
		out, err := cmd.Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), string(out)
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0, string(out)
	}
	appendTo := func(path, s string) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct {
		name, file, add string
		code            int
		out             string
	}{
		{"the earlier turn's checkpoint", "", "", 1, ""},
		{"a checkpoint at another point", rec, `{"type":"verify","point":"commit","prompt_id":"p2"}` + "\n", 1, ""},
		{"a torn stop checkpoint", rec, `{"type":"verify","point":"stop","prompt_id":"p2"`, 1, ""},
		{"the stop checkpoint completed", rec, "}\n", 0, "stop\n"},
		{"an API error in the turn", tr, `{"type":"assistant","isApiErrorMessage":true}` + "\n", 0, "stop_failure\n"},
	}
	for _, s := range steps {
		if s.file != "" {
			appendTo(s.file, s.add)
		}
		if code, out := turnend(); code != s.code || out != s.out {
			t.Errorf("after %s: exit %d printing %q, want %d printing %q", s.name, code, out, s.code, s.out)
		}
	}
}

// TestMS1y_DriverMarkersAreTheBinarysPrefixes: S2's markers are the
// binary's own: the steer line's prefix, and the prefix of the line recap
// prints, so a renamed prefix fails here rather than blinding the sensor.
func TestMS1y_DriverMarkersAreTheBinarysPrefixes(t *testing.T) {
	out, err := exec.Command("python3", filepath.Join(moduleRoot, "scripts", "measure1_check.py"), "markers").Output()
	if err != nil {
		t.Fatal(err)
	}
	var markers []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var m string
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("marker line %q: %v", l, err)
		}
		markers = append(markers, m)
	}
	if len(markers) != 2 || markers[0] != render.MessagePrefix {
		t.Fatalf("markers %q, want render.MessagePrefix %q and recap's", markers, render.MessagePrefix)
	}
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	if line := e.mustSpeak(stopEvent("Stop", "All 42 tests pass.", e.cwd)); !strings.Contains(line, markers[1]) {
		t.Errorf("recap's line %q does not hold the marker %q", line, markers[1])
	}
}

// TestMS1z_DriverRunsOnClaude2_1_308Shapes: Claude Code 2.1.308 records a
// pasted prompt wrapped in <pasted_content id> and sets hook variables unique
// to each process. A run still passes G1a (the arms differ only in those and
// RASHOMON_STEER_ARM) and confirms each probe's submission, so the turn ends
// on its stop checkpoint rather than as blocked.
func TestMS1z_DriverRunsOnClaude2_1_308Shapes(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"cc308": true}, m1Red)
	res := r.run("20")
	r.want(res, "equal but for RASHOMON_STEER_ARM=off and RASHOMON_STEER_ARM=steer",
		"measure1: G1c prompt 1 per probe: 1 request(s), 10 context tokens")
	for _, arm := range []string{"off", "steer"} {
		if got := r.out("g1c-" + arm + ".turns"); got != "prompt 1: stop\n" {
			t.Errorf("the %s probe's turns read %q, want the wrapped prompt confirmed and the turn ended by its stop", arm, got)
		}
		dump := "\n" + r.out("g1a-"+arm+".env")
		for _, k := range []string{"CLAUDE_CODE_MESSAGING_SOCKET=", "CLAUDE_CODE_MESSAGING_TOKEN=", "CLAUDE_ENV_FILE=", "CLAUDE_PID=", "GOGCCFLAGS="} {
			if !strings.Contains(dump, "\n"+k) {
				t.Errorf("the %s arm's dump lacks %s, so the test does not exercise it", arm, k)
			}
		}
	}
}

// TestMS1za_DriverStopsOnAnEnvDifferenceBeyondTheArm: a hook variable that
// only one arm sees is not per-process, and G1a stops on it.
func TestMS1za_DriverStopsOnAnEnvDifferenceBeyondTheArm(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, map[string]any{"cc308": true, "env_diff_for_arm": "steer"}, m1Red)
	res := r.run("20")
	if res.code == 0 {
		t.Errorf("exit 0, want G1a's stop")
	}
	r.want(res, "G1a: the arms' environments differ beyond RASHOMON_STEER_ARM")
	if r.ran("arm-[0-9]*") {
		t.Errorf("an arm ran past G1a's stop")
	}
}

// m1Check runs measure1_check.py's command directly.
func m1Check(t *testing.T, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("python3", append([]string{"-I", filepath.Join(moduleRoot, "scripts", "measure1_check.py")}, args...)...)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return ee.ExitCode(), string(out)
	case err != nil:
		t.Fatal(err)
	}
	return 0, string(out)
}

// TestMS1zb_SubmittedReadsThePastedWrapper: the submitted check confirms a
// user entry that is the prompt, bare or in exactly one matching
// <pasted_content> wrapper (tabs shown as four spaces), and keeps exit 3 for
// any other text, a wrapper with other text or mismatched ids included.
func TestMS1zb_SubmittedReadsThePastedWrapper(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prompt := "Task: fix Add\n\tgo test ./...\nthen stop.\n"
	pf := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(pf, []byte(prompt), 0o600); err != nil {
		t.Fatal(err)
	}
	wrap := func(open, body, shut string) string {
		return "\n\n<pasted_content id=\"" + open + "\">\n" + body + "\n</pasted_content id=\"" + shut + "\">\n"
	}
	spaced := strings.ReplaceAll(prompt, "\t", "    ")
	cases := []struct {
		name, text string
		want       int
	}{
		{"bare", prompt, 0},
		{"wrapped", wrap("40a4", prompt, "40a4"), 0},
		{"wrapped, tabs as spaces", wrap("40a4", spaced, "40a4"), 0},
		{"wrapped other text", wrap("40a4", "Task: something else\n", "40a4"), 3},
		{"other text", "something else", 3},
		{"ids differ", wrap("40a4", prompt, "40a5"), 3},
		{"no surrounding newlines", strings.TrimSpace(wrap("40a4", prompt, "40a4")), 3},
		{"two wrappers", wrap("1111", wrap("40a4", prompt, "40a4"), "1111"), 3},
		{"text beside the wrapper", "hello" + wrap("40a4", prompt, "40a4"), 3},
	}
	for _, c := range cases {
		entry, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": c.text}})
		tr := filepath.Join(dir, "t.jsonl")
		if err := os.WriteFile(tr, append(entry, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, out := m1Check(t, "submitted", tr, "0", pf); code != c.want {
			t.Errorf("%s: exit %d, want %d (%s)", c.name, code, c.want, out)
		}
	}
}

// TestMS1zc_EnvdiffNormalisesOnlyPerProcessValues: G1a's normaliser folds the
// arm directory, pid, token and go-build suffix of 2.1.308, each on its own
// key; any other difference, or a key on one side only, still fails.
func TestMS1zc_EnvdiffNormalisesOnlyPerProcessValues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	n := 0
	dump := func(arm, rnd, pid, tok, build string, extra ...string) string {
		d := "/var/folders/3p/xx/T/measure1-arm." + rnd
		lines := append([]string{
			"CLAUDE_CODE_MESSAGING_SOCKET=" + d + "/tmp/cc-socks/" + pid + ".sock",
			"CLAUDE_CODE_MESSAGING_TOKEN=" + tok,
			"CLAUDE_CONFIG_DIR=/var/folders/3p/xx/T//measure1-arm." + rnd + "/cc",
			"CLAUDE_ENV_FILE=" + d + "/cc/session-env/SID/sessionstart-hook-1.sh",
			"CLAUDE_PID=" + pid,
			"GOGCCFLAGS='-fPIC -ffile-prefix-map=" + d + "/tmp/go-build" + build + "=/tmp/go-build -fno-common'",
			"RASHOMON_STEER_ARM=" + arm}, extra...)
		n++
		p := filepath.Join(dir, strconv.Itoa(n)+".env")
		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	diff := func(a, b string) int {
		code, _ := m1Check(t, "envdiff", a, "/nowhere/a", "/nowhere/wa", "SID", b, "/nowhere/b", "/nowhere/wb", "SID")
		return code
	}
	off := func(extra ...string) string { return dump("off", "69KtT9", "76626", "5959aa", "1892231564", extra...) }
	if code := diff(off(), dump("steer", "FoXEY1", "76922", "3175bb", "930180087")); code != 0 {
		t.Errorf("per-process differences alone: exit %d, want 0", code)
	}
	fails := map[string]string{
		"a variable on one side only":   dump("steer", "FoXEY1", "76922", "3175bb", "930180087", "FOO=1"),
		"another variable that differs": dump("steer", "FoXEY1", "76922", "3175bb", "930180087", "FOO=2"),
		"a token that is not hex":       dump("steer", "FoXEY1", "76922", "not-hex!", "930180087"),
	}
	for name, b := range fails {
		a := off()
		if name == "another variable that differs" {
			a = off("FOO=1")
		}
		if code := diff(a, b); code == 0 {
			t.Errorf("%s: exit 0, want G1a's failure", name)
		}
	}
}

// TestMS1zd_G1cControlPassesOnTheFixedPrompt: both control probes ran the
// fixed prompt, not the task's prompt 1; the steer arm delivered the note
// with its model text, the off arm logged its arm_off twin, and the run went
// on to the arms. The prompt is in the pre-registration with the other
// inputs, and the cost projection is still prompt 1's.
func TestMS1zd_G1cControlPassesOnTheFixedPrompt(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, nil, m1Red)
	res := r.run("20")
	if res.code != 0 {
		t.Errorf("exit %d, want the run to pass G1c and finish", res.code)
	}
	r.want(res, "measure1: G1c control seen; the steer transcript holds the delivered note",
		"measure1: G1c prompt 1 per probe: 1 request(s)",
		"measure1: arm-1-")
	want, err := os.ReadFile(filepath.Join(moduleRoot, "scripts", "measure1_control_prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(r.m, "control-prompt.txt")); err != nil || string(got) != string(want) {
		t.Errorf("M/control-prompt.txt is %q (%v), want the script's fixed prompt %q", got, err, want)
	}
	var prereg struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal([]byte(r.out("prereg.json")), &prereg); err != nil {
		t.Fatal(err)
	}
	found := false
	for p := range prereg.SHA256 {
		found = found || strings.HasSuffix(p, "/control-prompt.txt")
	}
	if !found {
		t.Errorf("prereg.json's inputs %v do not name control-prompt.txt", prereg.SHA256)
	}
	for _, arm := range []string{"off", "steer"} {
		if got := r.out("g1c-control-" + arm + ".turns"); got != "prompt 1: stop\n" {
			t.Errorf("the %s control probe's turns read %q, want one turn ended by its stop", arm, got)
		}
		if got := r.out("g1c-control-" + arm + ".session"); got == r.out("g1c-"+arm+".session") || got == "" {
			t.Errorf("the %s control probe is not a session of its own (%q)", arm, got)
		}
	}
}

// TestMS1ze_G1cStopsWhenAnArmLacksItsLine: the steer arm delivering nothing,
// or the off arm logging no line, stops the run at G1c before any arm.
func TestMS1ze_G1cStopsWhenAnArmLacksItsLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, blind, want string }{
		{"the steer arm delivers nothing", "steer", "G1c: no model channel on this tree"},
		{"the off arm logs no twin", "off", "G1c: the off arm's probe logged no steer line"},
	} {
		r := newM1Rig(t, map[string]any{"control_blind": []string{c.blind}}, m1Red)
		res := r.run("20")
		if res.code == 0 {
			t.Errorf("%s: exit 0, want the stop at G1c", c.name)
		}
		r.want(res, c.want, "measure1: G1c: the positive control failed")
		if r.ran("arm-[0-9]*") {
			t.Errorf("%s: an arm ran past G1c's stop", c.name)
		}
	}
}

// m1SteerLine is one steer.ndjson line with the fields G1c's control reads.
func m1SteerLine(arm, delivery, by, key, text string) map[string]any {
	l := map[string]any{"type": "steer", "pattern": "task-preflight", "key": key, "level": "nudge", "arm": arm,
		"delivery": delivery, "model_text": text, "surface": "cli", "prompt_id": "p1"}
	if by != "" {
		l["suppressed_by"] = by
	}
	return l
}

// TestMS1zf_ControlReadsTheTwinPair: measure1_check.py control over hand-made
// stores. It passes on a delivered nudge with model text and an arm_off twin
// of the same pattern and key, and fails on each missing half: no delivery,
// no twin, a twin of another key, pattern or withholding reason, a line off
// the cli surface or with no prompt id.
func TestMS1zf_ControlReadsTheTwinPair(t *testing.T) {
	t.Parallel()
	type lines = []map[string]any
	good := func() (lines, lines) {
		return lines{m1SteerLine("steer", "delivered", "", "note", "the note")},
			lines{m1SteerLine("off", "shadow", "arm_off", "note", "")}
	}
	const noTwin = "the off arm has no task-preflight/note line withheld by arm_off at nudge"
	cases := []struct {
		name   string
		mutate func(on, off lines) (lines, lines)
		want   int
		out    string
	}{
		{"both halves", func(on, off lines) (lines, lines) { return on, off }, 0, "the note"},
		{"the steer arm delivers nothing", func(on, off lines) (lines, lines) { return nil, off }, 1, "no model channel on this tree"},
		{"delivered without model text", func(on, off lines) (lines, lines) {
			on[0]["model_text"] = ""
			return on, off
		}, 1, "no model channel on this tree"},
		{"the off arm logs nothing", func(on, off lines) (lines, lines) { return on, nil }, 1, "the off arm's probe logged no steer line"},
		{"the off line is another key", func(on, off lines) (lines, lines) {
			off[0]["key"] = "brief"
			return on, off
		}, 1, noTwin},
		{"the off line is withheld by another reason", func(on, off lines) (lines, lines) {
			off[0]["suppressed_by"] = "unverified_channel"
			return on, off
		}, 1, noTwin},
		{"the off line is another pattern", func(on, off lines) (lines, lines) {
			off[0]["pattern"] = "regression-location"
			return on, off
		}, 1, noTwin},
		{"a line off the cli surface", func(on, off lines) (lines, lines) {
			on[0]["surface"] = "sdk"
			return on, off
		}, 1, "not cli and an id"},
		{"a line with no prompt id", func(on, off lines) (lines, lines) {
			off[0]["prompt_id"] = ""
			return on, off
		}, 1, "not cli and an id"},
		{"the Stop's evidence lines beside the pair", func(on, off lines) (lines, lines) {
			ev := map[string]any{"type": "steer_evidence", "v": 1, "steer_id": "s1", "session_id": "sid",
				"prompt_id": "p1", "event": "UserPromptSubmit", "evidence": "seen"}
			return append(on, ev), append(off, ev)
		}, 0, "the note"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		on, off := c.mutate(good())
		for arm, ls := range map[string]lines{"steer": on, "off": off} {
			if err := os.MkdirAll(filepath.Join(dir, arm, "runs", "sid"), 0o700); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			for _, l := range ls {
				b, _ := json.Marshal(l)
				buf.Write(append(b, '\n'))
			}
			if err := os.WriteFile(filepath.Join(dir, arm, "runs", "sid", "steer.ndjson"), buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		code, out := m1Check(t, "control", filepath.Join(dir, "steer"), "sid", filepath.Join(dir, "off"), "sid")
		if code != c.want || !strings.Contains(out, c.out) {
			t.Errorf("%s: exit %d, output %q; want exit %d holding %q", c.name, code, out, c.want, c.out)
		}
	}
}

// TestMS1zg_RepsReportEveryPair: --reps K runs K seeded pairs of sessions and
// reports each pair, not only the last (c10: SID_<arm> was overwritten in the
// report's loop, so a 3-rep run printed one `measure pair`). Each rep's
// sensors run on its own two sessions, its pair is written to pair-N.txt and
// pair-N.json holding that rep's two session ids and no other, and pairs.txt
// lists every rep. A single rep keeps pair.txt and pair.json as before.
func TestMS1zg_RepsReportEveryPair(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, nil, m1Red)
	res := r.script(nil, "run", r.m, "--task", r.task, "--limit", "20", "--reps", "2")
	if res.code != 0 {
		t.Errorf("exit %d, want the two-rep run to finish", res.code)
	}
	sid := map[string]string{}
	for n := 1; n <= 4; n++ {
		for _, arm := range []string{"off", "steer"} {
			if s := strings.TrimSpace(r.out("arm-" + strconv.Itoa(n) + "-" + arm + ".session")); s != "" {
				sid[strconv.Itoa(n)+arm] = s
			}
		}
	}
	if len(sid) != 4 {
		t.Fatalf("premise: two reps should leave four arm sessions, found %v", sid)
	}
	// the seeded order puts each rep's two arms side by side: arms 1,2 then 3,4
	repSIDs := func(rep int) []string {
		var out []string
		for k, v := range sid {
			if n, _ := strconv.Atoi(k[:1]); (n+1)/2 == rep {
				out = append(out, v)
			}
		}
		return out
	}
	for rep := 1; rep <= 2; rep++ {
		name := "pair-" + strconv.Itoa(rep)
		raw := r.out(name + ".json")
		if !json.Valid([]byte(raw)) {
			t.Errorf("%s.json is not valid JSON (absent or damaged): %q", name, raw)
			continue
		}
		if r.out(name+".txt") == "" {
			t.Errorf("%s.txt is empty or absent", name)
		}
		for _, s := range repSIDs(rep) {
			if !strings.Contains(raw, s) {
				t.Errorf("%s.json does not hold rep %d's session %s", name, rep, s)
			}
		}
		for _, s := range repSIDs(3 - rep) {
			if strings.Contains(raw, s) {
				t.Errorf("%s.json holds the other rep's session %s", name, s)
			}
		}
		if !strings.Contains(r.out("pairs.txt"), "rep "+strconv.Itoa(rep)+": ") {
			t.Errorf("pairs.txt does not list rep %d: %q", rep, r.out("pairs.txt"))
		}
	}
	for _, l := range []string{"measure1: S1 off arm: ", "measure1: S1 steer arm: "} {
		if n := strings.Count(res.out, l); n != 2 {
			t.Errorf("the driver's output holds %q %d times, want once per rep (2)", l, n)
		}
	}
	if r.out("pair.json") != "" {
		t.Errorf("pair.json exists for a two-rep run; only a single rep keeps that name")
	}
}

// TestMS1zh_OneRepKeepsThePairFiles: a single rep still writes pair.txt and
// pair.json, beside pair-1.txt and pair-1.json and the one-line pairs.txt.
func TestMS1zh_OneRepKeepsThePairFiles(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, nil, m1Red)
	res := r.run("20")
	if res.code != 0 {
		t.Fatalf("exit %d\n%s", res.code, res.out)
	}
	if !json.Valid([]byte(r.out("pair.json"))) || r.out("pair.txt") == "" {
		t.Errorf("a one-rep run lacks pair.json or pair.txt")
	}
	if r.out("pair.json") != r.out("pair-1.json") || r.out("pair.txt") != r.out("pair-1.txt") {
		t.Errorf("pair.* differ from pair-1.*")
	}
	if !strings.Contains(r.out("pairs.txt"), "rep 1: ") || strings.Contains(r.out("pairs.txt"), "rep 2: ") {
		t.Errorf("pairs.txt reads %q, want rep 1 alone", r.out("pairs.txt"))
	}
}

// A pytest task (card c9): the task directory's RUNNER file says pytest, its
// FAIL_TO_PASS and PASS_TO_PASS lines are node ids, and the oracle is
// pytest --junitxml read into the shape go test -json gives check's oracle.
// The tests write the Python project themselves, in the rig's temp directory.
const (
	m1PyBroken = "def add(a, b):\n    return a - b\n\n\ndef sub(a, b):\n    return a - b\n"
	m1PyFixed  = "def add(a, b):\n    return a + b\n\n\ndef sub(a, b):\n    return a - b\n"
	m1PyUnimp  = "def add(a, b)\n    return a - b\n"
)

// m1NeedPytest skips the test unless python3 on PATH has pytest, saying
// what is missing.
func m1NeedPytest(t *testing.T) {
	t.Helper()
	out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput()
	if err != nil {
		t.Skipf("pytest is not installed for the python3 on PATH (python3 -m pytest --version: %v: %s); the pytest runner cannot be exercised", err, strings.TrimSpace(string(out)))
	}
}

// buildPyTask makes a pytest task: at base calc.py's add subtracts and
// tests/test_calc.py holds one passing test, test_zero; tests/test_other.py
// (not importing calc) passes. The fix repairs add, extends test_calc.py
// with test_add (which needs a fixture from the fix's own conftest.py) and
// so the existing test_zero is the PASS_TO_PASS that stays green.
func (r *m1Rig) buildPyTask(calc string) {
	r.t.Helper()
	git, commit := r.taskRepo()
	git("init", "-q")
	baseID := commit("base", map[string]string{
		"calc.py":             "" + calc,
		"tests/test_calc.py":  "from calc import add\n\n\ndef test_zero():\n    assert add(0, 0) == 0\n",
		"tests/test_other.py": "def test_sub():\n    assert 3 - 1 == 2\n",
	})
	fixID := commit("fix", map[string]string{
		"calc.py":           m1PyFixed,
		"tests/conftest.py": "import pytest\n\n\n@pytest.fixture\ndef pair():\n    return (1, 2)\n",
		"tests/test_calc.py": "from calc import add\n\n\ndef test_zero():\n    assert add(0, 0) == 0\n\n\n" +
			"def test_add(pair):\n    assert add(*pair) == 3\n",
	})
	for name, body := range map[string]string{"base": baseID + "\n", "fix": fixID + "\n", "RUNNER": "pytest\n",
		"issue.txt": "add subtracts.\n", "FAIL_TO_PASS": "tests/test_calc.py::test_add\n",
		"PASS_TO_PASS": "tests/test_calc.py::test_zero\ntests/test_other.py::test_sub\n"} {
		r.write(filepath.Join(r.task, name), body, 0o600)
	}
}

func (r *m1Rig) g2(task string) m1Result {
	r.t.Helper()
	return r.script(nil, "task", r.m, "--task", task)
}

// TestMS1zi_PytestTaskHoldsAtG2: a Python task whose fix repairs add passes
// G2 on pytest's junit result; the run's output names the python3 and pytest
// it used, and the oracle files are in go test -json's event shape.
func TestMS1zi_PytestTaskHoldsAtG2(t *testing.T) {
	t.Parallel()
	m1NeedPytest(t)
	r := newM1RigWith(t, nil, func(r *m1Rig) { r.buildPyTask(m1PyBroken) })
	res := r.g2(r.task)
	if res.code != 0 {
		t.Errorf("exit %d, want G2 to hold", res.code)
	}
	r.want(res, "oracle FAIL_TO_PASS 0/1 passed, 1 failed on their assertions; PASS_TO_PASS 0 failing apart from them; 0 package(s) did not build",
		"oracle FAIL_TO_PASS 1/1 passed, 0 failed on their assertions; PASS_TO_PASS 0 failing apart from them; 0 package(s) did not build",
		"G2 runner pytest: python ", "pytest ", "G2 the task holds")
	// The red run saw the fix's conftest.py (test_add's fixture): its test
	// failed on its assertion rather than erroring for a missing fixture.
	red := r.out("red.json")
	for _, want := range []string{`"Action": "fail"`, `"Test": "test_add"`, `"Package": "tests/test_calc.py"`} {
		if !strings.Contains(strings.ReplaceAll(red, `":"`, `": "`), want) {
			t.Errorf("red.json lacks %s:\n%s", want, red)
		}
	}
}

// TestMS1zj_PytestTaskRefusals: a base whose module does not import is
// refused as one that did not collect, not as tests that failed; a base
// already green is refused as such; a RUNNER the driver does not know is
// refused before anything runs.
func TestMS1zj_PytestTaskRefusals(t *testing.T) {
	t.Parallel()
	m1NeedPytest(t)
	t.Run("base does not import", func(t *testing.T) {
		t.Parallel()
		r := newM1RigWith(t, nil, func(r *m1Rig) { r.buildPyTask(m1PyUnimp) })
		res := r.g2(r.task)
		if res.code == 0 {
			t.Errorf("exit 0, want G2's rejection")
		}
		r.want(res, "G2: on base 0 of 1 FAIL_TO_PASS tests fail on their own assertions; tests/test_calc.py did not build, so its tests never ran; the task is rejected")
		if strings.Contains(res.out, "G2 the task holds") {
			t.Errorf("the task was accepted")
		}
	})
	t.Run("base already green", func(t *testing.T) {
		t.Parallel()
		r := newM1RigWith(t, nil, func(r *m1Rig) { r.buildPyTask(m1PyFixed) })
		res := r.g2(r.task)
		if res.code == 0 {
			t.Errorf("exit 0, want G2's rejection")
		}
		r.want(res, "G2: on base 0 of 1 FAIL_TO_PASS tests fail on their own assertions; the task is rejected")
		if strings.Contains(res.out, "did not build") && !strings.Contains(res.out, "0 package(s) did not build") {
			t.Errorf("a green base was reported as not collecting:\n%s", res.out)
		}
	})
	t.Run("unknown runner", func(t *testing.T) {
		t.Parallel()
		r := newM1RigWith(t, nil, func(r *m1Rig) { r.buildPyTask(m1PyBroken) })
		r.write(filepath.Join(r.task, "RUNNER"), "unittest\n", 0o600)
		res := r.g2(r.task)
		if res.code == 0 {
			t.Errorf("exit 0, want a refusal")
		}
		r.want(res, "RUNNER is unittest: go or pytest")
	})
}

// TestMS1zk_PyOracleMapsJunitToEvents: the junit-to-event mapping on junit
// results of pytest's shapes, with no pytest needed: a failure is the test's
// fail, a collection error is its file's failed build, a skip is a skip, a
// test's own error (setup) is the file's failed build as well, and class and
// parametrised ids keep their node-id tails.
func TestMS1zk_PyOracleMapsJunitToEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, f := range []string{"tests/test_a.py", "tests/test_b.py", "tests/test_c.py"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(f)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	xml := `<?xml version="1.0" encoding="utf-8"?><testsuites><testsuite name="pytest" errors="2" failures="1" skipped="1" tests="6">
<testcase classname="tests.test_a" name="test_ok" time="0.0"/>
<testcase classname="tests.test_a.TestK" name="test_bad[x y]" time="0.0"><failure message="assert 1 == 2">trace</failure></testcase>
<testcase classname="tests.test_a" name="test_sk" time="0.0"><skipped type="pytest.skip" message="s"/></testcase>
<testcase classname="" name="tests.test_b" time="0.0"><error message="collection failure">ImportError</error></testcase>
<testcase classname="tests.test_c" name="test_fx" time="0.0"><error message="failed on setup with fixture not found">E</error></testcase>
</testsuite></testsuites>`
	if err := os.WriteFile(filepath.Join(dir, "r.xml"), []byte(xml), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out := m1Check(t, "pyoracle", filepath.Join(dir, "r.xml"), dir)
	if code != 0 {
		t.Fatalf("pyoracle exit %d: %s", code, out)
	}
	type ev struct {
		Action, Package, Test string
		FailedBuild           bool
	}
	var got []ev
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		var e ev
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("not an event line %q: %v", l, err)
		}
		got = append(got, e)
	}
	want := []ev{
		{"pass", "tests/test_a.py", "test_ok", false},
		{"fail", "tests/test_a.py", "TestK::test_bad[x_y]", false},
		{"skip", "tests/test_a.py", "test_sk", false},
		{"fail", "tests/test_b.py", "", true},
		{"fail", "tests/test_c.py", "", true},
	}
	if len(got) != len(want) {
		t.Fatalf("events %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	// the F2P/P2P spec for check: node ids as "<file> <tail>" and files
	for name, body := range map[string]string{"f2p": "tests/test_a.py::TestK::test_bad[x y]\n", "p2p": "tests/test_a.py::test_ok\ntests/test_b.py\ntests/test_a.py::test_sk\n"} {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for kind, want := range map[string]string{"f2p": "tests/test_a.py TestK::test_bad[x_y]\n", "p2p": "tests/test_a.py\ntests/test_b.py\n"} {
		if code, out := m1Check(t, "pyspec", kind, filepath.Join(dir, kind+".txt")); code != 0 || out != want {
			t.Errorf("pyspec %s: exit %d, %q, want %q", kind, code, out, want)
		}
	}
	if code, _ := m1Check(t, "pyspec", "f2p", filepath.Join(dir, "p2p.txt")); code == 0 {
		t.Errorf("pyspec f2p accepted a file that is not a node id")
	}
}
