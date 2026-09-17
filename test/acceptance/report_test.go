package acceptance

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// report has two forms and one set of facts. --json is the consumer's form and
// is what every other test here parses; the bare form is for a terminal.
//
// The text form has exactly one way to be wrong that the JSON does not: the
// JSON says null for a fact this program does not have, and a renderer that
// prints 0 or an empty list in its place invents a measurement. So the null
// fields are what these tests are about.

// nullTranscriptFields are the fields that render null in the JSON when the
// transcript could not be read, with the word each must render instead. A
// count that does not exist is "not read"; a comparison that could not be made
// is "unknown".
var nullTranscriptFields = map[string]string{
	"files":                   "not read",
	"ids in transcript":       "not read",
	"results in transcript":   "not read",
	"missing from store":      "unknown",
	"missing from transcript": "unknown",
	"executed but unrecorded": "unknown",
	"declared without result": "unknown",
}

// TestReport_TextRendersUnknownNeverZero is the text form's contract. The
// default payload names a transcript path that is never written, which is the
// state in which every field above is null.
func TestReport_TextRendersUnknownNeverZero(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))
	e.probe("end", testSession)

	rep := e.report(testSession)
	if len(rep.Transcripts) != 1 || rep.Transcripts[0].Readable {
		t.Fatalf("premise broken: want one unreadable transcript group, got %+v", rep.Transcripts)
	}
	if rep.Transcripts[0].IDsInTranscript != nil {
		t.Fatalf("premise broken: the JSON form renders a count for an unreadable transcript")
	}

	res := e.run("", nil, "report", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if strings.HasPrefix(strings.TrimSpace(res.stdout), "{") {
		t.Fatalf("report rendered JSON without --json:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, testSession) {
		t.Errorf("the text form does not name the session:\n%s", res.stdout)
	}
	if got := fieldLine(t, res.stdout, "coverage"); got != rep.Coverage.State {
		t.Errorf("the text form renders coverage %q, want %q: the two forms report one fact",
			got, rep.Coverage.State)
	}
	for label, want := range nullTranscriptFields {
		got := fieldLine(t, res.stdout, label)
		if got != want {
			t.Errorf("%q renders as %q, want %q: the JSON has null there", label, got, want)
		}
		if strings.Contains(got, "0") {
			t.Errorf("%q renders %q, which carries a count for something that was never counted", label, got)
		}
	}
	// The counts that are this store's own records stay counts: they are known
	// whatever the transcript did, and rendering them as unknown would be the
	// same mistake pointed the other way.
	for label, want := range map[string]string{"ids recorded": "1", "ids executed": "1"} {
		if got := fieldLine(t, res.stdout, label); got != want {
			t.Errorf("%q renders as %q, want %q", label, got, want)
		}
	}
}

// TestReport_JSONFormIsUnchanged: --json is today's output, byte for byte. The
// golden is built from the same environment by the helper every other test
// uses, so a change in either form shows up here as a difference between them.
func TestReport_JSONFormIsUnchanged(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	ids := []string{"toolu_a", "toolu_b"}
	transcript := writeTranscript(t, t.TempDir(), testSession, ids, ids[:1], nil)
	e.hookIDs(transcript, ids...)
	e.postIDs(transcript, ids[0])
	e.mustHook(defaultPayload().build(t)) // a second, unreadable transcript group
	e.probe("end", testSession)

	golden := e.report(testSession)
	res := e.run("", nil, "report", "--json", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report --json: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []reportSession `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil {
		t.Fatalf("report --json is not JSON: %v\n%s", err, res.stdout)
	}
	if len(rep.Sessions) != 1 {
		t.Fatalf("report --json returned %d sessions, want 1", len(rep.Sessions))
	}
	if !reflect.DeepEqual(rep.Sessions[0], golden) {
		t.Errorf("report --json differs from the golden:\ngot:  %+v\nwant: %+v", rep.Sessions[0], golden)
	}
	// The encoding itself, not just what it decodes to: two-space indentation,
	// and a null where the fact is missing rather than a rendering of it.
	if !strings.HasPrefix(res.stdout, "{\n  \"generated_at_unix_ms\":") {
		t.Errorf("report --json is not the indented object it was:\n%s", res.stdout)
	}
	if !strings.Contains(res.stdout, `"ids_in_transcript": null`) {
		t.Errorf("report --json renders no null for the unreadable transcript:\n%s", res.stdout)
	}
}

// fieldLine reads the value of a "label: value" line from the text form.
func fieldLine(t *testing.T, text, label string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, label+":") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, label+":"))
		}
	}
	t.Fatalf("the text form has no %q line:\n%s", label, text)
	return ""
}
