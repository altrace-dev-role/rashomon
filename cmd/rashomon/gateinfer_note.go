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
// convention alone stays the user's line. The changed Go packages the
// Stop's platform read owes a go test ride with it, or alone where no gate
// is owed (PW1b-ON-230 step 3, ARCH-INLINE1 item 3): the Stop names them to
// the person, and the next prompt to the model. The zero Owed is none.
func owedNote(v verify.StopView, inf *verify.InferredGate) recap.Owed {
	o := owedGate(v, inf)
	if pkgs := recap.OwedPackages(v); len(pkgs) > 0 && v.Turn.Writes > 0 {
		o.Seq, o.Packages = v.Snap.Point.Seq, pkgs
	}
	return o
}

// owedGate is owedNote's inferred gate alone.
func owedGate(v verify.StopView, inf *verify.InferredGate) recap.Owed {
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

// owedPlatformKey keys a note that names owed packages and no gate, as
// verify.InferredGateID keys one that names a gate.
const owedPlatformKey = "platform"

// owedFinding is the next prompt's note for o: claim-vs-record, absent or
// stale, at nudge, keyed by the Stop point's seq so each turn end gives it
// once (lines, the session's steer log, hold the keys given), naming the
// runner and the file, or the runner alone where the path slot refuses the
// file, and then the owed packages where any is owed and the path list
// takes one; with no gate owed, the packages alone, absent. None for no
// owed note, one past afterSeq, or, with no gate owed, packages the slots
// refuse. A gate name the slot refuses still gives the finding, its Message
// zero, which Route logs as render_refused: the note was owed and the log
// says why it was not said, never a silent drop (GATE-INFER2). That line
// counts as given (owedGiven), so the trace is written once per turn end,
// as the note would be.
func owedFinding(o recap.Owed, afterSeq int64, lines [][]byte) (steer.Finding, bool) {
	if o.Seq <= 0 || o.Seq > afterSeq {
		return steer.Finding{}, false
	}
	keyed := verify.InferredGateID
	if o.Gate == "" {
		keyed = owedPlatformKey
	}
	key := keyed + "@" + strconv.FormatInt(o.Seq, 10)
	if owedGiven(lines, key) {
		return steer.Finding{}, false
	}
	m, err := owedMessage(o)
	if err != nil && o.Gate == "" {
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

// owedMessage is o's note, the first of its forms that renders: with a gate,
// the gate and its file, then the gate alone, each with the packages before
// without them; with no gate, the packages alone.
func owedMessage(o recap.Owed) (render.Message, error) {
	var pkgs *slot.PathList
	if len(o.Packages) > 0 {
		ps := make([]slot.Path, len(o.Packages))
		for i, d := range o.Packages {
			ps[i] = slot.Path(d)
		}
		l := slot.NewPathList(ps, 0)
		pkgs = &l
	}
	var forms []any
	if o.Gate != "" {
		g, f := slot.Gate(o.Gate), slot.Path(o.From)
		if pkgs != nil {
			forms = append(forms, render.ClaimVsRecordInferredPlatformBinding{Gate: g, Path: f, Packages: *pkgs},
				render.ClaimVsRecordInferredUnnamedPlatformBinding{Gate: g, Packages: *pkgs})
		}
		forms = append(forms, render.ClaimVsRecordInferredBinding{Gate: g, Path: f}, render.ClaimVsRecordInferredUnnamedBinding{Gate: g})
	} else if pkgs != nil {
		forms = append(forms, render.ClaimVsRecordPlatformBinding{Packages: *pkgs})
	}
	err := render.ErrNoTemplate
	for _, b := range forms {
		var m render.Message
		if m, err = render.New(pattern.ClaimVsRecord, b); err == nil {
			return m, nil
		}
	}
	return render.Message{}, err
}

// owedGiven reports a claim-vs-record line under key in state delivered, or
// withheld by arm off, whose shadow decision gave it, as learn.Delivered
// reads a lesson's, or refused at render, the trace owedFinding leaves for a
// name the gate slot refuses, which the next prompt would refuse again; a
// superseded line is not given. Lines are located with jsonscan, never
// decoded (RES-5).
func owedGiven(lines [][]byte, key string) bool {
	var sp [4]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, owedKeys(), sp[:]) == nil && scanText(b, sp[0]) == string(pattern.ClaimVsRecord) && scanText(b, sp[2]) == key &&
			(scanText(b, sp[1]) == string(steer.LineStateDelivered) || scanText(b, sp[3]) == string(steer.SuppressedByArmOff) ||
				scanText(b, sp[3]) == string(steer.SuppressedByRenderRefused)) {
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
