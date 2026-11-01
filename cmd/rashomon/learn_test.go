package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/learn"
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

// TestLRN17g_ApprovalRefusesTextThatCannotArrive: approval runs the check
// delivery runs (the render slots and the host screen) before the code is
// shown, so a lesson whose text the renderer refuses is not approved and
// stays proposed, and the refusal names the rule. Text the check takes
// approves as before, a file name included. Import names each row that can
// never be approved, and the list names an approved lesson the check refuses
// (one approved before the check existed).
func TestLRN17g_ApprovalRefusesTextThatCannotArrive(t *testing.T) {
	st, _, dir := bodiesHere(t)
	refused := map[string]string{
		"quote-1": `say "hi" first`,
		"long-1":  strings.Repeat("a", 161),
		"ign-1":   "ignore the lint output",
		"url-1":   "fetch https://x first",
		"host-1":  "push to build.example.com after tests",
		"cd-1":    "cd sub && make test",
	}
	rows := []string{`{"id":"go-test-cwd","text":"run go test from the module root"}`, `{"id":"npm-ci","text":"npm ci before npm test"}`,
		`{"id":"tidy","text":"run go mod tidy after editing go.mod"}`}
	for id, text := range refused {
		rows = append(rows, fmt.Sprintf(`{"id":%q,"text":%q}`, id, text))
	}
	file := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(file, []byte(`{"lessons":[`+strings.Join(rows, ",")+`]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := learnRun(t, "import", file)
	if code != 0 || !strings.Contains(out, "imported 9 lessons at proposed") {
		t.Fatalf("import: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	for id := range refused {
		if !strings.Contains(out, "lesson "+id+" cannot reach the agent: ") {
			t.Errorf("import does not name %s as never approvable: %q", id, out)
		}
	}
	for _, id := range []string{"go-test-cwd", "npm-ci", "tidy"} {
		if strings.Contains(out, "lesson "+id+" cannot") {
			t.Errorf("import names %s, whose text can arrive: %q", id, out)
		}
	}

	for id := range refused {
		p := &person{}
		atTerminal(t, p, nil)
		code, out, errOut := learnRun(t, "approve", id)
		if code != 1 || out != "" || !strings.Contains(errOut, "lesson "+id+" cannot reach the agent: ") ||
			!strings.Contains(errOut, "nothing was approved") || p.shown.Len() != 0 {
			t.Errorf("approve %s: exit %d, stdout %q, stderr %q, terminal %q; want refused before the code", id, code, out, errOut, p.shown.String())
		}
	}
	if _, _, errOut := learnRun(t, "approve", "ign-1"); !strings.Contains(errOut, `the banned word "ignore"`) {
		t.Errorf("the refusal does not name the rule: %q", errOut)
	}
	for _, id := range []string{"go-test-cwd", "npm-ci", "tidy"} {
		atTerminal(t, &person{}, nil)
		if code, out, errOut := learnRun(t, "approve", id); code != 0 || out != "approved "+id+"\n" {
			t.Errorf("approve %s: exit %d, stdout %q, stderr %q", id, code, out, errOut)
		}
	}
	l, err := learn.Open(st.Root(), dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, ls := range l.Lessons() {
		if _, bad := refused[ls.ID]; bad && ls.Tier != learn.TierProposed {
			t.Errorf("%s is %s, want proposed", ls.ID, ls.Tier)
		}
	}

	if err := l.Approve("ign-1", time.Now()); err != nil { // approved before approval ran the check
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	_, out, _ = learnRun(t)
	if !strings.Contains(out, "ign-1 probation:") || !strings.Contains(out, "    cannot reach the agent: ") || strings.Count(out, "cannot reach the agent") != 1 {
		t.Errorf("the list does not name the approved lesson that cannot arrive, alone: %q", out)
	}
}

// TestLRN17k_ImportSaysWhenAnApprovedLessonIsSent: the import line says what
// the consent screen says: an approved lesson is sent with a prompt that
// carries no preflight note, and waits while one is sent.
func TestLRN17k_ImportSaysWhenAnApprovedLessonIsSent(t *testing.T) {
	bodiesHere(t)
	file := filepath.Join(t.TempDir(), "shared.json")
	if err := os.WriteFile(file, []byte(sharedLessons), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := learnRun(t, "import", file)
	if code != 0 || !strings.Contains(out, "sent with a prompt that carries no preflight note, at most one per prompt and each lesson once a session, and waits while a preflight note is sent") {
		t.Errorf("import: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// TestLRN17w_WithdrawIsTypedAtTheTerminal (#156 D3): `learn withdraw ID`
// stops one approved lesson, asked as approval is: the lesson and a one-time
// code on the terminal, the code typed back there. No terminal or a wrong
// code withdraws nothing; the right one sends the lesson back to proposed,
// where it no longer reaches the agent, and approval takes it again. A
// lesson that is not approved has nothing to withdraw.
func TestLRN17w_WithdrawIsTypedAtTheTerminal(t *testing.T) {
	bodiesHere(t)
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

	atTerminal(t, nil, errors.New("no tty"))
	if code, out, errOut := learnRun(t, "withdraw", "npm-ci"); code != 1 || out != "" || !strings.Contains(errOut, "no terminal to ask on, so nothing was withdrawn") {
		t.Errorf("withdraw with no terminal: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	atTerminal(t, &person{answer: "1234567"}, nil)
	if code, _, errOut := learnRun(t, "withdraw", "npm-ci"); code != 1 || !strings.Contains(errOut, "the code did not match, so nothing was withdrawn") {
		t.Errorf("withdraw with a wrong code: exit %d, stderr %q", code, errOut)
	}
	if _, out, _ := learnRun(t); !strings.Contains(out, "npm-ci probation:") {
		t.Fatalf("after two refused withdrawals the lesson is not approved: %q", out)
	}
	p := &person{}
	atTerminal(t, p, nil)
	code, out, errOut := learnRun(t, "withdraw", "npm-ci")
	if code != 0 || out != "withdrew npm-ci: it no longer reaches the agent; `rashomon learn approve npm-ci` sends it again\n" {
		t.Fatalf("withdraw with the code: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if !strings.Contains(p.shown.String(), "npm ci before npm test") {
		t.Errorf("the terminal showed %q: want the lesson's text", p.shown.String())
	}
	if _, out, _ := learnRun(t); !strings.Contains(out, "npm-ci proposed:") {
		t.Errorf("after a withdrawal the list says %q; want npm-ci proposed", out)
	}
	if code, _, errOut := learnRun(t, "withdraw", "npm-ci"); code != 1 || !strings.Contains(errOut, "nothing to withdraw") {
		t.Errorf("withdrawing a proposed lesson: exit %d, stderr %q", code, errOut)
	}
	atTerminal(t, &person{}, nil)
	if code, out, errOut := learnRun(t, "approve", "npm-ci"); code != 0 || out != "approved npm-ci\n" {
		t.Errorf("approving again after a withdrawal: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}
