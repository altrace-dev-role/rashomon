package acceptance

// Detector precision: the fixture format, the labelling sheet and the
// statistics. This file is the pure half -- no process, no store -- so every
// rule in it is tested without the binary; detector_precision_test.go is the
// half that replays a fixture through the built binary and reads which
// detectors fired.
//
// A detector here is anything that puts a judgement in front of a user: a
// sentence of the end-of-turn line, or a report section that says a thing was
// found. Its precision is the share of its fires that a person, reading the
// run and the final message, agrees were right. Nothing else counts as a
// measurement:
//
//   - a fixture is a minimal recorded run (the hook payloads a turn produced,
//     written compactly as calls) and the transcript excerpt the detector
//     reads (the turn's final assistant message), scrubbed, under
//     test/corpus/detectors/<detector>/. A run of several turns gives turns,
//     each with its prompt id, calls and final message; a run that needs a
//     project to read (a go.mod, a Makefile) gives files;
//   - its label is the ground truth for the run's last turn, or the turn
//     label_turn names: true_positive (the detector
//     fires and is right), false_positive (it fires and is wrong),
//     true_negative (it stays silent and is right) or false_negative (it
//     stays silent and should not have). The harness holds the label to what
//     the detector really does, so a label that presupposes a fire which does
//     not happen fails the build;
//   - labelled_by names who decided. "seed" means the label was derived from
//     a written rule by whoever built the fixture, with no person confirming
//     it: a seed label is checked, printed and never counted. A person counts
//     only by replacing "seed" with their own name after reading the fixture;
//   - a real fire seen outside a fixture goes in the labelling sheet
//     (test/corpus/detectors/labelling-sheet.tsv), one row per fire, and
//     counts the same way once its labelled_by is a person's.
//
// A label reads one detector and says nothing about the rest of what was
// printed: a change that silences a detector by printing something worse
// keeps every label green. So every replay is also written whole, turn by
// turn, into a committed snapshot (detector_replay_test.go), and runs whose
// subject is the printed line rather than a detector live unlabelled under
// test/corpus/lines/, held to that snapshot alone.
//
// A detector is measured once it has minLabelledFires counted fires. Below
// that its row says unmeasured, however good its interval looks.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/learn"
)

// The labels. The first three are the three a precision figure is built from;
// false_negative is recorded so a blind spot has a fixture, and is never a
// fire.
const (
	labelTruePositive  = "true_positive"
	labelFalsePositive = "false_positive"
	labelTrueNegative  = "true_negative"
	labelFalseNegative = "false_negative"
)

// labelledBySeed marks a label nobody has confirmed.
const labelledBySeed = "seed"

// minLabelledFires is the number of counted fires below which a detector is
// unmeasured. A person-confirmed label counts; a seed label does not.
const minLabelledFires = 30

// Caps on what the parsers read, named so the over-cap path is tested.
const (
	maxFixtureBytes = 1 << 20
	maxFixtureCalls = 400
	maxCallRepeat   = 400
	maxSheetBytes   = 4 << 20
	maxSheetRows    = 20000
	maxFieldBytes   = 64 << 10
	// A run's turns, and its project: a handful of small files is what a
	// gate is inferred from (a go.mod, a Makefile, a CI file), and a cap
	// keeps a fixture from becoming a vendored tree.
	maxFixtureTurns     = 16
	maxPromptIDBytes    = 128
	maxProjectFiles     = 16
	maxProjectPathBytes = 256
	maxProjectFileBytes = 16 << 10
)

// fixturePromptID is the prompt id the single-turn shape's calls are sent
// with (defaultPayload's), and trailingPromptID the one the replay's last
// UserPromptSubmit carries, after the last turn: a prompt no turn of the
// run has, so it opens a new turn as the next real prompt would.
const (
	fixturePromptID  = "prompt-1"
	trailingPromptID = "prompt-next"
)

// projectToken, in a string of a call input, is replaced by the project
// directory the run's files were written to. That directory is a fresh
// temporary one, so a fixture cannot spell it; it spells the token.
const projectToken = "{project}"

// fpClasses is the closed set of false-positive classes. A false positive
// names one, so the classes can be counted and the fixes ordered by how many
// fires each one costs; "other" is for a class not yet named, and its reason
// says what it was.
var fpClasses = []string{
	"non_english",
	"recovered_command",
	"empty_search",
	"status_query",
	"denied_call",
	"not_a_claim",
	"pass_word_other_sense",
	"output_shows_pass",
	"external_fix",
	"test_file_error",
	"session_predates_install",
	"other",
}

// callOutcomes is the closed set of ways a call can end in a fixture.
//
//	ok         PostToolUse: the call ran and the tool reported success.
//	failed     PostToolUseFailure: the call ran and failed (a shell call
//	           carries "Exit code N" and its output, as Claude Code sends it).
//	interrupted PostToolUseFailure with is_interrupt true: the person stopped
//	           the call while it ran, which Claude Code reports on the
//	           failure event.
//	denied     a declaration and nothing after it: the user refused the call.
//	unrecorded a declaration and nothing after it: the call started and no
//	           ending arrived. The record cannot tell this from denied, which
//	           is why both exist: the fixture says which it was.
var callOutcomes = []string{"ok", "failed", "interrupted", "denied", "unrecorded"}

// fixtureCall is one tool call of a fixture's turn, in order.
type fixtureCall struct {
	Tool     string         `json:"tool"`
	Input    map[string]any `json:"input"`
	Outcome  string         `json:"outcome"`
	ExitCode int            `json:"exit_code,omitempty"`
	// Output is what a reader of the run would have seen of the call: a
	// shell call's stdout, or a failed call's error text. The recorder keeps
	// none of it; it is here so the person labelling can judge.
	Output string `json:"output,omitempty"`
	// Repeat replays the call that many times with distinct tool_use_ids.
	Repeat int `json:"repeat,omitempty"`
}

// fixtureTurn is one turn of a multi-turn run: the prompt id Claude Code sent
// with its calls, the calls in order, and the final message its Stop carried.
type fixtureTurn struct {
	PromptID     string        `json:"prompt_id"`
	FinalMessage string        `json:"final_message"`
	Calls        []fixtureCall `json:"calls,omitempty"`
}

// projectFile is one file written into the run's project directory before
// the session starts, so what rashomon reads from a project (the test
// command a go.mod or a Makefile implies) is there to be read.
type projectFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// fixtureRun is the part of a fixture that says what happened, shared by the
// detector corpus and the line corpus: a committed capture, or one turn's
// calls and final message, or turns. Files, when given, make a project
// directory that the payloads' cwd names.
type fixtureRun struct {
	// Capture, when set, names a committed capture under test/fixtures
	// (relative to it) to replay whole instead of Calls.
	Capture string `json:"capture,omitempty"`
	// ProbeStart false leaves the session's start probe unrecorded.
	ProbeStart   *bool         `json:"probe_start,omitempty"`
	FinalMessage string        `json:"final_message"`
	Calls        []fixtureCall `json:"calls,omitempty"`
	Turns        []fixtureTurn `json:"turns,omitempty"`
	Files        []projectFile `json:"files,omitempty"`
}

func (r fixtureRun) probeStart() bool { return r.ProbeStart == nil || *r.ProbeStart }

// turns is the run as the replay walks it. The single-turn shape is one turn
// under fixturePromptID, the prompt id its calls were always sent with.
func (r fixtureRun) turns() []fixtureTurn {
	if len(r.Turns) > 0 {
		return r.Turns
	}
	return []fixtureTurn{{PromptID: fixturePromptID, FinalMessage: r.FinalMessage, Calls: r.Calls}}
}

// detectorFixture is one labelled run.
type detectorFixture struct {
	Detector   string `json:"detector"`
	Label      string `json:"label"`
	Reason     string `json:"reason"`
	LabelledBy string `json:"labelled_by"`
	FPClass    string `json:"fp_class,omitempty"`
	// Provenance says where the run came from: which committed capture or
	// test payload it reuses, or the rule it was constructed from.
	Provenance string `json:"provenance"`
	// LabelTurn names, counting from 1, the turn of Turns the label is held
	// to. Left out, it is the last: the turn whose Stop a person read last.
	LabelTurn int `json:"label_turn,omitempty"`
	fixtureRun
}

// labelTurn is the index into turns() of the turn the label is held to.
func (f detectorFixture) labelTurn() int {
	if f.LabelTurn > 0 {
		return f.LabelTurn - 1
	}
	return len(f.turns()) - 1
}

// lineFixture is a run whose subject is what rashomon printed rather than
// whether one detector fired: the end-of-turn line, a note at a call, the
// next prompt's note. It has no label, because what it is held to is the
// replay snapshot, where a change to any of those shows as a diff; its
// reason says what the run shows and why it is in the corpus.
type lineFixture struct {
	Reason     string `json:"reason"`
	Provenance string `json:"provenance"`
	fixtureRun
}

// isFire reports a label that presupposes the detector fired.
func isFireLabel(label string) bool {
	return label == labelTruePositive || label == labelFalsePositive
}

func inSet(s string, set []string) bool {
	for _, v := range set {
		if s == v {
			return true
		}
	}
	return false
}

// validLabel reports one of the four labels.
func validLabel(s string) bool {
	return inSet(s, []string{labelTruePositive, labelFalsePositive, labelTrueNegative, labelFalseNegative})
}

// checkLabelling is the rules a label, its reason, its author and its false-
// positive class answer to, shared by the fixture and the sheet so a row
// cannot say in one place what the other refuses.
func checkLabelling(label, reason, by, class string) error {
	switch {
	case !validLabel(label):
		return fmt.Errorf("label %q is none of true_positive, false_positive, true_negative, false_negative", label)
	case strings.TrimSpace(reason) == "":
		return errors.New("reason is empty: a label without its reason cannot be confirmed")
	case strings.TrimSpace(by) == "":
		return errors.New(`labelled_by is empty: "seed" for an unconfirmed label, or the person's name`)
	case label == labelFalsePositive && class == "":
		return errors.New("a false_positive names its fp_class")
	case label != labelFalsePositive && class != "":
		return fmt.Errorf("fp_class %q on a %s: only a false_positive has a class", class, label)
	case class != "" && !inSet(class, fpClasses):
		return fmt.Errorf("fp_class %q is not in the closed set %v", class, fpClasses)
	}
	for _, v := range []string{reason, by} {
		if len(v) > maxFieldBytes || !utf8.ValidString(v) || strings.ContainsAny(v, "\x00") {
			return errors.New("a field is over its cap, is not UTF-8, or holds NUL")
		}
	}
	return nil
}

// decodeFixture is the strict decoding both corpora share: within the byte
// cap, UTF-8, one JSON document, no key the type does not name. It returns
// the keys the document gave, because the alternatives that exclude each
// other are told apart by presence: a final_message left out and an empty one
// decode alike.
func decodeFixture(b []byte, into any) (map[string]json.RawMessage, error) {
	if len(b) > maxFixtureBytes {
		return nil, fmt.Errorf("fixture is %d bytes, over the %d cap", len(b), maxFixtureBytes)
	}
	// The decoder repairs invalid UTF-8 into U+FFFD, so the bytes are checked
	// before it sees them: a fixture holds what its author wrote.
	if !utf8.Valid(b) {
		return nil, errors.New("fixture is not UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return nil, fmt.Errorf("decoding: %w", err)
	}
	if dec.More() {
		return nil, errors.New("more than one JSON document")
	}
	if err := exactKeys(b, reflect.TypeOf(into).Elem(), ""); err != nil {
		return nil, err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return nil, fmt.Errorf("decoding: %w", err)
	}
	return keys, nil
}

// exactKeys refuses, at every level of the document, a key that is not
// spelled as a json tag of the type it decodes into. encoding/json matches a
// key to a field whatever its letter case, and DisallowUnknownFields refuses
// none of those, so "Files" decoded into Files while the rules read by the
// key's presence ("files") never saw it: a path that escaped the project was
// written, and calls beside turns were dropped unread. Presence and decoding
// now cannot disagree. A value of a map type (a call's input) is the tool's
// own and is not read.
func exactKeys(raw []byte, t reflect.Type, at string) error {
	switch t.Kind() {
	case reflect.Pointer:
		return exactKeys(raw, t.Elem(), at)
	case reflect.Slice:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil // not an array: the decoder has judged it already
		}
		for i, it := range items {
			if err := exactKeys(it, t.Elem(), fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		var doc map[string]json.RawMessage
		if json.Unmarshal(raw, &doc) != nil {
			return nil
		}
		fields := jsonFields(t)
		for k, v := range doc {
			ft, ok := fields[k]
			if !ok {
				return fmt.Errorf("key %q%s is not spelled as the grammar spells a key (letter case counts)", k, inKey(at))
			}
			if err := exactKeys(v, ft, at+"."+k); err != nil {
				return err
			}
		}
	}
	return nil
}

// inKey is where exactKeys found a key, for its error: nowhere at the top.
func inKey(at string) string {
	if at == "" {
		return ""
	}
	return " in " + strings.TrimPrefix(at, ".")
}

// jsonFields is t's keys as its json tags spell them, an untagged embedded
// struct's merged in, as encoding/json reads them.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if f.Anonymous && tag == "" && f.Type.Kind() == reflect.Struct {
			for k, v := range jsonFields(f.Type) {
				out[k] = v
			}
			continue
		}
		if !f.IsExported() || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
	return out
}

// parseFixture reads one fixture file of the detector's directory. Strict:
// an unknown key, a second document or a field out of its set is an error,
// and nothing is a default except probe_start and label_turn.
func parseFixture(detector string, b []byte) (detectorFixture, error) {
	var f detectorFixture
	keys, err := decodeFixture(b, &f)
	if err != nil {
		return f, err
	}
	if f.Detector != detector {
		return f, fmt.Errorf("detector %q in a %q directory", f.Detector, detector)
	}
	if err := checkLabelling(f.Label, f.Reason, f.LabelledBy, f.FPClass); err != nil {
		return f, err
	}
	if strings.TrimSpace(f.Provenance) == "" {
		return f, errors.New("provenance is empty: say which capture, payload or rule the run came from")
	}
	if err := f.check(keys); err != nil {
		return f, err
	}
	if _, ok := keys["label_turn"]; (ok || f.LabelTurn != 0) && (len(f.Turns) == 0 || f.LabelTurn < 1 || f.LabelTurn > len(f.Turns)) {
		return f, fmt.Errorf("label_turn %d names no turn: it counts the turns of turns from 1", f.LabelTurn)
	}
	return f, nil
}

// parseLineFixture reads one fixture of the line corpus: the detector
// fixture's grammar without its label fields, which are unknown keys here,
// so a detector fixture filed in the wrong corpus is refused, not half read.
func parseLineFixture(b []byte) (lineFixture, error) {
	var f lineFixture
	keys, err := decodeFixture(b, &f)
	if err != nil {
		return f, err
	}
	if strings.TrimSpace(f.Reason) == "" {
		return f, errors.New("reason is empty: say what the run shows and why it is in the corpus")
	}
	if strings.TrimSpace(f.Provenance) == "" {
		return f, errors.New("provenance is empty: say which capture, payload or rule the run came from")
	}
	for _, v := range []string{f.Reason, f.Provenance} {
		if len(v) > maxFieldBytes || strings.Contains(v, "\x00") {
			return f, errors.New("reason or provenance is over its cap or holds NUL")
		}
	}
	return f, f.check(keys)
}

// check holds the run to its grammar; keys is what the document gave.
func (r fixtureRun) check(keys map[string]json.RawMessage) error {
	if err := checkMessage(r.FinalMessage); err != nil {
		return err
	}
	if r.Capture != "" && r.Capture != realCaptureName {
		return fmt.Errorf("capture %q is not a replayable capture (%q)", r.Capture, realCaptureName)
	}
	if r.Capture != "" && len(r.Calls) > 0 {
		return errors.New("capture and calls are alternatives: give one (no calls at all is a turn that used no tool)")
	}
	// Presence or a value: exactKeys makes the two agree, and either alone
	// still brings its rules.
	_, hasTurns := keys["turns"]
	_, hasFiles := keys["files"]
	hasTurns, hasFiles = hasTurns || len(r.Turns) > 0, hasFiles || len(r.Files) > 0
	if hasTurns {
		if err := r.checkTurnsAlone(keys); err != nil {
			return err
		}
	}
	if hasFiles {
		if err := r.checkFiles(); err != nil {
			return err
		}
	}
	total := 0
	seen := map[string]bool{}
	for i, turn := range r.turns() {
		where := ""
		if hasTurns {
			where = fmt.Sprintf("turn %d ", i+1)
			if err := checkPromptID(turn.PromptID, seen); err != nil {
				return fmt.Errorf("%s%w", where, err)
			}
			if err := checkMessage(turn.FinalMessage); err != nil {
				return fmt.Errorf("%s%w", where, err)
			}
		}
		n, err := checkCalls(turn.Calls, total, where, hasFiles)
		if err != nil {
			return err
		}
		total = n
	}
	return nil
}

// checkTurnsAlone is the rule that turns replaces the single-turn shape
// whole: a run is said one way, so no reader has to know which one wins.
func (r fixtureRun) checkTurnsAlone(keys map[string]json.RawMessage) error {
	for _, k := range []string{"calls", "final_message", "capture"} {
		if _, ok := keys[k]; ok {
			return fmt.Errorf("turns and %s are alternatives: each turn gives its own calls and final_message, and a capture brings its own turns", k)
		}
	}
	if len(r.Turns) == 0 {
		return errors.New("turns is empty: leave it out for a run of one turn")
	}
	if len(r.Turns) > maxFixtureTurns {
		return fmt.Errorf("%d turns, over the %d cap", len(r.Turns), maxFixtureTurns)
	}
	return nil
}

// checkFiles holds the project files to their caps, and each path to a
// place inside the project directory: relative, clean, slash-separated, and
// outside .git, which the replay makes itself.
func (r fixtureRun) checkFiles() error {
	if r.Capture != "" {
		return errors.New("capture and files are alternatives: a capture names its own cwd")
	}
	if len(r.Files) == 0 {
		return errors.New("files is empty: leave it out for a run with no project directory")
	}
	if len(r.Files) > maxProjectFiles {
		return fmt.Errorf("%d files, over the %d cap", len(r.Files), maxProjectFiles)
	}
	// Keyed in lower case: on a filesystem that folds case (macOS's default)
	// a second spelling would overwrite the first, and the project would
	// differ by machine.
	seen := map[string]bool{}
	for i, pf := range r.Files {
		p := pf.Path
		switch {
		case p == "" || len(p) > maxProjectPathBytes || strings.ContainsAny(p, "\x00\\:"):
			return fmt.Errorf("file %d path %q is empty, over %d bytes, or holds NUL, a backslash or a colon", i, p, maxProjectPathBytes)
		case path.IsAbs(p) || path.Clean(p) != p || p == "." || p == ".." || strings.HasPrefix(p, "../"):
			return fmt.Errorf("file %d path %q is not a clean relative path inside the project", i, p)
		case p == ".git" || strings.HasPrefix(p, ".git/"):
			return fmt.Errorf("file %d path %q is inside .git, which the replay makes", i, p)
		case seen[strings.ToLower(p)]:
			return fmt.Errorf("file %d path %q is given twice, in some letter case", i, p)
		case len(pf.Content) > maxProjectFileBytes || !utf8.ValidString(pf.Content) || strings.Contains(pf.Content, "\x00"):
			return fmt.Errorf("file %d (%s) content is over the %d cap, is not UTF-8, or holds NUL", i, p, maxProjectFileBytes)
		}
		seen[strings.ToLower(p)] = true
	}
	// A file cannot also be a directory another file is under, which only
	// writeProject's MkdirAll would otherwise find.
	for _, a := range r.Files {
		for _, b := range r.Files {
			if strings.HasPrefix(strings.ToLower(b.Path), strings.ToLower(a.Path)+"/") {
				return fmt.Errorf("file path %q is a directory file %q is under", a.Path, b.Path)
			}
		}
	}
	return nil
}

// checkPromptID holds a turn's prompt id to the characters of the ids Claude
// Code sends (UUIDs) and to being the turn's own: two turns under one id are
// one turn to the digest, and the prompt the replay sends after the last
// turn must not reopen a turn of the run.
func checkPromptID(id string, seen map[string]bool) error {
	const allowed = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-_."
	if id == "" || len(id) > maxPromptIDBytes || strings.Trim(id, allowed) != "" {
		return fmt.Errorf("prompt_id %q is empty, over %d bytes, or holds a character outside letters, digits, '-', '_' and '.'", id, maxPromptIDBytes)
	}
	if id == trailingPromptID || seen[id] {
		return fmt.Errorf("prompt_id %q is not the turn's own: another turn or the trailing prompt has it", id)
	}
	seen[id] = true
	return nil
}

// checkMessage is the cap a final message answers to. It may not name
// projectToken, which is replaced in a call's input only: sent as written,
// the token would read in the snapshot as the normalised project path a real
// message would have become.
func checkMessage(s string) error {
	if len(s) > maxFieldBytes || !utf8.ValidString(s) || strings.Contains(s, "\x00") {
		return errors.New("final_message is over its cap, is not UTF-8, or holds NUL")
	}
	if strings.Contains(s, projectToken) {
		return fmt.Errorf("final_message names %s, which is replaced in a call's input only", projectToken)
	}
	return nil
}

// checkCalls holds one turn's calls to the call rules and counts them, with
// their repeats, into total, the run's calls so far, against maxFixtureCalls.
// A call input may name the project directory as projectToken only when the
// run has files: without them there is no directory to name.
func checkCalls(calls []fixtureCall, total int, where string, project bool) (int, error) {
	if len(calls) > maxFixtureCalls {
		return total, fmt.Errorf("%s%d calls, over the %d cap", where, len(calls), maxFixtureCalls)
	}
	for i, c := range calls {
		if c.Tool == "" {
			return total, fmt.Errorf("%scall %d names no tool", where, i)
		}
		if !inSet(c.Outcome, callOutcomes) {
			return total, fmt.Errorf("%scall %d outcome %q is not one of %v", where, i, c.Outcome, callOutcomes)
		}
		if c.Repeat < 0 || c.Repeat > maxCallRepeat {
			return total, fmt.Errorf("%scall %d repeat %d is outside 0..%d", where, i, c.Repeat, maxCallRepeat)
		}
		if c.ExitCode < 0 || c.ExitCode > 255 || (c.ExitCode != 0 && c.Outcome != "failed") {
			return total, fmt.Errorf("%scall %d exit_code %d: only a failed call has one, 0..255", where, i, c.ExitCode)
		}
		if len(c.Output) > maxFieldBytes {
			return total, fmt.Errorf("%scall %d output is over its cap", where, i)
		}
		if strings.Contains(c.Output, projectToken) {
			return total, fmt.Errorf("%scall %d output names %s, which is replaced in a call's input only", where, i, projectToken)
		}
		if !project && namesProject(c.Input) {
			return total, fmt.Errorf("%scall %d input names %s, and the run has no files to make a project of", where, i, projectToken)
		}
		total += max(c.Repeat, 1)
		if total > maxFixtureCalls {
			return total, fmt.Errorf("repeats expand to over %d calls", maxFixtureCalls)
		}
	}
	return total, nil
}

// namesProject reports projectToken in any string of a call input.
func namesProject(v any) bool {
	switch v := v.(type) {
	case string:
		return strings.Contains(v, projectToken)
	case map[string]any:
		for _, x := range v {
			if namesProject(x) {
				return true
			}
		}
	case []any:
		for _, x := range v {
			if namesProject(x) {
				return true
			}
		}
	}
	return false
}

// sheetColumns is the labelling sheet's header, in order. One row per real
// fire; the first six columns are filled when the fire is seen, the last
// four by the person who judges it.
var sheetColumns = []string{
	"detector", "fire_ref", "fired_on", "line_text",
	"label", "fp_class", "reason", "labelled_by",
}

// sheetRow is one labelled real fire.
type sheetRow struct {
	Detector, FireRef, FiredOn, LineText string
	Label, FPClass, Reason, LabelledBy   string
}

// parseSheet reads the labelling sheet. A row with no label is a fire not yet
// judged and is skipped, not an error: the sheet is filled in over time. A
// row with a label answers to the same rules as a fixture's, and to the
// registry's detector ids (known).
func parseSheet(b []byte, known func(string) bool) ([]sheetRow, error) {
	if len(b) > maxSheetBytes {
		return nil, fmt.Errorf("sheet is %d bytes, over the %d cap", len(b), maxSheetBytes)
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, errors.New("sheet is not UTF-8 or holds NUL")
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != strings.Join(sheetColumns, "\t") {
		return nil, fmt.Errorf("header is not %q", strings.Join(sheetColumns, "\t"))
	}
	if len(lines) > maxSheetRows {
		return nil, fmt.Errorf("sheet has %d lines, over the %d cap", len(lines), maxSheetRows)
	}
	var rows []sheetRow
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != len(sheetColumns) {
			return nil, fmt.Errorf("line %d has %d columns, want %d", i+2, len(cols), len(sheetColumns))
		}
		r := sheetRow{cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7]}
		if !known(r.Detector) {
			return nil, fmt.Errorf("line %d names detector %q, which is not in the inventory", i+2, r.Detector)
		}
		if r.Label == "" {
			if r.FPClass != "" || r.Reason != "" || r.LabelledBy != "" {
				return nil, fmt.Errorf("line %d has judgement columns filled but no label", i+2)
			}
			continue
		}
		if !isFireLabel(r.Label) {
			return nil, fmt.Errorf("line %d: a sheet row is a fire the detector made, so its label is true_positive or false_positive, not %q", i+2, r.Label)
		}
		if err := checkLabelling(r.Label, r.Reason, r.LabelledBy, r.FPClass); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+2, err)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// fireTally is one detector's labelled fires. Counted ones were confirmed by a
// person; seed ones were not, and are kept apart so they are visible and
// cannot become a number.
type fireTally struct {
	TP, FP         int
	SeedTP, SeedFP int
}

// add counts one fire's label. A label that is no fire is ignored here.
func (t *fireTally) add(label, by string) {
	if !isFireLabel(label) {
		return
	}
	seed := by == labelledBySeed
	switch {
	case label == labelTruePositive && seed:
		t.SeedTP++
	case label == labelTruePositive:
		t.TP++
	case seed:
		t.SeedFP++
	default:
		t.FP++
	}
}

// N is the counted fires.
func (t fireTally) N() int { return t.TP + t.FP }

// wilson95 is the Wilson score interval for k true of n, at 95%: the
// lesson ledger's (learn.Wilson95, card P6), so the table
// detector_stats_test.go pins holds one formula, not two copies of it.
func wilson95(k, n int) (lo, hi float64, ok bool) {
	return learn.Wilson95(k, n)
}

// precisionRow is one printed line of the per-detector table.
type precisionRow struct {
	Detector string
	Tally    fireTally
	// Fixtures is false for a detector with no fixture directory at all.
	Fixtures bool
}

// status is measured or unmeasured, and for unmeasured says why in numbers.
func (r precisionRow) status() string {
	if r.Tally.N() >= minLabelledFires {
		return "measured"
	}
	s := fmt.Sprintf("unmeasured: %d of %d labelled fires", r.Tally.N(), minLabelledFires)
	if !r.Fixtures {
		return s + "; no fixtures"
	}
	if seed := r.Tally.SeedTP + r.Tally.SeedFP; seed > 0 {
		s += fmt.Sprintf("; %d seed fires (%d TP, %d FP) await a person", seed, r.Tally.SeedTP, r.Tally.SeedFP)
	}
	return s
}

// String renders the row: detector, counted N, precision, interval, status.
// Precision and interval are printed from counted fires only, and as n/a at
// N == 0: a seed label is never a number.
func (r precisionRow) String() string {
	prec, ival := "n/a", "n/a"
	if n := r.Tally.N(); n > 0 {
		lo, hi, _ := wilson95(r.Tally.TP, n)
		prec = fmt.Sprintf("%.3f", float64(r.Tally.TP)/float64(n))
		ival = fmt.Sprintf("[%.3f, %.3f]", lo, hi)
	}
	return fmt.Sprintf("%-31s N=%-3d precision=%-6s wilson95=%-16s %s", r.Detector, r.Tally.N(), prec, ival, r.status())
}
