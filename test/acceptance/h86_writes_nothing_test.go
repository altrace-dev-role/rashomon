package acceptance

import (
	"reflect"
	"testing"
)

// H-86 -- the digest writes nothing.
//
// Snapshots the store dir AND baseline/ before and after. Break: build it on
// report.Build and baseline/ changes -- report.Build writes
// baseline/<project>.json (internal/report/destinations.go), which is why
// digest cannot simply call it.
//
// The fixture names a host the wire actually saw, exactly as H-28's healthy
// fixture does, and NOT as an incidental detail: report.Build's baseline write
// only happens on the path taken when the proxy observation is real
// (destinations.go returns early, before ever reaching it, when no proxy
// store was configured) -- a fixture that named no host and configured no
// proxy would pass this test whether or not digest called report.Build, which
// would make it decoration rather than a guard.
func TestH86_TheDigestWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))
	post := defaultPost()
	post.ToolInput = p.ToolInput
	e.mustPost(post.build(t))
	e.writeProxyStore(t, "pypi.org")
	e.probe("end", testSession)

	if _, ok := walkStore(t, e.home)["baseline"]; ok {
		t.Fatal("premise: baseline/ should not exist before anything has ever called `rashomon report`")
	}

	before := walkStore(t, e.home)
	e.digest("--session", testSession, "--prompt", p.PromptID)
	after := walkStore(t, e.home)

	if !reflect.DeepEqual(before, after) {
		t.Errorf("the store changed after `rashomon digest`:\nbefore: %+v\nafter:  %+v", before, after)
	}
	if _, ok := after["baseline"]; ok {
		t.Error("a baseline/ directory exists after `rashomon digest`; digest must never write it")
	}
}
