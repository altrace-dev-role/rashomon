package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// refuseGitConfig writes dir's .git/config naming an origin, grown with a
// comment line to size bytes, and returns its path with symlinks resolved.
func refuseGitConfig(t *testing.T, dir string, size int) string {
	t.Helper()
	text := "[remote \"origin\"]\n\turl = https://host.example/org/repo.git\n"
	path := filepath.Join(dir, ".git", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text+"#"+strings.Repeat("x", size-len(text)-2)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// TestHeadCommit_RefusalIsAnErrorNotC1: a .git
// pointer or commondir that is there and refused (over
// baseline.MaxGitFileBytes, or not a regular file) is headCommit's error,
// so graph index fails loud instead of indexing at C1, the commit of no
// repository; no repository is still C1 with no error. The controls, the
// same worktree with each file regular and under the cap, read the branch's
// commit. Break: give C1 on a refusal.
func TestHeadCommit_RefusalIsAnErrorNotC1(t *testing.T) {
	a := strings.Repeat("a", 40)
	for _, refuse := range []string{"pointer over the cap", "commondir a directory"} {
		t.Run(refuse, func(t *testing.T) {
			write := func(path, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			gitdir := filepath.Join(t.TempDir(), ".git")
			write(filepath.Join(gitdir, "refs", "heads", "main"), a+"\n")
			wtgit := filepath.Join(gitdir, "worktrees", "wt")
			commondir := filepath.Join(wtgit, "commondir")
			write(commondir, "../..\n")
			write(filepath.Join(wtgit, "HEAD"), "ref: refs/heads/main\n")
			wt := t.TempDir()
			pointer := filepath.Join(wt, ".git")
			write(pointer, "gitdir: "+wtgit+"\n")
			if got, err := headCommit(wt); got != a[:12] || err != nil {
				t.Fatalf("control, regular files under the cap: headCommit = %q, %v; want %q, nil", got, err, a[:12])
			}
			if refuse == "pointer over the cap" {
				write(pointer, "gitdir: "+wtgit+"\n"+strings.Repeat(" ", baseline.MaxGitFileBytes))
			} else {
				if err := os.Remove(commondir); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(commondir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := headCommit(wt); err == nil {
				t.Errorf("%s: headCommit = %q, nil; want an error, not a commit", refuse, got)
			}
		})
	}
	if got, err := headCommit(t.TempDir()); got != "C1" || err != nil {
		t.Errorf("no repository: headCommit = %q, %v; want C1, nil", got, err)
	}
}

// TestGraphVerbs_RefusedGitReadFailsLoud: off the hook
// path a refused git read is an error naming the file and the cap, from
// graph index (index.Build's RepoIDErr) and from graph knowledge import,
// which fails before it writes a row, since a row keyed under no id or the
// path form would not match the index built once the config reads. The
// control, the same tree with the config at the cap: both verbs succeed,
// and the import writes its rows. Break: key either verb on RepoID.
func TestGraphVerbs_RefusedGitReadFailsLoud(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	src := filepath.Join(t.TempDir(), "LEDGER.txt")
	if err := os.WriteFile(src, []byte("id | when | who | what | layer | evidence | catcher | today | gap\n"+
		"B1 | 11-01 | me | changed the session ttl without jobs/purge_sessions.py | purge / DETECT | run 3 | me | no | the nightly purge must change with session lifetime\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	knowledgeDir := filepath.Join(dir, ".rashomon", "knowledge")
	if _, err := os.Stat(knowledgeDir); !os.IsNotExist(err) {
		t.Fatalf("the fixture has %s already (stat %v)", knowledgeDir, err)
	}
	config := refuseGitConfig(t, dir, baseline.MaxGitFileBytes+1)
	loud := func(what string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), config) || !strings.Contains(err.Error(), strconv.Itoa(baseline.MaxGitFileBytes)) {
			t.Errorf("config one byte over: %s = %v; want an error naming %s and %d", what, err, config, baseline.MaxGitFileBytes)
		}
	}
	loud("graph index", graphIndex([]string{dir}, io.Discard))
	t.Chdir(filepath.Join(dir, "services", "auth"))
	loud("graph knowledge import", cmdGraph([]string{"knowledge", "import", src}, io.Discard))
	if _, err := os.Stat(knowledgeDir); !os.IsNotExist(err) {
		t.Errorf("config one byte over: graph knowledge import wrote %s (stat %v); want no row written", knowledgeDir, err)
	}

	refuseGitConfig(t, dir, baseline.MaxGitFileBytes)
	if err := graphIndex([]string{dir}, io.Discard); err != nil {
		t.Errorf("control, config at the cap: graph index = %v", err)
	}
	if err := cmdGraph([]string{"knowledge", "import", src}, io.Discard); err != nil {
		t.Errorf("control, config at the cap: graph knowledge import = %v", err)
	}
	if _, err := os.Stat(knowledgeDir); err != nil {
		t.Errorf("control, config at the cap: no rows written (stat %v)", err)
	}
}
