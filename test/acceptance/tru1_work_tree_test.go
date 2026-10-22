package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const questionWorkTree = "Type that directory's name to confirm: "

// workTreeEnv is an env whose store root does not exist yet and would sit
// inside a git work tree whose top directory is named "project".
func workTreeEnv(t *testing.T) (e *env, top string) {
	t.Helper()
	e = newEnv(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	top = filepath.Join(base, "project")
	if err := os.MkdirAll(filepath.Join(top, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	e.home = filepath.Join(top, "state", "rashomon")
	return e, top
}

// watchToTheThirdQuestion runs watch on a terminal and answers y to the
// install question and n to the Agent SDK one.
func (e *env) watchToTheThirdQuestion(extraEnv ...string) *ptySession {
	e.t.Helper()
	s := startOnPTY(e.t, e.command("project\n", extraEnv, "watch"))
	s.waitFor(questionInstall)
	s.answer("y")
	s.waitFor(questionSDK)
	s.answer("n")
	s.waitFor(questionWorkTree)
	return s
}

// TestTRU1_AStoreRootInAWorkTreeIsConfirmedByName: with the store root
// inside a git work tree, watch asks a third question, after the other two
// and before any write, naming the root and the work tree; only the tree's
// top directory name, typed, confirms it. Anything else, EOF included,
// writes nothing and exits 1, and --yes cannot answer it. A confirmed
// answer is recorded in install.json as root_in_work_tree, and a failed
// write after it undoes it with the rest (Technical spec 7.2.3 step 5).
func TestTRU1_AStoreRootInAWorkTreeIsConfirmedByName(t *testing.T) {
	declined := func(t *testing.T, e *env, top string, res result, what string) {
		t.Helper()
		if res.exitCode != 1 || !strings.Contains(res.stderr, "nothing was written") {
			t.Errorf("%s: exit %d, stderr %q; want exit 1 and nothing was written", what, res.exitCode, res.stderr)
		}
		if entries, err := os.ReadDir(top); err != nil || len(entries) != 1 {
			t.Errorf("%s: the work tree holds %d entries (%v), want only .git", what, len(entries), err)
		}
	}

	for _, reply := range []string{"nope", "Project", "y", ""} {
		t.Run("typed "+reply, func(t *testing.T) {
			e, top := workTreeEnv(t)
			before := e.machine()
			s := e.watchToTheThirdQuestion()
			if !strings.Contains(s.out.String(), "The store root "+e.home+" is inside the git work tree "+top+".") {
				t.Errorf("the third question does not name the root and the work tree:\n%s", s.out.String())
			}
			s.answer(reply)
			declined(t, e, top, s.wait(), "typed "+reply)
			e.requireUnchanged(before, "a third question not confirmed")
		})
	}

	t.Run("EOF", func(t *testing.T) {
		e, top := workTreeEnv(t)
		s := e.watchToTheThirdQuestion()
		s.eof()
		declined(t, e, top, s.wait(), "EOF at the third question")
	})

	t.Run("the name then EOF", func(t *testing.T) {
		e, top := workTreeEnv(t)
		s := e.watchToTheThirdQuestion()
		s.typeThenEOF("project")
		declined(t, e, top, s.wait(), "the name typed, then EOF before Enter")
	})

	t.Run("yes flag", func(t *testing.T) {
		e, top := workTreeEnv(t)
		cmd := e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		res := e.wait(cmd)
		declined(t, e, top, res, "watch --yes")
		if !strings.Contains(res.stderr, "inside the git work tree "+top) {
			t.Errorf("watch --yes does not say why it refused: %q", res.stderr)
		}
	})

	t.Run("confirmed", func(t *testing.T) {
		e, top := workTreeEnv(t)
		s := e.watchToTheThirdQuestion()
		s.answer("project")
		if res := s.wait(); res.exitCode != 0 {
			t.Fatalf("watch confirmed: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		b, err := os.ReadFile(filepath.Join(e.home, "install.json"))
		if err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		if err := json.Unmarshal(b, &meta); err != nil {
			t.Fatal(err)
		}
		if meta["root_in_work_tree"] != top || meta["install_id"] == "" {
			t.Errorf("install.json = %s, want root_in_work_tree %s beside the install id", b, top)
		}
		e.requireInstalled()
	})

	// install.json is read before anything is asked; a record that cannot
	// be read refuses watch outright, and is never taken for one that
	// confirms no work tree.
	t.Run("unreadable record", func(t *testing.T) {
		e, top := workTreeEnv(t)
		if err := os.MkdirAll(e.home, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(e.home, "install.json"), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		before := e.machine()
		cmd := e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		res := e.wait(cmd)
		if res.exitCode == 0 || !strings.Contains(res.stderr, "install metadata is malformed") || strings.Contains(res.stderr, "inside the git work tree "+top) {
			t.Errorf("watch over an unreadable install.json: exit %d, stderr %q; want it refused for the record", res.exitCode, res.stderr)
		}
		e.requireUnchanged(before, "an unreadable install.json")
	})

	// A work tree install.json already records as confirmed is not asked
	// about again when watch shows its screen once more, by --yes or on the
	// terminal; one it records for another tree still is.
	t.Run("confirmed before", func(t *testing.T) {
		e, top := workTreeEnv(t)
		s := e.watchToTheThirdQuestion()
		s.answer("project")
		if res := s.wait(); res.exitCode != 0 {
			t.Fatalf("watch confirmed: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		meta := filepath.Join(e.home, "install.json")
		confirmed, err := os.ReadFile(meta)
		if err != nil {
			t.Fatal(err)
		}
		consentFile := filepath.Join(e.home, "consent.json")

		// No consent on file, so watch shows the screen again.
		if err := os.Remove(consentFile); err != nil {
			t.Fatal(err)
		}
		cmd := e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		if res := e.wait(cmd); res.exitCode != 0 {
			t.Errorf("watch --yes with the work tree already confirmed: exit %d, stderr %q", res.exitCode, res.stderr)
		}

		if err := os.Remove(consentFile); err != nil {
			t.Fatal(err)
		}
		s = startOnPTY(t, e.command("", nil, "watch"))
		s.waitFor(questionInstall)
		s.answer("y")
		s.waitFor(questionSDK)
		s.answer("n")
		if res := s.wait(); res.exitCode != 0 || strings.Contains(s.out.String(), questionWorkTree) {
			t.Errorf("watch on the terminal with the work tree already confirmed: exit %d, asked again %v, stderr %q",
				res.exitCode, strings.Contains(s.out.String(), questionWorkTree), res.stderr)
		}
		if now, err := os.ReadFile(meta); err != nil || string(now) != string(confirmed) {
			t.Errorf("install.json = %s (%v), want it as the confirmation left it: %s", now, err, confirmed)
		}

		// The key, not the first occurrence of top: install.json is written
		// with sorted keys, and created_dirs, which sorts first, holds paths
		// under top too.
		recordedKey := `"root_in_work_tree":"` + top + `"`
		if !strings.Contains(string(confirmed), recordedKey) {
			t.Fatalf("install.json = %s, want it to record %s", confirmed, recordedKey)
		}
		elsewhere := strings.Replace(string(confirmed), recordedKey, `"root_in_work_tree":"`+filepath.Join(filepath.Dir(top), "other")+`"`, 1)
		if err := os.WriteFile(meta, []byte(elsewhere), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(consentFile); err != nil {
			t.Fatal(err)
		}
		cmd = e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		if res := e.wait(cmd); res.exitCode != 1 || !strings.Contains(res.stderr, "inside the git work tree "+top) {
			t.Errorf("watch --yes with another work tree recorded: exit %d, stderr %q; want the refusal", res.exitCode, res.stderr)
		}

		unreadable := strings.Replace(string(confirmed), recordedKey, `"root_in_work_tree":7`, 1)
		if err := os.WriteFile(meta, []byte(unreadable), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd = e.command("", nil, "watch", "--yes")
		noTerminal(cmd)
		if res := e.wait(cmd); res.exitCode != 1 || !strings.Contains(res.stderr, "malformed") {
			t.Errorf("watch --yes with an unreadable recorded work tree: exit %d, stderr %q; want it named malformed", res.exitCode, res.stderr)
		}
	})

	for _, step := range []struct{ point, name, undid string }{
		{"watch.work_tree", "recording the confirmed work tree", "opening the store"},
		{"watch.sdk", "recording the Agent SDK answer", "recording the confirmed work tree, opening the store"},
	} {
		t.Run("failing at "+step.point, func(t *testing.T) {
			e, top := workTreeEnv(t)
			before := tree(t, top, e.configDir)
			s := e.watchToTheThirdQuestion("RASHOMON_FAULT=" + step.point + ":fail=1")
			s.answer("project")
			res := s.wait()
			if res.exitCode != 1 || !strings.Contains(res.stderr, step.name+" failed") ||
				!strings.Contains(res.stderr, "undid, in reverse: "+step.undid+";") {
				t.Errorf("watch failing at %s: exit %d, stderr %q; want it named and %q undone", step.point, res.exitCode, res.stderr, step.undid)
			}
			if after := tree(t, top, e.configDir); !reflect.DeepEqual(before, after) {
				t.Errorf("after a failure at %s the machine is not as it was:\nbefore %v\nafter  %v", step.point, keys(before), keys(after))
			}
		})
	}
}
