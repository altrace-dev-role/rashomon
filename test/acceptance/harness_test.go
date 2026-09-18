package acceptance

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Binaries under test. They are real compiled binaries, exec'd as subprocesses,
// because the thing being asserted is an exit code. Calling a function under
// test cannot observe one, and `go run` reports its child's exit status as 1,
// which would mask the single value that matters here.
var (
	rashomonBin string
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

	dir, err := os.MkdirTemp("", "rashomon-bins-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)

	// The binary under test carries the fault-injection points. A released
	// binary does not, which is why the tag exists: the injection path must not
	// ship, but the panic barrier it exercises is the same code either way.
	rashomonBin, err = build(dir, "rashomon", "./cmd/rashomon", "rashomonfault")
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

// env is one isolated machine: its own store, its own Claude configuration
// directory, its own working directory for project-level settings, and a
// managed-policy path that does not exist unless a test creates it.
type env struct {
	t         *testing.T
	home      string
	configDir string
	cwd       string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return &env{t: t, home: t.TempDir(), configDir: t.TempDir(), cwd: t.TempDir()}
}

func (e *env) settingsPath() string { return filepath.Join(e.configDir, "settings.json") }
func (e *env) managedPath() string  { return filepath.Join(e.configDir, "managed-settings.json") }

func (e *env) environ(extra ...string) []string {
	base := append(os.Environ(),
		"RASHOMON_HOME="+e.home,
		"CLAUDE_CONFIG_DIR="+e.configDir,
		"RASHOMON_MANAGED_SETTINGS_PATH="+e.managedPath(),
	)
	return append(base, extra...)
}

// result is everything an invocation is judged on.
type result struct {
	exitCode int
	stdout   string
	stderr   string
}

func (e *env) command(stdin string, extraEnv []string, args ...string) *exec.Cmd {
	cmd := exec.Command(rashomonBin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = e.environ(extraEnv...)
	cmd.Dir = e.cwd
	return cmd
}

func (e *env) run(stdin string, extraEnv []string, args ...string) result {
	e.t.Helper()
	return e.wait(e.command(stdin, extraEnv, args...))
}

func (e *env) wait(cmd *exec.Cmd) result {
	e.t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// An error here is expected whenever the exit code is non-zero; the exit
	// code is read from ProcessState, so a failure to start is what matters.
	if err := cmd.Run(); err != nil && cmd.ProcessState == nil {
		e.t.Fatalf("starting %v: %v", cmd.Args, err)
	}
	return result{exitCode: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
}

// runBin runs a binary other than the one under test, which is how a test
// exercises an install made from a copy sitting at a path of its choosing.
func (e *env) runBin(bin, stdin string, args ...string) result {
	e.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = e.environ()
	cmd.Dir = e.cwd
	return e.wait(cmd)
}

// sh runs an installed command line through a shell, which is what Claude Code
// does with it and the only reader whose opinion about quoting matters.
func (e *env) sh(line, stdin string) result {
	e.t.Helper()
	cmd := exec.Command("sh", "-c", line)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = e.environ()
	cmd.Dir = e.cwd
	return e.wait(cmd)
}

func (e *env) hook(payload string, extraEnv ...string) result {
	e.t.Helper()
	return e.run(payload, extraEnv, append([]string{"hook"}, e.installArgs()...)...)
}

// post runs the PostToolUse path, as watch installs it.
func (e *env) post(payload string, extraEnv ...string) result {
	e.t.Helper()
	return e.run(payload, extraEnv, append([]string{"post"}, e.installArgs()...)...)
}

func (e *env) probe(phase, sessionID string, extraEnv ...string) result {
	e.t.Helper()
	return e.run(e.sessionPayload(phase, sessionID), extraEnv, append([]string{"probe", phase}, e.installArgs()...)...)
}

func (e *env) sessionPayload(phase, sessionID string) string {
	return fmt.Sprintf(`{"session_id":%q,"hook_event_name":"Session%s","transcript_path":"/tmp/none.jsonl","cwd":%q}`,
		sessionID, strings.ToUpper(phase[:1])+phase[1:], e.cwd)
}

// installArgs names this environment's store, as watch would have named it in
// the installed command line. Several tests invoke a hook before anything has
// opened that store: there is then no id to name, and passing none is also how
// an entry installed before this argument existed arrives.
func (e *env) installArgs() []string {
	e.t.Helper()
	if _, err := os.Stat(filepath.Join(e.home, "install.json")); err != nil {
		return nil
	}
	return []string{"--install", e.installID()}
}

func (e *env) watch(extraEnv ...string) result  { e.t.Helper(); return e.run("", extraEnv, "watch") }
func (e *env) detach(extraEnv ...string) result { e.t.Helper(); return e.run("", extraEnv, "detach") }
func (e *env) status(extraEnv ...string) result { e.t.Helper(); return e.run("", extraEnv, "status") }
func (e *env) forget(since string) result {
	e.t.Helper()
	return e.run("", nil, "forget", "--since", since)
}

func (e *env) forgetBefore(before string) result {
	e.t.Helper()
	return e.run("", nil, "forget", "--before", before)
}

// watched installs the hook and starts a session, which is the state every
// verified-coverage assertion assumes.
func (e *env) watched(sessionID string) {
	e.t.Helper()
	if res := e.watch(); res.exitCode != 0 {
		e.t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("start", sessionID); res.exitCode != 0 {
		e.t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
}

func (e *env) mustHook(payload string) {
	e.t.Helper()
	if res := e.hook(payload); res.exitCode != 0 {
		e.t.Fatalf("hook: exit %d, stderr %q", res.exitCode, res.stderr)
	}
}

func (e *env) mustPost(payload string) {
	e.t.Helper()
	if res := e.post(payload); res.exitCode != 0 {
		e.t.Fatalf("post: exit %d, stderr %q", res.exitCode, res.stderr)
	}
}

// reportSession is the report output contract, as a test reads it.
type reportSession struct {
	SessionID string `json:"session_id"`
	InstallID string `json:"install_id"`
	Coverage  struct {
		State            string   `json:"state"`
		Reasons          []string `json:"reasons"`
		StartRecorded    bool     `json:"start_recorded"`
		EndRecorded      bool     `json:"end_recorded"`
		HookEntryAtStart string   `json:"hook_entry_at_start"`
		HookEntryAtEnd   string   `json:"hook_entry_at_end"`
	} `json:"coverage"`
	Declarations struct {
		Recorded          int      `json:"recorded"`
		WithoutTranscript int      `json:"without_transcript"`
		Unterminated      []string `json:"unterminated"`
		Dropped           []string `json:"dropped"`
		WithoutExecution  []struct {
			ToolUseID      string `json:"tool_use_id"`
			PermissionMode string `json:"permission_mode"`
		} `json:"without_execution"`
		ByTool map[string]int `json:"by_tool"`
	} `json:"declarations"`
	Executions struct {
		Recorded int `json:"recorded"`
	} `json:"executions"`
	Transcripts []struct {
		Path                  string   `json:"path"`
		Readable              bool     `json:"readable"`
		Files                 *int     `json:"files"`
		IDsInTranscript       *int     `json:"ids_in_transcript"`
		IDsRecorded           int      `json:"ids_recorded"`
		MissingFromStore      []string `json:"missing_from_store"`
		MissingFromTranscript []string `json:"missing_from_transcript"`
		IDsExecuted           int      `json:"ids_executed"`
		ResultsInTranscript   *int     `json:"results_in_transcript"`
		ExecutedButUnrecorded []string `json:"executed_but_unrecorded"`
		DeclaredWithoutResult []string `json:"declared_without_result"`
	} `json:"transcripts"`
	Gaps []map[string]any `json:"gaps"`
	// The destinations block, as much of it as the acceptance tests assert on.
	Destinations struct {
		Observed              bool     `json:"observed"`
		Reason                string   `json:"reason"`
		WireOnly              []string `json:"wire_only"`
		ClientPlane           []string `json:"client_plane"`
		DeclaredNotObserved   []string `json:"declared_not_observed"`
		NotObservable         []string `json:"not_observable"`
		ProxyOnPath           string   `json:"proxy_on_path"`
		ExecutedNotAsDeclared int      `json:"executed_not_as_declared"`
	} `json:"destinations"`
}

// report renders one session as JSON. The JSON form is what a consumer parses
// and what every assertion below is written against; the text form is for a
// terminal and is asserted separately, in report_test.go.
func (e *env) report(sessionID string) reportSession {
	e.t.Helper()
	res := e.run("", nil, "report", "--json", "--session", sessionID)
	if res.exitCode != 0 {
		e.t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []reportSession `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil {
		e.t.Fatalf("report output is not JSON: %v\n%s", err, res.stdout)
	}
	if len(rep.Sessions) != 1 {
		e.t.Fatalf("report returned %d sessions, want 1", len(rep.Sessions))
	}
	return rep.Sessions[0]
}

func (e *env) hasReason(sess reportSession, reason string) bool {
	for _, r := range sess.Coverage.Reasons {
		if r == reason {
			return true
		}
	}
	return false
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

func (r record) str(key string) string {
	s, _ := r.fields[key].(string)
	return s
}

// read parses one NDJSON file from a run directory. A missing file reads as no
// records, which is a real and distinct outcome from an empty one.
func (e *env) read(sessionID, name string) []record {
	e.t.Helper()
	return readNDJSON(e.t, filepath.Join(e.home, "runs", sessionID, name))
}

// records returns declarations and terminals, from the ordered stream and the
// spill file together, which is how a reader sees them.
func (e *env) records(sessionID string) []record {
	e.t.Helper()
	return append(e.read(sessionID, "records.ndjson"), e.read(sessionID, "spill.ndjson")...)
}

func (e *env) declarations(sessionID string) []record {
	e.t.Helper()
	return recordsOfType(e.records(sessionID), "declaration")
}

func (e *env) terminals(sessionID string) []record {
	e.t.Helper()
	return recordsOfType(e.records(sessionID), "terminal")
}

func (e *env) executions(sessionID string) []record {
	e.t.Helper()
	return recordsOfType(e.records(sessionID), "execution")
}

func (e *env) coverage(sessionID, phase string) []record {
	e.t.Helper()
	var out []record
	for _, r := range recordsOfType(e.read(sessionID, "coverage.ndjson"), "coverage") {
		if r.str("phase") == phase {
			out = append(out, r)
		}
	}
	return out
}

func (e *env) gaps() []record {
	e.t.Helper()
	return readNDJSON(e.t, filepath.Join(e.home, "gaps.ndjson"))
}

func readNDJSON(t *testing.T, path string) []record {
	t.Helper()
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
func (e *env) declarationsAfter(commands ...string) []record {
	e.t.Helper()
	for _, c := range commands {
		p := defaultPayload()
		p.ToolInput = map[string]any{"command": c}
		e.mustHook(p.build(e.t))
	}
	decls := e.declarations(testSession)
	if len(decls) != len(commands) {
		e.t.Fatalf("got %d declarations, want %d", len(decls), len(commands))
	}
	return decls
}

// walkStore returns every entry in the store with its mode and contents, for
// the canary sweep and the permission assertions.
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

// settingsFile is the user's settings.json as a test reads it back: through a
// generic decoder, which is the only reader that matters for validity.
type settingsFile struct {
	Hooks map[string][]matcherGroup  `json:"hooks"`
	Other map[string]json.RawMessage `json:"-"`
}

type matcherGroup struct {
	Matcher *string `json:"matcher"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
	raw json.RawMessage
}

func (e *env) settings() settingsFile {
	e.t.Helper()
	data, err := os.ReadFile(e.settingsPath())
	if err != nil {
		e.t.Fatalf("reading settings: %v", err)
	}
	var sf settingsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		e.t.Fatalf("settings.json is not valid JSON: %v\n%s", err, data)
	}
	// Keep every matcher group's raw bytes too, for byte-identity assertions.
	var loose struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &loose); err == nil {
		for event, raws := range loose.Hooks {
			for i := range raws {
				if i < len(sf.Hooks[event]) {
					sf.Hooks[event][i].raw = raws[i]
				}
			}
		}
	}
	return sf
}

func (e *env) settingsBytes() []byte {
	e.t.Helper()
	data, err := os.ReadFile(e.settingsPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		e.t.Fatalf("reading settings: %v", err)
	}
	return data
}

func (e *env) writeSettings(content string) {
	e.t.Helper()
	if err := os.WriteFile(e.settingsPath(), []byte(content), 0o644); err != nil {
		e.t.Fatalf("seeding settings: %v", err)
	}
}

// ours returns the matcher groups under an event that carry an --install id.
func ours(groups []matcherGroup) []matcherGroup {
	var out []matcherGroup
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, " --install ") {
				out = append(out, g)
				break
			}
		}
	}
	return out
}

func (e *env) installID() string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.home, "install.json"))
	if err != nil {
		e.t.Fatalf("reading install.json: %v", err)
	}
	var meta struct {
		InstallID string `json:"install_id"`
	}
	if err := json.Unmarshal(data, &meta); err != nil || meta.InstallID == "" {
		e.t.Fatalf("install.json is malformed: %s", data)
	}
	return meta.InstallID
}

// waitFor polls until cond holds or the deadline passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func jsonCanonical(v any) ([]byte, error) {
	// encoding/json sorts map keys, which is all canonical needs to mean here.
	return json.Marshal(v)
}

func (e *env) reportAll() []reportSession {
	e.t.Helper()
	res := e.run("", nil, "report", "--json")
	if res.exitCode != 0 {
		e.t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []reportSession `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil {
		e.t.Fatalf("report output is not JSON: %v\n%s", err, res.stdout)
	}
	return rep.Sessions
}

// writeTranscript writes a transcript holding one assistant message, and
// returns its path. Used by the redaction tests, which need the agent's own
// summary to exist before they can check it is not shared.
func (e *env) writeTranscript(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(e.home, "transcript.jsonl")
	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeProxyStore builds a causal.db in the proxy's shape and returns its path.
//
// The timestamp is written the way the PROXY writes it -- Go's time.Time
// String() rendering with the monotonic suffix attached -- because a fixture in
// RFC 3339 would let these tests pass against a reader that cannot parse the
// real thing.
func (e *env) writeProxyStore(t *testing.T, hosts ...string) string {
	t.Helper()
	path := filepath.Join(e.home, "causal.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open proxy store: %v", err)
	}
	defer db.Close() //nolint:errcheck // the exec errors below are what matter

	if _, err := db.Exec(`CREATE TABLE causal_records (
		sequence_num INTEGER PRIMARY KEY,
		request_id TEXT NOT NULL DEFAULT '',
		run_id TEXT NOT NULL DEFAULT '',
		timestamp DATETIME NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL DEFAULT '',
		target_host TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create proxy store: %v", err)
	}
	for i, h := range hosts {
		ts := time.Now().Format("2006-01-02 15:04:05.999999999 -0700 MST") +
			fmt.Sprintf(" m=+%d.000000000", i)
		if _, err := db.Exec(
			`INSERT INTO causal_records (sequence_num, request_id, timestamp, reason, action, target_host)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			i+1, fmt.Sprintf("req-%d", i), ts, "host_not_in_allowlist_observed", "WARN", h+":443",
		); err != nil {
			t.Fatalf("insert proxy row: %v", err)
		}
	}
	return path
}

// writeFullTranscript writes a transcript that matches a recorded call: an
// assistant tool_use block with the given id, a user tool_result answering it,
// and a final assistant message.
//
// The accounting equation is checked per transcript, so a fixture whose
// transcript names none of the recorded ids renders as a mismatch in both
// directions -- which is correct behaviour and makes such a fixture useless for
// testing the HEALTHY path. This is the shape a real session produces.
func (e *env) writeFullTranscript(t *testing.T, toolUseID, final string) string {
	t.Helper()
	path := filepath.Join(e.home, "full-transcript.jsonl")

	lines := []map[string]any{
		{"message": map[string]any{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "tool_use", "id": toolUseID, "name": "Bash", "input": map[string]any{}},
			},
		}},
		{"message": map[string]any{
			"role": "user",
			"content": []map[string]any{
				{"type": "tool_result", "tool_use_id": toolUseID},
			},
		}},
		{"message": map[string]any{
			"role":    "assistant",
			"content": []map[string]any{{"type": "text", "text": final}},
		}},
	}

	var buf bytes.Buffer
	for _, l := range lines {
		body, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(body)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
