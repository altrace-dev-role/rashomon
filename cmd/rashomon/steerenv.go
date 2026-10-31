package main

import (
	"os"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// steerEnv is the post's steer step's port, read from this process (card 2.3
// R1): the hook package reads no environment, so cmd does it here.
//   - The harness and surface are the host guard's identity, and the in-band
//     mirror is what Caps says that pair verifies.
//   - AI_AGENT carries the client's version (steer.ClientVersion reads it).
//   - CI set to anything is an unattended run.
//   - RASHOMON_HOLDOUT (hook.EnvHoldout) set to "on" draws the field holdout
//     where no spec's holdout object sets it, which is off by default, and
//     "off" skips the draw: the operator's two switches (card 2.5).
//   - RASHOMON_STEER_ARM (hook.EnvSteerArm) set to "off" or "steer" forces
//     the paired run's arm (card MEASURE1). It is honoured on a lab store
//     only (operatorArm), so it can never take the holdout's place on a
//     normal install.
//   - Consent is the steer_disclosure marker, never consent.json: the marker
//     is the hook path's view of it (D49, D84), which the SessionStart probe
//     keeps in step.
//   - ProjectDir and ManagedDir are specDirs'.
func steerEnv(id harness.Identity, now time.Time, root string) hook.SteerEnv {
	projectDir, managedDir := specDirs()
	return hook.SteerEnv{
		HarnessID:    string(id.ID),
		Surface:      string(id.Surface),
		MirrorInBand: harness.Caps(id.ID, id.Surface)[harness.CapMirrorInBand] == harness.Yes,
		AIAgent:      os.Getenv("AI_AGENT"),
		CI:           os.Getenv("CI") != "",
		HoldoutOff:   hook.HoldoutOff(os.Getenv(hook.EnvHoldout)),
		HoldoutOn:    hook.HoldoutOn(os.Getenv(hook.EnvHoldout)),
		Arm:          operatorArm(root),
		Consent:      store.SteerDisclosureMarker,
		Now:          now,
		ProjectDir:   projectDir,
		ManagedDir:   managedDir,
	}
}

// operatorArm is the operator's forced arm as the store at root lets it
// count. A repository's committed settings can set RASHOMON_STEER_ARM for
// every session opened in it, and every hook inherits it, so it is honoured
// only when root holds the lab marker `rashomon measure --prepare` writes
// (store.Lab, card MEASURE1). Elsewhere it reads as unset, with no print and
// no error, as hook.SteerArm already reads a value it does not know: a forced
// arm skips the holdout's draw, so on a normal store it would switch the
// holdout off. The marker is checked only when the variable names an arm, so
// an install without it pays nothing.
func operatorArm(root string) steer.Arm {
	arm := hook.SteerArm(os.Getenv(hook.EnvSteerArm))
	if arm == "" || store.Lab(root) {
		return arm
	}
	return ""
}

// armOff reports operatorArm as off for the store at root: every hook print
// is withheld from everyone and the records are written as in every arm (card
// MEASURE1), so the off arm differs from the steer arm only in what printed.
func armOff(root string) bool {
	return operatorArm(root) == steer.ArmOff
}

// specDirs is hook.DiscoverSpec's two directories from this process (card 3.1
// (c)): CLAUDE_PROJECT_DIR as claudeEnv reads it, and the managed-settings
// directory, under which the managed spec is rashomon/spec.json.
func specDirs() (projectDir, managedDir string) {
	return os.Getenv("CLAUDE_PROJECT_DIR"), settings.ManagedDir()
}
