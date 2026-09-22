package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// fileRecordingPaused is the marker under the store root that Pause writes
// and Resume removes. Its content is the Unix millisecond instant recording
// was paused, so that Resume can compute the window it closes without
// consulting anything else.
//
// It lives under the store root rather than as an environment variable or a
// settings edit, and the reasoning is Part 2's whole point rather than an
// implementation detail:
//
//   - An environment variable is per-process and invisible once the process
//     that set it exits, so a session that ran under one leaves no evidence of
//     why it stopped recording. internal/fault/fault.go:5-8 states the sibling
//     principle -- an attacker who could set an env var and make the recorder
//     abandon a run silently would be attacking the coverage guarantee this
//     program exists to provide. A `/config` plugin option is no better a
//     store of record for the same reason, and settings.json's own `env`
//     block is written by the same audited agent this threat model already
//     assumes can run arbitrary commands, so it inherits the objection rather
//     than answering it; either may still exist as an ENTRY POINT that calls
//     `rashomon pause`, but the file stays the state.
//   - A settings edit cannot reach a hook entry a PLUGIN owns -- the manifest
//     is one artefact shipped to every user and carries no per-machine id --
//     so it could not pause a plugin-only install at all.
//
// A file under the store root reaches every origin a hook can be installed
// under, plugin included, because every one of them resolves this same store
// before it does anything else.
const fileRecordingPaused = "pause"

// PausedSessionID is the session id the pause and resume commands' own gap
// records carry. Pausing and resuming are machine-level decisions with no
// Claude Code session behind them -- nobody's payload names a session id at
// the moment a human runs `rashomon pause` -- so their evidence needs a home
// of its own in the report rather than borrowing a real session id and
// misattributing a machine-wide decision to it.
const PausedSessionID = "recording-paused"

// Paused reports whether root's store is paused, and since when.
//
// It stats and reads one small file directly, never through Open: Open MINTS
// an install identity and a per-install key on first use, and a paused
// machine that has never recorded must not acquire one merely by being asked
// whether it is paused. A root that does not exist yet is not paused -- there
// is nothing under it to say so, and that is a real "no", not an error.
func Paused(root string) (paused bool, since time.Time, err error) {
	b, err := os.ReadFile(filepath.Join(root, fileRecordingPaused))
	if errors.Is(err, fs.ErrNotExist) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	ms, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if perr != nil {
		// A marker that exists but does not parse is still a marker: recording
		// stays paused, and the start instant is unknown rather than guessed.
		return true, time.Time{}, nil
	}
	return true, time.UnixMilli(ms), nil
}

// Pause engages root's paused state, creating root if this machine has never
// recorded before -- the marker has to live somewhere, and "under the store
// root" is that somewhere even before anything else is. It does NOT mint an
// install identity or a key: those come only from Open, and pausing a machine
// that has never recorded must not be the thing that makes it look like it
// has.
//
// It is idempotent. alreadyPaused reports whether this call found the marker
// already there, and since is the instant pausing first took effect either
// way -- calling pause twice must not forget when the window actually began,
// or resume's own gap record would understate it.
func Pause(root string, now time.Time) (since time.Time, alreadyPaused bool, err error) {
	already, existingSince, err := Paused(root)
	if err != nil {
		return time.Time{}, false, err
	}
	if already {
		return existingSince, true, nil
	}
	if err := os.MkdirAll(root, dirMode); err != nil {
		return time.Time{}, false, err
	}
	// MkdirAll honours the umask and does nothing to a directory that already
	// exists, so neither call alone guarantees the mode -- the same two-step
	// Open uses on this same root.
	if err := os.Chmod(root, dirMode); err != nil {
		return time.Time{}, false, err
	}
	content := []byte(strconv.FormatInt(now.UnixMilli(), 10) + "\n")
	if err := writeSmall(filepath.Join(root, fileRecordingPaused), content); err != nil {
		return time.Time{}, false, err
	}
	return now, false, nil
}

// Resume disengages root's paused state and reports the window it closes: the
// instant pause first took effect, and now. wasPaused is false, with a zero
// since, when the machine was not paused -- resume is idempotent in the same
// sense detach is, and calling it on a machine that is not paused is a
// satisfied request, not an error.
func Resume(root string, now time.Time) (wasPaused bool, since time.Time, err error) {
	paused, existingSince, err := Paused(root)
	if err != nil {
		return false, time.Time{}, err
	}
	if !paused {
		return false, time.Time{}, nil
	}
	if err := os.Remove(filepath.Join(root, fileRecordingPaused)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, time.Time{}, err
	}
	if existingSince.IsZero() {
		// The marker existed but its content did not parse; the true start is
		// unrecoverable, and now is the least wrong answer -- it understates
		// the paused window rather than overstating one that may not have
		// been this long.
		existingSince = now
	}
	return true, existingSince, nil
}
