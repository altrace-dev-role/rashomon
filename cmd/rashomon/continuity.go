package main

import (
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/directive"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// continuityCap is the largest transcript a compact start reads, 256 MiB
// (mgr ruling 11-02): the boundary pass and Read each read it once, forward,
// a line at a time. 37a15912, two days and 14 compactions, is 39.8 MB.
const continuityCap = 256 << 20

// readDirectives is DIRECTIVE-KEEP1's Read, a variable so a test can stand
// in for it until that row's body lands.
var readDirectives = directive.Read

// printStart is probe start's one write to stdout: the continuity briefing
// at a compaction (printContinuity), else the project brief (printBrief).
func printStart(args []string, payload *hookio.Payload, st *store.Store, lead string, id harness.Identity, stdout io.Writer) {
	if !printContinuity(args, payload, st, lead, id, stdout) {
		printBrief(args, payload, st, lead, stdout)
	}
}

// printContinuity is probe start's print at a compaction (card COMPACT-SLOT1,
// tech 4.2.4 :1255): the continuity briefing, the person's words the last
// compaction removed (directive.Briefing), as SessionStart's
// additionalContext with its mirror to the person, lead, the plugin route's
// consent line, opening the mirror. It reports whether it printed; when it
// did not, the brief prints as before (printBrief, which at compact is the
// lead alone, and at resume the resume line).
//   - Only at source compact, on a transcript at most continuityCap; at
//     source resume, whose channel is closed, a briefing owed is armed for
//     the session's next prompt (ArmBriefing, resumeBriefing; D-c) and
//     nothing is read or printed.
//   - The transcript's last compact_boundary keys the briefing, and one given
//     for it already is not given again (owedBoundary, D-b).
//   - The words are Read here, at delivery, and never stored: the steer line
//     holds each quote's keyed digest (intent.SaidDigest) and byte length
//     (D160, D-a).
//   - Route decides as for any note: the compact row's gate (P-TRU-1),
//     consent, the steer switch, the holdout, attendance and arm off, whose
//     line it logs and whose print it withholds.
func printContinuity(args []string, payload *hookio.Payload, st *store.Store, lead string, id harness.Identity, stdout io.Writer) bool {
	if len(args) == 0 || args[0] != store.PhaseStart || payload == nil {
		return false
	}
	var source, sessionID, cwd, transcript, mode string
	err := payload.Strs(hookio.Into(hookio.Source, &source), hookio.Into(hookio.SessionID, &sessionID), hookio.Into(hookio.CWD, &cwd),
		hookio.Into(hookio.TranscriptPath, &transcript), hookio.Into(hookio.PermissionMode, &mode))
	if err != nil || source != "compact" && source != "resume" || sessionID == "" || cwd == "" || transcript == "" {
		return false
	}
	b, ok := owedBoundary(st, sessionID, transcript)
	if !ok {
		return false
	}
	if source == "resume" {
		_ = st.ArmBriefing(sessionID) // a failed arm is a briefing not given, as before D-c
		return false
	}
	ds, err := readDirectives(transcript, continuityCap)
	if err != nil || len(ds) == 0 {
		return false
	}
	run, err := digest.ReadRun(st, sessionID)
	if err != nil {
		return false
	}
	f, ok := briefingOf(st, ds, b, run.LastSeq())
	if !ok {
		return false
	}
	env := steerEnv(id, time.Now(), st.Root())
	sp := preflightSpec(st, run, cwd, env.ProjectDir, env.ManagedDir)
	// The line's project follows the record scope as a post's does: the
	// cwd's project key where the scope keeps it (v5), "" at v4.
	project := ""
	if hook.PromptRecordScope(st) == store.RecordScopeV5 {
		project = baseline.Key(cwd)
	}
	ctx := hook.StartContextAt(st, sp, sessionID, cwd, project, mode, source, env)
	out := steer.Route(&ctx, []steer.Finding{f})
	// The line is durable before the briefing prints (REC-34); a failed
	// flush prints nothing.
	if err := st.Flush(); err != nil || out == (steer.Output{}) {
		return false
	}
	switch {
	case lead == "":
	case out.SystemMessage == "":
		out.SystemMessage = lead
	default:
		out.SystemMessage = lead + "\n" + out.SystemMessage
	}
	// Route withheld any channel the client cannot carry, so an error here
	// is a contract break: print nothing.
	p, err := out.MarshalFor(string(steer.EventSessionStart), ctx.Client)
	if err != nil || p == nil {
		return false
	}
	_, _ = stdout.Write(p)
	return true
}

// owedBoundary is the transcript's last compact_boundary when the session's
// main agent has not been given its briefing (directive.Given, D-b): one
// forward pass over a transcript at most continuityCap, and the steer log.
func owedBoundary(st *store.Store, sessionID, transcript string) (directive.Boundary, bool) {
	b, ok, err := directive.LastBoundary(transcript, continuityCap)
	if err != nil || !ok {
		return directive.Boundary{}, false
	}
	lines, _, _ := st.ReadSteer(sessionID)
	return b, !directive.Given(lines, b.UUID, "")
}

// briefingOf is the briefing finding for ds before b, its record keyed
// under the store's key (intent.SaidDigest; D160, D-a).
func briefingOf(st *store.Store, ds []directive.Directive, b directive.Boundary, afterSeq int64) (steer.Finding, bool) {
	key := st.Key()
	return directive.Briefing(ds, b, func(w string) string { return intent.SaidDigest(key, w) }, afterSeq)
}

// resumeBriefing is the briefing a resumed session's start armed (D-c),
// for the prompt path to route beside its own notes: it takes the arm
// first, so of two prompts only one reads, and whatever the route then
// decides, the arm is spent and later prompts pay one failed remove. A
// boundary given since the arm, or no quote before it, is no briefing.
func resumeBriefing(st *store.Store, run *store.Run, sessionID, transcript string) (steer.Finding, bool) {
	if sessionID == "" || transcript == "" || !st.TakeBriefingArm(sessionID) {
		return steer.Finding{}, false
	}
	b, ok := owedBoundary(st, sessionID, transcript)
	if !ok {
		return steer.Finding{}, false
	}
	ds, err := readDirectives(transcript, continuityCap)
	if err != nil || len(ds) == 0 {
		return steer.Finding{}, false
	}
	return briefingOf(st, ds, b, run.LastSeq())
}
