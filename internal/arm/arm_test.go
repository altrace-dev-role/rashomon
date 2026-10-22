package arm

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"testing"
)

func testAssigner(t *testing.T, key []byte) Assigner {
	t.Helper()
	a, err := New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func testKey(seed byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

func testIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("session-%d", i)
	}
	return ids
}

// A share Assign refuses is one that would measure nothing, or one that is not
// a share at all.
func TestAssign_RefusesAShareThatMeasuresNothing(t *testing.T) {
	a := testAssigner(t, testKey(1))
	for _, share := range []int{0, BasisPoints, -1, BasisPoints + 1} {
		if got, err := a.Assign("f", "s", share); !errors.Is(err, ErrShare) || got != "" {
			t.Errorf("Assign at share %v = %q, %v; want no arm and ErrShare", share, got, err)
		}
	}
}

// bucket spans 0 to 9999 exactly: the first draw of basis point k is
// ceil(k * 2^64 / BasisPoints), and the draw before it is in k-1. The ends
// do not wrap: draw 0 is in bucket 0 and the largest draw in 9999.
func TestBucket_SpansTheBasisPointsExactly(t *testing.T) {
	space := new(big.Int).Lsh(big.NewInt(1), 64)
	for _, k := range []int64{1, 2, 5000, 9998, 9999} {
		first := new(big.Int).Mul(big.NewInt(k), space)
		first.Add(first, big.NewInt(BasisPoints-1)).Quo(first, big.NewInt(BasisPoints))
		d := first.Uint64()
		if got := bucket(d); got != uint64(k) {
			t.Errorf("bucket(first draw of %d) = %d", k, got)
		}
		if got := bucket(d - 1); got != uint64(k-1) {
			t.Errorf("bucket(the draw before %d's first) = %d, want %d", k, got, k-1)
		}
	}
	if bucket(0) != 0 || bucket(math.MaxUint64) != BasisPoints-1 {
		t.Errorf("bucket(0) = %d, bucket(max) = %d; want 0 and %d", bucket(0), bucket(math.MaxUint64), BasisPoints-1)
	}
}

// A feature name with a NUL in it could reproduce another feature's draw input
// with a different session id, so it is refused, as is an empty one.
func TestAssign_RefusesAFeatureNameThatCouldCollide(t *testing.T) {
	a := testAssigner(t, testKey(3))
	for _, feature := range []string{"", "a\x00b", "\x00"} {
		if got, err := a.Assign(feature, "s", 5000); !errors.Is(err, ErrFeature) || got != "" {
			t.Errorf("Assign(%q) = %q, %v; want no arm and ErrFeature", feature, got, err)
		}
	}
	if _, err := a.Assign("steer-recap", "s", 5000); err != nil {
		t.Errorf("an ordinary feature name was refused: %v", err)
	}
}

// The zero Assigner has no key, and draws nothing rather than an unkeyed arm.
func TestAssign_TheZeroAssignerRefuses(t *testing.T) {
	var a Assigner
	if got, err := a.Assign("f", "s", 5000); !errors.Is(err, ErrNoKey) || got != "" {
		t.Errorf("zero Assigner = %q, %v; want no arm and ErrNoKey", got, err)
	}
}

// New copies its key. A caller that reused the slice would otherwise move
// sessions between arms in an assigner it had already handed out.
//
// Positive twin: the overwritten key does draw differently, so the sameness
// below is the copy's doing and not a draw that ignores its key.
func TestNew_CopiesTheKey(t *testing.T) {
	key := testKey(4)
	a := testAssigner(t, key)
	ids := testIDs(500)
	before := make([]Arm, len(ids))
	for i, id := range ids {
		before[i], _ = a.Assign("f", id, 5000)
	}
	copy(key, testKey(99))
	other := testAssigner(t, key)
	moved := 0
	for i, id := range ids {
		if got, _ := a.Assign("f", id, 5000); got != before[i] {
			t.Fatalf("session %s moved from %q to %q when the caller's key slice changed", id, before[i], got)
		}
		if got, _ := other.Assign("f", id, 5000); got != before[i] {
			moved++
		}
	}
	if moved == 0 {
		t.Error("a different key drew the same arm for every session; the copy proves nothing")
	}
}

// A layout passes Assign's refusals through, and an empty layout assigns
// nothing.
func TestLayout_PassesRefusalsThrough(t *testing.T) {
	a := testAssigner(t, testKey(5))
	if arms, err := a.Layout("s", Factor{"f", 5000}, Factor{"g", BasisPoints}); !errors.Is(err, ErrShare) || arms != nil {
		t.Errorf("Layout with a bad share = %v, %v; want no arms and ErrShare", arms, err)
	}
	if arms, err := a.Layout("s"); err != nil || len(arms) != 0 {
		t.Errorf("Layout with no factors = %v, %v; want no arms and no error", arms, err)
	}
}
