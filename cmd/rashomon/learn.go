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
  rashomon learn import FILE     add a shared lessons file's rows at proposed

`

// learnTerminal opens the terminal the one-time code is shown and typed on;
// a test replaces it.
var learnTerminal = openTerminal

// cmdLearn is `rashomon learn [approve ID | import FILE]` (card P6), the
// verb the consent screen names (disclose/paragraphs.go).
func cmdLearn(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return listLessons(stdout)
	}
	verb, rest := args[0], args[1:]
	if verb != "approve" && verb != "import" {
		return fmt.Errorf("unknown argument %q", verb)
	}
	if len(rest) != 1 {
		return fmt.Errorf("learn %s needs one argument", verb)
	}
	l, err := openLedger()
	if err != nil {
		return err
	}
	if verb == "import" {
		return importLessons(l, rest[0], stdout)
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

func listLessons(stdout io.Writer) error {
	l, err := openLedger()
	if errors.Is(err, store.ErrNoStore) {
		_, err = fmt.Fprintln(stdout, "no lessons: nothing has been recorded here")
		return err
	}
	if err != nil {
		return err
	}
	if at := l.QuarantinedAt(); !at.IsZero() {
		fmt.Fprintf(stdout, "quarantined %s: a write this command did not make changed the learning state, "+
			"so every lesson went back to proposed\n", at.UTC().Format(time.RFC3339))
	}
	for _, ls := range l.Lessons() {
		t := l.Tally(ls.ID)
		bound := "no bound"
		if lo, hi, ok := t.Bound(); ok {
			bound = fmt.Sprintf("bound [%.3f, %.3f]", lo, hi)
		}
		fmt.Fprintf(stdout, "%s %s: %d fires, %d confirmed, %d refuted, %d unresolved; %s\n    %s\n",
			ls.ID, ls.Tier, t.Fires, t.Confirmed, t.Refuted, t.Unresolved, bound, ls.Text)
	}
	return nil
}

func importLessons(l *learn.Ledger, path string, stdout io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	added, skipped, err := l.Import(f, time.Now())
	if err != nil {
		return err
	}
	if err := l.Save(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "imported %d lessons at proposed, skipped %d; "+
		"each reaches the agent only after `rashomon learn approve ID`\n", added, skipped)
	return err
}

// approveLesson shows the lesson and a one-time code on the terminal, never
// stdout or stdin, and approves only when the code is typed back there: an
// agent's shell has no terminal, so it can neither read the code nor answer.
func approveLesson(l *learn.Ledger, id string, stdout io.Writer) error {
	i := -1
	lessons := l.Lessons()
	for j, ls := range lessons {
		if ls.ID == id {
			i = j
		}
	}
	if i < 0 || lessons[i].Tier != learn.TierProposed {
		return l.Approve(id) // its refusal names the reason
	}
	in, out, closeTTY, err := learnTerminal()
	if err != nil {
		return errors.New("there is no terminal to ask on, so nothing was approved; a person approves a lesson at a terminal")
	}
	defer closeTTY()
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	fmt.Fprintf(out, "lesson %s:\n    %s\nto let it reach the agent, type %s: ", id, lessons[i].Text, code)
	line, _ := bufio.NewReader(in).ReadString('\n')
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(line)), []byte(code)) != 1 {
		return errors.New("the code did not match, so nothing was approved")
	}
	if err := l.Approve(id); err != nil {
		return err
	}
	if err := l.Save(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "approved %s\n", id)
	return err
}
