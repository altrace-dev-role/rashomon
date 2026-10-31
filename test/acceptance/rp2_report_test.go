package acceptance

// REPORT1b: what the project knows, why a note fired, and the previous
// session's line at startup, on the real binary against a real store.
// Everything here reads: the report block, `why` and the startup line print
// to the user, and `why` and the ledger read change no file.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// rpTree is every file under root with its bytes and modification time.
func rpTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b.WriteString(p + " " + info.ModTime().String() + " " + string(data) + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestRP2v_TheReportCarriesWhatTheProjectKnows: `report --session` prints the
// knowledge block after the autosteer block and carries it under `knowledge`
// in JSON; the report of every session carries none, null in JSON. Break:
// fill the block for every session, or drop it from one.
func TestRP2v_TheReportCarriesWhatTheProjectKnows(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	res := e.run("", nil, "report", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report: exit %d, %s", res.exitCode, res.stderr)
	}
	i, j := strings.Index(res.stdout, "  autosteer\n"), strings.Index(res.stdout, "  knowledge (project ")
	if i < 0 || j < i {
		t.Fatalf("the knowledge block must follow the autosteer block:\n%s", res.stdout)
	}
	for _, want := range []string{"    baseline: no earlier checkpoint of this project\n", "    retained: ", "    discovered this session: ", "    invalidated this session: none\n",
		"    gate states at the Stops: ", "    project facts: read at each session start, not stored (rashomon brief)\n"} {
		if !strings.Contains(res.stdout[j:], want) {
			t.Errorf("the knowledge block lacks %q:\n%s", want, res.stdout[j:])
		}
	}
	for _, args := range [][]string{{"report", "--json", "--session", testSession}, {"report", "--json"}} {
		out := e.run("", nil, args...)
		var rep struct {
			Sessions []struct {
				SessionID string          `json:"session_id"`
				Knowledge json.RawMessage `json:"knowledge"`
			} `json:"sessions"`
		}
		if out.exitCode != 0 || json.Unmarshal([]byte(out.stdout), &rep) != nil || len(rep.Sessions) == 0 {
			t.Fatalf("%v: exit %d, %s", args, out.exitCode, out.stderr)
		}
		for _, s := range rep.Sessions {
			one := len(args) == 4 && s.SessionID == testSession
			if has := string(s.Knowledge) != "null"; has != one {
				t.Errorf("%v: session %s knowledge present %v, want %v", args, s.SessionID, has, one)
			}
		}
	}
}

// TestRP2w_TheReportNeverWritesTheLedger: a ledger whose state does not check
// is named in the autosteer block and the knowledge block and is left exactly
// as it was; the quarantine is the next learn verb's to write. Break: open
// the ledger with learn.Open, which saves.
func TestRP2w_TheReportNeverWritesTheLedger(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	dir := strings.TrimSuffix(baseline.FileName(baseline.Key(e.cwd)), baseline.FileExt)
	ledger := filepath.Join(e.home, "learn", dir)
	if err := os.MkdirAll(ledger, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "lessons.json"), []byte(`{"schema":"lessons/1","lessons":[{"id":"npm-ci","tier":"proven","text":"npm ci before npm test","since_unix_ms":1}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := rpTree(t, filepath.Join(e.home, "learn"))
	res := e.run("", nil, "report", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report: exit %d, %s", res.exitCode, res.stderr)
	}
	for _, want := range []string{"    lessons: the ledger did not check, so all 1 read proposed (this project)\n",
		"the lesson ledger did not check, so every lesson reads proposed (learn)"} {
		if !strings.Contains(strings.Join(strings.Fields(res.stdout), " "), strings.Join(strings.Fields(want), " ")) {
			t.Errorf("the report lacks %q:\n%s", want, res.stdout)
		}
	}
	if after := rpTree(t, filepath.Join(e.home, "learn")); after != before {
		t.Errorf("the report wrote the ledger:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

// TestRP2x_TheStartupLineNamesTheLastSession: a session that starts in a
// project whose last session cut a checkpoint is shown "last session: <time>,
// <reason>; <pointer>" in place of the verify line, one systemMessage and no
// context for the model. The plugin's entry points at the plugin's command by
// the name its manifest declares, a settings install at the CLI. Break: print
// it as additional context, or point a settings install at a slash command.
func TestRP2x_TheStartupLineNamesTheLastSession(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	start := func(t *testing.T, bin string, marker ...string) (string, string) {
		t.Helper()
		res := e.runBin(bin, e.sessionPayload("start", "sess-2"), append([]string{"probe", "start"}, marker...)...)
		if res.exitCode != 0 {
			t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal([]byte(res.stdout), &out); err != nil || len(out) != 1 {
			t.Fatalf("stdout %q: want one systemMessage object (%v)", res.stdout, err)
		}
		var msg string
		if err := json.Unmarshal(out["systemMessage"], &msg); err != nil {
			t.Fatal(err)
		}
		_, line, ok := strings.Cut(msg, "\n  last session: ")
		if !ok || strings.Contains(msg, "verify:") {
			t.Fatalf("no last-session line in place of the verify line: %q", msg)
		}
		return strings.TrimRight(line, "\n"), msg
	}
	line, msg := start(t, rashomonBin, e.installArgs()...)
	if !strings.HasSuffix(line, "; rashomon report --session last") || !strings.Contains(line, ", record green; ") || len("  last session: "+line) > 80 {
		t.Errorf("settings install: %q", line)
	}
	if n := strings.Count(msg, "\n"); n > 8 {
		t.Errorf("the brief is %d lines after its first, want at most 8", n)
	}
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)
	if line, _ := start(t, fx.bin); !strings.HasSuffix(line, "; /rashomon:report last") || len("  last session: "+line) > 80 {
		t.Errorf("plugin: %q, want the command by the manifest's name (rashomon)", line)
	}
}

// TestRP2y_WhyExplainsOneNoteAndWritesNothing: `why <prefix>` prints the
// note's pattern and key, its state, the rows it cites and the gate it came
// from, with the session defaulting to the project's last; --json is the same
// view; --redact drops the texts; and no file under the store changes.
func TestRP2y_WhyExplainsOneNoteAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	before := rpTree(t, e.home)
	res := e.run("", nil, "why", "st-pro")
	if res.exitCode != 0 {
		t.Fatalf("why: exit %d, %s", res.exitCode, res.stderr)
	}
	for _, want := range []string{"rashomon why st-pro (session " + testSession + ", line st-prompt-1)\n", "  what: regression location, key ",
		"  state: delivered", "  evidence:\n", "  source: gate go (slot gate)\n"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("why lacks %q:\n%s", want, res.stdout)
		}
	}
	js := e.run("", nil, "why", "--json", "--redact", "--session", testSession, "st-prompt")
	var v map[string]any
	if js.exitCode != 0 || json.Unmarshal([]byte(js.stdout), &v) != nil || v["redacted"] != true || v["model_text"] != "" || v["line_id"] != "st-prompt-1" {
		t.Errorf("why --json --redact: exit %d, %s\n%s", js.exitCode, js.stderr, js.stdout)
	}
	for _, args := range [][]string{{"why"}, {"why", "abc"}, {"why", "zzzzzz"}, {"why", "--bogus", "st-prompt"}} {
		if r := e.run("", nil, args...); r.exitCode == 0 {
			t.Errorf("%v exited 0, want a refusal", args)
		}
	}
	if after := rpTree(t, e.home); after != before {
		t.Error("why changed a file under the store")
	}
}
