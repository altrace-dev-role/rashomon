package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/learn"
)

// TestSV1d_LessonsByTier: lessons lists this project's lessons under the
// four tiers the learner moves them through, proven first, each with the day
// it entered and its fires; a tier with none says none. A ledger written by
// something other than the learner is said quarantined first, with every
// lesson back at proposed. Break: list in ledger order with the tier as a
// word on each row, and an approved lesson reads like a candidate at a
// glance.
func TestSV1d_LessonsByTier(t *testing.T) {
	st, _, dir := bodiesHere(t)
	file := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(file, []byte(sharedLessons), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := learnRun(t, "import", file); code != 0 {
		t.Fatalf("import: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	atTerminal(t, &person{}, nil)
	if code, out, errOut := learnRun(t, "approve", "npm-ci"); code != 0 {
		t.Fatalf("approve: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	l, err := learn.Open(st.Root(), dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	day := map[string]string{}
	for _, ls := range l.Lessons() {
		day[ls.ID] = time.UnixMilli(ls.SinceUnixMS).UTC().Format(time.DateOnly)
	}
	var buf bytes.Buffer
	if err := cmdLessons(nil, &buf); err != nil {
		t.Fatal(err)
	}
	want := "lessons for this project: 2\n" +
		"proven: none\n" +
		"probation:\n" +
		"  npm-ci since " + day["npm-ci"] + ", 0 fires: npm ci before npm test\n" +
		"proposed:\n" +
		"  go-test-cwd since " + day["go-test-cwd"] + ", 0 fires: run go test from the module root\n" +
		"demoted: none\n"
	if buf.String() != want {
		t.Errorf("lessons =\n%s\nwant:\n%s", buf.String(), want)
	}

	sum := filepath.Join(st.Root(), learn.DirLearn, dir, learn.FileSum)
	if err := os.WriteFile(sum, []byte("not the manifest"), 0o600); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := cmdLessons(nil, &buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(buf.String(), "\n", 3)
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "quarantined: ") || !strings.HasSuffix(lines[0], "the next learn command records it") ||
		lines[1] != "lessons for this project: 2" || !strings.Contains(lines[2], "probation: none\nproposed:\n") {
		t.Errorf("a tampered ledger lists as:\n%s", buf.String())
	}
}

// treeOf is every file under root with its bytes and modification time, as
// one string.
func treeOf(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %x\n", p, info.ModTime().UnixNano(), data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestSV1o_LessonsWritesNothing: lessons over a ledger whose state does not
// check says so, lists every lesson as proposed, and leaves every file under
// the store byte-identical (it used to quarantine through learn.Open and
// demote the approved lessons for good); --json says quarantined; learn
// (its listing), which does write, still quarantines. Break: open the ledger for
// writing in lessons, and the manifest, the lessons and the approved tier
// are rewritten by a question.
func TestSV1o_LessonsWritesNothing(t *testing.T) {
	st, _, dir := bodiesHere(t)
	file := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(file, []byte(sharedLessons), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := learnRun(t, "import", file); code != 0 {
		t.Fatalf("import: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	atTerminal(t, &person{}, nil)
	if code, out, errOut := learnRun(t, "approve", "npm-ci"); code != 0 {
		t.Fatalf("approve: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	sum := filepath.Join(st.Root(), learn.DirLearn, dir, learn.FileSum)
	if err := os.WriteFile(sum, []byte("not the manifest"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := treeOf(t, st.Root())
	var text, js bytes.Buffer
	if err := cmdLessons(nil, &text); err != nil {
		t.Fatal(err)
	}
	if err := cmdLessons([]string{"--json"}, &js); err != nil {
		t.Fatal(err)
	}
	if after := treeOf(t, st.Root()); after != before {
		t.Errorf("lessons changed the store:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if !strings.Contains(text.String(), "probation: none\nproposed:\n") || !strings.Contains(js.String(), `"quarantined": true`) {
		t.Errorf("a tampered ledger read without writing lists as:\n%s\n%s", text.String(), js.String())
	}
	if code, out, errOut := learnRun(t); code != 0 || !strings.HasPrefix(out, "quarantined ") || treeOf(t, st.Root()) == before {
		t.Errorf("learn on the same state must quarantine and write (the control): exit %d, %q, %q", code, out, errOut)
	}
	text.Reset()
	if err := cmdLessons(nil, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text.String(), "quarantined 20") {
		t.Errorf("once the quarantine is recorded, lessons says when:\n%s", text.String())
	}
}
