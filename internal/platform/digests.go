package platform

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// Where `pgraph sync <root>` writes root's log and digests, and so where the
// ledger verb reads them (pgraph/syncroot.go StateDir, LogFile, DigestsFile):
// in the repository's tree, under pgraph's own directory, which deleting
// forgets; rashomon writes neither file. The names are the boundary, as
// LogHeader is; the acceptance item that runs pgraph sync and then the
// ledger verb (TestH156_*) fails if the two sides part.
const (
	stateDir    = ".pgraph"
	logFile     = "pgraph.log"
	digestsFile = "pgraph.digests"
)

// LogPath is where pgraph sync writes the log of the repository at root.
func LogPath(root string) string { return filepath.Join(root, stateDir, logFile) }

func digestsPath(root string) string { return filepath.Join(root, stateDir, digestsFile) }

// DigestsHeader is the one digests format this package reads, the first
// line pgraph's EncodeDigests writes: then one line per Go file the sync
// read, "package path sha256", tab-separated, the path relative to the sync
// root. Any other header is a format this package does not know, so every
// package the log holds reads stale.
const DigestsHeader = "pgraph-digests v0"

// The bounds of a digests read (INV-10): the file, one line (three fields at
// pgraph's field cap and two tabs), the files hashed for one Read, and one
// file hashed, pgraph's MaxSourceFileBytes, past which no sync read the file
// either. A package with a file past maxHashed reads stale: an unknown hash,
// never a guess.
const (
	MaxDigestsBytes = 16 << 20
	maxDigestLine   = 3*maxFieldBytes + 2
	maxHashed       = 512
	maxSourceBytes  = 8 << 20
)

// digests is a digests file read whole: each path's sha256, and each
// package's paths in path order.
type digests struct {
	files map[string]string
	byPkg map[string][]string
	// sum is the file's sha256, which the walk file vouches for.
	sum string
}

// readDigests is the digests file at p, refused whole over MaxDigestsBytes,
// when it is no regular file, or wherever parseDigests refuses it.
func readDigests(p string) (*digests, error) {
	f, err := regfile.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxDigestsBytes {
		return nil, fmt.Errorf("platform: the digests are over %d bytes", MaxDigestsBytes)
	}
	h := sha256.New()
	dg, err := parseDigests(io.TeeReader(f, h))
	if err != nil {
		return nil, err
	}
	dg.sum = hex.EncodeToString(h.Sum(nil))
	return dg, nil
}

// parseDigests reads a v0 digests file. It refuses another header, a line
// over maxDigestLine, a torn last line, a line that is not three non-empty
// tab-separated fields, a sha256 that is not 64 lower-case hex digits, and a
// path named twice, so a staleness answer is never read from a file this
// package cannot read whole.
func parseDigests(r io.Reader) (*digests, error) {
	br := bufio.NewReaderSize(r, maxDigestLine+1)
	header, err := readLine(br)
	if err != nil || string(header) != DigestsHeader {
		return nil, errors.New("platform: the digests' header is not " + DigestsHeader)
	}
	out := &digests{files: map[string]string{}, byPkg: map[string][]string{}}
	for n := 2; ; n++ {
		line, err := readLine(br)
		if err == io.EOF {
			for _, ps := range out.byPkg {
				slices.Sort(ps)
			}
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("platform: digests line %d: %w", n, err)
		}
		f := strings.Split(string(line), "\t")
		if len(f) != 3 || f[0] == "" || f[1] == "" || !isSHA256(f[2]) {
			return nil, fmt.Errorf("platform: digests line %d is not package, path and sha256", n)
		}
		if _, dup := out.files[f[1]]; dup {
			return nil, fmt.Errorf("platform: digests line %d names %s again", n, f[1])
		}
		out.files[f[1]] = f[2]
		out.byPkg[f[0]] = append(out.byPkg[f[0]], f[1])
	}
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// fresh reports whether the sync's digests dg vouch for package pkg now:
// every file they list for it reads as the sync read it, and every .go file
// of it the session wrote is among them (S3). A _test.go file the session
// wrote is passed over: no sync reads one, so no fact rests on it. No
// digests, an unreadable file, or a file past the maxHashed'th hashed by this
// Read (hashed counts them) is not fresh: an unknown hash, never a guess.
func fresh(root string, dg *digests, pkg string, written []string, hashed *int) bool {
	if dg == nil {
		return false
	}
	for _, p := range written {
		rel, ok := relUnder(root, p)
		if !ok {
			return false
		}
		if _, listed := dg.files[rel]; !listed && !strings.HasSuffix(rel, "_test.go") {
			return false
		}
	}
	for _, rel := range dg.byPkg[pkg] {
		if *hashed == maxHashed {
			return false
		}
		*hashed++
		if got, err := hashFile(filepath.Join(root, filepath.FromSlash(rel))); err != nil || got != dg.files[rel] {
			return false
		}
	}
	return true
}

// hashFile is the hex sha256 of the regular file at p, refused over
// maxSourceBytes; a named pipe or a device is refused at once (regfile).
func hashFile(p string) (string, error) {
	f, err := regfile.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxSourceBytes+1))
	if err != nil {
		return "", err
	}
	if n > maxSourceBytes {
		return "", fmt.Errorf("platform: %s is over %d bytes", p, maxSourceBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
