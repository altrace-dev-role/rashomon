//go:build perf

package acceptance

// The replay half of detector precision, built only with -tags perf.
//
// It replays every fixture of test/corpus/detectors through the real binary,
// some 55 of them, one fresh machine each: about 5 s on a laptop without the
// race detector, against the sub-second of everything else in this package
// (CI's log prints a package's time, not a test's). That is too much for every
// push of the Test job, so it is gated the way test/perf is: by the perf tag,
// and the `perf-linux` job of ci.yaml runs it on every pull request.
//
//	env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT go test -tags perf \
//	    ./test/acceptance -run '^TestDetectorPrecision_Table$' -count=1 -v
//
// What stays in the default build is what needs no replay: the parsers, the
// statistics, the label rules and the inventory (detector_stats_test.go,
// detector_fixture_test.go, detector_precision_test.go).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// runFixture replays f in a fresh machine and reports what the Stop path
// printed and what the digest says. Nothing is read from the store directly:
// the fires are the binary's.
func runFixture(t *testing.T, f detectorFixture) fixtureReplay {
	t.Helper()
	e := newEnv(t)
	var out fixtureReplay
	final := f.FinalMessage
	if f.Capture != "" {
		out.line, out.spoke, final = e.replayCapture(t)
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
	res := e.digestRaw("--session", session, "--last-assistant-message", final)
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
		id := spec.name()
		tallies[id] = &fireTally{}
		names, fixtures := loadDetectorFixtures(t, id)
		hasFixtures[id] = len(fixtures) > 0
		if len(fixtures) > 0 && spec.reportOnly {
			t.Fatalf("%s is report-only: its fire condition is not in the digest the replay reads", id)
		}
		for i, f := range fixtures {
			t.Run(id+"/"+names[i], func(t *testing.T) {
				fired, problems := fixtureProblems(spec, f, runFixture(t, f))
				for _, p := range problems {
					t.Error(p)
				}
				if fired && len(problems) == 0 {
					tallies[id].add(f.Label, f.LabelledBy)
					if f.Label == labelFalsePositive {
						reproduced[f.FPClass] = append(reproduced[f.FPClass], id+"/"+names[i])
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
		lines = append(lines, precisionRow{spec.name(), *tallies[spec.name()], hasFixtures[spec.name()]}.String())
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
