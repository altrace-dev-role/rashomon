package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"sort"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// The first pass of the session merge (merge.go): the index of a session's
// calls, built from every file before anything is yielded. It holds a digest of
// each tool_use_id's input and result, the Agent calls and the agents their
// results name, and the type each subagent's lines are attributed to; from them
// it decides which calls have copies that disagree and which subagents have a
// type.

type digest16 [16]byte

func hash16(parts ...[]byte) digest16 {
	h := sha256.New()
	for _, p := range parts {
		var n [4]byte
		n[0], n[1], n[2], n[3] = byte(len(p)>>24), byte(len(p)>>16), byte(len(p)>>8), byte(len(p))
		h.Write(n[:])
		h.Write(p)
	}
	var d digest16
	copy(d[:], h.Sum(nil))
	return d
}

// callState is what the index holds of one tool_use_id.
type callState struct {
	input, result digest16
	hasResult     bool
	// conflict: two copies differ in the tool, the input or the result.
	conflict bool

	// The Agent call's side of a link.
	agentCall bool
	subType   string
	claimed   string
	// claimConflict: the call claims two agents (a second result line, or a
	// copy, names another).
	claimConflict bool
	id            string
	// ownAgent is the agent the call itself belongs to, when it is a subagent's
	// (an Agent call made inside a subagent).
	ownAgent         string
	file, useLine    int
	resultLine       int
	usedAt, resultAt time.Time

	// pre, post: an event of that kind was yielded for the call.
	pre, post, denied bool
}

type agentSeen struct {
	calls        int
	attr         []string
	attrOverflow bool
}

type index struct {
	cap       int
	ids       map[digest16]*callState
	capHit    bool
	untracked int
	claims    map[string][]digest16
	agents    map[string]*agentSeen
}

func isAgentTool(name string) bool { return name == "Agent" || name == "Task" }

func inputDigest(c *history.Call) digest16 {
	in := []byte("<dropped>")
	if len(c.ToolInput) > 0 {
		var b bytes.Buffer
		if json.Compact(&b, c.ToolInput) == nil {
			in = b.Bytes()
		} else {
			in = c.ToolInput
		}
	}
	return hash16([]byte(c.ToolName), in)
}

// resultDigest is a digest of what the recorder reads of a result: its
// outcome, exit code, the two background bits and the head and tail of its
// text. ok is false for a call with no result.
func resultDigest(c *history.Call) (d digest16, ok bool) {
	if c.Outcome == history.OutcomeNone {
		return d, false
	}
	code := []byte("-")
	if c.ExitCode != nil {
		code = []byte(json.Number(itoa(*c.ExitCode)))
	}
	bits := []byte{b2b(c.ResultObject), b2b(c.ResultBackgroundTaskID), b2b(c.ResultBackgroundedByUser)}
	return hash16([]byte{byte(c.Outcome)}, code, bits, c.OutputHead, c.OutputTail), true
}

func b2b(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// add records one call read from a file.
func (ix *index) add(file int, c *history.Call) {
	key := hash16([]byte(c.ToolUseID))
	in := inputDigest(c)
	res, has := resultDigest(c)
	st := ix.ids[key]
	if st == nil {
		if len(ix.ids) >= ix.cap {
			ix.capHit = true
			ix.untracked++
			return
		}
		st = &callState{input: in, file: file, useLine: c.UsePos.Line, ownAgent: c.AgentID, id: c.ToolUseID}
		st.usedAt, _ = time.Parse(time.RFC3339Nano, c.Timestamp)
		if isAgentTool(c.ToolName) {
			st.agentCall = true
			st.subType = subagentType(c.ToolInput)
		}
		ix.ids[key] = st
		if c.Sidechain && c.AgentID != "" {
			if a := ix.agent(c.AgentID); a != nil {
				a.calls++
			}
		}
	} else if st.input != in {
		st.conflict = true
	}
	if has {
		switch {
		case !st.hasResult:
			st.hasResult, st.result = true, res
			st.resultLine = c.ResultPos.Line
			st.resultAt, _ = time.Parse(time.RFC3339Nano, c.ResultTimestamp)
		case st.result != res:
			st.conflict = true
		}
	}
	if c.Sidechain && c.AgentID != "" && c.AttributionAgent != "" {
		if a := ix.agent(c.AgentID); a != nil {
			a.note(c.AttributionAgent)
		}
	}
	if st.agentCall {
		if c.ResultAgentConflict {
			st.claimConflict = true
		}
		if a := c.ResultAgentID; a != "" {
			switch {
			case st.claimed == "":
				st.claimed = a
			case st.claimed != a:
				st.claimConflict = true
			}
			ix.claim(a, key)
		}
	}
}

func (ix *index) agent(id string) *agentSeen {
	a := ix.agents[id]
	if a == nil {
		if len(ix.agents) >= maxAgents {
			return nil
		}
		a = &agentSeen{}
		ix.agents[id] = a
	}
	return a
}

func (a *agentSeen) note(typ string) {
	for _, t := range a.attr {
		if t == typ {
			return
		}
	}
	if len(a.attr) >= maxAttributions {
		a.attrOverflow = true
		return
	}
	a.attr = append(a.attr, typ)
}

func (ix *index) claim(agent string, call digest16) {
	for _, c := range ix.claims[agent] {
		if c == call {
			return
		}
	}
	if len(ix.claims[agent]) < maxClaimants {
		ix.claims[agent] = append(ix.claims[agent], call)
	}
}

// subagentType is an Agent call's input.subagent_type: the string as written,
// "" when the input has none or it is not a string.
func subagentType(input json.RawMessage) string {
	var in struct {
		SubagentType json.RawMessage `json:"subagent_type"`
	}
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	var s string
	if json.Unmarshal(in.SubagentType, &s) != nil {
		return ""
	}
	return s
}

// resolve decides every subagent's link from the index alone, by the rule in
// the file comment, and returns the link of each and the claimant of those
// that stand.
func (ix *index) resolve() (links []AgentLink, claimant map[string]*callState) {
	claimant = map[string]*callState{}
	ids := make([]string, 0, len(ix.agents))
	for id := range ix.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		a := ix.agents[id]
		l := AgentLink{AgentID: id, Calls: a.calls}
		cs := ix.claims[id]
		switch {
		case len(cs) == 0:
			l.Status = LinkNoParent
		case len(cs) > 1:
			l.Status = LinkAmbiguous
		default:
			st := ix.ids[cs[0]]
			switch {
			case st == nil || st.claimConflict:
				l.Status = LinkAmbiguous
			case st.conflict:
				l.Status = LinkParentConflict
			case st.subType == "":
				l.Status = LinkNoType
			case a.attrOverflow || mismatch(a.attr, st.subType):
				l.Status = LinkAttributionMismatch
			default:
				l.Status, l.Type = LinkOK, st.subType
				claimant[id] = st
			}
		}
		links = append(links, l)
	}
	return links, claimant
}

func mismatch(attr []string, typ string) bool {
	for _, a := range attr {
		if a != typ {
			return true
		}
	}
	return false
}

// agentTurns builds Options.Agents from the links that stand: each agent's type
// and the turn its Agent call was made in, read from the claimant's file.
func agentTurns(links []AgentLink, claimant map[string]*callState, files []Source, opt Options) (map[string]Agent, error) {
	agents := map[string]Agent{}
	byFile := map[int][]string{}
	for _, l := range links {
		if l.Status != LinkOK {
			continue
		}
		st := claimant[l.AgentID]
		a := Agent{Type: l.Type, CallID: st.id, NotBefore: st.usedAt.UTC(), NotAfter: st.resultAt.UTC()}
		agents[l.AgentID] = a
		if st.ownAgent == "" {
			byFile[st.file] = append(byFile[st.file], l.AgentID)
		}
	}
	for file, ids := range byFile {
		rc, err := files[file].Open()
		if err != nil {
			return nil, err
		}
		// The turns are wanted, not the Stops: a budget of one byte holds none
		// of their text.
		cx, err := ReadContexts(rc, Options{FinalBudget: 1, Limits: opt.Limits})
		rc.Close()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			a, st := agents[id], claimant[id]
			a.Turn = turnOf(cx, st)
			agents[id] = a
		}
	}
	// A subagent started by a subagent runs in the turn its parent subagent did.
	for range maxAgentDepth {
		for _, l := range links {
			st := claimant[l.AgentID]
			if l.Status != LinkOK || st.ownAgent == "" {
				continue
			}
			if p, ok := agents[st.ownAgent]; ok && p.Turn != nil {
				a := agents[l.AgentID]
				t := *p.Turn
				a.Turn = &t
				agents[l.AgentID] = a
			}
		}
	}
	return agents, nil
}

func turnOf(cx *Contexts, st *callState) *AgentTurn {
	t, ok := cx.At(st.useLine)
	if !ok {
		return nil
	}
	return &AgentTurn{PromptID: t.PromptID, Mode: t.Mode,
		ModeChanged: t.ModeChangeLine != 0 && (st.resultLine == 0 || t.ModeChangeLine < st.resultLine)}
}
