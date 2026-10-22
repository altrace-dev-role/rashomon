package report

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Codex's rollout (the file a Codex run's transcript_path names) writes one
// item_completed event per thing a turn finished. For a tool call, the item's
// id is exactly the tool_use_id Codex sent the hooks, so it joins to a record
// on equality; nothing else in the rollout does (the model's own tool call
// has a call_id of its own, because it is a script that may run several
// tools). Measured in phase 4: all eight calls' ids are item ids
// (test/fixtures/codex/phase4).
//
// Only four fields of an item are read: id, type, exit_code and status. The
// rest -- the command, its output, the diff, the message text -- is skipped by
// the decoder and never held.
const (
	codexItemCompleted = "item_completed"

	// Item types. A command and a file change fire the hooks with the item's
	// id, so an item of either type with no record is a call the store
	// missed. The two agent-tool types also carry a call's id, but
	// SubAgentActivity also names things that are not calls
	// ("subagent-completed-..."), so neither is held to the store.
	codexCommand    = "CommandExecution"
	codexFileChange = "FileChange"
)

// codexItem is what this build reads of one item_completed entry.
type codexItem struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	ExitCode json.RawMessage `json:"exit_code"`
	Status   json.RawMessage `json:"status"`
}

// codexRollout is a rollout as the report uses it: every item id, and the
// outcome of each item this build can judge.
type codexRollout struct {
	ids map[string]bool
	// calls is the ids of command and file-change items: tool calls that
	// fire the hooks, so each should have a record.
	calls map[string]bool
	// outcomes is store.ExecOK or store.ExecFailed for an item judged, and
	// absent for one that was not.
	outcomes map[string]string
	// codes is the exit code of each command item judged.
	codes map[string]int
}

// errRolloutBudget is readCodexRollout running out of time.
var errRolloutBudget = errors.New("report: rollout not read within its time budget")

// readCodexRollout reads the item_completed entries of the rollout at path.
// A line that is not JSON, or not an item_completed entry, is skipped. An id
// seen twice with two different outcomes is judged neither.
//
// A non-zero deadline bounds the read, and a read that reaches it fails whole:
// a call's own item is near the end of the file, and a rollout read in part
// would judge what it happened to reach, a conflicting later entry included.
func readCodexRollout(path string, deadline time.Time) (*codexRollout, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only

	r := &codexRollout{
		ids:      map[string]bool{},
		calls:    map[string]bool{},
		outcomes: map[string]string{},
		codes:    map[string]int{},
	}
	conflicted := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), maxLine)
	for sc.Scan() {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, errRolloutBudget
		}
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				Type string    `json:"type"`
				Item codexItem `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "event_msg" ||
			line.Payload.Type != codexItemCompleted || line.Payload.Item.ID == "" {
			continue
		}
		it := line.Payload.Item
		r.ids[it.ID] = true
		if it.Type == codexCommand || it.Type == codexFileChange {
			r.calls[it.ID] = true
		}
		outcome, code, ok := codexOutcome(it)
		if !ok || conflicted[it.ID] {
			continue
		}
		if prev, seen := r.outcomes[it.ID]; seen && (prev != outcome || r.codes[it.ID] != code) {
			conflicted[it.ID] = true
			delete(r.outcomes, it.ID)
			delete(r.codes, it.ID)
			continue
		}
		r.outcomes[it.ID] = outcome
		if it.Type == codexCommand {
			r.codes[it.ID] = code
		}
	}
	return r, sc.Err()
}

// codexOutcome judges one item, and reports false for one it cannot judge.
//
// A command is judged by its exit code alone: 0 is ok, any other integer is
// failed. A file change is judged by its status: "completed" is ok, "failed"
// is failed. Every other case -- another item type, a missing field, an exit
// code that is not an integer, a status this build does not know -- is not
// judged, and the execution stays unobserved. Decided by Rishi, for Karthik's
// review: the rule names what each value means, and guesses nothing it does
// not name.
func codexOutcome(it codexItem) (string, int, bool) {
	switch it.Type {
	case codexCommand:
		code, err := strconv.Atoi(string(bytes.TrimSpace(it.ExitCode)))
		if err != nil {
			return "", 0, false
		}
		if code == 0 {
			return store.ExecOK, 0, true
		}
		return store.ExecFailed, code, true
	case codexFileChange:
		var status string
		if json.Unmarshal(it.Status, &status) != nil {
			return "", 0, false
		}
		switch status {
		case "completed":
			return store.ExecOK, 0, true
		case "failed":
			return store.ExecFailed, 0, true
		}
	}
	return "", 0, false
}

// codexRollouts reads every rollout a run's declarations name, by path. A
// rollout that cannot be read, or not before deadline, is absent from the map.
func codexRollouts(run *store.Run, deadline time.Time) map[string]*codexRollout {
	out := map[string]*codexRollout{}
	tried := map[string]bool{}
	for _, d := range run.Declarations {
		if d.TranscriptPath == "" || tried[d.TranscriptPath] {
			continue
		}
		tried[d.TranscriptPath] = true
		if r, err := readCodexRollout(d.TranscriptPath, deadline); err == nil {
			out[d.TranscriptPath] = r
		}
	}
	return out
}

// CodexOutcomesFromRollouts reads a Codex run's rollouts, stopping at
// deadline, and sets the outcomes they judge (CodexOutcomesAsRan). It is the
// turn digest's way in: the digest runs inside recap's five-second hook
// timeout, and a rollout not read in time leaves its calls unobserved.
func CodexOutcomesFromRollouts(run *store.Run, deadline time.Time) {
	CodexOutcomesAsRan(run, codexRollouts(run, deadline))
}

// CodexOutcomesAsRan sets, in memory, the outcome of each Codex execution its
// rollout judged. The record on disk keeps its empty outcome: the hook could
// not observe one, and that stays true of the record. An execution joins an
// item only on an exact tool_use_id = item.id match, in the rollout its own
// declaration names. One already carrying an outcome is left alone.
func CodexOutcomesAsRan(run *store.Run, rollouts map[string]*codexRollout) {
	pathOf := map[string]string{}
	for _, d := range run.Declarations {
		pathOf[d.ToolUseID] = d.TranscriptPath
	}
	for i := range run.Executions {
		x := &run.Executions[i]
		if x.Outcome != "" {
			continue
		}
		r := rollouts[pathOf[x.ToolUseID]]
		if r == nil {
			continue
		}
		outcome, ok := r.outcomes[x.ToolUseID]
		if !ok {
			continue
		}
		x.Outcome = outcome
		if code, ok := r.codes[x.ToolUseID]; ok {
			c := code
			x.ExitCode = &c
		}
	}
}

// codexAccounting is accounting for a Codex rollout. Being in the rollout and
// having a result are one fact there: an item is written when the call has
// completed. So an id recorded with no item is both missing from the
// transcript and declared without a result, and a command or file change with
// an item and no execution record is executed but unrecorded. Nothing in an
// item says a call was denied or refused before the hooks, so those lists are
// empty, as measured rather than as checked.
func codexAccounting(path string, r *codexRollout, recorded, executed, hooked map[string]bool) Transcript {
	t := Transcript{Path: path, IDsRecorded: len(recorded)}
	for id := range recorded {
		if executed[id] {
			t.IDsExecuted++
		}
	}
	if r == nil {
		return t
	}
	t.Readable = true
	files := 1
	t.Files = &files
	n := len(r.ids)
	t.IDsInTranscript = &n
	t.ResultsInTranscript = &n

	t.RefusedBeforeHooks = []string{}
	t.DeniedByUser = []string{}
	t.MissingFromStore = []string{}
	t.ExecutedButUnrecorded = []string{}
	for id := range r.calls {
		if !hooked[id] {
			t.MissingFromStore = append(t.MissingFromStore, id)
		}
		if !executed[id] {
			t.ExecutedButUnrecorded = append(t.ExecutedButUnrecorded, id)
		}
	}
	t.MissingFromTranscript = []string{}
	for id := range recorded {
		if !r.ids[id] {
			t.MissingFromTranscript = append(t.MissingFromTranscript, id)
		}
	}
	t.DeclaredWithoutResult = append([]string{}, t.MissingFromTranscript...)
	sort.Strings(t.MissingFromStore)
	sort.Strings(t.MissingFromTranscript)
	sort.Strings(t.ExecutedButUnrecorded)
	sort.Strings(t.DeclaredWithoutResult)
	return t
}
