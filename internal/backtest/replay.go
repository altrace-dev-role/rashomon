package backtest

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Fidelity is backtest/1's header note: what a replay can and cannot
// reproduce.
const Fidelity = "replayed from transcript copies through this build's hooks against a scratch store whose " +
	"coverage is verified, every call being replayed: the tool result a transcript keeps may differ from the " +
	"payload the hook saw live, a live run whose coverage was not verified judged no gate, a subagent's calls " +
	"are not replayed, a call whose result carries the tool_use_error tag is read as unhooked and skipped though " +
	"an error raised after PreToolUse carries the same tag, so such a call's fires are undercounted, and the " +
	"record scope is today's; so a fire can differ where those matter"

// Fire is one steer line: its pattern and the tool call it followed, the
// tool_use_id of the record at its after_seq. The call id is the same live
// and in a transcript; the line's key is not, for a learned gate's id
// digests its runner and location under the store's own key.
type Fire struct {
	Pattern string
	Call    string
}

// Replay runs calls through the PreToolUse and post hooks, each at the time
// the transcript gives it, against a scratch store at record scope scope,
// and returns the session's steer lines. The scratch store is removed before
// it returns. The holdout is off, so every session is treatment; a held-out
// call's line is written all the same, so fires do not depend on the draw.
func Replay(session, scope string, calls []Call) ([]Fire, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	root, err := os.MkdirTemp("", "rashomon-backtest-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root) //nolint:errcheck // a scratch store
	st, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	if err := store.WriteRecordScopeMarker(root, scope); err != nil {
		return nil, err
	}
	// Coverage asks whether the hooks saw every call; a replay runs every
	// call the transcript holds through both, so it holds by construction.
	// The scratch store's install is in no settings file, so the hooks' own
	// coverage records read hook_entry_absent, and without this one verify
	// would judge no gate at all.
	if err := st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: calls[0].At.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		return nil, err
	}
	sig := hook.WatchSignals()
	defer sig.Stop()
	for _, c := range calls {
		if sig.Delivered() {
			return nil, errors.New("backtest: interrupted")
		}
		if c.Unhooked {
			continue
		}
		pre, err := payload("PreToolUse", session, c, nil)
		if err != nil {
			return nil, err
		}
		h := hook.New(st, func() time.Time { return c.At })
		st.DeferDurability()
		h.Close(sig, h.CapturePayload(pre))
		if err := h.Flush(); err != nil || !c.Done {
			continue
		}
		event, more := "PostToolUse", map[string]any{"tool_response": json.RawMessage(nonEmpty(c.Response))}
		if c.IsError {
			event, more = "PostToolUseFailure", map[string]any{"error": c.Error, "is_interrupt": false,
				"duration_ms": c.DoneAt.Sub(c.At).Milliseconds()}
		}
		post, err := payload(event, session, c, more)
		if err != nil {
			return nil, err
		}
		p := hook.NewPost(st, func() time.Time { return c.DoneAt })
		st.DeferDurability()
		p.Close(sig, p.CapturePayload(post))
		if err := p.Flush(); err == nil {
			p.Steer(hook.SteerEnv{HoldoutOff: true, Now: c.DoneAt})
		}
	}
	return Fires(st, session)
}

// Fires reads a session's steer log, the live store's or a scratch one's,
// and names each line's call from the run's records.
func Fires(st *store.Store, session string) ([]Fire, error) {
	lines, _, err := st.ReadSteer(session)
	if err != nil || len(lines) == 0 {
		return nil, err
	}
	calls := map[int64]string{}
	run, err := st.ReadRun(session)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if run != nil {
		for _, d := range run.Declarations {
			calls[d.Seq] = d.ToolUseID
		}
		for _, e := range run.Executions {
			if e.Seq != nil {
				calls[*e.Seq] = e.ToolUseID
			}
		}
	}
	out := make([]Fire, 0, len(lines))
	for _, l := range lines {
		var line struct {
			Pattern  string `json:"pattern"`
			AfterSeq int64  `json:"after_seq"`
		}
		if json.Unmarshal(l, &line) == nil && line.Pattern != "" {
			out = append(out, Fire{Pattern: line.Pattern, Call: calls[line.AfterSeq]})
		}
	}
	return out, nil
}

// payload is the hook payload Claude Code would have sent for c.
func payload(event, session string, c Call, more map[string]any) (*hookio.Payload, error) {
	p := map[string]any{"hook_event_name": event, "session_id": session, "tool_name": c.Name,
		"tool_input": json.RawMessage(nonEmpty(c.Input)), "tool_use_id": c.ToolUseID, "cwd": c.Cwd,
		"permission_mode": c.Permission}
	if c.PromptID != "" {
		p["prompt_id"] = c.PromptID
	}
	for k, v := range more {
		p[k] = v
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return hookio.IntakeBytes(b)
}

func nonEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

// Matched counts the replay's fires that the live log also holds, each live
// fire answering at most one: a multiset intersection on (pattern, call).
func Matched(replay, live []Fire) int {
	left := map[Fire]int{}
	for _, f := range live {
		left[f]++
	}
	n := 0
	for _, f := range replay {
		if left[f] > 0 {
			left[f]--
			n++
		}
	}
	return n
}
