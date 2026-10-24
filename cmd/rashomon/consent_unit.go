package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
)

// consentUnitArgs is the arguments that are not flags: the unit names, which
// is how cmdConsent tells `consent <unit>` from `consent [--yes] [--show]`.
func consentUnitArgs(args []string) []string {
	var units []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			units = append(units, a)
		}
	}
	return units
}

// refuseUnitYes is --yes given to a unit: no flag stands in for a person's
// answer to the notice. It is returned only outside an agent session, where
// askConsent has already refused, so it names the flag to a person and never
// to an agent.
const refuseUnitYes = "a unit is agreed to at a terminal; --yes does not stand in for your answer; nothing was written. Run rashomon consent %s in your own terminal, without --yes."

// cmdConsentUnit is `rashomon consent <unit> [--show]` (Task 4.2): show the
// unit's notice and, at a terminal, append a unit_tty record that names the
// unit and holds the hash of the notice shown. It refuses inside an agent
// session and without a terminal exactly as watch does, and takes no --yes. A
// unit sits under record, so it refuses where there is no consent to install.
func cmdConsentUnit(args, units []string, stdout io.Writer) error {
	if len(units) > 1 {
		return errors.New("consent takes one unit at a time")
	}
	var yes, show bool
	for _, a := range args {
		switch a {
		case "--yes":
			yes = true
		case "--show":
			show = true
		case units[0]:
		default:
			return fmt.Errorf("unknown argument %q", a)
		}
	}
	unit, ok := consent.ParseUnit(units[0])
	if !ok {
		names := make([]string, len(consent.Units))
		for i, u := range consent.Units {
			names[i] = string(u)
		}
		return fmt.Errorf("%q is not a consent unit; the units are %s", units[0], strings.Join(names, ", "))
	}
	if unit == consent.UnitRecord {
		return errors.New("record is granted by rashomon watch, which installs; there is nothing to ask here")
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
	if plan.prior == nil {
		return errors.New("record is not on here, and a unit sits under it; rashomon consent shows the screen to agree to")
	}
	notice := []byte(consent.Notice(unit))
	verb := "consent " + string(unit)
	ans, err := askConsent(verb, notice, yes, show, workTreeQuestion{}, stdout, consent.QuestionUnit)
	if err != nil || ans == nil {
		return err
	}
	if !ans.tty {
		return fmt.Errorf(refuseUnitYes, unit)
	}
	id, err := consent.NewInvocationID()
	if err != nil {
		return err
	}
	rec := consent.Record{
		V:             consent.RecordVersion,
		Steer:         plan.prior.Steer,
		TextSHA256:    consent.TextSHA256(notice),
		BinaryVersion: version,
		BinarySHA256:  plan.exeSHA256,
		Via:           consent.ViaUnitTTY,
		TTY:           true,
		InvocationID:  id,
		At:            time.Now().UnixMilli(),
		Unit:          unit,
	}
	gate.hold()
	if err := consent.Append(plan.root, rec); err != nil {
		return fmt.Errorf("%s failed: %w; nothing was written", stepConsent, err)
	}
	fmt.Fprintf(stdout, "rashomon: %s is on (install %s), recorded in %s\n", unit, plan.installID, consent.Path(plan.root))
	if s := gate.heldSignal(); s != nil {
		return fmt.Errorf("%s arrived while consent was writing; it was held until the record above was written", s)
	}
	return nil
}
