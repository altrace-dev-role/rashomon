package acceptance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// status answers "what is installed here, and what has it recorded" without
// changing the answer. It is the only command a user runs when they suspect
// nothing is installed, which is exactly the machine on which creating a store
// -- key material and all -- would be an install nobody asked for. H-18 holds
// the store half of that; these hold what it says.

// TestStatus_OnAFreshMachineSaysSo: no store, no settings file, and no verdict
// about ownership, because without a store there is no install id to own
// anything.
func TestStatus_OnAFreshMachineSaysSo(t *testing.T) {
	e := newEnv(t)
	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, want := range []string{
		"store: " + e.home,
		"present: no",
		"settings: " + e.settingsPath(),
		"install ids present: none",
	} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("status does not report %q:\n%s", want, res.stdout)
		}
	}
	for _, ev := range installedEvents {
		if got := fieldLine(t, res.stdout, ev.event); got != "unknown" {
			t.Errorf("%s reads %q on a machine with no install id; there is nothing to call ours", ev.event, got)
		}
	}
	if _, err := os.Stat(e.settingsPath()); !os.IsNotExist(err) {
		t.Errorf("status wrote a settings file (stat: %v)", err)
	}
}

// TestStatus_AfterWatchReportsEveryEntry: all four entries, and the install id
// they were installed under, read back from the store rather than from the
// command line.
func TestStatus_AfterWatchReportsEveryEntry(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.mustHook(defaultPayload().build(t))

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stdout, "present: yes (install "+e.installID()+")") {
		t.Errorf("status does not name the store's install id %s:\n%s", e.installID(), res.stdout)
	}
	if got := fieldLine(t, res.stdout, "runs"); got != "1" {
		t.Errorf("status reports %q run directories, want 1", got)
	}
	for _, ev := range installedEvents {
		if got := fieldLine(t, res.stdout, ev.event); got != "present" {
			t.Errorf("%s reads %q after watch, want present", ev.event, got)
		}
	}
	if got := fieldLine(t, res.stdout, "other installs"); got != "none" {
		t.Errorf("other installs reads %q, want none", got)
	}
}

// TestStatus_NamesAnEntryThatIsAbsent is the assertion that keeps the one above
// honest: a status that said "present" whatever it found would pass it. The
// PostToolUse entry is removed by hand, which is the arrangement in which
// declarations arrive and executions do not.
func TestStatus_NamesAnEntryThatIsAbsent(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	e.writeSettings(withoutEvent(t, string(e.settingsBytes()), "PostToolUse"))

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := fieldLine(t, res.stdout, "PostToolUse"); got != "absent" {
		t.Errorf("PostToolUse reads %q after its entry was removed, want absent", got)
	}
	if got := fieldLine(t, res.stdout, "PreToolUse"); got != "present" {
		t.Errorf("PreToolUse reads %q, want present: only one entry was removed", got)
	}
}

// TestStatus_NamesForeignInstalls: two installs' entries share one file and one
// environment, and the other install's records go to this store under an id
// that is not ours. Naming them is the only warning a user gets.
func TestStatus_NamesForeignInstalls(t *testing.T) {
	e := newEnv(t)
	seedInstalls(e, installA)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := fieldLine(t, res.stdout, "other installs"); got != installA {
		t.Errorf("other installs reads %q, want %s", got, installA)
	}
	if strings.Contains(fieldLine(t, res.stdout, "other installs"), e.installID()) {
		t.Errorf("status counts this machine's own install among the others:\n%s", res.stdout)
	}
}

// TestStatus_NamesTheLayerThatDisabledHooks: an installed entry under a managed
// disableAllHooks never runs, and a status that reported the entry as present
// and stopped there would be describing a recorder that records nothing.
func TestStatus_NamesTheLayerThatDisabledHooks(t *testing.T) {
	e := newEnv(t)
	if err := os.WriteFile(e.managedPath(), []byte(`{"disableAllHooks": true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := e.status()
	if res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := fieldLine(t, res.stdout, "hooks"); got != "disabled by the managed settings layer" {
		t.Errorf("hooks reads %q, want the managed layer named", got)
	}
}

// withoutEvent removes one event from a settings file's hooks object, which is
// how a test reaches the state of an entry a user deleted by hand.
func withoutEvent(t *testing.T, body, event string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	hooks, ok := doc["hooks"].(map[string]any)
	if !ok || hooks[event] == nil {
		t.Fatalf("premise broken: there is no %s entry to remove:\n%s", event, body)
	}
	delete(hooks, event)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(out) + "\n"
}
