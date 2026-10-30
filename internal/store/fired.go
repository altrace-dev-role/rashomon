package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// FileFiredPrefix begins each hook event's marker in a run directory,
// fired-<Event>: created the first time that event's entry runs for the
// session, holding that time in Unix milliseconds, and never written again.
// status counts the sessions each event fired in from these (the consent
// screen's promise), so an entry that stopped firing shows as a count of 0.
const FileFiredPrefix = "fired-"

// firedEvents are the events a marker may name: install.Events(), in its
// order. The store cannot import install; a test pins the two lists equal.
var firedEvents = [...]string{
	"PreToolUse", "PostToolUse", "PostToolUseFailure", "SessionStart",
	"SessionEnd", "Stop", "StopFailure", "UserPromptSubmit",
}

// FiredEvents returns the events a marker may name, in install order, as a
// slice of its own.
func FiredEvents() []string { return slices.Clone(firedEvents[:]) }

// ErrFiredEvent is MarkFired's refusal of a name that is not one of
// FiredEvents: the name becomes part of a file name, so nothing else is let
// through.
var ErrFiredEvent = errors.New("store: not a hook event a fired marker may name")

// IsFiredFile reports whether name is one of the markers' file names.
func IsFiredFile(name string) bool {
	event, ok := strings.CutPrefix(name, FileFiredPrefix)
	return ok && slices.Contains(firedEvents[:], event)
}

// MarkFired records that event's entry ran for the session. Every entry calls
// it, so the marker already there costs one stat and nothing else; the first
// fire creates it exclusively, and of two entries racing to create it the
// later one finds it made and leaves the earlier time standing. The marker is
// not synced: a crash that loses it loses one session's count, which the
// next fire of that event in the session writes again.
func (s *Store) MarkFired(sessionID, event string, now time.Time) error {
	if !slices.Contains(firedEvents[:], event) {
		return ErrFiredEvent
	}
	if err := s.opened(); err != nil {
		return err
	}
	dir := s.RunDir(sessionID)
	marker := filepath.Join(dir, FileFiredPrefix+event)
	if _, err := os.Lstat(marker); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(strconv.FormatInt(now.UnixMilli(), 10) + "\n")
	return errors.Join(err, f.Close())
}

// RunActivity is what status reads of one run directory.
type RunActivity struct {
	// Calls is whether the run's records file exists and holds anything.
	Calls bool
	// LastAt is the records file's modification time, zero with none.
	LastAt time.Time
	// Fired is the events whose marker the run holds, in FiredEvents order.
	Fired []string
}

// Activity reads the run directory name, an entry of Runs: one stat of the
// records file and one listing of the directory, whatever the run's size. A
// directory that is not there, never made or evicted since Runs listed it,
// holds nothing.
func (s *Store) Activity(name string) (RunActivity, error) {
	if err := s.opened(); err != nil {
		return RunActivity{}, err
	}
	dir := filepath.Join(s.root, DirRuns, name)
	var a RunActivity
	info, err := os.Stat(filepath.Join(dir, FileRecords))
	if err == nil {
		a.Calls, a.LastAt = info.Size() > 0, info.ModTime()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return RunActivity{}, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return RunActivity{}, err
	}
	var held [len(firedEvents)]bool
	for _, e := range entries {
		if event, ok := strings.CutPrefix(e.Name(), FileFiredPrefix); ok {
			if i := slices.Index(firedEvents[:], event); i >= 0 {
				held[i] = true
			}
		}
	}
	for i, ok := range held {
		if ok {
			a.Fired = append(a.Fired, firedEvents[i])
		}
	}
	return a, nil
}
