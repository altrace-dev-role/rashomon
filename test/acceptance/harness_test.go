package acceptance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Binaries under test. They are real compiled binaries, exec'd as subprocesses,
// because the thing being asserted is an exit code. Calling a function under
// test cannot observe one, and `go run` reports its child's exit status as 1,
// which would mask the single value that matters here.
var (
	attestBin   string
	panickerBin string
	moduleRoot  string
)

func TestMain(m *testing.M) {
	code, err := buildAndRun(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func buildAndRun(m *testing.M) (int, error) {
	root, err := filepath.Abs("../..")
	if err != nil {
		return 0, err
	}
	moduleRoot = root

	dir, err := os.MkdirTemp("", "attest-bins-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)

	// The binary under test carries the fault-injection points. A released
	// binary does not, which is why the tag exists: the injection path must not
	// ship, but the panic barrier it exercises is the same code either way.
	attestBin, err = build(dir, "attest", "./cmd/attest", "attestfault")
	if err != nil {
		return 0, err
	}
	panickerBin, err = build(dir, "panicker", "./test/fixtures/panicker")
	if err != nil {
		return 0, err
	}
	return m.Run(), nil
}

func build(dir, name, pkg string, tags ...string) (string, error) {
	out := filepath.Join(dir, name)
	args := []string{"build", "-o", out}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, pkg)

	cmd := exec.Command("go", args...)
	cmd.Dir = moduleRoot
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building %s: %v\n%s", pkg, err, b)
	}
	return out, nil
}

// result is everything a hook invocation is judged on.
type result struct {
	exitCode int
	stdout   string
	stderr   string
}

// runHook invokes `attest hook` with payload on stdin against an isolated store.
func runHook(t *testing.T, home, payload string, env ...string) result {
	t.Helper()

	cmd := exec.Command(attestBin, "hook")
	cmd.Stdin = strings.NewReader(payload)
	cmd.Env = append(os.Environ(), "ATTEST_HOME="+home)
	cmd.Env = append(cmd.Env, env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// An error here is expected whenever the exit code is non-zero; the exit
	// code is read from ProcessState, so a failure to start is what matters.
	if err := cmd.Run(); err != nil && cmd.ProcessState == nil {
		t.Fatalf("starting %s: %v", attestBin, err)
	}
	return result{
		exitCode: cmd.ProcessState.ExitCode(),
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}
}

// record is one parsed NDJSON line alongside the bytes it was parsed from, so a
// test can assert on both structure and serialized width.
type record struct {
	fields map[string]any
	raw    string
}

func (r record) typ() string {
	s, _ := r.fields["type"].(string)
	return s
}

// readRecords parses one NDJSON file from a run directory. A missing file reads
// as no records, which is a real and distinct outcome from an empty one.
func readRecords(t *testing.T, home, sessionID, name string) []record {
	t.Helper()

	path := filepath.Join(home, "runs", sessionID, name)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()

	var out []record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for line := 1; sc.Scan(); line++ {
		raw := sc.Text()
		var fields map[string]any
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			t.Fatalf("%s line %d is not one JSON object: %v", path, line, err)
		}
		out = append(out, record{fields: fields, raw: raw})
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return out
}

func recordsOfType(recs []record, typ string) []record {
	var out []record
	for _, r := range recs {
		if r.typ() == typ {
			out = append(out, r)
		}
	}
	return out
}

// keyPaths flattens a record into dotted key paths, so an assertion covers
// nested objects too. A content field smuggled in one level down is exactly the
// failure a top-level key-set check would wave through.
func keyPaths(v any, prefix string, into map[string]bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, child := range obj {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		into[path] = true
		keyPaths(child, path, into)
	}
}

func pathsOf(r record) map[string]bool {
	out := map[string]bool{}
	keyPaths(r.fields, "", out)
	return out
}

func assertKeySet(t *testing.T, r record, allowed []string) {
	t.Helper()

	want := map[string]bool{}
	for _, k := range allowed {
		want[k] = true
	}
	got := pathsOf(r)

	for k := range got {
		if !want[k] {
			t.Errorf("record carries key %q, which is not in the allowlist", k)
		}
	}
	for k := range want {
		if !got[k] {
			t.Errorf("record is missing allowlisted key %q", k)
		}
	}
}

// walkStore returns every regular file in the store with its mode and contents,
// for the canary sweep and the permission assertions.
func walkStore(t *testing.T, home string) map[string]struct {
	mode fs.FileMode
	body []byte
} {
	t.Helper()

	out := map[string]struct {
		mode fs.FileMode
		body []byte
	}{}

	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		rel, rerr := filepath.Rel(home, path)
		if rerr != nil {
			return rerr
		}
		var body []byte
		if !d.IsDir() {
			body, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		out[rel] = struct {
			mode fs.FileMode
			body []byte
		}{mode: info.Mode(), body: body}
		return nil
	})
	if err != nil {
		t.Fatalf("walking store: %v", err)
	}
	return out
}

// nested walks a dotted key path into a record.
func nested(r record, path string) (any, bool) {
	var cur any = r.fields
	for _, part := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// declarationsAfter runs the hook once per command against one store and
// returns the declarations in order, so two records differ only by the input
// that produced them.
func declarationsAfter(t *testing.T, home string, commands ...string) []record {
	t.Helper()

	for _, c := range commands {
		p := defaultPayload()
		p.ToolInput = map[string]any{"command": c}
		if res := runHook(t, home, p.build(t)); res.exitCode != 0 {
			t.Fatalf("exit code %d, want 0 (stderr: %q)", res.exitCode, res.stderr)
		}
	}

	decls := recordsOfType(readRecords(t, home, testSession, "records.ndjson"), "declaration")
	if len(decls) != len(commands) {
		t.Fatalf("got %d declarations, want %d", len(decls), len(commands))
	}
	return decls
}
