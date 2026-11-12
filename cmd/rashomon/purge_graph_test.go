package main

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/purge"
	"github.com/altrace-dev-role/rashomon/internal/seed"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func writeAnchors(t *testing.T, root string) {
	t.Helper()
	for _, a := range []string{purge.InstallMeta, purge.InstallKey} {
		if err := os.WriteFile(filepath.Join(root, a), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPurge_RemovesPromptTextFromSessionLog (W-M2.11 1d, director 16:06
// item 4): the session log holds prompt text under consent; a purge plan
// removes the file, and no file under the root holds the text afterwards.
func TestPurge_RemovesPromptTextFromSessionLog(t *testing.T) {
	const text = "quartzwombat distinctive prompt text"
	root := t.TempDir()
	writeAnchors(t, root)
	l := graph.OpenSessionLog(root, "github.com/x/y", graph.RevCommit("c1"), true)
	if err := l.Append(l.Session("s1"), l.Prompt("s1", 1, text)); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(l.Path); err != nil || !strings.Contains(string(b), "quartzwombat") {
		t.Fatalf("the log does not hold the prompt text before the purge: %v", err)
	}
	plan, err := purge.Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RemoveRoot || len(plan.Foreign) != 0 {
		t.Fatalf("the plan keeps the root: Foreign %q", plan.Foreign)
	}
	if out := plan.Execute(); len(out.Remaining)+len(out.Stranded) != 0 {
		t.Fatalf("the purge left %q %q", out.Remaining, out.Stranded)
	}
	if _, err := os.Stat(l.Path); err == nil {
		t.Error("the session log is still there")
	}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "quartzwombat") {
				t.Errorf("%s still holds the prompt text", p)
			}
		}
		return nil
	})
}

// TestPurge_EveryWriterFileIsOwned (W-M2.11 1d; the guard): a store built
// through the real writers where one exists (the evidence file through
// graphBrief, the session log through OpenSessionLog, seed.json through
// seed.WriteKept), session_content by name (no writer yet), and every row
// of disclose's file inventory materialised, has nothing purge calls
// foreign. A row a new writer adds with no purge owner fails here.
func TestPurge_EveryWriterFileIsOwned(t *testing.T) {
	root := t.TempDir()
	writeAnchors(t, root)
	proj := t.TempDir()
	graphProject(t, proj)
	if _, ok := graphBrief(root, filepath.Join(proj, "services", "auth"), "s1", "p1", []byte("purging the sessions")); !ok {
		t.Fatal("no brief")
	}
	l := graph.OpenSessionLog(root, "github.com/x/y", graph.RevCommit("c1"), true)
	if err := l.Append(l.Session("s1")); err != nil {
		t.Fatal(err)
	}
	if err := seed.WriteKept(root, seed.Kept{Version: 1, KeptAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, store.FileSessionContent), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOwned(t, root)

	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Files) < 20 {
		t.Fatalf("the inventory has %d rows, fewer than expected: the read is broken", len(inv.Files))
	}
	// A "*" stands for a name its writer chooses: a hex id, a fired event, a
	// baseline key and digest, a learner file. One fill per kind of name.
	fills := append([]string{"0123456789abcdef", "x1", "proj-0123456789ab", learn.FileKey, learn.FileSum}, store.FiredEvents()...)
	for _, r := range inv.Files {
		rel := r.Pattern
		if r.Base != "" {
			rel = r.Base + "/" + r.Pattern
		}
		rel = strings.ReplaceAll(rel, "<session>", "sess1")
		owned := false
		for _, fill := range fills {
			one := t.TempDir()
			writeAnchors(t, one)
			name := filepath.Join(one, filepath.FromSlash(strings.ReplaceAll(rel, "*", fill)))
			if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if p, err := purge.Build(one, nil, ""); err == nil && p.RemoveRoot && len(p.Foreign) == 0 {
				owned = true
				break
			}
		}
		if !owned {
			t.Errorf("inventory row %q is written by a store writer and purge calls every name it can make foreign", path.Clean(rel))
		}
	}
}

func assertOwned(t *testing.T, root string) {
	t.Helper()
	p, err := purge.Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Foreign) != 0 || !p.RemoveRoot {
		t.Errorf("purge calls files the writers made foreign: RemoveRoot %v, Foreign %q", p.RemoveRoot, p.Foreign)
	}
}
