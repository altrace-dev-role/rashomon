package spend

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Discovery: which transcripts under a Claude Code config directory Read is
// given. Moved out of scan.go, unchanged, to keep that file under the
// 500-line law; reading a file's lines is scan.go's, finding the files is
// this one's.

// Found is what Discover found: the transcripts to read, and a count of what
// it passed over, so "nothing in the window" and "nothing at all" are two
// different answers.
type Found struct {
	Files []TranscriptFile
	// Stale counts transcripts not read because they were last written
	// before the window. Without it, a machine whose every transcript is old
	// printed "no Claude Code transcripts were found" -- false: they were
	// found, and none was written in the window.
	Stale int
	// UnreadableDirs counts folders under projects/ that could not be listed
	// (a permission, a symlink that loops or dangles). One such folder used
	// to abort the whole command; it is now passed over, counted and said,
	// because the transcripts it holds are spend this reader did not count.
	UnreadableDirs int
}

// TranscriptFile is one transcript on disk.
type TranscriptFile struct {
	Path     string
	Subagent bool
	// Session is the session id the file's location implies: the file's base
	// name for a main transcript, the <session> directory for a subagent's.
	// A line's own sessionId wins over it; this is the fallback for a line
	// that carries none.
	Session string
	// Main is the main transcript this file's conversation is keyed by: the
	// file itself for a main transcript, and projects/<project>/<session>.jsonl
	// for a subagent's -- whether or not that file exists or was read. It is
	// what the silent-failure line calls a transcript when it says which
	// were recorded (Join).
	Main string
}

// Discover lists the transcripts under a Claude Code configuration
// directory: projects/*/*.jsonl, and every agent-*.jsonl at any depth under
// projects/*/<session>/subagents/.
//
// At any depth, for the reason report's subagentTranscripts gives: a
// workflow's subagents write <session>/subagents/workflows/<run>/agent-*.jsonl,
// and a one-level glob misses them. modifiedSince skips a file whose
// modification time is older than the window: every line of a file was
// written at or before its mtime, so such a file cannot hold a response inside
// the window. The zero time reads everything.
//
// A missing projects directory is not an error: it is a machine where Claude
// Code has recorded nothing, and the answer to "what did it spend" is then an
// honest nothing. An unreadable projects directory is, since nothing at all
// could be counted. Below it, a folder that cannot be listed is counted in
// UnreadableDirs and passed over, and a folder reached through a symlink is
// read like any other (entryKind): a DirEntry reports a symlink as neither a
// file nor a directory, and a projects folder linked in from elsewhere was
// skipped without a word -- its spend an unflagged $0.
//
// Both counts are of resolved paths, as the files are (dedupeSpellings): a
// project folder linked in beside its target lists every old transcript and
// unreadable folder under two spellings, and counting each spelling said one
// old transcript was two.
func Discover(configDir string, modifiedSince time.Time) (*Found, error) {
	found := &Found{}
	root := filepath.Join(configDir, "projects")
	projects, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return found, nil
	}
	if err != nil {
		return nil, err
	}
	var out []TranscriptFile
	stale, unreadable := map[string]bool{}, map[string]bool{}
	fresh := func(p string) bool {
		if modifiedSince.IsZero() {
			return true
		}
		info, err := statFile(p)
		if err != nil {
			// A file that vanished since the folder was listed holds
			// nothing. One that cannot be stat'ed (a folder that can be
			// listed but not searched) is not known to be old, so it is
			// kept: Read then counts it as unreadable. Returning false here
			// dropped its transcripts with no count and no note.
			return !errors.Is(err, fs.ErrNotExist)
		}
		if info.ModTime().Before(modifiedSince) {
			stale[resolved(p)] = true
			return false
		}
		return true
	}
	for _, p := range projects {
		dir := filepath.Join(root, p.Name())
		kind := entryKind(dir, p)
		if kind == kindUnreadable {
			unreadable[resolved(dir)] = true
		}
		if kind != kindDir {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			unreadable[resolved(dir)] = true
			continue
		}
		for _, e := range entries {
			name := e.Name()
			path := filepath.Join(dir, name)
			switch kind := entryKind(path, e); {
			case kind == kindFile && strings.HasSuffix(name, ".jsonl"):
				if fresh(path) {
					out = append(out, TranscriptFile{Path: path, Session: strings.TrimSuffix(name, ".jsonl"), Main: path})
				}
			case kind == kindUnreadable:
				unreadable[resolved(path)] = true
			case kind == kindDir:
				subs, bad := subagentFiles(filepath.Join(path, "subagents"))
				for _, b := range bad {
					unreadable[resolved(b)] = true
				}
				for _, s := range subs {
					if fresh(s) {
						out = append(out, TranscriptFile{Path: s, Subagent: true, Session: name,
							Main: filepath.Join(dir, name+".jsonl")})
					}
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	found.Files = dedupeSpellings(out)
	found.Stale, found.UnreadableDirs = len(stale), len(unreadable)
	return found, nil
}

// resolved is a path with its symlinks resolved. A path that does not
// resolve -- a dangling or looping link, or an entry under a folder that
// cannot be searched, exactly the unreadable kinds -- is its resolved parent
// joined with its own name: keyed as written, one such entry reached under
// two spellings was counted twice.
func resolved(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	dir := filepath.Dir(p)
	if dir == p || dir == "." {
		return p
	}
	return filepath.Join(resolved(dir), filepath.Base(p))
}

// dedupeSpellings keeps the first of the files that resolve to one file.
//
// One transcript can be listed under two spellings: after a repo moves, a
// common way to keep its history is to symlink the new project folder to the
// old one beside it, and both folders are listed. Read twice, it was two
// transcripts, one of them never named by a record, so a recorded
// conversation read partly recorded and its whole cost not covered. files is
// sorted, so the spelling kept does not depend on listing order. A path that
// does not resolve is kept, keyed by its resolved parent plus its name: Read
// counts it if it cannot be read.
func dedupeSpellings(files []TranscriptFile) []TranscriptFile {
	seen := map[string]bool{}
	out := files[:0]
	for _, f := range files {
		real := resolved(f.Path)
		if seen[real] {
			continue
		}
		seen[real] = true
		out = append(out, f)
	}
	return out
}

// statFile is os.Stat, as a variable so a test can fail it the way a folder
// that can be listed but not searched fails it -- which root, as tests often
// run, never sees.
var statFile = os.Stat

// Kinds of directory entry, symlinks followed.
const (
	kindOther = iota
	kindFile
	kindDir
	kindUnreadable
)

// entryKind is what a directory entry is once a symlink is followed: a
// regular file, a directory, something else, or -- a symlink whose target
// cannot be reached (it loops, dangles or is not permitted) --
// unreadable. A dangling link is unreadable too: it names something this
// reader was meant to find and could not, which is not the same as nothing.
func entryKind(path string, e fs.DirEntry) int {
	t := e.Type()
	if t&fs.ModeSymlink != 0 {
		info, err := os.Stat(path)
		if err != nil {
			return kindUnreadable
		}
		t = info.Mode().Type()
	}
	switch {
	case t.IsDir():
		return kindDir
	case t.IsRegular():
		return kindFile
	}
	return kindOther
}

// subagentFiles lists every agent-*.jsonl under dir, at any depth, and the
// folders under it that could not be read. A missing directory is the common
// case, a session with no subagents. A folder that cannot be read is skipped
// and listed, so the rest of the walk -- and of spend -- goes on.
func subagentFiles(dir string) (out, unreadable []string) {
	// The walk function returns only nil or SkipDir, so WalkDir has no error to
	// return: every folder it could not read is in unreadable instead.
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir
			}
			unreadable = append(unreadable, p)
			if d != nil && !d.IsDir() {
				return nil
			}
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		// Match fails only on a malformed pattern, and this one is a constant.
		if ok, _ := filepath.Match("agent-*.jsonl", d.Name()); ok {
			out = append(out, p)
		}
		return nil
	})
	return out, unreadable
}
