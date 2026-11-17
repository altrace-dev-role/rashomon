package index

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// TestBuild_RefusedGitReadFailsLoud: off the hook path
// a refused git read is Build's error, naming the file and the cap, not an
// index keyed under no id or the path form, which an index built once the
// config reads would key apart. The control, the same tree with the config
// at the cap, builds under the origin's id, not ephemeral. Break: key Build
// on RepoID, and the tree one byte over indexes with no error.
func TestBuild_RefusedGitReadFailsLoud(t *testing.T) {
	root := shapes(t)
	text := "[remote \"origin\"]\n\turl = https://host.example/org/repo.git\n"
	config := filepath.Join(root, ".git", "config")
	size := func(n int) {
		t.Helper()
		put(t, config, text+"#"+strings.Repeat("x", n-len(text)-2)+"\n")
	}
	size(baseline.MaxGitFileBytes)
	r, err := Build(root, "c1", twoLangs)
	if err != nil || r.Header.Repo != "host.example/org/repo" {
		t.Fatalf("control, config at the cap: Build = repo %q, %v; want host.example/org/repo, nil", r.Header.Repo, err)
	}
	size(baseline.MaxGitFileBytes + 1)
	resolved, err := filepath.EvalSymlinks(config)
	if err != nil {
		t.Fatal(err)
	}
	r, err = Build(root, "c1", twoLangs)
	if err == nil || !strings.Contains(err.Error(), resolved) || !strings.Contains(err.Error(), strconv.Itoa(baseline.MaxGitFileBytes)) {
		t.Errorf("config one byte over: Build = repo %q, %v; want an error naming %s and %d", r.Header.Repo, err, resolved, baseline.MaxGitFileBytes)
	}
}
