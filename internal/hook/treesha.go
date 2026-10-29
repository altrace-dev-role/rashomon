package hook

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// MaxHashBytes is the largest file a named write's post or the Stop hashes
// (card P5A, D140; B48: one hash of at most 1 MiB per named write). A larger
// one has no sha, and a tree that holds it is unknown or moved.
const MaxHashBytes = 1 << 20

// fileSHA256 is the sha256 of the file at path, 64 lowercase hex. It is nil,
// with no error, for a file that is gone, not a regular file or over
// MaxHashBytes: content no run is bound to. An error is a file that could not
// be read, whose content is not known. The mode is read before the open, as
// spec.OSFS does, so a FIFO planted at the path never blocks the hook, and
// the read stops one byte past the cap. hs is the state reused across the
// files of a tree; nil makes one once the file is open.
func fileSHA256(path string, hs *hasher) (*string, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxHashBytes {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if hs == nil {
		hs = newHasher()
	}
	hs.h.Reset()
	n, err := io.CopyBuffer(hs.h, io.LimitReader(f, MaxHashBytes+1), hs.buf)
	if err != nil {
		return nil, err
	}
	if n > MaxHashBytes {
		return nil, nil
	}
	sum := hex.EncodeToString(hs.h.Sum(hs.sum[:0]))
	return &sum, nil
}

// hasher is what fileSHA256 reuses across the files of one tree: the copy
// buffer, one sha256 (Reset per file) and the sum's scratch, so a file costs
// its stat, open, read and hex, and no hash of its own (BenchmarkTreeDigest).
type hasher struct {
	buf []byte
	h   hash.Hash
	sum []byte
}

func newHasher() *hasher {
	return &hasher{make([]byte, 32<<10), sha256.New(), make([]byte, 0, sha256.Size)}
}

// writtenSHA is the sha256 of the file a named write's tool input names, as
// the call left it (store.ExecutionV5.ContentSHA256): a relative path joins
// the payload's cwd, and with no cwd there is no file to read. nil for a call
// that names no path and wherever fileSHA256 gives none.
func writtenSHA(pl *PostPayload) *string {
	path := shape.DeclaredPath(pl.ToolName, pl.ToolInput)
	if path == nil {
		return nil
	}
	file := *path
	if !filepath.IsAbs(file) {
		if pl.CWD == "" {
			return nil
		}
		file = filepath.Join(pl.CWD, file)
	}
	sum, _ := fileSHA256(file, nil)
	return sum
}

// TreeShas is the claim point's tree (verify.Point.Tree): each target's
// content now, by its path, nil for a file that is gone, not regular or over
// MaxHashBytes. A file that could not be read has no entry, so a run's tree
// that holds it reads unknown. nil for nil targets, a session over
// verify.MaxTreeFiles. The Stop's one I/O for the binding (D140): at most
// MaxTreeFiles files of at most MaxHashBytes. The hasher is made only when
// there is a file to read, so a session with no hashed write pays nothing
// for it at each recap turn (BenchmarkRecap/10).
func TreeShas(targets []verify.TreeTarget) map[string]*string {
	if targets == nil {
		return nil
	}
	tree := make(map[string]*string, len(targets))
	if len(targets) == 0 {
		return tree
	}
	hs := newHasher()
	for _, t := range targets {
		if sum, err := fileSHA256(t.File, hs); err == nil {
			tree[t.Path] = sum
		}
	}
	return tree
}
