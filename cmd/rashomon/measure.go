package main

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// measureUsage is the usage screen's measure entry, joined as receiptUsage
// is, so usage keeps its size under Law 10.
const measureUsage = `  rashomon measure --prepare [--home DIR]
                               mark the store at DIR (default: this
                               process's) as a measurement lab, the only
                               store whose hooks honour RASHOMON_STEER_ARM;
                               writes that one file

`

// cmdMeasure is `rashomon measure` (card MEASURE1): --prepare marks a store
// as a lab. It runs nothing: the driver starts the sessions and runs the
// oracle.
func cmdMeasure(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("measure needs --prepare")
	}
	switch args[0] {
	case "--prepare":
		return measurePrepare(args[1:], stdout)
	}
	return fmt.Errorf("unknown argument %q", args[0])
}

// measurePrepare writes the lab marker (store.WriteLab) into the store root
// --home names, else this process's, and says so. It is the spec's
// `watch --lab` form, apart from watch so the owner's act stays watch's own.
func measurePrepare(args []string, stdout io.Writer) error {
	root := ""
	switch {
	case len(args) == 2 && args[0] == "--home" && args[1] != "":
		abs, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		root = abs
	case len(args) != 0:
		return fmt.Errorf("unknown argument %q", args[0])
	default:
		r, err := store.DefaultRoot()
		if err != nil {
			return err
		}
		root = r
	}
	if err := store.WriteLab(root); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "rashomon measure: %s is a lab store: its hooks honour %s\n", root, hook.EnvSteerArm)
	return err
}
