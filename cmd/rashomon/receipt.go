package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// receiptUsage is the usage screen's receipt entries, joined before the
// hook-invoked part as hookUsage is joined (card 4.1b), so usage keeps its
// size under Law 10.
const receiptUsage = `  rashomon receipt on            print the two git hooks that put a receipt on
                               each commit, and the push line for the notes;
                               installs nothing and runs no git
  rashomon receipt trailer       print the Rashomon-Receipt trailer naming this
                               project's latest checkpoint; nothing if none
  rashomon receipt note DIGEST   print that checkpoint record when DIGEST is
                               its digest

`

// cmdReceipt is `rashomon receipt on | trailer | note DIGEST` (5.5): on
// prints the hook bodies and the push line, trailer the line naming the
// working directory's project's latest checkpoint, and note that record's
// bytes when the trailer named it. It writes nothing and runs nothing: the
// hooks the user installs run git, and only the user pushes (TRU-40).
func cmdReceipt(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("receipt needs on, trailer or note DIGEST")
	}
	verb, rest := args[0], args[1:]
	if verb == "note" {
		if len(rest) != 1 {
			return errors.New("receipt note needs the trailer's DIGEST")
		}
	} else if len(rest) > 0 {
		return fmt.Errorf("unknown argument %q", rest[0])
	}
	switch verb {
	case "on":
		_, err := io.WriteString(stdout, receiptOn)
		return err
	case "trailer":
		record, ok, err := latestCheckpoint()
		if err != nil || !ok {
			return err
		}
		_, err = fmt.Fprintln(stdout, receipt.Trailer(record))
		return err
	case "note":
		record, ok, err := latestCheckpoint()
		if err != nil {
			return err
		}
		if !ok || receipt.Digest(record) != rest[0] {
			return fmt.Errorf("this project's latest checkpoint is not %q; no note", rest[0])
		}
		_, err = fmt.Fprintf(stdout, "%s\n", record)
		return err
	}
	return fmt.Errorf("unknown argument %q", verb)
}

// latestCheckpoint is the working directory's project's latest checkpoint
// record. With no store there is none, and a commit made there is not held
// up: the trailer prints nothing.
func latestCheckpoint() ([]byte, bool, error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, false, err
	}
	return receipt.Latest(st, baseline.Key(cwd))
}

// receiptOn is what `rashomon receipt on` prints.
const receiptOn = `rashomon receipt: two git hooks you install by hand. rashomon installs
nothing and runs no git; the hooks do.

Save this as prepare-commit-msg in the directory "git rev-parse --git-path
hooks" names, and make it executable. It names the project's latest
checkpoint in each commit message, as a ` + receipt.TrailerKey + ` trailer:

` + receipt.PrepareCommitMsg + `
Save this as post-commit beside it, executable. It writes the checkpoint
record the trailer names to ` + receipt.NotesRef + ` for the new commit:

` + receipt.PostCommit + `
The note is the checkpoint record as rashomon stored it: gate states, claim
verdicts, and this project's path on this machine. It stays in this
repository until you push it, and nothing pushes it but you:

  git push <remote> ` + receipt.NotesRef + `

A merge guard reads it back with
  git fetch <remote> +` + receipt.NotesRef + `:` + receipt.NotesRef + `
`
