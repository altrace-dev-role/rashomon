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
// On this tree no note reaches the model at nudge, so a run stops at G1c;
// these tests hold everything up to that stop. G3 to G6 need a tree whose
// steer arm delivers one.

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
	r.buildTask(base)
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

// buildTask makes the task directory: a repository whose base holds base as
// calc.go and whose fix commit makes Add add and adds TestAdd.
func (r *m1Rig) buildTask(base string) {
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
			r.write(filepath.Join(repo, name), body, 0o600)
		}
		git("add", "-A")
		git("commit", "-q", "-m", msg)
		return git("rev-parse", "HEAD")
	}
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
// nothing at nudge, a run passes G0, G2, G1b and G1a and stops at G1c's
// positive control, before any arm runs. Each probe's turn ends on this
// binary's own stop checkpoint, the end the driver reads, and the probes'
// usage is projected over the arms before any arm spends (the stub's one
// call per turn: 10 tokens in, 5 out).
func TestMS1x_DriverStopsWithNoModelChannel(t *testing.T) {
	t.Parallel()
	r := newM1Rig(t, nil, m1Red)
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
		"measure1: G1c: no model channel on this tree",
		"measure1: G1c: the positive control failed")
	for _, arm := range []string{"off", "steer"} {
		if got := r.out("g1c-" + arm + ".turns"); got != "prompt 1: stop\n" {
			t.Errorf("the %s probe's turns read %q, want one turn ended by its stop checkpoint", arm, got)
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
	r := newM1Rig(t, map[string]any{"hang_at": 1}, m1Red)
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
	r := newM1Rig(t, map[string]any{"die_at": 1}, m1Red)
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
