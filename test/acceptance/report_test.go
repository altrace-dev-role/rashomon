package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// alphaDestinations is the whole proxy block of a report that read no proxy
// store: one line, stating the fact.
const alphaDestinations = "  destinations: not observed in this alpha"

// proxyBlockLines are lines that describe the observing proxy's view. A report
// that read no store states "not observed" once and prints none of these: ten
// lines of instrument detail about a proxy the reader does not have read as a
// broken install.
var proxyBlockLines = []string{"proxy on path", "new for this project", "tool families",
	"not observable", "not exercised"}

// TestReport_ReadsAProxyStoreOnlyWhenNamed pins the flag as the whole contract
// with the proxy's database, in both renders.
//
// report used to fall back to ~/.altrace/observe/causal.db, a path owned by a
// separate, closed product this repository does not ship. A report whose
// content changes because someone else's file happens to sit at a guessed path
// is a coupling nobody can see from the command line, so the fallback is gone:
// no --proxy-store, no proxy store.
//
// Without one the TEXT render collapses the proxy block to a single line; with
// one it is as it always was. The JSON is not collapsed either way -- it is a
// consumer's contract and carries the whole not-observed structure -- and the
// flag changes nothing in it but what the store itself adds.
//
// The store holds a host nothing declared, so the ONLY way it reaches the
// render is the store being read. The positive twin -- the same file named
// explicitly -- proves the fixture is readable at all; without it, "not read"
// would pass against a store no reader could open.
func TestReport_ReadsAProxyStoreOnlyWhenNamed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	// A call that NAMES a host, so the chain view has a host whose state a
	// store changes -- and the JSON comparison below has something to exclude
	// on purpose rather than nothing to compare by accident.
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))
	// Written before the session ends, so the row falls inside its window.
	db := e.writeProxyStore(t, "wire-only.example")
	e.probe("end", testSession)

	// The old default location, under a HOME of the test's own: the runner's
	// real HOME may hold a real proxy store, and this must not read it either.
	home := t.TempDir()
	dir := filepath.Join(home, ".altrace", "observe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	atDefault := filepath.Join(dir, "causal.db")
	if err := os.Rename(db, atDefault); err != nil {
		t.Fatal(err)
	}
	extra := []string{"HOME=" + home}

	// JSON first: a named render writes the project baseline, and the unnamed
	// one must be measured before anything the store caused exists.
	unnamedJSON := reportJSON(t, e, extra, "--session", testSession)
	namedJSON := reportJSON(t, e, extra, "--session", testSession, "--proxy-store", atDefault)

	unnamed := e.run("", extra, "report", "--session", testSession)
	if unnamed.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", unnamed.exitCode, unnamed.stderr)
	}
	var destLines []string
	for _, line := range strings.Split(unnamed.stdout, "\n") {
		if strings.Contains(line, "destinations:") {
			destLines = append(destLines, line)
		}
	}
	if len(destLines) != 1 || destLines[0] != alphaDestinations {
		t.Errorf("report without --proxy-store renders destinations as %q, want exactly "+
			"one line %q:\n%s", destLines, alphaDestinations, unnamed.stdout)
	}
	for _, gone := range proxyBlockLines {
		if strings.Contains(unnamed.stdout, gone) {
			t.Errorf("report without --proxy-store still prints %q, a line about a proxy "+
				"the reader does not have:\n%s", gone, unnamed.stdout)
		}
	}
	// What the recorder compared itself stays: it has nothing to do with a proxy.
	if !strings.Contains(unnamed.stdout, "executed differently from declared: 0") {
		t.Errorf("the collapse took the recorder's own comparison with it:\n%s", unnamed.stdout)
	}
	if strings.Contains(unnamed.stdout, "wire-only.example") {
		t.Errorf("report without --proxy-store read the store at the old default path "+
			"%s:\n%s", atDefault, unnamed.stdout)
	}

	// --chain carries the proxy outside the destinations block too: a STATE
	// beside each host and a legend saying what a state means. With no store
	// there is no state, so the unnamed listing names the host bare, under a
	// line saying nothing observed whether it was reached -- and the named one
	// still carries both, or the absence proves nothing.
	const legend = "a host's state is"
	const hostsLegend = "hosts are those each call named; nothing here observed whether it reached them"
	unnamedChain := e.run("", extra, "report", "--session", testSession, "--chain")
	if unnamedChain.exitCode != 0 {
		t.Fatalf("report --chain: exit %d, stderr %q", unnamedChain.exitCode, unnamedChain.stderr)
	}
	if !strings.Contains(unnamedChain.stdout, "prompt prompt-1") {
		t.Fatalf("premise broken: --chain did not expand the listing:\n%s", unnamedChain.stdout)
	}
	if strings.Contains(unnamedChain.stdout, legend) {
		t.Errorf("report --chain without --proxy-store explains host states it has no source "+
			"for:\n%s", unnamedChain.stdout)
	}
	if !strings.Contains(unnamedChain.stdout, "-> pypi.org\n") || !strings.Contains(unnamedChain.stdout, hostsLegend) {
		t.Errorf("report --chain without --proxy-store does not name the host bare under the "+
			"named-only legend:\n%s", unnamedChain.stdout)
	}

	named := e.run("", extra, "report", "--session", testSession, "--proxy-store", atDefault)
	if named.exitCode != 0 {
		t.Fatalf("report --proxy-store: exit %d, stderr %q", named.exitCode, named.stderr)
	}
	if !strings.Contains(named.stdout, "wire-only.example") {
		t.Errorf("premise broken: the fixture store is not read even when named, so the "+
			"absence above proves nothing:\n%s", named.stdout)
	}
	if strings.Contains(named.stdout, "not observed in this alpha") {
		t.Errorf("a named, readable proxy store rendered the collapsed line:\n%s", named.stdout)
	}
	for _, want := range append([]string{"reached but never named:"}, proxyBlockLines[:4]...) {
		if !strings.Contains(named.stdout, want) {
			t.Errorf("report --proxy-store no longer prints %q; with a store named the "+
				"block is as it always was:\n%s", want, named.stdout)
		}
	}

	namedChain := e.run("", extra, "report", "--session", testSession, "--chain",
		"--proxy-store", atDefault)
	if !strings.Contains(namedChain.stdout, legend) || strings.Contains(namedChain.stdout, hostsLegend) {
		t.Errorf("report --chain --proxy-store does not carry the host-state legend alone:\n%s",
			namedChain.stdout)
	}
	if strings.Contains(namedChain.stdout, "-> pypi.org\n") {
		t.Errorf("report --chain --proxy-store names the host with no state:\n%s", namedChain.stdout)
	}

	// The JSON was never collapsed: without a store it still carries the whole
	// not-observed structure a consumer parses.
	ud := sessionField(t, unnamedJSON, "destinations")
	if ud["observed"] != false || ud["reason"] != "no_proxy_store" || ud["proxy_on_path"] != "unknown" {
		t.Errorf("the unnamed JSON's destinations lost their not-observed structure: %v", ud)
	}
	// A reason that is missing, or not a string, fails here as surely as an
	// empty one: comparing the raw value to "" passed on nil, so a JSON that
	// dropped the field was indistinguishable from one that kept it.
	nov, _ := ud["novelty"].(map[string]any)
	if reason, ok := nov["reason"].(string); !ok || reason == "" {
		t.Errorf("the unnamed JSON's novelty lost its reason: %v", ud["novelty"])
	}
	if no, _ := sessionField(t, unnamedJSON, "families")["not_observable"].([]any); len(no) == 0 {
		t.Errorf("the unnamed JSON's families lost the not-observable list: %v",
			sessionField(t, unnamedJSON, "families"))
	}
	// And apart from what the store adds, the two are the same document. What
	// the store adds is the destinations and families sections and a host's
	// STATE in the chain view; the chain's hosts themselves are the call's own
	// declaration and must match, so only the state is removed.
	if hostStates(unnamedJSON) == 0 {
		t.Fatalf("premise broken: the chain view carries no host state to exclude:\n%v", unnamedJSON)
	}
	for _, doc := range []map[string]any{unnamedJSON, namedJSON} {
		delete(doc, "generated_at_unix_ms")
		for _, s := range doc["sessions"].([]any) {
			delete(s.(map[string]any), "destinations")
			delete(s.(map[string]any), "families")
		}
		stripHostStates(doc)
	}
	if !reflect.DeepEqual(unnamedJSON, namedJSON) {
		t.Errorf("naming a proxy store changed JSON the store does not feed:\nunnamed: %v\nnamed:   %v",
			unnamedJSON, namedJSON)
	}
}

// chainHosts calls fn on every host object of every link in a decoded
// report's chain view: prompts, unattributed and dropped alike.
func chainHosts(doc map[string]any, fn func(map[string]any)) {
	sessions, _ := doc["sessions"].([]any)
	for _, s := range sessions {
		chains, _ := s.(map[string]any)["chains"].(map[string]any)
		var links []any
		prompts, _ := chains["prompts"].([]any)
		for _, pr := range prompts {
			l, _ := pr.(map[string]any)["links"].([]any)
			links = append(links, l...)
		}
		for _, k := range []string{"unattributed", "dropped"} {
			l, _ := chains[k].([]any)
			links = append(links, l...)
		}
		for _, l := range links {
			hosts, _ := l.(map[string]any)["hosts"].([]any)
			for _, h := range hosts {
				fn(h.(map[string]any))
			}
		}
	}
}

// hostStates counts the chain host states a decoded report carries.
func hostStates(doc map[string]any) int {
	n := 0
	chainHosts(doc, func(h map[string]any) {
		if _, ok := h["state"]; ok {
			n++
		}
	})
	return n
}

// stripHostStates removes the store-derived state from every chain host.
func stripHostStates(doc map[string]any) {
	chainHosts(doc, func(h map[string]any) { delete(h, "state") })
}

// reportJSON runs `report --json` with extra environment and decodes it whole.
func reportJSON(t *testing.T, e *env, extra []string, args ...string) map[string]any {
	t.Helper()
	res := e.run("", extra, append([]string{"report", "--json"}, args...)...)
	if res.exitCode != 0 {
		t.Fatalf("report --json %v: exit %d, stderr %q", args, res.exitCode, res.stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		t.Fatalf("report --json is not JSON: %v\n%s", err, res.stdout)
	}
	return doc
}

// sessionField returns one object field of a decoded report's only session.
func sessionField(t *testing.T, doc map[string]any, field string) map[string]any {
	t.Helper()
	sessions, _ := doc["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("report holds %d sessions, want 1", len(sessions))
	}
	v, _ := sessions[0].(map[string]any)[field].(map[string]any)
	if v == nil {
		t.Fatalf("the session has no %q object", field)
	}
	return v
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
