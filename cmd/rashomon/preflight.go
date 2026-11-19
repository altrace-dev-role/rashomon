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
	"github.com/altrace-dev-role/rashomon/internal/recap"
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
	// unverified is the previous Stop's done verdict, taken from recap.json by
	// promptNotes before the preflight (recap.TakeUnverified): true once per
	// flagged Stop of this session, so a prompt that routes nothing still
	// consumes it, and the note is not read for at every prompt (card c4).
	unverified bool
	// framed is whether the host framed the prompt, nothing typed in it
	// (harness.Framed), set by promptNotes: a peer's or a job's text is not
	// the user's request, so such a prompt names no path and is told no
	// done verdict, though it takes one.
	framed bool
	// owed is what the session's last turn end owes this prompt
	// (recap.Turn), which preflightNote routes beside its note.
	owed recap.Owed
	// indexOverCap is set by preflightNote when the graph brief found the
	// repository index over its size cap (graphBriefWith), for the turn's
	// once-per-session notice (promptTurnNotes).
	indexOverCap bool
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
// the request's task-preflight note and a lesson from proj (promptFindings),
// and what the last turn end owes this prompt (owedFinding, card
// GATE-INFER1, W5), which ranks above both (claim-vs-record). The zero
// Output when there is no run to log it under (no session_id, H-107), no cwd
// to resolve against, or nothing new to say. The repository graph's brief
// (graphBrief, GRAPH-SLOT1) is read first, so a request whose words select
// code has a note whatever else it names; it routes as a task-preflight
// finding of its own (graphFinding). A request that names no path and no
// identifier, with no lesson to give, nothing owed and no graph under its
// cwd, stops after that one index lookup (none for a fire that holds only
// host frames), before any other read and before the steer environment is
// built.
// proj is the caller's read (readPromptProject). A session's first prompt
// finds its run empty, so the note decides at after_seq 0, which CheckOn
// admits at UserPromptSubmit and SessionStart only. The first prompt after
// a resume its start armed also routes the continuity briefing
// (resumeBriefing, D-c), which takes the call's note on its own channel.
func preflightNote(st *store.Store, run *store.Run, p *pendingPayload, proj promptProject, id harness.Identity, now time.Time) (steer.Output, steer.Client) {
	if run == nil || p.CWD == "" {
		return steer.Output{}, steer.Client{}
	}
	named := !p.framed && intent.Names(p.Prompt, p.PromptCut)
	done := p.unverified
	brief, briefed := resumeBriefing(st, run, p.SessionID, p.Harness.TranscriptPath)
	storeRoot := ""
	if st != nil {
		storeRoot = st.Root()
	}
	specOf := onceSpec(st, run, p.CWD)
	scoped := promptScope{st: st}
	// A fire that holds only host frames (p.framed) reads no graph: it loads
	// no index and writes no evidence file, unit brief or conflict base over
	// what the turn's typed fire wrote. Build's notice refuses the same
	// fires under the idf and tf scorers, so no brief they would send is lost.
	gb, graphed := indexBrief{}, false
	if !p.framed {
		gb, graphed = graphBriefWith(storeRoot, p.CWD, p.SessionID, p.PromptID, p.Prompt, p.PromptCut, specOf, scoped.get)
		p.indexOverCap = gb.OverCap
	}
	if !named && !proj.lessons.Any() && !done && p.owed.Seq <= 0 && !briefed && !graphed {
		return steer.Output{}, steer.Client{}
	}
	env := steerEnv(id, now, st.Root())
	sp := specOf()
	hook.NoteSpecFound(st, p.SessionID, p.PromptID, p.CWD, env.ProjectDir, env.ManagedDir, sp)
	p.specNoted = true
	lines, _, _ := st.ReadSteer(p.SessionID)
	var in *intent.Intent
	var digest func(string) string
	scope := scoped.get()
	// At record scope v4 the steer line holds each named path as a keyed
	// digest and the delivered set compares digests; the note that prints
	// is the same at either scope. The graph brief's record form follows
	// the same rule, and at v4 the brief wrote no evidence file
	// (writeGraphEvidence).
	if scope != store.RecordScopeV5 && (named || graphed) {
		key := st.Key()
		digest = func(path string) string { return intent.PathDigest(key, path) }
	}
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
		resolved := intent.Resolve(p.Prompt, p.PromptCut, intent.Project{Root: root, Cwd: cwd, Spec: sp, Index: idx, NowMS: env.Now.UnixMilli(),
			Symbols: promptSymbols(root)})
		in = &resolved
	}
	found := promptFindings(in, proj.lessons, lines, run.LastSeq(), digest)
	if f, ok := graphFinding(gb, keyGraph, run.LastSeq(), digest); graphed && ok {
		found = append(found, f)
	}
	if f, ok := owedFinding(p.owed, run.LastSeq(), lines); ok {
		found = append(found, f)
	}
	if briefed {
		found = append(found, brief)
	}
	if len(found) == 0 && !done {
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
	if f, ok := doneFinding(run.LastSeq()); ok && done && !withheld(&ctx) {
		found = append(found, f)
	}
	if len(found) == 0 {
		return steer.Output{}, steer.Client{}
	}
	return steer.Route(&ctx, found), ctx.Client
}

// printTurn prints the catch-up's one stdout object: the preflight note's
// Output with the catch-up line, when there is one, and then the turn's
// once-per-session notices (tn), each joined to its systemMessage in that
// order. st, when not nil, is flushed first, so the note's steer line is
// durable before the note prints (REC-34); a caller that already flushed
// passes nil. A failed flush prints nothing. A notice's marker is taken
// before the print and given back when nothing prints, on a failed flush or
// a refused Output, so the next prompt says it.
func printTurn(st *store.Store, stdout io.Writer, note steer.Output, c steer.Client, line string, tn turnNotes) error {
	notices, taken := tn.take()
	if note == (steer.Output{}) && line == "" && notices == "" {
		return nil
	}
	if st != nil {
		if err := st.Flush(); err != nil {
			tn.giveBack(taken)
			return err
		}
	}
	for _, l := range [...]string{line, notices} {
		switch {
		case l == "":
		case note.SystemMessage == "":
			note.SystemMessage = l
		default:
			note.SystemMessage += "\n" + l
		}
	}
	// Route withheld any channel the client cannot carry, so an error here
	// is a contract break: print nothing.
	b, err := note.MarshalFor(string(steer.EventUserPromptSubmit), c)
	if err != nil || b == nil {
		tn.giveBack(taken)
		return nil
	}
	_, _ = stdout.Write(b)
	return nil
}

// The turn's once-per-session notices (READ-SAFE1a c5 and D7): user-only
// lines of fixed words, with no path and no text of an error. The manifest
// line names one class of its refusal.
const (
	indexCapNotice = "rashomon: this repository's index is over its size cap, so prompts get no brief from it; " +
		"run rashomon graph index to write it again. Shown once per session."
	manifestNoticeHead = "rashomon: the project's manifest file was not read ("
	manifestNoticeTail = "), so imports in the files edited are not mapped. Shown once per session."
	manifestNotRegular = manifestNoticeHead + "not a regular file" + manifestNoticeTail
	manifestReadFailed = manifestNoticeHead + "read failed" + manifestNoticeTail
)

// turnNotes is the turn's once-per-session notices, which printTurn joins
// after the catch-up line in this order: the repository index over its size
// cap, as the prompt's graph brief found it (pendingPayload.indexOverCap),
// then the manifest notice an edit's overlay left owed
// (store.OweManifestNotice). The zero value has none.
type turnNotes struct {
	st       *store.Store
	session  string
	indexCap bool
	manifest bool
}

// promptTurnNotes is the notices p's prompt may print: the index cap as the
// preflight found it, and the manifest notice when its owed marker is
// present, a stat of that one marker on every prompt. The off arm withholds
// every hook print (armOff), these lines too, and takes no marker.
func promptTurnNotes(st *store.Store, p *pendingPayload, root string) turnNotes {
	if st == nil || p.SessionID == "" {
		return turnNotes{}
	}
	t := turnNotes{st: st, session: p.SessionID, indexCap: p.indexOverCap,
		manifest: st.NoticeTaken(p.SessionID, store.FileNoticeManifestOwed)}
	if t.due() && armOff(root) {
		return turnNotes{}
	}
	return t
}

// due reports whether t has a notice to try.
func (t turnNotes) due() bool { return t.indexCap || t.manifest }

// take takes the printed marker of each notice t has that the session has
// not had (store.TakeNotice), and returns their lines, one per line, and the
// markers it took. The manifest line names the failed read when the class
// marker is present, else the file that is not regular.
func (t turnNotes) take() (string, []string) {
	var lines, taken []string
	if t.indexCap && t.st.TakeNotice(t.session, store.FileNoticeIndexCap) {
		lines, taken = append(lines, indexCapNotice), append(taken, store.FileNoticeIndexCap)
	}
	if t.manifest && t.st.TakeNotice(t.session, store.FileNoticeManifest) {
		line := manifestNotRegular
		if t.st.NoticeTaken(t.session, store.FileNoticeManifestReadFailed) {
			line = manifestReadFailed
		}
		lines, taken = append(lines, line), append(taken, store.FileNoticeManifest)
	}
	return strings.Join(lines, "\n"), taken
}

// giveBack returns the markers take took, for a print that did not happen.
func (t turnNotes) giveBack(taken []string) {
	for _, m := range taken {
		t.st.ReturnNotice(t.session, m)
	}
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

// onceSpec is preflightSpec under specDirs (steerEnv's ProjectDir and
// ManagedDir), read on first use and kept, so the conflict base and the
// prompt's note read one discovery.
func onceSpec(st *store.Store, run *store.Run, cwd string) func() *spec.Compiled {
	var sp *spec.Compiled
	read := false
	return func() *spec.Compiled {
		if !read {
			projectDir, managedDir := specDirs()
			sp, read = preflightSpec(st, run, cwd, projectDir, managedDir), true
		}
		return sp
	}
}

// readPromptScope is promptScope's one read of the store's record scope, a
// variable so a test counts the reads a prompt makes (TestPromptScope_*).
var readPromptScope = hook.PromptRecordScope

// promptScope is the store's record scope as the prompt path reads it,
// hook.PromptRecordScope read on first use and kept (get), so the evidence
// file's gate (writeGraphEvidence) and the note's record form read one
// marker, and a prompt that finds no index reads it only if the note goes
// on. preflightNote holds it as a value and hands its get down.
type promptScope struct {
	st    *store.Store
	scope string
	read  bool
}

// get is the record scope, read on the first call; "" with no store, which
// is not v5.
func (s *promptScope) get() string {
	if !s.read {
		if s.st != nil {
			s.scope = readPromptScope(s.st)
		}
		s.read = true
	}
	return s.scope
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
