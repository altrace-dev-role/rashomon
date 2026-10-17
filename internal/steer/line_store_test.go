package steer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// appendAndRead writes lines to a fresh store's steer log for session and
// reads them back as Lines, strictly: a key Line does not carry fails it.
func appendAndRead(t *testing.T, session string, lines ...*Line) ([]Line, error) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	batch := make([]store.SteerLine, 0, len(lines))
	for _, l := range lines {
		batch = append(batch, l)
	}
	appendErr := st.AppendSteer(session, func(io.ReaderAt) ([]store.SteerLine, error) { return batch, nil })
	raw, skipped, err := st.ReadSteer(session)
	if err != nil {
		t.Fatalf("ReadSteer: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("ReadSteer skipped %d lines of a clean log", skipped)
	}
	got := make([]Line, 0, len(raw))
	for _, b := range raw {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		var l Line
		if err := dec.Decode(&l); err != nil {
			t.Fatalf("a stored steer line does not decode as a Line: %v\n%s", err, b)
		}
		got = append(got, l)
	}
	return got, appendErr
}

// TestREC42f_StoredLineReadsBackAsALine: the line RECORD's writer stores is
// STEER's Line, key for key, and reads back equal to what was appended.
func TestREC42f_StoredLineReadsBackAsALine(t *testing.T) {
	prompt := "p1"
	want := Line{
		Type: LineType, V: LineVersion, ID: "0123456789abcdef0123456789abcdef", SessionID: "sess-1",
		PromptID: &prompt, Pattern: "regression-location", Level: "nudge", Channel: ChannelPostToolBatch,
		Event: string(EventPostToolBatch), Attendance: AttendanceAttended, Arm: ArmLive, AfterSeq: 57,
		EvidenceSeqs: []int64{37, 52}, Slots: map[string]json.RawMessage{"exit": json.RawMessage(`1`)},
		ModelText: "rashomon record: go test failed", UserText: "※ rashomon told the agent: \"rashomon record: go test failed\"",
		State: LineStateDelivered, SteerID: "0123456789abcdef0123456789abcdef", Mirror: MirrorSystemMessage,
	}
	l := want
	got, err := appendAndRead(t, "sess-1", &l)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("read %d lines, want 1", len(got))
	}
	a, err := json.Marshal(got[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("the stored line reads back as\n%s\nwant\n%s", a, b)
	}
}

// TestREC52f_SteerTextCapsAtNAndNPlusOne: model_text and user_text are each
// capped at 2,048 bytes (Technical spec 1.2.9). At the cap the text is
// stored whole; one byte over, CapText withholds it the store's way --
// empty, with its *_withheld key saying "oversize" (1.2.11) -- and keeps
// the other text and the rest of the line, and a line capped that way
// before it is appended lands as capped.
func TestREC52f_SteerTextCapsAtNAndNPlusOne(t *testing.T) {
	if store.MaxSteerTextBytes != 2048 {
		t.Fatalf("MaxSteerTextBytes is %d, want 2,048 (Technical spec 1.2.9)", store.MaxSteerTextBytes)
	}
	for _, field := range []string{"model_text", "user_text"} {
		for _, over := range []bool{false, true} {
			n := store.MaxSteerTextBytes
			if over {
				n++
			}
			l := Line{Type: LineType, V: LineVersion, ID: "id-" + field, ModelText: "model", UserText: "user"}
			text, other := &l.ModelText, func(l Line) string { return l.UserText }
			if field == "user_text" {
				text, other = &l.UserText, func(l Line) string { return l.ModelText }
			}
			*text = strings.Repeat("é", n/2) + strings.Repeat("x", n%2)
			keep := other(l)

			if withheld := l.CapText(); withheld != over {
				t.Errorf("%s at %d bytes: CapText reported %v, want %v", field, n, withheld, over)
			}
			got, err := appendAndRead(t, "sess-cap", &l)
			if err != nil {
				t.Fatalf("%s at %d bytes, capped first: AppendSteer returned %v, want nil", field, n, err)
			}
			if len(got) != 1 {
				t.Fatalf("%s at %d bytes: read %d lines, want 1", field, n, len(got))
			}
			g := got[0]
			gotText, withheld := g.ModelText, g.ModelTextWithheld
			if field == "user_text" {
				gotText, withheld = g.UserText, g.UserTextWithheld
			}
			switch {
			case !over && (len(gotText) != n || withheld != nil):
				t.Errorf("%s at the cap: %d bytes stored, withheld %v; want %d, nil", field, len(gotText), withheld, n)
			case over && (gotText != "" || withheld == nil || *withheld != "oversize"):
				t.Errorf("%s one over the cap: %d bytes stored, withheld %v; want 0, oversize", field, len(gotText), withheld)
			}
			if other(g) != keep || g.ID != "id-"+field {
				t.Errorf("%s at %d bytes: the rest of the line changed: id %q, other text %q", field, n, g.ID, other(g))
			}
		}
	}
}

// TestREC52g_AnUncappedTextIsNeverStoredAsDelivered: a delivered Line whose
// text reaches the writer still over its cap is not stored at all, nor is
// any line appended with it, and the append says why: the caller prints
// nothing, and the steer log lists exactly the notes that printed (STR-10).
// Either text alone is enough.
func TestREC52g_AnUncappedTextIsNeverStoredAsDelivered(t *testing.T) {
	for _, field := range []string{"model_text", "user_text"} {
		l := Line{Type: LineType, V: LineVersion, ID: "id-long", State: LineStateDelivered, ModelText: "model", UserText: "user"}
		text := &l.ModelText
		if field == "user_text" {
			text = &l.UserText
		}
		*text = strings.Repeat("x", store.MaxSteerTextBytes+1)
		short := Line{Type: LineType, V: LineVersion, ID: "id-short", State: LineStateDelivered, ModelText: "model", UserText: "user"}

		got, err := appendAndRead(t, "sess-uncapped", &short, &l)
		if !errors.Is(err, store.ErrSteerTextWithheld) {
			t.Errorf("%s over its cap, uncapped: AppendSteer returned %v, want ErrSteerTextWithheld", field, err)
		}
		if len(got) != 0 {
			t.Errorf("%s over its cap, uncapped: the log holds %d lines, want none", field, len(got))
		}
	}
}
