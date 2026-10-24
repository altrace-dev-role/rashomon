package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// cmdConsent is `rashomon consent [--yes] [--show]` (Technical spec 7.2.3,
// 7.2.11): the consent screen of an installed store, opening with what
// changed since the consent on file, under watch's terminal rule. y appends
// a consent record and nothing else; it is the one command that widens the
// record scope of a store installed by an older release.
//
// With a unit name -- `rashomon consent <unit> [--show]` -- it is the notice
// for that unit instead of the install screen (cmdConsentUnit).
func cmdConsent(args []string, stdout io.Writer) error {
	if units := consentUnitArgs(args); len(units) > 0 {
		return cmdConsentUnit(args, units, stdout)
	}
	yes, show, err := consentFlags(args)
	if err != nil {
		return err
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
	screen, err := consent.Screen(inv, plan.screenInput())
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
		return fmt.Errorf("%s failed: %w; nothing was written", stepConsent, err)
	}
	fmt.Fprintf(stdout, "rashomon: consent recorded for scope %d (install %s) in %s\n", rec.ScopeVersion, plan.installID, consent.Path(plan.root))
	if s := gate.heldSignal(); s != nil {
		return fmt.Errorf("%s arrived while consent was writing; it was held until the record above was written", s)
	}
	return nil
}
