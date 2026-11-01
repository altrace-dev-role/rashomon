package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/selfguard"
)

// cmdConsentUnit is `rashomon consent <unit> [--yes] [--show] [off]` (card
// 4.2): the unit's screen, asked as the install screen is, and one record
// holding the answer. off is the user narrowing their own consent, so it asks
// nothing: it prints one line and the record hashes that line. A unit the
// inventory has no paragraph for has no command in this release and is not
// asked at all.
func cmdConsentUnit(unit string, args []string, stdout io.Writer) error {
	if !slices.Contains(consent.Units, unit) {
		return fmt.Errorf("unknown argument %q; the consent units are %s", unit, strings.Join(consent.Units, ", "))
	}
	inv, err := disclose.Default()
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(inv.Units, func(u disclose.UnitParagraph) bool { return u.Unit == unit }) {
		return fmt.Errorf("no command in this release uses %s; nothing to agree to", unit)
	}
	off := slices.Contains(args, "off")
	yes, show, full, err := consentFlags(slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "off" }))
	if err != nil {
		return err
	}
	if full {
		return errors.New("a unit's screen is whole; --full is for the install screen (rashomon consent --full)")
	}
	if off && (yes || show) {
		return errors.New("off asks nothing and shows one line, so it takes no --yes or --show")
	}
	// Granting a unit is refused in an agent session by askConsent, which
	// shows the screen and writes nothing. Withdrawing one asks nothing, so
	// it needs its own refusal: the unit is the person's to turn off, and an
	// agent that could would stop what was agreed to, before a record is
	// appended or a lock taken.
	if s := selfguard.AgentSignal(os.Getenv); off && s != "" {
		return fmt.Errorf("%s is set, so this is an agent session; nothing was written. Run `rashomon consent %s off` in your own terminal.", s, unit)
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
	shown := []byte("rashomon: " + unit + " is off; nothing more is done under it until you run rashomon consent " + unit + "\n")
	ans := &answers{via: consent.ViaConsentYes}
	if off {
		if _, err := stdout.Write(shown); err != nil {
			return err
		}
	} else {
		if shown, err = consent.UnitScreen(inv, unit, plan.screenInput()); err != nil {
			return err
		}
		if ans, err = askConsent("consent", shown, yes, show, workTreeQuestion{}, stdout, consent.QuestionConsent); err != nil || ans == nil {
			return err
		}
	}
	id, err := consent.NewInvocationID()
	if err != nil {
		return err
	}
	rec := consent.Record{V: consent.RecordVersion, TextSHA256: consent.TextSHA256(shown), BinaryVersion: buildVersion(),
		BinarySHA256: plan.exeSHA256, Via: ans.via, TTY: ans.tty, InvocationID: id, At: time.Now().UnixMilli(),
		Unit: unit, UnitOn: !off}
	gate.hold()
	if err := consent.Append(plan.root, rec); err != nil {
		return consentAppendError(plan.root, err)
	}
	if !off {
		fmt.Fprintf(stdout, "rashomon: consent to %s recorded in %s\n", unit, consent.Path(plan.root))
	}
	if s := gate.heldSignal(); s != nil {
		return fmt.Errorf("%s arrived while consent was writing; it was held until the record above was written", s)
	}
	return nil
}
