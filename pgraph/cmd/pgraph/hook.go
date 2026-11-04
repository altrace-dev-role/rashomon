package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/pgraph"
)

// The walk file: pgraph's own, beside the log and the digests, which no
// reader but this command reads. Its lines are walkHeader, "walk\t" and the
// tree's WalkDigest, then either "log\t" and "digests\t" with the SHA-256 of
// each file as the sync wrote it, or "refused" for a tree a hook's sync
// could not write.
const (
	walkFile   = "pgraph.walk"
	walkHeader = "pgraph-walk v0"
)

// hookStart is where the hook looks for the project: the cwd of the call in
// Claude Code's payload on in, where rashomon's recorder keys the session's
// project from (baseline.KeyOf), else CLAUDE_PROJECT_DIR, else the working
// directory. At most 1 MiB of the payload is read, and a terminal is not
// read at all, so a person who types the command is not left waiting.
func hookStart(in io.Reader) string {
	if f, ok := in.(*os.File); ok {
		if fi, err := f.Stat(); err != nil || fi.Mode()&os.ModeCharDevice != 0 {
			in = nil
		}
	}
	if in != nil {
		var p struct {
			CWD string `json:"cwd"`
		}
		b, _ := io.ReadAll(io.LimitReader(in, 1<<20))
		if json.Unmarshal(b, &p) == nil && p.CWD != "" {
			return p.CWD
		}
	}
	if d := os.Getenv("CLAUDE_PROJECT_DIR"); d != "" {
		return d
	}
	return "."
}

// hookSync is a sync as a Claude Code hook runs it, from start. The project
// is the nearest directory at or above start holding .git; without one, or
// without a go.mod at its root, there is nothing to do, so a project that is
// not a Go module costs the hook a walk up and a stat. Under the root's lock
// the tree is walked, and a tree the walk file was written from is not
// loaded again, whether its sync was written or refused: a README edit costs
// a walk, and code that does not compile costs one load, not one per call
// until it does. A refusal is remembered only where .pgraph already is, so a
// project whose first sync fails is left with nothing written. root is the
// project found, for the error line.
func hookSync(ctx context.Context, start string) (root string, err error) {
	root, ok, err := projectRoot(start)
	if err != nil || !ok {
		return root, err
	}
	if fi, err := os.Lstat(filepath.Join(root, "go.mod")); err != nil || !fi.Mode().IsRegular() {
		return root, nil
	}
	unlock, err := lockRoot(ctx, root)
	if err != nil {
		return root, err
	}
	defer unlock()
	walk, err := pgraph.WalkDigest(ctx, root)
	if err != nil {
		return root, err
	}
	dir := filepath.Join(root, pgraph.StateDir)
	if current(dir, walk) {
		return root, nil
	}
	if _, err = syncLocked(ctx, root, walk); err != nil {
		if fi, serr := os.Lstat(dir); serr == nil && fi.IsDir() {
			_ = writeAtomic(filepath.Join(dir, walkFile), encodeWalk(walk, nil, nil))
		}
	}
	return root, err
}

// projectRoot is the nearest directory at or above start that holds a .git
// entry, start made absolute with its symlinks resolved: rashomon's
// baseline.KeyOf rule, spelled again here since pgraph imports nothing of
// rashomon's. ok is false when no ancestor holds one.
func projectRoot(start string) (root string, ok bool, err error) {
	dir, err := filepath.Abs(start)
	if err == nil {
		dir, err = filepath.EvalSymlinks(dir)
	}
	if err != nil {
		return "", false, err
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

// encodeWalk is the walk file for a tree that walked to walk: the sync that
// wrote log and digests, or a refused one where both are nil.
func encodeWalk(walk string, log, digests []byte) []byte {
	head := walkHeader + "\nwalk\t" + walk + "\n"
	if log == nil && digests == nil {
		return []byte(head + "refused\n")
	}
	return []byte(head + "log\t" + sum(log) + "\ndigests\t" + sum(digests) + "\n")
}

// current is whether dir's walk file was written from a tree that walked to
// walk, by a sync that was refused there or by one whose log and digests
// still hold the bytes it wrote. Anything else (no walk file, another
// header, a log moved aside or rewritten since) is not current, and the
// sync that follows rewrites the pair.
func current(dir, walk string) bool {
	b, err := readCapped(filepath.Join(dir, walkFile), 4096)
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) < 3 || lines[0] != walkHeader || lines[1] != "walk\t"+walk {
		return false
	}
	if len(lines) == 3 {
		return lines[2] == "refused"
	}
	if len(lines) != 4 {
		return false
	}
	log, err := readCapped(filepath.Join(dir, pgraph.LogFile), pgraph.MaxLogBytes)
	if err != nil || lines[2] != "log\t"+sum(log) {
		return false
	}
	digests, err := readCapped(filepath.Join(dir, pgraph.DigestsFile), pgraph.MaxLogBytes)
	return err == nil && lines[3] == "digests\t"+sum(digests)
}

// readCapped is the regular file at path, refused over max bytes.
func readCapped(path string, max int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > max {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(path)
}

// sum is the hex SHA-256 of b.
func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
