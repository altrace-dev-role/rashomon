package main

import (
	"os"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/settings"
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
//   - Consent is the steer_disclosure marker, never consent.json: the marker
//     is the hook path's view of it (D49, D84), which the SessionStart probe
//     keeps in step.
//   - ProjectDir and ManagedDir are specDirs'.
func steerEnv(id harness.Identity, now time.Time) hook.SteerEnv {
	projectDir, managedDir := specDirs()
	return hook.SteerEnv{
		HarnessID:    string(id.ID),
		Surface:      string(id.Surface),
		MirrorInBand: harness.Caps(id.ID, id.Surface)[harness.CapMirrorInBand] == harness.Yes,
		AIAgent:      os.Getenv("AI_AGENT"),
		CI:           os.Getenv("CI") != "",
		HoldoutOff:   hook.HoldoutOff(os.Getenv(hook.EnvHoldout)),
		HoldoutOn:    hook.HoldoutOn(os.Getenv(hook.EnvHoldout)),
		Consent:      store.SteerDisclosureMarker,
		Now:          now,
		ProjectDir:   projectDir,
		ManagedDir:   managedDir,
	}
}

// specDirs is hook.DiscoverSpec's two directories from this process (card 3.1
// (c)): CLAUDE_PROJECT_DIR as claudeEnv reads it, and the managed-settings
// directory, under which the managed spec is rashomon/spec.json.
func specDirs() (projectDir, managedDir string) {
	return os.Getenv("CLAUDE_PROJECT_DIR"), settings.ManagedDir()
}
