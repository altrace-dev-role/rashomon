package main

import (
	"os"

	"github.com/altrace-dev-role/rashomon/internal/platform"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// platformRead is StopInput.Platform for the Stop and its replay (card
// PL-WIRE1): internal/platform's Read of the pgraph log `pgraph sync <root>`
// wrote under the project root, LogPath filled only where that file exists,
// so a project nobody synced reads as no log, every family unknown, rather
// than as an unreadable one. The Stop writes no file of the platform graph's:
// pgraph is a command of its own that a person runs, and this binary starts
// no process (H-17).
func platformRead(root string, written []string) verify.PlatformRead {
	run := platform.RunView{Root: root, WrittenPaths: written}
	if root == "" {
		return platform.Read(run)
	}
	if lp := platform.LogPath(root); exists(lp) {
		run.LogPath = lp
	}
	return platform.Read(run)
}

// exists reports whether something is at p.
func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
