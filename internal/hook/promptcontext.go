package hook

import (
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// PromptContext is Route's context for a note made at UserPromptSubmit
// outside a Post: recap's catch-up, which delivers the request's preflight.
// It is a post's context (contextOf) for the new prompt, drawn the same way,
// so a held-out session's preflight goes to the user only, like its other
// notes. sp is the spec discovered for the prompt's cwd, whose holdout
// object sets the share; project is the project root the note is about.
func PromptContext(st *store.Store, sp *spec.Compiled, sessionID, promptID, project, permissionMode string, env SteerEnv) steer.Context {
	p := &Post{st: st, sp: sp, sessionID: sessionID, promptID: promptID, permissionMode: permissionMode,
		event: string(steer.EventUserPromptSubmit)}
	return p.contextOf(env, verify.SteerView{ProjectRoot: project})
}
