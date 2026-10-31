package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/report/gather"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// whyUsage is the usage screen's why entry (a const of its own, joined in
// verbs.go).
const whyUsage = `  rashomon why [--session ID|last] [--json] [--redact] ID-PREFIX
                               why one note fired: its pattern and key, when
                               and after which call, who was shown it, the
                               calls it cites and the gate or rule it came
                               from; ID-PREFIX is six or more characters of
                               the id the report prints beside the note

`

// cmdWhy prints why one steer line fired. It reads: the store is opened
// without creating one, no report is built and no ledger is opened, so the
// verb writes nothing. With no --session it asks about `last`, the newest
// session of this project that holds a checkpoint.
func cmdWhy(args []string, stdout io.Writer) error {
	session, prefix := lastSessionWord, ""
	asJSON, redact := false, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--session":
			if i+1 >= len(args) {
				return errors.New("--session needs a value")
			}
			session = args[i+1]
			i++
		case "--json":
			asJSON = true
		case "--redact":
			redact = true
		default:
			if len(a) > 0 && a[0] == '-' || prefix != "" {
				return fmt.Errorf("unknown argument %q", a)
			}
			prefix = a
		}
	}
	if prefix == "" {
		return errors.New("why needs an id prefix: the id the report prints beside a note")
	}
	session, err := sessionArg(session)
	if err != nil {
		return err
	}
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return errors.New("nothing has been recorded here, so there is nothing to explain")
	}
	if err != nil {
		return err
	}
	v, err := gather.Why(st, session, prefix)
	if err != nil {
		return err
	}
	if t := recap.TitleOf(v.Pattern); t != "note" {
		v.Title = t
	}
	view := &v
	if redact {
		view = report.RedactWhy(view, st.Key())
	}
	if asJSON {
		return printJSON(stdout, view)
	}
	return report.WhyText(stdout, view)
}
