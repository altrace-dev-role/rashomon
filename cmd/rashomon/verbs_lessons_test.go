package main

import (
	"bytes"
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
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "quarantined ") || !strings.HasSuffix(lines[0], "so every lesson went back to proposed") ||
		lines[1] != "lessons for this project: 2" || !strings.Contains(lines[2], "probation: none\nproposed:\n") {
		t.Errorf("a tampered ledger lists as:\n%s", buf.String())
	}
}
