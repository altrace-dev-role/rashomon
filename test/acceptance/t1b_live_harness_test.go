package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// T1B -- the AutoSteer live harness (test/live/autosteer) presents every hook
// as a terminal session's, refuses to judge without its tools, and keeps the
// judged T1 slice frozen.
//
// The T1 pilot (2026-11-05, nine runs) found three defects in the harness and
// none in the product; the execution record holds them as B448 to B450.
//   B448  Claude Code 2.1.280 sets CLAUDE_CODE_ENTRYPOINT=sdk-cli for a hook
//         command of a `claude -p` session. hook.sh exported cli only when the
//         variable was unset, so every R and G hook ran as an SDK session's,
//         whose notes the product suppresses by design (no person at the
//         prompt: SuppressedByNoMirror), and no additionalContext reached any
//         agent. The harness stands in for a terminal session, so it exports
//         cli always, after recording what Claude Code set.
//   B449  the batch driver's PATH held no go: a3's build, a6's vet and tests
//         and a7's go test exited 127 on all nine runs, and the rows read them
//         as unparsed and red. The T1b judge writes no row without its tools.
//   B450  a6 judged gofmt by its output alone, so a missing gofmt printed
//         "gofmt pass". The T1b a6 reads the exit status too.
// The T1 slice's files are never edited after a run (README.txt); a change
// opens a new slice, which T1b is. Their digests at 158c7a29 are held here.
//
// Break: hook.sh passing sdk-cli through, or hook-env.txt losing what Claude
// Code set; the T1b judge writing a row with go missing; the T1b a6 reading a
// missing gofmt as a pass; any T1 slice file moving.

// t1bStub stands in for the arm's binary: it prints the identity it was
// handed and its arguments.
const t1bStub = `#!/bin/sh
printf 'entrypoint=%s agent=%s args=%s\n' "${CLAUDE_CODE_ENTRYPOINT:-unset}" "${AI_AGENT:-unset}" "$*"
`

func autosteerDir() string { return filepath.Join(moduleRoot, "test", "live", "autosteer") }

// toolPath is a PATH of one directory holding links to the tools the harness
// scripts call, and nothing else: no go and no gofmt, wherever the machine
// keeps them. A tool the machine lacks is left out.
func toolPath(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"sh", "python3", "perl", "dirname", "basename", "mkdir", "cat", "tr", "head", "tail",
		"grep", "sed", "sort", "wc", "date", "env", "cp", "rm", "mktemp", "git", "awk", "cut", "tee", "find", "tar", "uuidgen"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		if err := os.Symlink(p, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

// runSh runs sh script args... in dir under environ, a whole environment.
func runSh(t *testing.T, dir string, environ []string, script string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.Env = environ
	var out, errs strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil && cmd.ProcessState == nil {
		t.Fatalf("starting sh %s: %v", script, err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), errs.String()
}

func TestT1B_HookExportsTheTerminalEntrypoint(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.sh")
	writeFile(t, stub, t1bStub) // h115's helper
	if err := os.Chmod(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(dir, "run")
	if err := os.Mkdir(run, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(autosteerDir(), "hook.sh")
	session := []string{"CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=sdk-cli", "AI_AGENT=claude-code_2-1-280_harness", "RK_BIN=" + stub}
	code, out, errs := runSh(t, dir, append(append(os.Environ(), session...), "RK_RUN_DIR="+run), hook, "recap", "pending", "--install", "x")
	if code != 0 {
		t.Fatalf("hook.sh exit %d: %s", code, errs)
	}
	if !strings.Contains(out, "entrypoint=cli ") {
		t.Errorf("the binary saw %q; want CLAUDE_CODE_ENTRYPOINT=cli under an sdk-cli session (B448)", strings.TrimSpace(out))
	}
	if !strings.Contains(out, "agent=claude-code_2-1-280_harness ") {
		t.Errorf("the binary saw %q; want the AI_AGENT Claude Code set", strings.TrimSpace(out))
	}
	env, err := os.ReadFile(filepath.Join(run, "hook-env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "CLAUDE_CODE_ENTRYPOINT=sdk-cli\n") {
		t.Errorf("hook-env.txt = %q; want the value Claude Code set, recorded before the override", env)
	}
	log, err := os.ReadFile(filepath.Join(run, "hooks.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "recap pending --install x rc=0") || !strings.Contains(string(log), "entrypoint=cli ") {
		t.Errorf("hooks.log = %q; want the verb header and the stdout", log)
	}
	// Without RK_RUN_DIR the hook execs the binary; the override holds there too.
	code, out, _ = runSh(t, dir, append(os.Environ(), session...), hook, "post")
	if code != 0 || !strings.Contains(out, "entrypoint=cli ") {
		t.Errorf("exec path: exit %d, %q; want cli", code, strings.TrimSpace(out))
	}
}

func TestT1B_JudgeWritesNoRowWithoutGo(t *testing.T) {
	dir := t.TempDir()
	run := filepath.Join(dir, "t1b-x")
	rows := filepath.Join(dir, "rows")
	writeFile(t, filepath.Join(run, "arm"), "R\n")
	writeFile(t, filepath.Join(run, "tree", "a.go"), "package a\n")
	environ := []string{"PATH=" + toolPath(t), "HOME=" + dir, "RK_RUNS=" + rows}
	code, out, errs := runSh(t, dir, environ, filepath.Join(autosteerDir(), "t1b", "judge", "judge.sh"), run)
	if code != 3 || !strings.Contains(errs, "go") {
		t.Errorf("judge.sh exit %d, stdout %q, stderr %q; want exit 3 naming go (B449)", code, out, errs)
	}
	for _, p := range []string{filepath.Join(rows, "t1b-rows.txt"), filepath.Join(rows, "t1-rows.txt"), filepath.Join(run, "judge", "row.txt")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s was written with go missing", p)
		}
	}
}

func TestT1B_A6ReadsAMissingGofmtAsAFailure(t *testing.T) {
	dir := t.TempDir()
	run := filepath.Join(dir, "t1b-x")
	writeFile(t, filepath.Join(run, "tree", "a.go"), "package a\n")
	writeFile(t, filepath.Join(run, "status.txt"), " M a.go\n")
	writeFile(t, filepath.Join(run, "new-files.txt"), "")
	environ := []string{"PATH=" + toolPath(t), "HOME=" + dir}
	_, out, errs := runSh(t, dir, environ, filepath.Join(autosteerDir(), "t1b", "judge", "a6.sh"), run)
	b, err := os.ReadFile(filepath.Join(run, "judge", "a6.json"))
	if err != nil {
		t.Fatalf("no a6.json: %v; stdout %q, stderr %q", err, out, errs)
	}
	var got struct {
		Checks map[string]bool `json:"checks"`
		A6     bool            `json:"a6"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if v, ok := got.Checks["gofmt"]; !ok || v {
		t.Errorf("gofmt check = %v (present %v); want false with gofmt missing (B450)", v, ok)
	}
	if got.A6 {
		t.Errorf("a6 = true with gofmt missing")
	}
}

func TestT1B_TheT1SliceIsFrozen(t *testing.T) {
	frozen := map[string]string{
		"judge/common.py":       "5e05aa65cd6501a127302e9eda661a5baaac3f0577a00404469f47b0b92f1554",
		"t1/consumers.txt":      "9f89eb6e727de983cff7a3dd8920ce0a181557c1e973370f346d87f8ee533c76",
		"t1/graph.json":         "f5681815397cf20e9cf297b584977aaed75b47d40d0f284df66743d2743eccba",
		"t1/judge/a1.py":        "6824b3bf806bd8e026e9d1c950773e740c85d3c4b4734419b0e757974bbc94d8",
		"t1/judge/a2.sh":        "ecbb74661135af3695f63c630b5d6e91d12355a66131e52fd623fff749470503",
		"t1/judge/a3.sh":        "e1fdb2d07db87d8bd44ed714ce967fe4898c23083bd5184303ae18c06575c72c",
		"t1/judge/a6.sh":        "dedf2d67965b6e17a8a1982f00bf018d864071df1c448d69d4387d203b8464f0",
		"t1/judge/a7.sh":        "6c8a8bfdb498726d9c0a944751c2718974cca2899bab1f1f042d98a85bf921e3",
		"t1/judge/a8.py":        "e53ca5a3680ada783e744e167f374d864d69a78052ecb4009265ef99e102e160",
		"t1/judge/judge.sh":     "e5af2b667d8dfb7a443b51314e732d58342c15fb9a398f5fb5d3eb0d1dcd04a9",
		"t1/judge/oracle.py":    "f263d7526a6f0ff32d082280a36617c58abb994ec14307b6c0cb011fdda75e40",
		"t1/oracle-symbols.txt": "9bb974460b8ecabb10d69ec54accab9dd985ec3286cb0c607bf358d61c961082",
		"t1/pins.txt":           "20e3f3a33811cdecc892f6534d518a396f0fd50f58f22302b048d3a4072540e4",
		"t1/prompt.txt":         "304d88a2755a93e02a85ca5595568be4d4bf70796e756edd1c0adfe2c1e79865",
	}
	for rel, want := range frozen {
		b, err := os.ReadFile(filepath.Join(autosteerDir(), filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s is at %s; the T1 slice is frozen at %s (a change opens a new slice)", rel, got, want)
		}
	}
}
