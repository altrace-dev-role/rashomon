package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/hook"
)

// cmdConsent is `rashomon consent [--yes] [--show] [--full]` (Technical
// spec 7.2.3, 7.2.11): the consent screen of an installed store, opening
// with what changed since the consent on file, under watch's terminal rule.
// y appends a consent record and nothing else; it is the one command that
// widens the record scope of a store installed by an older release.
func cmdConsent(args []string, stdout io.Writer) error {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return cmdConsentUnit(args[0], args[1:], stdout)
	}
	yes, show, full, err := consentFlags(args)
	if err != nil {
		return err
	}
	if full {
		return printFull(stdout)
	}
	gate := startSignalGate(os.Stderr)
	defer gate.stop()

	plan, err := readPlan(nil)
	if err != nil {
		return err
	}
	if plan.installID == "" {
		return errors.New("nothing is installed here, so there is nothing to agree to; rashomon watch shows the screen and installs")
	}
	inv, err := disclose.Default()
	if err != nil {
		return err
	}
	screen, err := consent.Short(inv, plan.screenInput())
	if err != nil {
		return err
	}
	ans, err := askConsent("consent", screen, yes, show, workTreeQuestion{}, stdout, consent.QuestionConsent)
	if err != nil || ans == nil {
		return err
	}
	rec, err := plan.record(screen, ans)
	if err != nil {
		return err
	}
	gate.hold()
	if err := consent.Append(plan.root, rec); err != nil {
		return consentAppendError(plan.root, err)
	}
	fmt.Fprintf(stdout, "rashomon: consent recorded for scope %d (install %s) in %s\n", rec.ScopeVersion, plan.installID, consent.Path(plan.root))
	if s := gate.heldSignal(); s != nil {
		return fmt.Errorf("%s arrived while consent was writing; it was held until the record above was written", s)
	}
	return nil
}

// consentFlags parses watch's and consent's flags: --yes, --show and
// --full, and nothing else. --full prints every detail and writes nothing,
// so it takes neither of the others.
func consentFlags(args []string) (yes, show, full bool, err error) {
	for _, a := range args {
		switch a {
		case "--yes":
			yes = true
		case "--show":
			show = true
		case "--full":
			full = true
		default:
			return false, false, false, fmt.Errorf("unknown argument %q", a)
		}
	}
	if full && (yes || show) {
		return false, false, false, errors.New("--full prints every detail and writes nothing, so it takes no --yes or --show")
	}
	return yes, show, full, nil
}

// printFull is --full for watch and consent (CONSENT-SHORT1): the five
// sections the short screen sums up, every detail of what a yes does, printed
// before any install check and from an agent session alike, writing nothing,
// so a person can read all of it before typing y. --show stays the screen a
// y is asked under, the bytes text_sha256 hashes.
func printFull(stdout io.Writer) error {
	plan, err := readPlan(nil)
	if err != nil {
		return err
	}
	inv, err := disclose.Default()
	if err != nil {
		return err
	}
	screen, err := consent.Screen(inv, plan.screenInput())
	if err != nil {
		return err
	}
	_, err = stdout.Write(screen)
	return err
}

// wireConsentViews hands the hook package consent.json's three readers,
// which it cannot import without a cycle: the SessionStart probe syncs the
// record_scope and steer_disclosure markers through two, and SessionEnd asks
// the third whether the bodies unit is granted. main calls it before any
// hook runs.
func wireConsentViews() {
	hook.ConsentRecordScope = consent.RecordScope
	hook.ConsentSteerDisclosure = consent.SteerDisclosure
	hook.ConsentHas = consent.Has
}

// consentAppendError is the consent command's message for an Append error.
// A line on file whose marker after it was not written says the record is
// held and what the missing marker withholds; only an Append that wrote
// nothing says nothing was written. With both markers unwritten, the record
// scope's consequence is named and the error names both.
func consentAppendError(root string, err error) error {
	switch {
	case errors.Is(err, consent.ErrScopeMarker):
		return fmt.Errorf("%s: %s holds the record, but %w; recording keeps the v4 shape until it is", stepConsent, consent.Path(root), err)
	case errors.Is(err, consent.ErrSteerMarker):
		return fmt.Errorf("%s: %s holds the record, but %w; notes reach the agent as flags until it is", stepConsent, consent.Path(root), err)
	}
	return fmt.Errorf("%s failed: %w; nothing was written", stepConsent, err)
}
