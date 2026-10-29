package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// rgArgs carries the receipt tests' rashomon arguments to the child, one
// unit separator between each.
const rgArgs = "RG_RASHOMON_ARGS"

// TestRGHelperRashomon is the rashomon the receipt tests' hook bodies find on
// the PATH: the test binary, run as rashomon on the arguments rgArgs carries.
// Run any other way it skips.
func TestRGHelperRashomon(t *testing.T) {
	raw, ok := os.LookupEnv(rgArgs)
	if !ok {
		t.Skip("the receipt tests' rashomon; run only as their child")
	}
	var args []string
	if raw != "" {
		args = strings.Split(raw, "\x1f")
	}
	os.Exit(run(args, os.Stdin, os.Stdout, os.Stderr))
}

// receiptRepo is a git repository with a store beside it, and the
// environment the hook bodies run in: the store at RASHOMON_HOME, rashomon
// on the PATH (TestRGHelperRashomon), and git reading no configuration of
// this machine's, so no user hook path or template reaches the run.
type receiptRepo struct {
	dir, home, rashomon string
	env                 []string
}

func newReceiptRepo(t *testing.T) *receiptRepo {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the hook bodies are POSIX sh")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	wrapper := "#!/bin/sh\n" + rgArgs + "=$(printf '%s\\037' \"$@\"); " + rgArgs + "=${" + rgArgs + "%?}\n" +
		"export " + rgArgs + "\nexec '" + os.Args[0] + "' -test.run='^TestRGHelperRashomon$'\n"
	if err := os.WriteFile(filepath.Join(bin, "rashomon"), []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &receiptRepo{dir: filepath.Join(tmp, "repo"), home: filepath.Join(tmp, "store"), rashomon: filepath.Join(bin, "rashomon")}
	r.env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RASHOMON_HOME="+r.home, "HOME="+tmp, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=rg", "GIT_AUTHOR_EMAIL=rg@example.invalid",
		"GIT_COMMITTER_NAME=rg", "GIT_COMMITTER_EMAIL=rg@example.invalid")
	if err := os.Mkdir(r.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	r.git(t, "init", "-q")
	return r
}

// try runs name in the repository and returns its stdout and exit code; a
// failure to start fails the test.
func (r *receiptRepo) try(t *testing.T, stdin []byte, name string, args ...string) (string, int) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir, c.Env, c.Stdin = r.dir, r.env, bytes.NewReader(stdin)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err := c.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out.String(), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("%s %q: %v\nstderr: %s", name, args, err, errb.String())
	}
	return out.String(), 0
}

// cmd is try, and a non-zero exit fails the test.
func (r *receiptRepo) cmd(t *testing.T, stdin []byte, name string, args ...string) string {
	t.Helper()
	out, code := r.try(t, stdin, name, args...)
	if code != 0 {
		t.Fatalf("%s %q: exit %d", name, args, code)
	}
	return out
}

func (r *receiptRepo) git(t *testing.T, args ...string) string { return r.cmd(t, nil, "git", args...) }

// hook runs a hook body under sh with args, as git would run the file.
func (r *receiptRepo) hook(t *testing.T, body string, args ...string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hook")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	r.cmd(t, nil, "sh", append([]string{path}, args...)...)
}

// seed appends a checkpoint for project to a session in the store, through
// the store's own write path, and returns the record as stored.
func (r *receiptRepo) seed(t *testing.T, session, project string) []byte {
	t.Helper()
	st, err := store.Open(r.home)
	if err != nil {
		t.Fatal(err)
	}
	cp := &verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion,
		RecordedAtUnixMS: time.Now().UnixMilli(), SessionID: session, Point: verify.PointStop, PointSeq: 1,
		ProjectRoot: project, Gates: []verify.CheckpointGate{{Gate: "spec:unit", Kind: "test", Source: "spec",
			Required: true, State: verify.Green}}}
	if err := st.AppendRecord(session, cp); err != nil {
		t.Fatal(err)
	}
	line, ok, _, err := st.LatestRecordBytes(session, store.TypeVerify)
	if err != nil || !ok {
		t.Fatalf("the seeded checkpoint does not read back: ok %t, err %v", ok, err)
	}
	return line
}

// message writes a commit message file in the repository's git dir.
func (r *receiptRepo) message(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(r.dir, ".git", "COMMIT_EDITMSG")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func rgRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRG1a_TrailerWrittenByHook (5.5): the prepare-commit-msg body `receipt
// on` prints, run under sh in a repository whose project has a checkpoint,
// gives the message the trailer naming that record's digest once, before
// git's comment lines; a second run changes nothing; a message carrying an
// older receipt has it replaced, not doubled. Break: print the trailer over
// other bytes, or append without replacing, and a row names it.
func TestRG1a_TrailerWrittenByHook(t *testing.T) {
	r := newReceiptRepo(t)
	record := r.seed(t, "sess-rg1a", baseline.Key(r.dir))
	trailer := receipt.Trailer(record)

	msg := r.message(t, "subject\n\nbody\n\n# git's comment\n")
	r.hook(t, receipt.PrepareCommitMsg, msg)
	want := "subject\n\nbody\n\n" + trailer + "\n\n# git's comment\n"
	if got := rgRead(t, msg); got != want {
		t.Fatalf("after the hook: %q, want %q", got, want)
	}
	r.hook(t, receipt.PrepareCommitMsg, msg)
	if got := rgRead(t, msg); got != want {
		t.Errorf("after a second run: %q, want it unchanged, %q", got, want)
	}

	old := r.message(t, "subject\n\nRashomon-Receipt: sha256:00\n")
	r.hook(t, receipt.PrepareCommitMsg, old)
	if got, want := rgRead(t, old), "subject\n\n"+trailer+"\n"; got != want {
		t.Errorf("an older receipt: %q, want it replaced, %q", got, want)
	}
}

// TestRG1b_NotesRefHoldsTheCheckpoint (5.5): after a commit whose message the
// prepare-commit-msg body named, the post-commit body leaves on that commit,
// in refs/notes/rashomon, the checkpoint record as the store holds it and one
// newline, and the trailer is that note's digest; `receipt note` for a
// digest the latest checkpoint does not have prints nothing and fails. Break:
// write the note for another commit, digest bytes other than the stored
// record's, or print the record whatever the digest, and a check fails.
func TestRG1b_NotesRefHoldsTheCheckpoint(t *testing.T) {
	r := newReceiptRepo(t)
	record := r.seed(t, "sess-rg1b", baseline.Key(r.dir))
	if err := os.WriteFile(filepath.Join(r.dir, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.git(t, "add", "a.txt")
	msg := r.message(t, "add a\n")
	r.hook(t, receipt.PrepareCommitMsg, msg)
	r.git(t, "commit", "-q", "-F", msg)
	r.hook(t, receipt.PostCommit)

	note := r.git(t, "notes", "--ref", "rashomon", "show", "HEAD")
	if note != string(record)+"\n" {
		t.Fatalf("HEAD's note = %q, want the stored record and a newline, %q", note, record)
	}
	value := strings.TrimSpace(r.git(t, "log", "-1", "--format=%(trailers:key=Rashomon-Receipt,valueonly)", "HEAD"))
	if digest := receipt.Digest([]byte(strings.TrimSuffix(note, "\n"))); value != digest {
		t.Errorf("HEAD's trailer names %q; its note's digest is %q", value, digest)
	}
	if out, code := r.try(t, nil, r.rashomon, "receipt", "note", "sha256:00"); code != exitFail || out != "" {
		t.Errorf("receipt note for a digest the latest checkpoint does not have: exit %d, stdout %q; want 1 and nothing", code, out)
	}
}

// TestRG1c_PushIsExplicitOnly (5.5): `receipt on` prints both hook bodies as
// they are and the one push line, naming the notes ref; neither body pushes,
// so nothing reaches a remote but the line the user runs (and rashomon runs
// no git at all: TRU-40). Break: drop the push line, or let a body push, and
// a check fails.
func TestRG1c_PushIsExplicitOnly(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"receipt", "on"}, nil, &out, &errb); code != exitOK || errb.Len() != 0 {
		t.Fatalf("receipt on: exit %d, stderr %q", code, errb.String())
	}
	for _, want := range []string{receipt.PrepareCommitMsg, receipt.PostCommit,
		"\n  git push <remote> refs/notes/rashomon\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("receipt on does not print %q", want)
		}
	}
	for name, body := range map[string]string{"prepare-commit-msg": receipt.PrepareCommitMsg, "post-commit": receipt.PostCommit} {
		if strings.Contains(body, "push") {
			t.Errorf("the %s body pushes:\n%s", name, body)
		}
	}
	errb.Reset()
	if code := run([]string{"receipt", "on", "--yes"}, nil, &out, &errb); code != exitFail ||
		errb.String() != "rashomon: unknown argument \"--yes\"\n" {
		t.Errorf("receipt on --yes: exit %d, stderr %q; want it refused", code, errb.String())
	}
}

// TestRG1d_NoSessionNoTrailer (5.5): with no store, and with a store holding
// only another project's checkpoint, `receipt trailer` prints nothing and
// exits 0, and the prepare-commit-msg body leaves the message as it was, so
// a commit made where rashomon recorded nothing is neither named nor held up.
// Break: fall back to the store's newest checkpoint whatever its project,
// and the second row gains a trailer.
func TestRG1d_NoSessionNoTrailer(t *testing.T) {
	for _, other := range []bool{false, true} {
		r := newReceiptRepo(t)
		if other {
			r.seed(t, "sess-rg1d", filepath.Join(r.home, "elsewhere"))
		}
		if got := r.cmd(t, nil, r.rashomon, "receipt", "trailer"); got != "" {
			t.Errorf("another project's checkpoint %t: receipt trailer printed %q, want nothing", other, got)
		}
		msg := r.message(t, "subject\n")
		r.hook(t, receipt.PrepareCommitMsg, msg)
		if got := rgRead(t, msg); got != "subject\n" {
			t.Errorf("another project's checkpoint %t: the message became %q", other, got)
		}
	}
}
