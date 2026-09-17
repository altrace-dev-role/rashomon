package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestH3_InstalledEntryReadBackInFull reads the file back through a generic
// decoder -- the only reader whose opinion matters -- and checks every field
// the spec names, plus the two probe entries watch installs beside the recorder.
func TestH3_InstalledEntryReadBackInFull(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	sf := e.settings()

	// Under hooks.PreToolUse: not a sibling event, not a typo.
	for event := range sf.Hooks {
		switch event {
		case "PreToolUse", "SessionStart", "SessionEnd":
		default:
			t.Errorf("watch wrote an entry under hooks.%s", event)
		}
	}
	groups := ours(sf.Hooks["PreToolUse"])
	if len(groups) != 1 {
		t.Fatalf("got %d entries of ours under hooks.PreToolUse, want 1", len(groups))
	}
	g := groups[0]
	if g.Matcher == nil || *g.Matcher != "*" {
		t.Errorf("matcher is %v, want \"*\"", g.Matcher)
	}
	if len(g.Hooks) != 1 {
		t.Fatalf("entry carries %d hooks, want 1", len(g.Hooks))
	}
	h := g.Hooks[0]
	if h.Type != "command" {
		t.Errorf("type is %q, want \"command\"", h.Type)
	}
	if h.Timeout != 5 {
		t.Errorf("timeout is %d, want 5", h.Timeout)
	}
	if want := " hook --install " + e.installID(); !strings.HasSuffix(h.Command, want) {
		t.Errorf("command %q does not end with %q", h.Command, want)
	}

	// The command path resolves to a file that exists and is executable.
	exe := strings.Fields(h.Command)[0]
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("command path %q does not resolve: %v", exe, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("command path %q is not executable (mode %04o)", exe, info.Mode().Perm())
	}
	wantExe, _ := filepath.EvalSymlinks(attestBin)
	if exe != wantExe {
		t.Errorf("command path is %q, want the binary under test %q", exe, wantExe)
	}

	// The probe, installed alongside.
	for _, event := range []string{"SessionStart", "SessionEnd"} {
		pg := ours(sf.Hooks[event])
		if len(pg) != 1 {
			t.Errorf("got %d entries of ours under hooks.%s, want 1", len(pg), event)
			continue
		}
		if pg[0].Matcher != nil {
			t.Errorf("%s entry has matcher %q; session events match on source, not tool, and must not carry one", event, *pg[0].Matcher)
		}
		if len(pg[0].Hooks) != 1 || pg[0].Hooks[0].Timeout != 5 || pg[0].Hooks[0].Type != "command" {
			t.Errorf("%s entry is %+v, want one command hook with timeout 5", event, pg[0].Hooks)
		}
	}
}

// foreignEntry is written with deliberately irregular spacing and with its keys
// in an order no encoder would choose. If the file has been through an encoder,
// these bytes will not survive.
const foreignEntry = `{"hooks":  [ {"type":"command",   "command": "echo foreign-sentinel",
        "timeout":30} ],  "matcher":"Bash"}`

// TestH4_ForeignHooksSurvive seeds the arrangement that actually occurs -- a
// foreign entry under a narrow matcher -- and checks it is untouched, byte for
// byte, by both watch and detach. This catches an installer that assigns where
// it should append.
func TestH4_ForeignHooksSurvive(t *testing.T) {
	e := newEnv(t)
	e.writeSettings(`{"hooks": {"PreToolUse": [` + foreignEntry + `]}}` + "\n")

	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !bytes.Contains(e.settingsBytes(), []byte(foreignEntry)) {
		t.Errorf("after watch, the foreign entry is not byte-identical:\n%s", e.settingsBytes())
	}
	sf := e.settings()
	if got := len(sf.Hooks["PreToolUse"]); got != 2 {
		t.Errorf("after watch, hooks.PreToolUse has %d entries, want 2 (foreign + ours)", got)
	}
	if got := len(ours(sf.Hooks["PreToolUse"])); got != 1 {
		t.Errorf("after watch, %d entries of ours, want 1", got)
	}

	if res := e.detach(); res.exitCode != 0 {
		t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !bytes.Contains(e.settingsBytes(), []byte(foreignEntry)) {
		t.Errorf("after detach, the foreign entry is not byte-identical:\n%s", e.settingsBytes())
	}
	sf = e.settings()
	if got := len(sf.Hooks["PreToolUse"]); got != 1 {
		t.Errorf("after detach, hooks.PreToolUse has %d entries, want 1 (the foreign one)", got)
	}
	if got := len(ours(sf.Hooks["PreToolUse"])); got != 0 {
		t.Errorf("after detach, %d entries of ours remain", got)
	}
}

// TestH5_FirstRunOnACleanMachine: absent, empty, and present-without-hooks are
// all machines that have not run this program before, and the second and third
// are machines that have run Claude Code before. All three must work.
func TestH5_FirstRunOnACleanMachine(t *testing.T) {
	const permissions = `{"allow": ["Bash(ls:*)", "Read(/tmp/**)"]}`
	cases := []struct {
		name string
		seed *string
	}{
		{"absent", nil},
		{"empty", ptr("")},
		{"present without a hooks key", ptr(`{"permissions": ` + permissions + `}` + "\n")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			if c.seed != nil {
				e.writeSettings(*c.seed)
			}
			if res := e.watch(); res.exitCode != 0 {
				t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
			}
			sf := e.settings() // fatal if not valid JSON
			if got := len(ours(sf.Hooks["PreToolUse"])); got != 1 {
				t.Errorf("%d entries of ours under hooks.PreToolUse, want 1", got)
			}
			if c.seed != nil && strings.Contains(*c.seed, "permissions") &&
				!bytes.Contains(e.settingsBytes(), []byte(permissions)) {
				t.Errorf("the permissions value was not carried over byte for byte:\n%s", e.settingsBytes())
			}
		})
	}
}

func ptr(s string) *string { return &s }

// TestH6_IdempotentInstallOneOwner: watch twice installs one entry; detach
// removes it when no other install claims one, and only its own otherwise.
func TestH6_IdempotentInstallOneOwner(t *testing.T) {
	t.Run("watch twice then detach", func(t *testing.T) {
		e := newEnv(t)
		e.watch()
		first := e.settingsBytes()
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("second watch: exit %d", res.exitCode)
		}
		if !bytes.Equal(first, e.settingsBytes()) {
			t.Errorf("a second watch changed the file")
		}
		sf := e.settings()
		for _, event := range []string{"PreToolUse", "SessionStart", "SessionEnd"} {
			if got := len(ours(sf.Hooks[event])); got != 1 {
				t.Errorf("after two watches, %d entries of ours under %s, want 1", got, event)
			}
		}

		if res := e.detach(); res.exitCode != 0 {
			t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		sf = e.settings()
		for _, event := range []string{"PreToolUse", "SessionStart", "SessionEnd"} {
			if got := len(ours(sf.Hooks[event])); got != 0 {
				t.Errorf("after detach, %d entries of ours remain under %s", got, event)
			}
		}
	})

	t.Run("detach removes only its own claim", func(t *testing.T) {
		e := newEnv(t)
		e.watch()

		// Another install's entries, as another install would write them.
		const other = "ffffffffffffffffffffffffffffffff"
		otherEntry := func(sub string) map[string]any {
			m := map[string]any{"hooks": []map[string]any{{
				"type": "command", "command": attestBin + " " + sub + " --install " + other, "timeout": 5,
			}}}
			if sub == "hook" {
				m["matcher"] = "*"
			}
			return m
		}
		var doc map[string]any
		if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
			t.Fatal(err)
		}
		hooks := doc["hooks"].(map[string]any)
		for event, sub := range map[string]string{"PreToolUse": "hook", "SessionStart": "probe start", "SessionEnd": "probe end"} {
			hooks[event] = append(hooks[event].([]any), otherEntry(sub))
		}
		out, _ := json.MarshalIndent(doc, "", "  ")
		e.writeSettings(string(out) + "\n")

		// Our own watch still sees exactly one of ours, not two.
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch with another install present: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if res := e.detach(); res.exitCode != 0 {
			t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
		}

		sf := e.settings()
		for _, event := range []string{"PreToolUse", "SessionStart", "SessionEnd"} {
			remaining := ours(sf.Hooks[event])
			if len(remaining) != 1 {
				t.Errorf("under %s, %d install entries remain, want 1 (the other install's)", event, len(remaining))
				continue
			}
			if !strings.HasSuffix(remaining[0].Hooks[0].Command, " --install "+other) {
				t.Errorf("under %s, the remaining entry is %q, want the other install's", event, remaining[0].Hooks[0].Command)
			}
		}
	})
}

// TestH7_DetachPreservesConcurrentEdits: Claude Code rewrites the whole file
// when the user accepts "always allow". detach must still find its entry in the
// rewritten file, remove it, and leave the new permission standing.
func TestH7_DetachPreservesConcurrentEdits(t *testing.T) {
	e := newEnv(t)
	e.watch()

	// As Claude Code does it: decode, mutate, re-encode the whole file.
	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	doc["permissions"] = map[string]any{"allow": []string{"Bash(git log:*)"}}
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")

	res := e.detach()
	if res.exitCode != 0 {
		t.Fatalf("detach aborted: exit %d, stderr %q; abort is reserved for a change inside our own entry", res.exitCode, res.stderr)
	}
	var after struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(e.settingsBytes(), &after); err != nil {
		t.Fatalf("settings.json is not valid JSON after detach: %v", err)
	}
	if len(after.Permissions.Allow) != 1 || after.Permissions.Allow[0] != "Bash(git log:*)" {
		t.Errorf("the concurrent permissions edit did not survive detach: %v", after.Permissions.Allow)
	}
	if got := len(ours(e.settings().Hooks["PreToolUse"])); got != 0 {
		t.Errorf("%d entries of ours remain after detach", got)
	}
}

// TestH7_DetachSaysSoWhenItCannot: a change inside our entry's region is the
// one thing detach refuses over, and it says which field.
func TestH7_DetachSaysSoWhenItCannot(t *testing.T) {
	e := newEnv(t)
	e.watch()

	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	pre := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	pre[0].(map[string]any)["matcher"] = "Bash" // narrowed by hand
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")
	before := e.settingsBytes()

	res := e.detach()
	if res.exitCode == 0 {
		t.Fatalf("detach removed an entry that had been edited; it must refuse and say why")
	}
	if !strings.Contains(res.stderr, "matcher") {
		t.Errorf("detach refused without naming the field: %q", res.stderr)
	}
	if !bytes.Equal(before, e.settingsBytes()) {
		t.Errorf("a refusing detach still changed the file")
	}
}

// TestH8_AtomicWriteWithTheWindowForced injects a deterministic fault inside
// the write, at the two instants that distinguish an atomic writer from one
// that edits in place, and asserts the trichotomy: the file is always exactly
// the pre-state or exactly the post-state. The loop is asserted to have seen
// both, or it never exercised the window.
//
// fsync is required and is review-only: a process kill cannot verify it,
// because the page cache survives the process.
func TestH8_AtomicWriteWithTheWindowForced(t *testing.T) {
	e := newEnv(t)
	seed := `{"permissions": {"allow": ["Bash(ls:*)"]}, "hooks": {"PreToolUse": [` + foreignEntry + `]}}` + "\n"

	e.writeSettings(seed)
	pre := e.settingsBytes()
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("clean watch: exit %d", res.exitCode)
	}
	post := e.settingsBytes()
	if bytes.Equal(pre, post) {
		t.Fatal("premise broken: a clean watch did not change the file")
	}

	faults := []string{"", pointSettingsOpened, pointSettingsBeforeRename, "", pointSettingsOpened, pointSettingsBeforeRename}
	seenPre, seenPost := false, false
	for i, point := range faults {
		e.writeSettings(seed)
		var extra []string
		if point != "" {
			extra = []string{"ATTEST_FAULT=" + point + ":plain_panic"}
		}
		res := e.watch(extra...)
		if res.exitCode == 2 {
			t.Errorf("iteration %d: watch exited 2", i)
		}

		got := e.settingsBytes()
		switch {
		case bytes.Equal(got, pre):
			seenPre = true
			if point == "" {
				t.Errorf("iteration %d: a clean watch left the pre-state", i)
			}
		case bytes.Equal(got, post):
			seenPost = true
			if point != "" {
				t.Errorf("iteration %d: a fault at %s still produced the post-state", i, point)
			}
		default:
			t.Errorf("iteration %d (fault %q): the file is neither the pre-state nor the post-state:\n%s", i, point, got)
		}
	}
	if !seenPre || !seenPost {
		t.Fatalf("the loop never exercised the window (saw pre=%v post=%v)", seenPre, seenPost)
	}

	entries, _ := os.ReadDir(e.configDir)
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), ".settings.json.attest-") {
			t.Errorf("a temporary file was left behind: %s", en.Name())
		}
	}
}

// TestH9_RefuseWhenHooksAreDisabledInBothDirections: managed wins over
// project; project wins over user. The second case is the false positive that
// matters -- a resolver that only read the user file would refuse to install on
// a machine where hooks run.
func TestH9_RefuseWhenHooksAreDisabledInBothDirections(t *testing.T) {
	cases := []struct {
		name                          string
		managed, user, project, local string
		wantRefuse                    bool
		wantLayer                     string
	}{
		{"managed true beats project false", `{"disableAllHooks": true}`, "", `{"disableAllHooks": false}`, "", true, "managed"},
		{"project false beats user true", "", `{"disableAllHooks": true}`, `{"disableAllHooks": false}`, "", false, ""},
		{"local false beats project true", "", "", `{"disableAllHooks": true}`, `{"disableAllHooks": false}`, false, ""},
		{"user true alone refuses", "", `{"disableAllHooks": true}`, "", "", true, "user"},
		{"nothing set proceeds", "", "", "", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			write := func(path, content string) {
				if content == "" {
					return
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write(e.managedPath(), c.managed)
			write(e.settingsPath(), c.user)
			write(filepath.Join(e.cwd, ".claude", "settings.json"), c.project)
			write(filepath.Join(e.cwd, ".claude", "settings.local.json"), c.local)
			before := e.settingsBytes()

			res := e.watch()
			if c.wantRefuse {
				if res.exitCode == 0 {
					t.Fatalf("watch installed although hooks are disabled by the %s layer", c.wantLayer)
				}
				if !strings.Contains(res.stderr, c.wantLayer) {
					t.Errorf("refusal does not name the deciding layer %q: %q", c.wantLayer, res.stderr)
				}
				if !bytes.Equal(before, e.settingsBytes()) {
					t.Errorf("a refusing watch still wrote the settings file")
				}
				return
			}
			if res.exitCode != 0 {
				t.Fatalf("watch refused although hooks run in this arrangement: %q", res.stderr)
			}
			if got := len(ours(e.settings().Hooks["PreToolUse"])); got != 1 {
				t.Errorf("%d entries of ours installed, want 1", got)
			}
		})
	}
}

// TestH7_WatchRefusesAnEntryItNoLongerRecognises: the same rule as detach,
// from the other side. A hook someone added to our group is their work, and
// watch refreshing "its" entry must not delete it.
func TestH7_WatchRefusesAnEntryItNoLongerRecognises(t *testing.T) {
	e := newEnv(t)
	e.watch()

	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	group := doc["hooks"].(map[string]any)["PreToolUse"].([]any)[0].(map[string]any)
	group["hooks"] = append(group["hooks"].([]any), map[string]any{"type": "command", "command": "my-linter"})
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")
	before := e.settingsBytes()

	res := e.watch()
	if res.exitCode == 0 {
		t.Fatalf("watch overwrote a group someone had added a hook to")
	}
	if !strings.Contains(res.stderr, "2 hooks") {
		t.Errorf("refusal does not say what changed: %q", res.stderr)
	}
	if !bytes.Equal(before, e.settingsBytes()) {
		t.Errorf("a refusing watch still changed the file")
	}
}

// TestH3_SpacedExecutablePathRunsAsInstalled follows the installed command line
// all the way to a shell. A path with a space in it is ordinary -- macOS puts
// one in "Application Support" -- and an installer that writes it bare produces
// a configuration that parses, reads plausibly, and records nothing at all.
func TestH3_SpacedExecutablePathRunsAsInstalled(t *testing.T) {
	e := newEnv(t)
	exe := copyBinary(t, attestBin, filepath.Join(t.TempDir(), "Application Support", "attest"))

	if res := e.runBin(exe, "", "watch"); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	groups := ours(e.settings().Hooks["PreToolUse"])
	if len(groups) != 1 {
		t.Fatalf("got %d entries of ours under hooks.PreToolUse, want 1", len(groups))
	}
	line := groups[0].Hooks[0].Command

	fields := shellSplit(t, line)
	if len(fields) == 0 || fields[0] != exe {
		t.Fatalf("the command line %s splits to %q; a shell would run something other than %q", line, fields, exe)
	}

	res := e.sh(line, defaultPayload().build(t))
	if res.exitCode != 0 {
		t.Fatalf("sh -c %s: exit %d, stderr %q", line, res.exitCode, res.stderr)
	}
	if got := len(e.declarations(testSession)); got != 1 {
		t.Errorf("the installed command line recorded %d declarations, want 1", got)
	}
}

// copyBinary places a copy of a binary at dst, creating its directory. The path
// it sits at is the whole point of the copy.
func copyBinary(t *testing.T, src, dst string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(dst), err)
	}
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	if err := os.WriteFile(dst, body, 0o700); err != nil {
		t.Fatalf("writing %s: %v", dst, err)
	}
	resolved, err := filepath.EvalSymlinks(dst)
	if err != nil {
		t.Fatalf("resolving %s: %v", dst, err)
	}
	return resolved
}

// shellSplit splits a command line on whitespace outside single quotes, which
// is as much of a shell as an installed path needs. It is written out here
// rather than borrowed from the tokenizer under test, which would agree with
// whatever that produced.
func shellSplit(t *testing.T, line string) []string {
	t.Helper()
	var (
		fields  []string
		cur     strings.Builder
		quoted  bool
		started bool
	)
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\'':
			quoted = !quoted
			started = true
		case (c == ' ' || c == '\t') && !quoted:
			if started {
				fields = append(fields, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if quoted {
		t.Fatalf("the command line has an unbalanced quote: %s", line)
	}
	if started {
		fields = append(fields, cur.String())
	}
	return fields
}
