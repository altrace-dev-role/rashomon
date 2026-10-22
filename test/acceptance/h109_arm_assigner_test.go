package acceptance

// H-109 -- one keyed holdout randomiser for every steering feature.
//
// A steering feature can say what it changed only by comparing the sessions it
// acted on with the sessions it left alone, and that comparison is evidence
// only if nothing about a session chose its side. internal/arm is the one
// place the side is chosen: a session's arm is drawn from an HMAC, under the
// per-install key the store already holds, of the feature name and the session
// id. So the same session always lands in the same arm, which lets every hook
// call in a session agree without storing anything; another install lands the
// same id independently; and an agent that knows the whole derivation but not
// the key does no better than a coin. There is no per-session opt-out and no
// environment override, because a holdout a session can leave is a
// self-selected one, and it measures who left rather than what the feature
// did.
//
// THE BOUND. Every split below is held to five standard deviations of a fair
// binomial draw, n*p +/- 5*sqrt(n*p*(1-p)). A fair randomiser lands outside
// that with probability about 5.7e-7 per check (normal approximation), so a
// failure here is a finding, not noise. The keys are minted by store.Open, the
// way an install gets its own, so every run draws a fresh sample rather than
// re-checking one lucky key; a failing test logs its key, which is what makes
// the failure reproducible.
//
// Breaks, one named on each test: the holdout share is ignored; the draw
// changes from call to call; the derivation moves; the key is left out of the
// digest; the feature name is left out of the digest; a session without an id
// is given an arm; an environment variable overrides the arm.

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// armSessions is how many synthetic session ids each distribution check draws.
const armSessions = 10000

// armSessionIDs returns n ids shaped like the UUIDs Claude Code assigns.
// Sequential on purpose: a draw that mixes its input badly shows it on
// neighbouring inputs first.
func armSessionIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%08x-0000-4000-8000-%012x", i, i)
	}
	return ids
}

// armInstallKey mints a key the way an install does, by opening a fresh store,
// and logs it if the test fails so the draw can be replayed.
func armInstallKey(t *testing.T) []byte {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("opening a store: %v", err)
	}
	key := s.Key()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("install key for this run: %x", key)
		}
	})
	return key
}

func newArmAssigner(t *testing.T, key []byte) arm.Assigner {
	t.Helper()
	a, err := arm.New(key)
	if err != nil {
		t.Fatalf("arm.New with a %d-byte install key: %v", len(key), err)
	}
	return a
}

func mustAssign(t *testing.T, a arm.Assigner, feature, sessionID string, holdout int) arm.Arm {
	t.Helper()
	got, err := a.Assign(feature, sessionID, holdout)
	if err != nil {
		t.Fatalf("Assign(%q, %q, %d): %v", feature, sessionID, holdout, err)
	}
	if got != arm.Treatment && got != arm.Holdout {
		t.Fatalf("Assign(%q, %q, %d) = %q, which is neither arm", feature, sessionID, holdout, got)
	}
	return got
}

// withinFiveSigma holds a count of n draws at probability p to the bound in
// the header.
func withinFiveSigma(t *testing.T, what string, got, n int, p float64) {
	t.Helper()
	mean := float64(n) * p
	sd := math.Sqrt(float64(n) * p * (1 - p))
	lo, hi := mean-5*sd, mean+5*sd
	if float64(got) < lo || float64(got) > hi {
		t.Errorf("%s: %d of %d, want %.0f to %.0f (n*p +/- 5 sd at p = %g)", what, got, n, lo, hi, p)
	}
}

// TestH109_TheSplitOverTenThousandSessionsIsTheHoldoutShare is the plan's first
// done-when, at three shares rather than one: at a half, a randomiser that
// ignores the share, or holds out its complement, cannot be told from a
// correct one.
//
// Break: the holdout share is ignored.
func TestH109_TheSplitOverTenThousandSessionsIsTheHoldoutShare(t *testing.T) {
	a := newArmAssigner(t, armInstallKey(t))
	ids := armSessionIDs(armSessions)
	for _, share := range []int{5000, 1000, 2500} {
		held := 0
		for _, id := range ids {
			if mustAssign(t, a, "steer-split", id, share) == arm.Holdout {
				held++
			}
		}
		withinFiveSigma(t, fmt.Sprintf("held out at %d bp", share), held, len(ids), float64(share)/arm.BasisPoints)
	}
}

// TestH109_TheSameSessionAlwaysLandsInTheSameArm. Each hook call in a session
// asks again, in a process of its own, so the answer has to come back the same
// from the store's key alone: across calls, across a second assigner, and
// across a reopen of the store, which is what the next hook process does.
//
// Positive twin: both arms occur. A function that returned one constant arm
// would pass every sameness check here.
//
// Break: the draw changes from call to call.
func TestH109_TheSameSessionAlwaysLandsInTheSameArm(t *testing.T) {
	root := t.TempDir()
	first, err := store.Open(root)
	if err != nil {
		t.Fatalf("opening a store: %v", err)
	}
	reopened, err := store.Open(root)
	if err != nil {
		t.Fatalf("reopening the store: %v", err)
	}
	a := newArmAssigner(t, first.Key())
	b := newArmAssigner(t, reopened.Key())

	counts := map[arm.Arm]int{}
	for _, id := range armSessionIDs(armSessions) {
		want := mustAssign(t, a, "steer-same", id, 5000)
		counts[want]++
		for i, other := range []arm.Assigner{a, a, b, b} {
			if got := mustAssign(t, other, "steer-same", id, 5000); got != want {
				t.Fatalf("session %s: asked again (%d), it landed in %q after %q", id, i+1, got, want)
			}
		}
	}
	if counts[arm.Treatment] == 0 || counts[arm.Holdout] == 0 {
		t.Errorf("over %d sessions the arms were %v; a constant arm is trivially the same every time",
			armSessions, counts)
	}
}

// TestH109_TheDerivationIsPinned holds the derivation still across releases.
// No session stores its arm yet, so a session that spans an upgrade is
// assigned again by the new binary, and a derivation that moved would move the
// session to the other side halfway through.
//
// The draws were computed with Python's hmac module, not with this package:
// HMAC-SHA256(key, "\x00arm\x00" + feature + "\x00" + session id), its first
// eight bytes big-endian over 2^64. Each is checked with one share just below
// it and one just above, which pins it to within 1e-9 rather than to one bit.
// The last two vectors would concatenate to the same bytes without the NUL
// between feature and session id.
//
// Break: the domain separator is dropped.
func TestH109_TheDerivationIsPinned(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	a := newArmAssigner(t, key)
	for _, v := range []struct {
		feature, session string
		bucket           int // the draw's basis point
	}{
		{"steer-recap", "3f2b9c1e-8a4d-4f6b-9c2e-1d7a5b8e0f42", 7089},
		{"steer-recap", "00000000-0000-4000-8000-000000000000", 356},
		{"steer-nudge", "3f2b9c1e-8a4d-4f6b-9c2e-1d7a5b8e0f42", 6749},
		{"ab", "c-session", 7490},
		{"a", "bc-session", 6728},
	} {
		if got := mustAssign(t, a, v.feature, v.session, v.bucket); got != arm.Treatment {
			t.Errorf("(%q, %q) at %d bp: %q, want treatment (its draw is in bucket %d)",
				v.feature, v.session, v.bucket, got, v.bucket)
		}
		if got := mustAssign(t, a, v.feature, v.session, v.bucket+1); got != arm.Holdout {
			t.Errorf("(%q, %q) at %d bp: %q, want holdout (its draw is in bucket %d)",
				v.feature, v.session, v.bucket+1, got, v.bucket)
		}
	}
}

// TestH109_ADifferentInstallKeyReshufflesTheAssignment is the "cannot predict
// its arm without the key" half. An agent that knows this derivation and every
// input but the key can compute an arm under a key of its own, and the question
// is how often that agrees with the real one. At a half share, independent
// arms agree half the time -- what a coin gets -- so the agreement is held to
// the bound around a half.
//
// What this does not cover, and cannot: install.key sits in the store, mode
// 0600, owned by the user the agent runs as, and an agent that reads it
// computes its arm exactly.
//
// Break: the key is left out of the digest.
func TestH109_ADifferentInstallKeyReshufflesTheAssignment(t *testing.T) {
	ka, kb := armInstallKey(t), armInstallKey(t)
	if string(ka) == string(kb) {
		t.Fatal("premise: two fresh installs minted the same key")
	}
	a, b := newArmAssigner(t, ka), newArmAssigner(t, kb)

	agree := 0
	for _, id := range armSessionIDs(armSessions) {
		if mustAssign(t, a, "steer-key", id, 5000) == mustAssign(t, b, "steer-key", id, 5000) {
			agree++
		}
	}
	withinFiveSigma(t, "sessions in the same arm under two install keys", agree, armSessions, 0.5)
}

// TestH109_TwoFeaturesInATwoByTwoAreAssignedIndependently. Two features
// measured at once each need to see the other's arms in equal measure, or one
// feature's effect is read as the other's. Each cell of a half-by-half layout
// holds a quarter, to the bound at p = 1/4.
//
// And each factor lands where it would alone: adding a second feature to a
// running measurement must not move any session in the first. A feature named
// twice is refused, because its two "independent" draws would be one draw.
//
// Break: the feature name is left out of the digest, so every session's two
// arms are the same arm and two cells stay empty.
func TestH109_TwoFeaturesInATwoByTwoAreAssignedIndependently(t *testing.T) {
	a := newArmAssigner(t, armInstallKey(t))
	recap := arm.Factor{Feature: "steer-recap", HoldoutBP: 5000}
	nudge := arm.Factor{Feature: "steer-nudge", HoldoutBP: 5000}

	cells := map[[2]arm.Arm]int{}
	for _, id := range armSessionIDs(armSessions) {
		arms, err := a.Layout(id, recap, nudge)
		if err != nil {
			t.Fatalf("Layout(%q): %v", id, err)
		}
		if len(arms) != 2 {
			t.Fatalf("Layout(%q) returned %d arms for two factors", id, len(arms))
		}
		for i, f := range []arm.Factor{recap, nudge} {
			if alone := mustAssign(t, a, f.Feature, id, f.HoldoutBP); arms[i] != alone {
				t.Fatalf("session %s: %s is %q in the layout and %q alone", id, f.Feature, arms[i], alone)
			}
		}
		cells[[2]arm.Arm{arms[0], arms[1]}]++
	}
	for _, r := range []arm.Arm{arm.Treatment, arm.Holdout} {
		for _, n := range []arm.Arm{arm.Treatment, arm.Holdout} {
			withinFiveSigma(t, fmt.Sprintf("recap %s, nudge %s", r, n), cells[[2]arm.Arm{r, n}], armSessions, 0.25)
		}
	}

	if arms, err := a.Layout(armSessionIDs(1)[0], recap, recap); !errors.Is(err, arm.ErrDuplicateFeature) {
		t.Errorf("a layout naming %s twice returned %v, %v; want ErrDuplicateFeature", recap.Feature, arms, err)
	}
}

// TestH109_ASessionWithoutAnIdGetsNoArm. A payload with no session id cannot be
// told from any other payload with none, so a default arm would put all of them
// on one side, chosen by whatever made the id go missing. It gets an error and
// no arm, and so does an assigner with no key: there is no unkeyed fallback,
// because the unkeyed draw is the one an agent can compute for itself.
//
// Positive twin: the same call with an id does assign.
//
// Break: a session without an id is given an arm.
func TestH109_ASessionWithoutAnIdGetsNoArm(t *testing.T) {
	a := newArmAssigner(t, armInstallKey(t))
	mustAssign(t, a, "steer-noid", "a-session", 5000)

	if got, err := a.Assign("steer-noid", "", 5000); !errors.Is(err, arm.ErrNoSession) || got != "" {
		t.Errorf("Assign with no session id = %q, %v; want no arm and ErrNoSession", got, err)
	}
	if arms, err := a.Layout("", arm.Factor{Feature: "steer-noid", HoldoutBP: 5000}); !errors.Is(err, arm.ErrNoSession) || arms != nil {
		t.Errorf("Layout with no session id = %v, %v; want no arms and ErrNoSession", arms, err)
	}
	for _, key := range [][]byte{nil, {}} {
		if _, err := arm.New(key); !errors.Is(err, arm.ErrNoKey) {
			t.Errorf("arm.New(%#v) = %v; want ErrNoKey", key, err)
		}
	}
}

// TestH109_TheArmDependsOnNothingButTheKeyTheFeatureAndTheSession is the
// no-override half, checked where it can be: in what the package imports. An
// environment override, an opt-out file, a draw seeded from the clock or from
// math/rand, a lookup of anything the session can set -- each needs an import
// this list does not hold. The list is exhaustive on purpose, so that widening
// it is a decision a reviewer sees in this test's diff.
//
// Every non-test file is parsed, whatever its build constraint: `go list` would
// report only this platform's files, and an override in a _windows.go file
// would pass on Linux.
//
// Positive twin: the package was found and read, and it imports crypto/hmac.
//
// Break: an environment variable overrides the arm.
func TestH109_TheArmDependsOnNothingButTheKeyTheFeatureAndTheSession(t *testing.T) {
	allowed := map[string]bool{
		"crypto/hmac":     true,
		"crypto/sha256":   true,
		"encoding/binary": true,
		"errors":          true,
		"strings":         true,
		// The draw's basis point: a 128-bit product, arithmetic only.
		"math/bits": true,
	}
	files, err := filepath.Glob(filepath.Join(moduleRoot, "internal", "arm", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	parsed := 0
	imported := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", f, err)
		}
		parsed++
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("%s: import %s: %v", filepath.Base(f), imp.Path.Value, err)
			}
			imported[path] = true
			if !allowed[path] {
				t.Errorf("internal/arm/%s imports %s; the arm may depend on the key, the feature, "+
					"the session id and the share, and on nothing an import brings in", filepath.Base(f), path)
			}
		}
	}
	if parsed == 0 {
		t.Fatalf("no source file found under internal/arm; this read the wrong place")
	}
	if !imported["crypto/hmac"] {
		t.Errorf("internal/arm does not import crypto/hmac, so its draw is not keyed")
	}
}
