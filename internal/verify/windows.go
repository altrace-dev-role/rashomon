package verify

import (
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ScopeTurn is the scope of a turn row (W-M3.1): one prompt's calls, the
// window TurnWindow returns. Turn rows come after the session rows and change
// none of them.
const ScopeTurn = "turn"

// Scopes is the closed list of the scopes an obligation is owed in: the three
// in obligations.go (ScopeSession, ScopeSubtask, ScopeCommit) in their order,
// then turn. A row about one unit is a turn row with Unit set, never a scope
// of its own.
var Scopes = []string{ScopeSession, ScopeSubtask, ScopeCommit, ScopeTurn}

// Window is a span of record seqs, From and To inclusive, owed in Scope.
// From past To is the empty window.
type Window struct {
	Scope    string
	From, To int64
}

// Empty reports whether w holds no seq.
func (w Window) Empty() bool { return w.From > w.To }

// TurnWindow is the window of the point's own turn at seq at: To is at, and
// From is the first declaration at or before at that carries promptID, the
// turn's first call, as countTurn reads it into TurnCounts.FirstSeq. An empty
// promptID resolves as countTurn resolves it, to latestPrompt's id at at:
// the turn current at the point, never one whose first call is after it
// (card W-M3.1, Q4). A declaration whose prompt id is nil or "" is in no
// turn, so a run where no declaration carries a prompt id has the empty
// window, as countTurn's is, never the whole run. A turn with no call by at
// has the empty window, From at+1, and owes no turn row. A turn is a prompt
// id, whatever sent the prompt: a person, a peer session and a scheduled
// prompt each start one, since the record does not say which.
func TurnWindow(run *store.Run, promptID string, at int64) Window {
	if promptID == "" {
		promptID = latestPrompt(run.Declarations, at)
	}
	w := Window{Scope: ScopeTurn, From: at + 1, To: at}
	if promptID == "" {
		return w
	}
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if d.Seq < w.From && d.PromptID != nil && *d.PromptID == promptID {
			w.From = d.Seq
		}
	}
	return w
}

// TurnObligationID is the id of a turn row: ObligationID in the turn scope
// over a key of the window's From, then family with its length before it,
// then unit, "" for a row about no one unit. Two Stops in one turn share From
// and so their rows share ids; two turns' non-empty windows never share a
// From, a declaration carrying one prompt id, so their rows never do. An
// empty window's From, at+1, may be a later turn's first call, so an empty
// window owes no turn row (TestLedger_EmptyWindowAddsNoTurnRow). To is not
// in the key. The length keeps the key one to one: family "a1" with unit ""
// and family "a" with unit "1" are two keys, as in the conflict family's key.
func TurnObligationID(kind ObligationKind, family string, w Window, unit string) string {
	key := "t" + strconv.FormatInt(w.From, 10) + ":" + strconv.Itoa(len(family)) + ":" + family + unit
	return ObligationID(ScopeTurn, kind, key)
}

// UnitOf maps a project-relative path the session wrote to the global id of
// the unit that holds it, the form a conflict row's Unit has
// (graph.GlobalID: "g:", the repository id, "#", the local id), or "" for a
// path in no unit. The Stop's caller fills StopInput.UnitOf from the graph
// index, as it fills Conflicts, so this package reads no graph. A recorded
// ledger digests the id below record scope 5, as conflict_check rows do.
type UnitOf func(rel string) string

// MaxTurnUnitRows is the most unit rows one Stop adds (W-M3.1). Past it the
// ledger keeps the first MaxTurnUnitRows in gate order, then unit id
// ascending, and StopView.UnitRowsCut counts the rest, so the cut is the same
// on every replay and never silent.
const MaxTurnUnitRows = 32
