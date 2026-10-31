package main

import (
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// pendingPayload is recapPayload plus what only the UserPromptSubmit
// catch-up reads: the new prompt, in a buffer clearPrompt zeroes.
type pendingPayload struct {
	recapPayload
	Prompt         []byte
	PromptCut      bool
	PromptID       string
	PermissionMode string
}

// readPendingPayload is readRecapPayload for `recap pending`.
func readPendingPayload(in io.Reader, timeout time.Duration) pendingPayload {
	return readPayload(in, timeout, true)
}

// clearPrompt zeroes the prompt's bytes and drops the slice. The streamer's
// own window and head buffer held the bytes too and are not reachable here
// (the limit GATEOUT-WIRE1 accepted).
func clearPrompt(p *pendingPayload) {
	clear(p.Prompt)
	p.Prompt = nil
}

// preflightNote is the new prompt's task-preflight note, routed and logged
// under the session, and the client its Output is marshalled for; the zero
// Output when there is no run to log it under (no session_id, H-107), no
// cwd to resolve against, or nothing new to say. A request that names no
// path and no identifier stops at intent.Names, before any read and before
// the steer environment is built: with no brief on this branch it can have
// no note. A session's first prompt finds its run empty, so the note decides at
// after_seq 0, which CheckOn admits at UserPromptSubmit only.
func preflightNote(st *store.Store, run *store.Run, p *pendingPayload, id harness.Identity, now time.Time) (steer.Output, steer.Client) {
	if run == nil || p.CWD == "" || !intent.Names(p.Prompt, p.PromptCut) {
		return steer.Output{}, steer.Client{}
	}
	env := steerEnv(id, now, st.Root())
	sp := preflightSpec(st, run, p.CWD, env.ProjectDir, env.ManagedDir)
	var idx *verify.ProjectIndex
	if key := baseline.Key(p.CWD); key != "" {
		idx, _ = verify.ReadIndex(verify.IndexPath(st.Root(), key))
	}
	lines, _, _ := st.ReadSteer(p.SessionID)
	in := intent.Resolve(p.Prompt, p.PromptCut, intent.Project{Root: p.CWD, Spec: sp, Index: idx, NowMS: env.Now.UnixMilli()})
	f, ok := intent.Finding(in, intent.ReadDelivered(lines), nil, run.LastSeq())
	if !ok {
		return steer.Output{}, steer.Client{}
	}
	ctx := hook.PromptContext(st, sp, p.SessionID, p.PromptID, p.CWD, p.PermissionMode, env)
	return steer.Route(&ctx, []steer.Finding{f}), ctx.Client
}

// printTurn prints the catch-up's one stdout object: the preflight note's
// Output with the catch-up line, when there is one, joined to its
// systemMessage. st, when not nil, is flushed first, so the note's steer
// line is durable before the note prints (REC-34); a caller that already
// flushed passes nil. A failed flush prints nothing.
func printTurn(st *store.Store, stdout io.Writer, note steer.Output, c steer.Client, line string) error {
	if note == (steer.Output{}) && line == "" {
		return nil
	}
	if st != nil {
		if err := st.Flush(); err != nil {
			return err
		}
	}
	switch {
	case line == "":
	case note.SystemMessage == "":
		note.SystemMessage = line
	default:
		note.SystemMessage += "\n" + line
	}
	// Route withheld any channel the client cannot carry, so an error here
	// is a contract break: print nothing.
	b, err := note.MarshalFor(string(steer.EventUserPromptSubmit), c)
	if err != nil || b == nil {
		return nil
	}
	_, _ = stdout.Write(b)
	return nil
}

// preflightSpec is the spec the prompt's note reads, discovered as the steer
// step and the Stop discover it: the trust bindings are read under the store
// root only while the session has not written there by the run's last seq
// (hook.TrustRoot, D203), so a session that could have forged its own
// binding gets no trusted repository spec on the prompt path either.
func preflightSpec(st *store.Store, run *store.Run, cwd, projectDir, managedDir string) *spec.Compiled {
	return hook.DiscoverSpec(cwd, projectDir, managedDir, hook.TrustRoot(run, st.Root(), run.LastSeq()))
}
