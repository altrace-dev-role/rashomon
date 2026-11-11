package graph

import (
	"errors"
	"fmt"
)

// CertainConf is the lowest confidence a certain edge carries.
const CertainConf = 0.8

// ErrUncertainBasis is CheckBasis's refusal: an obligation may not be
// derived from an edge that is not certain.
var ErrUncertainBasis = errors.New("graph: obligation basis is not certain")

// certain is the one writer rule (contracts-v2 section 3.3): asserted, at a
// commit, from a tier that is not inferred, at or over CertainConf, and on a
// predicate that states a fact rather than a possibility. The store applies
// it on every Assert; nothing else sets the field.
func certain(e Edge) bool {
	if e.Status != Asserted || e.Rev.Kind != RevKindCommit || e.Conf < CertainConf {
		return false
	}
	switch e.Tier {
	case Proven, Algorithmic, Declared, Observed:
	default:
		return false
	}
	switch e.Pred {
	case MayCall, MayResolveTo, CoChangedWith, DuplicatesValue:
		return false
	}
	return true
}

// EdgeRef names one edge as an obligation's basis.
type EdgeRef struct {
	Src  ID
	Pred Pred
	Dst  ID
}

// CheckBasis is the obligation writer's gate: every ref must be an asserted
// edge at rev with Certain set. The first failure names the edge; an
// obligation with a failing basis is never written.
func CheckBasis(s *Store, at Rev, refs []EdgeRef) error {
	if len(refs) == 0 {
		return fmt.Errorf("%w: no basis", ErrUncertainBasis)
	}
	for _, r := range refs {
		e, ok := s.Edge(r.Src, r.Pred, r.Dst, at)
		if !ok {
			return fmt.Errorf("%w: %s %s %s absent at %s", ErrUncertainBasis, r.Src, r.Pred, r.Dst, at)
		}
		if !e.Certain {
			return fmt.Errorf("%w: %s %s %s tier=%s conf=%.2f rev=%s", ErrUncertainBasis, r.Src, r.Pred, r.Dst, e.Tier, e.Conf, e.Rev)
		}
	}
	return nil
}
