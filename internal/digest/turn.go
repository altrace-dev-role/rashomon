package digest

import (
	"math"
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// turnWindow is a turn's declarations and its span in RECORDED TIME.
//
// Declarations carry prompt_id directly, so grouping THEM needs no window --
// selectTurn does it with a plain map. The window exists for everything else
// in the store that a turn needs to account for but that carries no prompt_id
// of its own: a coverage record (RollupCoverage's doc), a dropped terminal
// (Declarations.Dropped's doc), and a gap. Time is the only signal those
// share with a turn, so it is the only join available, and it is documented
// as the approximation it is rather than presented as an exact attribution.
type turnWindow struct {
	promptID     string
	declarations []store.Declaration // this turn's own, in seq order
	// known is false when promptID names a turn with no declarations of its
	// own -- including the default "current turn" on a run with none at
	// all. There is then no start to derive a window FROM, and every
	// window-dependent check below falls back to the whole run rather than
	// asserting a boundary the records do not support -- see
	// buildTurnCoverage's doc on why that fallback is the safer direction for
	// H-85's zero-vs-unknown split.
	known   bool
	startMS int64
	endMS   int64 // exclusive; math.MaxInt64 for the open/current turn
}

func (w turnWindow) contains(ms int64) bool {
	return w.known && ms >= w.startMS && ms < w.endMS
}

// selectTurn groups a run's declarations by prompt_id -- and ONLY prompt_id,
// deliberately not (transcript_path, prompt_id) the way chains.go groups for
// the causal view. A subagent's declarations carry the parent's prompt_id
// under the subagent's OWN transcript path (record.go's TranscriptPath doc),
// so a (transcript, prompt) key would put them in a different bucket from
// the turn that spawned them -- silently, and by a confident ~half on the
// sessions that use subagents (account.go's SubagentSummary measurement).
// H-89 is the test that a (transcript, prompt) key fails.
//
// promptID empty means "the current turn": the one with the latest start.
// That is the only sense "current" can have from records alone -- there is
// no separate marker for "the turn in progress", and none is needed, since a
// turn only starts by a declaration naming a prompt_id nothing before it
// carried.
func selectTurn(run *store.Run, promptID string) turnWindow {
	type group struct {
		minSeq  int64
		startMS int64
		decls   []store.Declaration
	}
	byPrompt := map[string]*group{}
	var order []string
	for _, d := range run.Declarations {
		if d.PromptID == nil || *d.PromptID == "" {
			continue
		}
		id := *d.PromptID
		g, ok := byPrompt[id]
		if !ok {
			g = &group{minSeq: d.Seq, startMS: d.RecordedAtMS}
			byPrompt[id] = g
			order = append(order, id)
		}
		if d.Seq < g.minSeq {
			g.minSeq, g.startMS = d.Seq, d.RecordedAtMS
		}
		g.decls = append(g.decls, d)
	}
	sort.Slice(order, func(i, j int) bool { return byPrompt[order[i]].minSeq < byPrompt[order[j]].minSeq })

	if promptID == "" {
		if len(order) == 0 {
			return turnWindow{}
		}
		promptID = order[len(order)-1]
	}

	g, ok := byPrompt[promptID]
	if !ok {
		// Named explicitly but recorded nothing: no window to derive.
		return turnWindow{promptID: promptID}
	}

	endMS := int64(math.MaxInt64)
	for i, id := range order {
		if id != promptID {
			continue
		}
		if i+1 < len(order) {
			endMS = byPrompt[order[i+1]].startMS
		}
		break
	}

	sort.SliceStable(g.decls, func(i, j int) bool { return g.decls[i].Seq < g.decls[j].Seq })
	return turnWindow{
		promptID:     promptID,
		declarations: g.decls,
		known:        true,
		startMS:      g.startMS,
		endMS:        endMS,
	}
}

// turnToolUseIDs is the set of tool_use_ids this turn's own declarations
// name -- the key that ties executions and terminals to the turn without
// needing a window at all, since those records answer a specific declaration
// by id.
func turnToolUseIDs(w turnWindow) map[string]bool {
	ids := make(map[string]bool, len(w.declarations))
	for _, d := range w.declarations {
		ids[d.ToolUseID] = true
	}
	return ids
}

func filterExecutions(execs []store.Execution, ids map[string]bool) []store.Execution {
	out := make([]store.Execution, 0, len(execs))
	for _, x := range execs {
		if ids[x.ToolUseID] {
			out = append(out, x)
		}
	}
	return out
}
