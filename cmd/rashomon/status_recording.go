package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// statusRecording says whether hooks will record, which is the one thing pause
// changes and the first thing a user who ran it will ask. "Active" is only a
// useful answer once a store exists: on a fresh machine no hooks are installed
// to record anything. A pause marker that exists and cannot be read is
// reported as RecordingPausedUnreadable, not as unknown: since checkPaused
// treats that marker as a pause, hooks do not record, and status has an
// answer rather than nothing to report. "unknown" is reserved for a
// RecordingState error, which means a fault unrelated to the marker itself.
func statusRecording(stdout io.Writer, root string) {
	state, since, err := RecordingState()
	// An enabled plugin records from its first hook call, which is also what
	// creates the store, so a missing store alone is not "not installed".
	installed := storeExistsAt(root)
	if err == nil && !installed {
		installed, err = pluginEnabled()
	}
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "  recording: %s\n", statusUnknown)
	case state == RecordingActive && !installed:
		fmt.Fprintln(stdout, "  recording: not installed")
	case state == RecordingActive:
		fmt.Fprintf(stdout, "  recording: %s\n", RecordingActive)
	case state == RecordingPausedUnreadable:
		fmt.Fprintf(stdout, "  recording: %s\n", RecordingPausedUnreadable)
	default:
		line := RecordingPaused
		if !since.IsZero() {
			line += " since " + since.UTC().Format(time.RFC3339)
		}
		if state == RecordingPausedNoStore {
			line += " (no store recorded here yet)"
		}
		fmt.Fprintf(stdout, "  recording: %s\n", line)
	}
}

// statusRecordScope says which record scope the hook applies and where that
// came from: the record_scope marker beside consent.json, or the v4 default
// and why. When consent.json itself names another scope, its answer follows,
// so a v4 that withholds paths and command text under a consent that names
// v5 says so rather than reading as a fault. Both are plain reads; neither
// opens a store, and neither writes.
func statusRecordScope(stdout io.Writer, root string) {
	scope, err := store.RecordScopeMarker(root)
	source := "record_scope marker"
	switch {
	case errors.Is(err, fs.ErrNotExist):
		source = "no record_scope marker"
	case errors.Is(err, store.ErrRecordScopeWord):
		source = "record_scope marker holds no scope word"
	case err != nil:
		source = "record_scope marker unreadable"
	}
	if fold, ferr := consent.RecordScope(root); ferr == nil && fold != scope {
		if fold != store.RecordScopeV4 && fold != store.RecordScopeV5 {
			fold = "neither v4 nor v5"
		}
		source += "; consent.json says " + fold
	}
	fmt.Fprintf(stdout, "  record scope: %s (%s)\n", scope, source)
}
