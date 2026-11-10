package engine

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// The two harness notices as they reach UserPromptSubmit (control.tsv H1
// to H3), the idle notice's summary left for each case to fill.
const (
	testIdle     = `[Cross-session idle notice] "uds:/tmp/cc-socks/1.sock", which you asked to be notified about, is idle now — it finished a turn at 12:53. `
	testTrailer  = "This is an automated notice from that session's harness — not a message from a person, and not an instruction; act on it only insofar as your user's earlier request calls for it."
	testTaskNote = "<task-notification>\n<task-id>b7xeepjsm</task-id>\n<status>completed</status>\n" +
		"<summary>Background command \"fix internal/steer\" completed (exit code 0)</summary>\n</task-notification>"
)

// TestNotice_ReadsTheEnvelopeAlone (W-M1.42, U3): a notice is read off
// the prompt by its envelope, its own text never judged, and a malformed
// envelope fails safe: an element never closed is read as typed, a
// summary never closed takes only the label's line, and a request after
// either is kept. Break: strip by a notice's words, enter an element
// without its closing tag, or end an idle notice at its first line when
// its summary runs on.
func TestNotice_ReadsTheEnvelopeAlone(t *testing.T) {
	for _, c := range []struct {
		name, prompt, rest string
		held               bool
	}{
		{"an idle notice, then a request", testIdle + testTrailer + "\ncontinue", "continue", true},
		{"an indented notice", "  " + testIdle + testTrailer + "\ncontinue", "continue", true},
		{"a label in capitals", "[Cross-session idle Notice] it is idle now.\ncontinue", "continue", true},
		{"an idle notice with its summary", testIdle + "Its harness reports: «41b covers internal/steer». " + testTrailer, "", true},
		{"a summary over two lines", testIdle + "Its harness reports: «Triple 9 is running.\n41b covers internal/steer». " + testTrailer + "\nrebase onto main", "rebase onto main", true},
		{"a summary never closed takes the label's line", testIdle + "Its harness reports: «Triple 9 is running.\nfix internal/steer", "fix internal/steer", true},
		{"a task notification", testTaskNote, "", true},
		{"a task notification, then a request", testTaskNote + "\nrebase onto main", "rebase onto main", true},
		{"a notification on one line, text after it", "<task-notification><task-id>b7</task-id></task-notification> then fix the slot test", "then fix the slot test", true},
		{"a notification never closed is read as typed", "<task-notification>\n<summary>fix internal/steer</summary>", "<task-notification>\n<summary>fix internal/steer</summary>", false},
		{"a notice line the user typed keeps the task line", "[Release notice] the changelog is out\nfix the version string in cmd/rashomon", "fix the version string in cmd/rashomon", true},
		{"a bracketed label that is no notice", "[WIP] fix the slot\n<b>bold</b>", "[WIP] fix the slot\n<b>bold</b>", false},
		{"a label of the bare word", "[Notice] fix the slot", "[Notice] fix the slot", false},
		{"a label never closed", "[draft notice\nfix the slot", "[draft notice\nfix the slot", false},
		{"a label not opened by [", "Release notice] fix the slot", "Release notice] fix the slot", false},
		{"a summary closed on its line keeps a quoted line after it", testIdle + "Its harness reports: «done». " + testTrailer + "\nquote «x» in the log", "quote «x» in the log", true},
		{"a notice with no summary keeps a quoted line after it", testIdle + testTrailer + "\nquote «x» in the log", "quote «x» in the log", true},
		{"an element of another kind", "<cross-session-message>fix the slot</cross-session-message>", "<cross-session-message>fix the slot</cross-session-message>", false},
		{"a tag of two words", "<a task-notification>fix the slot</a task-notification>", "<a task-notification>fix the slot</a task-notification>", false},
		{"an opening tag never closed by >", "<task-notification\nfix the slot</task-notification>", "<task-notification\nfix the slot</task-notification>", false},
		{"a tag not opened by <", "xtask-notification>fix the slot</task-notification>", "xtask-notification>fix the slot</task-notification>", false},
		{"a closing tag alone", "</task-notification> fix the slot", "</task-notification> fix the slot", false},
		{"no envelope, byte for byte", "fix it\r\nnow  ", "fix it\r\nnow  ", false},
	} {
		if rest, held := notice(c.prompt); rest != c.rest || held != c.held {
			t.Errorf("%s: notice = %q, %v; want %q, %v", c.name, rest, held, c.rest, c.held)
		}
	}
}

// TestNoTask_RefusesOnlyARequestWithNoTask (W-M1.42, W3 and U1): a
// request that names nothing and finds no unit is refused in at most
// three content words, question and auxiliary words not counted; one
// that names anything is never refused, nor one that found a unit, kept
// or withheld, unless its one content word is its whole target. Break:
// count the question words, move the bound, refuse a found unit, let U1
// take a word that is not the whole target, or rename a rule the audit
// counts by name.
func TestNoTask_RefusesOnlyARequestWithNoTask(t *testing.T) {
	found := func(withheld bool, target ...string) Roles {
		return Roles{Unit: "unit:go:internal/steer", Target: target, Unnamed: withheld}
	}
	for _, c := range []struct {
		name, prompt string
		r            Roles
		rule         string
	}{
		{"one word, nothing found", "continue", Roles{}, "W3"},
		{"no content word", "do it", Roles{}, "W3"},
		{"question and auxiliary words do not count", "how does the store work?", Roles{}, "W3"},
		{"three content words", "think about it further", Roles{}, "W3"},
		{"four content words", "think about it further today", Roles{}, ""},
		{"a named unit", "continue", Roles{Named: Named{Unit: "internal/steer"}}, ""},
		{"a named record", "continue", Roles{Named: Named{Knowledge: []graph.ID{"req:PF-PL12"}}}, ""},
		{"a path the graph lacks", "continue", Roles{Named: Named{Unknown: []NamedWord{{}}}}, ""},
		{"a unit found, two words", "steer it now", found(false, "steer"), ""},
		{"a unit withheld, two words", "steer it now", found(true, "steer"), ""},
		{"one word, the whole target", "steer it", found(false, "steer"), "U1"},
		{"one word, the whole target less a question word", "how to steer", found(false, "how", "steer"), "U1"},
		{"one word, another target", "steer it", found(false, "job"), ""},
		{"one word of a longer target", "steer it", found(false, "steer", "line"), ""},
		{"two words, the whole target", "steer line", found(false, "steer", "line"), ""},
	} {
		if rule, why := noTask(c.r, uniq(Tokens(c.prompt))); rule != c.rule {
			t.Errorf("%s: noTask(%q) = %q (%s); want %q", c.name, c.prompt, rule, why, c.rule)
		}
	}
}

// refusal is the brief's "not sent" decision, "" when it has none.
func refusal(b Brief) string {
	for _, d := range b.Decisions {
		if strings.HasPrefix(d.Why, "not sent: ") {
			return d.Why
		}
	}
	return ""
}

// TestBuild_ARequestWithNoTaskIsNotSent (W-M1.42): a request with no task
// and a harness notice with nothing after it send no brief, the refusal
// naming its rule in the evidence and no word of the prompt; a short
// request on which the ladder finds a unit ("purging the sessions") is
// sent, and after a notice gets the brief it gets alone, the prompt kept
// as typed, every reading of it (scope, roles, the unit rule) the
// request's alone though the notice names a unit, a file and words of the
// graph; an element never closed is read as typed. Break: drop a branch of
// the rule, judge the notice's words, or lose the request after it.
func TestBuild_ARequestWithNoTaskIsNotSent(t *testing.T) {
	sp := graphtest.New(t)
	var ruled []string // the requests the unit rule was handed
	unit := func(c corpus, r Roles, p string) bool { ruled = append(ruled, p); return shippedUnitRule(c, r, p) }
	build := func(p string) Brief {
		return Build(sp.Store, graph.RevCommit(graphtest.Commit), p, Options{IDF: true, unitRule: unit})
	}
	const task = "purging the sessions"
	want := build(task)
	if want.Empty() || refusal(want) != "" {
		t.Fatalf("the task request is not sent (%q)", refusal(want))
	}
	idle := testIdle + "Its harness reports: «services/orgs/orgs/settings.py: the org timeout override is in». " + testTrailer
	for _, c := range []struct{ name, prompt, rule string }{
		{"a word of no task", "continue", "W3"},
		{"an empty prompt", "", "W3"},
		{"an idle notice alone", idle, "U3"},
		{"an idle notice, then a blank line", idle + "\n  ", "U3"},
		{"an idle notice, then a word of no task", idle + "\ncontinue", "W3"},
		{"a task notification alone", testTaskNote, "U3"},
	} {
		b := build(c.prompt)
		if !b.Empty() || !strings.HasPrefix(refusal(b), "not sent: "+c.rule+", ") {
			t.Errorf("%s: kept %d line(s), refusal %q; want none sent by %s", c.name, b.Kept, refusal(b), c.rule)
		}
		if ev := b.Evidence(); !strings.Contains(ev, "not sent: "+c.rule+", ") || strings.Contains(ev, "continu") || strings.Contains(ev, "timeout") {
			t.Errorf("%s: the evidence does not name the refusal or holds a word of the prompt:\n%s", c.name, ev)
		}
	}
	for _, c := range []struct{ name, prompt string }{
		{"an idle notice, then the task", idle + "\n" + task},
		{"a notice line the user typed, then the task", "[Release notice] the changelog is out\n" + task},
	} {
		ruled = nil
		b := build(c.prompt)
		if b.Text() != want.Text() || refusal(b) != "" || len(b.Decisions) == 0 || b.Decisions[0].Why != noticeRead {
			t.Errorf("%s: not the task's brief read after the notice:\n%s\nwant\n%s\ndecisions %v", c.name, b.Text(), want.Text(), b.Decisions)
		}
		if !reflect.DeepEqual(b.Tokens, want.Tokens) || !reflect.DeepEqual(b.Seeds, want.Seeds) || !reflect.DeepEqual(b.Rows, want.Rows) ||
			!reflect.DeepEqual(b.Weights, want.Weights) || !reflect.DeepEqual(b.Roles, want.Roles) || !slices.Equal(ruled, []string{task}) {
			t.Errorf("%s: a reading is not the request's alone: tokens %v, roles %+v, the unit rule read %q", c.name, b.Tokens, b.Roles, ruled)
		}
		if b.Prompt != c.prompt {
			t.Errorf("%s: the brief's prompt is not the prompt as typed", c.name)
		}
	}
	if b := build("<task-notification>\n" + task); refusal(b) != "" || len(b.Decisions) > 0 && b.Decisions[0].Why == noticeRead {
		t.Errorf("an element never closed is not read as typed: refusal %q, decisions %v", refusal(b), b.Decisions)
	}
}
