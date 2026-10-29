package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/share"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// cmdShare is `rashomon share build outcomes --out FILE` (PF-SH2): with the
// user's share.outcomes consent it builds the outcomes bundle, prints it in
// full, then one line, the sha256 of those same bytes, and then writes them to
// FILE. What is printed is what is written, byte for byte, so the hash a
// receiver echoes back can be checked against the screen (PF-SH3, the
// receiver's). Nothing is written unless all of it printed, and nothing else
// is printed. rashomon's part ends at the file.
func cmdShare(args []string, stdout io.Writer) error {
	out, err := shareFlags(args)
	if err != nil {
		return err
	}
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	ok, err := consent.Has(root, "share.outcomes")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("sharing outcomes needs your consent first: rashomon consent share.outcomes")
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		return err
	}
	bundle, err := share.BuildOutcomes(st)
	if err != nil {
		return err
	}
	b, err := share.Encode(bundle)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "%ssha256 %x\n", b, sha256.Sum256(b)); err != nil {
		return err
	}
	return writeShareFile(out, b)
}

// shareFlags is share's one verb and unit, build outcomes, and its --out
// FILE, which it requires: a bundle goes nowhere the user did not name.
// Anything else, --help included, is an argument it does not know.
func shareFlags(args []string) (string, error) {
	verb, out := []string{"build", "outcomes"}, ""
	for i := 0; i < len(args); i++ {
		switch {
		case i < len(verb) && args[i] == verb[i]:
		case i < len(verb) || args[i] != "--out":
			return "", fmt.Errorf("unknown argument %q", args[i])
		case i+1 >= len(args):
			return "", errors.New("--out needs a value")
		default:
			out = args[i+1]
			i++
		}
	}
	if len(args) < len(verb) || out == "" {
		return "", errors.New("share expects build outcomes --out FILE")
	}
	return out, nil
}

// writeShareFile writes b to path, 0600, through a temporary file beside it,
// synced and renamed over it, so path holds the whole bundle or what it held
// before. path is the user's own, outside the store, so no store helper writes
// it.
func writeShareFile(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
