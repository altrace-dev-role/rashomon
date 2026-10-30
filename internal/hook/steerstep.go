package hook

import (
	"bytes"
	"path/filepath"
	"strings"
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
// HoldoutOff reads it. ProjectDir is CLAUDE_PROJECT_DIR and ManagedDir the
// managed-settings directory (settings.ManagedDir), DiscoverSpec's two
// inputs beside the call's cwd (card 3.1 (c)).
type SteerEnv struct {
	HarnessID, Surface string
	MirrorInBand       bool
	AIAgent            string
	CI                 bool
	HoldoutOff         bool
	Consent            func(root string) (int, error)
	Now                time.Time
	ProjectDir         string
	ManagedDir         string
}

// steps is the trigger (card 2.3 R3; card P5B, 5.7): an execution that
// landed in the ordered stream, at a failure event that carried a runner
// digest, the one call that can raise regression-location (a success event
// is outcome ok, D61), or at either post event when the call's input names
// .claude/rashomon, the one call that can raise integrity/store-touched
// (D-B7). Every other call costs these branches and storeNamed's scan.
func steps(event string, landedSeq int64, runner, inStore bool) bool {
	failure := event == FailureEvent
	return landedSeq > 0 && (failure && runner || inStore && (failure || event == string(steer.EventPostToolUse)))
}

// storeNamed reports a tool input whose raw bytes name .claude/rashomon, its
// slash plain or escaped as JSON may write it (D-B7), or name root, the
// store root (rootNamed, D173): a byte scan, with no decode. It is a
// superset of the calls that write there, and verify says which did. A Bash
// line that reaches the store by cd and a relative path is missed here; its
// store_touched entry still reaches the ledger and the Stop view.
func storeNamed(input []byte, root string) bool {
	if rootNamed(input, root) {
		return true
	}
	return bytes.Contains(input, []byte(".claude/rashomon")) || bytes.Contains(input, []byte(`.claude\/rashomon`))
}

// rootNamed reports a tool input whose raw bytes name root, its slashes plain
// or escaped. The root's last element is looked for first: an input that
// names the root holds it, and it is short, so a call that does not name it
// is one scan. The escaped form is built only for an input that escapes a
// slash, which neither encoding/json nor Claude Code's payloads do, so the
// common path allocates nothing (each []byte conversion is read-only and
// zero-copy). A root of "" is none.
func rootNamed(input []byte, root string) bool {
	if root == "" || !bytes.Contains(input, []byte(filepath.Base(root))) {
		return false
	}
	if bytes.Contains(input, []byte(root)) {
		return true
	}
	return bytes.Contains(input, []byte(`\/`)) && bytes.Contains(input, []byte(strings.ReplaceAll(root, "/", `\/`)))
}

// Steer runs the step after the record's Flush succeeded and returns the
// bytes to print, or nil. It returns no error: each exit before Route prints
// nothing (no trigger, an unreadable run), and every exit after Route
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

// judge is the step's compute before Route: the trigger, the run read once,
// the spec discovered from the payload's cwd (per trigger, not per call:
// D128), the run judged at the landed seq against that spec, and Route's
// context for the call. ok is false when the step does not run or the run
// could not be read. It writes nothing, so BenchmarkPostSteer's eval rows
// measure it apart from the append and the sync.
func (p *Post) judge(env SteerEnv) (steer.Context, verify.SteerView, bool) {
	if !steps(p.event, p.landedSeq, p.landedRunner, p.landedStore) {
		return steer.Context{}, verify.SteerView{}, false
	}
	run, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget)
	if err != nil {
		return steer.Context{}, verify.SteerView{}, false
	}
	root := p.st.Root()
	p.sp = DiscoverSpec(p.cwd, env.ProjectDir, env.ManagedDir, TrustRoot(run, root, p.landedSeq))
	pt := verify.Point{Seq: p.landedSeq, SessionID: p.sessionID, PromptID: nilIfEmpty(p.promptID), AgentID: nilIfEmpty(p.agentID),
		StoreRoot: root}
	view := verify.SteerAt(run, nil, p.sp, p.st.Key(), pt, env.Now, p.event, p.toolUseID)
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
		Holdout:    p.heldOut(env),
	}
}

// holdoutFeature is the arm feature the steer holdout draws under: one
// name, so every reader of a session's arm draws the same answer.
const holdoutFeature = "steer"

// holdoutBP is the field holdout's share in basis points with no spec: on
// by default at 1,000 (PRD PF-MS5, opt-out; D109 as amended). A discovered
// spec's holdout object sets the share instead (5.6, heldOut).
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
// arm's keyed draw for the session, the same on each of its calls, at the
// share the discovered spec's holdout object gives at env.Now (On: none when
// it is off or its window has closed), or at holdoutBP with no spec (5.6).
// The operator's off is above both. A draw arm refuses is treatment, and
// nothing records it: the store's key exists on every opened store.
func (p *Post) heldOut(env SteerEnv) bool {
	if env.HoldoutOff {
		return false
	}
	bp := holdoutBP
	if p.sp != nil {
		on, rate := p.sp.Holdout.On(env.Now)
		if !on {
			return false
		}
		bp = rate
	}
	a, err := arm.New(p.st.Key())
	if err != nil {
		return false
	}
	got, err := a.Assign(holdoutFeature, p.sessionID, bp)
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
