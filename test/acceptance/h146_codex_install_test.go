package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// H-146 -- watch --harness codex writes our entries into Codex's hooks.json,
// detach --harness codex removes exactly them, and status says what is there.
//
// The entries are judged by running them: each command line watch wrote is
// run through a shell with the capture's payloads, as Codex would run it, and
// the store must hold what H-145's direct replay holds. None of them may print
// on stdout, because Codex adds a SessionStart hook's stdout to the agent's
// context.

// codexSeed is a hooks.json someone already has: a description, a hook of
// their own under an event we install into, and one under an event we do not.
const codexSeed = `{
  "description": "my hooks",
  "hooks": {
    "PreToolUse": [{"matcher":"Bash","hooks":[{"type":"command","command":"echo foreign-sentinel","timeout":30}]}],
    "Stop": [{"hooks":[{"type":"command","command":"notify-me"}]}]
  }
}
`

var codexWant = map[string]struct {
	sub     string
	matcher bool
}{
	"PreToolUse":   {"hook", true},
	"PostToolUse":  {"post", true},
	"SessionStart": {"probe start", false},
}

func (e *env) codexPath() string { return filepath.Join(e.codexDir, "hooks.json") }

func (e *env) makeCodexDir(seed string) {
	e.t.Helper()
	if err := os.MkdirAll(e.codexDir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if seed != "" {
		if err := os.WriteFile(e.codexPath(), []byte(seed), 0o644); err != nil {
			e.t.Fatal(err)
		}
	}
}

func (e *env) codexBytes() []byte {
	e.t.Helper()
	data, err := os.ReadFile(e.codexPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return data
}

func (e *env) codexHooks() map[string][]matcherGroup {
	e.t.Helper()
	var sf settingsFile
	if err := json.Unmarshal(e.codexBytes(), &sf); err != nil {
		e.t.Fatalf("hooks.json is not valid JSON: %v\n%s", err, e.codexBytes())
	}
	return sf.Hooks
}

func (e *env) watchCodex(extraEnv ...string) result {
	e.t.Helper()
	return e.run("", extraEnv, "watch", "--harness", "codex")
}

func (e *env) detachCodex(args ...string) result {
	e.t.Helper()
	return e.run("", nil, append([]string{"detach", "--harness", "codex"}, args...)...)
}

func canonicalJSON(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%v:\n%s", err, b)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

func TestH146_WatchCodexInstallsEntriesThatRecordWhenRun(t *testing.T) {
	e := newEnv(t)
	e.makeCodexDir(codexSeed)

	res := e.watchCodex()
	if res.exitCode != 0 {
		t.Fatalf("watch --harness codex: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	id := e.installID()
	for _, want := range []string{
		"watching Codex (install " + id + ") via " + e.codexPath(),
		"run /hooks, and trust the rashomon entries for PreToolUse, PostToolUse, SessionStart. Until then nothing is recorded.",
		"coverage: unverified (hook_entry_unresolved), trusted or not",
		"not checked: whether hooks = false under [features]",
		"rashomon detach --harness codex --install " + id + "\n",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("watch output does not say %q:\n%s", want, res.stdout)
		}
	}
	if e.settingsBytes() != nil {
		t.Errorf("watch --harness codex wrote Claude Code's settings:\n%s", e.settingsBytes())
	}

	hooks := e.codexHooks()
	lines := map[string]string{}
	for event, groups := range hooks {
		mine := ours(groups)
		want, installed := codexWant[event]
		if !installed {
			if len(mine) != 0 {
				t.Errorf("%s: %d rashomon entries, want none", event, len(mine))
			}
			continue
		}
		if len(mine) != 1 || len(mine[0].Hooks) != 1 {
			t.Errorf("%s: %d rashomon entries, want exactly one with one hook", event, len(mine))
			continue
		}
		h := mine[0].Hooks[0]
		if !strings.HasSuffix(h.Command, " "+want.sub+" --harness codex --install "+id) || h.Type != "command" || h.Timeout != 5 {
			t.Errorf("%s: %+v, want a command ending %q with timeout 5", event, h, want.sub+" --harness codex --install "+id)
		}
		if got := mine[0].Matcher != nil && *mine[0].Matcher == "*"; got != want.matcher {
			t.Errorf("%s: matcher %v, want \"*\" only on the tool events", event, mine[0].Matcher)
		}
		lines[event] = h.Command
	}
	if len(lines) != len(codexWant) {
		t.Fatalf("installed %v, want one entry each for %v", lines, codexWant)
	}
	if !strings.Contains(string(e.codexBytes()), `{"matcher":"Bash","hooks":[{"type":"command","command":"echo foreign-sentinel","timeout":30}]}`) {
		t.Errorf("the user's own hook did not survive byte for byte:\n%s", e.codexBytes())
	}

	// Run what was installed, the way Codex runs it.
	paths, err := filepath.Glob(filepath.Join(moduleRoot, "test", "fixtures", "codex", "payloads", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(paths, func(i, j int) bool { return fixtureSeq(paths[i]) < fixtureSeq(paths[j]) })
	ran := map[string]int{}
	for _, path := range paths {
		event, _, _ := strings.Cut(filepath.Base(path), "-")
		line, ok := lines[event]
		if !ok {
			continue
		}
		ran[event]++
		res := e.sh(line, string(mustRead(t, path)))
		if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q; want 0 and silence", filepath.Base(path), res.exitCode, res.stdout, res.stderr)
		}
	}
	if ran["PreToolUse"] != 7 || ran["PostToolUse"] != 7 || ran["SessionStart"] != 2 {
		t.Fatalf("ran %v, want 7 PreToolUse, 7 PostToolUse and 2 SessionStart", ran)
	}
	const session = "01a15f65-82dd-7020-afc6-a28414a29cad"
	if got := len(e.declarations(session)); got != 7 {
		t.Errorf("%d declarations, want 7", got)
	}
	execs := e.executions(session)
	if len(execs) != 7 {
		t.Errorf("%d executions, want 7", len(execs))
	}
	for _, x := range execs {
		if x.str("outcome") != "" {
			t.Errorf("%s: outcome %q; under Codex no outcome is observed", x.str("tool_use_id"), x.str("outcome"))
		}
	}
	for _, c := range e.coverage(session, "start") {
		if c.str("state") != "unverified" || c.str("reason") != "hook_entry_unresolved" {
			t.Errorf("start coverage %s / %s, want unverified / hook_entry_unresolved", c.str("state"), c.str("reason"))
		}
	}

	// A second watch changes nothing, so nothing has to be trusted again.
	before := e.codexBytes()
	res = e.watchCodex()
	if res.exitCode != 0 || !strings.Contains(res.stdout, "already watching Codex") {
		t.Errorf("second watch: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	if strings.Contains(res.stdout, "rewritten") {
		t.Errorf("second watch says it rewrote entries:\n%s", res.stdout)
	}
	if !bytes.Equal(e.codexBytes(), before) {
		t.Errorf("second watch moved hooks.json:\n%s\n%s", before, e.codexBytes())
	}

	res = e.detachCodex()
	if res.exitCode != 0 || !strings.Contains(res.stdout, "detached from Codex (install "+id+", 3 entries)") {
		t.Fatalf("detach --harness codex: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	if got, want := canonicalJSON(t, e.codexBytes()), canonicalJSON(t, []byte(codexSeed)); got != want {
		t.Errorf("detach did not leave hooks.json as found:\n%s\nwant\n%s", got, want)
	}
}

// TestH146_WatchCodexSaysAMovedBinaryNeedsTrustAgain: Codex trusts an entry by
// its hash, and a moved binary changes every command line.
func TestH146_WatchCodexSaysAMovedBinaryNeedsTrustAgain(t *testing.T) {
	e := newEnv(t)
	e.makeCodexDir("")
	if res := e.watchCodex(); res.exitCode != 0 {
		t.Fatalf("watch: %q", res.stderr)
	}
	moved := strings.ReplaceAll(string(e.codexBytes()), rashomonBin, "/old/place/rashomon")
	if err := os.WriteFile(e.codexPath(), []byte(moved), 0o644); err != nil {
		t.Fatal(err)
	}
	res := e.watchCodex()
	if res.exitCode != 0 || !strings.Contains(res.stdout, "Codex treats a rewritten entry as new") {
		t.Errorf("watch over a moved binary: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	if strings.Contains(string(e.codexBytes()), "/old/place/") {
		t.Errorf("the moved binary's entries were not refreshed:\n%s", e.codexBytes())
	}
}

// TestH146_WatchCodexRefusesWithoutCodexDirectory: no directory is created, no
// store is minted, and the message says where it looked and what to do.
func TestH146_WatchCodexRefusesWithoutCodexDirectory(t *testing.T) {
	e := newEnv(t)
	storeBefore := walkStore(t, e.home)
	res := e.watchCodex()
	if res.exitCode != 1 || !strings.Contains(res.stderr, "no Codex directory at "+e.codexDir) || !strings.Contains(res.stderr, "CODEX_HOME") {
		t.Errorf("exit %d, stderr %q; want a refusal naming %s and CODEX_HOME", res.exitCode, res.stderr, e.codexDir)
	}
	if _, err := os.Stat(e.codexDir); !os.IsNotExist(err) {
		t.Errorf("the refusal created %s", e.codexDir)
	}
	if got := walkStore(t, e.home); !reflect.DeepEqual(got, storeBefore) {
		t.Errorf("the refusal wrote the store: %v", got)
	}

	// detach on the same machine finds nothing, and creates nothing either.
	res = e.detachCodex("--all")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "nothing to detach from Codex") {
		t.Errorf("detach --all: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	if _, err := os.Stat(e.codexDir); !os.IsNotExist(err) {
		t.Errorf("detach created %s", e.codexDir)
	}
}

// TestH146_UnknownHarnessIsRefused: a person typed it, so it is an error that
// names the value, and nothing is written.
func TestH146_UnknownHarnessIsRefused(t *testing.T) {
	e := newEnv(t)
	e.makeCodexDir("")
	storeBefore := walkStore(t, e.home)
	for _, args := range [][]string{
		{"watch", "--harness", "cursor"},
		{"detach", "--harness", "cursor", "--all"},
		{"watch", "--harness"},
		{"watch", "--harness", "codex", "--all"},
	} {
		res := e.run("", nil, args...)
		if res.exitCode != 1 || res.stdout != "" {
			t.Errorf("%v: exit %d, stdout %q; want 1 and nothing on stdout", args, res.exitCode, res.stdout)
		}
		if args[len(args)-1] == "cursor" && !strings.Contains(res.stderr, `unknown harness "cursor"`) {
			t.Errorf("%v: stderr %q does not name the harness", args, res.stderr)
		}
	}
	if e.codexBytes() != nil || e.settingsBytes() != nil || !reflect.DeepEqual(walkStore(t, e.home), storeBefore) {
		t.Errorf("a refused command wrote something")
	}
}

// TestH146_DetachCodexRules: --install leaves another install's entries,
// --all needs no store, an edited entry is refused until --force, and each
// harness's detach leaves the other harness's file alone.
func TestH146_DetachCodexRules(t *testing.T) {
	e := newEnv(t)
	e.makeCodexDir(codexSeed)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: %q", res.stderr)
	}
	if res := e.watchCodex(); res.exitCode != 0 {
		t.Fatalf("watch --harness codex: %q", res.stderr)
	}
	id := e.installID()
	claude := e.settingsBytes()

	// Plain detach is Claude Code's, and leaves Codex's file alone.
	codexBefore := e.codexBytes()
	if res := e.detach(); res.exitCode != 0 {
		t.Fatalf("detach: %q", res.stderr)
	}
	if !bytes.Equal(e.codexBytes(), codexBefore) {
		t.Errorf("plain detach changed hooks.json")
	}
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch again: %q", res.stderr)
	}
	claude = e.settingsBytes()

	if res := e.detachCodex("--install", strings.Repeat("f", 32)); res.exitCode != 0 || !strings.Contains(res.stdout, "nothing to detach from Codex") {
		t.Errorf("detach of another install: exit %d, stdout %q", res.exitCode, res.stdout)
	}
	if !bytes.Equal(e.codexBytes(), codexBefore) {
		t.Errorf("detach of another install changed hooks.json")
	}

	edited := strings.Replace(string(codexBefore), `"timeout": 5`, `"timeout": 30`, 1)
	if err := os.WriteFile(e.codexPath(), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	res := e.detachCodex()
	if res.exitCode != 1 || !strings.Contains(res.stderr, "timeout is 30, expected 5") {
		t.Errorf("detach over an edited entry: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if string(e.codexBytes()) != edited {
		t.Errorf("a refusing detach changed hooks.json")
	}

	// With the store gone, --all and --force remove every entry of ours.
	if err := os.RemoveAll(e.home); err != nil {
		t.Fatal(err)
	}
	res = e.detachCodex("--all", "--force")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "detached from Codex (every install, 3 entries)") ||
		!strings.Contains(res.stdout, "removed the Codex PreToolUse entry you had edited") {
		t.Errorf("detach --all --force: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	if got, want := canonicalJSON(t, e.codexBytes()), canonicalJSON(t, []byte(codexSeed)); got != want {
		t.Errorf("hooks.json after detach --all:\n%s\nwant\n%s", got, want)
	}
	if _, err := os.Stat(e.home); !os.IsNotExist(err) {
		t.Errorf("detach --all created a store")
	}
	if !bytes.Equal(e.settingsBytes(), claude) {
		t.Errorf("detach --harness codex changed Claude Code's settings")
	}
	if !strings.Contains(string(claude), " --install "+id) {
		t.Errorf("Claude Code's entries are missing:\n%s", claude)
	}
}

// TestH146_WatchCodexDefaultsToHomeDotCodex, with HOME a throwaway: the real
// ~/.codex of whoever runs the suite is never reached.
func TestH146_WatchCodexDefaultsToHomeDotCodex(t *testing.T) {
	e := newEnv(t)
	home := t.TempDir()
	dir := filepath.Join(home, ".codex")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "CODEX_HOME="}
	if res := e.watchCodex(env...); res.exitCode != 0 || !strings.Contains(res.stdout, filepath.Join(dir, "hooks.json")) {
		t.Fatalf("watch: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if err != nil || !strings.Contains(string(data), "--harness codex --install") {
		t.Errorf("~/.codex/hooks.json: %v\n%s", err, data)
	}
	if e.codexBytes() != nil {
		t.Errorf("wrote %s although CODEX_HOME was unset", e.codexPath())
	}
}
