package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/purge"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The plan's headers. Every path listed under purgeRemovesHeader is removed
// and nothing else is; purgeIfEmpty marks a directory that goes only once
// nothing is left in it.
const (
	purgeRemovesHeader = "rashomon purge removes:"
	purgeLeavesHeader  = "rashomon purge leaves:"
	purgeIfEmpty       = " (if empty)"
	purgePrompt        = "Remove all of this? [y/N] "
)

// errPurgeDeclined is a purge the user answered no to.
var errPurgeDeclined = errors.New("nothing was removed")

// cmdPurge is `rashomon purge [--yes] [--binary]` (Technical spec 7.2.9):
// refuse a root that is not provably a store, print every path it will
// remove and every path it leaves, ask on the terminal unless --yes, then
// detach, remove the inventory with install.json and install.key last, the
// empty root, and the empty directories watch created, deepest first. It
// exits 1 when a planned path remains.
func cmdPurge(args []string, stdout io.Writer) error {
	yes, withBinary := false, false
	for _, a := range args {
		switch a {
		case "--yes":
			yes = true
		case "--binary":
			withBinary = true
		default:
			return fmt.Errorf("unknown argument %q", a)
		}
	}

	t, err := resolvePurgeTarget()
	if err != nil {
		return err
	}
	plan, err := purge.Build(t.root, t.createdDirs, t.binaryIf(withBinary))
	if err != nil {
		return fmt.Errorf("reading %s: %w; nothing was removed", t.root, err)
	}
	printPurgePlan(stdout, t, plan, withBinary)

	if !yes {
		ok, err := confirmOnTerminal()
		if err != nil {
			return err
		}
		if !ok {
			return errPurgeDeclined
		}
	}

	if err := cmdDetach([]string{"--install", t.rec.InstallID}, stdout); err != nil {
		return fmt.Errorf("detach: %w; nothing was removed", err)
	}
	fault.Inject(fault.PointPurgeAfterDetach)

	out := plan.Execute()
	for _, p := range out.Removed {
		fmt.Fprintf(stdout, "rashomon: removed %s\n", p)
	}
	for _, p := range out.Left {
		fmt.Fprintf(stdout, "rashomon: left %s -- not empty\n", p)
	}
	for _, p := range out.Remaining {
		fmt.Fprintf(stdout, "rashomon: left %s -- could not be removed\n", p)
	}
	if len(out.Remaining) > 0 {
		return fmt.Errorf("purge incomplete: %d planned paths remain (listed above); run `rashomon purge` again once whatever holds them has stopped",
			len(out.Remaining))
	}
	fmt.Fprintln(stdout, "rashomon: purge complete")
	return nil
}

// purgeTarget is everything purge resolved and checked before planning.
type purgeTarget struct {
	root         string
	rec          store.InstallRecord
	settingsPath string
	settingsGoes bool // detach removes the file, which watch created
	createdDirs  []string
	refusedDirs  []string
	binary       string // "" when it could not be resolved
	binaryErr    error
}

func (t purgeTarget) binaryIf(on bool) string {
	if on {
		return t.binary
	}
	return ""
}

// resolvePurgeTarget applies step 1's refusals, in the order a user can act
// on them, and reads what the plan needs. Nothing here writes.
func resolvePurgeTarget() (purgeTarget, error) {
	var t purgeTarget
	root, err := store.DefaultRoot()
	if err != nil {
		return t, err
	}
	if t.root, err = filepath.Abs(root); err != nil {
		return t, err
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return t, errors.New("the home directory is not known, so purge cannot tell whether the store root is or encloses it; nothing was removed")
	}
	if home, err = filepath.Abs(home); err != nil {
		return t, err
	}
	resolvedRoot := resolvedOr(t.root)
	for _, r := range []string{t.root, resolvedRoot} {
		for _, h := range []string{home, resolvedOr(home)} {
			if err := purge.CheckRoot(r, h); err != nil {
				return t, err
			}
		}
	}
	for _, anchor := range []string{purge.InstallMeta, purge.InstallKey} {
		if _, err := os.Lstat(filepath.Join(t.root, anchor)); err != nil {
			return t, fmt.Errorf("%s holds no %s, so it is not a rashomon store; nothing was removed", t.root, anchor)
		}
	}
	if t.rec, err = store.ReadInstallRecord(t.root); err != nil {
		return t, fmt.Errorf("%s: %w; nothing was removed", filepath.Join(t.root, purge.InstallMeta), err)
	}
	if top := purge.WorkTreeTop(resolvedRoot); top != "" && !confirmedWorkTree(t.rec.RootInWorkTree, top) {
		return t, fmt.Errorf("%s is inside the git work tree %s, and watch recorded no confirmation of that root; "+
			"refusing to purge it, nothing was removed", t.root, top)
	}

	if t.settingsPath, err = settings.UserPath(); err != nil {
		return t, err
	}
	if err := refuseUnderEnabledPlugin(t.settingsPath); err != nil {
		return t, err
	}
	if t.settingsGoes, err = detachRemovesSettings(t.settingsPath, &t.rec); err != nil {
		return t, err
	}

	configDir, err := settings.ConfigDir()
	if err != nil {
		return t, err
	}
	if configDir, err = filepath.Abs(configDir); err != nil {
		return t, err
	}
	t.createdDirs, t.refusedDirs = purge.FilterCreatedDirs(t.rec.CreatedDirs, t.root, configDir, home)
	t.binary, t.binaryErr = install.Executable()
	return t, nil
}

// resolvedOr is p with symlinks resolved, or p itself when that fails.
func resolvedOr(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// confirmedWorkTree reports whether install.json's root_in_work_tree names
// the work tree the root is in now. A root that has since moved into another
// work tree was not confirmed for that one.
func confirmedWorkTree(recorded, top string) bool {
	if recorded == "" {
		return false
	}
	recorded = filepath.Clean(recorded)
	return recorded == top || resolvedOr(recorded) == top
}

// refuseUnderEnabledPlugin refuses while the rashomon plugin is enabled: its
// next hook would treat the missing store as a first run and create it
// again. enabledPlugins is read, never written.
func refuseUnderEnabledPlugin(settingsPath string) error {
	doc, err := settings.Load(settingsPath)
	if err != nil {
		return fmt.Errorf("%s: %w; nothing was removed", settingsPath, err)
	}
	p, err := install.FindPlugin(doc)
	if err != nil {
		return fmt.Errorf("reading the plugin install: %w; nothing was removed", err)
	}
	if p != nil && p.Enabled && len(p.Events) > 0 {
		return fmt.Errorf("the %s plugin is enabled, and its next hook would create the store again; "+
			"run `/plugin disable %s` in Claude Code first, then purge; nothing was removed", p.Key, p.Key)
	}
	return nil
}

// detachRemovesSettings reports whether purge's detach will remove the
// settings file: watch created it, it is still the file watch left, and
// taking this install's entries out leaves nothing in it.
func detachRemovesSettings(path string, rec *store.InstallRecord) (bool, error) {
	before, existed, err := settings.ReadState(path)
	if err != nil {
		return false, err
	}
	if !existed || !restoresAbsent(rec)(before) {
		return false, nil
	}
	doc, err := settings.Parse(before)
	if err != nil {
		return false, fmt.Errorf("%s: %w; nothing was removed", path, err)
	}
	if _, _, err := install.Remove(doc, install.Spec{InstallID: rec.InstallID}, false); err != nil {
		return false, err
	}
	return doc.Empty(), nil
}

// printPurgePlan is step 2: every path purge removes, and every path it
// leaves with what it would take to remove it.
func printPurgePlan(w io.Writer, t purgeTarget, p purge.Plan, withBinary bool) {
	fmt.Fprintf(w, "rashomon purge detaches install %s from %s\n", t.rec.InstallID, t.settingsPath)
	fmt.Fprintln(w, purgeRemovesHeader)
	if t.settingsGoes {
		fmt.Fprintf(w, "  %s\n", t.settingsPath)
	}
	for _, f := range p.Files {
		fmt.Fprintf(w, "  %s\n", f)
	}
	if p.RemoveRoot {
		fmt.Fprintf(w, "  %s\n", p.Root)
	}
	for _, d := range p.CreatedDirs {
		if d != p.Root {
			fmt.Fprintf(w, "  %s%s\n", d, purgeIfEmpty)
		}
	}
	if p.Binary != "" {
		fmt.Fprintf(w, "  %s\n", p.Binary)
	}

	fmt.Fprintln(w, purgeLeavesHeader)
	if !t.settingsGoes {
		fmt.Fprintf(w, "  %s -- kept; only rashomon's entries are taken out of it\n", t.settingsPath)
	}
	for _, f := range p.Foreign {
		info, err := os.Lstat(f)
		fmt.Fprintf(w, "  %s -- not written by rashomon; remove it yourself if it is not wanted: %s\n", f, removeCommand(f, err == nil && info.IsDir()))
	}
	if !p.RemoveRoot {
		fmt.Fprintf(w, "  %s -- kept, because it holds what is listed above\n", p.Root)
	}
	for _, d := range t.refusedDirs {
		fmt.Fprintf(w, "  %s -- install.json names it, but it is not a directory watch creates\n", d)
	}
	if !withBinary {
		if t.binary != "" {
			fmt.Fprintf(w, "  %s -- the binary, kept without --binary; remove it with: %s\n", t.binary, removeCommand(t.binary, false))
		} else {
			fmt.Fprintf(w, "  the rashomon binary -- kept without --binary (its path is not known: %v)\n", t.binaryErr)
		}
	}
}

// removeCommand is the shell command that removes path, a directory and
// what is in it when dir is set, quoted for the platform's shell.
func removeCommand(path string, dir bool) string {
	if runtime.GOOS == "windows" {
		cmd := "Remove-Item "
		if dir {
			cmd += "-Recurse "
		}
		return cmd + `"` + strings.ReplaceAll(path, `"`, "`\"") + `"`
	}
	cmd := "rm "
	if dir {
		cmd += "-r "
	}
	return cmd + "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// confirmOnTerminal asks purgePrompt on the controlling terminal -- never on
// stdin, which a pipe can answer for the user. With no terminal it refuses
// and names --yes.
func confirmOnTerminal() (bool, error) {
	in, out, closeTTY, err := openTerminal()
	if err != nil {
		return false, errors.New("there is no terminal to ask on, so nothing was removed; " +
			"run `rashomon purge --yes` to purge without being asked")
	}
	defer closeTTY()
	return confirm(in, out), nil
}

// confirm writes the prompt and reads one answer: y or yes, in any case, is
// yes; anything else, end of input included, is no.
func confirm(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, purgePrompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// openTerminal opens the controlling terminal for one question.
func openTerminal() (io.Reader, io.Writer, func(), error) {
	if runtime.GOOS == "windows" {
		in, err := os.Open("CONIN$")
		if err != nil {
			return nil, nil, nil, err
		}
		out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			in.Close() //nolint:errcheck // the open error is what matters
			return nil, nil, nil, err
		}
		return in, out, func() {
			in.Close()  //nolint:errcheck // read-only handle
			out.Close() //nolint:errcheck // nothing buffered
		}, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	return tty, tty, func() { tty.Close() }, nil //nolint:errcheck // nothing buffered
}
