package report

// Autosteer is what the record shows of a session's AutoSteer notes and of
// its last Stop (card REPORT1a, D162: evidence, never a score). Every count
// is the length of a list of records, each with its seq or id, and nothing is
// estimated. recap.ImpactOf derives it, and the Stop line's clause is read
// from the same derivation, so the two never disagree; internal/report cannot
// import recap, so the caller (cmd/rashomon) fills it from there.
//
// Null on a Session the report did not read it for: a report of every
// session reads none, since each needs a replay of its last Stop. A session
// with no Stop recorded has an Autosteer whose Stop is null.
type Autosteer struct {
	// Stop is the session's last Stop as replayed from its checkpoint, and
	// Gates the gates as that Stop saw them: the first screen's story.
	Stop  *AutosteerStop  `json:"stop"`
	Gates []AutosteerGate `json:"gates"`
	// Clause is the AutoSteer clause's counts in its own words, "" with
	// nothing to count.
	Clause    string             `json:"clause"`
	Delivered []AutosteerNote    `json:"delivered"`
	Findings  []AutosteerFinding `json:"findings"`
	Checks    []AutosteerCheck   `json:"checks"`
	Lessons   *AutosteerLessons  `json:"lessons"`
	Unknown   []AutosteerUnknown `json:"unknown"`
}

// AutosteerStop is the last Stop: its point seq, the row of verify's table it
// read and the state sentence the value line printed for it.
type AutosteerStop struct {
	Seq      int64  `json:"seq"`
	Row      string `json:"row"`
	Sentence string `json:"sentence"`
}

// AutosteerGate is one gate at the Stop: a program word or the runner's name (the kind when the
// program is a prefix word), its state and the seqs of its last run and last
// green run, null with none.
type AutosteerGate struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	State        string `json:"state"`
	LastRunSeq   *int64 `json:"last_run_seq"`
	LastGreenSeq *int64 `json:"last_green_seq"`
}

// AutosteerNote is one note delivered to the agent: its steer line's id, the
// pattern and its title in the clause, the event, the prompt (empty with
// none) and the seq the line was decided after.
type AutosteerNote struct {
	ID       string `json:"id"`
	Pattern  string `json:"pattern"`
	Title    string `json:"title"`
	Event    string `json:"event"`
	PromptID string `json:"prompt_id"`
	Seq      int64  `json:"seq"`
}

// AutosteerFinding is one finding. State is resolved, open (the line names a
// gate and no green run of it came after) or unchecked (it names no gate, so
// no run settles it); ResolvedBy is the green run's seq, 0 unless resolved.
type AutosteerFinding struct {
	ID         string `json:"id"`
	Pattern    string `json:"pattern"`
	Title      string `json:"title"`
	Gate       string `json:"gate"`
	Seq        int64  `json:"seq"`
	State      string `json:"state"`
	ResolvedBy int64  `json:"resolved_by"`
}

// AutosteerCheck is one expected evidence row at the Stop: its kind and state
// (met, unmet, open or unknown), the gate that settles it, the seq of that
// gate's green run when met (0 otherwise) and, when unmet, the reason in the
// record's words.
type AutosteerCheck struct {
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Gate   string `json:"gate"`
	Seq    int64  `json:"seq"`
	Reason string `json:"reason"`
}

// AutosteerLessons is the project's lesson ledger by tier. Autosteer holds
// none (null) when the report opened no ledger, which is unknown and never
// zero.
type AutosteerLessons struct {
	Approved int `json:"approved"`
	Proposed int `json:"proposed"`
	Demoted  int `json:"demoted"`
}

// AutosteerUnknown is something the record cannot settle: a masked run (Seq
// its call) or an expected check with no readable state (Seq 0).
type AutosteerUnknown struct {
	Kind string `json:"kind"`
	Seq  int64  `json:"seq"`
}
