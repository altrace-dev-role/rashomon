package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The detach self-check's lines (Technical spec 7.2.9). The bug line is the
// spec's, word for word: it is what a user pastes into a report.
const (
	detachRestoredFmt = "rashomon: restored %s byte for byte, as it was before watch\n"
	detachBugLine     = "rashomon: detach could not restore settings.json byte for byte; this is a bug, please report it"
	detachKeptFmt     = "rashomon: removed rashomon's %d entries; every other byte is as you and other tools left it\n"
	detachExpectedFmt = "rashomon: expected the file before watch, sha256 %s; detach left %s\n"
	detachDiffHeader  = "rashomon: what this detach changed in settings.json, every value replaced by " + settings.Elided + ":"
)

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// missingDirs lists, for each of dirs, it and every parent of it that does
// not exist yet, outermost first. It is called before watch creates
// anything, so what it returns is what watch is about to create.
func missingDirs(dirs ...string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			return nil, err
		}
		var chain []string
		for p := abs; ; p = filepath.Dir(p) {
			_, err := os.Lstat(p)
			if err == nil {
				break
			}
			if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			chain = append(chain, p)
			if filepath.Dir(p) == p {
				break
			}
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !seen[chain[i]] {
				seen[chain[i]] = true
				out = append(out, chain[i])
			}
		}
	}
	return out, nil
}

// holdsEntriesOf reports whether doc already carries an entry claimed by
// installID, under any event.
func holdsEntriesOf(doc *settings.Document, installID string) (bool, error) {
	for _, event := range install.Events() {
		entries, err := doc.HookEntries(event)
		if err != nil {
			return false, err
		}
		for _, e := range entries {
			if install.Owner(e, event) == installID {
				return true, nil
			}
		}
	}
	return false, nil
}

// recordCreatedDirs adds to install.json the directories this watch
// created, for purge to remove once they are empty. A directory is added
// once and never dropped by a later watch.
func recordCreatedDirs(root string, created []string) error {
	if len(created) == 0 {
		return nil
	}
	return store.UpdateInstallRecord(root, func(r *store.InstallRecord) {
		for _, d := range created {
			if !contains(r.CreatedDirs, d) {
				r.CreatedDirs = append(r.CreatedDirs, d)
			}
		}
	})
}

// recordWatch writes into install.json the settings file's hashes before
// and after this watch's edit.
//
// The pre-image is the file before the FIRST watch that wrote our entries
// into it. A later watch that refreshes them keeps it only when the file is
// still the one the last watch left; when it has changed since, nobody knows
// what the file held before our entries went in, and the pre-image is
// cleared rather than guessed, so detach takes the edited-since-watch path.
func recordWatch(root string, edit settingsEdit, hadOurs bool) error {
	if !edit.changed {
		return nil
	}
	return store.UpdateInstallRecord(root, func(r *store.InstallRecord) {
		before := sha256Hex(edit.before)
		switch {
		case !hadOurs:
			existed := edit.existed
			r.SettingsExisted = &existed
			r.PreSHA256 = before
		case r.PreSHA256 != "" && r.PostSHA256 == before:
			// A refresh of the file the last watch left: its pre-image stands.
		default:
			r.SettingsExisted = nil
			r.PreSHA256 = ""
		}
		r.PostSHA256 = sha256Hex(edit.after)
	})
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// selfCheckRecord is this store's install record when a detach removes this
// store's own install, the only one whose hashes it holds, and nil
// otherwise: --all, another install's id, or no store to read.
func selfCheckRecord(installID string, all bool) *store.InstallRecord {
	if all {
		return nil
	}
	root, err := store.DefaultRoot()
	if err != nil {
		return nil
	}
	rec, err := store.ReadInstallRecord(root)
	if err != nil || rec.InstallID != installID {
		return nil
	}
	return &rec
}

// unchangedSinceWatch reports whether before is the file watch left, with a
// pre-image recorded to compare a restore against.
func unchangedSinceWatch(rec *store.InstallRecord, before []byte, existed bool) bool {
	return rec.PostSHA256 != "" && rec.PreSHA256 != "" && rec.SettingsExisted != nil &&
		existed && sha256Hex(before) == rec.PostSHA256
}

// restoresAbsent says, for detach's edit, when emptying the file means
// removing it: watch created it, and it is still the file watch left.
func restoresAbsent(rec *store.InstallRecord) func(before []byte) bool {
	if rec == nil {
		return nil
	}
	return func(before []byte) bool {
		return unchangedSinceWatch(rec, before, true) && !*rec.SettingsExisted
	}
}

// selfCheck says what detach did to the settings file (Technical spec
// 7.2.9). Unchanged since watch: the result must be the pre-image, and the
// line says so, or the bug line and a values-free diff follow. Edited since
// watch: the line names how many entries went and that every other byte is
// as it was -- checked, not assumed: the result must be the file as found
// with bytes only taken out, and what was taken out must be our entries and
// nothing else. Either way the result is already written and
// detach exits 0.
func selfCheck(w io.Writer, path string, rec *store.InstallRecord, edit settingsEdit, removed int) {
	if unchangedSinceWatch(rec, edit.before, edit.existed) {
		restored := edit.removed && !*rec.SettingsExisted ||
			!edit.removed && *rec.SettingsExisted && sha256Hex(edit.after) == rec.PreSHA256
		if restored {
			fmt.Fprintf(w, detachRestoredFmt, path)
			return
		}
		fmt.Fprintln(w, detachBugLine)
		got := "no file"
		if !edit.removed {
			got = "sha256 " + sha256Hex(edit.after)
		}
		fmt.Fprintf(w, detachExpectedFmt, rec.PreSHA256, got)
		printDiff(w, edit)
		return
	}
	if !edit.removed && install.OnlyEntriesRemoved(edit.before, edit.after, rec.InstallID) {
		fmt.Fprintf(w, detachKeptFmt, removed)
		return
	}
	fmt.Fprintln(w, detachBugLine)
	printDiff(w, edit)
}

// printDiff is the values-free diff of what this detach did. install.json
// keeps only the pre-image's hash, never its bytes, so the diff is between
// the file as detach found it and as it left it.
func printDiff(w io.Writer, edit settingsEdit) {
	fmt.Fprintln(w, detachDiffHeader)
	for _, line := range settings.ValuesFreeDiff(edit.before, edit.after) {
		fmt.Fprintln(w, line)
	}
}
