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
//   - RASHOMON_HOLDOUT (hook.EnvHoldout) set to "off" skips the field
//     holdout's draw: the operator's off (card 2.5, D109 as amended).
//   - RASHOMON_STEER_ARM (hook.EnvSteerArm) set to "off" or "steer" forces
//     the paired run's arm (card MEASURE1).
//   - Both are honoured on a lab store only (operatorEnv).
//   - Consent is the steer_disclosure marker, never consent.json: the marker
//     is the hook path's view of it (D49, D84), which the SessionStart probe
//     keeps in step.
//   - ProjectDir and ManagedDir are specDirs'.
func steerEnv(id harness.Identity, now time.Time, root string) hook.SteerEnv {
	projectDir, managedDir := specDirs()
	arm, holdoutOff := operatorEnv(root)
	return hook.SteerEnv{
		HarnessID:    string(id.ID),
		Surface:      string(id.Surface),
		MirrorInBand: harness.Caps(id.ID, id.Surface)[harness.CapMirrorInBand] == harness.Yes,
		AIAgent:      os.Getenv("AI_AGENT"),
		CI:           os.Getenv("CI") != "",
		HoldoutOff:   holdoutOff,
		Arm:          arm,
		Consent:      store.SteerDisclosureMarker,
		Now:          now,
		ProjectDir:   projectDir,
		ManagedDir:   managedDir,
	}
}

// operatorEnv is the operator's two session switches, the forced arm and the
// holdout's off, as the store at root lets them count. A repository's
// committed settings can set either variable for every session opened in it,
// and every hook inherits it, so both are honoured only when root holds the
// lab marker `rashomon measure --prepare` writes (store.Lab, card MEASURE1).
// Elsewhere both read as unset, with no print and no error, as HoldoutOff
// already reads a value it does not know. The marker is checked only when one
// of the two is set, so an install with neither pays nothing.
func operatorEnv(root string) (steer.Arm, bool) {
	arm, holdoutOff := hook.SteerArm(os.Getenv(hook.EnvSteerArm)), hook.HoldoutOff(os.Getenv(hook.EnvHoldout))
	if (arm == "" && !holdoutOff) || store.Lab(root) {
		return arm, holdoutOff
	}
	return "", false
}

// armOff reports operatorEnv's arm as off for the store at root: every hook
// print is withheld from everyone and the records are written as in every
// arm (card MEASURE1), so the off arm differs from the steer arm only in
// what printed.
func armOff(root string) bool {
	arm, _ := operatorEnv(root)
	return arm == steer.ArmOff
}

// specDirs is hook.DiscoverSpec's two directories from this process (card 3.1
// (c)): CLAUDE_PROJECT_DIR as claudeEnv reads it, and the managed-settings
// directory, under which the managed spec is rashomon/spec.json.
func specDirs() (projectDir, managedDir string) {
	return os.Getenv("CLAUDE_PROJECT_DIR"), settings.ManagedDir()
}
