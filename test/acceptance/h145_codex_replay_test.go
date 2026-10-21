package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// H-145 -- Codex's own payloads, replayed through the real hook entry points
// with --harness codex, record what Codex said and nothing it did not.
//
// The fixtures are a real capture (test/fixtures/codex/README.md). Four
// claims, each against the store the replay leaves behind:
//
//   - every PreToolUse is a declaration;
//   - every PostToolUse is an execution with no outcome -- including
//     `python3 missing.py`, which failed while its payload read exactly like a
//     success. Recording it ok is the failure this item exists for;
//   - the subagent's call carries its agent_id;
//   - calls group into the turn the user's prompt opened, the subagent's call
//     included, though Codex gave the subagent a turn_id of its own.
//
// Around them: under Codex the coverage reads none of Claude Code's
// configuration, even with our Claude Code entry installed beside it; recap
// prints nothing; an unknown --harness writes nothing; and no tool output
// reaches the store.
//
// SubagentStart and SubagentStop have no entry point, and are skipped and
// counted rather than dropped silently.
func TestH145_CodexPayloadsRecordWhatCodexSaidAndNothingItDidNot(t *testing.T) {
	const (
		session  = "01a15f65-82dd-7020-afc6-a28414a29cad"
		missing  = "exec-8d96742b-8c96-4f9f-9be0-4900185974e5"
		subCall  = "exec-ea7e7830-f91f-4383-8278-eb7810bb4d9d"
		subAgent = "01a15f66-a3c9-7722-bebe-d5fb9eaede6b"
		subTurn  = "01a15f66-a3d4-7842-930f-be17ec7930e9"
		addTurn  = "01a15f65-b632-7451-a5d4-d6ba306e58dd"
		runTurn  = "01a15f66-2117-7803-9779-763b28829ce9"
		failTurn = "01a15f66-54ba-73c2-9c2e-7418294e1e9c"
		subagent = "01a15f66-97f9-75a1-9c9f-ae79fa458dc5"
	)
	wantTurn := map[string]string{
		"exec-3517123b-6ae8-4db9-af45-24db0b10d8a6": addTurn,
		"exec-8fa2e41b-2d8c-4a4f-9dbf-bed4316e9276": addTurn,
		"exec-9b09c866-dfba-499c-8892-00482b8b37e1": runTurn,
		missing:                                 failTurn,
		"call_4da41e9ca7c649b4a9831d1f33439b00": subagent,
		"call_6518bddb110c4788a05e1ff98266cae2": subagent,
		subCall:                                 subagent,
	}

	e := newEnv(t)
	// Our Claude Code entry is installed on this machine too: the coverage
	// below must not borrow it.
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	codex := append([]string{"--harness", "codex"}, e.installArgs()...)

	paths, err := filepath.Glob(filepath.Join(moduleRoot, "test", "fixtures", "codex", "payloads", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(paths, func(i, j int) bool { return fixtureSeq(paths[i]) < fixtureSeq(paths[j]) })
	if len(paths) != 28 {
		t.Fatalf("found %d fixtures, want the 28 the capture holds", len(paths))
	}

	var pre, post, skipped int
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		event, _, _ := strings.Cut(filepath.Base(path), "-")
		var args []string
		switch event {
		case "SessionStart":
			args = []string{"probe", "start"}
		case "PreToolUse":
			args, pre = []string{"hook"}, pre+1
		case "PostToolUse":
			args, post = []string{"post"}, post+1
		case "UserPromptSubmit":
			args = []string{"recap", "pending"}
		case "Stop":
			args = []string{"recap"}
		default:
			skipped++
			continue
		}
		res := e.run(string(body), nil, append(args, codex...)...)
		if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want 0 and silence", filepath.Base(path), res.exitCode, res.stdout, res.stderr)
		}
	}
	if pre != 7 || post != 7 || skipped != 2 {
		t.Fatalf("replayed %d PreToolUse, %d PostToolUse, skipped %d; want 7, 7, 2", pre, post, skipped)
	}

	decls := e.declarations(session)
	if len(decls) != pre {
		t.Fatalf("%d declarations, want one per PreToolUse (%d)", len(decls), pre)
	}
	for _, d := range decls {
		id := d.str("tool_use_id")
		if got := d.str("prompt_id"); got != wantTurn[id] {
			t.Errorf("%s: prompt_id %q, want %q", id, got, wantTurn[id])
		}
		if id == subCall && d.str("agent_id") != subAgent {
			t.Errorf("the subagent's call carries agent_id %q, want %q", d.str("agent_id"), subAgent)
		}
		if id != subCall && d.fields["agent_id"] != nil {
			t.Errorf("%s: a main-agent call carries agent_id %v", id, d.fields["agent_id"])
		}
		if d.str("prompt_id") == subTurn {
			t.Errorf("%s: recorded under the subagent's own turn", id)
		}
	}

	execs := e.executions(session)
	if len(execs) != post {
		t.Fatalf("%d executions, want one per PostToolUse (%d)", len(execs), post)
	}
	sawMissing := false
	for _, x := range execs {
		id := x.str("tool_use_id")
		sawMissing = sawMissing || id == missing
		if x.str("outcome") != "" || x.fields["exit_code"] != nil || x.fields["is_interrupt"] != nil {
			t.Errorf("%s: outcome %q, exit %v, interrupt %v; want none of them observed",
				id, x.str("outcome"), x.fields["exit_code"], x.fields["is_interrupt"])
		}
		if x.str("executed_digest") == "" {
			t.Errorf("%s: no executed digest; the record dropped what Codex did say", id)
		}
	}
	if !sawMissing {
		t.Error("no execution for python3 missing.py")
	}

	// The turn the subagent was spawned in counts it, and nothing reads ok.
	d := e.digest("--session", session, "--prompt", subagent)
	if d.Declarations.Recorded != 3 || d.Subagents.Declarations != 1 || d.SilentFailures.Unobserved != 3 {
		t.Errorf("subagent turn: %d declarations, %d subagent, %d unobserved; want 3, 1, 3",
			d.Declarations.Recorded, d.Subagents.Declarations, d.SilentFailures.Unobserved)
	}
	var rep struct {
		Sessions []struct {
			SilentFailures struct {
				Failed     int `json:"failed"`
				Unobserved int `json:"outcome_unobserved"`
			} `json:"silent_failures"`
		} `json:"sessions"`
	}
	res := e.run("", nil, "report", "--json", "--session", session)
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
		t.Fatalf("report: exit %d, %v\n%s", res.exitCode, err, res.stdout)
	}
	if sf := rep.Sessions[0].SilentFailures; sf.Unobserved != post || sf.Failed != 0 {
		t.Errorf("report: %d unobserved, %d failed; want %d and 0", sf.Unobserved, sf.Failed, post)
	}

	cov := e.read(session, "coverage.ndjson")
	if len(cov) == 0 {
		t.Fatal("no coverage records")
	}
	for _, c := range cov {
		if c.str("hook_entry") != "unknown" || c.str("state") != "unverified" || c.str("reason") != "hook_entry_unresolved" {
			t.Errorf("coverage %s: hook_entry %q, state %q, reason %q; want unknown, unverified, hook_entry_unresolved",
				c.str("phase"), c.str("hook_entry"), c.str("state"), c.str("reason"))
		}
	}

	// Tool output from three of the PostToolUse payloads, and the subagent's
	// encrypted spawn message: none of it is a value the store may hold.
	for path, f := range walkStore(t, e.home) {
		for _, canary := range []string{"Return the sum of two numbers", "No such file or directory", "capture.py", "gAAAAABq2O9E"} {
			if strings.Contains(string(f.body), canary) {
				t.Errorf("%s holds %q from a payload", path, canary)
			}
		}
	}

	before := walkStore(t, e.home)
	res = e.run(string(mustRead(t, paths[5])), nil, append([]string{"hook", "--harness", "cursor"}, e.installArgs()...)...)
	if res.exitCode != 0 || res.stderr != "rashomon: unknown harness; standing down\n" {
		t.Errorf("unknown harness: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	after := walkStore(t, e.home)
	if len(after) != len(before) {
		t.Errorf("unknown harness changed the store: %d entries before, %d after", len(before), len(after))
	}
	for path, f := range before {
		if string(after[path].body) != string(f.body) {
			t.Errorf("unknown harness rewrote %s", path)
		}
	}
}

// fixtureSeq is a fixture's place in the capture: the NN of <event>-<NN>.json.
func fixtureSeq(path string) string {
	_, seq, _ := strings.Cut(filepath.Base(path), "-")
	return seq
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
