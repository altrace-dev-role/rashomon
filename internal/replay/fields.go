package replay

// Class says what became of one payload key in the replay: whether the
// recorder reads it, and whether the transcript supplies it.
type Class string

const (
	// Equal (a): read by the recorder and rebuilt byte for byte equal to the
	// payload Claude Code sent (JSON values compared after compacting
	// whitespace).
	Equal Class = "a"
	// Normalised (b): read by the recorder and rebuilt equal after a
	// normalisation Field.Note states.
	Normalised Class = "b"
	// Unread (c): no code the entry points reach consults it. The replay does
	// not rebuild it, except where Field.Note says it is supplied for the
	// payload's shape and then it is Equal.
	Unread Class = "c"
	// Unrecoverable (d): read by the recorder and not in the transcript, so an
	// event that needs it is Unsupported (Field.Reason) and gets no payload.
	// The fixtures hold none: neither paired session has a subagent.
	Unrecoverable Class = "d"
	// Omitted (e): read and stored by the recorder, not in the transcript, and
	// consulted by nothing that decides a verdict. The replay leaves it out,
	// and the recorder's own rule for an absent value applies. This is the
	// one place a read field is not a reason to mark the event Unsupported;
	// see Field.Note for duration_ms and the cost of the alternative.
	Omitted Class = "e"
)

// Field is one payload key of one event kind.
type Field struct {
	Key   string
	Class Class
	// Tools restricts the row to Bash calls ("bash") or to every other tool
	// ("other"); empty is every call.
	Tools string
	// Reason is, for Unrecoverable, the Reason an event that needs the key
	// carries.
	Reason Reason
	// Supplied: an Unread key the replay builds anyway, for the shape of the
	// payload, and which must then be Equal. A key the recorder reads only
	// on a path that event never takes is Unread and Supplied.
	Supplied bool
	// NotExercised says why the entry-point test cannot show an effect of
	// removing or changing a read key on these fixtures, when it cannot: the
	// code that reads it, and what would have to be true to see it. Empty for
	// every key whose removal or change is seen.
	NotExercised string
	// Note is the evidence: the normalisation for Normalised, the call chain
	// that reads the key for the read classes, and for Unread the entry
	// points checked.
	Note string
}

// Fields is the field table for an event kind: every key present in either
// a payload Claude Code sent for that event on the paired fixtures
// (test/fixtures/claude-code/2.1.280 and 2.1.312) or one the replay builds,
// and what became of it. TestREPLAY1_FieldCompleteness asserts every row
// against both fixtures, and cmd/rashomon's TestREPLAY5_ asserts the Unread
// rows and the read rows against the entry points themselves.
//
// The entry points are the ones Claude Code runs (install.Events), in
// process: cmdHook (PreToolUse), cmdPost (PostToolUse, PostToolUseFailure),
// cmdRecap (Stop; UserPromptSubmit as `recap pending`) in cmd/rashomon. The
// call chains whose reads the rows cite:
//
//   - cmdHook: claudeHostGuard (hookio.Read, hostguard.EvaluateRaw,
//     hookio.IntakeBytes, harness.Detect), checkPaused, openForHook,
//     standsDown, newHandler, Handler.CapturePayload (Payload.from, Locate
//     with shape.DeriveWithDirs and baseline.Key, shape.Hosts,
//     shape.DeclaredPath, fileLabel, store.GateDeclaration and CapDeclaration,
//     AppendDeclaration), Handler.Close (BuildCoverage).
//   - cmdPost: the same guard and gates, Post.CapturePayload (PostPayload.from,
//     shape.Derive, backgroundedCall, exitCode, store.LandExecution),
//     Post.Close (BuildCoverage), and Post.Steer (judge, verify.SteerAt,
//     steer.Route, which read the prompt id, agent id, permission mode and
//     event the payload gave).
//   - cmdRecap: readRecapPayload and readFinalMessage (the streamed paths),
//     recapHostGuard (harness.Detect), standsDown, digest.BuildFrom,
//     stopView (verify.EvaluateStop, recap.DeliveredNotes) and recap.Claim.
//
// hookio.KeyNames lists the 22 keys any consumer of hookio.Payload may read,
// and readFinalMessage's paths are the keys the recap entry streams; a key in
// neither is not read by construction, which is a first cut: the entry-point
// test is the proof, since a chain can reach a key through a helper.
func Fields(k Kind) []Field { return fieldTable[k] }

const (
	noteSession  = "hook.Payload.from, PostPayload.from and readFinalMessage name the session; the recorder files a payload by it (attribute)."
	noteCWD      = "the working directory: Payload.CWD (Locate, baseline.Key; the declaration's cwd, project, cwd_digest), PostPayload.CWD (the coverage record), readFinalMessage's cwd (the project index key)."
	noteMode     = "Payload.PermissionMode is stored on the declaration; PostPayload.PermissionMode reaches the steer step (contextOf: dontAsk is unattended)."
	notePrompt   = "Payload.PromptID is stored on the declaration; PostPayload.PromptID reaches the steer step (Point, Context.PromptID)."
	noteToolName = "shape.Derive and shape.Label key on it; the record names it."
	noteInput    = "shape.DeriveWithDirs, Hosts, DeclaredPath and Label read it; Post derives the executed digest and runner from it. Normalised: byte-equal after compaction on every call but the Edits, whose transcript input is the same JSON value with its keys in another order (replace_all first); whether the order can reach a record is TestREPLAY6_'s question. A hook that rewrote the input leaves the transcript with the original, so a rewrite is invisible to the replay (not detectable)."
	noteTransc   = "Normalised: the caller's path for the file replaces Claude Code's. The captured value is the live location (projects/<dir>/<session_id>.jsonl, a layout harness.Detect recognises); the fixture is relocated, so the test substitutes one for the other and checks the captured layout. Stored on the declaration; read by harness.Detect in claudeHostGuard and recapHostGuard when CLAUDE_CODE_ENTRYPOINT is unset (deleting it then stamps harness_id unknown)."
	noteUnread   = "no entry point reaches it: removing it from every payload of the kind changes neither the store nor any output, under both host-guard environments (TestREPLAY5_)."
	noteDuration = "Omitted: PostPayload.DurationMS is stored as execution.duration_ms (positive: a zero or absent value is stored as null, the recorder's own 'nobody timed it'); nothing reads it back (report, verify and steer never touch it). A transcript holds no client-side duration: the timestamps of the tool_use and tool_result lines differ from it (210 ms between the lines of 2.1.312's toolu_fixture03 against 171 ms measured), so a duration taken from them would be a different number, written as if measured. Strictly (d), this would make every PostToolUse Unsupported and the imported corpus empty of executions. B2b must compare execution records with duration_ms excluded."
)

var fieldTable = map[Kind][]Field{
	UserPromptSubmit: {
		{Key: "session_id", Class: Equal, Note: noteSession},
		{Key: "transcript_path", Class: Normalised, Note: noteTransc},
		{Key: "cwd", Class: Equal, Note: noteCWD, NotExercised: "stopView keys the project index on it, and only a Stop writes the index, so on a session whose turns no Stop reached (the catch-up scenario) there is nothing under the key to read (stopView, baseline.Key, verify.ReadIndex)"},
		{Key: "hook_event_name", Class: Equal, Note: "harness.Detect reads it (the Codex rule); cmdRecap's recapPending is the argument `pending`, not the payload.", NotExercised: "harness.Detect compares it only beside turn_id or model, which a Claude Code payload does not carry"},
		{Key: "prompt", Class: Unread, Note: "the NEW prompt's text, never read (recapTurn); not rebuilt, so no prompt text enters the timeline. " + noteUnread},
		{Key: "prompt_id", Class: Unread, Note: "digest.BuildFrom picks the prompt itself from the run. " + noteUnread},
		{Key: "permission_mode", Class: Unread, Note: noteUnread},
		{Key: "scratchpad_dir", Class: Unread, Note: "not a hookio key. " + noteUnread},
	},
	PreToolUse: {
		{Key: "session_id", Class: Equal, Note: noteSession},
		{Key: "transcript_path", Class: Normalised, Note: noteTransc},
		{Key: "cwd", Class: Equal, Note: noteCWD + " The tool_use line's: where the call was made."},
		{Key: "prompt_id", Class: Equal, Note: notePrompt + " A tool_use line names none; the prompt of the turn the line falls in (the last genuine prompt before it) is the value, and equals the captured one on every call."},
		{Key: "permission_mode", Class: Equal, Note: noteMode + " The turn's prompt line names it; a change mid-turn is invisible unless a permission-mode line says so (PermissionModeChanged, unverified)."},
		{Key: "hook_event_name", Class: Equal, Note: "Payload.HookEventName is parsed and not consulted on the Pre path; the host guard reads it (harness.Detect).", NotExercised: "harness.Detect compares it only beside turn_id or model, which a Claude Code payload does not carry"},
		{Key: "tool_name", Class: Equal, Note: noteToolName},
		{Key: "tool_input", Class: Normalised, Note: noteInput},
		{Key: "tool_use_id", Class: Equal, Note: "the call's key to its execution; store.CapID."},
		{Key: "agent_id", Class: Unrecoverable, Reason: SidechainAgentType, Note: "stored on the declaration. A subagent's line names agentId, but agent_type (stored beside it) is in no line of its own. Walk alone has no link, so a sidechain call is Unsupported; WalkSession recovers the type through the parent's Agent call alone (Options.Agents, MergeSummary.Agents) and the pair is Equal to the payload's on the 2.1.312-subagents fixture (TestREPLAY25_)."},
		{Key: "agent_type", Class: Unrecoverable, Reason: SidechainAgentType, Note: "see agent_id."},
		{Key: "effort", Class: Unread, Note: "2.1.312 only; not a hookio key. " + noteUnread},
		{Key: "scratchpad_dir", Class: Unread, Note: "2.1.312 only; not a hookio key. " + noteUnread},
	},
	PostToolUse:        postFields(false),
	PostToolUseFailure: postFields(true),
	Stop: {
		{Key: "session_id", Class: Equal, Note: noteSession},
		{Key: "transcript_path", Class: Normalised, Note: noteTransc},
		{Key: "cwd", Class: Equal, Note: noteCWD + " The turn's last assistant line's: where the shell stood when the turn ended (2.1.312: wd/sub, after the cd)."},
		{Key: "hook_event_name", Class: Equal, Note: "stopView: writes only for Stop and StopFailure; harness.Detect."},
		{Key: "last_assistant_message", Class: Equal, Note: "digest.BuildFrom scans it for claims; stopView hands its head and tail to verify.EvaluateStop (readFinalMessage: head 256 KiB, tail 64 KiB). The turn's last message's one text block, byte for byte up to what the recorder keeps: a message longer than its head and tail is fed as those two (keepFinal) and the recorder captures the same from either; a message with no text block or more than one, one the live payload could not have carried, or one the replay's budget cannot hold (Options.FinalBudget) is Unsupported."},
		{Key: "prompt_id", Class: Unread, Note: "digest.BuildFrom chooses the prompt from the run, not the payload. " + noteUnread},
		{Key: "permission_mode", Class: Unread, Note: noteUnread},
		{Key: "stop_hook_active", Class: Unread, Note: "parsed into recapPayload.StopHookActive and consulted nowhere ('carried unread', cmd/rashomon/recap_turn.go). " + noteUnread},
		{Key: "background_tasks", Class: Unread, Note: "not a key readFinalMessage streams. " + noteUnread},
		{Key: "session_crons", Class: Unread, Note: "not a key readFinalMessage streams. " + noteUnread},
		{Key: "effort", Class: Unread, Note: "2.1.312 only. " + noteUnread},
		{Key: "scratchpad_dir", Class: Unread, Note: "2.1.312 only. " + noteUnread},
	},
}

// postFields is the table for PostToolUse and, with failed set,
// PostToolUseFailure, which `post` serves as one.
func postFields(failed bool) []Field {
	common := []Field{
		{Key: "session_id", Class: Equal, Note: noteSession},
		{Key: "transcript_path", Class: Normalised, Note: noteTransc + " PostPayload does not read it; the guard does."},
		{Key: "cwd", Class: Equal, Note: noteCWD + " The tool_result line's: where the shell ended up, which differs from the tool_use line's after `cd sub && ...` (2.1.312, toolu_fixture03)."},
		{Key: "tool_name", Class: Equal, Note: noteToolName},
		{Key: "tool_input", Class: Normalised, Note: noteInput},
		{Key: "tool_use_id", Class: Equal, Note: "the key the execution closes its declaration by."},
		{Key: "agent_id", Class: Unrecoverable, Reason: SidechainAgentType, Note: "PostPayload.AgentID reaches the steer step. A sidechain call is Unsupported unless the type was recovered through the parent link (see PreToolUse's agent_id); a subagent's result line carries no toolUseResult, so a foreground Bash call of its is fed as not backgrounded and the event is Assumed (SidechainResponseAbsent)."},
		{Key: "duration_ms", Class: Omitted, Note: noteDuration},
		{Key: "effort", Class: Unread, Note: "2.1.312 only. " + noteUnread},
		{Key: "scratchpad_dir", Class: Unread, Note: "2.1.312 only. " + noteUnread},
	}
	if failed {
		return append(common,
			Field{Key: "prompt_id", Class: Equal, Note: notePrompt + " The tool_result line's own promptId."},
			Field{Key: "permission_mode", Class: Equal, Note: noteMode + " Only dontAsk differs (the steer step reads the call as unattended); the fixtures run under default, so TestREPLAY5_ sees it by changing the value to dontAsk, not by removing it."},
			Field{Key: "hook_event_name", Class: Equal, Note: "`post` dispatches on it: PostToolUseFailure is the failure outcome (Post.capture), and any error result that is not a denial is one."},
			Field{Key: "error", Class: Equal, Note: "PostPayload.Error is the head of the message (8 KiB) and only its first line is parsed (exitCode); the rebuilt value is the result's text, truncated at history's head bound."},
			Field{Key: "is_interrupt", Class: Equal, Note: "stored as execution.is_interrupt; an interrupt makes the outcome interrupted. false for every failure the transcript does not mark interrupted; a call it does mark is Unsupported (InterruptedCall)."},
		)
	}
	return append(common,
		Field{Key: "prompt_id", Class: Unread, Supplied: true, Note: "PostPayload keeps it for the steer step, which runs only for a failure event (steps), so a PostToolUse never consults it. " + noteUnread},
		Field{Key: "permission_mode", Class: Unread, Supplied: true, Note: "kept for the steer step's attendance, which runs only for a failure event. " + noteUnread},
		Field{Key: "hook_event_name", Class: Equal, Note: "`post` compares it with PostToolUseFailure; PostToolUse is every other value, and none.", NotExercised: "an absent name reads as PostToolUse, the same as the real one, and a different one is a different event"},
		Field{Key: "tool_response", Class: Normalised, Tools: "bash", Note: "Normalised: projection onto the two presence bits backgroundedCall reads (backgroundTaskId, backgroundedByUser). Neither is set on any fixture call, so the replay leaves tool_response out and the recorder, finding no object, reads the call as foreground. A key set in toolUseResult is rebuilt as true; a result whose toolUseResult is not an object is Unsupported (ResponseUnreadable)."},
		Field{Key: "tool_response", Class: Unread, Tools: "other", Note: "Post.capture reads it only for Bash. " + noteUnread},
	)
}
