package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// The installed command line is an absolute path to the binary, and the install
// id is written down in exactly one place: the store. Both can be deleted, and
// when they are, every tool call in the session errors and the configuration
// still points at a binary that is not there. The recovery forms below are what
// is left at that point, so neither of them may read the store.

// Two installs that are not this machine's, in the shape an install id has.
const (
	installA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	installB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// The events watch installs into, with the subcommand each one runs.
var installedEvents = []struct{ event, sub string }{
	{"PreToolUse", "hook"},
	{"PostToolUse", "post"},
	{"SessionStart", "probe start"},
	{"SessionEnd", "probe end"},
}

// TestH6_DetachByInstallIDNeedsNoStore: the id comes off the terminal, so the
// store is not consulted -- and must not be created, on a machine where it has
// just been deleted. Another install's entries are present throughout, because
// an id that is not used as a filter is not an id.
func TestH6_DetachByInstallIDNeedsNoStore(t *testing.T) {
	e := newEnv(t)
	seedInstalls(e, installA)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	id := e.installID()
	if err := os.RemoveAll(e.home); err != nil {
		t.Fatal(err)
	}

	res := e.run("", nil, "detach", "--install", id)
	if res.exitCode != 0 {
		t.Fatalf("detach --install: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if _, err := os.Stat(e.home); !os.IsNotExist(err) {
		t.Errorf("detach --install created a store at %s (stat: %v)", e.home, err)
	}
	assertInstallEntries(e, id, 0)
	assertInstallEntries(e, installA, 1)
	if !bytes.Contains(e.settingsBytes(), []byte(foreignEntry)) {
		t.Errorf("the foreign entry is not byte-identical:\n%s", e.settingsBytes())
	}
}

// TestH6_DetachAllRemovesEveryInstall: with the store gone the id is unknown,
// and the marker in the command line is all there is left to go on. Everything
// carrying one goes; everything else stays as it was found.
func TestH6_DetachAllRemovesEveryInstall(t *testing.T) {
	e := newEnv(t)
	seedInstalls(e, installA, installB)
	if err := os.RemoveAll(e.home); err != nil {
		t.Fatal(err)
	}

	res := e.run("", nil, "detach", "--all")
	if res.exitCode != 0 {
		t.Fatalf("detach --all: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	assertInstallEntries(e, installA, 0)
	assertInstallEntries(e, installB, 0)
	sf := e.settings()
	for _, ev := range installedEvents {
		if got := len(ours(sf.Hooks[ev.event])); got != 0 {
			t.Errorf("under %s, %d entries carrying an install marker remain", ev.event, got)
		}
	}
	if !bytes.Contains(e.settingsBytes(), []byte(foreignEntry)) {
		t.Errorf("the foreign entry is not byte-identical:\n%s", e.settingsBytes())
	}
	if _, err := os.Stat(e.home); !os.IsNotExist(err) {
		t.Errorf("detach --all created a store at %s (stat: %v)", e.home, err)
	}
}

// TestH6_DetachAllRefusesAnEditedEntry: removing entries whose id is unknown
// widens what is considered, never what may be done to one. The first install's
// entry is intact and is walked past before the edited one is reached, so this
// also asserts that a refusal discards the removals it had already made.
func TestH6_DetachAllRefusesAnEditedEntry(t *testing.T) {
	e := newEnv(t)
	seedInstalls(e, installA, installB)

	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	pre := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	pre[2].(map[string]any)["matcher"] = "Bash" // narrowed by hand
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")
	before := e.settingsBytes()

	res := e.run("", nil, "detach", "--all")
	if res.exitCode != 1 {
		t.Fatalf("detach --all: exit %d, want 1; an entry that was edited is refused, not removed", res.exitCode)
	}
	if !strings.Contains(res.stderr, "matcher") {
		t.Errorf("the refusal does not name the field: %q", res.stderr)
	}
	if !bytes.Equal(before, e.settingsBytes()) {
		t.Errorf("a refusing detach --all still changed the file:\n%s", e.settingsBytes())
	}
}

// TestH6_PlainDetachLeavesNoStoreBehind is H-18's rule from the other side.
// Plain detach reads the install id from the store, so on a machine that has
// none it has to say so rather than open one: a store created by the command
// that uninstalls is an install nobody asked for, key material and all.
func TestH6_PlainDetachLeavesNoStoreBehind(t *testing.T) {
	e := newEnv(t)
	seedInstalls(e, installA)
	if err := os.RemoveAll(e.home); err != nil {
		t.Fatal(err)
	}

	res := e.detach()
	if _, err := os.Stat(e.home); !os.IsNotExist(err) {
		t.Errorf("plain detach created a store at %s (stat: %v)", e.home, err)
	}
	if res.exitCode == 0 {
		t.Fatalf("plain detach reported success with no store to read the id from: %q", res.stdout)
	}
	for _, want := range []string{"--install", "--all"} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("the refusal does not point at %s: %q", want, res.stderr)
		}
	}
}

// TestH6_WatchPrintsTheUndoLine: the id exists in one place, the store, and the
// undo form that survives losing it needs the id. Printing the line at install
// time is what puts it somewhere a deleted store cannot take it from.
func TestH6_WatchPrintsTheUndoLine(t *testing.T) {
	e := newEnv(t)
	// Twice: the install and the no-op. The second is the run a user is most
	// likely to be looking at when they need the id.
	for _, which := range []string{"first watch", "second watch"} {
		res := e.watch()
		if res.exitCode != 0 {
			t.Fatalf("%s: exit %d, stderr %q", which, res.exitCode, res.stderr)
		}
		want := "rashomon detach --install " + e.installID()
		if !hasLine(res.stdout, want) {
			t.Errorf("%s did not print the line %q:\n%s", which, want, res.stdout)
		}
	}
}

func hasLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// seedInstalls writes a settings file carrying one entry per event for each
// install named, and the foreign entry beside them under PreToolUse. It is
// built as text and spliced, so the foreign bytes are the ones the constant
// declares and not an encoder's rendering of them.
func seedInstalls(e *env, ids ...string) {
	e.t.Helper()
	var events []string
	for _, ev := range installedEvents {
		var entries []string
		if ev.event == "PreToolUse" {
			entries = append(entries, foreignEntry)
		}
		for _, id := range ids {
			entries = append(entries, installEntry(e.t, ev.sub, id))
		}
		events = append(events, fmt.Sprintf("%q: [%s]", ev.event, strings.Join(entries, ", ")))
	}
	e.writeSettings("{\"hooks\": {" + strings.Join(events, ", ") + "}}\n")
}

// installEntry renders another install's entry as that install would have
// written it.
func installEntry(t *testing.T, sub, id string) string {
	t.Helper()
	m := map[string]any{"hooks": []map[string]any{{
		"type": "command", "command": rashomonBin + " " + sub + " --install " + id, "timeout": 5,
	}}}
	if sub == "hook" || sub == "post" {
		m["matcher"] = "*"
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertInstallEntries checks how many entries each event carries for one
// install id.
func assertInstallEntries(e *env, id string, want int) {
	e.t.Helper()
	sf := e.settings()
	for _, ev := range installedEvents {
		got := 0
		for _, g := range sf.Hooks[ev.event] {
			for _, h := range g.Hooks {
				if strings.HasSuffix(h.Command, " --install "+id) {
					got++
				}
			}
		}
		if got != want {
			e.t.Errorf("under %s, %d entries for install %s, want %d", ev.event, got, id, want)
		}
	}
}

// TestH06_DetachLeavesTheFileByteForByteAsFound is the strongest available form
// of the promise detach makes in its own help text: "remove them, leaving
// everything else as found".
//
// Byte equality rather than semantic equality, because the settings document is
// a byte-preserving editor by design and because byte equality is what the user
// checks: they look at the file, or at `git diff` on a dotfiles repository. A
// residue that parses the same but reads differently still costs them a
// diff they have to think about.
//
// FOUND THE HARD WAY. watch creates a hooks object and one key per event it
// installs; detach removed its entries and left those keys behind as empty
// arrays. On a file that had no hooks at all, watch-then-detach left four empty
// event keys and a hooks object that were not there before, and the file had to
// be repaired by hand.
func TestH06_DetachLeavesTheFileByteForByteAsFound(t *testing.T) {
	e := newEnv(t)

	// A file with no hooks key at all, which is the shape that shows the residue.
	original := []byte(`{
  "env": {
    "SOMETHING": "1"
  },
  "model": "opus"
}
`)
	if err := os.WriteFile(e.settingsPath(), original, 0o600); err != nil {
		t.Fatal(err)
	}

	watch := e.run("", nil, "watch")
	if watch.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", watch.exitCode, watch.stderr)
	}
	installed, err := os.ReadFile(e.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(installed, original) {
		t.Fatal("premise: watch did not change the file, so detach has nothing to restore")
	}

	detach := e.run("", nil, "detach")
	if detach.exitCode != 0 {
		t.Fatalf("detach: exit %d, stderr %q", detach.exitCode, detach.stderr)
	}

	after, err := os.ReadFile(e.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("detach did not leave the file as found.\n--- before ---\n%s\n--- after ---\n%s",
			original, after)
	}
}

// TestH06_DetachKeepsAForeignHookEventIntact is the guard against the wrong fix.
//
// Removing empty keys must not become removing keys: an event another tool owns,
// or an event whose array the user left empty themselves inside a hooks object
// they wrote, is not ours to tidy. Only a hooks object we emptied completely
// goes away with us.
func TestH06_DetachKeepsAForeignHookEventIntact(t *testing.T) {
	e := newEnv(t)
	original := []byte(`{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/usr/local/bin/other-tool"
          }
        ]
      }
    ]
  }
}
`)
	if err := os.WriteFile(e.settingsPath(), original, 0o600); err != nil {
		t.Fatal(err)
	}

	if r := e.run("", nil, "watch"); r.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", r.exitCode, r.stderr)
	}
	if r := e.run("", nil, "detach"); r.exitCode != 0 {
		t.Fatalf("detach: exit %d, stderr %q", r.exitCode, r.stderr)
	}

	after, err := os.ReadFile(e.settingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("a file with a foreign hook was not left as found.\n--- before ---\n%s\n"+
			"--- after ---\n%s", original, after)
	}
}

// TestH6_DetachForceRemovesAnEditedEntry is the way out of the refusal above.
//
// Refusing to clobber an entry someone edited is right, and it is what the two
// items above assert. Refusing with no override is a different thing: a single
// edited timeout -- the most natural edit there is to these entries -- left
// detach, detach --all and watch all failing, every entry still installed and
// firing on every tool call, and no supported way to remove them. The refusal
// now names the way out, and this item is the way out.
//
// Break: make --force an unknown argument again.
func TestH6_DetachForceRemovesAnEditedEntry(t *testing.T) {
	e := newEnv(t)
	seedInstalls(e, installA, installB)

	var doc map[string]any
	if err := json.Unmarshal(e.settingsBytes(), &doc); err != nil {
		t.Fatal(err)
	}
	pre := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	pre[2].(map[string]any)["matcher"] = "Bash" // narrowed by hand
	out, _ := json.MarshalIndent(doc, "", "  ")
	e.writeSettings(string(out) + "\n")

	// The refusal stands by default, and now says how to get past it.
	res := e.run("", nil, "detach", "--all")
	if res.exitCode != 1 {
		t.Fatalf("detach --all: exit %d, want 1; the default still refuses", res.exitCode)
	}
	if !strings.Contains(res.stderr, "--force") {
		t.Errorf("the refusal does not name the way out:\n%s", res.stderr)
	}

	res = e.run("", nil, "detach", "--all", "--force")
	if res.exitCode != 0 {
		t.Fatalf("detach --all --force: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "you had edited") {
		t.Errorf("--force removed the edited entry without saying so:\n%s", res.stdout)
	}
	// Ours are gone; the foreign entry seeded alongside them is untouched,
	// because --force is permission to remove an edited entry OF OURS and
	// never permission to touch somebody else's.
	body := string(e.settingsBytes())
	if strings.Contains(body, "--install") {
		t.Errorf("an entry of ours survived detach --all --force:\n%s", body)
	}
	if !strings.Contains(body, "foreign-sentinel") {
		t.Errorf("--force removed a hook that was not ours:\n%s", body)
	}
}
