package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/history"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The replay harness (docs/autosteer-prescan.md, B2b).
//
// internal/replay rebuilds, from a Claude Code transcript, the hook events the
// live recorder would have received. This file feeds them to the live
// recorder's own entry points, cmdHook for a PreToolUse, cmdPost for the two
// PostToolUse events, cmdRecap for a Stop and `recap pending` for a prompt,
// into a store of its own, so that what imported history means is what live
// recording means. It derives nothing: every record in the replay's store was
// written by the code that writes a live one, and the verdict at a Stop is the
// StopView recapTurn computed (stopSeen), not a second evaluation of the same
// inputs. The harness lives in cmd/rashomon, by ruling, so it reuses recapTurn
// and stopView where they are; nothing was extracted. It is unexported and no
// subcommand calls it yet: the later `rashomon scan` will.
//
// # Isolation
//
// A replay writes to its world (replayWorld) and nowhere else. The world is
// two directories under a scratch directory the caller owns: a store root and a
// stand-in Claude Code configuration directory. The entry points find both
// through the process environment, so enter points the environment at them for
// the length of a replay and restores it after, and checks, before it feeds
// anything, that the entry points resolve to the world (store.DefaultRoot,
// settings.ConfigDir). A world is refused if its scratch directory overlaps the
// store root or the configuration directory the caller's environment names, so
// a replay cannot be aimed at the real ones by accident.
//
// What a replay therefore never touches: the caller's store, its project index
// (the verify/projects directory is under the store root), baseline, consent
// files and pause marker (all under the store root, or written by the world's
// own setup into the world's root), the Claude Code settings and its install
// entries (the world has settings of its own), any other file in the caller's
// home, and any terminal. The entry points are handed buffers for stdin,
// stdout and stderr, and nothing a replay calls prompts or opens a tty. What it
// prints (a recap line, a steer note) goes into those buffers, which are
// dropped unless a caller keeps them for comparison (replayConfig.KeepOutput).
// TestREPLAY7_ snapshots a real-looking store, configuration and home before
// and after a replay and asserts they are byte for byte the same.
//
// What a replay reads from outside its world is HOME, and only for what the
// recorder reads it for: the home that `cd ~/x` in a Bash command expands to
// (hook.startDirectory), an input to a digest and never a path written. It is
// kept as the caller's, deliberately: the location digest of a call that said
// `cd ~/proj` should be the one the live recorder on this machine would have
// computed. Nothing else is read from HOME, because the store root and the
// configuration directory are set.
//
// The process environment is global state, so a replay is serialised
// (replayMu) and is not safe to run beside other entry points in one process.
// A signal the hook entry points watch for (SIGINT, SIGTERM) is delivered to a
// channel they poll, not to the process's default handler, while one of them
// runs; a caller that wants Ctrl-C to stop a long replay must watch for it
// itself, between events.
//
// # Time
//
// The recorder stamps every record with its clock, and the live entry points
// read time.Now. A replay must not: a three-month-old transcript replayed with
// the present would make every gate "stale" and every index entry current. So
// the entry points read entryNow, which is time.Now except while a replay
// runs, and a replay sets it to the event's historical time (replay.Event.At,
// the timestamp of the transcript line the event is taken from) before each
// event, and to nothing else. The clock reads one time for all of an event's
// records. Every time.Now on the path was checked (TestREPLAY8_ lists the
// non-test sites in this package and in internal/):
//
//   - cmd/rashomon: the five entry points' sites are entryNow (hook.New,
//     hook.NewPost, steerEnv, recapTurn, the probes, the pause record, the
//     host-guard counters). The others, in watch, forget, consent_unit and the
//     reports, are commands a replay does not call.
//   - internal/recap/state.go removeStaleTemps compares a file's modification
//     time with now, to delete a temporary file a killed write left behind
//     under the replay's own root. It stamps no record.
//   - internal/store: lock-wait deadlines, and the install record's creation
//     time, which the world pins before the store opens (newReplayWorld).
//
// # Host guard
//
// Both guards, claudeHostGuard and recapHostGuard, decide whether the entry is
// Claude Code's to handle. They read CLAUDE_CODE_ENTRYPOINT and the
// payload's transcript_path (harness.Detect). The replay pins the first:
// CLAUDE_CODE_ENTRYPOINT is set to replayEntrypoint ("cli") whatever the
// caller's environment says, and CLAUDE_PROJECT_DIR is unset. So the guard
// identifies Claude Code, surface cli, from the environment and not from the
// path a caller names for the file: a transcript copied, or read from a
// backup, or from a directory that is not under a Claude Code configuration
// directory would otherwise be recorded as the unknown harness, stamped
// harness_id "unknown" on every declaration, which no live Claude Code session
// is. The transcript is by construction a Claude Code one (internal/history
// reads no other). What the pin costs is that the surface is the same for every
// session: the transcripts' own entrypoint key (the CLAUDE_CODE_ENTRYPOINT of
// the session that wrote them) could name the real one, and the one thing it
// changes is the steer note's channel (MirrorInBand), which is report-only
// here. TestREPLAY9_ shows a caller's environment does not leak in, and that
// the unpinned guard records the same payloads as the unknown harness.
//
// The other environment the path reads is pinned for the same reason, to the
// value that makes the replay a function of the transcript: no field holdout
// (RASHOMON_HOLDOUT=off; the draw assigns future sessions to an experiment arm,
// and a historical session has no arm), not CI, no AI_AGENT, and the default
// store cap.
//
// # What a replay result says
//
// A Stop's verdict is Verified, in the sense of the doc's revision 7, when the
// replay evaluated it and no gap lies at or before it (replay.Event.Taint):
// "consistent with the transcript". It is not "equivalent to what executed",
// because a PreToolUse hook could have rewritten a call's input and the
// transcript keeps the original; each session therefore carries the
// flagRewriteAbsent flag. A Stop with a gap at or before it is evaluated,
// since the recorder was fed what the replay has, and reported not Verified,
// with the first gap named; a Stop the transcript does not establish, or that
// cannot be fed, is not evaluated, with the reason.

// entryNow is the entry points' clock. It is time.Now, and a replay swaps it
// for the event's historical time while it runs (see "Time" above). Nothing
// else assigns it; the swap is made under replayMu.
var entryNow = time.Now

// stopSeen, when not nil, is called by recapTurn with the Stop view it
// computed, before it speaks. It is the replay's tap: the verdict a replay
// reports for a Stop is the view the recorder's own code computed from the
// store as it stood, not a second computation. It is nil outside a replay and
// recapTurn changes nothing for it.
var stopSeen func(event string, pending bool, sessionID string, v verify.StopView)

// replayMu serialises replays: the environment and the clock are global.
var replayMu sync.Mutex

// replayEntrypoint is the CLAUDE_CODE_ENTRYPOINT a replay runs under. See
// "Host guard".
const replayEntrypoint = "cli"

// Coverage flags a replay attaches to each session it fed. Each names a way
// in which the replayed record is not a live one. A Verified replayed Stop is
// "consistent with the transcript"; these say what the transcript could not
// show, so a consumer can leave such sessions out of a conclusion that needs it.
//
// All of a replay's sessions carry all of them today, which is the point of
// naming them: a flag a session does not carry is a claim the replay makes,
// and a later change that earns one (a transcript field that records the
// executed input, say) removes it from one place.
const (
	// flagRewriteAbsent: the declaration's input and the execution's input
	// both come from the transcript's one input, so the recorder's rewrite
	// check (declared digest against executed digest) cannot fire. A
	// PreToolUse hook that rewrote an input is invisible. No field of a
	// transcript was found that records the input actually executed: see
	// TestREPLAY16_.
	flagRewriteAbsent = "rewrite_evidence_absent"
	// flagDurationNull: execution records carry duration_ms null, the
	// recorder's value for an unmeasured call, by ruling (revision 7). Nothing
	// reads it back (TestREPLAY11_).
	flagDurationNull = "duration_ms_unmeasured"
	// flagProbeStartSynthesized: the transcript has no SessionStart, and a
	// run with no liveness marker reads coverage_unverified, which silences
	// every verdict (verify.recording). The replay therefore feeds the
	// recorder a SessionStart of its own at the session's first event, which
	// marks that the replay's recorder was alive for the session. It is
	// scaffolding, not evidence: it says nothing about the install that
	// recorded the transcript, and it must not raise any coverage of the
	// imported session to complete. A reader of the replay's records that
	// reads "recording complete" is reading the scaffolding (see
	// sessionBasis, which keeps this apart from the grade of the runs).
	flagProbeStartSynthesized = "probe_start_synthesized"
	// flagSessionEndUnknown: whether the session ended, and how, is unknown.
	// A transcript has no moment for SessionEnd, and a session killed or still
	// running never fired one; the transcript cannot tell the two from a clean
	// close, so the replay neither feeds one (it would claim a clean close) nor
	// calls the session killed. The end probe's records (run closed,
	// unterminated entries) are therefore absent, and the report reads the run
	// as not closed (TestREPLAY24_).
	flagSessionEndUnknown = "session_end_unknown"
)

// replayFlags is the flags every replayed session carries, in order.
var replayFlags = []string{flagRewriteAbsent, flagDurationNull, flagProbeStartSynthesized, flagSessionEndUnknown}

// sessionStartPayload is the SessionStart payload a replay feeds for a session:
// the keys the probe reads (session_id, cwd) and the ones the host guard
// reads (hook_event_name, transcript_path). Claude Code's own carries more
// (source, scratchpad_dir) which nothing reads (TestREPLAY17_).
type sessionStartPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
}

// replayIdentity is the store key and install id a world is pinned to. The
// digests the recorder writes (a gate's location, a quote) are HMACs under
// the key, so two worlds with one key give comparable records and two with
// different keys do not. The zero value is defaultReplayIdentity, a constant
// of this program: a replay's records are comparable with each other and with
// nothing recorded under a real install's key. A caller that needs them
// comparable with a live store's passes that store's key, which it alone can
// read.
type replayIdentity struct {
	KeyHex, InstallID string
}

var defaultReplayIdentity = replayIdentity{
	KeyHex:    strings.Repeat("52504c59", 8),
	InstallID: strings.Repeat("52504c59", 4),
}

// replayWorld is where a replay records: a store root and a configuration
// directory under a scratch directory, with an install the recorder takes as
// healthy.
type replayWorld struct {
	root, config string
	id           replayIdentity
	// clock is the time entryNow reads, set before each event.
	clock time.Time
	// afterClock, when set, is called each time an entry point reads the
	// clock, inside the entry point. It is a seam for tests, which use it to
	// make an entry point panic and to see the environment an entry point
	// sees; production code never sets it.
	afterClock func()
}

// newReplayWorld makes a world under scratch, which must not exist or must be
// empty, and must not overlap the store root or the configuration directory
// the process environment names. The world's install mirrors a healthy live
// one: a pinned key and install id, a consent record at the current scope with
// steering off, and a settings file with the eight hook entries, so coverage
// resolves them as present. The consent and the settings are the world's own;
// the caller's are neither read nor written.
func newReplayWorld(scratch string, id replayIdentity) (*replayWorld, error) {
	// The ambient check below reads the process environment, which another
	// replay may be holding set to its own world: a world is made between
	// replays, not during one.
	replayMu.Lock()
	defer replayMu.Unlock()
	if id == (replayIdentity{}) {
		id = defaultReplayIdentity
	}
	scratch, err := realPath(scratch)
	if err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(scratch); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("replay: scratch directory %s is not empty", scratch)
	}
	if err := refuseAmbient(scratch); err != nil {
		return nil, err
	}
	w := &replayWorld{root: filepath.Join(scratch, "store"), config: filepath.Join(scratch, "claude"), id: id}
	for _, d := range []string{w.root, w.config} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return nil, err
		}
	}
	// Pinned before the store opens, so Open reads them and mints neither.
	pins := map[string]string{
		store.FileInstallKey: id.KeyHex + "\n",
		installMetaFile: fmt.Sprintf(`{"install_id":%q,"created_at_unix_ms":0,"schema_version":%d}`+"\n",
			id.InstallID, store.SchemaVersion),
	}
	for name, body := range pins {
		if err := os.WriteFile(filepath.Join(w.root, name), []byte(body), 0o600); err != nil {
			return nil, err
		}
	}
	st, err := store.Open(w.root)
	if err != nil {
		return nil, err
	}
	if st.InstallID() != id.InstallID {
		return nil, errors.New("replay: the world's install id did not take")
	}
	if err := consent.Append(w.root, consent.Record{
		V: consent.RecordVersion, ScopeVersion: disclose.ScopeVersion, RecordScope: consent.RecordScopeV5,
		Steer: consent.SteerOn, Via: consent.ViaWatchYes, At: 0,
	}); err != nil {
		return nil, err
	}
	doc, err := settings.Parse(nil)
	if err != nil {
		return nil, err
	}
	if _, err := install.Apply(doc, install.Spec{Executable: "/usr/local/bin/rashomon", InstallID: id.InstallID}); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(w.config, "settings.json"), doc.Bytes(), 0o600); err != nil {
		return nil, err
	}
	return w, nil
}

// Root is the world's store root.
func (w *replayWorld) Root() string { return w.root }

// realPath is p made absolute with every symlink in its existing part
// resolved, so two spellings of one place compare equal.
func realPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest), nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest), nil
		}
		rest, p = filepath.Join(filepath.Base(p), rest), parent
	}
}

// within reports whether p is dir or lies under it.
func within(p, dir string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// refuseAmbient is the check that a world cannot be the caller's real store
// or configuration directory, nor inside one, nor contain one.
func refuseAmbient(scratch string) error {
	var ambient []string
	if r, err := store.DefaultRoot(); err == nil {
		ambient = append(ambient, r)
	}
	if c, err := settings.ConfigDir(); err == nil {
		ambient = append(ambient, c)
	}
	for _, a := range ambient {
		a, err := realPath(a)
		if err != nil {
			return err
		}
		if within(scratch, a) || within(a, scratch) {
			return fmt.Errorf("replay: scratch directory %s overlaps %s, which this process's environment uses", scratch, a)
		}
	}
	return nil
}

// replayEnv is every environment variable the entry points read, with the
// value a replay gives it; a nil pointer means unset. HOME is not here: see
// "Isolation".
func (w *replayWorld) replayEnv() map[string]*string {
	s := func(v string) *string { return &v }
	return map[string]*string{
		"RASHOMON_HOME":                  s(w.root),
		"CLAUDE_CONFIG_DIR":              s(w.config),
		"RASHOMON_MANAGED_SETTINGS_PATH": s(filepath.Join(w.config, "absent")),
		"CLAUDE_CODE_ENTRYPOINT":         s(replayEntrypoint),
		"RASHOMON_HOLDOUT":               s("off"),
		"CLAUDE_PROJECT_DIR":             nil,
		"AI_AGENT":                       nil,
		"CI":                             nil,
		"XDG_STATE_HOME":                 nil,
		"RASHOMON_STORE_CAP_BYTES":       nil,
	}
}

// replayRun is a replay in progress: the world entered, one event at a time.
type replayRun struct {
	w   *replayWorld
	cfg replayConfig
	// outputs are the entry points' outputs, kept only under cfg.KeepOutput.
	outputs []replayOutput
	// seen is the Stop view recapTurn handed stopSeen for the event being fed.
	seen *verify.StopView
}

// replayConfig configures a replay.
type replayConfig struct {
	// KeepOutput keeps what the entry points printed, in replayRun.outputs,
	// for a test to compare. Otherwise it is dropped as it is produced.
	KeepOutput bool
	// Ctx, when set, is checked before each event is fed: a replay whose
	// context is done stops with its error. The environment, the clock and the
	// tap are put back whatever stops a replay (with).
	Ctx context.Context
	// Observe, when set, is called with each event the walk yields and whether
	// it was fed, after it was; its error stops the replay.
	Observe func(e replay.Event, fed bool) error
}

// replayOutput is what one entry point returned and printed.
type replayOutput struct {
	Event, ToolUseID string
	Exit             int
	Stdout, Stderr   string
}

// replayStep is one hook invocation to feed: the event's name, the payload,
// and the time the recorder's clock reads while it handles it.
type replayStep struct {
	Event string
	Raw   []byte
	At    time.Time
}

// with enters the world for fn: the environment, the clock and the Stop tap are
// the world's until fn returns, and are restored whatever fn does: an error, a
// panic in an entry point, a cancelled context (every restore is deferred, and
// the mutex that makes two replays one after the other is released after them).
// The mutex is held for the whole of fn, so a second replay waits for the
// first to finish and put everything back; two never run at once
// (TestREPLAY26_).
func (w *replayWorld) with(cfg replayConfig, fn func(*replayRun) error) (err error) {
	replayMu.Lock()
	defer replayMu.Unlock()

	type saved struct {
		val string
		set bool
	}
	prev := map[string]saved{}
	for k := range w.replayEnv() {
		v, ok := os.LookupEnv(k)
		prev[k] = saved{v, ok}
	}
	defer func() {
		for k, p := range prev {
			if p.set {
				_ = os.Setenv(k, p.val)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}()
	for k, v := range w.replayEnv() {
		if v == nil {
			err = os.Unsetenv(k)
		} else {
			err = os.Setenv(k, *v)
		}
		if err != nil {
			return err
		}
	}
	// The check that the isolation took: where the entry points will write.
	if root, rerr := store.DefaultRoot(); rerr != nil || root != w.root {
		return fmt.Errorf("replay: the entry points would use store %q, not the world's %q", root, w.root)
	}
	if dir, derr := settings.ConfigDir(); derr != nil || dir != w.config {
		return fmt.Errorf("replay: the entry points would use configuration %q, not the world's %q", dir, w.config)
	}

	prevNow, prevSeen := entryNow, stopSeen
	entryNow = func() time.Time { return w.clock }
	defer func() { entryNow, stopSeen = prevNow, prevSeen }()
	if w.afterClock != nil {
		inner := entryNow
		entryNow = func() time.Time { w.afterClock(); return inner() }
	}

	r := &replayRun{w: w, cfg: cfg}
	stopSeen = func(event string, pending bool, _ string, v verify.StopView) {
		if event == install.EventStop && !pending {
			r.seen = &v
		}
	}
	return fn(r)
}

// feed hands one event to the entry point that handles it, as Claude Code
// would have run it, and returns the Stop view the recorder computed if the
// event was a Stop that produced one. The entry point's exit code and output
// are the recorder's; none is an error here, because none is one live.
func (r *replayRun) feed(s replayStep, toolUseID string) (*verify.StopView, error) {
	if s.At.IsZero() {
		return nil, fmt.Errorf("replay: %s %s has no time to set the clock to", s.Event, toolUseID)
	}
	if r.cfg.Ctx != nil {
		if err := r.cfg.Ctx.Err(); err != nil {
			return nil, err
		}
	}
	r.w.clock = s.At
	r.seen = nil
	var stdout, stderr bytes.Buffer
	in := bytes.NewReader(s.Raw)
	args := []string{install.Marker, r.w.id.InstallID}
	var code int
	switch s.Event {
	case install.EventPreToolUse:
		code = cmdHook(args, in, &stderr)
	case install.EventPostToolUse, install.EventPostToolUseFailure:
		code = cmdPost(args, in, &stdout, &stderr)
	case install.EventStop:
		code = cmdRecap(args, in, &stdout, &stderr)
	case install.EventUserPromptSubmit:
		code = cmdRecap(append([]string{"pending"}, args...), in, &stdout, &stderr)
	case install.EventSessionStart:
		code = cmdProbe(append([]string{"start"}, args...), in, &stderr)
	case install.EventSessionEnd:
		code = cmdProbe(append([]string{"end"}, args...), in, &stderr)
	default:
		return nil, fmt.Errorf("replay: no entry point for event %q", s.Event)
	}
	if r.cfg.KeepOutput {
		r.outputs = append(r.outputs, replayOutput{s.Event, toolUseID, code, stdout.String(), stderr.String()})
	}
	v := r.seen
	r.seen = nil
	return v, nil
}

// stopVerdict is the Stop view recapTurn computed at a Stop: the snapshot of
// the gates, claims and recording at the Stop's point, and the turn's summary.
// The point (Snap.Point.Seq) is the greatest seq in the store when the Stop
// was handled, so it holds records before the Stop and nothing after.
type stopVerdict struct {
	Snap    verify.Snapshot
	Summary verify.Summary
	Turn    verify.TurnCounts
}

// stopResult is what a replay says about one Stop point of the transcript.
type stopResult struct {
	// Pos and At are where the Stop is in the file and the time the clock read.
	Pos history.Pos
	At  time.Time
	// SessionID is the session the Stop belongs to.
	SessionID string
	// Evaluated: the recorder was fed the Stop and computed a view. When it is
	// not, Unevaluated names why (the replay.Reason of an event that could not
	// be fed or whose boundary the transcript does not establish) and Verdict
	// is nil. A Stop not evaluated is a coverage loss, never a pass.
	Evaluated   bool
	Unevaluated string
	Verdict     *stopVerdict
	// Verified: evaluated, and no gap at or before the Stop. When Evaluated and
	// not Verified, Downgrade says why: the first gap, by its reason and
	// position. Verified is "consistent with the transcript"; the session's
	// flags say what that does not cover.
	Verified  bool
	Downgrade string
	// Taint is replay.Event.Taint for the Stop.
	Taint history.Pos
	// FinalElided: the final message was longer than the recorder keeps, and
	// the replay fed its head and tail, which are all the recorder keeps of it.
	FinalElided bool
}

// sessionStart is the SessionStart payload for the session an event's payload
// belongs to: the session, transcript path and cwd that payload names, read
// the way every entry point reads a payload (hookio.Intake).
func sessionStart(first []byte) ([]byte, error) {
	in, err := hookio.Intake(bytes.NewReader(first))
	if err != nil {
		return nil, err
	}
	p := sessionStartPayload{HookEventName: install.EventSessionStart}
	for _, f := range []struct {
		key int
		dst *string
	}{{hookio.SessionID, &p.SessionID}, {hookio.TranscriptPath, &p.TranscriptPath}, {hookio.CWD, &p.CWD}} {
		if *f.dst, err = in.Str(f.key); err != nil {
			return nil, err
		}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&p); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// sessionCoverage is what a replay says about one session as a whole.
type sessionCoverage struct {
	SessionID string
	// Fed counts the events fed for it.
	Fed int
	// Flags are the named ways its replayed record is not a live one
	// (replayFlags), for a consumer that has to leave such sessions out.
	Flags []string
}

// skippedEvent is an event the replay did not feed, with why.
type skippedEvent struct {
	Kind      replay.Kind
	Pos       history.Pos
	ToolUseID string
	Reasons   []replay.Reason
}

// replayResult is a replay's report.
type replayResult struct {
	Summary  replay.Summary
	Fed      int
	Probes   int
	Skipped  []skippedEvent
	Stops    []stopResult
	Sessions []sessionCoverage
	// Outputs are the entry points' outputs, with cfg.KeepOutput.
	Outputs []replayOutput
}

// replayTranscript replays src, one transcript file, into the world, event by
// event in file order, and reports. Events the transcript cannot supply are
// skipped with their reason (never fed a guess), and every skip is a gap that
// taints the Stops after it.
func (w *replayWorld) replayTranscript(src io.ReadSeeker, opt replay.Options, cfg replayConfig) (*replayResult, error) {
	res := &replayResult{}
	err := w.replayEvents(res, cfg, func(yield func(replay.Event) error) error {
		sum, err := replay.Walk(src, opt, yield)
		res.Summary = sum
		return err
	})
	return res, err
}

// replayEvents feeds the events walk yields to the recorder, in the order it
// yields them, and fills res. The events may come from one transcript or from
// a session's merged files (replay.WalkSession): the feeding is the same, and
// the position a taint names is a file and a place in it.
func (w *replayWorld) replayEvents(res *replayResult, cfg replayConfig, walk func(yield func(replay.Event) error) error) error {
	bySession := map[string]int{}
	var taintReason string
	err := w.with(cfg, func(r *replayRun) error {
		err := walk(func(e replay.Event) error {
			// The first gap, named, once the timeline has one.
			if !e.Taint.IsZero() && taintReason == "" {
				if e.Gap != "" && e.Taint == e.Pos && e.TaintFile == e.File {
					taintReason = string(e.Gap)
				} else {
					taintReason = "unreadable_line"
				}
			}
			stop := stopResult{Pos: e.Pos, At: e.At, SessionID: e.SessionID, Taint: e.Taint, FinalElided: e.FinalElided}
			if !e.Supported() {
				res.Skipped = append(res.Skipped, skippedEvent{e.Kind, e.Pos, e.ToolUseID, e.Unsupported})
				if e.Kind == replay.Stop {
					stop.Unevaluated = string(e.Unsupported[0])
					res.Stops = append(res.Stops, stop)
				}
				return observe(cfg, e, false)
			}
			i, started := bySession[e.SessionID]
			if !started {
				// The session's first event: the replay's recorder starts
				// the session (flagProbeStartSynthesized).
				start, err := sessionStart(e.Payload)
				if err != nil {
					return err
				}
				if _, err := r.feed(replayStep{install.EventSessionStart, start, e.At}, ""); err != nil {
					return err
				}
				res.Probes++
				i = len(res.Sessions)
				bySession[e.SessionID] = i
				res.Sessions = append(res.Sessions, sessionCoverage{SessionID: e.SessionID, Flags: append([]string(nil), replayFlags...)})
			}
			view, err := r.feed(replayStep{e.Kind.String(), e.Payload, e.At}, e.ToolUseID)
			if err != nil {
				return err
			}
			res.Fed++
			res.Sessions[i].Fed++
			if e.Kind != replay.Stop {
				return observe(cfg, e, true)
			}
			if view == nil {
				stop.Unevaluated = "recorder_produced_no_view"
				res.Stops = append(res.Stops, stop)
				return observe(cfg, e, true)
			}
			stop.Evaluated = true
			stop.Verdict = &stopVerdict{Snap: view.Snap, Summary: view.Summary, Turn: view.Turn}
			stop.Verified = e.Taint.IsZero()
			if !stop.Verified {
				stop.Downgrade = fmt.Sprintf("tainted_by_%s_at_line_%d", taintReason, e.Taint.Line)
			}
			res.Stops = append(res.Stops, stop)
			return observe(cfg, e, true)
		})
		res.Outputs = r.outputs
		return err
	})
	return err
}

// observe hands an event to the config's observer, if it has one.
func observe(cfg replayConfig, e replay.Event, fed bool) error {
	if cfg.Observe == nil {
		return nil
	}
	return cfg.Observe(e, fed)
}
