// Package receipt is the commit receipt (5.5, PF-RG1 in v0's form): a git
// trailer that names a checkpoint record by its digest, a git note that holds
// the record, and the two hook bodies that put them on a commit. The record
// is the project's latest checkpoint exactly as the store wrote it, so the
// trailer is the note's digest and the note cannot be swapped without the
// commit message changing.
//
// rashomon runs no git and no child process at all (H-17, TRU-40: os/exec is
// in no package a shipped binary links). `rashomon receipt` prints; the hooks
// the user installs by hand run git; nothing pushes the note but the user.
package receipt

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TrailerKey is the trailer's key in the commit message.
const TrailerKey = "Rashomon-Receipt"

// NotesRef is where the record goes, one note per commit.
const NotesRef = "refs/notes/rashomon"

// Digest is "sha256:" and the lowercase hex of b's SHA-256.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Trailer is the commit message line that names record by its digest.
func Trailer(record []byte) string {
	return TrailerKey + ": " + Digest(record)
}

// written is a run directory, its records file's modification time and size.
type written struct {
	name string
	at   int64
	size int64
}

// Latest is the newest checkpoint record cut for project (baseline.Key of a
// directory, which a checkpoint's project_root holds), by recorded_at, among
// each run's last checkpoint, exactly as the store wrote it. A run's last
// checkpoint is the only one it offers: an earlier one is older, and a
// session that moved to another project since has left this one.
//
// Runs are read newest first by their records file's modification time, and
// the walk stops at a run last written before the best record found: every
// record in that run was recorded no later than its file was written, so it
// holds none newer. A commit reads the runs written since, not the store.
// It is Previous with no skip and no bound.
func Latest(st *store.Store, project string) (record []byte, ok bool, err error) {
	line, _, sc, err := scan(st, Query{Project: project})
	if err != nil || !sc.Found {
		return nil, false, err
	}
	return line, true, nil
}

// byWritten is the store's runs that hold a records file, newest written
// first. A run with no records file holds no checkpoint.
func byWritten(st *store.Store) ([]written, error) {
	names, err := st.Runs()
	if err != nil {
		return nil, err
	}
	runs := make([]written, 0, len(names))
	for _, name := range names {
		info, err := os.Stat(filepath.Join(st.RunDir(name), store.FileRecords))
		if err != nil {
			continue
		}
		runs = append(runs, written{name, info.ModTime().UnixMilli(), info.Size()})
	}
	slices.SortStableFunc(runs, func(a, b written) int { return cmp.Compare(b.at, a.at) })
	return runs, nil
}

// PrepareCommitMsg is the prepare-commit-msg hook body `rashomon receipt on`
// prints. The message gains the trailer for the project's latest checkpoint,
// replacing an older one (an amended commit names the checkpoint it now
// carries); with no checkpoint, no rashomon on the PATH, or any failure, the
// message is left as it is. No commit is ever blocked: every path exits 0.
const PrepareCommitMsg = `#!/bin/sh
# rashomon receipt: name the project's latest checkpoint in the message.
t=$(rashomon receipt trailer) && [ -n "$t" ] || exit 0
git interpret-trailers --in-place --if-exists replace --trailer "$t" "$1" || true
exit 0
`

// PostCommit is the post-commit hook body: the new commit's note is the
// checkpoint record its trailer names, the record's bytes and one newline.
// A commit with no trailer, or whose checkpoint has moved on since, gets no
// note.
const PostCommit = `#!/bin/sh
# rashomon receipt: the new commit's note is the checkpoint its trailer names.
v=$(git log -1 --format='%(trailers:key=Rashomon-Receipt,valueonly)' HEAD) && [ -n "$v" ] || exit 0
n=$(rashomon receipt note "$v") && [ -n "$n" ] || exit 0
printf '%s\n' "$n" | git notes --ref rashomon add -f -F - HEAD || true
exit 0
`
