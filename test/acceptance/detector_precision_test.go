package acceptance

// Detector precision, the replay half: each fixture's run is fed to the built
// binary as the hook payloads it would have sent, the Stop path prints its
// end-of-turn line, `digest` reports which detectors fired, and the label the
// fixture carries is held to what happened. Then the per-detector table.
//
// Read the table with:
//
//	env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT go test ./test/acceptance \
//	    -run '^TestDetectorPrecision_Table$' -count=1 -v
//
// This file measures and never changes a detector: no detector's behaviour is
// read from anywhere but the binary, and nothing here demotes a line.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// realCaptureName is the one committed capture a fixture may replay whole:
// the Claude Code 2.1.280 session under test/fixtures/claude-code.
const realCaptureName = "claude-code/2.1.280"

// detectorCorpus is the fixtures' and the sheet's directory, relative to this
// package.
const detectorCorpus = "../corpus/detectors"

// detectorDigest is the part of `rashomon digest` the detectors' fire
// conditions are read from.
type detectorDigest struct {
	Coverage struct {
		State string `json:"state"`
	} `json:"coverage"`
	Declarations struct {
		WithoutExecution []json.RawMessage `json:"without_execution"`
	} `json:"declarations"`
	Unknown        bool `json:"unknown"`
	SilentFailures struct {
		Fires bool `json:"fires"`
	} `json:"silent_failures"`
	MaskedRuns struct {
		Fires bool `json:"fires"`
	} `json:"masked_runs"`
	TestBending struct {
		TestsOnlyThenGreen        []json.RawMessage `json:"tests_only_then_green"`
		TestsOnlyThenGreenOmitted int               `json:"tests_only_then_green_omitted"`
		Flaky                     []json.RawMessage `json:"flaky"`
		FlakyOmitted              int               `json:"flaky_omitted"`
	} `json:"test_bending"`
	Truncated bool `json:"truncated"`
}

// detectorSpec is one entry of the inventory. fired reads the structured
// digest, which is what a rewording of the line leaves alone; sentence reads
// the line itself, and a disagreement between the two fails the fixture, so a
// change to either surfaces here. A detector with no sentence is report-only.
type detectorSpec struct {
	id       string
	fired    func(detectorDigest) bool
	sentence func(line string) bool
}

func lineHas(s string) func(string) bool {
	return func(line string) bool { return strings.Contains(line, s) }
}

func lineMatches(re string) func(string) bool {
	r := regexp.MustCompile(re)
	return func(line string) bool { return r.MatchString(line) }
}

// detectorInventory is every detector that puts a judgement in front of a
// user, in the order the end-of-turn line states them, then the report-only
// ones. Its ids are the directory names under the corpus, the rows of
// label-rules.tsv and the sheet's detector column.
var detectorInventory = []detectorSpec{
	{"digest_unknown", func(d detectorDigest) bool { return d.Unknown }, lineHas("digest unknown (")},
	{"coverage_unverified", func(d detectorDigest) bool { return !d.Unknown && d.Coverage.State != "verified" }, lineHas("coverage unverified (")},
	{"digest_truncated", func(d detectorDigest) bool { return d.Truncated }, lineHas("digest truncated (")},
	{"silent_failure", func(d detectorDigest) bool { return d.SilentFailures.Fires }, lineMatches(`\d+ recorded failures?`)},
	{"masked_run", func(d detectorDigest) bool { return d.MaskedRuns.Fires }, lineHas("pass claimed; exit status of ")},
	{"tests_only_then_green", func(d detectorDigest) bool {
		return len(d.TestBending.TestsOnlyThenGreen)+d.TestBending.TestsOnlyThenGreenOmitted > 0
	}, lineHas("test command failed, then the only recorded edits were to files named like tests, then it passed")},
	{"flaky_pair", func(d detectorDigest) bool { return len(d.TestBending.Flaky)+d.TestBending.FlakyOmitted > 0 },
		lineHas("same test command had both outcomes with no recorded file edit between")},
	{"declaration_without_execution", func(d detectorDigest) bool { return len(d.Declarations.WithoutExecution) > 0 },
		lineMatches(`\d+ declarations? without recorded execution`)},
	// Report-only: sections of `rashomon report` that state a finding and have
	// no sentence on the end-of-turn line. No fixtures yet; each row of the
	// table says so.
	{"novel_destination", nil, nil},
	{"executed_differently_from_declared", nil, nil},
}

func detectorSpecByID(id string) (detectorSpec, bool) {
	for _, d := range detectorInventory {
		if d.id == id {
			return d, true
		}
	}
	return detectorSpec{}, false
}

func knownDetector(id string) bool { _, ok := detectorSpecByID(id); return ok }

// loadDetectorFixtures reads every fixture of one detector, sorted by name.
// A detector with no directory has none, which is not an error.
func loadDetectorFixtures(t *testing.T, id string) (names []string, fixtures []detectorFixture) {
	t.Helper()
	dir := filepath.Join(detectorCorpus, id)
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			t.Fatalf("%s/%s: a fixture directory holds .json fixtures and nothing else", id, ent.Name())
		}
		b, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		f, err := parseFixture(id, b)
		if err != nil {
			t.Fatalf("%s/%s: %v", id, ent.Name(), err)
		}
		names = append(names, strings.TrimSuffix(ent.Name(), ".json"))
		fixtures = append(fixtures, f)
	}
	return names, fixtures
}

// loadSheet reads the labelling sheet.
func loadSheet(t *testing.T) []sheetRow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(detectorCorpus, "labelling-sheet.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseSheet(b, knownDetector)
	if err != nil {
		t.Fatalf("labelling-sheet.tsv: %v", err)
	}
	return rows
}

// fixtureReplay is what one fixture produced.
type fixtureReplay struct {
	line    string
	spoke   bool
	digest  detectorDigest
	rawJSON string
	// final is the turn's final assistant message the digest was given.
	final string
}

// runFixture replays f in a fresh machine and reports what the Stop path
// printed and what the digest says. Nothing is read from the store directly:
// the fires are the binary's.
func runFixture(t *testing.T, f detectorFixture) fixtureReplay {
	t.Helper()
	e := newEnv(t)
	out := fixtureReplay{final: f.FinalMessage}
	if f.Capture != "" {
		out.line, out.spoke, out.final = e.replayCapture(t)
	} else {
		if res := e.watch(); res.exitCode != 0 {
			t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		if f.probeStart() {
			if res := e.probe("start", testSession); res.exitCode != 0 {
				t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
			}
		}
		n := 0
		for _, c := range f.Calls {
			reps := c.Repeat
			if reps == 0 {
				reps = 1
			}
			for i := 0; i < reps; i++ {
				n++
				e.replayCall(t, fmt.Sprintf("toolu_fx%04d", n), c)
			}
		}
		out.line, out.spoke = e.recapLine(stopPayload(testSession, f.FinalMessage, false))
	}
	session := testSession
	if f.Capture != "" {
		session = realSession
	}
	res := e.digestRaw("--session", session, "--last-assistant-message", out.final)
	if res.exitCode != 0 {
		t.Fatalf("digest: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	out.rawJSON = res.stdout
	if err := json.Unmarshal([]byte(res.stdout), &out.digest); err != nil {
		t.Fatalf("digest is not JSON: %v\n%s", err, res.stdout)
	}
	return out
}

// replayCall feeds the binary one call as Claude Code would have: the
// PreToolUse payload, then the PostToolUse or PostToolUseFailure that
// answers it, or nothing for a call that has no ending.
func (e *env) replayCall(t *testing.T, id string, c fixtureCall) {
	t.Helper()
	pre := defaultPayload()
	pre.ToolUseID, pre.ToolName, pre.ToolInput = id, c.Tool, c.Input
	e.mustHook(pre.build(t))
	switch c.Outcome {
	case "ok":
		post := defaultPost()
		post.ToolUseID, post.ToolName, post.ToolInput = id, c.Tool, c.Input
		if c.Tool == "Bash" {
			post.ToolResponse = map[string]any{"stdout": c.Output, "stderr": "", "interrupted": false}
		} else {
			post.ToolResponse = map[string]any{"output": c.Output}
		}
		e.mustPost(post.build(t))
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
		b, err := json.Marshal(map[string]any{
			"hook_event_name": "PostToolUseFailure",
			"session_id":      testSession,
			"transcript_path": "/tmp/transcripts/sess-1.jsonl",
			"cwd":             "/tmp/project",
			"permission_mode": "default",
			"tool_name":       c.Tool,
			"tool_input":      c.Input,
			"tool_use_id":     id,
			"error":           msg,
			"is_interrupt":    false,
			"duration_ms":     30,
		})
		if err != nil {
			t.Fatal(err)
		}
		e.mustPost(string(b))
	}
}

// replayCapture replays the committed Claude Code capture the way
// replayReal does, except that the Stop payload's line is kept: replayReal
// discards it, and a second Stop for the same turn is a duplicate that says
// nothing. It returns that line and the Stop's final message.
func (e *env) replayCapture(t *testing.T) (line string, spoke bool, final string) {
	t.Helper()
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
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
	for _, p := range realPayloads(t, transcript) {
		args, ok := sub[p.event]
		if !ok {
			t.Fatalf("no subcommand for captured event %q", p.event)
		}
		res := e.run(p.raw+"\n", nil, append(append([]string{}, args...), e.installArgs()...)...)
		if res.exitCode != 0 {
			t.Fatalf("%s (%s): exit %d, stderr %q", p.event, p.tool, res.exitCode, res.stderr)
		}
		if p.event != "Stop" {
			continue
		}
		var s struct {
			Message string `json:"last_assistant_message"`
		}
		if err := json.Unmarshal([]byte(p.raw), &s); err != nil {
			t.Fatal(err)
		}
		final = s.Message
		if res.stdout != "" {
			var o recapOutput
			if err := json.Unmarshal([]byte(res.stdout), &o); err != nil {
				t.Fatalf("recap stdout is not JSON: %v", err)
			}
			line, spoke = o.SystemMessage, true
		}
	}
	return line, spoke, final
}

// fixtureProblems holds a replay to its fixture: the label must match what
// the detector did, and the structured digest and the printed line must agree
// that it did it. It returns whether the detector fired and what is wrong.
func fixtureProblems(spec detectorSpec, f detectorFixture, r fixtureReplay) (fired bool, problems []string) {
	fired = spec.fired(r.digest)
	switch {
	case isFireLabel(f.Label) && !fired:
		problems = append(problems, fmt.Sprintf("labelled %s but %s did not fire: the label presupposes a fire. Either the detector's behaviour changed or the fixture is wrong.\nline: %q\ndigest: %s",
			f.Label, spec.id, r.line, r.rawJSON))
	case !isFireLabel(f.Label) && fired:
		problems = append(problems, fmt.Sprintf("labelled %s but %s fired.\nline: %q\ndigest: %s", f.Label, spec.id, r.line, r.rawJSON))
	}
	if spec.sentence != nil && fired != (r.spoke && spec.sentence(r.line)) {
		problems = append(problems, fmt.Sprintf("the digest says fired=%v for %s but the printed line (%q, printed %v) disagrees: the line's wording changed, or the digest's fire condition did. Update detectorInventory with the new wording only after reading what the line now says.",
			fired, spec.id, r.line, r.spoke))
	}
	return fired, problems
}

// TestDetectorPrecision_Table replays every fixture, holds each label to the
// binary's behaviour, and prints one row per detector: N, precision, the
// Wilson 95% interval and a status. Only labels a person confirmed count; a
// detector below the floor is printed unmeasured, whatever its seeds say.
func TestDetectorPrecision_Table(t *testing.T) {
	tallies := map[string]*fireTally{}
	hasFixtures := map[string]bool{}
	// reproduced is each false-positive class a fixture reproduced, with the
	// fixtures that show it: what a fix would have to cover.
	reproduced := map[string][]string{}
	for _, spec := range detectorInventory {
		tallies[spec.id] = &fireTally{}
		names, fixtures := loadDetectorFixtures(t, spec.id)
		hasFixtures[spec.id] = len(fixtures) > 0
		if len(fixtures) > 0 && spec.fired == nil {
			t.Fatalf("%s has fixtures but no fire condition", spec.id)
		}
		for i, f := range fixtures {
			t.Run(spec.id+"/"+names[i], func(t *testing.T) {
				fired, problems := fixtureProblems(spec, f, runFixture(t, f))
				for _, p := range problems {
					t.Error(p)
				}
				if fired && len(problems) == 0 {
					tallies[spec.id].add(f.Label, f.LabelledBy)
					if f.Label == labelFalsePositive {
						reproduced[f.FPClass] = append(reproduced[f.FPClass], spec.id+"/"+names[i])
					}
				}
			})
		}
	}
	for _, row := range loadSheet(t) {
		tallies[row.Detector].add(row.Label, row.LabelledBy)
		hasFixtures[row.Detector] = true
	}

	var lines []string
	for _, spec := range detectorInventory {
		lines = append(lines, precisionRow{spec.id, *tallies[spec.id], hasFixtures[spec.id]}.String())
	}
	var classes []string
	for _, c := range fpClasses {
		if len(reproduced[c]) > 0 {
			classes = append(classes, fmt.Sprintf("%-26s %s", c, strings.Join(reproduced[c], ", ")))
		}
	}
	t.Logf("false-positive classes reproduced (class, fixtures):\n%s", strings.Join(classes, "\n"))
	t.Logf("detector precision (counted = confirmed by a person; seed labels are listed, never counted; floor %d)\n%s",
		minLabelledFires, strings.Join(lines, "\n"))
}

// TestDetectorPrecision_InventoryMatchesCorpus: the inventory, the fixture
// directories and the label rules are one list. A directory with no detector,
// a detector with no rule, or a rule with no detector is a drift.
func TestDetectorPrecision_InventoryMatchesCorpus(t *testing.T) {
	ents, err := os.ReadDir(detectorCorpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if ent.IsDir() && !knownDetector(ent.Name()) {
			t.Errorf("directory %s is no detector of the inventory", ent.Name())
		}
	}
	b, err := os.ReadFile(filepath.Join(detectorCorpus, "label-rules.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if want := "detector\tfires_when\ttrue_positive_when\tfalse_positive_when"; lines[0] != want {
		t.Fatalf("label-rules.tsv header = %q, want %q", lines[0], want)
	}
	ruled := map[string]bool{}
	for _, l := range lines[1:] {
		cols := strings.Split(l, "\t")
		if len(cols) != 4 {
			t.Errorf("label-rules.tsv row %q has %d columns, want 4", l, len(cols))
			continue
		}
		if !knownDetector(cols[0]) {
			t.Errorf("label-rules.tsv names %s, which is no detector of the inventory", cols[0])
		}
		for _, c := range cols[1:] {
			if strings.TrimSpace(c) == "" {
				t.Errorf("label-rules.tsv row for %s has an empty cell", cols[0])
			}
		}
		ruled[cols[0]] = true
	}
	var missing []string
	for _, d := range detectorInventory {
		if !ruled[d.id] {
			missing = append(missing, d.id)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no label rule for %v: a person cannot label what no rule defines", missing)
	}
}
