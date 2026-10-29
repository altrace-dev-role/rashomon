package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// mgArgs carries the planner's arguments to the child, one unit separator
// between each.
const mgArgs = "MG_PLANNER_ARGS"

// TestMGHelperPlanner is the planner mergeguard.sh runs in these tests: the
// test binary, run as mergeguard on the arguments mgArgs carries. Run any
// other way it skips.
func TestMGHelperPlanner(t *testing.T) {
	raw, ok := os.LookupEnv(mgArgs)
	if !ok {
		t.Skip("the guard tests' planner; run only as their child")
	}
	var args []string
	if raw != "" {
		args = strings.Split(raw, "\x1f")
	}
	os.Exit(guard(args, os.Stdin, os.Stdout, os.Stderr))
}

// guardEnv is a scratch directory for repositories, and the environment
// mergeguard.sh runs in: MERGEGUARD naming the planner, and git reading no
// configuration of this machine's.
type guardEnv struct {
	tmp, script string
	env         []string
}

func newGuardEnv(t *testing.T) *guardEnv {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mergeguard.sh is POSIX sh")
	}
	script, err := filepath.Abs("mergeguard.sh")
	if err != nil {
		t.Fatal(err)
	}
	g := &guardEnv{tmp: t.TempDir(), script: script}
	planner := filepath.Join(g.tmp, "planner")
	wrapper := "#!/bin/sh\n" + mgArgs + "=$(printf '%s\\037' \"$@\"); " + mgArgs + "=${" + mgArgs + "%?}\n" +
		"export " + mgArgs + "\nexec '" + os.Args[0] + "' -test.run='^TestMGHelperPlanner$'\n"
	if err := os.WriteFile(planner, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	g.env = append(os.Environ(), "MERGEGUARD="+planner, "HOME="+g.tmp,
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=mg", "GIT_AUTHOR_EMAIL=mg@example.invalid",
		"GIT_COMMITTER_NAME=mg", "GIT_COMMITTER_EMAIL=mg@example.invalid")
	return g
}

// run runs name in dir and returns its stdout and exit code; a failure to
// start fails the test.
func (g *guardEnv) run(t *testing.T, dir, name string, args ...string) (string, int) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir, c.Env = dir, g.env
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err := c.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return out.String(), exit.ExitCode()
	case err != nil:
		t.Fatalf("%s %q: %v\nstderr: %s", name, args, err, errb.String())
	}
	return out.String(), 0
}

// git runs git in dir; a non-zero exit fails the test.
func (g *guardEnv) git(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, code := g.run(t, dir, "git", args...); code != 0 {
		t.Fatalf("git %q: exit %d: %s", args, code, out)
	}
}

// write writes a file under dir, making its directories.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// guardSpec declares unit (run in sub, green only there), lint (red) and
// one ci command (green).
const guardSpec = `{"rashomon_spec": 1,
 "gates": [
  {"id": "unit", "kind": "test", "argv": ["test", "-f", "marker"], "dir": "sub"},
  {"id": "lint", "kind": "lint", "argv": ["false"]}
 ],
 "ci_commands": [["true"]]}
`

// record is a checkpoint record holding gates, as the store would write it.
func record(t *testing.T, gates ...verify.CheckpointGate) []byte {
	t.Helper()
	b, err := json.Marshal(verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion,
		Point: verify.PointStop, ProjectRoot: "/work/p", Gates: gates})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// repo is a repository holding the spec, sub/marker, and one commit whose
// message is msg; note, when not nil, is that commit's rashomon note.
func (g *guardEnv) repo(t *testing.T, name, msg string, note []byte) string {
	t.Helper()
	dir := filepath.Join(g.tmp, name)
	write(t, dir, ".claude/rashomon/spec.json", guardSpec)
	write(t, dir, "sub/marker", "")
	g.git(t, dir, "init", "-q")
	g.git(t, dir, "add", ".")
	write(t, g.tmp, name+".msg", msg)
	g.git(t, dir, "commit", "-q", "-F", filepath.Join(g.tmp, name+".msg"))
	if note != nil {
		write(t, g.tmp, name+".note", string(note))
		g.git(t, dir, "notes", "--ref", "rashomon", "add", "-F", filepath.Join(g.tmp, name+".note"), "HEAD")
	}
	return dir
}

func gate(id string, state verify.State) verify.CheckpointGate {
	return verify.CheckpointGate{Gate: id, State: state}
}

// TestRG2a_GuardPrintsAndExitsZero (6.4): mergeguard.sh exits 0 and ends on
// the counts line whatever it finds: no trailer, a trailer with no note, a
// note that is not the record the trailer names, and a gate whose rerun
// differs from the checkpoint. v0 reports and never fails a build. Break:
// exit non-zero on a difference, and the last row names it.
func TestRG2a_GuardPrintsAndExitsZero(t *testing.T) {
	g := newGuardEnv(t)
	rec := record(t, gate("spec:lint", verify.Green))
	none := "mergeguard: 0 agree, 0 differ, 0 not rerun\n"
	for _, c := range []struct {
		name, msg string
		note      []byte
		want      string
	}{
		{"untrailed", "subject\n", nil, "mergeguard: HEAD carries no Rashomon-Receipt trailer\n" + none},
		{"unnoted", "subject\n\n" + receipt.Trailer(rec) + "\n", nil, "mergeguard: HEAD has no rashomon note\n" + none},
		{"swapped", "subject\n\n" + receipt.Trailer(rec) + "\n", []byte("{}\n"),
			"mergeguard: the note's digest " + receipt.Digest([]byte("{}")) + " is not the trailer's " + receipt.Digest(rec) + "\n" + none},
		{"differs", "subject\n\n" + receipt.Trailer(rec) + "\n", append(rec, '\n'),
			"spec:lint: checkpoint green, rerun red\nmergeguard: 0 agree, 1 differ, 0 not rerun\n"},
	} {
		dir := g.repo(t, c.name, c.msg, c.note)
		out, code := g.run(t, dir, "sh", g.script, "HEAD")
		if code != 0 || out != c.want {
			t.Errorf("%s: exit %d, stdout %q; want exit 0 and %q", c.name, code, out, c.want)
		}
	}
}

// TestRG2b_GuardComparesGateByGate (6.4): in a clone that lacks the notes,
// mergeguard.sh fetches refs/notes/rashomon from the remote it is given,
// reruns each gate the repository's spec declares (spec:unit in its dir,
// spec:lint, ci:0) and compares each with the checkpoint's state, line by
// line in the checkpoint's order; a learned gate declares no command and is
// not rerun; the last line counts the three. Break: skip the fetch, run a
// gate outside its dir, compare against anything but the checkpoint's state,
// or count wrong, and the output differs.
func TestRG2b_GuardComparesGateByGate(t *testing.T) {
	g := newGuardEnv(t)
	rec := record(t, gate("spec:unit", verify.Green), gate("spec:lint", verify.Green),
		gate("ci:0", verify.Red), gate("learned:go-test", verify.Unknown))
	origin := g.repo(t, "origin", "subject\n\n"+receipt.Trailer(rec)+"\n", append(rec, '\n'))
	clone := filepath.Join(g.tmp, "clone")
	g.git(t, g.tmp, "clone", "-q", origin, clone)

	out, code := g.run(t, clone, "sh", g.script, "HEAD", "origin")
	want := "spec:unit: checkpoint green, rerun green\n" +
		"spec:lint: checkpoint green, rerun red\n" +
		"ci:0: checkpoint red, rerun green\n" +
		"learned:go-test: checkpoint unknown, rerun not declared\n" +
		"mergeguard: 1 agree, 2 differ, 1 not rerun\n"
	if code != 0 || out != want {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 0 and:\n%s", code, out, want)
	}
}

// TestRG2c_TheNoteNeverRuns (6.4): what a note carries reaches the script
// as data. A gate id holding a quote and a command substitution is printed
// as it is and runs nothing, and a gate the note names that the spec does not
// declare is not rerun, whatever the note claims. Break: quote anything less,
// and the marker file appears.
func TestRG2c_TheNoteNeverRuns(t *testing.T) {
	g := newGuardEnv(t)
	marker := filepath.Join(g.tmp, "ran")
	id := "spec:x'; touch " + marker + "; '$(touch " + marker + ")"
	rec := record(t, gate(id, verify.State("green'$(touch "+marker+")")))
	dir := g.repo(t, "hostile", "subject\n\n"+receipt.Trailer(rec)+"\n", append(rec, '\n'))
	out, code := g.run(t, dir, "sh", g.script, "HEAD")
	want := id + ": checkpoint green'$(touch " + marker + "), rerun not declared\n" +
		"mergeguard: 0 agree, 0 differ, 1 not rerun\n"
	if code != 0 || out != want {
		t.Errorf("exit %d, stdout %q; want exit 0 and %q", code, out, want)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Errorf("the note's text ran: %s exists", marker)
	}
}

// TestRG2c_ACommandShCannotCarryIsNotRun: a declared command with a NUL in a
// word or its dir, which no sh word can hold, is not runnable rather than
// cut short at the NUL. Break: drop either check, and its row names it.
func TestRG2c_ACommandShCannotCarryIsNotRun(t *testing.T) {
	for _, c := range []struct {
		cmd  command
		want bool
	}{
		{command{"", []string{"true"}}, true},
		{command{"", nil}, false},
		{command{"", []string{"true", "a\x00b"}}, false},
		{command{"sub\x00", []string{"true"}}, false},
	} {
		if got := runnable(c.cmd); got != c.want {
			t.Errorf("runnable(%q) = %t, want %t", c.cmd, got, c.want)
		}
	}
}
