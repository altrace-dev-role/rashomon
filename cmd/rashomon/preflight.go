package main

import (
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/learn"
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
	// specNoted is set by preflightNote when its discovery made the turn's
	// no-spec note (hook.NoteSpecFound), so noteTurnSpec makes no second one.
	specNoted bool
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

// promptProject is the payload's project as the prompt path reads it, once a
// prompt (D204): key, baseline.Key's walk from the cwd, names both the
// project index the preflight reads and the lesson ledger, and lessons is
// that ledger's approved lessons (readPromptProject).
type promptProject struct {
	key     string
	lessons learn.Active
}

// preflightNote is the new prompt's notes, routed and logged under the
// session in one Route call, and the client its Output is marshalled for:
// the request's task-preflight note and a lesson from proj
// (lessonFinding), which ranks below the note and so is superseded on a
// prompt that carries one (D170 L3). The zero Output when there is no run to
// log it under (no session_id, H-107), no cwd to resolve against, or nothing
// new to say. A request that names no path and no identifier, with no lesson
// to give, stops at intent.Names, before any read and before the steer
// environment is built: with no brief on this branch it can have no note.
// proj is the caller's read (readPromptProject). A session's first prompt
// finds its run empty, so the note decides at after_seq 0, which CheckOn
// admits at UserPromptSubmit only.
func preflightNote(st *store.Store, run *store.Run, p *pendingPayload, proj promptProject, id harness.Identity, now time.Time) (steer.Output, steer.Client) {
	if run == nil || p.CWD == "" {
		return steer.Output{}, steer.Client{}
	}
	named := intent.Names(p.Prompt, p.PromptCut)
	if !named && !proj.lessons.Any() {
		return steer.Output{}, steer.Client{}
	}
	env := steerEnv(id, now)
	sp := preflightSpec(st, run, p.CWD, env.ProjectDir, env.ManagedDir)
	hook.NoteSpecFound(st, p.SessionID, p.PromptID, p.CWD, env.ProjectDir, env.ManagedDir, sp)
	p.specNoted = true
	lines, _, _ := st.ReadSteer(p.SessionID)
	var found []steer.Finding
	var paths []string
	scope := hook.PromptRecordScope(st)
	if named {
		var idx *verify.ProjectIndex
		if proj.key != "" {
			idx, _ = verify.ReadIndex(verify.IndexPath(st.Root(), proj.key))
		}
		root := promptRoot(sp, p.CWD, proj.key)
		cwd := p.CWD
		if cwd != root {
			cwd = resolvedDir(cwd)
		}
		in := intent.Resolve(p.Prompt, p.PromptCut, intent.Project{Root: root, Cwd: cwd, Spec: sp, Index: idx, NowMS: env.Now.UnixMilli()})
		paths = in.Paths
		// At record scope v4 the steer line holds each named path as a
		// keyed digest and the delivered set compares digests; the note
		// that prints is the same at either scope.
		var digest func(string) string
		if scope != store.RecordScopeV5 {
			key := st.Key()
			digest = func(path string) string { return intent.PathDigest(key, path) }
		}
		if f, ok := intent.FindingKeyed(in, intent.ReadDelivered(lines), nil, run.LastSeq(), digest); ok {
			found = append(found, f)
		}
	}
	if f, ok := lessonFinding(proj.lessons, run.LastSeq(), lines, paths); ok {
		found = append(found, f)
	}
	if len(found) == 0 {
		return steer.Output{}, steer.Client{}
	}
	// The line's project follows the record scope as a post's does: the
	// project's key where the scope keeps it (v5), "" at v4.
	project := ""
	if scope == store.RecordScopeV5 {
		project = proj.key
		if project == "" {
			project = baseline.Key(p.CWD)
		}
	}
	ctx := hook.PromptContextAt(st, sp, p.SessionID, p.PromptID, p.CWD, project, p.PermissionMode, env)
	return steer.Route(&ctx, found), ctx.Client
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

// noteTurnSpec is the one spec discovery every prompt makes for the turn's
// no-spec note (card SPEC-RULES1, D210), when preflightNote's did not: a
// prompt that names no path or identifier, which the note itself never
// discovers for. A project with no spec costs the lean discovery, and the
// turn's rules-only posts then skip theirs (hook.NoteSpecFound). A prompt
// with no run, no session or no cwd writes nothing, so its posts discover.
func noteTurnSpec(st *store.Store, run *store.Run, p *pendingPayload) {
	if st == nil || run == nil || p.SessionID == "" || p.CWD == "" || p.specNoted {
		return
	}
	projectDir, managedDir := specDirs()
	hook.NoteSpecFound(st, p.SessionID, p.PromptID, p.CWD, projectDir, managedDir,
		preflightSpec(st, run, p.CWD, projectDir, managedDir))
	p.specNoted = true
}

// preflightSpec is the spec the prompt's note reads, discovered as the steer
// step and the Stop discover it: the trust bindings are read under the store
// root only while the session has not written there by the run's last seq
// (hook.TrustRoot, D203), so a session that could have forged its own
// binding gets no trusted repository spec on the prompt path either.
func preflightSpec(st *store.Store, run *store.Run, cwd, projectDir, managedDir string) *spec.Compiled {
	return hook.DiscoverSpec(cwd, projectDir, managedDir, hook.TrustRoot(run, st.Root(), run.LastSeq()))
}

// repoSpecSuffix ends the path of a repository spec source: the file sits at
// <project>/.claude/rashomon/spec.json.
const repoSpecSuffix = "/.claude/rashomon/spec.json"

// promptRoot is the project a request's named paths and a spec's rule paths
// resolve against: the project of the repository spec that matched
// (hook.SpecProject), else key, baseline.Key(cwd) as the caller read it.
// Resolving against the cwd
// instead fails from a subdirectory: a path named from the project root
// does not exist there, and an anchored rule never covers what is named.
func promptRoot(sp *spec.Compiled, cwd, key string) string {
	if sp != nil {
		for i := len(sp.Sources) - 1; i >= 0; i-- {
			if path := sp.Sources[i].Path; strings.HasSuffix(filepath.ToSlash(path), repoSpecSuffix) {
				return hook.SpecProject(path)
			}
		}
	}
	// Not cmp.Or(key, baseline.Key(cwd)): that would run the walk and its
	// symlink evaluation on every call, key set or not.
	if key != "" {
		return key
	}
	return baseline.Key(cwd)
}

// resolvedDir is dir with its symlinks evaluated, as baseline.Key resolves a
// cwd, else dir as given, so the cwd and the project root compare in one form.
func resolvedDir(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return dir
}
