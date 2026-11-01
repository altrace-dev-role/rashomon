package hook

import (
	"github.com/altrace-dev-role/rashomon/internal/harness"
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
// object sets the share; project is the project root the note is about, and
// the cwd the call's switches are read for.
func PromptContext(st *store.Store, sp *spec.Compiled, sessionID, promptID, project, permissionMode string, env SteerEnv) steer.Context {
	return PromptContextAt(st, sp, sessionID, promptID, project, project, permissionMode, env)
}

// PromptContextAt is PromptContext with the project the line is filed under
// apart from the cwd (card PREFLIGHT1b): the steer switches read the cwd's
// project, and the line's project follows the record scope, "" at v4 as a
// post's line carries it there.
func PromptContextAt(st *store.Store, sp *spec.Compiled, sessionID, promptID, cwd, project, permissionMode string, env SteerEnv) steer.Context {
	p := &Post{st: st, sp: sp, sessionID: sessionID, promptID: promptID, permissionMode: permissionMode, cwd: cwd,
		event: string(steer.EventUserPromptSubmit)}
	return p.contextOf(env, verify.SteerView{ProjectRoot: project})
}

// PromptRecordScope is the record scope the prompt path files its line
// under: the marker the post path reads (recordScope), v4 when it is absent
// or unreadable, so an unknown scope withholds.
func PromptRecordScope(st *store.Store) string {
	return recordScope(st)
}

// attendanceOf is who the call reaches (card PREFLIGHT1b, D207). A post's
// is unattended under CI or the dontAsk permission mode (ED-9). The prompt
// path's follows the surface instead: the note there is context for the
// model, not a mirror for a watcher, so CI alone does not silence it, and an
// SDK or cloud surface, which has no one at a keyboard, does. dontAsk stays
// unattended on both.
func attendanceOf(event string, env SteerEnv, permissionMode string) steer.Attendance {
	unattended := env.CI
	if event == string(steer.EventUserPromptSubmit) {
		unattended = env.Surface == string(harness.SurfaceSDK) || env.Surface == string(harness.SurfaceCloud)
	}
	if unattended || permissionMode == "dontAsk" {
		return steer.AttendanceUnattended
	}
	return steer.AttendanceAttended
}
