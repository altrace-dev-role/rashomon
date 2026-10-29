package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/replay"
)

// replayedStopPayload is a Stop payload as the replay encodes one, for a message.
func replayedStopPayload(text string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"session_id": tSession, "transcript_path": tPath, "cwd": "/repo",
		"hook_event_name": "Stop", "last_assistant_message": text})
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// positional is text of at least n bytes in which every position is told apart
// from its neighbours, made of unit-led tokens so that a multi-byte unit
// leaves the head's and the tail's cuts at awkward byte offsets.
func positional(n int, unit string) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "%s%d ", unit, i)
	}
	return b.String()
}

// walkStops returns every Stop event a transcript's walk yields.
func walkStops(t *testing.T, d *tdoc, opt replay.Options) ([]replay.Event, replay.Summary) {
	t.Helper()
	if opt.TranscriptPath == "" {
		opt.TranscriptPath = tPath
	}
	var stops []replay.Event
	sum, err := replay.Walk(d.reader(), opt, func(e replay.Event) error {
		if e.Kind == replay.Stop {
			stops = append(stops, e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return stops, sum
}

func oneTurn(msg string) *tdoc {
	d := newTdoc()
	d.promptLine("p1")
	d.ends("m1", msg)
	return d
}

// TestREPLAY15_FinalMessageMemoryIsBoundedAcrossTheReplay holds the three
// parts of the ruling on final-message memory (revision 7):
//
//   - the replay keeps what the live recorder keeps, its head and its tail
//     (replay.MessageHeadBytes and MessageTailBytes are the recorder's
//     constants), and for a message longer than both feeds the head and the
//     tail of it, so that the recorder's own reader captures from the payload
//     exactly what it captures from the whole message, for texts of every
//     width of rune and of every escape, at and around each threshold;
//   - the text of the Stops not yet replayed is bounded by Options.FinalBudget
//     however many turns the file has, and is released as each Stop is
//     replayed;
//   - a Stop whose text the budget cannot hold is not fed, with a named
//     reason, so the claim evaluation that needed it is downgraded and never
//     run on a cut or empty message; and neither is a message the live
//     payload could not have carried.
func TestREPLAY15_FinalMessageMemoryIsBoundedAcrossTheReplay(t *testing.T) {
	if replay.MessageHeadBytes != messageHeadBytes || replay.MessageTailBytes != messageTailBytes {
		t.Fatalf("the replay keeps %d and %d bytes, the recorder %d and %d", replay.MessageHeadBytes, replay.MessageTailBytes, messageHeadBytes, messageTailBytes)
	}
	keep := messageHeadBytes + messageTailBytes

	t.Run("the recorder captures the same from the elided message", func(t *testing.T) {
		units := map[string]string{"ascii": "w", "two-byte": "é", "three-byte": "漢", "four-byte": "😀", "escapes": "\"\\\n\t\x01"}
		// Around each threshold for every unit; well past them for the one
		// that differ most (plain text).
		around := []int{100, messageHeadBytes + 1, keep, keep + 32, keep + 33, keep + 4096}
		for name, unit := range units {
			sizes := around
			if name == "ascii" {
				sizes = append(append([]int(nil), around...), 600<<10)
			}
			for _, n := range sizes {
				text := positional(n, unit)
				stops, _ := walkStops(t, oneTurn(text), replay.Options{})
				if len(stops) != 1 {
					t.Fatalf("%s %d: stops %+v", name, n, stops)
				}
				whole := replayedStopPayload(text)
				// A message the live payload could not carry (or comes within the
				// headroom of not carrying) is a Stop the live recorder never
				// saw.
				if len(whole) > hookio.MaxPayloadBytes-16<<10 {
					if stops[0].Supported() || stops[0].Unsupported[0] != replay.FinalTextTooLarge {
						t.Errorf("%s %d: a %d-byte live payload was fed", name, len(text), len(whole))
					}
					continue
				}
				if !stops[0].Supported() {
					t.Fatalf("%s %d: unsupported: %v", name, n, stops[0].Unsupported)
				}
				fmWhole, okW := readFinalMessage(bytes.NewReader(whole), true, true)
				fmKept, okK := readFinalMessage(bytes.NewReader(stops[0].Payload), true, true)
				if !okW || !okK {
					t.Fatalf("%s %d: the recorder's reader refused a payload (whole %v, replayed %v)", name, n, okW, okK)
				}
				if !reflect.DeepEqual(fmWhole, fmKept) {
					t.Errorf("%s, %d bytes: the recorder captures different text from the replayed payload (head %d/%d bytes, tail %d/%d bytes)",
						name, len(text), len(fmKept.Message), len(fmWhole.Message), len(fmKept.Tail), len(fmWhole.Tail))
				}
				if elided := len(text) > keep+2*32; stops[0].FinalElided != elided && (len(text) > keep+64 || len(text) <= keep) {
					t.Errorf("%s %d: FinalElided %v", name, len(text), stops[0].FinalElided)
				}
				// What is held and fed is the recorder's head and tail and the
				// margins, with escapes at most doubling them.
				if len(text) > keep+64 && len(stops[0].Payload) > 2*(keep+64)+1024 {
					t.Errorf("%s %d: the payload is %d bytes", name, len(text), len(stops[0].Payload))
				}
			}
		}
	})

	t.Run("the total held is bounded and released", func(t *testing.T) {
		// Thirty turns of 100 KiB: 3 MiB of final messages against a 1 MiB
		// budget.
		d := newTdoc()
		text := positional(100<<10, "t")
		for i := 0; i < 30; i++ {
			d.promptLine(fmt.Sprintf("p%d", i))
			d.ends(fmt.Sprintf("m%d", i), text)
		}
		const budget = 1 << 20
		stops, sum := walkStops(t, d, replay.Options{FinalBudget: budget})
		fed := 0
		for _, s := range stops {
			if s.Supported() {
				fed++
				if !bytes.Contains(s.Payload, []byte(`"last_assistant_message":"`+text)) {
					t.Fatalf("a Stop was fed a message that is not the whole of its text (%d bytes of payload)", len(s.Payload))
				}
				continue
			}
			if len(s.Unsupported) != 1 || s.Unsupported[0] != replay.FinalTextBudget || s.Payload != nil || s.Gap != replay.FinalTextBudget {
				t.Fatalf("a Stop the budget could not hold: %+v", s)
			}
		}
		if want := budget / len(text); fed != want || sum.FinalDropped != 30-want {
			t.Errorf("%d Stops fed and %d dropped, want %d and %d", fed, sum.FinalDropped, want, 30-want)
		}
		if sum.FinalHeldPeak > budget || sum.FinalHeldPeak < budget-len(text) {
			t.Errorf("the most held at once was %d bytes, against a budget of %d", sum.FinalHeldPeak, budget)
		}
		if sum.FinalHeldAtEnd != 0 {
			t.Errorf("%d bytes of final messages were still held at the end: Stops' text is not released as they are replayed", sum.FinalHeldAtEnd)
		}
		// Every Stop after the first dropped one is tainted.
		var first bool
		for _, s := range stops {
			if !s.Supported() {
				first = true
			}
			if first && s.Taint.IsZero() {
				t.Fatalf("the Stop at %+v follows a dropped Stop and is not tainted", s.Pos)
			}
		}
	})

	t.Run("the replay reports the Stop it could not feed as not evaluated", func(t *testing.T) {
		d := newTdoc()
		for i := 0; i < 4; i++ {
			d.promptLine(fmt.Sprintf("p%d", i))
			d.uses(fmt.Sprintf("u%d", i), tBash(fmt.Sprintf("t%d", i), "python3 -m unittest"))
			d.results("", tOK(fmt.Sprintf("t%d", i)))
			d.ends(fmt.Sprintf("m%d", i), "All the tests pass. "+positional(100<<10, "x"))
		}
		res, rep := replayDoc(t, d.reader(), replay.Options{FinalBudget: 250 << 10})
		if len(res.Stops) != 4 {
			t.Fatalf("%d Stops", len(res.Stops))
		}
		for i, s := range res.Stops {
			switch {
			case i < 2 && (!s.Evaluated || !s.Verified):
				t.Errorf("Stop %d (within the budget): evaluated %v, verified %v", i, s.Evaluated, s.Verified)
			case i >= 2 && (s.Evaluated || s.Verified || s.Unevaluated != string(replay.FinalTextBudget) || s.Verdict != nil):
				t.Errorf("Stop %d (over the budget): %+v", i, s)
			}
		}
		if got := kindCount(dumpStore(t, rep.root), "verify"); got != 2 {
			t.Errorf("the recorder wrote %d checkpoints: it was fed a Stop the replay had no text for", got)
		}
		if res.Summary.FinalDropped != 2 {
			t.Errorf("FinalDropped %d", res.Summary.FinalDropped)
		}
	})

	t.Run("a message the live payload could not have carried is not fed", func(t *testing.T) {
		if testing.Short() {
			t.Skip("builds 8 MiB messages")
		}
		// Text that is under the cap as written and over it once JSON-encoded
		// (every character an escape). Plain text of the cap's size is the
		// same case with a longer build.
		for name, text := range map[string]string{
			"escapes": positional(hookio.MaxPayloadBytes*5/8, "\"\\\n\t\x01"),
		} {
			stops, _ := walkStops(t, oneTurn(text), replay.Options{})
			if len(stops) != 1 || stops[0].Supported() || stops[0].Unsupported[0] != replay.FinalTextTooLarge {
				t.Fatalf("%s, %d bytes (%d encoded): %d stops, unsupported %v", name, len(text), len(replayedStopPayload(text)), len(stops), stops[0].Unsupported)
			}
		}
	})

	t.Run("a message longer than the recorder keeps is fed, flagged, and evaluated", func(t *testing.T) {
		d := newTdoc()
		d.promptLine("p1")
		d.uses("u1", tBash("t1", "python3 -m unittest"))
		d.results("", tOK("t1"))
		d.ends("m1", "All the tests pass. "+positional(1<<20, "y"))
		res, _ := replayDoc(t, d.reader(), replay.Options{})
		if len(res.Stops) != 1 || !res.Stops[0].Evaluated || !res.Stops[0].FinalElided || !res.Stops[0].Verified {
			t.Fatalf("%+v", res.Stops)
		}
		if c := res.Stops[0].Verdict.Snap.Claims; len(c) != 1 || c[0].Verdict != "green" {
			t.Errorf("the claim at the head of an elided message was not evaluated against the gate: %+v", c)
		}
	})
}
