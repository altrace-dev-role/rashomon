package acceptance

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestConsentGate2_APluginEnableReachesThePythonReadersScope (PY-K): the
// plugin route's act of consent, a plugin enable, records this
// build's scope (pluginConsent, pinned by
// TestPC1_EnablingThePluginIsConsentAndItsFirstSessionSteers), so on a build
// whose screen names Python files a plugin user's .py edits are kept. On a
// machine with nothing on file, and on one holding a yes of 8 from before
// (the screen that names Go files alone), the plugin's first session on this
// build leaves the highest scope on file no lower than the one pylite states
// its files kept from (lang.KeptFrom): a file below this build's scope does
// not cover it (consent.Covers), so the enable writes its own record. Breaks:
// write the plugin's record at the scope before this build's; take a file
// whose scope is older as covering this build.
func TestConsentGate2_APluginEnableReachesThePythonReadersScope(t *testing.T) {
	for _, c := range []struct {
		name   string
		before int // a yes on file before the plugin's first session; 0 for none
	}{
		{"nothing on file", 0},
		{"a yes of 8 on file", 8},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, fx := pcEnv(t)
			if c.before > 0 {
				e.lrAppend(c.before, consent.RecordScopeV5)
				if got, err := consent.ScopeOnFile(e.home); err != nil || got != c.before {
					t.Fatalf("premise: the highest scope on file reads %d (%v); want %d", got, err, c.before)
				}
			}
			e.pcStart(fx, testSession)
			got, err := consent.ScopeOnFile(e.home)
			if err != nil || got < pylite.SessionKeptScope {
				t.Errorf("after the plugin's first session the highest scope on file is %d (%v); want at least %d, the scope pylite states its files kept from",
					got, err, pylite.SessionKeptScope)
			}
		})
	}
}
