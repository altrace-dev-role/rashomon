package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The pure half of the multi-turn replay, tested without the binary, in
// detector_stats_test.go's style: the turns and files grammar, the line
// corpus's grammar, the snapshot's rendering, its normaliser and its
// comparison. Each refusal differs from an accepted document in one place,
// and each cap is accepted at its value before it is refused past it.

// goodTurns is a two-turn fixture with a project: the accepted spelling the
// refusals below each differ from in one place.
const goodTurns = `{"detector":"declaration_without_execution","label":"true_positive","reason":"r","labelled_by":"seed","provenance":"p",
"files":[{"path":"go.mod","content":"module m\n"}],
"turns":[{"prompt_id":"p1","final_message":"a","calls":[{"tool":"Edit","input":{"file_path":"{project}/a.go"},"outcome":"unrecorded"}]},
{"prompt_id":"p2","final_message":"Done."}]}`

// withTurns is goodTurns with its turns array replaced.
func withTurns(turns string) string {
	head, _, _ := strings.Cut(goodTurns, `"turns":[`)
	return head + `"turns":` + turns + `}`
}

// nTurns is a turns array of n turns with no calls, prompt ids t1..tn.
func nTurns(n int) string {
	var ts []string
	for i := 1; i <= n; i++ {
		ts = append(ts, fmt.Sprintf(`{"prompt_id":"t%d","final_message":""}`, i))
	}
	return "[" + strings.Join(ts, ",") + "]"
}

// detectorOf is the detector a test document names, so a refusal is read
// in that detector's directory and fails for the rule it breaks.
func detectorOf(doc string) string {
	if strings.Contains(doc, `"silent_failure"`) {
		return "silent_failure"
	}
	return "declaration_without_execution"
}

// TestDetectorPrecision_ParseTurns: the turns shape is accepted with its
// label held to the last turn or the one label_turn names, and each rule of
// it refuses a fixture that differs from the accepted one in one place.
func TestDetectorPrecision_ParseTurns(t *testing.T) {
	f, err := parseFixture("declaration_without_execution", []byte(goodTurns))
	if err != nil || len(f.turns()) != 2 || f.labelTurn() != 1 || f.turns()[0].PromptID != "p1" || len(f.Files) != 1 {
		t.Fatalf("the accepted turns fixture was refused or misread: %+v, %v", f, err)
	}
	// The single-turn shape is one turn under the prompt id it always had.
	single, err := parseFixture("silent_failure", []byte(goodFixture))
	if err != nil || len(single.turns()) != 1 || single.turns()[0].PromptID != fixturePromptID || single.labelTurn() != 0 ||
		single.turns()[0].FinalMessage != "m" || len(single.turns()[0].Calls) != 1 {
		t.Fatalf("the single-turn shape is not one turn under %q: %+v, %v", fixturePromptID, single.turns(), err)
	}
	mut := func(old, new string) string { return strings.Replace(goodTurns, old, new, 1) }
	repeat200 := `{"tool":"Bash","input":{},"outcome":"ok","repeat":200}`
	for _, ok := range []struct {
		name, doc string
		label     int
	}{
		{"label_turn 1", mut(`"provenance":"p"`, `"provenance":"p","label_turn":1`), 0},
		{"label_turn 2, the last", mut(`"provenance":"p"`, `"provenance":"p","label_turn":2`), 1},
		{"turns at the cap", withTurns(nTurns(maxFixtureTurns)), maxFixtureTurns - 1},
		{"a prompt id at its cap", mut(`"p1"`, `"`+strings.Repeat("a", maxPromptIDBytes)+`"`), 1},
		{"a UUID prompt id", mut(`"p1"`, `"00000000-0000-4000-8000-000000000101"`), 1},
		{"an interrupted call", mut(`"outcome":"unrecorded"`, `"outcome":"interrupted"`), 1},
		// A call's input is the tool's, and its keys are not the grammar's.
		{"an input key in any case", mut(`"file_path"`, `"File_Path"`), 1},
		{"calls across turns at the cap", withTurns(`[{"prompt_id":"a","final_message":"","calls":[` + repeat200 + `]},` +
			`{"prompt_id":"b","final_message":"","calls":[` + repeat200 + `]}]`), 1},
	} {
		f, err := parseFixture("declaration_without_execution", []byte(ok.doc))
		if err != nil || f.labelTurn() != ok.label {
			t.Errorf("%s: refused or misread (label turn %d, want %d): %v", ok.name, f.labelTurn(), ok.label, err)
		}
	}
	tooMany := strings.TrimSuffix(strings.Repeat(`{"tool":"Bash","input":{},"outcome":"ok"},`, maxFixtureCalls+1), ",")
	for name, tc := range map[string]struct{ doc, want string }{
		"turns and calls":           {mut(`"turns":[`, `"calls":[],"turns":[`), "turns and calls are alternatives"},
		"turns and final_message":   {mut(`"turns":[`, `"final_message":"","turns":[`), "turns and final_message are alternatives"},
		"turns and capture":         {mut(`"turns":[`, `"capture":"`+realCaptureName+`","turns":[`), "turns and capture are alternatives"},
		"empty turns":               {withTurns(`[]`), "turns is empty"},
		"turns over the cap":        {withTurns(nTurns(maxFixtureTurns + 1)), "17 turns, over the 16 cap"},
		"empty prompt id":           {mut(`"p1"`, `""`), `prompt_id ""`},
		"no prompt id":              {mut(`"prompt_id":"p1",`, ""), `prompt_id ""`},
		"prompt id with a space":    {mut(`"p1"`, `"p 1"`), "outside letters"},
		"prompt id over its cap":    {mut(`"p1"`, `"`+strings.Repeat("a", maxPromptIDBytes+1)+`"`), "over 128 bytes"},
		"prompt id twice":           {mut(`"p2"`, `"p1"`), "not the turn's own"},
		"the trailing prompt id":    {mut(`"p2"`, `"`+trailingPromptID+`"`), "not the turn's own"},
		"turn final_message NUL":    {mut(`"Done."`, `"Done.\u0000"`), "turn 2 final_message"},
		"turn call names no tool":   {mut(`"tool":"Edit"`, `"tool":""`), "turn 1 call 0 names no tool"},
		"turn call bad outcome":     {mut(`"outcome":"unrecorded"`, `"outcome":"vanished"`), "turn 1 call 0 outcome"},
		"turn calls over the cap":   {withTurns(`[{"prompt_id":"a","final_message":"","calls":[` + tooMany + `]}]`), "turn 1 401 calls, over"},
		"calls across turns expand": {withTurns(`[{"prompt_id":"a","final_message":"","calls":[` + repeat200 + `]},{"prompt_id":"b","final_message":"","calls":[` + repeat200 + `,{"tool":"Bash","input":{},"outcome":"ok"}]}]`), "expand"},
		"label_turn without turns":  {strings.Replace(goodFixture, `"provenance":"p"`, `"provenance":"p","label_turn":1`, 1), "label_turn 1 names no turn"},
		"label_turn 0":              {mut(`"provenance":"p"`, `"provenance":"p","label_turn":0`), "label_turn 0 names no turn"},
		"label_turn past the last":  {mut(`"provenance":"p"`, `"provenance":"p","label_turn":3`), "label_turn 3 names no turn"},
		"label_turn negative":       {mut(`"provenance":"p"`, `"provenance":"p","label_turn":-1`), "label_turn -1 names no turn"},
		"unknown key in a turn":     {mut(`"prompt_id":"p2"`, `"prompt_id":"p2","prompt":"hi"`), "unknown field"},
		// encoding/json would decode each of these into its field.
		"Calls beside turns":                    {mut(`"turns":[`, `"Calls":[{"tool":"Edit","input":{},"outcome":"failed"}],"turns":[`), `key "Calls" is not spelled`},
		"Turns beside calls":                    {strings.Replace(goodFixture, `"calls"`, `"Turns":[{"prompt_id":"prompt-next","final_message":""}],"calls"`, 1), `key "Turns" is not spelled`},
		"Label_Turn":                            {mut(`"provenance":"p"`, `"provenance":"p","Label_Turn":7`), `key "Label_Turn" is not spelled`},
		"a turn's Prompt_ID":                    {mut(`"prompt_id":"p2"`, `"Prompt_ID":"p2"`), `key "Prompt_ID" in turns[1] is not spelled`},
		"a call's OUTCOME":                      {mut(`"outcome":"unrecorded"`, `"OUTCOME":"unrecorded"`), `key "OUTCOME" in turns[0].calls[0] is not spelled`},
		"an interrupted call with an exit code": {mut(`"outcome":"unrecorded"`, `"outcome":"interrupted","exit_code":1`), "only a failed call has one"},
	} {
		_, err := parseFixture(detectorOf(tc.doc), []byte(tc.doc))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// TestDetectorPrecision_ParseFiles: the project files are accepted at their
// caps and in the paths a project holds, and refused past a cap, outside the
// project, inside .git, twice, beside a capture, or named by a run that has
// none.
func TestDetectorPrecision_ParseFiles(t *testing.T) {
	withFiles := func(files string) string {
		return strings.Replace(goodTurns, `[{"path":"go.mod","content":"module m\n"}]`, files, 1)
	}
	file := func(p, content string) string {
		b, err := json.Marshal(projectFile{Path: p, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	many := func(n int) string {
		var fs []string
		for i := 0; i < n; i++ {
			fs = append(fs, file(fmt.Sprintf("f%d", i), ""))
		}
		return "[" + strings.Join(fs, ",") + "]"
	}
	for name, doc := range map[string]string{
		"a nested path":          withFiles("[" + file("internal/a/b.go", "package b\n") + "]"),
		"a dot file beside .git": withFiles("[" + file(".gitignore", "*.o\n") + "," + file(".github/workflows/ci.yaml", "on: push\n") + "]"),
		"files at the cap":       withFiles(many(maxProjectFiles)),
		"content at its cap":     withFiles("[" + file("big", strings.Repeat("x", maxProjectFileBytes)) + "]"),
		"a path at its cap":      withFiles("[" + file(strings.Repeat("p", maxProjectPathBytes), "") + "]"),
		"an empty file":          withFiles("[" + file("Makefile", "") + "]"),
	} {
		if _, err := parseFixture("declaration_without_execution", []byte(doc)); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
	noProject := strings.Replace(goodFixture, `{"command":"grep x y"}`, `{"command":"grep x {project}/y"}`, 1)
	for name, tc := range map[string]struct{ doc, want string }{
		"no files":                {withFiles("[]"), "files is empty"},
		"files over the cap":      {withFiles(many(maxProjectFiles + 1)), "17 files, over the 16 cap"},
		"an empty path":           {withFiles("[" + file("", "") + "]"), "is empty, over"},
		"an absolute path":        {withFiles("[" + file("/etc/passwd", "") + "]"), "not a clean relative path"},
		"a parent path":           {withFiles("[" + file("../x", "") + "]"), "not a clean relative path"},
		"a path through a parent": {withFiles("[" + file("a/../b", "") + "]"), "not a clean relative path"},
		"a dot path":              {withFiles("[" + file("./a", "") + "]"), "not a clean relative path"},
		"a doubled separator":     {withFiles("[" + file("a//b", "") + "]"), "not a clean relative path"},
		"a trailing separator":    {withFiles("[" + file("a/", "") + "]"), "not a clean relative path"},
		"the project itself":      {withFiles("[" + file(".", "") + "]"), "not a clean relative path"},
		".git":                    {withFiles("[" + file(".git", "") + "]"), "inside .git"},
		"inside .git":             {withFiles("[" + file(".git/config", "") + "]"), "inside .git"},
		"a backslash":             {withFiles("[" + file(`a\b`, "") + "]"), "a backslash"},
		"a colon":                 {withFiles("[" + file("c:x", "") + "]"), "a colon"},
		"NUL in a path":           {withFiles("[" + file("a\x00b", "") + "]"), "NUL"},
		"a path over its cap":     {withFiles("[" + file(strings.Repeat("p", maxProjectPathBytes+1), "") + "]"), "over 256 bytes"},
		"a path twice":            {withFiles("[" + file("go.mod", "") + "," + file("go.mod", "") + "]"), "given twice"},
		"a path twice by case":    {withFiles("[" + file("go.mod", "") + "," + file("Go.mod", "") + "]"), "given twice, in some letter case"},
		"a file under a file":     {withFiles("[" + file("a", "") + "," + file("A/b", "") + "]"), `"a" is a directory file "A/b" is under`},
		"Files escaping":          {strings.Replace(goodTurns, `"files"`, `"Files"`, 1), `key "Files" is not spelled`},
		"a file's Path":           {withFiles(`[{"Path":"../../x","content":""}]`), `key "Path" in files[0] is not spelled`},
		"{project} in an output":  {strings.Replace(goodTurns, `"outcome":"unrecorded"`, `"outcome":"failed","output":"{project}/a.go:3: undefined"`, 1), "turn 1 call 0 output names {project}"},
		"{project} in a message":  {strings.Replace(goodTurns, `"Done."`, `"Edited {project}/a.go."`, 1), "final_message names {project}"},
		"content over its cap":    {withFiles("[" + file("big", strings.Repeat("x", maxProjectFileBytes+1)) + "]"), "content is over"},
		"NUL in content":          {withFiles("[" + file("go.mod", "a\x00") + "]"), "holds NUL"},
		"an unknown file key":     {withFiles(`[{"path":"go.mod","content":"","mode":"0755"}]`), "unknown field"},
		"capture and files": {`{"detector":"declaration_without_execution","label":"true_negative","reason":"r","labelled_by":"seed","provenance":"p","capture":"` +
			realCaptureName + `","final_message":"","files":[` + file("go.mod", "") + `]}`, "capture and files are alternatives"},
		"{project} with no files":       {noProject, "names {project}"},
		"{project} deep in an input":    {strings.Replace(goodFixture, `{"command":"grep x y"}`, `{"edits":[{"file_path":"{project}/a"}]}`, 1), "names {project}"},
		"{project} in a turn, no files": {strings.Replace(goodTurns, `"files":[{"path":"go.mod","content":"module m\n"}],`, "", 1), "turn 1 call 0 input names {project}"},
	} {
		_, err := parseFixture(detectorOf(tc.doc), []byte(tc.doc))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}
	// The positive twin of the {project} refusals: the same input, with files.
	withProjectFiles := strings.Replace(noProject, `"calls"`, `"files":[`+file("go.mod", "")+`],"calls"`, 1)
	if _, err := parseFixture("silent_failure", []byte(withProjectFiles)); err != nil {
		t.Errorf("{project} in a run with files was refused: %v", err)
	}
}

const goodLine = `{"reason":"r","provenance":"p","final_message":"m",
"calls":[{"tool":"Bash","input":{"command":"go test ./..."},"outcome":"ok","output":"ok\n"}]}`

// TestDetectorPrecision_ParseLineFixture: the line corpus takes the run
// grammar whole and none of the label fields, and asks a reason and a
// provenance of every fixture.
func TestDetectorPrecision_ParseLineFixture(t *testing.T) {
	f, err := parseLineFixture([]byte(goodLine))
	if err != nil || len(f.turns()) != 1 || f.turns()[0].Calls[0].Outcome != "ok" || !f.probeStart() {
		t.Fatalf("the accepted line fixture was refused or misread: %+v, %v", f, err)
	}
	head, _, _ := strings.Cut(goodTurns, `"files"`)
	turns := strings.Replace(goodTurns, head, `{"reason":"r","provenance":"p",`, 1)
	if f, err := parseLineFixture([]byte(turns)); err != nil || len(f.turns()) != 2 || len(f.Files) != 1 {
		t.Errorf("a turns line fixture with files was refused or misread: %+v, %v", f, err)
	}
	mut := func(old, new string) string { return strings.Replace(goodLine, old, new, 1) }
	for name, tc := range map[string]struct{ doc, want string }{
		"a detector":        {mut(`"reason"`, `"detector":"silent_failure","reason"`), "unknown field"},
		"a label":           {mut(`"reason"`, `"label":"true_positive","reason"`), "unknown field"},
		"a labeller":        {mut(`"reason"`, `"labelled_by":"seed","reason"`), "unknown field"},
		"a class":           {mut(`"reason"`, `"fp_class":"other","reason"`), "unknown field"},
		"a label turn":      {strings.Replace(turns, `"reason"`, `"label_turn":1,"reason"`, 1), "unknown field"},
		"empty reason":      {mut(`"reason":"r"`, `"reason":" "`), "reason is empty"},
		"empty provenance":  {mut(`"provenance":"p"`, `"provenance":""`), "provenance is empty"},
		"NUL in reason":     {mut(`"reason":"r"`, `"reason":"r\u0000"`), "NUL"},
		"reason over cap":   {mut(`"reason":"r"`, `"reason":"`+strings.Repeat("r", maxFieldBytes+1)+`"`), "over its cap"},
		"second document":   {goodLine + goodLine, "more than one JSON document"},
		"over the byte cap": {strings.Repeat(" ", maxFixtureBytes) + goodLine, "over the"},
		"turns and calls":   {strings.Replace(turns, `"turns"`, `"calls":[],"turns"`, 1), "alternatives"},
		"bad outcome":       {mut(`"outcome":"ok"`, `"outcome":"maybe"`), "outcome"},
		"not UTF-8":         {mut(`"m"`, "\"m\xff\""), "UTF-8"},
		"a Reason":          {mut(`"reason":"r"`, `"reason":"r","Reason":"s"`), `key "Reason" is not spelled`},
		"Turns beside calls": {strings.Replace(goodLine, `"calls"`, `"Turns":[{"prompt_id":"a","final_message":""}],"calls"`, 1),
			`key "Turns" is not spelled`},
	} {
		if _, err := parseLineFixture([]byte(tc.doc)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// TestDetectorPrecision_EveryFixtureParses: every committed fixture of both
// corpora is read by its parser in the default build, so a fixture the
// grammar refuses fails every push, not only the perf job that replays it.
func TestDetectorPrecision_EveryFixtureParses(t *testing.T) {
	detectors := 0
	for _, spec := range detectorInventory {
		_, fs := loadDetectorFixtures(t, spec.name())
		detectors += len(fs)
	}
	_, lines := loadLineFixtures(t)
	// The positive twin of a parse that read nothing: both corpora are there.
	if detectors == 0 || len(lines) == 0 {
		t.Fatalf("read %d detector fixtures and %d line fixtures", detectors, len(lines))
	}
}

// TestReplaySnapshot_RenderSaysEverything: a turn's block names its prompt,
// the prompt the digest read and every detector that fired, says nothing
// for an event that printed nothing and why for one not sent, lists the
// call events that printed, and writes stdout key by key, a failed exit
// with its stderr, and stdout that is not JSON as text.
func TestReplaySnapshot_RenderSaysEverything(t *testing.T) {
	tr := turnReplay{
		promptID: "p1",
		before:   []printed{{label: "session start", unsent: "probe_start is false"}},
		calls:    2,
		atCalls: []printed{{label: "call 2 Bash PostToolUse", res: result{
			stdout: `{"systemMessage":"a\nb","hookSpecificOutput":{"additionalContext":"","n":3,"ok":true,"list":[],"obj":{}}}` + "\n"}}},
		stop: printed{label: "stop"},
		next: printed{label: "next prompt x", res: result{exitCode: 2, stderr: "boom\n", stdout: "not json"}},
	}
	tr.digest.PromptID = "p0"
	tr.digest.Coverage.State = store.StateVerified
	tr.digest.SilentFailures.Fires = true
	tr.digest.MaskedRuns.Fires = true
	want := `=== lines/x turn 1 of 2
prompt: p1
digest read: p0
fired: silent_failure, masked_run
session start: not sent: probe_start is false
calls: 2; call events that printed: 1
call 2 Bash PostToolUse:
  hookSpecificOutput.additionalContext: ""
  hookSpecificOutput.list: []
  hookSpecificOutput.n: 3
  hookSpecificOutput.obj: {}
  hookSpecificOutput.ok: true
  systemMessage:
    | a
    | b
stop:
  nothing
next prompt x:
  exit: 2
  stderr:
    | boom
    |
  stdout, not JSON:
    | not json
`
	if got := renderTurn("lines/x", 0, 2, tr); got != want {
		t.Errorf("renderTurn:\n--- got\n%s--- want\n%s", got, want)
	}
	var b strings.Builder
	renderPrinted(&b, printed{label: "e", res: result{stdout: `"hi"`}})
	if b.String() != "e:\n  stdout:\n    | hi\n" {
		t.Errorf("a JSON scalar on stdout rendered as %q", b.String())
	}
	// Nothing fired and no prompt read: both say none, never an empty field.
	quiet := turnReplay{stop: printed{label: "stop"}, next: printed{label: "next prompt y"}}
	quiet.digest.Coverage.State = store.StateVerified
	if got := renderTurn("lines/q", 1, 2, quiet); !strings.Contains(got, "prompt: none\ndigest read: none\nfired: none\ncalls: 0; call events that printed: 0\nstop:\n  nothing\n") {
		t.Errorf("a quiet turn rendered as\n%s", got)
	}
}

// TestReplaySnapshot_NormaliserNamesEveryPath: each named path, its tail as
// the brief shortens it, and the longer of two nested paths become their
// placeholders; a temporary path nobody named is reported, and a normalised
// text is not.
func TestReplaySnapshot_NormaliserNamesEveryPath(t *testing.T) {
	n := newReplayNorm("/r/T1/001", map[string]string{"/r/T1/001": "{store}", "/r/T1/002": "{cwd}"}).with("/r/T1/002/sub", "{project}")
	in := "a /r/T1/002/sub/x b /r/T1/001 c .../002 d .../sub e .../T1/001"
	want := "a {project}/x b {store} c {cwd} d {project} e {store}"
	got := n.apply(in)
	if got != want {
		t.Errorf("apply(%q) = %q, want %q", in, got, want)
	}
	if root, ok := n.leak(got); ok {
		t.Errorf("a normalised text was reported as leaking %s", root)
	}
	if root, ok := n.leak(n.apply("f /r/T1/003/y")); !ok || root != "/r/T1" {
		t.Errorf("an unnamed temporary path was not reported: %q %v", root, ok)
	}
}

// TestReplaySnapshot_ParseAndCompare: a formatted snapshot reads back to its
// blocks; a header given twice is refused; a changed block and a new one are
// problems, and a committed block no fixture produced is one only when every
// fixture was replayed.
func TestReplaySnapshot_ParseAndCompare(t *testing.T) {
	a, b := "=== a turn 1 of 1\nfired: none\n", "=== b turn 1 of 1\nstop:\n  systemMessage:\n    | x\n"
	pre, blocks, order, err := parseSnapshot(formatSnapshot([]string{a, b}))
	if err != nil || pre != snapshotPreamble || strings.Join(order, ",") != "=== a turn 1 of 1,=== b turn 1 of 1" ||
		blocks[blockHeader(a)] != a || blocks[blockHeader(b)] != b {
		t.Fatalf("round trip: preamble kept %v, order %q, blocks %q, err %v", pre == snapshotPreamble, order, blocks, err)
	}
	if _, _, _, err := parseSnapshot(formatSnapshot([]string{a, a})); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("a header given twice: got %v", err)
	}
	if p := snapshotProblems([]string{a, b}, blocks, true); len(p) != 0 {
		t.Errorf("the same blocks are problems: %q", p)
	}
	changed := strings.Replace(b, "| x", "| y", 1)
	for name, tc := range map[string]struct {
		got      []string
		complete bool
		want     []string
	}{
		"a changed block":             {[]string{a, changed}, true, []string{"=== b turn 1 of 1: differs"}},
		"a new block":                 {[]string{a, b, "=== c turn 1 of 1\n"}, true, []string{"=== c turn 1 of 1: not in the snapshot"}},
		"a block gone, every fixture": {[]string{a}, true, []string{"=== b turn 1 of 1: in the snapshot, and no fixture produced it"}},
		"a block not run, a filter":   {[]string{a}, false, nil},
		"a changed block under -run":  {[]string{changed}, false, []string{"=== b turn 1 of 1: differs"}},
		"nothing produced, a filter":  {nil, false, nil},
		"nothing produced, every one": {nil, true, []string{"=== a turn 1 of 1: in the snapshot", "=== b turn 1 of 1: in the snapshot"}},
	} {
		p := snapshotProblems(tc.got, blocks, tc.complete)
		if len(p) != len(tc.want) {
			t.Errorf("%s: %d problems %q, want %d", name, len(p), p, len(tc.want))
			continue
		}
		for i, w := range tc.want {
			if !strings.HasPrefix(p[i], w) {
				t.Errorf("%s: problem %q, want it to start %q", name, p[i], w)
			}
		}
	}
}
