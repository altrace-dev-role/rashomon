package platform

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Where `pgraph sync <root>` writes root's log and digests, and so where the
// Stop reads them (pgraph/syncroot.go StateDir, LogFile, DigestsFile): in the
// repository's tree, not the store, which records no content, and the log is
// names read from the tree's source. This package writes neither file. The
// names are the boundary, as LogHeader is; the acceptance item that runs
// pgraph sync and then the hook (TestPLW1_*) fails if the two sides part.
const (
	stateDir    = ".rashomon"
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
// package reads stale.
const DigestsHeader = "pgraph-digests v0"

// The bounds of a digests read (INV-10): the file, one line (three fields at
// pgraph's field cap and two tabs), the written files hashed for one Stop,
// and one file hashed, pgraph's MaxSourceFileBytes, past which no sync read
// the file either. A Stop that wrote more Go files than maxHashed reads the
// packages of the rest stale: an unknown hash, never a guess.
const (
	MaxDigestsBytes = 16 << 20
	maxDigestLine   = 3*maxFieldBytes + 2
	maxHashed       = 64
	maxSourceBytes  = 8 << 20
)

// readDigests is the digests file at p as path to sha256, refused whole
// over MaxDigestsBytes or wherever parseDigests refuses it.
func readDigests(p string) (map[string]string, error) {
	f, err := os.Open(p)
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
	return parseDigests(f)
}

// parseDigests reads a v0 digests file. It refuses another header, a line
// over maxDigestLine, a torn last line, a line that is not three non-empty
// tab-separated fields, a sha256 that is not 64 lower-case hex digits, and a
// path named twice, so a staleness answer is never read from a file this
// package cannot read whole.
func parseDigests(r io.Reader) (map[string]string, error) {
	br := bufio.NewReaderSize(r, maxDigestLine+1)
	header, err := readLine(br)
	if err != nil || string(header) != DigestsHeader {
		return nil, errors.New("platform: the digests' header is not " + DigestsHeader)
	}
	out := map[string]string{}
	for n := 2; ; n++ {
		line, err := readLine(br)
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("platform: digests line %d: %w", n, err)
		}
		f := strings.Split(string(line), "\t")
		if len(f) != 3 || f[0] == "" || f[1] == "" || !isSHA256(f[2]) {
			return nil, fmt.Errorf("platform: digests line %d is not package, path and sha256", n)
		}
		if _, dup := out[f[1]]; dup {
			return nil, fmt.Errorf("platform: digests line %d names %s again", n, f[1])
		}
		out[f[1]] = f[2]
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

// staleOf is the packages of files, written paths under root to their
// package, whose facts the sync's digests do not vouch for: a written .go
// file whose content now differs from its recorded sha256, that is not
// recorded (a file the sync never read), or that cannot be read, past the
// maxHashed'th file hashed, or every package where the digests cannot be
// read. A _test.go file makes nothing stale: no sync reads one, so no fact
// rests on it.
func staleOf(root string, files map[string]string) map[string]bool {
	stale := map[string]bool{}
	sums, err := readDigests(digestsPath(root))
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	hashed := 0
	for _, p := range paths {
		pkg := files[p]
		rel, ok := relUnder(root, p)
		if stale[pkg] || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if !ok || err != nil || hashed == maxHashed {
			stale[pkg] = true
			continue
		}
		hashed++
		got, herr := hashFile(filepath.Join(root, filepath.FromSlash(rel)))
		if want, ok := sums[rel]; !ok || herr != nil || got != want {
			stale[pkg] = true
		}
	}
	return stale
}

// hashFile is the hex sha256 of the file at p, refused over maxSourceBytes.
func hashFile(p string) (string, error) {
	f, err := os.Open(p)
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
