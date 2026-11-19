// Command pgraph writes a Go repository's platform graph where the rashomon
// ledger verb reads it (card PL-WIRE1):
//
//	pgraph sync [-timeout d] <root>
//	pgraph sync -hook [-timeout d]
//
// loads every Go module at or under root (pgraph.ScanRoots) offline
// (pgraph.LoaderEnv), extracts the PROVEN facts and the ALGORITHMIC dispatch
// facts, reconciles them into the log root/.pgraph/pgraph.log holds, or a new
// one, and writes it back with the per-package file digests the load read
// (root/.pgraph/pgraph.digests). Deleting root/.pgraph forgets both. A sync
// that cannot load every module writes nothing: a partial log would read as
// a program with fewer packages.
//
// It is a command of its own, not a rashomon verb: the rashomon binary links
// no package that starts a process (H-17, TRU-40), so it cannot run this one;
// a person runs it, or Claude Code does from a hook entry (-hook, card
// PW1b-ON-230), and `rashomon report --ledger` reads what it wrote. Every
// sync holds a lock on the root while it reads and writes, and writes a walk
// file last, the digest of the tree it read, which lets a hook skip a tree
// that has not changed since.
//
// What it prints a person fits 80 columns and names no absolute path: the
// root reads as ".", the home directory as "~" and the Go root as $GOROOT. The
// summary keeps the tier names PROVEN and ALGORITHMIC, the words the PRD gives
// people for them (PF-PL4).
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
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

const usage = "usage: pgraph sync [-timeout d] <root>\n" +
	"writes <root>/.pgraph, its graph and file digests; deleting it forgets them\n" +
	"       pgraph sync -hook [-timeout d]: the same, as a Claude Code hook\n"

// columns is the width every line pgraph prints is fitted to.
const columns = 80

// hookInput is what -hook reads Claude Code's payload from.
var hookInput io.Reader = os.Stdin

// loads counts the syncs that went on to load the tree; tests read it to
// tell a sync that loaded from one the walk file let go.
var loads int

// errLocked is a sync that could not take the root's lock before its
// timeout: another sync of the same root is running.
var errLocked = errors.New("another sync of this root is still running; nothing was written")

// run is the command: 0 on a sync written, 1 on a sync that failed and wrote
// nothing, 2 on a usage error. With -hook it is 0 whatever the sync does.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "sync" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	fl := flag.NewFlagSet("pgraph sync", flag.ContinueOnError)
	fl.SetOutput(stderr)
	timeout := fl.Duration("timeout", defaultTimeout, "the most the loads of one sync may take")
	hook := fl.Bool("hook", false, "run as a Claude Code hook: the project the call ran in, nothing on stdout, exit 0")
	roots := 1
	err := fl.Parse(args[1:])
	if *hook {
		roots = 0
	}
	if err != nil || fl.NArg() != roots || *timeout <= 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if *hook {
		if _, err := hookSync(ctx, hookStart(hookInput)); err != nil {
			fmt.Fprintln(stderr, hookLine(ctx, err))
		}
		return 0
	}
	root, err := filepath.Abs(fl.Arg(0))
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err == nil {
		var s synced
		if s, err = syncRoot(ctx, root); err == nil {
			report(stdout, s)
			return 0
		}
	}
	fmt.Fprint(stderr, fitted(sanitize(err.Error(), fl.Arg(0), root)))
	return 1
}

// report is a sync's two lines.
func report(stdout io.Writer, s synced) {
	fmt.Fprintf(stdout, "pgraph: synced %s in %s into %s\n", count(s.packages, "package"), count(s.modules, "module"),
		filepath.Join(pgraph.StateDir, pgraph.LogFile))
	fmt.Fprintf(stdout, "pgraph: facts PROVEN +%d -%d, ALGORITHMIC +%d -%d\n",
		s.proven.Asserted, s.proven.Retracted, s.algorithmic.Asserted, s.algorithmic.Retracted)
}

// sanitize is msg with the root, as given and resolved, read as ".", the home
// directory as "~" and the Go root as $GOROOT, and without the "pgraph: " a
// library error leads with, wherever a wrapper put it, which the line adds
// once.
func sanitize(msg, given, root string) string {
	if abs, err := filepath.Abs(given); err == nil {
		msg = strings.ReplaceAll(msg, abs, ".")
	}
	if root != "" {
		msg = strings.ReplaceAll(msg, root, ".")
	}
	if g := runtime.GOROOT(); g != "" {
		msg = strings.ReplaceAll(msg, g, "$GOROOT")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" && h != "/" {
		msg = strings.ReplaceAll(msg, h, "~")
	}
	return strings.ReplaceAll(msg, "pgraph: ", "")
}

// fitted is msg as "pgraph: " lines of at most columns, broken at spaces and,
// where one word is longer than a line, inside it; continuation lines are
// indented under the first's text.
func fitted(msg string) string {
	const lead = "pgraph: "
	width := columns - len(lead)
	var b strings.Builder
	line := ""
	flush := func() {
		if b.Len() == 0 {
			b.WriteString(lead)
		} else {
			b.WriteString(strings.Repeat(" ", len(lead)))
		}
		b.WriteString(line + "\n")
		line = ""
	}
	for _, w := range strings.Fields(msg) {
		for len(w) > width {
			if line != "" {
				flush()
			}
			line = w[:width]
			flush()
			w = w[width:]
		}
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= width:
			line += " " + w
		default:
			flush()
			line = w
		}
	}
	if line != "" || b.Len() == 0 {
		flush()
	}
	return b.String()
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

// syncRoot is a person's sync of abs, a root made absolute and
// symlink-resolved: under the root's lock, the walk, then the sync.
func syncRoot(ctx context.Context, abs string) (synced, error) {
	unlock, err := lockRoot(ctx, abs)
	if err != nil {
		return synced{}, err
	}
	defer unlock()
	walk, err := pgraph.WalkDigest(ctx, abs)
	if err != nil {
		return synced{}, err
	}
	return syncLocked(ctx, abs, walk)
}

// syncLocked loads the modules of abs, whose lock the caller holds and whose
// tree walked to walk before the load, reconciles the log and writes the
// log, then the digests, then the walk file. In that order: between the
// first two writes the digests are the last sync's, so a file changed since
// reads stale and its package's facts unknown; the other order would let
// new digests vouch for old facts. The walk file is last, since it vouches
// for both.
func syncLocked(ctx context.Context, abs, walk string) (synced, error) {
	loads++
	mods, err := pgraph.ScanRoots(abs)
	if err != nil {
		return synced{}, err
	}
	s := synced{modules: len(mods)}
	progs := make([]*pgraph.Program, 0, len(mods))
	for _, m := range mods {
		p, err := pgraph.Load(ctx, m, pgraph.Config{})
		if err != nil {
			rel, _ := filepath.Rel(abs, m)
			return synced{}, fmt.Errorf("module %s: %w", filepath.ToSlash(rel), err)
		}
		progs = append(progs, p)
		s.packages += len(p.PackagePaths())
	}
	dir := filepath.Join(abs, pgraph.StateDir)
	logPath := filepath.Join(dir, pgraph.LogFile)
	l, err := readLog(logPath, filepath.Join(pgraph.StateDir, pgraph.LogFile))
	if err != nil {
		return synced{}, err
	}
	if s.proven, s.algorithmic, err = pgraph.SyncPrograms(l, progs, snapshot(abs, mods, progs)); err != nil {
		return synced{}, err
	}
	enc := l.Encode()
	if len(enc) > pgraph.MaxLogBytes {
		return synced{}, fmt.Errorf("the log would be %w; nothing was written", &overCapError{Size: int64(len(enc)), Max: pgraph.MaxLogBytes})
	}
	digests, err := pgraph.EncodeDigests(abs, progs)
	if err != nil {
		return synced{}, err
	}
	symbols, err := pgraph.EncodeSymbols(abs, progs, digests)
	if err != nil {
		return synced{}, err
	}
	sd, err := stateDir(abs, true)
	if err != nil {
		return synced{}, err
	}
	defer sd.Close()
	if err := ignoreAll(sd); err != nil {
		return synced{}, err
	}
	removeCut(sd)
	if err := writeAtomic(sd, pgraph.LogFile, enc); err != nil {
		return synced{}, err
	}
	if err := writeAtomic(sd, pgraph.DigestsFile, digests); err != nil {
		return synced{}, err
	}
	if err := writeAtomic(sd, pgraph.SymbolsFile, symbols); err != nil {
		return synced{}, err
	}
	return s, writeAtomic(sd, walkFile, encodeWalk(walk, enc, digests, symbols))
}

// stateDir is abs's .pgraph held as a root, made first when create is set
// and it is not there. Every write of a sync goes through it: an os.Root
// resolves each name under the directory it holds, a step at a time with
// O_NOFOLLOW, and follows a link only while it stays inside, so a .pgraph
// committed as a link out of the tree is refused, not written through, and
// no name is checked first and opened after. A link that stays inside abs
// is followed. Mkdir never follows its last name, so a link there is left
// for OpenRoot to refuse. WRITE-SAFE1 gives the root module's writes under
// a clone root the same form; a change to one is a change to both.
func stateDir(abs string, create bool) (*os.Root, error) {
	r, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if create {
		if err := r.Mkdir(pgraph.StateDir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return r.OpenRoot(pgraph.StateDir)
}

// removeCut removes from sd the temp files of a sync cut off inside
// writeAtomic, which Claude Code does to a background hook still running
// when the session exits: regular files named ".<name>.tmp-*" for each file
// writeAtomic writes. The caller holds the root lock, so no live sync has
// one open. A file it cannot remove stays for the next sync.
func removeCut(sd *os.Root) {
	d, err := sd.Open(".")
	if err != nil {
		return
	}
	ents, err := d.ReadDir(-1)
	d.Close()
	if err != nil {
		return
	}
	for _, e := range ents {
		for _, name := range []string{pgraph.LogFile, pgraph.DigestsFile, pgraph.SymbolsFile, walkFile} {
			if e.Type().IsRegular() && strings.HasPrefix(e.Name(), "."+name+".tmp-") {
				_ = sd.Remove(e.Name())
			}
		}
	}
}

// readLog is the log at path, named name in an error, or an empty one where
// there is none. A log that does not decode is an error and is left as it
// is: its history is the record of what was retracted, and a sync does not
// start one over silently. The log is read by readCapped, so a named pipe
// or a link at the name is refused and never opened (a pipe would hold the
// sync in its open, a link to a device could read without end), and a log
// over the cap is refused on its size before any read.
func readLog(path, name string) (*pgraph.Log, error) {
	b, err := readCapped(path, pgraph.MaxLogBytes)
	var oc *overCapError
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return &pgraph.Log{}, nil
	case errors.As(err, &oc):
		return nil, fmt.Errorf("%s is %w", name, oc)
	case errors.Is(err, errNotRegular), errors.Is(err, errReplaced):
		return nil, fmt.Errorf("%s is %w; move it aside to start a new log", name, errNotRegular)
	case err != nil:
		return nil, err
	}
	l, err := pgraph.DecodeLog(b)
	if err != nil {
		return nil, fmt.Errorf("%s does not decode (%w); move it aside to start a new log", name, err)
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

// ignoreAll writes sd's .gitignore as "*" when there is none, so git leaves
// the directory out of a commit; one already there is the person's and is
// left alone.
func ignoreAll(sd *os.Root) error {
	f, err := sd.OpenFile(".gitignore", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
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

// writeAtomic replaces name in sd with b through a temporary file beside it
// and a rename, so a reader sees the old file or the new one, never a torn
// one. The rename replaces a link at name, never what it names.
func writeAtomic(sd *os.Root, name string, b []byte) error {
	f, tmp, err := createTemp(sd, name)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	// The mode is set on the open file, never by its name: a change by name
	// after the Close would change whatever file holds the name by then.
	// createTemp makes the file 0600; pgraph's files are read as 0644.
	if err == nil {
		err = f.Chmod(0o644)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = sd.Rename(tmp, name)
	}
	if err != nil {
		sd.Remove(tmp)
	}
	return err
}

// createTemp is os.CreateTemp in sd, which an os.Root does not offer: a new
// file ".<name>.tmp-" and a random number, made 0600 by an exclusive create
// and made again under another number if one holds that name. removeCut
// matches the same form.
func createTemp(sd *os.Root, name string) (*os.File, string, error) {
	for try := 0; ; try++ {
		tmp := "." + name + ".tmp-" + strconv.FormatUint(uint64(rand.Uint32()), 10)
		f, err := sd.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) && try < 10000 {
			continue
		}
		return f, tmp, err
	}
}
