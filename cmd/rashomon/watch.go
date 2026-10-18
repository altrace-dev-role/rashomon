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
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ttyPath is where the questions are asked. Never stdin: a script or an
// agent can write to a command's stdin, and an answer read from there is not
// a person's answer.
const ttyPath = "/dev/tty"

// The refusals watch and consent print after the screen. Neither names
// --yes: an agent told how to get past a refusal would use it.
const (
	refuseAgent    = "%s was run from an agent session; nothing was written. Read the screen above, then run rashomon %s in your own terminal."
	refuseNoTTY    = "%s needs a terminal to ask; nothing was written. Read the screen above, then run it in your own terminal."
	nothingWritten = "nothing was written"
)

// consentFlags parses watch's and consent's flags: --yes and --show, and
// nothing else.
func consentFlags(args []string) (yes, show bool, err error) {
	for _, a := range args {
		switch a {
		case "--yes":
			yes = true
		case "--show":
			show = true
		default:
			return false, false, fmt.Errorf("unknown argument %q", a)
		}
	}
	return yes, show, nil
}

// watchPlan is what steps 1 and 2 found: everything step 6 needs, read
// without writing anything.
type watchPlan struct {
	loc       settings.Locations
	root      string
	exe       string
	exeSHA256 string
	installID string // "" when there is no store yet
	changes   bool   // install.Apply would change settings.json
	covered   bool   // the consent on file covers this build's scope
	prior     *consent.Record
}

// cmdWatch is `rashomon watch [--yes] [--show]` (Technical spec 7.2.3).
func cmdWatch(args []string, stdout io.Writer) error {
	yes, show, err := consentFlags(args)
	if err != nil {
		return err
	}
	gate := startSignalGate(os.Stderr)
	defer gate.stop()

	plan, err := planWatch()
	if err != nil {
		return err
	}
	if !plan.changes && plan.covered {
		fmt.Fprintf(stdout, "rashomon: already watching (install %s) via %s\n", plan.installID, plan.loc.User)
		capProblem(stdout, "rashomon: ")
		printUndoLine(stdout, plan.installID)
		return nil
	}

	inv, err := disclose.Default()
	if err != nil {
		return err
	}
	screen, err := consent.Screen(inv, plan.screenInput())
	if err != nil {
		return err
	}
	ans, err := askConsent("watch", screen, yes, show, stdout, consent.QuestionInstall, consent.QuestionSDK)
	if err != nil || ans == nil {
		return err
	}

	rec, err := plan.record(screen, ans)
	if err != nil {
		return err
	}
	gate.hold()
	installID, foreign, err := applyWatch(plan, rec, ans.sdk)
	if err != nil {
		// When settings.json could not be put back, the entries stay
		// installed, and this line is the one way left to take them out.
		if errors.Is(err, errEntriesLeft) {
			printUndoLine(stdout, installID)
		}
		return err
	}
	fmt.Fprintf(stdout, "rashomon: watching (install %s) via %s\n", installID, plan.loc.User)
	// Said here because there is nowhere else it can be said: the foreign
	// entries run under this environment, stand down against this store, and
	// leave nothing behind to notice afterwards.
	if len(foreign) > 0 {
		fmt.Fprintf(stdout, "rashomon: entries from other installs are present (%s); they will fire in this environment and record nothing\n",
			strings.Join(foreign, ", "))
	}
	capProblem(stdout, "rashomon: ")
	printUndoLine(stdout, installID)
	fmt.Fprintln(stdout, "rashomon: rashomon disclose shows everything rashomon does here")
	if s := gate.heldSignal(); s != nil {
		return fmt.Errorf("%s arrived while watch was writing; it was held until the install above completed", s)
	}
	return nil
}

// printUndoLine prints the detach line for installID. It is printed in
// full, on its own line, because it is the line that still works once this
// binary or the store is gone: the id cannot be read back from a store that
// is not there, and there is nowhere else it is written down.
func printUndoLine(stdout io.Writer, installID string) {
	fmt.Fprintln(stdout, "rashomon: undo with this line -- it needs no store, and any rashomon binary will do:")
	fmt.Fprintf(stdout, "rashomon detach --install %s\n", installID)
}

// answers is what the person, or --yes, said.
type answers struct {
	via consent.Via
	tty bool
	sdk bool
}

// askConsent is step 4 and step 5 for watch and consent: print the screen
// and stop (--show), refuse an agent session, take --yes, or ask on the
// terminal. It returns nil answers, and a nil error only for --show, when
// the command is done without writing anything. Every question is asked
// here, before anything is written.
func askConsent(verb string, screen []byte, yes, show bool, stdout io.Writer, questions ...string) (*answers, error) {
	if show {
		_, err := stdout.Write(screen)
		return nil, err
	}
	if signal := selfguard.AgentSignal(os.Getenv); signal != "" {
		if _, err := stdout.Write(screen); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf(refuseAgent, verb, verb)
	}
	viaYes, viaTTY := consent.ViaWatchYes, consent.ViaWatchTTY
	if verb == "consent" {
		viaYes, viaTTY = consent.ViaConsentYes, consent.ViaConsentTTY
	}
	if yes {
		if _, err := stdout.Write(screen); err != nil {
			return nil, err
		}
		// Neither question is asked, so the Agent SDK one takes its
		// default, no.
		return &answers{via: viaYes}, nil
	}

	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		if _, werr := stdout.Write(screen); werr != nil {
			return nil, werr
		}
		return nil, fmt.Errorf(refuseNoTTY, verb)
	}
	defer tty.Close() //nolint:errcheck // a terminal, nothing buffered to lose
	if _, err := tty.Write(screen); err != nil {
		return nil, err
	}
	term := consent.NewTerminal(tty)
	ans := &answers{via: viaTTY, tty: true}
	for i, q := range questions {
		reply, eof, err := term.Ask("\n" + q)
		if err != nil {
			return nil, err
		}
		yes := !eof && consent.IsYes(reply)
		switch {
		case i == 0 && !yes:
			return nil, errors.New(nothingWritten)
		case i == 1:
			ans.sdk = yes
		}
	}
	return ans, nil
}

// planWatch is steps 1 and 2: the refusals, then what install.Apply would
// change, computed on an in-memory copy, and the consent on file, read
// without opening the store.
func planWatch() (*watchPlan, error) {
	plan, err := readPlan(refuseUnrunnable)
	if err != nil {
		return nil, err
	}
	original, err := settings.ReadOriginal(plan.loc.User)
	if err != nil {
		return nil, err
	}
	doc, err := settings.Parse(original)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", plan.loc.User, err)
	}
	planID := plan.installID
	if planID == "" {
		planID = consent.NewInstallPlaceholder
	}
	if plan.changes, err = install.Apply(doc, install.Spec{Executable: plan.exe, InstallID: planID}); err != nil {
		return nil, err
	}
	return plan, nil
}

// readPlan is what watch and consent read before they show the screen, and
// none of it writes: the settings locations (refused by refuse, when given),
// the binary and its hash, the store root and install id, read without
// opening the store, and the consent on file.
func readPlan(refuse func(settings.Locations) error) (*watchPlan, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	loc, err := settings.DefaultLocations(cwd)
	if err != nil {
		return nil, err
	}
	if refuse != nil {
		if err := refuse(loc); err != nil {
			return nil, err
		}
	}
	exe, err := install.Executable()
	if err != nil {
		return nil, err
	}
	exeSHA, err := fileSHA256(exe)
	if err != nil {
		return nil, fmt.Errorf("hashing %s: %w", exe, err)
	}
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, err
	}
	id, err := store.ReadInstallID(root)
	if err != nil && !errors.Is(err, store.ErrNoStore) {
		return nil, err
	}

	plan := &watchPlan{loc: loc, root: root, exe: exe, exeSHA256: exeSHA, installID: id}
	// An unreadable consent.json covers nothing and has no prior: the
	// screen is shown and asked again rather than trusted.
	if covered, err := consent.ScopeCovered(root); err == nil {
		plan.covered = covered
	}
	if recs, err := consent.Read(root); err == nil {
		if r, ok := consent.LastAccepted(recs); ok {
			plan.prior = &r
		}
	}
	return plan, nil
}

// refuseUnrunnable is step 1: a settings layer that cannot be parsed, hooks
// disabled at any layer, managed policy allowing only its own hooks, or a
// plugin already providing the entries. Each would leave an entry that
// never runs, or one that records every call twice.
func refuseUnrunnable(loc settings.Locations) error {
	decision, err := settings.HooksDisabled(loc)
	if err != nil {
		return err
	}
	if decision.Disabled {
		return fmt.Errorf("hooks are disabled by the %s settings layer; an installed entry would never run", decision.Layer)
	}
	only, err := settings.ManagedHooksOnly(loc)
	if err != nil {
		return err
	}
	if only {
		return errors.New("the managed settings layer sets allowManagedHooksOnly, so Claude Code runs only the hooks policy defines; an entry in your settings would never run")
	}
	// standsDown cannot save this arrangement -- it stands down on an
	// install-id MISMATCH, and a plugin invocation names no id to mismatch
	// against, so both origins would record every tool call.
	userDoc, err := settings.Load(loc.User)
	if err != nil {
		return err
	}
	if p, err := install.FindPlugin(userDoc); err != nil {
		return err
	} else if p != nil && p.Live() {
		return fmt.Errorf("the %s plugin is enabled and already provides these hook entries (%s); "+
			"installing here would record every tool call twice -- run `/plugin disable %s` first, "+
			"or skip watch and keep using the plugin", p.Key, strings.Join(p.Declared, ", "), p.Key)
	}
	return nil
}

func (p *watchPlan) screenInput() consent.ScreenInput {
	return consent.ScreenInput{
		Version:      version,
		SettingsPath: p.loc.User,
		Executable:   p.exe,
		InstallID:    p.installID,
		StoreRoot:    p.root,
		StoreExists:  p.installID != "",
		Prior:        p.prior,
	}
}

// record is the consent line step 6 appends. A --yes record carries
// steer_disclosure 0: the screen went to a script, not a person, so notes
// wait until it has been shown in an attended session.
func (p *watchPlan) record(screen []byte, ans *answers) (consent.Record, error) {
	inv, err := consent.NewInvocationID()
	if err != nil {
		return consent.Record{}, err
	}
	disclosure := steer.DisclosureVersion
	if !ans.tty {
		disclosure = 0
	}
	return consent.Record{
		V:               consent.RecordVersion,
		ScopeVersion:    disclose.ScopeVersion,
		RecordScope:     consent.RecordScopeV5,
		SteerDisclosure: disclosure,
		Steer:           consent.SteerOn,
		TextSHA256:      consent.TextSHA256(screen),
		BinaryVersion:   version,
		BinarySHA256:    p.exeSHA256,
		Via:             ans.via,
		TTY:             ans.tty,
		InvocationID:    inv,
		At:              time.Now().UnixMilli(),
	}, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// undoStep is one write of step 6 that has landed, and how to take it back.
type undoStep struct {
	name string
	undo func() error
}

// writes runs step 6's writes in order and, when one fails, undoes the ones
// before it in reverse and names the one that failed.
type writes struct{ done []undoStep }

func (w *writes) fail(step string, err error) error {
	var undone, stuck []string
	settingsLeft := false
	for i := len(w.done) - 1; i >= 0; i-- {
		if uerr := w.done[i].undo(); uerr != nil {
			stuck = append(stuck, fmt.Sprintf("%s (%v)", w.done[i].name, uerr))
			settingsLeft = settingsLeft || w.done[i].name == stepSettings
			continue
		}
		undone = append(undone, w.done[i].name)
	}
	msg := fmt.Sprintf("%s failed: %v", step, err)
	if len(undone) > 0 {
		msg += "; undid, in reverse: " + strings.Join(undone, ", ")
	}
	if settingsLeft {
		return fmt.Errorf("%s; COULD NOT UNDO: %s; %w", msg, strings.Join(stuck, ", "), errEntriesLeft)
	}
	if len(stuck) > 0 {
		return fmt.Errorf("%s; COULD NOT UNDO: %s", msg, strings.Join(stuck, ", "))
	}
	return errors.New(msg + "; nothing was left written")
}

// errEntriesLeft is a failed watch that could not put settings.json back:
// its entries are installed, and the undo line is printed for them.
var errEntriesLeft = errors.New("the entries are installed, and the line above removes them")

// Step names, as watch reports a failure in one.
const (
	stepStore    = "opening the store"
	stepCreated  = "recording the directories watch created"
	stepSDK      = "recording the Agent SDK answer"
	stepSettings = "editing settings.json"
	stepRecord   = "recording what watch changed"
	stepConsent  = "recording your consent"
)

// applyWatch is step 6: open the store, record the Agent SDK answer, edit
// settings.json, append the consent record. A failure undoes what came
// before it in reverse.
func applyWatch(plan *watchPlan, rec consent.Record, sdk bool) (installID string, foreign []string, err error) {
	var w writes

	// Taken before anything is created, so they name exactly what this watch
	// makes: the store root and its missing parents, the configuration
	// directory and its missing parents. purge removes them once empty.
	created, err := missingDirs(plan.root, filepath.Dir(plan.loc.User))
	if err != nil {
		return "", nil, err
	}
	st, undo, err := openStoreUndoably(plan.root)
	if err != nil {
		return "", nil, w.fail(stepStore, err)
	}
	w.done = append(w.done, undoStep{stepStore, undo})
	installID = st.InstallID()
	// Recorded before settings.json is touched (Technical spec 7.2.3 step
	// 6), so a watch that dies after its edit has still told purge which
	// directories it made.
	if err := recordCreatedDirs(st.Root(), created); err != nil {
		return installID, nil, w.fail(stepCreated, err)
	}
	fault.Inject(fault.PointWatchWriting)

	undo, err = restorable(consent.HarnessesPath(plan.root), fault.PointWatchSDK, func() error {
		return consent.WriteSDKAnswer(plan.root, sdk)
	})
	if err != nil {
		return installID, nil, w.fail(stepSDK, err)
	}
	w.done = append(w.done, undoStep{stepSDK, undo})

	spec := install.Spec{Executable: plan.exe, InstallID: installID}
	hadOurs := false
	edit, undo, err := editSettingsUndoably(plan.loc.User, func(doc *settings.Document) (bool, error) {
		var err error
		if hadOurs, err = holdsEntriesOf(doc, spec.InstallID); err != nil {
			return false, err
		}
		applied, err := install.Apply(doc, spec)
		if err != nil {
			return false, err
		}
		foreign, err = install.ForeignOwners(doc, spec.InstallID)
		return applied, err
	})
	if err != nil {
		return installID, nil, w.fail(stepSettings, err)
	}
	w.done = append(w.done, undoStep{stepSettings, undo})

	// What the edit found and left, which detach checks its restore against.
	fault.Inject(fault.PointWatchRecord)
	err = fault.Fail(fault.PointWatchRecord)
	if err == nil {
		err = recordWatch(st.Root(), edit, hadOurs)
	}
	if err != nil {
		return installID, nil, w.fail(stepRecord, err)
	}

	if _, err := restorable(consent.Path(plan.root), fault.PointWatchConsent, func() error {
		return consent.Append(plan.root, rec)
	}); err != nil {
		return installID, nil, w.fail(stepConsent, err)
	}
	return installID, foreign, nil
}

// openStoreUndoably opens the store, and returns how to remove what opening
// it created: install.json and install.key when they were not there, then
// the root and each parent directory it made, deepest first, while empty.
func openStoreUndoably(root string) (*store.Store, func() error, error) {
	if err := fault.Fail(fault.PointWatchStore); err != nil {
		return nil, nil, err
	}
	created, err := missingDirs(root)
	if err != nil {
		return nil, nil, err
	}
	var made []string
	for _, f := range []string{store.FileInstallKey, store.FileInstallMeta} {
		p := filepath.Join(root, f)
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			made = append(made, p)
		} else if err != nil {
			return nil, nil, err
		}
	}
	var mode fs.FileMode
	if info, err := os.Stat(root); err == nil {
		mode = info.Mode().Perm()
	}
	undo := func() error {
		var errs []error
		for i := len(made) - 1; i >= 0; i-- {
			if err := os.Remove(made[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
		if len(created) == 0 && mode != 0 {
			if err := os.Chmod(root, mode); err != nil {
				errs = append(errs, err)
			}
		}
		errs = append(errs, removeDirs(created))
		return errors.Join(errs...)
	}
	st, err := store.Open(root)
	if err != nil {
		if uerr := undo(); uerr != nil {
			return nil, nil, errors.Join(err, uerr)
		}
		return nil, nil, err
	}
	return st, undo, nil
}

// restorable runs write, after the fault point, and returns how to put the
// file at path back as it was: its old bytes, or no file where there was
// none. A write that fails part way is put back before the error returns.
func restorable(path, point string, write func() error) (func() error, error) {
	old, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	undo := func() error {
		if !existed {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}
		return os.WriteFile(path, old, 0o600)
	}
	if err := fault.Fail(point); err != nil {
		return nil, err
	}
	if err := write(); err != nil {
		if uerr := undo(); uerr != nil {
			return nil, errors.Join(err, uerr)
		}
		return nil, err
	}
	return undo, nil
}

// editSettingsUndoably applies edit to settings.json, and returns what the
// edit found and left and how to put the file back: its old bytes, or no
// file and none of the directories the write created where there was none
// -- each only while the file still holds what this edit wrote, so a change
// made since is never overwritten.
func editSettingsUndoably(path string, edit func(*settings.Document) (bool, error)) (settingsEdit, func() error, error) {
	if err := fault.Fail(fault.PointWatchSettings); err != nil {
		return settingsEdit{}, nil, err
	}
	dirs, err := missingDirs(filepath.Dir(path))
	if err != nil {
		return settingsEdit{}, nil, err
	}
	res, err := editSettings(path, edit, nil)
	if err != nil {
		return settingsEdit{}, nil, errors.Join(err, removeDirs(dirs))
	}
	if !res.changed {
		return res, func() error { return nil }, nil
	}
	return res, func() error {
		if !res.existed {
			if err := settings.Remove(path, res.after); err != nil {
				return err
			}
			return removeDirs(dirs)
		}
		return settings.Write(path, res.after, res.before)
	}, nil
}

// removeDirs removes dirs, as missingDirs lists them (outermost first),
// deepest first, failing on one that is no longer empty rather than deleting
// what something else put there.
func removeDirs(dirs []string) error {
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Remove(dirs[i]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
