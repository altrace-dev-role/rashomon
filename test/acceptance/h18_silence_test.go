package acceptance

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestH18_NothingInstallsSilently: the package was installed -- built -- by
// TestMain. Every command except watch must leave the settings file absent, or
// unchanged when it exists.
func TestH18_NothingInstallsSilently(t *testing.T) {
	exercise := func(e *env) {
		e.run("", nil, "version")
		e.hook(defaultPayload().build(t))
		e.post(defaultPost().build(t))
		e.probe("start", testSession)
		e.probe("end", testSession)
		e.run("", nil, "report")
		e.run("", nil, "report", "--json")
		e.status()
		e.forget("1h")
		e.forgetBefore("1h")
		e.detach()
		// The recovery forms too: they take the id rather than reading it, so
		// nothing stops them running on a machine that never installed at all.
		e.run("", nil, "detach", "--install", installA)
		e.run("", nil, "detach", "--all")
	}

	t.Run("absent stays absent", func(t *testing.T) {
		e := newEnv(t)
		exercise(e)
		if _, err := os.Stat(e.settingsPath()); !os.IsNotExist(err) {
			t.Fatalf("settings.json exists after running everything but watch (stat: %v)", err)
		}
	})

	t.Run("present stays unchanged", func(t *testing.T) {
		e := newEnv(t)
		seed := `{"permissions": {"allow": ["Bash(ls:*)"]}}` + "\n"
		e.writeSettings(seed)
		exercise(e)
		if got := e.settingsBytes(); !bytes.Equal(got, []byte(seed)) {
			t.Fatalf("settings.json changed without watch:\n%s", got)
		}
	})

	// The same rule pointed at the store, from TestH6_PlainDetachLeavesNoStoreBehind.
	// status is the command a user runs when they suspect nothing is installed;
	// opening a store creates one, so the command that reports "nothing here"
	// must not be the command that puts something here.
	t.Run("status leaves no store behind", func(t *testing.T) {
		e := newEnv(t)
		if err := os.RemoveAll(e.home); err != nil {
			t.Fatal(err)
		}
		res := e.status()
		if res.exitCode != 0 {
			t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if _, err := os.Stat(e.home); !os.IsNotExist(err) {
			t.Errorf("status created a store at %s (stat: %v)", e.home, err)
		}
		if !strings.Contains(res.stdout, "present: no") {
			t.Errorf("status does not say the store is absent:\n%s", res.stdout)
		}
	})

	t.Run("only watch writes", func(t *testing.T) {
		e := newEnv(t)
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d", res.exitCode)
		}
		if _, err := os.Stat(e.settingsPath()); err != nil {
			t.Fatalf("watch did not write settings.json: %v", err)
		}
	})
}

// TestH19_DetachDoesNotRewriteHistory: coverage is decided at run time. A
// completed run renders the same after detach as before, because report reads
// what the run recorded about itself and never today's configuration.
func TestH19_DetachDoesNotRewriteHistory(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	if res := e.probe("end", testSession); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d", res.exitCode)
	}

	before := e.report(testSession)
	if before.Coverage.State != "verified" {
		t.Fatalf("premise broken: the run is not verified before detach (%v)", before.Coverage.Reasons)
	}

	if res := e.detach(); res.exitCode != 0 {
		t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := len(ours(e.settings().Hooks["PreToolUse"])); got != 0 {
		t.Fatalf("premise broken: %d entries of ours remain after detach", got)
	}

	after := e.report(testSession)
	if after.Coverage.State != "verified" {
		t.Errorf("detach retroactively invalidated a completed run: %v", after.Coverage.Reasons)
	}
	if after.Coverage.HookEntryAtStart != "present_settings" || after.Coverage.HookEntryAtEnd != "present_settings" {
		t.Errorf("hook entry renders %s/%s after detach; the run recorded present_settings at both ends",
			after.Coverage.HookEntryAtStart, after.Coverage.HookEntryAtEnd)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("the report changed after detach:\nbefore: %+v\nafter:  %+v", before, after)
	}
}
