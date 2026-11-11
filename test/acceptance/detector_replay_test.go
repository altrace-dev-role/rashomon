package acceptance

// The replay of one fixture through the built binary, and the snapshot of
// what it printed. This file is in the default build so that an acceptance
// item can replay a single fixture and read everything it printed; the sweep
// over both corpora, the precision table and the snapshot comparison are in
// detector_replay_perf_test.go, behind the perf tag.
//
// A replay is one fresh machine per fixture. Turn by turn it sends the
// turn's calls (PreToolUse, then PostToolUse or PostToolUseFailure, or no
// ending), the Stop with the turn's final message, reads the digest the way
// the Stop read it, and only then sends the next prompt (UserPromptSubmit,
// `recap pending`): the next turn's prompt id, or after the last turn
// trailingPromptID. So a verdict is read from the record as the Stop left
// it, before anything the next prompt writes. No UserPromptSubmit opens the
// first turn: the detector fixtures were labelled on runs without one, and
// one there would move what they record.
//
// What is kept of each turn is everything rashomon put in front of the
// person or the model: the session start probe's output on the first turn,
// any output at a call, the Stop's, and the next prompt's, each decoded
// whole, with the digest-level detectors that fired. A hook's stderr is kept
// only beside a non-zero exit: Claude Code shows it for a failed hook only.
// The cost: a hook that writes to stderr and exits 0 changes nothing here,
// and a person running Claude Code in verbose mode would have seen it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/digest"
)

// replaySnapshot is the committed snapshot of every replay of both corpora.
var replaySnapshot = filepath.Join("testdata", "replay.golden")

// lineCorpus is the unlabelled runs' directory, relative to this package.
const lineCorpus = "../corpus/lines"

// replayEnv pins, for every hook a replay runs, what the runner's own
// environment would otherwise decide: CI set anywhere makes a session
// unattended and withholds notes, AI_AGENT names the client version the
// delivery floors are read against (a suite run inside Claude Code would
// carry its own), RASHOMON_HOLDOUT unset draws the holdout from each fresh
// store's random key, and CLAUDE_PROJECT_DIR and RASHOMON_STEER_ARM are
// operator settings a hook would inherit. Each of them changes what is
// printed, so without the pins the snapshot would hold one machine's
// environment. The client is Claude Code 2.1.280, the steer tests' (stAgent)
// and the committed capture's.
var replayEnv = []string{"CI=", stAgent, "CLAUDE_PROJECT_DIR=", "RASHOMON_HOLDOUT=off", "RASHOMON_STEER_ARM="}

// replayWatchEnv is the install's: a person at a terminal, so no agent
// signal, with the same operator settings cleared.
var replayWatchEnv = []string{"CI=", "AI_AGENT=", "CLAUDE_PROJECT_DIR=", "RASHOMON_HOLDOUT=off", "RASHOMON_STEER_ARM="}

// nextPromptText is the text of every UserPromptSubmit a replay sends. The
// prompt's words are not what a replay is about, and a fixed one keeps them
// from becoming a variable.
const nextPromptText = "Continue."

// printed is what one hook invocation put on stdout, under the label the
// snapshot gives it. unsent, when set, says why the event was not sent.
type printed struct {
	label  string
	res    result
	unsent string
}

// spoke reports output a reader would see: stdout, or a failed exit.
func (p printed) spoke() bool { return p.unsent == "" && (p.res.stdout != "" || p.res.exitCode != 0) }

// turnReplay is one replayed turn: what was printed around it, in order, and
// the digest read right after its Stop.
type turnReplay struct {
	promptID string
	// before is printed whether or not anything was said: the session start
	// on the first turn, and a capture's own prompt.
	before []printed
	calls  int
	// atCalls holds only the call events that printed something.
	atCalls []printed
	stop    printed
	next    printed
	// after is what a capture sends past its Stop: the session's end.
	after     []printed
	digest    digest.Digest
	rawDigest string
}

// fired is the digest-level detectors that fired on the turn, by their code
// identifiers and in the inventory's order.
func (tr turnReplay) fired() []string {
	var out []string
	for _, spec := range detectorInventory {
		if !spec.reportOnly && spec.fired(&tr.digest) {
			out = append(out, spec.name())
		}
	}
	return out
}

// judged is the turn as a label is held to it: the Stop's line and the
// digest. A Stop that exits non-zero or prints what is not JSON breaks
// recap's own contract (H-93), and no verdict is read past it.
func (tr turnReplay) judged(t *testing.T) fixtureReplay {
	t.Helper()
	r := fixtureReplay{digest: tr.digest, rawJSON: tr.rawDigest}
	if tr.stop.res.exitCode != 0 {
		t.Fatalf("recap: exit %d, stderr %q", tr.stop.res.exitCode, tr.stop.res.stderr)
	}
	if tr.stop.res.stdout != "" {
		var o recapOutput
		if err := json.Unmarshal([]byte(tr.stop.res.stdout), &o); err != nil {
			t.Fatalf("recap stdout is not JSON: %v\n%s", err, tr.stop.res.stdout)
		}
		r.line, r.spoke = o.SystemMessage, true
	}
	return r
}

// replayRun replays r in a fresh machine and returns its turns and the
// normaliser for that machine's paths.
func replayRun(t *testing.T, r fixtureRun) ([]turnReplay, replayNorm) {
	t.Helper()
	turns, norm, _ := replayRunEnv(t, r)
	return turns, norm
}

// replayRunEnv is replayRun with the machine it replayed in, for an item
// that reads the store the run left (a report) after the last prompt.
func replayRunEnv(t *testing.T, r fixtureRun) ([]turnReplay, replayNorm, *env) {
	t.Helper()
	e := newEnv(t)
	// HOME too is the test's own, so nothing a hook reads under a home
	// directory comes from the machine running the suite.
	e.fakeHome = t.TempDir()
	if res := e.watch(replayWatchEnv...); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	norm := newReplayNorm(e.home, map[string]string{
		e.home: "{store}", e.configDir: "{config}", e.cwd: "{cwd}", e.fakeHome: "{home}",
	})
	if r.Capture != "" {
		return []turnReplay{e.replayCapture(t)}, norm, e
	}
	dir := ""
	if len(r.Files) > 0 {
		dir = writeProject(t, r.Files)
		norm = norm.with(dir, projectToken)
		// Claude Code runs a hook in the project, and the session's start
		// probe names it as its cwd.
		e.cwd = dir
	}
	start := printed{label: "session start", unsent: "probe_start is false"}
	if r.probeStart() {
		start = printed{label: "session start", res: e.probe("start", testSession, replayEnv...)}
	}
	turns := r.turns()
	out := make([]turnReplay, 0, len(turns))
	n := 0
	for i, turn := range turns {
		tr := turnReplay{promptID: turn.PromptID}
		if i == 0 {
			tr.before = []printed{start}
		}
		for _, c := range turn.Calls {
			for range max(c.Repeat, 1) {
				n++
				tr.calls++
				tr.atCalls = append(tr.atCalls, e.replayCall(t, n, turn.PromptID, dir, c)...)
			}
		}
		tr.stop = printed{label: "stop", res: e.recap(replayStop(turn.FinalMessage, dir), replayEnv...)}
		tr.digest, tr.rawDigest = e.turnDigest(t, testSession, turn.FinalMessage)
		next := trailingPromptID
		if i+1 < len(turns) {
			next = turns[i+1].PromptID
		}
		res := e.run(replayPrompt(t, next, dir), replayEnv, append([]string{"recap", "pending"}, e.installArgs()...)...)
		tr.next = printed{label: "next prompt " + next, res: res}
		out = append(out, tr)
	}
	return out, norm, e
}

// writeProject makes the run's project: a git work tree's root (an empty
// .git, as gi1Project makes one) holding the fixture's files. The path is
// resolved, because macOS's temporary directory is behind a symlink and the
// payloads must name the directory the binary will find.
func writeProject(t *testing.T, files []projectFile) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// replayCall feeds the binary call n of the run as Claude Code would have:
// the PreToolUse payload, then the PostToolUse or PostToolUseFailure that
// answers it, or nothing for a call that has no ending. Both carry the
// turn's prompt id, as Claude Code sends them: the post's steer step reads
// it, and a turn is the calls under one. With no project the payloads name
// /tmp/project, the cwd every fixture was written against. It returns the
// events that printed.
func (e *env) replayCall(t *testing.T, n int, promptID, dir string, c fixtureCall) []printed {
	t.Helper()
	id := fmt.Sprintf("toolu_fx%04d", n)
	cwd, input := "/tmp/project", c.Input
	if dir != "" {
		cwd = dir
		input, _ = withProject(c.Input, dir).(map[string]any)
	}
	var out []printed
	keep := func(event string, res result) {
		if p := (printed{label: fmt.Sprintf("call %d %s %s", n, c.Tool, event), res: res}); p.spoke() {
			out = append(out, p)
		}
	}
	pre := defaultPayload()
	pre.ToolUseID, pre.ToolName, pre.ToolInput, pre.PromptID, pre.CWD = id, c.Tool, input, promptID, cwd
	keep("PreToolUse", e.hook(pre.build(t), replayEnv...))

	body := map[string]any{
		"session_id": testSession, "prompt_id": promptID, "transcript_path": "/tmp/transcripts/sess-1.jsonl",
		"cwd": cwd, "permission_mode": "default", "tool_name": c.Tool, "tool_input": input, "tool_use_id": id,
	}
	switch c.Outcome {
	case "ok":
		body["hook_event_name"] = "PostToolUse"
		body["tool_response"] = map[string]any{"output": c.Output}
		if c.Tool == "Bash" {
			body["tool_response"] = map[string]any{"stdout": c.Output, "stderr": "", "interrupted": false}
		}
	case "failed":
		// A shell call fails as Claude Code sends it: "Exit code N" and then the
		// command's output. Another tool's error is its own text.
		msg := c.Output
		if c.Tool == "Bash" {
			msg = fmt.Sprintf("Exit code %d", max(c.ExitCode, 1))
			if c.Output != "" {
				msg += "\n" + c.Output
			}
		} else if msg == "" {
			msg = "tool failed"
		}
		body["hook_event_name"] = "PostToolUseFailure"
		body["error"], body["is_interrupt"], body["duration_ms"] = msg, false, 30
	case "interrupted":
		// The failure event with is_interrupt set; what it says is the
		// fixture's, since no reader of the record keeps it.
		msg := c.Output
		if msg == "" {
			msg = "interrupted"
		}
		body["hook_event_name"] = "PostToolUseFailure"
		body["error"], body["is_interrupt"], body["duration_ms"] = msg, true, 30
	default:
		return out
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	keep(body["hook_event_name"].(string), e.post(string(b), replayEnv...))
	return out
}

// withProject is a copy of v with projectToken in every string replaced by
// the project directory.
func withProject(v any, dir string) any {
	switch v := v.(type) {
	case string:
		return strings.ReplaceAll(v, projectToken, dir)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			out[k] = withProject(x, dir)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, x := range v {
			out[i] = withProject(x, dir)
		}
		return out
	}
	return v
}

// replayStop is the turn's Stop payload: stopPayload's, which names no cwd,
// for a run with no project, as every fixture was replayed before projects
// existed; with one, the project as its cwd.
func replayStop(final, dir string) string {
	if dir == "" {
		return stopPayload(testSession, final, false)
	}
	return stopEvent("Stop", final, dir)
}

// replayPrompt is the next prompt's UserPromptSubmit payload. Like the Stop
// it names the project as its cwd when there is one, and no cwd otherwise,
// so the next prompt reads the same project the Stop did.
func replayPrompt(t *testing.T, promptID, dir string) string {
	t.Helper()
	m := map[string]any{
		"session_id": testSession, "hook_event_name": "UserPromptSubmit", "transcript_path": "/tmp/transcripts/sess-1.jsonl",
		"permission_mode": "default", "prompt": nextPromptText, "prompt_id": promptID,
	}
	if dir != "" {
		m["cwd"] = dir
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// turnDigest reads the digest of the session's newest prompt with recorded
// calls, the one the Stop itself builds (it names no prompt either).
func (e *env) turnDigest(t *testing.T, session, final string) (digest.Digest, string) {
	t.Helper()
	res := e.digestRaw("--session", session, "--last-assistant-message", final)
	if res.exitCode != 0 {
		t.Fatalf("digest: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var d digest.Digest
	if err := json.Unmarshal([]byte(res.stdout), &d); err != nil {
		t.Fatalf("digest is not JSON: %v\n%s", err, res.stdout)
	}
	return d, res.stdout
}

// replayCapture replays the committed Claude Code capture as replayReal
// does, keeping what each event printed. The capture is one turn: its own
// session start and prompt, its calls, its Stop, after which the digest is
// read, and the session's end. It sends no next prompt, because its session
// ended.
func (e *env) replayCapture(t *testing.T) turnReplay {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(realFixtureDir, "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(e.configDir, "projects", "-private-tmp-rashomon-fixture-wd", realSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	sub := map[string][]string{
		"SessionStart": {"probe", "start"}, "SessionEnd": {"probe", "end"},
		"PreToolUse": {"hook"}, "PostToolUse": {"post"}, "PostToolUseFailure": {"post"},
		"Stop": {"recap"}, "UserPromptSubmit": {"recap", "pending"},
	}
	tr := turnReplay{next: printed{label: "next prompt", unsent: "the capture ends its session"}}
	callOf := map[string]int{}
	stopped := false
	for _, p := range realPayloads(t, transcript) {
		args, ok := sub[p.event]
		if !ok {
			t.Fatalf("no subcommand for captured event %q", p.event)
		}
		res := e.run(p.raw+"\n", replayEnv, append(append([]string{}, args...), e.installArgs()...)...)
		var head struct {
			PromptID string `json:"prompt_id"`
			Message  string `json:"last_assistant_message"`
		}
		if err := json.Unmarshal([]byte(p.raw), &head); err != nil {
			t.Fatal(err)
		}
		switch p.event {
		case "SessionStart":
			tr.before = append(tr.before, printed{label: "session start", res: res})
		case "UserPromptSubmit":
			tr.before = append(tr.before, printed{label: "prompt " + head.PromptID + " (it opens the turn)", res: res})
		case "PreToolUse", "PostToolUse", "PostToolUseFailure":
			if p.event == "PreToolUse" {
				tr.calls++
				callOf[p.toolUseID] = tr.calls
			}
			if c := (printed{label: fmt.Sprintf("call %d %s %s", callOf[p.toolUseID], p.tool, p.event), res: res}); c.spoke() {
				tr.atCalls = append(tr.atCalls, c)
			}
		case "Stop":
			if stopped {
				t.Fatal("premise: the capture is one turn, and it has a second Stop")
			}
			stopped = true
			tr.promptID = head.PromptID
			tr.stop = printed{label: "stop", res: res}
			tr.digest, tr.rawDigest = e.turnDigest(t, realSession, head.Message)
		case "SessionEnd":
			tr.after = append(tr.after, printed{label: "session end", res: res})
		}
	}
	if !stopped {
		t.Fatal("premise: the capture has no Stop")
	}
	return tr
}

// replayNorm turns one machine's paths into placeholders, and knows the
// temporary root every one of them sits under, so that a path it did not
// name is caught rather than committed.
type replayNorm struct {
	pairs [][2]string
	roots []string
}

// newReplayNorm names each path by its placeholder, in both spellings
// (macOS's temporary directory is reached through a symlink), with the
// temporary root the test's directories share taken from under.
func newReplayNorm(under string, names map[string]string) replayNorm {
	var n replayNorm
	root := filepath.Dir(under)
	n.roots = []string{root}
	if r, err := filepath.EvalSymlinks(root); err == nil && r != root {
		n.roots = append(n.roots, r)
	}
	for p, name := range names {
		n = n.with(p, name)
	}
	return n
}

// with is n with one more path named. The session start's brief prints a
// root too long for its line as "..." and the root's tail from a separator
// (brief.shown), and whether a temporary path is that long is the machine's
// (TMPDIR's length) and the test name's, so each such tail of a named path
// is named too, by the same placeholder as the whole.
func (n replayNorm) with(p, name string) replayNorm {
	spellings := []string{p}
	if r, err := filepath.EvalSymlinks(p); err == nil && r != p {
		spellings = append(spellings, r)
	}
	pairs := slices.Clone(n.pairs)
	for _, s := range spellings {
		pairs = append(pairs, [2]string{s, name})
		for i := 1; i < len(s); i++ {
			if s[i] == filepath.Separator {
				pairs = append(pairs, [2]string{"..." + s[i:], name})
			}
		}
	}
	// Longest first, so a path is never half replaced by one it starts with.
	sort.SliceStable(pairs, func(i, j int) bool { return len(pairs[i][0]) > len(pairs[j][0]) })
	return replayNorm{pairs: pairs, roots: n.roots}
}

// apply replaces every named path by its placeholder.
func (n replayNorm) apply(s string) string {
	for _, p := range n.pairs {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

// leak reports a temporary path that survived apply: a directory the
// normaliser was not told about reached the output, and a snapshot holding
// it would differ on the next machine.
func (n replayNorm) leak(s string) (string, bool) {
	for _, r := range n.roots {
		if strings.Contains(s, r) {
			return r, true
		}
	}
	return "", false
}

// renderRun is the run's snapshot blocks, one per turn, normalised. A
// temporary path left in a block fails the test that rendered it.
func renderRun(t *testing.T, name string, turns []turnReplay, norm replayNorm) []string {
	t.Helper()
	blocks := make([]string, 0, len(turns))
	for i, tr := range turns {
		b := norm.apply(renderTurn(name, i, len(turns), tr))
		if root, ok := norm.leak(b); ok {
			t.Errorf("%s turn %d: a temporary path under %s reached the snapshot; name it in replayRun's normaliser:\n%s", name, i+1, root, b)
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// renderTurn is one turn's block, before normalising:
//
//	=== <corpus>/<fixture> turn <i> of <n>
//	prompt: <the turn's prompt id>
//	digest read: <the prompt id the digest judged, or none>
//	fired: <the digest-level detectors that fired, or none>
//	session start:                    the first turn only
//	calls: <sent>; call events that printed: <m>
//	call <k> <tool> <hook event>:     each of the m, in order
//	stop:
//	next prompt <prompt id>:
//
// and under each event either "nothing", "not sent: <why>", or its stdout's
// JSON leaves by dotted key path, a string as one "| " row per line. A call
// event that printed nothing is counted on the calls line and not listed.
// The digest reads the newest prompt that recorded a call, as the Stop does,
// so a turn that made no call is judged on an earlier turn's prompt, and
// "digest read" says which.
func renderTurn(name string, i, n int, tr turnReplay) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s turn %d of %d\n", name, i+1, n)
	fmt.Fprintf(&b, "prompt: %s\n", orNone(tr.promptID))
	fmt.Fprintf(&b, "digest read: %s\n", orNone(tr.digest.PromptID))
	fmt.Fprintf(&b, "fired: %s\n", orNone(strings.Join(tr.fired(), ", ")))
	for _, p := range tr.before {
		renderPrinted(&b, p)
	}
	fmt.Fprintf(&b, "calls: %d; call events that printed: %d\n", tr.calls, len(tr.atCalls))
	for _, p := range tr.atCalls {
		renderPrinted(&b, p)
	}
	renderPrinted(&b, tr.stop)
	renderPrinted(&b, tr.next)
	for _, p := range tr.after {
		renderPrinted(&b, p)
	}
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// renderPrinted writes one event: why it was not sent, or its exit and
// stderr when it failed, and its stdout decoded as JSON, key by key, or as
// text when it is not JSON. Nothing printed says so: an absence renders as
// an absence.
func renderPrinted(b *strings.Builder, p printed) {
	if p.unsent != "" {
		fmt.Fprintf(b, "%s: not sent: %s\n", p.label, p.unsent)
		return
	}
	fmt.Fprintf(b, "%s:\n", p.label)
	if p.res.exitCode != 0 {
		fmt.Fprintf(b, "  exit: %d\n", p.res.exitCode)
		renderText(b, "stderr", p.res.stderr)
	}
	if p.res.stdout == "" {
		if p.res.exitCode == 0 {
			b.WriteString("  nothing\n")
		}
		return
	}
	var v any
	if err := json.Unmarshal([]byte(p.res.stdout), &v); err != nil {
		renderText(b, "stdout, not JSON", p.res.stdout)
		return
	}
	renderJSON(b, "", v)
}

// renderJSON writes v's leaves under their dotted key paths, keys sorted.
func renderJSON(b *strings.Builder, path string, v any) {
	switch v := v.(type) {
	case map[string]any:
		if len(v) == 0 {
			fmt.Fprintf(b, "  %s: {}\n", leafKey(path))
			return
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			renderJSON(b, joinPath(path, k), v[k])
		}
	case []any:
		if len(v) == 0 {
			fmt.Fprintf(b, "  %s: []\n", leafKey(path))
			return
		}
		for i, x := range v {
			renderJSON(b, joinPath(path, fmt.Sprint(i)), x)
		}
	case string:
		renderText(b, leafKey(path), v)
	default:
		j, _ := json.Marshal(v)
		fmt.Fprintf(b, "  %s: %s\n", leafKey(path), j)
	}
}

func joinPath(path, k string) string {
	if path == "" {
		return k
	}
	return path + "." + k
}

// leafKey is a leaf's key path; stdout that is a JSON scalar, not an
// object, has none and is named for the stream.
func leafKey(path string) string {
	if path == "" {
		return "stdout"
	}
	return path
}

// renderText writes a string value one line to a "|" row, so leading
// spaces, empty lines and a trailing newline all show; an empty string is
// written as "".
func renderText(b *strings.Builder, key, s string) {
	if s == "" {
		fmt.Fprintf(b, "  %s: \"\"\n", key)
		return
	}
	fmt.Fprintf(b, "  %s:\n", key)
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			b.WriteString("    |\n")
			continue
		}
		fmt.Fprintf(b, "    | %s\n", line)
	}
}

// snapshotPreamble heads the snapshot file. It is read back as part of it,
// so a change to it is a diff like any other.
const snapshotPreamble = `# What rashomon printed on every replay of test/corpus/detectors and
# test/corpus/lines, one block per fixture and turn, on the code that wrote it.
# Rewrite it only for a change you mean, and read the diff:
#   env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT go test -tags perf ./test/acceptance \
#       -run '^TestDetectorPrecision_Table$' -count=1 -update
# systemMessage is what Claude Code shows the person; additionalContext is
# what it gives the model. fired is read from the digest, never the wording.
# Paths: {store} the store, {config} Claude Code's configuration directory,
# {home} HOME, {cwd} the hooks' directory, {project} the run's project.
`

// parseSnapshot splits a snapshot into its preamble and its blocks, by
// header line. A header given twice is an error: one of the two would hide.
func parseSnapshot(text string) (preamble string, blocks map[string]string, order []string, err error) {
	blocks = map[string]string{}
	var cur *strings.Builder
	var head string
	var pre strings.Builder
	flush := func() error {
		if cur == nil {
			return nil
		}
		if _, dup := blocks[head]; dup {
			return fmt.Errorf("block %q is given twice", head)
		}
		blocks[head] = strings.TrimRight(cur.String(), "\n") + "\n"
		order = append(order, head)
		return nil
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(line, "=== ") {
			if err := flush(); err != nil {
				return "", nil, nil, err
			}
			head, cur = strings.TrimSuffix(line, "\n"), &strings.Builder{}
		}
		if cur == nil {
			pre.WriteString(line)
			continue
		}
		cur.WriteString(line)
	}
	if err := flush(); err != nil {
		return "", nil, nil, err
	}
	return strings.TrimRight(pre.String(), "\n") + "\n", blocks, order, nil
}

// formatSnapshot is the file parseSnapshot reads: the preamble, then each
// block after a blank line.
func formatSnapshot(blocks []string) string {
	var b strings.Builder
	b.WriteString(snapshotPreamble)
	for _, blk := range blocks {
		b.WriteString("\n")
		b.WriteString(blk)
	}
	return b.String()
}

// blockHeader is a block's first line.
func blockHeader(block string) string {
	head, _, _ := strings.Cut(block, "\n")
	return head
}

// snapshotProblems compares the blocks a run produced with the committed
// ones. complete says every fixture was replayed, so a committed block the
// run did not produce is a fixture that is gone; a run filtered with -run
// cannot say that, and is compared on its own blocks only.
func snapshotProblems(got []string, want map[string]string, complete bool) []string {
	var out []string
	produced := map[string]bool{}
	for _, blk := range got {
		head := blockHeader(blk)
		produced[head] = true
		w, ok := want[head]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s: not in the snapshot\n--- got\n%s", head, blk))
		case w != blk:
			out = append(out, fmt.Sprintf("%s: differs from the snapshot\n--- got\n%s--- want\n%s", head, blk, w))
		}
	}
	if complete {
		var gone []string
		for head := range want {
			if !produced[head] {
				gone = append(gone, head)
			}
		}
		sort.Strings(gone)
		for _, head := range gone {
			out = append(out, fmt.Sprintf("%s: in the snapshot, and no fixture produced it", head))
		}
	}
	return out
}

// readSnapshot reads the committed snapshot's blocks.
func readSnapshot(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(replaySnapshot)
	if err != nil {
		t.Fatalf("%v: write it with the perf-tagged table and -update", err)
	}
	pre, blocks, _, err := parseSnapshot(string(b))
	if err != nil {
		t.Fatalf("%s: %v", replaySnapshot, err)
	}
	if pre != snapshotPreamble {
		t.Errorf("%s: the preamble differs from snapshotPreamble:\n--- got\n%s--- want\n%s", replaySnapshot, pre, snapshotPreamble)
	}
	return blocks
}

// loadDetectorFixtures reads every fixture of one detector, sorted by name.
// A detector with no directory has none, which is not an error.
func loadDetectorFixtures(t *testing.T, id string) (names []string, fixtures []detectorFixture) {
	t.Helper()
	for _, name := range corpusFiles(t, filepath.Join(detectorCorpus, id)) {
		b, err := os.ReadFile(filepath.Join(detectorCorpus, id, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		f, err := parseFixture(id, b)
		if err != nil {
			t.Fatalf("%s/%s: %v", id, name, err)
		}
		names = append(names, name)
		fixtures = append(fixtures, f)
	}
	return names, fixtures
}

// loadLineFixtures reads every fixture of the line corpus, sorted by name.
func loadLineFixtures(t *testing.T) (names []string, fixtures []lineFixture) {
	t.Helper()
	names = corpusFiles(t, lineCorpus)
	if len(names) == 0 {
		t.Fatalf("premise: %s holds no fixture", lineCorpus)
	}
	for _, name := range names {
		fixtures = append(fixtures, readLineFixture(t, name))
	}
	return names, fixtures
}

// corpusFiles is the fixture names of one directory, without .json, in
// ReadDir's sorted order. A directory that does not exist has none.
func corpusFiles(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ent := range ents {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			t.Fatalf("%s/%s: a fixture directory holds .json fixtures and nothing else", dir, ent.Name())
		}
		names = append(names, strings.TrimSuffix(ent.Name(), ".json"))
	}
	return names
}

// readLineFixture reads one fixture of the line corpus by name.
func readLineFixture(t *testing.T, name string) lineFixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(lineCorpus, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parseLineFixture(b)
	if err != nil {
		t.Fatalf("lines/%s: %v", name, err)
	}
	return f
}

// TestReplaySnapshot_OneFixtureReplaysToItsCommittedBlocks replays one line
// fixture in the default build and holds what it printed to its blocks of
// the committed snapshot, so every push replays one run end to end through
// the helpers the corpus sweep uses (that sweep runs only with -tags perf),
// and a change to what a green turn prints fails here before it reaches the
// perf job. The control is the run whose line every other line fixture is
// read against.
func TestReplaySnapshot_OneFixtureReplaysToItsCommittedBlocks(t *testing.T) {
	const name = "control-edit-green-test-done"
	turns, norm := replayRun(t, readLineFixture(t, name).fixtureRun)
	got := renderRun(t, "lines/"+name, turns, norm)
	// The positive twin of an empty comparison: the run produced the one
	// block a one-turn run has, and its Stop printed a line, read the way a
	// label reads it.
	if j := turns[0].judged(t); len(got) != 1 || !j.spoke || !strings.Contains(got[0], "\nstop:\n  systemMessage:\n") {
		t.Fatalf("premise: the control produced %d blocks and its Stop printed %q", len(got), turns[0].stop.res.stdout)
	}
	for _, p := range snapshotProblems(got, readSnapshot(t), false) {
		t.Error(p)
	}
}
