// Command pgraph writes a Go repository's platform graph where the rashomon
// hook reads it (card PL-WIRE1):
//
//	pgraph sync [-timeout d] <root>
//
// loads every Go module at or under root (pgraph.ScanRoots) offline
// (pgraph.LoaderEnv), extracts the PROVEN facts and the ALGORITHMIC dispatch
// facts, reconciles them into the log root/.rashomon/pgraph.log holds, or a
// new one, and writes it back with the per-package file digests the load
// read (root/.rashomon/pgraph.digests). A sync that cannot load every module
// writes nothing: a partial log would read as a program with fewer packages.
//
// It is a command of its own, not a rashomon verb: the rashomon binary links
// no package that starts a process (H-17, TRU-40), so it cannot run this one;
// a person runs it, and the hook reads what it wrote.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/altrace-dev-role/rashomon/pgraph"
)

// defaultTimeout bounds the loads of one sync. Loading this repository's two
// modules with dependency bodies and building their dispatch took under two
// seconds on four cores (pgraph/cha.go's measurement); a minute leaves a
// repository thirty times its size room and still ends a sync that hangs.
const defaultTimeout = time.Minute

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "usage: pgraph sync [-timeout d] <root>\n"

// run is the command: 0 on a sync written, 1 on a sync that failed and wrote
// nothing, 2 on a usage error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "sync" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fl := flag.NewFlagSet("pgraph sync", flag.ContinueOnError)
	fl.SetOutput(stderr)
	timeout := fl.Duration("timeout", defaultTimeout, "the most the loads of one sync may take")
	if err := fl.Parse(args[1:]); err != nil || fl.NArg() != 1 || *timeout <= 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	s, err := syncRoot(ctx, fl.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "pgraph: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "pgraph: synced %s in %s into %s\n", count(s.packages, "package"), count(s.modules, "module"),
		filepath.Join(pgraph.StateDir, pgraph.LogFile))
	fmt.Fprintf(stdout, "pgraph: facts PROVEN +%d -%d, ALGORITHMIC +%d -%d\n",
		s.proven.Asserted, s.proven.Retracted, s.algorithmic.Asserted, s.algorithmic.Retracted)
	return 0
}

// count is n and noun, plural but for one.
func count(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}

// synced is what one sync did, for its two lines of output.
type synced struct {
	modules, packages   int
	proven, algorithmic pgraph.Delta
}

// syncRoot loads root's modules, reconciles the log and writes the log, then
// the digests. In that order: between the two writes the digests are the
// last sync's, so a file changed since reads stale and its package's facts
// unknown; the other order would let new digests vouch for old facts.
func syncRoot(ctx context.Context, root string) (synced, error) {
	abs, err := filepath.Abs(root)
	if err == nil {
		abs, err = filepath.EvalSymlinks(abs)
	}
	if err != nil {
		return synced{}, err
	}
	mods, err := pgraph.ScanRoots(abs)
	if err != nil {
		return synced{}, err
	}
	s := synced{modules: len(mods)}
	progs := make([]*pgraph.Program, 0, len(mods))
	for _, m := range mods {
		p, err := pgraph.Load(ctx, m, pgraph.Config{})
		if err != nil {
			return synced{}, fmt.Errorf("%s: %w", m, err)
		}
		progs = append(progs, p)
		s.packages += len(p.PackagePaths())
	}
	dir := filepath.Join(abs, pgraph.StateDir)
	logPath := filepath.Join(dir, pgraph.LogFile)
	l, err := readLog(logPath)
	if err != nil {
		return synced{}, err
	}
	if s.proven, s.algorithmic, err = pgraph.SyncPrograms(l, progs, snapshot(abs, mods, progs)); err != nil {
		return synced{}, err
	}
	enc := l.Encode()
	if len(enc) > pgraph.MaxLogBytes {
		return synced{}, fmt.Errorf("the log would be %d bytes, over the %d byte cap; nothing was written", len(enc), pgraph.MaxLogBytes)
	}
	digests, err := pgraph.EncodeDigests(abs, progs)
	if err != nil {
		return synced{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return synced{}, err
	}
	if err := ignoreAll(dir); err != nil {
		return synced{}, err
	}
	if err := writeAtomic(logPath, enc); err != nil {
		return synced{}, err
	}
	return s, writeAtomic(filepath.Join(dir, pgraph.DigestsFile), digests)
}

// readLog is the log at path, or an empty one where there is none. A log
// that does not decode is an error and is left as it is: its history is the
// record of what was retracted, and a sync does not start one over silently.
func readLog(path string) (*pgraph.Log, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &pgraph.Log{}, nil
	}
	if err != nil {
		return nil, err
	}
	if fi.Size() > pgraph.MaxLogBytes {
		return nil, fmt.Errorf("%s is %d bytes, over the %d byte cap", path, fi.Size(), pgraph.MaxLogBytes)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	l, err := pgraph.DecodeLog(b)
	if err != nil {
		return nil, fmt.Errorf("%s does not decode (%w); move it aside to start a new log", path, err)
	}
	return l, nil
}

// snapshot is the commit field the facts of this sync carry: the working
// tree it read, which may be uncommitted, named by a digest of each module's
// place under root and its inputs (Program.Digest), rather than a git commit
// the tree may not be at.
func snapshot(root string, mods []string, progs []*pgraph.Program) string {
	h := sha256.New()
	for i, p := range progs {
		rel, _ := filepath.Rel(root, mods[i])
		io.WriteString(h, filepath.ToSlash(rel)+"\x00"+p.Digest()+"\n")
	}
	return "tree:" + hex.EncodeToString(h.Sum(nil))[:16]
}

// ignoreAll writes dir/.gitignore as "*" when there is none, so git leaves
// the directory out of a commit; one already there is the person's and is
// left alone.
func ignoreAll(dir string) error {
	f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString("*\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// writeAtomic replaces path with b through a temporary file in the same
// directory and a rename, so a reader sees the old file or the new one,
// never a torn one.
func writeAtomic(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
