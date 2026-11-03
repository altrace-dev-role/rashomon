package main

import (
	"strconv"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// owedNote is what the Stop view v leaves the session's next prompt (card
// GATE-INFER1, W5): the inferred gate's row absent or stale after a write
// it counts, in a turn that wrote, where the project declares the command
// (inf.Declared) or the index has seen a test run (inf.Learned). A Stop
// reaches the model only by continuing the turn, which the card rules out
// for absence (tosteer_card.go), so the next prompt carries the fact. A
// convention alone stays the user's line. The zero Owed is none.
func owedNote(v verify.StopView, inf *verify.InferredGate) recap.Owed {
	name := v.Programs[verify.InferredGateID]
	if inf == nil || !inf.Declared && !inf.Learned || v.Turn.Writes == 0 || name == "" {
		return recap.Owed{}
	}
	for _, g := range v.Snap.Gates {
		if g.Gate == verify.InferredGateID && (g.State == verify.Absent || g.State == verify.Stale) && g.WritesSinceGreen > 0 {
			return recap.Owed{Seq: v.Snap.Point.Seq, Stale: g.State == verify.Stale, Gate: name, From: inf.From}
		}
	}
	return recap.Owed{}
}

// owedFinding is the next prompt's note for o: claim-vs-record, absent or
// stale, at nudge, keyed by the Stop point's seq so each turn end gives it
// once (lines, the session's steer log, hold the keys given), naming the
// runner and the file, or the runner alone where the path slot refuses the
// file. None for no owed note, one past afterSeq, or a name the gate slot
// refuses.
func owedFinding(o recap.Owed, afterSeq int64, lines [][]byte) (steer.Finding, bool) {
	if o.Seq <= 0 || o.Seq > afterSeq {
		return steer.Finding{}, false
	}
	key := verify.InferredGateID + "@" + strconv.FormatInt(o.Seq, 10)
	if owedGiven(lines, key) {
		return steer.Finding{}, false
	}
	m, err := render.New(pattern.ClaimVsRecord, render.ClaimVsRecordInferredBinding{Gate: slot.Gate(o.Gate), Path: slot.Path(o.From)})
	if err != nil {
		m, err = render.New(pattern.ClaimVsRecord, render.ClaimVsRecordInferredUnnamedBinding{Gate: slot.Gate(o.Gate)})
	}
	if err != nil {
		return steer.Finding{}, false
	}
	f := steer.Finding{Pattern: pattern.ClaimVsRecord, Kind: "absent", Key: key, MaxLevel: steer.LevelNudge,
		StopForm: steer.StopFormNone, Message: m, AfterSeq: afterSeq}
	if o.Stale {
		f.Kind = "stale"
	}
	f.Priority, _ = pattern.Priority(pattern.ClaimVsRecord)
	return f, true
}

// owedGiven reports a claim-vs-record line under key in state delivered, or
// withheld by arm off, whose shadow decision gave it, as learn.Delivered
// reads a lesson's; a superseded line is not given. Lines are located with
// jsonscan, never decoded (RES-5).
func owedGiven(lines [][]byte, key string) bool {
	var sp [4]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, owedKeys(), sp[:]) == nil && scanText(b, sp[0]) == string(pattern.ClaimVsRecord) && scanText(b, sp[2]) == key &&
			(scanText(b, sp[1]) == string(steer.LineStateDelivered) || scanText(b, sp[3]) == string(steer.SuppressedByArmOff)) {
			return true
		}
	}
	return false
}

// owedKeys is the one KeySet owedGiven reads a line with, built on its first
// call and never at init (RES-2).
func owedKeys() *jsonscan.KeySet {
	owedKeysOnce.Do(func() { owedKeysOnce.v = jsonscan.NewKeySet("pattern", "state", "key", "suppressed_by") })
	return owedKeysOnce.v
}

var owedKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

// scanText is a string value's text, "" for any other kind or a bad escape.
func scanText(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}
