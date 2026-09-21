package acceptance

import "testing"

// H-99 -- the recorder still reaches no network.
//
// Break: import it from internal/hook and H-99 fails.
//
// Part 5 is what H-17's own design anticipates without needing to change:
// the "type": "prompt" hook makes Claude Code call a model on rashomon's
// behalf, so rashomon's own process gains no new caller of net, net/http,
// crypto/tls or os/exec, and internal/install/reading.go -- the whole of
// Part 5's footprint in this binary -- never needed to touch
// internal/hook at all. TestH17_NoNetworkInTheRecordersDependencyGraph
// already re-verifies the recorder's dependency graph on every run and
// would fail if that stopped being true; what H-99 adds is the positive,
// named claim that stays true FOR THE RIGHT REASON: Part 5's own package
// is a named source with no forbidden import of its own, not merely a
// package the sweep never got around to checking.
func TestH99_ReadingPackageCarriesNoForbiddenImport(t *testing.T) {
	pkgs := deps(t, "./internal/install")
	for _, pkg := range pkgs {
		if why, bad := forbiddenImports[pkg]; bad {
			t.Errorf("internal/install (which renders Part 5's entry) depends on %s, "+
				"which %s -- Part 5's entire mechanism is that Claude Code makes the "+
				"model call, not this program, and this import means something else "+
				"is trying to", pkg, why)
		}
	}
}

// TestH99_RecorderStillReachesNoNetworkWithReadingPresent is the direct
// re-run of H-17's own recorder test, kept here under Part 5's name rather
// than trusted to fail silently under H-17's. internal/hook already
// depends on internal/install -- coverage.go calls install.Present to
// resolve hook_entry, a dependency Part 5 did not create and does not
// touch -- so the claim H-99 actually needs to make is narrower than "the
// recorder never imports internal/install": it is that adding reading.go
// to that already-shared package did not smuggle in anything forbidden
// alongside it. This is the same assertion TestH17_..., re-run under this
// name so a reviewer looking for H-99's evidence finds it without having to
// know H-17 already covers it.
func TestH99_RecorderStillReachesNoNetworkWithReadingPresent(t *testing.T) {
	pkgs := deps(t, "./internal/hook")
	for _, pkg := range pkgs {
		if why, bad := forbiddenImports[pkg]; bad {
			t.Errorf("the recorder depends on %s, which %s -- Part 5's own package "+
				"must not be the reason", pkg, why)
		}
	}
}
