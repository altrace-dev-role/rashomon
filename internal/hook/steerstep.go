package hook

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The steer step (card 2.3): once a post's record is durable, the call is
// judged against the run and the note it raises, if any, is returned as the
// bytes the hook prints. The hook never writes stdout; cmd prints. Nothing
// in the step reads the environment, the clock or stdout: SteerEnv carries
// all three in, so the step is a function of the store and its arguments.

// SteerReadBudget is how long the step waits for the run's locks before it
// gives up and prints nothing. It is a lock-wait deadline, not a cap on the
// read: BenchmarkPostSteer measures what the read and the evaluation cost.
const SteerReadBudget = 10 * time.Millisecond

// SteerEnv is the step's port: what cmd reads from the process for it.
// Consent is the steer_disclosure marker's reader (store.SteerDisclosureMarker,
// D84); a nil reader reads as no consent. HoldoutOff is EnvHoldout as
// HoldoutOff reads it.
type SteerEnv struct {
	HarnessID, Surface string
	MirrorInBand       bool
	AIAgent            string
	CI                 bool
	HoldoutOff         bool
	Consent            func(root string) (int, error)
	Now                time.Time
}

// steps is the v0 trigger (card 2.3 R3): a failure event whose execution
// landed in the ordered stream and carried a runner digest. No other call can
// raise regression-location (a success event is outcome ok, D61), so every
// other call costs this one branch.
func steps(event string, landedSeq int64, runner bool) bool {
	return event == FailureEvent && landedSeq > 0 && runner
}

// Steer runs the step after the record's Flush succeeded and returns the
// bytes to print, or nil. It returns no error: each exit before Route prints
// nothing (no landed runner, an unreadable run), and every exit after Route
// has its line in the steer log.
func (p *Post) Steer(env SteerEnv) []byte {
	ctx, view, ok := p.judge(env)
	if !ok {
		return nil
	}
	out := steer.Route(&ctx, view.Findings)
	// The line is durable before its note prints (REC-34).
	if err := fault.Fail(fault.PointSteerFlush); err != nil {
		return nil
	}
	if err := p.st.Flush(); err != nil {
		return nil
	}
	// Route withheld any channel the client cannot carry, so an error here
	// is a contract break: print nothing.
	b, err := out.MarshalFor(p.event, ctx.Client)
	if err != nil {
		return nil
	}
	return b
}

// judge is the step's compute before Route: the trigger, the run read once
// and judged at the landed seq, and Route's context for the call. ok is false
// when the step does not run or the run could not be read. It writes nothing,
// so BenchmarkPostSteer's eval rows measure it apart from the append and the
// sync.
func (p *Post) judge(env SteerEnv) (steer.Context, verify.SteerView, bool) {
	if !steps(p.event, p.landedSeq, p.landedRunner) {
		return steer.Context{}, verify.SteerView{}, false
	}
	run, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget)
	if err != nil {
		return steer.Context{}, verify.SteerView{}, false
	}
	pt := verify.Point{Seq: p.landedSeq, SessionID: p.sessionID, PromptID: nilIfEmpty(p.promptID), AgentID: nilIfEmpty(p.agentID)}
	view := verify.SteerAt(run, nil, nil, pt, env.Now, p.event, p.toolUseID)
	return p.contextOf(env, view), view, true
}

// contextOf is Route's context for this call (card 2.3 R4.5). Attendance is
// unattended under CI or the dontAsk permission mode. Consent is the marker
// at or over the disclosure this build shows. The project is the view's
// root only: under record scope v4 the record holds none and the line
// follows the record, so the payload's cwd is never a fallback. Holdout is
// the session's draw (heldOut).
func (p *Post) contextOf(env SteerEnv, view verify.SteerView) steer.Context {
	version, source := steer.ClientVersion(env.AIAgent)
	attendance := steer.AttendanceAttended
	if env.CI || p.permissionMode == "dontAsk" {
		attendance = steer.AttendanceUnattended
	}
	return steer.Context{
		Event:      steer.Event(p.event),
		Client:     steer.Client{Version: version, Source: source, MirrorInBand: env.MirrorInBand, Subagent: p.agentID != ""},
		Store:      p.st,
		SessionID:  p.sessionID,
		PromptID:   nilIfEmpty(p.promptID),
		AgentID:    p.agentID,
		Project:    view.ProjectRoot,
		HarnessID:  env.HarnessID,
		Surface:    env.Surface,
		NowUnixMS:  env.Now.UnixMilli(),
		Consent:    consented(env.Consent, p.st.Root()),
		Arm:        steer.ArmLive,
		Attendance: attendance,
		Holdout:    p.heldOut(env.HoldoutOff),
	}
}

// holdoutFeature is the arm feature the steer holdout draws under: one
// name, so every reader of a session's arm draws the same answer.
const holdoutFeature = "steer"

// holdoutBP is the field holdout's share in basis points: on by default at
// 1,000 (PRD PF-MS5, opt-out; D109 as amended). 5.6 adds the off switch and
// the rate, the spec file's holdout object, when 3.1's discovery gives the
// hook its one spec read.
const holdoutBP = 1000

// EnvHoldout names the operator's switch for the field holdout: "off" skips
// the draw, and every session is treatment. The project file's holdout object
// (5.6, with 3.1 PR 2's discovery) is the configured form (PRD PF-MS5); this
// is the operator's override, kept after 5.6 as the organisation's off until
// the organisation file PF-MS5 names exists.
const EnvHoldout = "RASHOMON_HOLDOUT"

// HoldoutOff reads EnvHoldout's value: true for "off" exactly. Any other
// value, "OFF" and "0" among them, is ignored, with no error and no log.
func HoldoutOff(v string) bool {
	return v == "off"
}

// heldOut is the session's draw into the field holdout (card 2.5, D109):
// arm's keyed draw for the session at holdoutBP, the same on each of its
// calls, unless the operator's off is set. A draw arm refuses is treatment,
// and nothing records it: the store's key exists on every opened store.
func (p *Post) heldOut(off bool) bool {
	if off {
		return false
	}
	a, err := arm.New(p.st.Key())
	if err != nil {
		return false
	}
	got, err := a.Assign(holdoutFeature, p.sessionID, holdoutBP)
	return err == nil && got == arm.Holdout
}

// consented reads the marker; no reader, a read error or a value below
// steer.DisclosureVersion is no consent, which Route clamps to a flag.
func consented(read func(root string) (int, error), root string) bool {
	if read == nil {
		return false
	}
	n, err := read(root)
	return err == nil && n >= steer.DisclosureVersion
}
