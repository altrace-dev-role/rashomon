//go:build rashomonfault

package steer

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/fault"
)

// str8BarrierFault is STR-8's fault case: on the hook's deferred store, the
// barrier after the append fails, the lines may be in the log but are not
// ordered, and the call prints nothing and counts an append failure. The
// count is large so a repeated run (-count) still selects the failure.
func str8BarrierFault(t *testing.T) {
	ctx := routerContext(t)
	ctx.Store.DeferDurability()
	t.Setenv(fault.EnvVar, fault.PointStoreBarrier+":"+fault.FailN+"=1000000")
	before := AppendFailures()
	if out := Route(ctx, []Finding{regressionNote(t, "go test", 5, "a", LevelNudge)}); out != (Output{}) {
		t.Errorf("a failed barrier printed %+v", out)
	}
	if got := AppendFailures() - before; got != 1 {
		t.Errorf("AppendFailures rose by %d, want 1", got)
	}
}
