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

// printContinuity is probe start's print at a compaction (card COMPACT-SLOT1,
// tech 4.2.4 :1255): the continuity briefing, the person's words the last
// compaction removed (directive.Briefing), as SessionStart's
// additionalContext with its mirror to the person, lead, the plugin route's
// consent line, opening the mirror. It reports whether it printed; when it
// did not, the brief prints as before (printBrief, which at compact is the
// lead alone).
//   - Only at source compact, on a transcript at most continuityCap.
//   - The transcript's last compact_boundary keys the briefing, and one given
//     for it already is not given again (directive.Given, D-b).
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
	if err != nil || source != "compact" || sessionID == "" || cwd == "" || transcript == "" {
		return false
	}
	b, ok, err := directive.LastBoundary(transcript, continuityCap)
	if err != nil || !ok {
		return false
	}
	lines, _, _ := st.ReadSteer(sessionID)
	if directive.Given(lines, b.UUID, "") {
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
	key := st.Key()
	f, ok := directive.Briefing(ds, b, func(w string) string { return intent.SaidDigest(key, w) }, run.LastSeq())
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
