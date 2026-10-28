package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Transcript copies (card 4.1, PRD PF-RC14): a byte copy of a session's
// transcript, taken at SessionEnd, kept under DirBodies, read by CLI commands
// only. Off by default and on per project by the FileBodies switch, which
// caps the copies' total. That cap is theirs alone: Evict's
// (RASHOMON_STORE_CAP_BYTES) measures runs/ and this one measures bodies/,
// so neither moves the other.
const (
	// DirBodies holds a directory per project: <project>/<session>.jsonl.
	DirBodies = "bodies"
	// FileBodies is the switch: the project keys that keep copies, and the cap.
	FileBodies = "bodies.json"
	// BodiesTempPrefix names a copy in flight, renamed over its session's file
	// when complete, or left by a process that died first.
	BodiesTempPrefix = ".body.tmp-"
	// DefaultBodiesMaxBytes is the cap of a switch that names none (PRD).
	DefaultBodiesMaxBytes int64 = 2 << 30
	// maxBodiesSettingBytes bounds the switch's read: a list of keys.
	maxBodiesSettingBytes = 1 << 20
)

// ErrBodyOverCap is CopyBody's refusal of a transcript larger than the whole
// cap: no eviction could make room, so none is done.
var ErrBodyOverCap = errors.New("bodies: the transcript is larger than the cap")

// BodiesSetting is the switch's content.
type BodiesSetting struct {
	MaxBytes int64    `json:"max_bytes"`
	Projects []string `json:"projects"`
}

// ReadBodies reads root's switch; ok is false, with no error, when there is
// none, which is copies off.
func ReadBodies(root string) (set BodiesSetting, ok bool, err error) {
	f, err := os.Open(filepath.Join(root, FileBodies))
	if errors.Is(err, fs.ErrNotExist) {
		return BodiesSetting{}, false, nil
	}
	if err != nil {
		return BodiesSetting{}, false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBodiesSettingBytes+1))
	if err == nil && len(b) > maxBodiesSettingBytes {
		err = fmt.Errorf("%s is over %d bytes", FileBodies, maxBodiesSettingBytes)
	}
	if err == nil {
		err = json.Unmarshal(b, &set)
	}
	if err != nil {
		return BodiesSetting{}, false, fmt.Errorf("%s: %w", FileBodies, err)
	}
	if set.MaxBytes <= 0 {
		set.MaxBytes = DefaultBodiesMaxBytes
	}
	return set, true, nil
}

// WriteBodies replaces root's switch with set.
func WriteBodies(root string, set BodiesSetting) error {
	b, err := json.Marshal(set)
	if err != nil {
		return err
	}
	return ReplaceFile(filepath.Join(root, FileBodies), append(b, '\n'))
}

// CopyBody copies transcriptPath to DirBodies/projectDir/<session>.jsonl when
// the switch lists projectKey, first evicting the oldest copies of every
// project so the total stays within the cap; the copy's mtime is now, the
// order eviction reads. Off, or a project not listed, is (false, nil) before
// the transcript is looked at. The caller names projectDir, one plain path
// element: it comes from internal/baseline, which imports this package.
func (s *Store) CopyBody(sessionID, transcriptPath, projectKey, projectDir string, now time.Time) (bool, error) {
	set, on, err := ReadBodies(s.root)
	if err != nil || !on || !slices.Contains(set.Projects, projectKey) {
		return false, err
	}
	if !isPlainSegment(projectDir) {
		return false, fmt.Errorf("bodies: project directory %q is not one path element", projectDir)
	}
	info, err := os.Stat(transcriptPath)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("bodies: %s is not a regular file", transcriptPath)
	}
	if info.Size() > set.MaxBytes {
		return false, ErrBodyOverCap
	}
	dir := filepath.Join(s.root, DirBodies, projectDir)
	dst := filepath.Join(dir, s.DirName(sessionID)+".jsonl")
	if err := s.evictBodies(set.MaxBytes-info.Size(), dst); err != nil {
		return false, err
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return false, err
	}
	if err := copyBody(transcriptPath, info.Size(), dir, dst); err != nil {
		return false, err
	}
	return true, os.Chtimes(dst, now, now)
}

// copyBody writes size bytes of src to a temporary file in dir, 0600 and
// synced, and renames it over dst. A transcript still growing copies as it
// stood at the size CopyBody checked against the cap.
func copyBody(src string, size int64, dir, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, BodiesTempPrefix+"*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // a no-op after the rename; cleanup before it
	err = tmp.Chmod(fileMode)
	if err == nil {
		_, err = io.Copy(tmp, io.LimitReader(in, size))
	}
	if err == nil {
		err = syncFull(tmp) // through syncAs, as ReplaceFile's is, so REC-34's recorder sees it
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// evictBodies removes copies, oldest mtime first and by path among equals,
// until the rest total at most budget bytes. except, the file about to be
// replaced, is neither counted nor removed, and nor is anything not named
// *.jsonl: a temporary file is a copy in flight, or one a crash left, which
// is purge's to remove, and any other file is the user's, as purge's
// treeFile has it.
func (s *Store) evictBodies(budget int64, except string) error {
	type body struct {
		path string
		size int64
		mod  time.Time
	}
	var all []body
	var total int64
	projects, err := os.ReadDir(filepath.Join(s.root, DirBodies))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(s.root, DirBodies, p.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			info, err := e.Info()
			path := filepath.Join(dir, e.Name())
			if err != nil || !info.Mode().IsRegular() || path == except || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			all = append(all, body{path, info.Size(), info.ModTime()})
			total += info.Size()
		}
	}
	slices.SortFunc(all, func(a, b body) int {
		if c := a.mod.Compare(b.mod); c != 0 {
			return c
		}
		return strings.Compare(a.path, b.path)
	})
	for _, b := range all {
		if total <= budget {
			break
		}
		if err := os.Remove(b.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		total -= b.size
	}
	return nil
}

// ForgetBodies removes projectDir's copies, and the directory when nothing
// else is in it, and says how many copies went.
func (s *Store) ForgetBodies(projectDir string) (int, error) {
	if !isPlainSegment(projectDir) {
		return 0, fmt.Errorf("bodies: project directory %q is not one path element", projectDir)
	}
	dir := filepath.Join(s.root, DirBodies, projectDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		name := e.Name()
		temp := strings.HasPrefix(name, BodiesTempPrefix)
		if !e.Type().IsRegular() || !(temp || strings.HasSuffix(name, ".jsonl")) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return removed, err
		}
		if !temp {
			removed++
		}
	}
	_ = os.Remove(dir) // left when it holds anything not a copy
	return removed, nil
}
