package main

import (
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// keyDoneUnverified is the finding's key: one per prompt, since the router
// counts a key once a prompt and recap.TakeUnverified lets one through per
// flagged Stop.
const keyDoneUnverified = "last-turn"

// doneFinding is the note that the previous turn's Stop raised the done flag
// (card c4, PF-PG9), a claim-vs-record finding of the frozen kind
// done-unverified, at the nudge level, with no stop form: the card's ceiling
// for the kind is a flag at the Stop and the fact at the next prompt
// (verify/tosteer_card.go). afterSeq is the last record seq the prompt saw.
func doneFinding(afterSeq int64) (steer.Finding, bool) {
	m, err := render.New(pattern.ClaimVsRecord, render.ClaimVsRecordDoneUnverifiedBinding{})
	if err != nil {
		return steer.Finding{}, false
	}
	prio, _ := pattern.Priority(pattern.ClaimVsRecord)
	return steer.Finding{Pattern: pattern.ClaimVsRecord, Kind: "done-unverified", Key: keyDoneUnverified, Priority: prio,
		MaxLevel: steer.LevelNudge, StopForm: steer.StopFormNone, Message: m, AfterSeq: afterSeq}, true
}

// withheld reports a context the router would clamp a note to a flag on:
// steering off or no consent. A config.json that does not read is off
// (hook.steerOff), so SteerOff covers it. A flag would print the note's text
// to the person as a second line about a turn whose flag they already saw, so
// the done note is not routed there (it is consumed, not logged); the arm, the
// channel and the budget stay the router's.
func withheld(ctx *steer.Context) bool {
	return ctx.SteerOff || !ctx.Consent
}
