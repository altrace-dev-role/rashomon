package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The blast-radius note (card ARCH-INLINE1, PF-PL7): after an Edit, Write or
// MultiEdit of a non-test Go file, the model is told which other packages
// the platform graph shows calling into the edited file's package, so it
// learns what the edit may break while it can still act. The graph's facts
// are read through BlastPort, which cmd wires to internal/platform: the hook
// links no reader of pgraph's files, as the Stop's platform read is cmd's
// (recap_stop.go). Two gates keep the note to news (A2):
//   - read: a marker per package and log version (store.TakeBlastRead),
//     taken before the read, so the log is read about once per package each
//     time pgraph syncs, and returned when the note did not go out;
//   - print: the note's key carries a digest of the caller set, and it is
//     given only when that digest differs from the last one the session's
//     steer lines record as given for the package, so a re-sync that left
//     the callers as they were says nothing.

// BlastSite is where a Go edit lands for the read: the sync root, the edited
// file's package (its import path) and the sha256 of the log the walk file
// vouches for (platform.Site).
type BlastSite struct {
	Root, Package, Log string
}

// BlastCaller is one other package with a PROVEN call into the site's
// package: its import path, its directory relative to the root ("." for the
// root) and whether that directory holds a test file (platform.Caller).
type BlastCaller struct {
	Package, Dir string
	Tests        bool
}

// BlastPort is the read's port. Locate is the site of an edit of file under
// root, false where no sync vouches for a log or the file is in no package;
// Callers reads the site's callers, fresh ones only, sorted by package. A
// port with either func nil reads nothing.
type BlastPort struct {
	Locate  func(root, file string) (BlastSite, bool)
	Callers func(site BlastSite) ([]BlastCaller, error)
}

// blastDraft is a call's blast-radius finding and the read marker it took.
type blastDraft struct {
	f      steer.Finding
	marker string
}

// blast is the call's blast-radius finding, or false: a success event's
// Edit, Write or MultiEdit that landed, of a .go file under a Git root, at
// record scope v5 (the note names directories in the clear, which v4
// withholds), with a port wired. ctx is the call's steer context, built here
// from contextOf when the judge built none (ok false). A context whose note
// could reach no one (steering off, no consent, no prompt) reads nothing.
// The read marker is kept when the read answers nothing new (an error, no
// caller, none it can name, the callers last given): that log's answer
// stands until the next sync.
func (p *Post) blast(env SteerEnv, ctx *steer.Context, ok bool) (blastDraft, bool) {
	if env.Blast.Locate == nil || env.Blast.Callers == nil || !p.scopeV5 || p.landedSeq <= 0 ||
		p.event != string(steer.EventPostToolUse) || !editTool(p.toolName) {
		return blastDraft{}, false
	}
	file := shape.DeclaredPath(p.toolName, p.toolInput)
	if file == nil || !strings.HasSuffix(*file, ".go") {
		return blastDraft{}, false
	}
	root := blastRoot(p.cwd)
	if root == "" {
		return blastDraft{}, false
	}
	site, found := env.Blast.Locate(root, filepath.Join(resolved(filepath.Dir(*file)), filepath.Base(*file)))
	if !found {
		return blastDraft{}, false
	}
	if !ok {
		*ctx = p.contextOf(env, verify.SteerView{ProjectRoot: root})
	}
	if ctx.SteerOff || ctx.ConfigUnreadable || !ctx.Consent || ctx.PromptID == nil {
		return blastDraft{}, false
	}
	marker := blastMarker(p.st.Key(), site)
	if !p.st.TakeBlastRead(p.sessionID, marker) {
		return blastDraft{}, false
	}
	callers, err := env.Blast.Callers(site)
	if err != nil || len(callers) == 0 {
		return blastDraft{}, false
	}
	digest := callersDigest(callers)
	lines, _, _ := p.st.ReadSteer(p.sessionID)
	if lastGiven(lines, site.Package) == digest {
		return blastDraft{}, false
	}
	m := blastMessage(callers)
	if m.Text() == "" {
		return blastDraft{}, false
	}
	f := steer.Finding{Pattern: pattern.BlastRadius, Key: site.Package + "@" + digest, MaxLevel: steer.LevelNudge,
		StopForm: steer.StopFormNone, Message: m, AfterSeq: p.landedSeq, AgentID: p.agentID}
	f.Priority, _ = pattern.Priority(pattern.BlastRadius)
	return blastDraft{f: f, marker: marker}, true
}

// blastRoot is the Git root the read looks under, "" for none: baseline.Key
// of cwd, with cwd resolved here so that a cwd not on disk, which holds no
// sync to read, stops before KeyOf walks up to / for a .git. Through
// baseline.Key, that walk was 33 of the step's 35 allocations a call on the
// BenchmarkRunPost/edit_314k row, whose cwd is not on disk (se3-j10).
func blastRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	dir, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return ""
	}
	return gitRoot(baseline.KeyOf(dir))
}

// editTool reports the tools whose input names the one file they change,
// the edits the note follows (NotebookEdit writes no .go file).
func editTool(name string) bool {
	return name == "Edit" || name == "Write" || name == "MultiEdit"
}

// settleBlast returns the read marker of a blast-radius note that did not
// print to the agent (a correction outranked it, the prompt's budget was
// spent, the append failed), so the package's next edit reads and offers it
// again. A held-out session or the off arm withholds by design: its line is
// the decision, and the marker stays.
func (p *Post) settleBlast(d blastDraft, ctx *steer.Context, out steer.Output) {
	if out.AdditionalContext == d.f.Message.Text() || ctx.Holdout || ctx.Arm == steer.ArmOff {
		return
	}
	p.st.ReturnBlastRead(p.sessionID, d.marker)
}

// blastMarker is the read marker's name for site: store.FileBlastReadPrefix
// and 16 hex digits of an HMAC under the store's key over the package and
// the log's sha256, so the name holds no package path.
func blastMarker(key []byte, site BlastSite) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(site.Package + "\n" + site.Log))
	return store.FileBlastReadPrefix + hex.EncodeToString(m.Sum(nil))[:16]
}

// callersDigest is 12 hex digits of the sha256 over the callers' import
// paths in order, one per line: the print gate's unit, equal for an equal
// caller set whatever the log's version.
func callersDigest(callers []BlastCaller) string {
	h := sha256.New()
	for _, c := range callers {
		h.Write([]byte(c.Package + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// blastMessage is the note for callers: the count, the directories as a
// path list (a caller at the root is counted, never listed, since a path
// slot holds no "."), and how many hold a test file. The zero Message for a
// render the gate refuses, and where no directory is left to list
// (slot.ErrNoPathsLeft: every caller at the root, or left out by the deny
// list or the entropy test): a note naming no package is not said (the DE's
// condition (a)).
func blastMessage(callers []BlastCaller) render.Message {
	var dirs []slot.Path
	unlisted, tested := 0, 0
	for _, c := range callers {
		if c.Tests {
			tested++
		}
		if c.Dir == "." || c.Dir == "" {
			unlisted++
			continue
		}
		dirs = append(dirs, slot.Path(c.Dir))
	}
	m, _ := render.New(pattern.BlastRadius, render.BlastRadiusBinding{
		Count: slot.Count(len(callers)), Dirs: slot.NewPathList(dirs, unlisted), Tested: slot.Count(tested)})
	return m
}

// lastGiven is the caller digest of the last blast-radius line under pkg
// that the session gave: delivered (a held-out line keeps that state), or
// withheld by the off arm, whose decision gave it (as owedGiven reads a
// claim). "" for none.
// Lines are located with jsonscan, never decoded (RES-5).
func lastGiven(lines [][]byte, pkg string) string {
	var sp [4]jsonscan.Span
	last := ""
	for _, b := range lines {
		if jsonscan.Top(b, blastKeys(), sp[:]) != nil || spanText(b, sp[0]) != string(pattern.BlastRadius) {
			continue
		}
		state, by := spanText(b, sp[1]), spanText(b, sp[3])
		if state != string(steer.LineStateDelivered) && by != string(steer.SuppressedByArmOff) {
			continue
		}
		if d, ok := strings.CutPrefix(spanText(b, sp[2]), pkg+"@"); ok {
			last = d
		}
	}
	return last
}

// spanText is a string span's value, "" for any other span or an escape
// that does not decode.
func spanText(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}

// blastKeys is the one KeySet lastGiven reads a line with, built on its
// first call and never at init (RES-2).
func blastKeys() *jsonscan.KeySet {
	blastKeysOnce.Do(func() { blastKeysOnce.v = jsonscan.NewKeySet("pattern", "state", "key", "suppressed_by") })
	return blastKeysOnce.v
}

var blastKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}
