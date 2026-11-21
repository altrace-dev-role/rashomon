package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ubPyYes leaves one record in s's consent.json, a yes through the watch
// screen at scope, and has the hook read it as main wires it
// (wireConsentViews): ConsentScopeOnFile is consent.ScopeOnFile for the test,
// since nothing in process wires it. consent.Append writes the store's three
// markers from consent.json's own answer, and a plan session's are its own
// (newPlanSession, ubPostSession), so they are written again here as they
// were: between two scopes only the scope on file differs. Premise: the
// highest scope on file reads scope.
func ubPyYes(t *testing.T, s *planSession, scope int) {
	t.Helper()
	old := hook.ConsentScopeOnFile
	hook.ConsentScopeOnFile = consent.ScopeOnFile
	t.Cleanup(func() { hook.ConsentScopeOnFile = old })
	if err := os.Remove(consent.Path(s.root)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if err := consent.Append(s.root, consent.Record{
		V: consent.RecordVersion, ScopeVersion: scope, RecordScope: consent.RecordScopeV5,
		SteerDisclosure: steer.DisclosureVersion, Steer: consent.SteerOn, Via: consent.ViaWatchYes, At: 1760000000001,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRecordScopeMarker(s.root, store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSteerDisclosureMarker(s.root, steer.DisclosureVersion); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := consent.ScopeOnFile(s.root); err != nil || got != scope {
		t.Fatalf("premise: the highest scope on file reads %d (%v); want %d", got, err, scope)
	}
}

// TestConsentGate2_APyEditUnderAYesOfEightWritesNoMiss (card x381): over
// ubPySession, the header naming this build's py producer and no brief
// stored, a landed edit of the py unit's file under a yes of 8, the
// session-content marker set, writes no unitbrief_miss and prints nothing:
// pylite states its files kept from 9 (lang.KeptFrom) and no yes on file
// reaches it, so the edit passes no gate and the post has no landed edit to
// read a unit for (hook.Post.LandedGoEdit). Under a yes of 9 the same edit
// writes the one miss TestUnitBrief_PyUnitMissIsCounted reads. Base (x371):
// the scope was read past the gate, so under 8 the miss was written. Break:
// leave the scope check out of the gate.
func TestConsentGate2_APyEditUnderAYesOfEightWritesNoMiss(t *testing.T) {
	for _, c := range []struct {
		name   string
		scope  int
		misses int
	}{
		{"a yes of 8", 8, 0},
		{"a yes of 9", 9, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := ubPySession(t, pylite.Resolver{}.Producer())
			ubPyYes(t, s, c.scope)
			out := s.editPy("toolu_cg2m", ubPyHeld)
			if n := len(s.misses()); n != c.misses {
				t.Errorf("the edit under a yes of %d wrote %d unitbrief_miss records; want %d", c.scope, n, c.misses)
			}
			if c.misses == 0 && out != "" {
				t.Errorf("the edit under a yes of %d printed %q; want nothing", c.scope, out)
			}
		})
	}
}
