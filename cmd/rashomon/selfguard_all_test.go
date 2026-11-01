package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// agentHere is a machine watch has run on: a store with its consent, and this
// install's entries in settings.json, rewritten with whitespace the settings
// encoder never writes, so even a rewrite of the same entries changes its
// bytes. The agent signal is cleared until the test sets it.
func agentHere(t *testing.T) (root, cfg, installID string) {
	t.Helper()
	root, cfg = t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	t.Setenv("CLAUDECODE", "")
	installID = benchInstall(t, root, cfg)
	p := filepath.Join(cfg, "settings.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(append([]byte("  "), b...), '\n', '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	return root, cfg, installID
}

// treeState is every directory and file under dirs, each file with its
// bytes' digest, so two calls differ when anything was added, removed or
// rewritten with other bytes.
func treeState(t *testing.T, dirs ...string) string {
	t.Helper()
	var b strings.Builder
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() {
				fmt.Fprintf(&b, "%s/\n", p)
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fmt.Fprintf(&b, "%s %x\n", p, sha256.Sum256(data))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return b.String()
}

// wantRefusal is the one form every agent-session refusal takes (#177 review
// D1, D2): the reason first, then the command and where to run it -- a
// terminal outside Claude Code, since a `!` line typed into Claude Code
// carries the signal too -- then what was left as it was, and the signal
// last. got is the error's text, with or without guarded's "rashomon: ".
func wantRefusal(t *testing.T, what, got, command, nothing string) {
	t.Helper()
	want := ", so only you can run it, in a terminal outside Claude Code: `rashomon " + command + "`. Nothing was " + nothing + "."
	i, sig := strings.Index(got, want), strings.Index(got, "(CLAUDECODE is set here")
	reason := ""
	if i > 0 {
		reason = strings.TrimPrefix(got[:i], "rashomon: ")
	}
	if reason == "" || strings.Contains(reason, "CLAUDECODE") || sig < i+len(want) {
		t.Errorf("%s: the refusal is %q; want the reason first, then %q, then the signal", what, got, want)
	}
}

// detach and pause are the person's ways to stop the recording, so the agent
// being recorded cannot use them (R31): each refuses an agent session before
// it reads the store or settings.json, names the command to run in a
// terminal outside Claude Code, and changes nothing -- no entry removed, no
// pause marker, no gap record.
func TestAGDP1_AnAgentSessionCannotDetachOrPause(t *testing.T) {
	root, cfg, id := agentHere(t)
	before := treeState(t, root, cfg)
	t.Setenv("CLAUDECODE", "1")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"detach"}, "detach"},
		{[]string{"detach", "--force"}, "detach --force"},
		{[]string{"detach", "--install", id}, "detach --install " + id},
		{[]string{"detach", "--force", "--all"}, "detach --all --force"},
		{[]string{"pause"}, "pause"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(c.args, nil, &stdout, &stderr)
		if code != exitFail || stdout.Len() != 0 {
			t.Errorf("rashomon %s in an agent session: exit %d, stdout %q, stderr %q", strings.Join(c.args, " "), code, stdout.String(), stderr.String())
		}
		wantRefusal(t, strings.Join(c.args, " "), stderr.String(), c.want, "written")
	}
	if after := treeState(t, root, cfg); after != before {
		t.Errorf("an agent session changed the store or settings.json:\n%s\nwas:\n%s", after, before)
	}
}

// TestAGRF1_EveryGuardReturnsTheOneRefusal: every agent-signal check in this
// package returns selfguard.Refusal, so the reason-first form and the place
// to run the command are one text in one place (#177 review L1). A guard
// written with its own fmt.Errorf fails here.
func TestAGRF1_EveryGuardReturnsTheOneRefusal(t *testing.T) {
	calls := func(n ast.Node, pkg, name string) bool {
		found := false
		ast.Inspect(n, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok {
				sel := s.Sel
				if x, ok := s.X.(*ast.Ident); ok && x.Name == pkg && sel.Name == name {
					found = true
				}
			}
			return !found
		})
		return found
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset, guards := token.NewFileSet(), 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			s, ok := n.(*ast.IfStmt)
			if !ok || s.Init == nil || !calls(s.Init, "selfguard", "AgentSignal") {
				return true
			}
			guards++
			if !calls(s.Body, "selfguard", "Refusal") {
				t.Errorf("%s: an agent-signal check that does not return selfguard.Refusal", fset.Position(s.Pos()))
			}
			return true
		})
	}
	// steer on|off and --repair, summary, bodies, consent off, spec trust and
	// untrust, purge, detach, pause, and watch's and consent's askConsent.
	if guards != 11 {
		t.Errorf("found %d agent-signal checks, want 11; a guard added or removed changes this count and selfguard's package comment together", guards)
	}
}
