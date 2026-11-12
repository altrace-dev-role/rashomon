package session

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestEvent_MergeOrder (W-M4.2): a session's records and steer lines merge
// by (seq, record before line, at_unix_ms, id). A line's seq is its
// after_seq, the last record seq its decision saw, so it sorts after that
// record and before the next one, whatever the two clocks say.
func TestEvent_MergeOrder(t *testing.T) {
	want := []Event{
		{Seq: 0, Steer: true, AtUnixMS: 900, ID: "a0", Type: "steer"}, // the first prompt's line: no record yet
		{Seq: 1, Type: "declaration"},
		{Seq: 2, Type: "terminal"},
		{Seq: 2, Steer: true, AtUnixMS: 100, ID: "z9", Type: "steer"}, // an earlier clock than record 2's, still after it
		{Seq: 2, Steer: true, AtUnixMS: 200, ID: "a1", Type: "steer"},
		{Seq: 2, Steer: true, AtUnixMS: 200, ID: "b1", Type: "steer"},
		{Seq: 5, Type: "verify"},
		{Seq: 5, Steer: true, AtUnixMS: 50, ID: "c5", Type: "steer"},
		{Seq: 6, Type: "declaration"},
	}
	for _, e := range want {
		if e.Before(e) {
			t.Fatalf("%+v sorts before itself", e)
		}
	}
	r := rand.New(rand.NewSource(1))
	for round := 0; round < 50; round++ {
		got := append([]Event{}, want...)
		r.Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sort.Slice(got, func(i, j int) bool { return got[i].Before(got[j]) })
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("round %d: merged order\n got %+v\nwant %+v", round, got, want)
		}
	}
}

// TestEvent_Decode (W-M4.2): a record line decodes to its seq and type, a
// steer line to its after_seq, clock and id. A steer_evidence line has no
// after_seq and feeds no frontier field: not an event and not an error. A
// line the merge cannot place is an error.
func TestEvent_Decode(t *testing.T) {
	rec := `{"type":"declaration","schema_version":10,"seq":7,"recorded_at_unix_ms":1793897985945,"tool_use_id":"toolu_1","session_id":"s1","tool_name":"Read"}`
	ev, ok, err := RecordEvent([]byte(rec))
	if err != nil || !ok || ev != (Event{Seq: 7, Type: "declaration"}) {
		t.Errorf("record: %+v, %v, %v", ev, ok, err)
	}
	line := `{"type":"steer","v":1,"id":"0a6742a0","at_unix_ms":1793897981372,"session_id":"s1","kind":"unit","key":"example.com/m/internal/a@0a1b2c3d","after_seq":7,"model_text":"some words","user_text":"more words","state":"delivered"}`
	ev, ok, err = SteerEvent([]byte(line))
	if err != nil || !ok || ev != (Event{Seq: 7, Steer: true, AtUnixMS: 1793897981372, ID: "0a6742a0", Type: "steer", State: "delivered", Key: "example.com/m/internal/a@0a1b2c3d", Kind: "unit"}) {
		t.Errorf("steer line: %+v, %v, %v", ev, ok, err)
	}
	evidence := `{"type":"steer_evidence","v":1,"at_unix_ms":1793897981400,"session_id":"s1","steer_id":"0a6742a0","event":"UserPromptSubmit"}`
	if ev, ok, err = SteerEvent([]byte(evidence)); err != nil || ok {
		t.Errorf("steer_evidence: %+v, %v, %v; want not an event, no error", ev, ok, err)
	}
	for name, c := range map[string]struct {
		steer bool
		line  string
	}{
		"record without seq":      {false, `{"type":"declaration","schema_version":10}`},
		"record without type":     {false, `{"seq":3}`},
		"record not json":         {false, `{"type":"declaration","seq":`},
		"steer without after_seq": {true, `{"type":"steer","id":"x","at_unix_ms":5}`},
		"steer without id":        {true, `{"type":"steer","at_unix_ms":5,"after_seq":3}`},
		"steer not json":          {true, `{"type":"steer",`},
	} {
		dec := RecordEvent
		if c.steer {
			dec = SteerEvent
		}
		if ev, ok, err := dec([]byte(c.line)); err == nil {
			t.Errorf("%s: %+v, %v, no error", name, ev, ok)
		}
	}
}

// TestEvent_HoldsNoWords (W-M4.2, AC-M4.1): an event keeps the order key and
// the fields a rule reads, never a line's words (model_text, user_text). A
// string field not named here fails by name.
func TestEvent_HoldsNoWords(t *testing.T) {
	named := map[string]string{
		"ID":    "steer line id",
		"Type":  "record type or steer.LineType",
		"State": "a closed steer.LineState name",
		"Key":   "a finding's dedupe key: a unit id and a digest",
		"Kind":  "a closed steer.Kind name, or empty",
	}
	var got []string
	stringLeaves(reflect.TypeOf(Event{}), "", &got)
	seen := map[string]bool{}
	for _, p := range got {
		seen[p] = true
		if _, ok := named[p]; !ok {
			t.Errorf("Event.%s: a string not named as a reference; name it or keep words out", p)
		}
	}
	for p := range named {
		if !seen[p] {
			t.Errorf("Event.%s: named but gone", p)
		}
	}
}

// TestStep_Delivered (W-M4.2): a steer line in state delivered becomes one
// LineRef at its after_seq with its id, key and kind. A line in any other
// state and a record leave Delivered as it was.
func TestStep_Delivered(t *testing.T) {
	f := newFrontier("s1")
	Step(&f, Event{Seq: 1, Type: "declaration"})
	Step(&f, Event{Seq: 1, Steer: true, AtUnixMS: 5, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	for i, st := range steer.AllLineStates() {
		if st != steer.LineStateDelivered {
			Step(&f, Event{Seq: 2, Steer: true, AtUnixMS: int64(10 + i), ID: "n-" + string(st), Type: "steer", State: string(st), Key: "kn"})
		}
	}
	Step(&f, Event{Seq: 3, Steer: true, AtUnixMS: 30, ID: "d4", Type: "steer", State: "delivered", Key: "k4", Kind: "unit"})
	want := []LineRef{{ID: "d1", Key: "k1", Seq: 1}, {ID: "d4", Key: "k4", Kind: "unit", Seq: 3}}
	if !reflect.DeepEqual(f.Delivered, want) {
		t.Fatalf("Delivered\n got %+v\nwant %+v", f.Delivered, want)
	}
}

// TestStep_DeliveredOncePerID (W-M4.2): one LineRef per steer line id, the
// uniqueness canon's order rests on; a line seen again keeps its first ref.
func TestStep_DeliveredOncePerID(t *testing.T) {
	f := newFrontier("s1")
	Step(&f, Event{Seq: 4, Steer: true, AtUnixMS: 5, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	Step(&f, Event{Seq: 9, Steer: true, AtUnixMS: 9, ID: "d1", Type: "steer", State: "delivered", Key: "k1"})
	if want := []LineRef{{ID: "d1", Key: "k1", Seq: 4}}; !reflect.DeepEqual(f.Delivered, want) {
		t.Fatalf("Delivered\n got %+v\nwant %+v", f.Delivered, want)
	}
}
