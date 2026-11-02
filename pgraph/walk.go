package pgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// WalkDigest is the SHA-256 over every file whose change can change what a
// sync of root reads, found without the go tool: each go.mod and go.sum, and
// each .go file that is not a test, in each directory ScanRoots walks
// (testdata, vendor, and names starting with . or _ are skipped; symbolic
// links to directories are not followed). Each file counts as its slash path
// under root and the SHA-256 of its bytes, in path order. It is a superset of
// what a sync reads (a file a build tag leaves out still counts), so an equal
// digest means nothing a sync could read has changed, and a different one
// may sync to the same facts. A sync takes it before it loads, so an edit
// racing a sync costs one more sync, never a log the next walk calls
// current. O(bytes walked); ctx ends a walk of a tree too large for it.
func WalkDigest(ctx context.Context, root string) (string, error) {
	var files []File
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case name == "go.mod" || name == "go.sum":
		case strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") &&
			!strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_"):
		default:
			return nil
		}
		if !d.Type().IsRegular() {
			// A link or a pipe named like source: its name counts, not
			// its bytes, so opening it can never block the walk.
			files = append(files, File{relSlash(root, p), "not-regular"})
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > MaxSourceFileBytes {
			// Over the cap a sync reads a file to (a generated file a build
			// tag leaves out, say): its size and time stand for its bytes.
			files = append(files, File{relSlash(root, p), fmt.Sprintf("over-cap %d %d", info.Size(), info.ModTime().UnixNano())})
			return nil
		}
		sum, err := hashFile(p)
		if err != nil {
			return err
		}
		files = append(files, File{relSlash(root, p), sum})
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("pgraph: walking the tree: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	h := sha256.New()
	for _, f := range files {
		io.WriteString(h, f.Path+"\x00"+f.SHA256+"\n")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// relSlash is p relative to root, slash-separated; p is under root.
func relSlash(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}
