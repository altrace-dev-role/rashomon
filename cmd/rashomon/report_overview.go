package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The overview of one session (card RPT-1): which runs of `rashomon report`
// print it, and how it is written.

// reportFlag is one flag of a report run and whether the run gave it.
type reportFlag struct {
	on   bool
	name string
}

// overviewTarget is the session whose overview a report run prints, or ""
// for today's text, given the session the run named (after `last` was
// resolved), whether it gave --overview, and its other flags that pick a
// view: --full, --json and the expanding ones, in the order an error names
// them. --redact is not among them: it composes with both views.
//
//   - --overview with any of them is an error naming the one to drop.
//   - --overview alone prints the named session, or the one that last names;
//     where last has none, the error is lastSession's, in its own words.
//   - Any of them without --overview, or a named session, is today's text.
//   - A bare run prints the session that last names, and today's text where
//     nothing is recorded here or no session of the project cut a checkpoint.
func overviewTarget(sessionID string, overview bool, others []reportFlag) (string, error) {
	for _, f := range others {
		switch {
		case f.on && overview:
			return "", fmt.Errorf("--overview takes only --session and --redact: drop %s", f.name)
		case f.on:
			return "", nil
		}
	}
	switch {
	case overview && sessionID != "":
		return sessionID, nil
	case overview:
		return lastSession()
	case sessionID != "":
		return "", nil
	}
	id, ok, err := latestSession()
	switch {
	case errors.Is(err, store.ErrNoStore):
		return "", nil
	case err != nil:
		return "", err
	case !ok:
		return "", nil
	}
	return id, nil
}

// reportOverview writes the overview of session id to stdout: the report
// built for that one session, redacted before the renderer sees it when the
// run asked, and painted only when useColor says the reader is a terminal.
func reportOverview(id string, redact bool, stdout io.Writer) error {
	rep, key, err := reportOrEmpty(id, "", time.Now())
	if err != nil {
		return err
	}
	if redact {
		rep = report.Redact(rep, key)
	}
	var opts []report.OverviewOption
	if useColor(stdout) {
		opts = append(opts, report.WithOverviewColor())
	}
	return report.Overview(stdout, rep, opts...)
}

// useColor reports whether the overview is painted: stdout is a file that is
// a character device, NO_COLOR is unset or empty (no-color.org), TERM is not
// dumb, and the platform is not Windows, whose console is not assumed to read
// SGR codes. It reads the mode from the Stat every file offers, so it adds no
// call of its own to the system; a pipe or a regular file is never painted,
// and /dev/null, a character device, is.
func useColor(stdout io.Writer) bool {
	f, ok := stdout.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || runtime.GOOS == "windows" {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
