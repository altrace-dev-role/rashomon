package main

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/learn"
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
// newest checkpoint for the working directory's project (latestSession). No
// checkpoint for the project is an error saying so, never another project's
// session.
func lastSession() (string, error) {
	id, ok, err := latestSession()
	switch {
	case errors.Is(err, store.ErrNoStore):
		return "", errors.New("--session last: nothing has been recorded here")
	case err != nil:
		return "", err
	case !ok:
		return "", errors.New("--session last: no session of this project holds a checkpoint; name one with --session <id>")
	}
	return id, nil
}

// latestSession is the session that cut the newest checkpoint for the
// working directory's project (receipt.Latest, the same pick the commit
// trailer makes), with ok false when no session of the project holds one.
// No store here is store.ErrNoStore, so a caller can tell nothing recorded
// from no checkpoint: `--session last` refuses both, in words of its own,
// and a bare report prints today's text for both (card RPT-1).
func latestSession() (id string, ok bool, err error) {
	st, err := openStoreForRead()
	if err != nil {
		return "", false, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	record, found, err := receipt.Latest(st, baseline.Key(cwd))
	if err != nil {
		return "", false, err
	}
	var out [1]jsonscan.Span
	if found && jsonscan.Top(record, sessionKey(), out[:]) == nil && out[0].Kind == jsonscan.String {
		if id, err := jsonscan.DecodeString(record, out[0]); err == nil && id != "" {
			return id, true, nil
		}
	}
	return "", false, nil
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
// for: the one named, or with none named the one session the report covers
// (report.md :231), where the calls without a session id are not a session.
// A report of more sessions reads none: each is a replay, and the listing is
// what a person scrolls.
func attachImpact(st *store.Store, rep *report.Report, sessionID string) {
	if sessionID == "" {
		n := 0
		for _, s := range rep.Sessions {
			if s.SessionID != store.UnattributedSession {
				n, sessionID = n+1, s.SessionID
			}
		}
		if n != 1 || sessionID == "" {
			return
		}
	}
	for i := range rep.Sessions {
		if rep.Sessions[i].SessionID == sessionID {
			rep.Sessions[i].Autosteer = autosteerOf(st, sessionID)
		}
	}
}

// autosteerOf is sessionID's Autosteer: nil when the replay did not read
// (not read, which is not none), with a nil Stop when the session cut no
// checkpoint, and otherwise the replay's counts. The lessons are read through
// learn.OpenReadOnly, which never writes (learn.Open quarantines a state that
// does not check, and a report must not write the learning state): a ledger
// that does not check says so, and its lessons all read proposed.
func autosteerOf(st *store.Store, sessionID string) *report.Autosteer {
	r, err := replayLastStop(st, sessionID, nil)
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
	ls, _ := learn.OpenReadOnly(st.Root(), strings.TrimSuffix(baseline.FileName(r.Checkpoint.ProjectRoot), baseline.FileExt))
	im := recap.ImpactOf(v, lines, ls)
	if im.Lessons.Read {
		a.Lessons = &report.AutosteerLessons{Approved: im.Lessons.Approved, Proposed: im.Lessons.Proposed, Demoted: im.Lessons.Demoted, Unchecked: ls.Quarantined}
	}
	a.Stop = &report.AutosteerStop{Seq: v.Snap.Point.Seq, Row: string(v.Summary.Row), Sentence: recap.StateSentence(v)}
	a.Clause = recap.ClauseBody(im)
	// The replay reads no final message (MessageUnread), so v.Scan holds no
	// claim: the live Stop's verdicts are the checkpoint's.
	a.NotGreen = notGreenOf(r.Checkpoint, v.Done != nil)
	a.ClaimsOmitted = r.Checkpoint.ClaimsOmitted
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
	a.Mistakes = mistakesOf(recap.MistakesOf(v, lines, im, recap.ImpactShown(st.Root(), sessionID)))
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

// notGreenOf is the strict claims of cp whose verdict is not green, each named
// by class and verdict, with the seq each of its gates' rows in cp rests on
// and the count of its gates cp keeps no row for. Not green is either of two
// verdicts: a gate state other than green (recap.ClaimUnbacked, the done
// flag's claim term), or no_gate_known, a claim no gate checked, which the
// flag reads as backed (D79). So the list is not the flag's decision: the
// flag also reads the row, its sentence and the unmet expected kinds, it
// reads a claim no gate checked as backed, and no record keeps what it
// decided. earlier is whether the done flag at this Stop read the turns since
// the last edit (verify.StopView.Done) and not the Stop's own turn: one mark
// for the Stop, the same on every claim. No verdict depends on it, as a
// verdict is its gates' states at the Stop. Non-nil.
func notGreenOf(cp *verify.Checkpoint, earlier bool) []report.AutosteerClaim {
	out := []report.AutosteerClaim{}
	for _, c := range cp.Claims {
		unchecked := c.Strict && c.Verdict == verify.NoGateKnown
		if !recap.ClaimUnbacked(c) && !unchecked {
			continue
		}
		f := report.AutosteerClaim{Class: string(c.Class), Verdict: string(c.Verdict), Evidence: []int64{}, EarlierWindow: earlier}
		for _, id := range c.Gates {
			i := slices.IndexFunc(cp.Gates, func(g verify.CheckpointGate) bool { return g.Gate == string(id) })
			if i < 0 {
				f.NoGate++
				continue
			}
			at := cp.Gates[i].LastRunSeq
			if cp.Gates[i].State == verify.Green {
				at = cp.Gates[i].LastGreenSeq
			}
			if at != nil {
				f.Evidence = append(f.Evidence, *at)
			}
		}
		slices.Sort(f.Evidence)
		f.Evidence = slices.Compact(f.Evidence)
		out = append(out, f)
	}
	return out
}

// mistakesOf is m in the report's shape: every list non-null, and Later
// null, since nothing records a mistake found after the session.
func mistakesOf(m recap.Mistakes) *report.AutosteerMistakes {
	refs := func(fs []recap.FindingRef) []report.AutosteerMistake {
		out := []report.AutosteerMistake{}
		for _, f := range fs {
			out = append(out, report.AutosteerMistake{ID: f.ID, Seq: f.Seq, ResolvedBy: f.ResolvedBy})
		}
		return out
	}
	out := &report.AutosteerMistakes{Caught: refs(m.Caught), Reached: []report.AutosteerMistake{}, Missed: refs(m.Missed),
		Unsettled: refs(m.Unsettled), FixedUnprompted: refs(m.Fixed)}
	for _, f := range m.Reached {
		out.Reached = append(out.Reached, report.AutosteerMistake{ID: f.ID, Seq: f.Seq, ResolvedBy: f.ResolvedBy, Why: string(f.Why)})
	}
	return out
}

// sessionArg is --session's value with `last` resolved to a session id.
func sessionArg(v string) (string, error) {
	if v != lastSessionWord {
		return v, nil
	}
	return lastSession()
}
