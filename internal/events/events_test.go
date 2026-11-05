package events

import (
	"encoding/json"
	"testing"
)

// TestEvent_Vocabulary (W-M1.7, AD19): the ten kinds and the five boundary
// forms are closed lists; Known admits a listed kind with no form, a
// boundary only with a boundary form, a session start with any source
// word, and nothing else; the wire form carries the field names Part B6
// fixes and no agent's.
func TestEvent_Vocabulary(t *testing.T) {
	if len(Kinds()) != 10 || len(Boundaries()) != 5 {
		t.Fatalf("%d kinds, %d boundaries; want 10 and 5", len(Kinds()), len(Boundaries()))
	}
	for _, k := range Kinds() {
		if !Known(k, "") && k != Boundary {
			t.Errorf("Known(%s, \"\") = false", k)
		}
	}
	for _, b := range Boundaries() {
		if !Known(Boundary, b) || Known(Prompt, b) {
			t.Errorf("boundary form %q: Known(boundary)=%v, Known(prompt)=%v; want true, false", b, Known(Boundary, b), Known(Prompt, b))
		}
	}
	if Known(Boundary, "") || Known(Boundary, "startup") || !Known(SessionStart, "startup") || Known("session_started", "") {
		t.Errorf("Known admits a boundary with no form or an unknown kind")
	}
	e := Event{Kind: Boundary, Form: Compact, Seq: 7, Session: "s1", Agent: Agent{Name: "claude-code", Version: "2.1.280"},
		At: 1700000000000, Repo: "github.com/x/y", Payload: json.RawMessage(`{"a":1}`)}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"kind":"boundary","form":"compact","seq":7,"session":"s1","agent":{"name":"claude-code","version":"2.1.280"},"at":1700000000000,"repo":"github.com/x/y","payload":{"a":1}}`
	if string(b) != want {
		t.Errorf("wire = %s\nwant   %s", b, want)
	}
	var back Event
	if err := json.Unmarshal(b, &back); err != nil || back.Kind != Boundary || back.Form != Compact || back.Seq != 7 || string(back.Payload) != `{"a":1}` {
		t.Errorf("round trip = %+v, %v", back, err)
	}
	if b, _ := json.Marshal(Event{Kind: Prompt}); string(b) != `{"kind":"prompt","seq":0,"session":"","agent":{"name":"","version":""},"at":0,"repo":""}` {
		t.Errorf("zero form and payload printed: %s", b)
	}
}
