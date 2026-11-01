package main

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// learnUsage is the usage screen's learn entries, joined as receiptUsage
// is, so usage keeps its size under Law 10.
const learnUsage = `  rashomon learn                 list this project's lessons: each one's tier,
                               its labels and the 95% bound that moves it
  rashomon learn approve ID      show lesson ID and a one-time code on the
                               terminal; the lesson may reach the agent only
                               once that code is typed back there
  rashomon learn withdraw ID     the same, to stop an approved lesson: once
                               the code is typed back, it no longer reaches
                               the agent until it is approved again
  rashomon learn import FILE     add a shared lessons file's rows at proposed

`

// learnTerminal opens the terminal the one-time code is shown and typed on;
// a test replaces it.
var learnTerminal = openTerminal

// cmdLearn is `rashomon learn [approve ID | withdraw ID | import FILE]`
// (cards P6 and LEARN-DX1), the verb the consent screen names
// (disclose/paragraphs.go).
func cmdLearn(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return listLessons(stdout)
	}
	verb, rest := args[0], args[1:]
	if verb != "approve" && verb != "withdraw" && verb != "import" {
		return fmt.Errorf("unknown argument %q", verb)
	}
	if len(rest) != 1 {
		return fmt.Errorf("learn %s needs one argument", verb)
	}
	l, err := openLedger()
	if err != nil {
		return err
	}
	switch verb {
	case "import":
		return importLessons(l, rest[0], stdout)
	case "withdraw":
		return withdrawLesson(l, rest[0], stdout)
	}
	return approveLesson(l, rest[0], stdout)
}

// openLedger opens the working directory's project's ledger in an existing
// store; it does not make a store to hold one.
func openLedger() (*learn.Ledger, error) {
	st, err := openStoreForRead()
	if err != nil {
		return nil, err
	}
	_, dir, err := bodiesProject()
	if err != nil {
		return nil, err
	}
	return learn.Open(st.Root(), dir, time.Now())
}

// openLedgerReadOnly is openLedger through learn.OpenReadOnly: a listing
// must not rewrite the ledger a mismatch would quarantine.
func openLedgerReadOnly() (*learn.Ledger, error) {
	st, err := openStoreForRead()
	if err != nil {
		return nil, err
	}
	_, dir, err := bodiesProject()
	if err != nil {
		return nil, err
	}
	return learn.OpenReadOnly(st.Root(), dir)
}

func listLessons(stdout io.Writer) error {
	l, err := openLedger()
	if errors.Is(err, store.ErrNoStore) {
		_, err = fmt.Fprintln(stdout, "no lessons: nothing has been recorded here")
		return err
	}
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, quarantineLine(l.QuarantinedAt()))
	printLessons(stdout, l.Lessons(), l.Tally)
	return nil
}

// printLessons is the listing's rows, each lesson with its tally, and under
// it why an approved one is not sent (notSentLine).
func printLessons(w io.Writer, lessons []learn.Lesson, tally func(id string) learn.Tally) {
	for _, ls := range lessons {
		t := tally(ls.ID)
		bound := "no bound"
		if lo, hi, ok := t.Bound(); ok {
			bound = fmt.Sprintf("bound [%.3f, %.3f]", lo, hi)
		}
		fmt.Fprintf(w, "%s %s: %d fires, %d confirmed, %d refuted, %d unresolved; %s\n    %s\n",
			ls.ID, ls.Tier, t.Fires, t.Confirmed, t.Refuted, t.Unresolved, bound, ls.Text)
		fmt.Fprint(w, notSentLine(ls))
	}
}

// notSentLine is the line under a lesson past proposed that is not sent, ""
// for any other: text the check refuses, or else an approval a build before
// the stamp made, which Pick never sends (#156 D2). A refused text is not
// offered approval again: approveLesson runs the same check first and would
// refuse it too (LEARN-DX1 F1). learn's listing and lessons both print it
// (LEARN-DX1 F2).
func notSentLine(ls learn.Lesson) string {
	if ls.Tier == learn.TierProposed {
		return ""
	}
	if _, refused := learn.Message(ls); refused != nil {
		return fmt.Sprintf("    cannot reach the agent: %v\n", refused)
	}
	if ls.AwaitsApproval() {
		return fmt.Sprintf("    approved before this build; approve again to send it: `rashomon learn approve %s`\n", ls.ID)
	}
	return ""
}

func importLessons(l *learn.Ledger, path string, stdout io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	had := map[string]bool{}
	for _, ls := range l.Lessons() {
		had[ls.ID] = true
	}
	added, skipped, err := l.Import(f, time.Now())
	if err != nil {
		return err
	}
	if err := l.Save(); err != nil {
		return err
	}
	if _, err = fmt.Fprintf(stdout, "imported %d lessons at proposed, skipped %d; "+
		"an approved lesson is sent with a prompt, at most one per prompt and each lesson once a session: "+
		"as the last line of that prompt's preflight note when the prompt names a path the lesson names, "+
		"and otherwise with the next prompt that carries no preflight note; `rashomon learn approve ID` approves one\n", added, skipped); err != nil {
		return err
	}
	// Import admits text approval refuses (2,048 bytes in any charset), so
	// each row that can never be approved is named now, not at approval.
	for _, ls := range l.Lessons() {
		if _, merr := learn.Message(ls); merr != nil && !had[ls.ID] {
			if _, err := fmt.Fprintf(stdout, "lesson %s cannot reach the agent: %v\n", ls.ID, merr); err != nil {
				return err
			}
		}
	}
	return nil
}

// approveLesson shows the lesson and a one-time code on the terminal, never
// stdout or stdin, and approves only when the code is typed back there: an
// agent's shell has no terminal, so it can neither read the code nor answer.
// A lesson whose text cannot reach the agent (learn.Message: the renderer's
// slot checks and the host screen) is refused before the code is shown, so
// a person approves only text that arrives (L8).
func approveLesson(l *learn.Ledger, id string, stdout io.Writer) error {
	ls, ok := lessonByID(l, id)
	if !ok || !ls.AwaitsApproval() {
		return l.Approve(id, time.Now()) // its refusal names the reason
	}
	if _, err := learn.Message(ls); err != nil {
		return fmt.Errorf("lesson %s cannot reach the agent: %w; nothing was approved", id, err)
	}
	if err := typedAtTerminal(ls, "to let it reach the agent", "approved", "approves"); err != nil {
		return err
	}
	if err := l.Approve(id, time.Now()); err != nil {
		return err
	}
	if err := l.Save(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "approved %s\n", id)
	return err
}

// withdrawLesson stops one approved lesson (#156 D3), asked as approval is:
// the lesson and a one-time code on the terminal, the code typed back
// there. The lesson goes back to proposed and leaves active.json; approval
// takes it again.
func withdrawLesson(l *learn.Ledger, id string, stdout io.Writer) error {
	ls, ok := lessonByID(l, id)
	if !ok || ls.Tier == learn.TierProposed {
		return l.Withdraw(id) // its refusal names the reason
	}
	if err := typedAtTerminal(ls, "to stop it reaching the agent", "withdrawn", "withdraws"); err != nil {
		return err
	}
	if err := l.Withdraw(id); err != nil {
		return err
	}
	if err := l.Save(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "withdrew %s: it no longer reaches the agent; `rashomon learn approve %s` sends it again\n", id, id)
	return err
}

func lessonByID(l *learn.Ledger, id string) (learn.Lesson, bool) {
	for _, ls := range l.Lessons() {
		if ls.ID == id {
			return ls, true
		}
	}
	return learn.Lesson{}, false
}

// typedAtTerminal shows the lesson and a one-time code on the terminal,
// never stdout or stdin, and returns nil only when the code is typed back
// there: an agent's shell has no terminal, so it can neither read the code
// nor answer. to leads the prompt's "type CODE"; done and does fill the
// refusals: "nothing was approved; a person approves a lesson at a terminal".
func typedAtTerminal(ls learn.Lesson, to, done, does string) error {
	in, out, closeTTY, err := learnTerminal()
	if err != nil {
		return fmt.Errorf("there is no terminal to ask on, so nothing was %s; a person %s a lesson at a terminal", done, does)
	}
	defer closeTTY()
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	fmt.Fprintf(out, "lesson %s:\n    %s\n%s, type %s: ", ls.ID, ls.Text, to, code)
	line, _ := bufio.NewReader(in).ReadString('\n')
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(line)), []byte(code)) != 1 {
		return fmt.Errorf("the code did not match, so nothing was %s", done)
	}
	return nil
}
