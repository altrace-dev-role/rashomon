package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/arm"
	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The steer step (card 2.3): once a post's record is durable, the call is
// judged against the run and the note it raises, if any, is returned as the
// bytes the hook prints. The hook never writes stdout; cmd prints. Nothing
// in the step reads the environment, the clock or stdout: SteerEnv carries
// all three in, so the step is a function of the store and its arguments.

// SteerReadBudget is how long the step waits for the run's locks before it
// gives up and prints nothing. It is a lock-wait deadline, not a cap on the
// read: BenchmarkPostSteer measures what the read and the evaluation cost.
const SteerReadBudget = 10 * time.Millisecond

// SteerEnv is the step's port: what cmd reads from the process for it.
// Consent is the steer_disclosure marker's reader (store.SteerDisclosureMarker,
// D84); a nil reader reads as no consent. HoldoutOff and HoldoutOn are
// EnvHoldout as HoldoutOff and HoldoutOn read it, and Arm is EnvSteerArm as
// SteerArm reads it (cmd lets it count on a lab store only). ProjectDir is
// CLAUDE_PROJECT_DIR and ManagedDir the managed-settings directory
// (settings.ManagedDir), DiscoverSpec's two inputs beside the call's cwd
// (card 3.1 (c)).
type SteerEnv struct {
	HarnessID, Surface string
	MirrorInBand       bool
	AIAgent            string
	CI                 bool
	HoldoutOff         bool
	HoldoutOn          bool
	Arm                steer.Arm
	Consent            func(root string) (int, error)
	Now                time.Time
	ProjectDir         string
	ManagedDir         string
}

// steps is the trigger (card 2.3 R3; card P5B, 5.7): an execution that
// landed in the ordered stream, at a failure event that carried a runner
// digest, the one call that can raise regression-location (a success event
// is outcome ok, D61), or at either post event when the call's input names
// .claude/rashomon, the one call that can raise integrity/store-touched
// (D-B7). Every other call costs these branches and storeNamed's scan.
func steps(event string, landedSeq int64, runner, inStore bool) bool {
	failure := event == FailureEvent
	return landedSeq > 0 && (failure && runner || inStore && (failure || event == string(steer.EventPostToolUse)))
}

// storeNamed reports a tool input whose raw bytes name .claude/rashomon, its
// slash plain or escaped as JSON may write it (D-B7), or name root, the
// store root (rootNamed, D173): a byte scan, with no decode. It is a
// superset of the calls that write there, and verify says which did. A Bash
// line that reaches the store by cd and a relative path is missed here; its
// store_touched entry still reaches the ledger and the Stop view.
func storeNamed(input []byte, root string) bool {
	if rootNamed(input, root) {
		return true
	}
	return bytes.Contains(input, []byte(".claude/rashomon")) || bytes.Contains(input, []byte(`.claude\/rashomon`))
}

// rootNamed reports a tool input whose raw bytes name root, its slashes plain
// or escaped. The root's last element is looked for first: an input that
// names the root holds it, and it is short, so a call that does not name it
// is one scan. The escaped form is built only for an input that escapes a
// slash, which neither encoding/json nor Claude Code's payloads do, so the
// common path allocates nothing (each []byte conversion is read-only and
// zero-copy). A root of "" is none.
func rootNamed(input []byte, root string) bool {
	if root == "" || !bytes.Contains(input, []byte(filepath.Base(root))) {
		return false
	}
	if bytes.Contains(input, []byte(root)) {
		return true
	}
	return bytes.Contains(input, []byte(`\/`)) && bytes.Contains(input, []byte(strings.ReplaceAll(root, "/", `\/`)))
}

// writtenNamed is the paths a call names as written, read as the
// declaration path reads them (handle.go): the one path a file-changing
// tool's input names, else a shell line's write targets. A line the shape
// says writes nothing is not tokenized again, so no other call pays for it.
func writtenNamed(tool string, input json.RawMessage, mayWrite bool) []string {
	if p := shape.DeclaredPath(tool, input); p != nil {
		return []string{*p}
	}
	if !mayWrite {
		return nil
	}
	paths, _ := shape.WrittenPaths(tool, input)
	return paths
}

// writtenCall is a success event's call as the stale trigger reads it: the
// tool and input writtenNamed decodes, and the shape's two answers to
// whether it may name a written path at all (a verb class of write, a shell
// line that may write), which cost nothing to keep. The paths are decoded
// only for a write the session is armed for (staleWrite).
type writtenCall struct {
	tool      string
	input     json.RawMessage
	writeVerb bool
	mayWrite  bool
}

// Steer runs the step after the record's Flush succeeded and returns the
// bytes to print, or nil. It returns no error: each exit before Route prints
// nothing (no trigger, an unreadable run with no rule raised), and every
// exit after Route has its line in the steer log. A gate run's post arms the
// stale trigger first (armStale), whatever the step then does.
func (p *Post) Steer(env SteerEnv) []byte {
	p.armStale()
	ctx, view, ok := p.judge(env)
	if !ok {
		return nil
	}
	p.keepHits(view.Hits, env.Now)
	out := steer.Route(&ctx, view.Findings)
	// The line is durable before its note prints (REC-34).
	if err := fault.Fail(fault.PointSteerFlush); err != nil {
		return nil
	}
	if err := p.st.Flush(); err != nil {
		return nil
	}
	// Route withheld any channel the client cannot carry, so an error here
	// is a contract break: print nothing.
	b, err := out.MarshalFor(p.event, ctx.Client)
	if err != nil {
		return nil
	}
	return b
}

// keepHits appends each forbid_command rule the call matched to the run as a
// spec_event rule_hit (card SPEC-RULES-STOP1), at either post event and
// whatever Route then decides: the steer switch, the arm, the holdout,
// consent and attendance do not reach it. It runs before Route, so Steer's
// one Flush makes the hits durable before any note prints (REC-34); at a
// failure, where Route writes no line, that Flush is the one sync the hits
// add. A hit that does not land (a lock timeout, an append error) is lost,
// and the Stop then reads that rule undecided, never clear.
func (p *Post) keepHits(hits []verify.RuleHit, now time.Time) {
	for _, h := range hits {
		_ = p.st.AppendRecord(p.sessionID, store.NewRuleHit(p.sessionID, p.event, h.RuleID, h.SourceDigest, p.landedSeq, now.UnixMilli()))
	}
}

// armStale arms the session's stale trigger at the post of a gate run: a
// runner call, the only kind that forms a family (verify classify), landed
// at a success event. The marker is a hint (store.ArmStale), so an arm that
// failed to write is a missed nudge and nothing else.
func (p *Post) armStale() {
	if p.landedSeq > 0 && p.landedRunner && p.event == string(steer.EventPostToolUse) {
		_ = p.st.ArmStale(p.sessionID)
	}
}

// staleWrite is the stale trigger (card STALE-NUDGE1, D166): a write landed
// at a success event while the session is armed, naming a path under the
// project root that a gate counts (verify.RelevantUnder), in a project with
// an index file. It returns the project key it resolved and whether this
// call took the arm, which only one of two racing posts does. An unarmed
// write pays one Lstat and decodes no path; an armed one adds the key's
// walk, its paths' decode (writtenNamed) and a second Lstat; only the call
// that takes the arm reads the run.
func (p *Post) staleWrite() (key string, took, indexless bool) {
	w := &p.landedWrite
	if p.landedSeq == 0 || p.event != string(steer.EventPostToolUse) || !(w.writeVerb || w.mayWrite) {
		return "", false, false
	}
	if armed, _ := p.st.StaleArmed(p.sessionID); !armed {
		return "", false, false
	}
	key = baseline.Key(p.cwd)
	if key == "" || !slices.ContainsFunc(writtenNamed(w.tool, w.input, w.mayWrite), func(path string) bool { return verify.RelevantUnder(path, key, p.cwd) }) {
		return "", false, false
	}
	if _, err := os.Lstat(verify.IndexPath(p.st.Root(), key)); err != nil {
		// No index file: only a spec's declared required gate can raise the
		// note (D222), and judge takes the arm before it reads the run and
		// reads the spec after.
		return key, false, true
	}
	return key, p.st.TakeStaleArm(p.sessionID), false
}

// ruled is the spec rules' trigger (card SPEC-RULES1): an execution that
// landed, at either post event, of a Bash call or a tool that names the
// file it writes, the calls a forbid_command rule (at PostToolUse) or a
// protect_path rule raises at from the call alone (verify.RulesAtCall).
// Such a call costs the spec's discovery at every post, whether the spec
// declares a rule or not.
func ruled(event string, landedSeq int64, toolName string) bool {
	if landedSeq <= 0 || event != FailureEvent && event != string(steer.EventPostToolUse) {
		return false
	}
	switch toolName {
	case "Bash", "Edit", "MultiEdit", "NotebookEdit", "Write":
		return true
	}
	return false
}

// judge is the step's compute before Route: the triggers, the run read once
// where steps (or the stale trigger) fires, the spec discovered from the
// payload's cwd (per trigger, not per call: D128), the project index read
// from the cwd's project key as the Stop path reads it (recap_stop.go; card
// INDEX-POST1), the key walked once for both (discoverSpecAt), the run judged
// at the landed seq against both, the call judged against the spec's rules,
// and Route's context for the call. The rules read the call and the spec,
// never the run, so a run that could not be read drops the step's findings
// and keeps theirs. An index that cannot be read, or is over its cap, reads
// as none (ReadIndex). ok is false when neither trigger fires, or when no run
// was read and the rules neither raised nor hit anything. It writes nothing
// but the stale trigger's disarm, so BenchmarkPostSteer's eval rows measure
// it apart from the append and the sync; the view's hits are Steer's to
// keep.
func (p *Post) judge(env SteerEnv) (steer.Context, verify.SteerView, bool) {
	var key string
	var indexless bool
	stepped := steps(p.event, p.landedSeq, p.landedRunner, p.landedStore)
	if !stepped {
		var took bool
		if key, took, indexless = p.staleWrite(); took {
			stepped = true
		}
	}
	// The index-less path takes the arm before the read, as the indexed path
	// does in staleWrite: a read that errors (budget) must not leave it set,
	// or every later relevant write would read the run and discover the spec
	// again for the same answer. A taken arm reads the run, as a step does;
	// the spec test below decides whether its findings stand (D222).
	tookIndexless := indexless && p.st.TakeStaleArm(p.sessionID)
	readRun := stepped || tookIndexless
	if p.landedStore {
		p.st.ClearNoSpec(p.sessionID)
	}
	if !readRun && (!ruled(p.event, p.landedSeq, p.toolName) || p.noSpecNoted(env)) {
		return steer.Context{}, verify.SteerView{}, false
	}
	var run *store.Run
	if readRun {
		if r, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget); err == nil {
			run = r
		}
	}
	if key == "" {
		key = baseline.Key(p.cwd)
	}
	root := p.st.Root()
	p.sp = p.discoverFor(env, key, run, readRun)
	if tookIndexless && !verify.DeclaresRequired(p.sp) {
		run = nil // the stale write's step stands only on a required gate; the rules still judge
	}
	pt := verify.Point{Seq: p.landedSeq, SessionID: p.sessionID, PromptID: nilIfEmpty(p.promptID), AgentID: nilIfEmpty(p.agentID),
		StoreRoot: root}
	rules := verify.RulesAtCall(p.sp, p.ruleCall(run), pt, p.event)
	if run == nil {
		if len(rules.Findings) == 0 && len(rules.Hits) == 0 {
			return steer.Context{}, verify.SteerView{}, false
		}
		return p.contextOf(env, rules), rules, true
	}
	var idx *verify.ProjectIndex
	if key != "" {
		idx, _ = verify.ReadIndex(verify.IndexPath(p.st.Root(), key))
	}
	view := verify.SteerAt(run, idx, p.sp, p.st.Key(), pt, env.Now, p.event, p.toolUseID)
	view.Findings = append(view.Findings, rules.Findings...)
	view.Dropped += rules.Dropped
	view.Hits = rules.Hits
	return p.contextOf(env, view), view, true
}

// discoverFor is the spec the call is judged against. With the run read,
// it is discovered under TrustRoot's root (D173); a step whose run could not
// be read has none to check and takes the path below. A rules-only call reads no
// run, so it is discovered once with no trust, and a repository spec the bindings
// hold is checked: the run is read and, if it names a write under the store
// root or cannot be read, the spec stays untrusted, so a session that could
// have written its own binding never raises a model-visible rule (D173). A
// spec that binds nothing, the common case, costs no run read.
func (p *Post) discoverFor(env SteerEnv, key string, run *store.Run, stepped bool) *spec.Compiled {
	root := p.st.Root()
	if stepped && run != nil {
		return discoverSpecAt(p.cwd, key, env.ProjectDir, env.ManagedDir, TrustRoot(run, root, p.landedSeq))
	}
	sp := discoverSpecAt(p.cwd, key, env.ProjectDir, env.ManagedDir, TrustedRoot{})
	if sp == nil || !boundSource(sp, root) {
		return sp
	}
	if r, err := p.st.ReadRunConsistent(p.sessionID, SteerReadBudget); err == nil {
		if t := TrustRoot(r, root, p.landedSeq); t != (TrustedRoot{}) {
			return discoverSpecAt(p.cwd, key, env.ProjectDir, env.ManagedDir, t)
		}
	}
	return sp
}

// boundSource reports that the bindings file under root holds a source of sp
// that was discovered with no trust. A session's own record is read before it
// is believed (TrustRoot), so discoverFor discovers first under no root, and
// only a spec the bindings could make trusted pays for the run read and a
// second discovery.
func boundSource(sp *spec.Compiled, root string) bool {
	var b *spec.Bindings
	for _, s := range sp.Sources {
		if s.Trust != spec.Untrusted {
			continue
		}
		if b == nil {
			var err error
			if b, err = spec.ReadBindings(spec.OSFS{}, root); err != nil {
				return false
			}
		}
		if b.Holds(SpecProject(s.Path), s.Path, s.Digest) {
			return true
		}
	}
	return false
}

// ruleCall is the call the spec's rules judge, read from the input only as
// far as a discovered rule reads it: a Bash call's line, under the cap a
// recorded command keeps whole, where a forbid_command rule is declared,
// and a file tool's path, with the project key a declaration carries
// (baseline.Key of the cwd), where a protect_path rule is.
//
// An order rule reads the calls before this one, so a Bash call of a spec
// that declares one carries the run: the one this judge read, else its own
// read, which is no step's (the step's findings stand on its own run). A run
// that cannot be read leaves the order rule raising nothing.
func (p *Post) ruleCall(run *store.Run) verify.RuleCall {
	c := verify.RuleCall{Seq: p.landedSeq, ToolName: p.toolName, CWD: p.cwd, ToolUseID: p.toolUseID}
	command, path := ruleReads(p.sp)
	if p.toolName == "Bash" && declaresOrder(p.sp) {
		if run == nil {
			run, _ = p.st.ReadRunConsistent(p.sessionID, SteerReadBudget)
		}
		c.Run = run
	}
	if command {
		c.Command, _ = shape.CommandLine(p.toolName, p.toolInput, store.MaxCommandBytes)
	}
	if path {
		if fp := shape.DeclaredPath(p.toolName, p.toolInput); fp != nil {
			c.FilePath, c.Project = *fp, baseline.Key(p.cwd)
		}
	}
	return c
}

// declaresOrder reports an order rule among sp's own, the kind that reads
// the run at a post.
func declaresOrder(sp *spec.Compiled) bool {
	if sp == nil {
		return false
	}
	for i := range sp.Rules {
		if sp.Rules[i].Source != spec.SourceImplied && spec.RuleKind(sp.Rules[i].Rule.Kind) == spec.Order {
			return true
		}
	}
	return false
}

// ruleReads reports whether sp declares a forbid_command rule and whether
// it declares a protect_path rule, the two kinds that read a call; the
// implied task-file rule reads none (RulesAtCall passes it over).
func ruleReads(sp *spec.Compiled) (command, path bool) {
	if sp == nil {
		return false, false
	}
	for i := range sp.Rules {
		if sp.Rules[i].Source == spec.SourceImplied {
			continue
		}
		switch spec.RuleKind(sp.Rules[i].Rule.Kind) {
		case spec.ForbidCommand:
			command = true
		case spec.ProtectPath:
			path = true
		}
	}
	return command, path
}

// contextOf is Route's context for this call (card 2.3 R4.5). Attendance is
// attendanceOf's: CI or the dontAsk permission mode, and on the prompt path
// the surface. Consent is the marker
// at or over the disclosure this build shows. The project is the view's
// root only: under record scope v4 the record holds none and the line
// follows the record, so the payload's cwd is never a fallback. Arm is
// armOf's, and Holdout is the session's draw (heldOut).
func (p *Post) contextOf(env SteerEnv, view verify.SteerView) steer.Context {
	version, source := steer.ClientVersion(env.AIAgent)
	attendance := attendanceOf(p.event, env, p.permissionMode)
	steerOff, unreadable := p.steerOff()
	return steer.Context{
		Event:            steer.Event(p.event),
		Client:           steer.Client{Version: version, Source: source, MirrorInBand: env.MirrorInBand, Subagent: p.agentID != ""},
		Store:            p.st,
		SessionID:        p.sessionID,
		PromptID:         nilIfEmpty(p.promptID),
		AgentID:          p.agentID,
		Project:          view.ProjectRoot,
		HarnessID:        env.HarnessID,
		Surface:          env.Surface,
		NowUnixMS:        env.Now.UnixMilli(),
		Consent:          consented(env.Consent, p.st.Root()),
		SteerOff:         steerOff,
		ConfigUnreadable: unreadable,
		Arm:              armOf(env),
		Attendance:       attendance,
		Holdout:          p.heldOut(env),
	}
}

// steerOff is the user's steering switch for this call (card STEER-CLI1): the
// store's config.json read once, at one Lstat when the file is absent, and the
// user's key or the key of the call's project (baseline.Key of the cwd,
// walked only when the file names a project) set to off. A file that does not
// read is off: the notes reach the model, so a switch that cannot be read is
// not one to override, and the second result says that is why (each finding is
// logged config_unreadable, so the person is told to fix the file). Off is a
// clamp (clampReason): nothing reaches the agent, the user still sees a flag,
// and each finding is logged steer_off.
func (p *Post) steerOff() (off, unreadable bool) {
	c, err := store.ReadConfig(p.st.Root())
	if err != nil {
		return true, true
	}
	if c.SteerOffUser() {
		return true, false
	}
	return len(c.SteerProjects) > 0 && c.SteerOffFor(baseline.Key(p.cwd)), false
}

// armOf is the context's arm: the operator's forced arm when env names one,
// else live, the arm of every install no operator forced.
func armOf(env SteerEnv) steer.Arm {
	if env.Arm != "" {
		return env.Arm
	}
	return steer.ArmLive
}

// holdoutFeature is the arm feature the steer holdout draws under: one
// name, so every reader of a session's arm draws the same answer.
const holdoutFeature = "steer"

// holdoutBP is the field holdout's share in basis points when EnvHoldout is
// "on" and no spec's holdout object gives one (PRD PF-MS5). With neither, the
// share is 0 and no session is held out: the alpha is opt-in, and MEASURE's
// sessions opt in.
const holdoutBP = 1000

// EnvHoldout names the operator's switch for the field holdout: "on" draws
// every session at holdoutBP where no spec's holdout object sets the share,
// and "off" skips the draw, so every session is treatment, whatever a spec
// says. The project file's holdout object (5.6, with 3.1 PR 2's discovery)
// is the configured form (PRD PF-MS5); this is the operator's override,
// kept after 5.6 as the organisation's switch until the organisation file
// PF-MS5 names exists.
const EnvHoldout = "RASHOMON_HOLDOUT"

// HoldoutOff reads EnvHoldout's value: true for "off" exactly. Any other
// value, "OFF" and "0" among them, is ignored, with no error and no log.
func HoldoutOff(v string) bool {
	return v == "off"
}

// HoldoutOn reads EnvHoldout's value: true for "on" exactly. Any other
// value, "ON" and "1" among them, is ignored, with no error and no log.
func HoldoutOn(v string) bool {
	return v == "on"
}

// EnvSteerArm names the operator's forced arm for a paired run: "off" decides
// every note and logs it as shadow with nothing printed, "steer" delivers as
// live. Either is written on each steer line as the arm, and a forced arm
// skips the holdout draw, so a forced session never reads as a drawn one.
const EnvSteerArm = "RASHOMON_STEER_ARM"

// SteerArm reads EnvSteerArm's value: steer.ArmOff for "off" and
// steer.ArmSteer for "steer" exactly, else "" (no forced arm). Any other
// value, "live" and "OFF" among them, is ignored, with no error and no log.
func SteerArm(v string) steer.Arm {
	switch v {
	case "off":
		return steer.ArmOff
	case "steer":
		return steer.ArmSteer
	}
	return ""
}

// heldOut is the session's draw into the field holdout (card 2.5, D109):
// arm's keyed draw for the session, the same on each of its calls, at the
// share the discovered spec's holdout object gives at env.Now (On: none when
// it is off or its window has closed), else at holdoutBP under EnvHoldout
// "on", else at none (5.6). A spec with no holdout object reads as none: the
// parser never leaves an object's rate at 0 (spec.Holdout), so a rate of 0
// is no object. The operator's off and a forced arm are above all of them: a
// forced arm decides, not the draw. A draw arm refuses
// is treatment, and nothing records it: the store's key exists on every
// opened store. With no share, no draw is made.
func (p *Post) heldOut(env SteerEnv) bool {
	if env.HoldoutOff || env.Arm != "" {
		return false
	}
	bp := 0
	if env.HoldoutOn {
		bp = holdoutBP
	}
	if p.sp != nil && p.sp.Holdout.RateBP != 0 {
		on, rate := p.sp.Holdout.On(env.Now)
		if !on {
			return false
		}
		bp = rate
	}
	if bp == 0 {
		return false
	}
	a, err := arm.New(p.st.Key())
	if err != nil {
		return false
	}
	got, err := a.Assign(holdoutFeature, p.sessionID, bp)
	return err == nil && got == arm.Holdout
}

// consented reads the marker; no reader, a read error or a value below
// steer.DisclosureVersion is no consent, which Route clamps to a flag.
func consented(read func(root string) (int, error), root string) bool {
	if read == nil {
		return false
	}
	n, err := read(root)
	return err == nil && n >= steer.DisclosureVersion
}
