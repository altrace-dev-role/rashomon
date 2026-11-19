package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// rv3Open is a peer message's opening tag and rv3From its sender key, as the
// hook receives them (Claude Code 2.1.322): the attribute names a recorded
// message's, the values made up; internal/harness's frame test says what was
// measured.
const (
	rv3From  = ` from="uds:/tmp/cc-socks/1.sock"`
	rv3Open  = `<cross-session-message` + rv3From + ` from-name="peer-a" from-mode="bypass">`
	rv3Close = "</cross-session-message>"
)

// rv3Bare is a peer message carrying body, as the hook receives it: the bare
// element, with no lead line and no closing paragraph.
func rv3Bare(body string) string {
	return rv3Open + "\n" + body + "\n" + rv3Close
}

// rv3Sized is rv3Bare of exactly n bytes: ask on the body's first line, then
// lines of dots, which hold no word.
func rv3Sized(t *testing.T, ask string, n int) string {
	t.Helper()
	least := len(rv3Bare(ask + "\n"))
	if n <= least {
		t.Fatalf("rv3Sized: %d bytes is not over the element's own %d", n, least)
	}
	pad := n - least
	s := rv3Bare(ask + "\n" + strings.Repeat(strings.Repeat(".", 79)+"\n", pad/80+1)[:pad])
	if len(s) != n {
		t.Fatalf("rv3Sized: %d bytes, want %d", len(s), n)
	}
	return s
}

// rv3Lines are every task-preflight line logged at promptID, of any key.
func (e *lessonEnv) rv3Lines(sessionID, promptID string) []rv1Line {
	e.t.Helper()
	st, err := store.Open(e.root)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _, err := st.ReadSteer(sessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	var out []rv1Line
	for _, b := range raw {
		var l rv1Line
		if err := json.Unmarshal(b, &l); err != nil {
			e.t.Fatal(err)
		}
		if l.Pattern == "task-preflight" && l.PromptID != nil && *l.PromptID == promptID {
			out = append(out, l)
		}
	}
	return out
}

// rv3First is a session over a project with no graph index that holds
// rv1PlainFiles, started and given no prompt yet.
func rv3First(t *testing.T) *lessonEnv {
	t.Helper()
	e := newLessonEnv(t)
	for _, rel := range rv1PlainFiles {
		p := filepath.Join(e.cwd, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package reader\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e.start("s1")
	return e
}

// rv3Listed is how many paths the task-preflight lines at promptID list,
// over every key.
func (e *lessonEnv) rv3Listed(t *testing.T, sessionID, promptID string) int {
	t.Helper()
	n := 0
	for _, l := range e.rv3Lines(sessionID, promptID) {
		n += rv1Lists(t, &l, "paths")
	}
	return n
}

// TestRV1_3b_FramedPromptBuildsNoPreflight: a session's first prompt that
// the host injected (a peer's message naming a file, as the hook receives
// it) builds no task-preflight line of any key, and the next prompt the user
// types, naming the same file, lists it: the framed prompt gave nothing, so
// nothing counts as given. Either side of the reader's cap, through the real
// intake: an element of exactly the cap arrives whole and uncut and is
// framed as one; one byte more arrives cut to the cap, and its head is
// framed only as cut; neither builds a line. Over a graph index a peer
// message, whole or cut, builds no line of any key, the graph brief's
// included. A peer message of exactly the cap with typed text after it
// arrives cut, its head the whole element with the closing tag last: read
// as cut that is an element with text after it, typed, and over the index
// it builds the graph brief's line; read as uncut it is held, and the brief
// sends nothing. Controls: the same request typed builds a line that lists
// the file, and over the index the graph brief's line; the cut message
// without its from= key is typed and builds both. Break: read the request's
// names off a framed prompt, count a framed prompt's paths as given, or drop
// the cut on its way to Framed or to the graph brief.
func TestRV1_3b_FramedPromptBuildsNoPreflight(t *testing.T) {
	ask := rv1PlainAsks[1].text
	t.Run("control_typed_first", func(t *testing.T) {
		e := rv3First(t)
		e.prompt("s1", "p1", ask)
		if n := e.rv3Listed(t, "s1", "p1"); n == 0 {
			t.Fatalf("premise: a typed first prompt naming %s listed no path (lines %d)", rv1PlainFiles[0], len(e.rv3Lines("s1", "p1")))
		}
	})
	t.Run("framed_first", func(t *testing.T) {
		e := rv3First(t)
		out := e.prompt("s1", "p1", rv3Bare(ask))
		if ls := e.rv3Lines("s1", "p1"); len(ls) != 0 {
			for _, l := range ls {
				t.Errorf("framed p1 logged a task-preflight line, key %q: %s", l.Key, l.row())
			}
		}
		if out.system != "" || out.context != "" {
			t.Errorf("framed p1 printed system %q, context %q; want nothing", out.system, out.context)
		}
		e.call("s1", "p1", "tu1")
		e.prompt("s1", "p2", ask)
		if n := e.rv3Listed(t, "s1", "p2"); n != 1 {
			t.Errorf("typed p2 listed %d paths, want 1: the framed prompt's path is fresh (lines %d)", n, len(e.rv3Lines("s1", "p2")))
		}
	})
	t.Run("either_side_of_the_cap", func(t *testing.T) {
		whole := rv3Sized(t, ask, intent.MaxPromptBytes)
		over := rv3Sized(t, ask, intent.MaxPromptBytes+1)
		p := readPendingPayload(strings.NewReader(promptPayload(t, whole)), time.Second)
		if p.PromptCut || len(p.Prompt) != intent.MaxPromptBytes {
			t.Fatalf("an element of the cap: cut %v, %d bytes; want uncut, %d", p.PromptCut, len(p.Prompt), intent.MaxPromptBytes)
		}
		if framed, _ := harness.Framed(string(p.Prompt), false); !framed {
			t.Errorf("an element of the cap, uncut, is not framed")
		}
		q := readPendingPayload(strings.NewReader(promptPayload(t, over)), time.Second)
		if !q.PromptCut || len(q.Prompt) != intent.MaxPromptBytes {
			t.Fatalf("one byte over the cap: cut %v, %d bytes; want cut, %d", q.PromptCut, len(q.Prompt), intent.MaxPromptBytes)
		}
		if framed, _ := harness.Framed(string(q.Prompt), false); framed {
			t.Errorf("the head read as uncut is framed; want an element never closed, typed")
		}
		if framed, _ := harness.Framed(string(q.Prompt), true); !framed {
			t.Errorf("the head read as cut is not framed")
		}
		for _, c := range []struct{ name, prompt string }{{"of the cap", whole}, {"one byte over the cap", over}} {
			e := rv3First(t)
			out := e.prompt("s1", "p1", c.prompt)
			for _, l := range e.rv3Lines("s1", "p1") {
				t.Errorf("a peer message %s logged a task-preflight line, key %q: %s", c.name, l.Key, l.row())
			}
			if out.system != "" || out.context != "" {
				t.Errorf("a peer message %s printed system %q, context %q; want nothing", c.name, out.system, out.context)
			}
		}
		e := rv3First(t)
		e.prompt("s1", "p1", strings.Replace(rv3Sized(t, ask, intent.MaxPromptBytes+500), rv3From, "", 1))
		if n := e.rv3Listed(t, "s1", "p1"); n == 0 {
			t.Errorf("control: the cut message without from= listed no path, so the cut case cannot fail (lines %d)", len(e.rv3Lines("s1", "p1")))
		}
	})
	t.Run("graphed", func(t *testing.T) {
		gask := rv1GraphAsks[0].text
		e := rv1Graphed(t, "", nil)
		e.prompt("s1", "p2", gask)
		if _, g := e.rv1Lines("s1", "p2"); g == nil {
			t.Fatalf("premise: the typed request over the index logged no graph line")
		}
		for _, c := range []struct{ name, prompt string }{
			{"a peer message", rv3Bare(gask)},
			{"a cut peer message", rv3Sized(t, gask, intent.MaxPromptBytes+500)},
		} {
			e := rv1Graphed(t, "", nil)
			e.prompt("s1", "p2", c.prompt)
			for _, l := range e.rv3Lines("s1", "p2") {
				t.Errorf("%s over the index logged a task-preflight line, key %q: %s", c.name, l.Key, l.row())
			}
		}
		e = rv1Graphed(t, "", nil)
		e.prompt("s1", "p2", strings.Replace(rv3Sized(t, gask, intent.MaxPromptBytes+500), rv3From, "", 1))
		if _, g := e.rv1Lines("s1", "p2"); g == nil {
			t.Errorf("control: the cut message without from= logged no graph line, so the cut case cannot fail")
		}
		// The reader keeps the whole element and marks it cut: under the cut
		// it is typed and reaches the graph brief; without it, it is held.
		whole := rv3Sized(t, gask, intent.MaxPromptBytes)
		after := whole + "\nthanks"
		p := readPendingPayload(strings.NewReader(promptPayload(t, after)), time.Second)
		if !p.PromptCut || string(p.Prompt) != whole {
			t.Fatalf("premise: an element of the cap with text after it: cut %v, %d bytes; want cut, the element's %d", p.PromptCut, len(p.Prompt), len(whole))
		}
		if framed, _ := harness.Framed(whole, true); framed {
			t.Fatalf("premise: the cut head that ends in the closing tag is framed; want typed")
		}
		if framed, _ := harness.Framed(whole, false); !framed {
			t.Fatalf("premise: the head read as uncut is not framed, so the case cannot fail")
		}
		e = rv1Graphed(t, "", nil)
		e.prompt("s1", "p2", after)
		if _, g := e.rv1Lines("s1", "p2"); g == nil {
			t.Errorf("an element of the cap with text after it logged no graph line: the cut did not reach the graph brief")
		}
	})
}
