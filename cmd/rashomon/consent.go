package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/hook"
)

// cmdConsent is `rashomon consent [--yes] [--show]` (Technical spec 7.2.3,
// 7.2.11): the consent screen of an installed store, opening with what
// changed since the consent on file, under watch's terminal rule. y appends
// a consent record and nothing else; it is the one command that widens the
// record scope of a store installed by an older release.
func cmdConsent(args []string, stdout io.Writer) error {
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
		return consentAppendError(plan.root, err)
	}
	fmt.Fprintf(stdout, "rashomon: consent recorded for scope %d (install %s) in %s\n", rec.ScopeVersion, plan.installID, consent.Path(plan.root))
	if s := gate.heldSignal(); s != nil {
		return fmt.Errorf("%s arrived while consent was writing; it was held until the record above was written", s)
	}
	return nil
}

// wireConsentViews hands the hook package consent.json's two readers, which
// it cannot import without a cycle: the SessionStart probe syncs the
// record_scope and steer_disclosure markers through them. main calls it
// before any hook runs.
func wireConsentViews() {
	hook.ConsentRecordScope = consent.RecordScope
	hook.ConsentSteerDisclosure = consent.SteerDisclosure
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
