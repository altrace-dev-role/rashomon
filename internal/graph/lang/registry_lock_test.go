package lang

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRegistry_ConcurrentClaimsUnderOneRootRunTheStepOnce: the first Claim
// under a root is held inside the configuration step while a second Claim,
// of another path under the same root, is given time to reach it. The lock
// step.at holds across the step keeps the second waiting until the first
// has kept its result, so the step runs once and both Claims return the one
// resolver. With the lock held, the count cannot reach two whatever the
// scheduler does; the wait only gives a step with no lock the time to show
// itself. Break: take the lock out of step.at.
func TestRegistry_ConcurrentClaimsUnderOneRootRunTheStepOnce(t *testing.T) {
	var runs atomic.Int32
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var g Registry
	g.RegisterConfigured(fake{l: "x", ext: ".x"}, func(root string) (Resolver, error) {
		runs.Add(1)
		entered <- struct{}{}
		<-release
		return fake{l: "x", ext: ".x", at: root}, nil
	})

	type claim struct {
		r   Resolver
		ok  bool
		err error
	}
	got := make([]claim, 2)
	var wg sync.WaitGroup
	start := func(i int, rel string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, ok, err := g.Claim("/root", rel)
			got[i] = claim{r, ok, err}
		}()
	}
	start(0, "a.x")
	<-entered
	start(1, "b.x")
	select {
	case <-entered:
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	wg.Wait()

	if n := runs.Load(); n != 1 {
		t.Fatalf("two concurrent Claims under one root ran the step %d times, want 1: the step is not held under its lock", n)
	}
	want := fake{l: "x", ext: ".x", at: "/root"}
	for i, c := range got {
		if !c.ok || c.err != nil || c.r != want {
			t.Errorf("Claim %d = %v, %v, %v; want %v, true, nil", i, c.r, c.ok, c.err, want)
		}
	}
}
