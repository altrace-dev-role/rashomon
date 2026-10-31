package main

import (
	"errors"
	"os"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The session report's impact (card REPORT1a): the first screen and the
// autosteer block read recap.ImpactOf over the replay of the session's last
// Stop, the derivation the Stop line's clause reads, so the two cannot
// disagree about a count. It lives here and not in internal/report or its
// gather because recap imports report, and the replay's inputs (the spec,
// the tree) are this package's (laststop.go).

// lastSessionWord is what `--session` takes for the newest session of the
// working directory's project that holds a checkpoint.
const lastSessionWord = "last"

// lastSession is the session `--session last` names: the one that cut the
// newest checkpoint for the working directory's project (receipt.Latest, the
// same pick the commit trailer makes). No checkpoint for the project is an
// error saying so, never another project's session.
func lastSession() (string, error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return "", errors.New("--session last: nothing has been recorded here")
	}
	if err != nil {
		return "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	record, ok, err := receipt.Latest(st, baseline.Key(cwd))
	if err != nil {
		return "", err
	}
	var out [1]jsonscan.Span
	if ok && jsonscan.Top(record, sessionKey(), out[:]) == nil && out[0].Kind == jsonscan.String {
		if id, err := jsonscan.DecodeString(record, out[0]); err == nil && id != "" {
			return id, nil
		}
	}
	return "", errors.New("--session last: no session of this project holds a checkpoint; name one with --session <id>")
}

// sessionKey reads a checkpoint's session id by span, as the hook paths read
// a payload: no decode of the record (TestRES5_PayloadScannedOnce).
func sessionKey() *jsonscan.KeySet {
	sessionKeyOnce.Do(func() { sessionKeyOnce.v = jsonscan.NewKeySet("session_id") })
	return sessionKeyOnce.v
}

// sessionKeyOnce holds what sessionKey returns, built on its first call and never at
// init (RES-2).
var sessionKeyOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

// attachImpact fills the report's Autosteer for the one session it was built
// for. A report of every session reads none: each is a replay, and the
// listing is what a person scrolls.
func attachImpact(st *store.Store, rep *report.Report, sessionID string) {
	if sessionID == "" {
		return
	}
	for i := range rep.Sessions {
		if rep.Sessions[i].SessionID == sessionID {
			rep.Sessions[i].Autosteer = autosteerOf(st, sessionID)
		}
	}
}

// autosteerOf is sessionID's Autosteer: nil when the replay did not read
// (not read, which is not none), with a nil Stop when the session cut no
// checkpoint, and otherwise the replay's counts. The lessons are not read:
// the ledger's open can write (learn.Open quarantines), and a report that
// writes the learning state is not this card's to make.
func autosteerOf(st *store.Store, sessionID string) *report.Autosteer {
	r, err := replayLastStop(st, sessionID)
	if err != nil {
		return nil
	}
	a := &report.Autosteer{Gates: []report.AutosteerGate{}, Delivered: []report.AutosteerNote{},
		Findings: []report.AutosteerFinding{}, Checks: []report.AutosteerCheck{}, Unknown: []report.AutosteerUnknown{}}
	if r.Checkpoint == nil {
		return a
	}
	lines, _, _ := st.ReadSteer(sessionID)
	v := r.View
	im := recap.ImpactOf(v, lines, nil)
	a.Stop = &report.AutosteerStop{Seq: v.Snap.Point.Seq, Row: string(v.Summary.Row), Sentence: recap.StateSentence(v)}
	a.Clause = recap.ClauseBody(im)
	for _, g := range v.Snap.Gates {
		ag := report.AutosteerGate{Name: recap.GateName(v, g), Kind: g.Kind, State: string(g.State)}
		if g.LastRun != nil {
			ag.LastRunSeq = &g.LastRun.Seq
		}
		if g.LastGreen != nil {
			ag.LastGreenSeq = &g.LastGreen.Seq
		}
		a.Gates = append(a.Gates, ag)
	}
	for _, n := range im.Delivered {
		a.Delivered = append(a.Delivered, report.AutosteerNote{ID: n.ID, Pattern: n.Pattern, Title: recap.TitleOf(n.Pattern),
			Event: n.Event, PromptID: n.PromptID, Seq: n.Seq})
	}
	for _, f := range im.Findings {
		a.Findings = append(a.Findings, report.AutosteerFinding{ID: f.ID, Pattern: f.Pattern, Title: recap.TitleOf(f.Pattern),
			Gate: f.Gate, Seq: f.Seq, State: string(f.State), ResolvedBy: f.ResolvedBy})
	}
	for _, c := range im.Checks {
		reason := c.Reason
		if c.State == verify.ObligationUnknown && c.Kind == "judgment_disclosed" {
			reason = "the final message is not kept in the record"
		}
		a.Checks = append(a.Checks, report.AutosteerCheck{Kind: c.Kind, State: string(c.State), Gate: c.Gate, Seq: c.Seq, Reason: reason})
	}
	for _, u := range im.Unknown {
		a.Unknown = append(a.Unknown, report.AutosteerUnknown{Kind: u.Kind, Seq: u.Seq})
	}
	return a
}

// sessionArg is --session's value with `last` resolved to a session id.
func sessionArg(v string) (string, error) {
	if v != lastSessionWord {
		return v, nil
	}
	return lastSession()
}
