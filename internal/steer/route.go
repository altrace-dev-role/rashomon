package steer

import (
	"errors"
	"io"
	"slices"
	"sync/atomic"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Context is Route's whole input besides the findings (tech 4.2.4; card 2.2
// R2): values the hook's caller already holds, or a constant until their
// reader lands. The router reads no file but the steer log, under its lock,
// and writes none but the log and the session's notice markers.
// The per-prompt counters (steer.budget, RES-19) are not a field: the router
// derives them from the log under the same lock that writes its lines.
type Context struct {
	// Event is the hook event the Output is printed on.
	Event Event
	// Client is the running Claude Code client and whether its in-band
	// mirror is verified (2.1): the gate a note's additionalContext passes.
	Client Client
	// Store is the run store whose steer log the lines go to; nil writes
	// nothing and prints nothing.
	Store *store.Store
	// SessionID names the run, and PromptID the prompt the budget counts
	// against: nil before the first UserPromptSubmit, where every note is a
	// flag (tech 4.2.5: a missing budget counts as spent).
	SessionID string
	PromptID  *string
	// AgentID is the subagent the call came from, "" for the main agent.
	AgentID string
	// Project is the project the line is filed under.
	Project string
	// InvocationID is the supervisor's id for this child (7.2.5 G5), ""
	// until the supervisor's pipe carries it.
	InvocationID string
	// HarnessID and Surface are the detected harness and its surface.
	HarnessID, Surface string
	// NowUnixMS is the decision time every line of the call carries.
	NowUnixMS int64
	// NewID mints a line id; nil mints 32 hex characters from crypto/rand.
	NewID func() string
	// Consent is the steer-consent marker's answer (D49, 2.2c); false
	// clamps every note to a flag.
	Consent bool
	// SteerOff is the user's steering switch, from the user's or the
	// project's key of config.json (store.Config.SteerOffFor), which the hook
	// reads. It is a clamp: no note reaches the agent, and what the note
	// would have said is shown to the user as a flag, logged steer_off.
	SteerOff bool
	// Arm is the lab arm: ArmLive on every install that is not a lab
	// install, and none is yet; "" reads as ArmLive.
	Arm Arm
	// Attendance is who is at the keyboard (tech 4.2.8); "" reads as
	// attended, the spec's default.
	Attendance Attendance
	// StopHookActive is the Stop payload's stop_hook_active: false until
	// the payload reader carries it.
	StopHookActive bool
	// Holdout is the session's draw into the field holdout (card 2.5, D109),
	// made by the hook under the store's key: the call's note goes to neither
	// side, and the session's first prints the holdout notice. The router
	// itself draws nothing.
	Holdout bool
}

// Route is the only path to Output (tech 4.2.4; tech 1141: "the only path
// to output"): it is where the step 8 priorities, tie-breaking by lower
// AfterSeq then input order, dedup by Key, and every budget/suppression
// decision happen.
//
// Every finding passes Finding.Check before the router sees it: Route is
// admit then route, and TestDEL22f pins that body, so P-30 fills in route
// and cannot drop the check by forgetting it (P07N-14).
func Route(ctx *Context, findings []Finding) Output {
	return route(ctx, admit(ctx, findings))
}

// dropped counts findings admit has refused since process start. Route's
// body is pinned to admit then route with nothing else in it
// (TestDEL22f), so a dropped finding is counted here rather than by
// changing what Route returns.
var dropped atomic.Int64

// DroppedFindings is the number of findings admit has refused since
// process start: a producer sending a finding outside the contract. P-30
// wires this into the router's own telemetry.
func DroppedFindings() int64 { return dropped.Load() }

// admit keeps the findings that pass Check, in order, and drops the rest:
// a finding outside the contract says nothing rather than reach a hook's
// stdout (fail closed). Logging a dropped finding is P-30's, with the
// router's own telemetry; DroppedFindings is the count it reads from.
func admit(ctx *Context, findings []Finding) []Finding {
	var event Event
	if ctx != nil {
		event = ctx.Event
	}
	kept := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if f.CheckOn(event) == nil {
			kept = append(kept, f)
		} else {
			dropped.Add(1)
		}
	}
	return kept
}

// lockTimeouts and appendFailures count the calls route printed nothing
// for because the steer log could not be locked, or could not take the
// lines: the record of a lock_timeout until the notices queue exists, since
// a lock_timeout line cannot be written without the lock (card 2.2 R1).
var lockTimeouts, appendFailures atomic.Int64

// LockTimeouts is the number of routed calls since process start that
// printed nothing because the steer lock was not taken in its budget.
func LockTimeouts() int64 { return lockTimeouts.Load() }

// AppendFailures is the number of routed calls since process start that
// printed nothing because the append under the lock failed.
func AppendFailures() int64 { return appendFailures.Load() }

// route is the imperative shell around decide (card 2.2 R1). Nothing to
// say -- no Context, no store, no finding above off -- writes nothing and
// prints nothing, and never takes the lock. Otherwise, under the steer
// log's lock, it counts the prompt's delivered lines (countBudget), decides
// (decide) and appends every line; a call that prints nothing else may print
// the session's once-per-session notice (notice), recorded on the line it
// stands in for. It returns the Output only when the append succeeded, so
// nothing prints that no line records (STR-8), and an append that failed
// returns the notice's marker for a later call. It never flushes: the caller
// flushes before it prints a non-empty Output (REC-34's group commit).
//
// The Client's Subagent is read from either source, the Client or a
// non-empty AgentID: one fact with two sources, and the stricter wins.
func route(ctx *Context, findings []Finding) Output {
	if ctx == nil || ctx.Store == nil || !slices.ContainsFunc(findings, func(f Finding) bool { return f.MaxLevel > LevelOff }) {
		return Output{}
	}
	c := ctx.Client
	c.Subagent = c.Subagent || ctx.AgentID != ""
	mint := ctx.NewID
	if mint == nil {
		mint = newID
	}
	var out Output
	var marker string
	err := ctx.Store.AppendSteer(ctx.SessionID, func(locked io.ReaderAt) ([]store.SteerLine, error) {
		have, err := countBudget(locked, ctx)
		if err != nil {
			return nil, err
		}
		o, lines := decide(ctx, c, findings, have, mint)
		if o == (Output{}) {
			o, marker = notice(ctx, c, lines)
		}
		batch := make([]store.SteerLine, len(lines))
		for i, l := range lines {
			batch[i] = l
		}
		out = o
		return batch, nil
	})
	if err != nil {
		if marker != "" {
			ctx.Store.ReturnNotice(ctx.SessionID, marker)
		}
		if errors.Is(err, store.ErrLockTimeout) {
			lockTimeouts.Add(1)
		} else {
			appendFailures.Add(1)
		}
		return Output{}
	}
	return out
}
