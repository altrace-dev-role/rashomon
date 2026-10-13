package acceptance

import (
	"reflect"
	"testing"
)

// H-86 -- the digest writes nothing.
//
// Snapshots the store dir AND baseline/ before and after. Break: build it on
// gather.Build and baseline/ changes -- gather.Build writes
// baseline/<project>.json (internal/report/gather), which is why
// digest cannot simply call it.
//
// The fixture names a host in a tool call, exactly as H-28's healthy fixture
// does: gather.Build writes baseline/ for any session with a recorded cwd and
// session id, host or not, but naming one gives the file content, so a
// premature baseline/ this test would otherwise miss has something in it to
// diff.
func TestH86_TheDigestWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))
	post := defaultPost()
	post.ToolInput = p.ToolInput
	e.mustPost(post.build(t))
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
