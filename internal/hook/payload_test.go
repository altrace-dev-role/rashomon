package hook

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

func TestAttribute_OverCapSessionIDGetsItsOwnBucket(t *testing.T) {
	if got := attribute(""); got != store.UnattributedSession {
		t.Errorf("attribute(\"\") = %q, want %q", got, store.UnattributedSession)
	}
	within := strings.Repeat("s", store.MaxIDBytes)
	if got := attribute(within); got != within {
		t.Errorf("attribute(within-cap) = %q, want it unchanged", got)
	}
	a := strings.Repeat("a", store.MaxIDBytes+1)
	b := strings.Repeat("b", store.MaxIDBytes+1)
	ga, gb := attribute(a), attribute(b)
	if ga == store.UnattributedSession || gb == store.UnattributedSession {
		t.Fatalf("attribute(a)=%q attribute(b)=%q: an over-cap id must not share the unattributed bucket", ga, gb)
	}
	if ga == gb {
		t.Fatalf("attribute(a) == attribute(b) == %q: two different over-cap ids must not collide", ga)
	}
}
