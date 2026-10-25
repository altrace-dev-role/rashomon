package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/purge"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestTRU28_ConfirmIsYesOnlyForYes: purge's question takes y or yes, in any
// case, as yes, and everything else -- an empty line, end of input, a word
// that starts with y -- as no.
func TestTRU28_ConfirmIsYesOnlyForYes(t *testing.T) {
	for in, want := range map[string]bool{
		"y\n": true, "YES\n": true, " yes \n": true, "y": true,
		"\n": false, "": false, "n\n": false, "yeah\n": false, "no\n": false,
	} {
		var out bytes.Buffer
		if got := confirm(strings.NewReader(in), &out); got != want {
			t.Errorf("confirm(%q) = %v, want %v", in, got, want)
		}
		if out.String() != purgePrompt {
			t.Errorf("confirm(%q) asked %q, want %q", in, out.String(), purgePrompt)
		}
	}
}

// TestTRU28_PlanSaysTheRootStaysBecauseOfTheFilesItLists: with a path of the
// user's left in the root, or inside runs, the plan says in one line that the
// root, and the directories above the listed files, stay because of those
// files; with nothing left, it does not say so and the root is listed as
// removed.
func TestTRU28_PlanSaysTheRootStaysBecauseOfTheFilesItLists(t *testing.T) {
	build := func(extra ...string) (purge.Plan, string) {
		root := filepath.Join(t.TempDir(), "rashomon")
		for _, f := range append([]string{store.FileInstallMeta, store.FileInstallKey, "runs/s1/" + store.FileRecords}, extra...) {
			p := filepath.Join(root, filepath.FromSlash(f))
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		p, err := purge.Build(root, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return p, root
	}
	target := func(root string) purgeTarget {
		return purgeTarget{root: root, settingsPath: filepath.Join(root, "settings.json"), binary: "/bin/rashomon"}
	}
	count := func(plan purge.Plan, root string) (int, string) {
		var out bytes.Buffer
		printPurgePlan(&out, target(root), plan, true)
		return strings.Count(out.String(), purgeRootStays), out.String()
	}

	for _, foreign := range []string{"notes.txt", "runs/s1/my-analysis.md"} {
		plan, root := build(foreign)
		if plan.RemoveRoot {
			t.Fatalf("%s: the root is planned for removal", foreign)
		}
		n, out := count(plan, root)
		if n != 1 {
			t.Errorf("%s: the plan says why the root stays %d times, want once:\n%s", foreign, n, out)
		}
		line := ""
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, purgeRootStays) {
				line = l
			}
		}
		if !strings.HasPrefix(strings.TrimSpace(line), root+" -- ") || !strings.Contains(out, foreign[strings.LastIndex(foreign, "/")+1:]) {
			t.Errorf("%s: the line does not name the root, or the file is not listed:\n%s", foreign, out)
		}
	}

	plan, root := build()
	if n, out := count(plan, root); n != 0 || !plan.RemoveRoot {
		t.Errorf("a store with nothing left in it says the root stays (%d times, RemoveRoot %v):\n%s", n, plan.RemoveRoot, out)
	}
}
