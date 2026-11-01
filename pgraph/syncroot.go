package pgraph

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Where a sync writes, relative to its sync root (card PL-WIRE1). The
// rashomon hook reads both files at the same place and imports nothing from
// here: the names are the boundary, as LogHeader is, and its own copies are
// pinned against these by a test that runs this command and then the hook.
//
// The directory is in the repository's tree, not in rashomon's store. The
// log is names read from the tree's own source (packages, functions, the
// calls between them), and the store's first rule is that it records no
// content; next to the source it holds nothing the tree does not already
// hold. Its name starts with a dot, which the go tool skips when it resolves
// ./..., so the log never becomes part of the graph it describes, and the
// command writes a .gitignore of "*" into it so a commit of the tree does not
// pick it up.
const (
	StateDir    = ".rashomon"
	LogFile     = "pgraph.log"
	DigestsFile = "pgraph.digests"
)

// DigestsHeader is the first line of a digests file: one line per Go file a
// sync read, "package path sha256", tab-separated, path relative to the sync
// root and slash-separated, in path order. A reader compares a file's content
// now against it to tell whether the log's facts about its package still hold
// (PF-PL6, first half). A change of line meaning takes a new header.
const DigestsHeader = "pgraph-digests v0"

// MaxModules bounds the go.mod files one sync root may hold.
const MaxModules = 256

// ScanRoots is every directory at or under root that holds a go.mod, root
// first when it holds one, then in path order: the scan roots one sync
// loads. Directories the go tool ignores (testdata, and names starting with
// . or _) and vendor are not walked, so a fixture module is never a scan
// root; symbolic links are not followed. None is an error, and so is more
// than MaxModules.
func ScanRoots(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if n := d.Name(); p != root && (n == "testdata" || n == "vendor" || strings.HasPrefix(n, ".") || strings.HasPrefix(n, "_")) {
			return filepath.SkipDir
		}
		if fi, err := os.Lstat(filepath.Join(p, "go.mod")); err == nil && fi.Mode().IsRegular() {
			if len(out) == MaxModules {
				return fmt.Errorf("pgraph: more than %d modules under %s", MaxModules, root)
			}
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("pgraph: no go.mod at or under %s", root)
	}
	return out, nil
}

// SyncPrograms reconciles the PROVEN facts of every program in progs, then
// their ALGORITHMIC dispatch facts, into l at commit, as one program per
// tier: Reconcile retracts whatever a producer held under a config and was
// not given, so syncing module by module would retract each module's facts
// in the next one's pass. Every program must have been loaded under one
// Config, with dependency bodies (Load). Dispatch runs per program, so a call
// from one module into another is no may-call fact.
func SyncPrograms(l *Log, progs []*Program, commit string) (proven, algorithmic Delta, err error) {
	if len(progs) == 0 {
		return Delta{}, Delta{}, errors.New("pgraph: no program to sync")
	}
	cfg := progs[0].cfg.String()
	for _, p := range progs {
		if p.cfg.String() != cfg {
			return Delta{}, Delta{}, fmt.Errorf("pgraph: programs loaded under %s and %s", cfg, p.cfg.String())
		}
	}
	var pt, at []Triple
	for _, p := range progs {
		ts, err := Dispatch(p)
		if err != nil {
			return Delta{}, Delta{}, err
		}
		pt = append(pt, Extract(p)...)
		at = append(at, ts...)
	}
	if proven, err = l.Reconcile(gotypesProducer(), cfg, commit, pt); err != nil {
		return Delta{}, Delta{}, err
	}
	algorithmic, err = l.Reconcile(chaProducer(), cfg, commit, at)
	return proven, algorithmic, err
}

// digestLine is one line of a digests file.
type digestLine struct{ pkg, path, sum string }

// EncodeDigests is the digests file of progs read under root, the sync root
// they were found under (ScanRoots): every Go file each program's packages
// compiled, with its package's import path and the sha256 Load hashed it to.
// Test files are no part of a Program and so of no line: the facts are not
// read from them.
func EncodeDigests(root string, progs []*Program) ([]byte, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	var lines []digestLine
	for _, p := range progs {
		sums := make(map[string]string, len(p.files))
		for _, f := range p.files {
			sums[f.Path] = f.SHA256
		}
		for _, pk := range p.pkgs {
			for _, f := range pk.CompiledGoFiles {
				inMod, err := normaliseUnder(p.root, f)
				if err != nil {
					return nil, err
				}
				rel, err := normaliseUnder(real, f)
				if err != nil {
					return nil, err
				}
				lines = append(lines, digestLine{pk.PkgPath, rel, sums[inMod]})
			}
		}
	}
	return encodeDigests(lines)
}

// encodeDigests renders lines under DigestsHeader in path order, refusing a
// field the format cannot carry (empty, a tab or a newline in it).
func encodeDigests(lines []digestLine) ([]byte, error) {
	sort.Slice(lines, func(i, j int) bool { return lines[i].path < lines[j].path })
	var b strings.Builder
	b.WriteString(DigestsHeader + "\n")
	for _, l := range lines {
		for _, f := range [...]string{l.pkg, l.path, l.sum} {
			if err := checkField("digest field", f); err != nil {
				return nil, err
			}
		}
		b.WriteString(l.pkg + "\t" + l.path + "\t" + l.sum + "\n")
	}
	return []byte(b.String()), nil
}
