package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/share"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// shareUsage is the one form `rashomon share` takes in this release.
const shareUsage = "usage: rashomon share build outcomes --out <file>"

// errNoOutcomes is what the outcomes reader says until Phase 2 writes the
// labels it reads (fired, fixed, ignored, overridden): there is nothing to
// count, and a bundle of zeros would say something false.
var errNoOutcomes = errors.New("this release does not record outcomes yet, so there is nothing to build; nothing was written")

// noOutcomes is the share.Reader cmdShare wires today. It is replaced by the
// reader over Phase 2's data when that lands; shareBuild takes the reader as
// a value so neither the command's logic nor its tests change then.
type noOutcomes struct{}

func (noOutcomes) Outcomes() (share.Outcomes, error) { return share.Outcomes{}, errNoOutcomes }

// cmdShare is `rashomon share build outcomes --out <file>` (Task 4.3).
func cmdShare(args []string, stdout io.Writer) error {
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	return shareBuild(root, noOutcomes{}, args, stdout)
}

// shareBuild parses the command, checks the share.outcomes unit is on in
// root's consent.json, builds the bundle from rd and writes it to the --out
// file. It prints the bundle in full, then its SHA-256, then writes the same
// bytes, so what was on screen is what is in the file. The file is created
// first, exclusively and at mode 0600, so a path that cannot be written, or a
// file that is already there, is refused before anything is shown; it is
// removed if anything after that fails. Nothing is written in a refusal.
func shareBuild(root string, rd share.Reader, args []string, stdout io.Writer) error {
	out, err := shareArgs(args)
	if err != nil {
		return err
	}
	on, err := consent.Has(root, consent.UnitShareOutcomes)
	if err != nil {
		return err
	}
	if !on {
		return fmt.Errorf("%s is off, so there is nothing to build; read what it lets this command do and turn it on with: rashomon consent %s; nothing was written",
			consent.UnitShareOutcomes, consent.UnitShareOutcomes)
	}
	bundle, err := share.Build(rd)
	if err != nil {
		return err
	}
	data, err := share.Render(bundle)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)

	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("%w; nothing was written", err)
	}
	fail := func(err error) error {
		f.Close()      //nolint:errcheck // the first error is what matters
		os.Remove(out) //nolint:errcheck // best effort: the file holds a part of the bundle at most
		return err
	}
	if _, err := stdout.Write(data); err != nil {
		return fail(err)
	}
	if _, err := fmt.Fprintf(stdout, "sha256: %s\n", hex.EncodeToString(sum[:])); err != nil {
		return fail(err)
	}
	if _, err := f.Write(data); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	_, err = fmt.Fprintf(stdout, "rashomon: wrote %s (%d bytes); rashomon sends nothing anywhere, so sharing it is your call\n", out, len(data))
	return err
}

// shareArgs reads `build outcomes --out <file>` (or --out=<file>) and nothing
// else: another bundle, another flag or a missing file name is the usage line.
func shareArgs(args []string) (out string, err error) {
	if len(args) < 2 || args[0] != "build" || args[1] != "outcomes" {
		return "", errors.New(shareUsage)
	}
	rest := args[2:]
	switch {
	case len(rest) == 2 && rest[0] == "--out":
		out = rest[1]
	case len(rest) == 1 && strings.HasPrefix(rest[0], "--out="):
		out = strings.TrimPrefix(rest[0], "--out=")
	}
	if out == "" {
		return "", errors.New(shareUsage)
	}
	return out, nil
}
