package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-148 -- detach --harness codex deletes Codex's hooks.json when watch
// created it and our entries were all it held, and in no other case.
//
// Phase 4 measured the gap: on a machine with no hooks.json, watch then detach
// left a 3-byte "{}" behind. The file is Codex's configuration, so deleting it
// when it was not ours to delete is the larger error, and most of this item is
// the cases where the file must stay: it was there before watch; the user put
// an entry of their own in it; the store that would say who created it is
// gone; the store says it created a file at another path.
func TestH148_DetachDeletesTheHooksFileWatchCreated(t *testing.T) {
	marker := func(e *env) string { return filepath.Join(e.home, "codex-hooks-created") }

	t.Run("created by watch, nothing else in it", func(t *testing.T) {
		e := newEnv(t)
		e.makeCodexDir("")
		mustWatchCodex(t, e)
		if fi, err := os.Stat(marker(e)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("watch recorded no creation, or with the wrong mode: %v", err)
		}
		// A second watch finds the file there and must not forget who made it.
		mustWatchCodex(t, e)
		res := e.detachCodex()
		if res.exitCode != 0 || !strings.Contains(res.stdout, "rashomon: removed "+e.codexPath()+": watch created it, and nothing else was in it\n") {
			t.Errorf("detach: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
		}
		if b := e.codexBytes(); b != nil {
			t.Errorf("hooks.json is still there:\n%s", b)
		}
		if _, err := os.Stat(marker(e)); !os.IsNotExist(err) {
			t.Errorf("the creation marker outlived the file: %v", err)
		}
	})

	t.Run("there before watch, and empty", func(t *testing.T) {
		e := newEnv(t)
		e.makeCodexDir("{}\n")
		mustWatchCodex(t, e)
		if _, err := os.Stat(marker(e)); !os.IsNotExist(err) {
			t.Errorf("watch recorded creating a file that was already there")
		}
		if res := e.detachCodex(); res.exitCode != 0 || strings.Contains(res.stdout, "rashomon: removed ") {
			t.Errorf("detach: exit %d, stdout %q", res.exitCode, res.stdout)
		}
		if got := string(e.codexBytes()); got != "{}\n" {
			t.Errorf("hooks.json is %q, want the \"{}\" the user had", got)
		}
	})

	t.Run("there before watch, with the user's hooks", func(t *testing.T) {
		e := newEnv(t)
		e.makeCodexDir(codexSeed)
		mustWatchCodex(t, e)
		if res := e.detachCodex(); res.exitCode != 0 {
			t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if got, want := canonicalJSON(t, e.codexBytes()), canonicalJSON(t, []byte(codexSeed)); got != want {
			t.Errorf("hooks.json after detach:\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("created by watch, then the user added a hook", func(t *testing.T) {
		e := newEnv(t)
		e.makeCodexDir("")
		mustWatchCodex(t, e)
		withUser := strings.Replace(string(e.codexBytes()), `"hooks": {`,
			`"hooks": {
    "UserPromptSubmit": [{"hooks":[{"type":"command","command":"notify-me"}]}],`, 1)
		if err := os.WriteFile(e.codexPath(), []byte(withUser), 0o600); err != nil {
			t.Fatal(err)
		}
		if res := e.detachCodex(); res.exitCode != 0 || strings.Contains(res.stdout, "rashomon: removed ") {
			t.Errorf("detach: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
		}
		if got := string(e.codexBytes()); !strings.Contains(got, "notify-me") || strings.Contains(got, "--harness codex") {
			t.Errorf("hooks.json after detach:\n%s", got)
		}
	})

	t.Run("created by watch, store gone", func(t *testing.T) {
		e := newEnv(t)
		e.makeCodexDir("")
		mustWatchCodex(t, e)
		if err := os.RemoveAll(e.home); err != nil {
			t.Fatal(err)
		}
		if res := e.detachCodex("--all"); res.exitCode != 0 {
			t.Fatalf("detach --all: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if got := string(e.codexBytes()); got != "{}\n" {
			t.Errorf("hooks.json is %q; with no record of who made it, it stays", got)
		}
		if _, err := os.Stat(e.home); !os.IsNotExist(err) {
			t.Errorf("detach created a store looking for the marker")
		}
	})

	t.Run("the store created a hooks.json at another path", func(t *testing.T) {
		e := newEnv(t)
		e.makeCodexDir("")
		mustWatchCodex(t, e)
		if err := os.WriteFile(marker(e), []byte(filepath.Join(t.TempDir(), "hooks.json")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if res := e.detachCodex(); res.exitCode != 0 {
			t.Fatalf("detach: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if got := string(e.codexBytes()); got != "{}\n" {
			t.Errorf("hooks.json is %q; the marker names another file", got)
		}
	})
}

func mustWatchCodex(t *testing.T, e *env) {
	t.Helper()
	if res := e.watchCodex(); res.exitCode != 0 {
		t.Fatalf("watch --harness codex: exit %d, stderr %q", res.exitCode, res.stderr)
	}
}
