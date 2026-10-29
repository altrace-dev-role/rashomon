package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// person is a terminal with a person at it: it answers the prompt with the
// code the prompt showed, or with answer when that is set.
type person struct {
	shown  bytes.Buffer
	answer string
	read   bool
}

var shownCode = regexp.MustCompile(`type (\d{6}): $`)

func (p *person) Write(b []byte) (int, error) { return p.shown.Write(b) }

func (p *person) Read(b []byte) (int, error) {
	if p.read {
		return 0, io.EOF
	}
	p.read = true
	answer := p.answer
	if m := shownCode.FindStringSubmatch(p.shown.String()); answer == "" && m != nil {
		answer = m[1]
	}
	return copy(b, answer+"\n"), nil
}

func atTerminal(t *testing.T, p *person, err error) {
	t.Helper()
	old := learnTerminal
	learnTerminal = func() (io.Reader, io.Writer, func(), error) {
		if err != nil {
			return nil, nil, nil, err
		}
		return p, p, func() {}, nil
	}
	t.Cleanup(func() { learnTerminal = old })
}

func learnRun(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"learn"}, args...), strings.NewReader("123456\n"), &out, &errOut)
	return code, out.String(), errOut.String()
}

const sharedLessons = `{"schema":"lessons/1","lessons":[
	{"id":"go-test-cwd","tier":"proven","text":"run go test from the module root"},
	{"id":"npm-ci","text":"npm ci before npm test"}]}`

// TestP6_LearnImportThenApproveAtTheTerminal: an import lands at proposed;
// approval shows the lesson and a code on the terminal and takes the code
// typed back there, never one on stdin; a wrong code, or no terminal, leaves
// the lesson at proposed.
func TestP6_LearnImportThenApproveAtTheTerminal(t *testing.T) {
	bodiesHere(t)
	file := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(file, []byte(sharedLessons), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := learnRun(t, "import", file); code != 0 || !strings.HasPrefix(out, "imported 2 lessons at proposed, skipped 0;") {
		t.Fatalf("import: exit %d, stdout %q, stderr %q", code, out, errOut)
	}

	atTerminal(t, nil, errors.New("no tty"))
	if code, out, errOut := learnRun(t, "approve", "npm-ci"); code != 1 || out != "" || !strings.Contains(errOut, "no terminal to ask on") {
		t.Errorf("approve with no terminal and a code on stdin: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	wrong := &person{answer: "1234567"}
	atTerminal(t, wrong, nil)
	if code, _, errOut := learnRun(t, "approve", "npm-ci"); code != 1 || !strings.Contains(errOut, "the code did not match") {
		t.Errorf("approve with a wrong code: exit %d, stderr %q", code, errOut)
	}
	p := &person{}
	atTerminal(t, p, nil)
	if code, out, errOut := learnRun(t, "approve", "npm-ci"); code != 0 || out != "approved npm-ci\n" {
		t.Fatalf("approve with the code: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if !strings.Contains(p.shown.String(), "npm ci before npm test") || strings.Contains(p.shown.String(), "123456") {
		t.Errorf("the terminal showed %q: want the lesson's text and a code of its own", p.shown.String())
	}
	_, out, _ := learnRun(t)
	if !strings.Contains(out, "go-test-cwd proposed: 0 fires") || !strings.Contains(out, "npm-ci probation: 0 fires, 0 confirmed, 0 refuted, 0 unresolved; no bound") {
		t.Errorf("lessons listed %q", out)
	}
	if code, _, errOut := learnRun(t, "approve", "npm-ci"); code != 1 || !strings.Contains(errOut, "not proposed") {
		t.Errorf("a second approval: exit %d, stderr %q", code, errOut)
	}
}

// TestP6_LearnArguments: no store answers with no lessons and makes none;
// an argument the verb does not know is refused.
func TestP6_LearnArguments(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	t.Setenv("RASHOMON_HOME", root)
	if code, out, _ := learnRun(t); code != 0 || out != "no lessons: nothing has been recorded here\n" {
		t.Errorf("lessons with no store: exit %d, stdout %q", code, out)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lessons with no store made %s (%v)", root, err)
	}
	for _, args := range [][]string{{"--help"}, {"approve"}, {"import", "a", "b"}, {"list"}} {
		if code, out, _ := learnRun(t, args...); code != 1 || out != "" {
			t.Errorf("lessons %v: exit %d, stdout %q; want refused", args, code, out)
		}
	}
}
