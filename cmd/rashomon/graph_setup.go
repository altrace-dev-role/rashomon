package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/rootio"
)

// beforeSetupWrite runs just before graph index creates the configuration it
// inferred, with the root: a variable so a test can put a racing writer's
// file on the name between the read that found none and the create.
var beforeSetupWrite = func(string) {}

// maxLeftNamed is how many left-out paths graph index names, one to a line;
// past it one line counts the rest.
const maxLeftNamed = 20

// graphConfig is the configuration graph index builds root under. When root
// has a configuration file, or anything at its name that index.ReadConfig
// refuses, it is ReadConfig's answer and nothing is written. When nothing is
// there, index.Infer infers one: each unit is named with the manifest and
// rule it came from, each path left out is named up to maxLeftNamed, and the
// file is created by rootio.CreateFile, which writes only where nothing is
// at the name and through no link, so a person's file, or a racing
// writer's, is read and never overwritten; then the file is read back as
// any configuration is. A unit-less or over-cap inference writes nothing.
// Only the person-typed `rashomon graph` reaches here (graphIndex); no hook
// writes the file.
func graphConfig(root string, stdout io.Writer) (index.Config, error) {
	cfg, err := index.ReadConfig(root)
	if !errors.Is(err, fs.ErrNotExist) {
		return cfg, err
	}
	inf, err := index.Infer(root)
	for _, u := range inf.Units {
		fmt.Fprintf(stdout, "inferred unit %s (%s) from %s: %s\n", shown(u.Path), u.Lang, shown(u.From), u.Rule)
	}
	for _, l := range inf.Left[:min(len(inf.Left), maxLeftNamed)] {
		fmt.Fprintf(stdout, "left out %s: %s\n", shown(l.Path), shown(l.Why))
	}
	if n := len(inf.Left) - maxLeftNamed; n > 0 {
		fmt.Fprintf(stdout, "left out %d more path(s)\n", n)
	}
	if err != nil {
		return index.Config{}, err
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return index.Config{}, err
	}
	defer r.Close() //nolint:errcheck // the root holds no write of its own
	beforeSetupWrite(root)
	err = rootio.CreateFile(r, filepath.FromSlash(index.ConfigFile), 0o644, index.MaxConfigBytes, func(w io.Writer) error {
		_, err := w.Write(inf.Bytes)
		return err
	})
	var left *rootio.LeftError
	switch {
	case errors.Is(err, fs.ErrExist):
		cfg, rerr := index.ReadConfig(root)
		if errors.Is(rerr, fs.ErrNotExist) {
			return cfg, fmt.Errorf("%w; it is a link to nothing, and rashomon writes no file through a link", err)
		}
		fmt.Fprintf(stdout, "not written: %s appeared before the write; reading it\n", index.ConfigFile)
		return cfg, rerr
	case errors.As(err, &left):
		fmt.Fprintf(stdout, "warning: %s was left beside the configuration; remove it\n", shown(filepath.ToSlash(left.Temp)))
	case err != nil:
		return index.Config{}, err
	}
	fmt.Fprintf(stdout, "wrote %s: %d unit(s); review it, then commit it\n", index.ConfigFile, len(inf.Units))
	return index.ReadConfig(root)
}
