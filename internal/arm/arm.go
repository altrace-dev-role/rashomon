// Package arm assigns a session to one arm of a holdout. It is the one
// randomiser every steering feature shares, so that "held out" means the same
// thing in each of them and two features can be measured at once.
//
// A STEERING FEATURE MEASURES ITSELF AGAINST THE SESSIONS IT LEFT ALONE, and the
// difference says something about the feature only if nothing about a session
// chose its side. So the arm is a function of four inputs and no others: the
// install key, the feature name, the session id and the holdout share. Not the
// clock, not an environment variable, not a file, not anything a session can
// set. There is deliberately no per-session opt-out: a holdout a session can
// leave is self-selected, and the difference it shows is between the sessions
// that left and the ones that stayed. A user who wants none of it turns the
// feature off, which turns off both arms together.
//
// KEYED, under the per-install key the store already holds (store.Key). The
// same session id draws the same arm in every process of one install, which is
// what lets every hook call in a session agree without storing anything, and
// draws independently on another install. An agent that knows this whole
// derivation but not the key does no better than a coin at predicting its arm.
//
// LIMITATION, stated rather than hidden: the key is install.key in the store,
// mode 0600 and owned by the user the agent runs as, so an agent that reads it
// can compute its arm. Keying keeps the arm out of reach of an agent that
// guesses. It is not secrecy against one that reads the store.
package arm

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
)

// Arm is the side of a holdout a session is on. The zero value is no arm,
// which is what a caller holds when Assign refused, and it is neither side.
type Arm string

const (
	// Treatment sessions get the feature.
	Treatment Arm = "treatment"
	// Holdout sessions do not, and are what the feature is measured against.
	Holdout Arm = "holdout"
)

var (
	// ErrNoKey: there is no install key to draw with.
	ErrNoKey = errors.New("arm: no install key to draw with")
	// ErrNoSession: there is no session id to assign.
	ErrNoSession = errors.New("arm: no session id to assign")
	// ErrFeature: the feature name is empty or contains a NUL.
	ErrFeature = errors.New("arm: a feature name must be non-empty and contain no NUL")
	// ErrShare: the holdout share is not strictly between 0 and 1.
	ErrShare = errors.New("arm: the holdout share must be strictly between 0 and 1")
	// ErrDuplicateFeature: a layout names one feature twice.
	ErrDuplicateFeature = errors.New("arm: a layout names the same feature twice")
)

// domain separates this draw from every other use of the install key. It
// starts with a NUL, as shape.CWDDigest's does, so no tool name can produce it,
// and it is none of "forget-host", "redact" or the session tag's. The arm is a
// bit of this digest, so if the digest were one a record or a shared report
// carries, the arm could be read off it by somebody who never held the key.
const domain = "\x00arm\x00"

// Assigner draws arms under one install's key.
type Assigner struct{ key []byte }

// New returns an assigner drawing under key, which is the store's install key.
// The key is copied, so a caller that reuses its slice cannot move sessions
// between arms after the fact.
//
// An empty key is refused rather than drawn with. An unkeyed draw is the one an
// agent can compute for itself, and a silent fall back to it is how a holdout
// becomes predictable without anybody noticing.
func New(key []byte) (Assigner, error) {
	if len(key) == 0 {
		return Assigner{}, ErrNoKey
	}
	return Assigner{key: append([]byte(nil), key...)}, nil
}

// Assign puts one session in one feature's arm: Holdout for the given share of
// sessions, Treatment for the rest.
//
// On an error there is no arm, and the caller treats the session as unassigned
// -- on neither side and counted in neither -- rather than defaulting it to
// one. A default puts every session that errs on the same side, and the reasons
// a call errs are not random: a payload with no session id comes from somewhere
// in particular.
//
// A share of 0 or 1 is refused. One has no comparison and the other no
// feature, so either is a configuration that measures nothing while looking
// like a measurement; the feature's own switch is the honest way to say on or
// off.
func (a Assigner) Assign(feature, sessionID string, holdout float64) (Arm, error) {
	if len(a.key) == 0 {
		return "", ErrNoKey
	}
	if feature == "" || strings.IndexByte(feature, 0) >= 0 {
		return "", ErrFeature
	}
	if sessionID == "" {
		return "", ErrNoSession
	}
	// Written so that NaN fails it: every comparison with NaN is false.
	if !(holdout > 0 && holdout < 1) {
		return "", ErrShare
	}
	if a.draw(feature, sessionID) < cut(holdout) {
		return Holdout, nil
	}
	return Treatment, nil
}

// draw is the first eight bytes of HMAC-SHA256(key, domain || feature || NUL ||
// session id), read big-endian as a uniform 64-bit integer. The NUL keeps
// ("ab", "c") and ("a", "bc") apart, which is why a feature may not contain
// one; the session id comes last, so it may.
//
// The digest never leaves this function. What a caller gets is one bit of it.
func (a Assigner) draw(feature, sessionID string) uint64 {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(domain))
	m.Write([]byte(feature))
	m.Write([]byte{0})
	m.Write([]byte(sessionID))
	return binary.BigEndian.Uint64(m.Sum(nil)[:8])
}

// cut is the draw below which a session is held out: that share of the 2^64
// possible draws. The largest float64 below 1 times 2^64 is 2^64 - 2^11, which
// a uint64 holds exactly, so no share Assign accepts can wrap. Shares resolve to
// 2^-64, far below anything a sample of sessions could tell apart.
func cut(holdout float64) uint64 {
	return uint64(holdout * (1 << 64))
}

// Factor is one feature in a layout and the share of sessions it holds out.
type Factor struct {
	Feature string
	Holdout float64
}

// Layout assigns one session in every factor at once and returns the arms in
// factor order; two factors make the two-by-two. Each feature is drawn under
// its own name, so the arms are independent and every pairing of them holds its
// share of sessions, which is what lets one feature's effect be read without
// the other's mixed in.
//
// Each arm is exactly the one Assign gives that feature alone, so adding a
// second feature to a running measurement moves no session in the first.
//
// A feature named twice is refused. Its two draws would be one draw, and a
// layout reporting four cells over what is really two would leave two of them
// empty for a reason nobody would see.
func (a Assigner) Layout(sessionID string, factors ...Factor) ([]Arm, error) {
	arms := make([]Arm, 0, len(factors))
	for i, f := range factors {
		for _, earlier := range factors[:i] {
			if earlier.Feature == f.Feature {
				return nil, ErrDuplicateFeature
			}
		}
		got, err := a.Assign(f.Feature, sessionID, f.Holdout)
		if err != nil {
			return nil, err
		}
		arms = append(arms, got)
	}
	return arms, nil
}
