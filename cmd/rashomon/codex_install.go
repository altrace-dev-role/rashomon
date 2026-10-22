package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// harnessArgs is noArgs for a command that also takes --harness codex: that
// one argument picks codexCmd, no argument picks cmd, and anything else is
// refused before either runs.
func harnessArgs(args []string, stdout io.Writer, cmd, codexCmd func(io.Writer) error) func() error {
	return func() error {
		if len(args) > 0 && args[0] == hook.HarnessFlag {
			if len(args) < 2 {
				return errors.New("--harness needs a value")
			}
			if err := knownHarness(args[1]); err != nil {
				return err
			}
			return noArgs(args[2:], stdout, codexCmd)()
		}
		return noArgs(args, stdout, cmd)()
	}
}

// knownHarness refuses a --harness value a person typed that this build does
// not install for. A hook entry point stands down on one silently; a command a
// person typed says what it knows.
func knownHarness(v string) error {
	if hook.Harness(v) != hook.Codex {
		return fmt.Errorf("unknown harness %q; this build installs for Claude Code (no --harness) and codex", v)
	}
	return nil
}

// codexHome returns Codex's directory, or an error naming it when it is not
// there. watch refuses rather than creating it: a directory nobody's Codex
// reads would hold entries that never run, on a machine that may not have
// Codex at all.
func codexHome() (string, error) {
	dir, err := settings.CodexHome()
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("no Codex directory at %s; install and run Codex first, or set CODEX_HOME to the directory your Codex uses", dir)
	}
	return dir, nil
}

// codexTrustLines are what watch --harness codex says after installing, and
// they are said every time: whether the user has trusted the entries is not
// something this program can read, so it never assumes they have.
var codexTrustLines = []string{
	"rashomon: Codex runs none of these entries until they are trusted: start Codex, run /hooks, " +
		"and trust the rashomon entries for " + strings.Join(install.CodexEvents, ", ") + ". Until then nothing is recorded.",
	"rashomon: Codex runs read coverage: unverified (hook_entry_unresolved), trusted or not -- " +
		"rashomon cannot read whether Codex trusts an entry.",
	"rashomon: not checked: whether hooks = false under [features] in Codex's config.toml or requirements.toml " +
		"turns hooks off; with it set, nothing is recorded.",
}

func cmdWatchCodex(stdout io.Writer) error {
	if _, err := codexHome(); err != nil {
		return err
	}
	path, err := settings.CodexHooksPath()
	if err != nil {
		return err
	}
	exe, err := install.Executable()
	if err != nil {
		return err
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	spec := install.Spec{Executable: exe, InstallID: st.InstallID()}

	var (
		foreign []string
		had     bool
	)
	done, err := editSettingsFile(path, false, func(doc *settings.Document) (bool, error) {
		had = false
		for _, event := range install.CodexEvents {
			present, err := install.CodexPresent(doc, spec.InstallID, event)
			if err != nil {
				return false, err
			}
			had = had || present
		}
		applied, err := install.ApplyCodex(doc, spec)
		if err != nil {
			return false, err
		}
		foreign, err = install.CodexForeignOwners(doc, spec.InstallID)
		return applied, err
	})
	if err != nil {
		return err
	}
	changed := done.changed
	// Recorded so that detach can put the machine back as it was: without it,
	// detach has removed our entries from a file nobody else wrote, and
	// leaves "{}" where there was no file.
	if done.created {
		if err := store.MarkCodexHooksCreated(st.Root(), path); err != nil {
			return err
		}
	}
	if changed {
		fmt.Fprintf(stdout, "rashomon: watching Codex (install %s) via %s\n", st.InstallID(), path)
	} else {
		fmt.Fprintf(stdout, "rashomon: already watching Codex (install %s) via %s\n", st.InstallID(), path)
	}
	// An entry rewritten in place -- the binary moved -- has a new hash, and
	// Codex skips it until it is trusted again, however long ago the old one
	// was.
	if changed && had {
		fmt.Fprintln(stdout, "rashomon: entries already in the file were rewritten, and Codex treats a rewritten entry as new")
	}
	if len(foreign) > 0 {
		fmt.Fprintf(stdout, "rashomon: entries from other installs are present (%s); they will fire in this environment and record nothing\n",
			strings.Join(foreign, ", "))
	}
	for _, line := range codexTrustLines {
		fmt.Fprintln(stdout, line)
	}
	fmt.Fprintln(stdout, "rashomon: undo with this line -- it needs no store, and any rashomon binary will do:")
	fmt.Fprintf(stdout, "rashomon detach --harness codex --install %s\n", st.InstallID())
	return nil
}

// detachCodex is cmdDetach for Codex's hooks.json. It never creates the file
// or its directory: on a machine without them there is nothing to remove, and
// editSettings writes only when something was. It deletes the file when watch
// created it and our entries were all it held, so the machine is back to
// having none (measured in phase 4: detach left "{}" where there was no file).
func detachCodex(installID string, all, force bool, stdout io.Writer) error {
	remove := func(doc *settings.Document) (int, []install.Modified, error) {
		return install.RemoveCodex(doc, install.Spec{InstallID: installID}, force)
	}
	who := "install " + installID
	if all {
		remove = func(doc *settings.Document) (int, []install.Modified, error) {
			return install.RemoveCodexIf(doc, func(string) bool { return true }, force)
		}
		who = "every install"
	}

	path, err := settings.CodexHooksPath()
	if err != nil {
		return err
	}
	// The file goes only if watch recorded creating it, here, at this path,
	// and our entries were all it held. The marker is read without opening
	// the store, so a detach on a machine whose store is gone creates none,
	// and keeps the file.
	root, rootErr := store.DefaultRoot()
	created := rootErr == nil && store.CodexHooksCreated(root, path)
	removed := 0
	var left []install.Modified
	done, err := editSettingsFile(path, created, func(doc *settings.Document) (bool, error) {
		n, l, err := remove(doc)
		removed, left = n, l
		return n > 0, err
	})
	if err != nil {
		return err
	}
	changed := done.changed
	if done.removed {
		if err := store.ClearCodexHooksCreated(root); err != nil {
			return err
		}
	}
	for _, m := range left {
		fmt.Fprintf(stdout, "rashomon: removed the Codex %s entry you had edited (%s), because --force was given\n", m.Event, m.Detail)
	}
	if !changed {
		fmt.Fprintf(stdout, "rashomon: nothing to detach from Codex (%s) in %s\n", who, path)
		return nil
	}
	fmt.Fprintf(stdout, "rashomon: detached from Codex (%s, %d entries) from %s\n", who, removed, path)
	if done.removed {
		fmt.Fprintf(stdout, "rashomon: removed %s: watch created it, and nothing else was in it\n", path)
	}
	return nil
}

// statusCodex reports our entries in Codex's hooks.json, or nothing at all on
// a machine with no Codex directory: a Claude Code-only user's status reads as
// it did before Codex support. hooks.json lives in that directory, so no
// rashomon Codex entry can be present without it.
//
// The file is read, never the trust: present means watch's entry is in the
// file, not that Codex runs it. The last two lines say so, every time.
func statusCodex(stdout io.Writer, installID string) error {
	dir, err := settings.CodexHome()
	if err != nil {
		return err
	}
	path, err := settings.CodexHooksPath()
	if err != nil {
		return err
	}
	fi, statErr := os.Stat(dir)
	exists := statErr == nil && fi.IsDir()
	if !exists {
		return nil
	}
	doc, readErr := settings.Load(path)

	fmt.Fprintf(stdout, "codex: %s\n", path)
	label := "other installs"
	if installID == "" {
		label = "install ids present"
	}
	if readErr != nil {
		for _, event := range install.CodexEvents {
			fmt.Fprintf(stdout, "  %s: %s\n", event, statusUnreadable)
		}
		fmt.Fprintf(stdout, "  %s: %s\n", label, statusUnreadable)
	} else {
		if installID == "" {
			fmt.Fprintln(stdout, "  no store here, so no install id is ours and no entry can be called ours")
		}
		for _, event := range install.CodexEvents {
			fmt.Fprintf(stdout, "  %s: %s\n", event, codexEntryState(doc, installID, event))
		}
		switch others, err := install.CodexForeignOwners(doc, installID); {
		case err != nil:
			fmt.Fprintf(stdout, "  %s: %s\n", label, statusUnreadable)
		case len(others) == 0:
			fmt.Fprintf(stdout, "  %s: none\n", label)
		default:
			fmt.Fprintf(stdout, "  %s: %s\n", label, strings.Join(others, ", "))
		}
	}
	fmt.Fprintln(stdout, "  trust: not readable by rashomon; Codex skips an entry until it is trusted in /hooks, "+
		"and Codex runs read coverage: unverified (hook_entry_unresolved)")
	fmt.Fprintln(stdout, "  hooks = false in config.toml or requirements.toml: not checked")
	return nil
}

// codexEntryState is entryState for Codex's file.
func codexEntryState(doc *settings.Document, installID, event string) string {
	if installID == "" {
		return statusUnknown
	}
	switch present, err := install.CodexPresent(doc, installID, event); {
	case err != nil:
		return statusUnreadable
	case present:
		return "present"
	default:
		return "absent"
	}
}
