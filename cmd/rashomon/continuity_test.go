package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/directive"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The person's words the stand-in Read returns (DIRECTIVE-KEEP1's seam):
// three directives before the boundary, one a correction of another, and
// one after it, which the compaction did not remove.
const (
	saidSplit    = "they go vertical, we go horizontal"
	saidFollowUp = "put the release notes in a follow-up"
	saidSame     = "keep the release notes in the same commit"
	saidAfter    = "the pager rotation starts on Monday"
)

// standInRead replaces readDirectives with a Read that returns the fixed
// directives above and counts its calls, until the test ends.
func standInRead(t *testing.T) *int {
	t.Helper()
	calls := 0
	was := readDirectives
	readDirectives = func(string, int64) ([]directive.Directive, error) {
		calls++
		return []directive.Directive{
			{Kind: directive.KindDirective, Said: saidSplit, Line: 1},
			{Kind: directive.KindDirective, Said: saidFollowUp, Line: 2},
			{Kind: directive.KindCorrection, Said: saidSame, Line: 3, Restates: 2},
			{Kind: directive.KindDirective, Said: saidAfter, Line: 6},
		}, nil
	}
	t.Cleanup(func() { readDirectives = was })
	return &calls
}

// compactTranscript writes a transcript whose boundaries are the uuids given,
// each after three user lines, and returns its path, outside the store.
func compactTranscript(t *testing.T, path string, boundaries ...string) string {
	t.Helper()
	var b strings.Builder
	for _, id := range boundaries {
		b.WriteString(`{"type":"user","uuid":"u1","message":{"role":"user","content":"one"}}` + "\n")
		b.WriteString(`{"type":"user","uuid":"u2","message":{"role":"user","content":"two"}}` + "\n")
		b.WriteString(`{"type":"user","uuid":"u3","message":{"role":"user","content":"three"}}` + "\n")
		b.WriteString(`{"type":"system","subtype":"compact_boundary","uuid":"` + id + `","content":"Conversation compacted"}` + "\n")
	}
	b.WriteString(`{"type":"user","uuid":"u5","message":{"role":"user","content":"after"}}` + "\n")
	b.WriteString(`{"type":"user","uuid":"u6","message":{"role":"user","content":"later"}}` + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// compactStart is the session's SessionStart probe at a compaction, and what
// it printed.
func (e *lessonEnv) compactStart(sessionID, transcript string) []byte {
	e.t.Helper()
	return e.startAt(sessionID, transcript, "compact")
}

// startAt is the session's SessionStart probe from source, and what it
// printed.
func (e *lessonEnv) startAt(sessionID, transcript, source string) []byte {
	e.t.Helper()
	in := benchJSON(e.t, struct {
		benchHead
		Source string `json:"source"`
	}{benchHead{sessionID, transcript, e.cwd, "default", "SessionStart"}, source})
	var stdout, stderr bytes.Buffer
	if code := cmdProbe([]string{"start", install.Marker, e.installID}, bytes.NewReader(in), &stdout, &stderr); code != exitOK {
		e.t.Fatalf("probe start %s: exit %d, stderr %q", source, code, stderr.String())
	}
	return stdout.Bytes()
}

// briefingLine is the part of a continuity-briefing steer line these tests read.
type briefingLine struct {
	Pattern      string                     `json:"pattern"`
	Key          string                     `json:"key"`
	Project      string                     `json:"project"`
	Event        string                     `json:"event"`
	State        string                     `json:"state"`
	SuppressedBy *string                    `json:"suppressed_by"`
	ModelText    string                     `json:"model_text"`
	Slots        map[string]json.RawMessage `json:"slots"`
}

// briefingLines are the session's continuity-briefing steer lines, in order.
func (e *lessonEnv) briefingLines(sessionID string) []briefingLine {
	e.t.Helper()
	st, err := store.Open(e.root)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _, err := st.ReadSteer(sessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	var out []briefingLine
	for _, b := range raw {
		var l briefingLine
		if err := json.Unmarshal(b, &l); err != nil {
			e.t.Fatal(err)
		}
		if l.Pattern == "continuity-briefing" {
			out = append(out, l)
		}
	}
	return out
}

// storeHolding is every file under root whose bytes hold any of words.
func storeHolding(t *testing.T, root string, words ...string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, w := range words {
			if bytes.Contains(b, []byte(w)) {
				hits = append(hits, strings.TrimPrefix(path, root)+" holds "+w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// startOutput is the part of a SessionStart print these tests read.
type startOutput struct {
	SystemMessage      string `json:"systemMessage"`
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// TestCSL1_ACompactStartQuotesThePersonsWordsAndKeepsNone: at SessionStart
// with source compact, on a Claude Code whose compact row P-TRU-1 opened
// (2.1.316), the probe prints the continuity briefing to the model with its
// mirror to the person: the words the compaction removed, newest first, the
// correction with the words it restates, and nothing said after the
// boundary. Its steer line is delivered at SessionStart under the boundary's
// uuid and holds keyed digests, and no file under the store root holds any
// of the person's words (D160). Break: print nothing at compact, quote from
// after the boundary, record the words, or key the line apart from the
// boundary, and this fails.
func TestCSL1_ACompactStartQuotesThePersonsWordsAndKeepsNone(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-316_harness")
	calls := standInRead(t)
	const session = "sess-csl1-quote"
	e.start(session)
	tr := compactTranscript(t, filepath.Join(t.TempDir(), session+".jsonl"), "b-cmd-1")

	out := e.compactStart(session, tr)
	var o startOutput
	if err := json.Unmarshal(out, &o); err != nil {
		t.Fatalf("stdout %q: %v", out, err)
	}
	ctx := o.HookSpecificOutput.AdditionalContext
	if o.HookSpecificOutput.HookEventName != "SessionStart" || !strings.HasPrefix(ctx, "rashomon record: the person's words") {
		t.Fatalf("stdout = %s; want the briefing as SessionStart's additionalContext", out)
	}
	same, split := strings.Index(ctx, "'"+saidSame+"', restating '"+saidFollowUp+"'"), strings.Index(ctx, "'"+saidSplit+"'")
	if same < 0 || split < 0 || same > split || strings.Contains(ctx, saidAfter) || strings.Count(ctx, saidFollowUp) != 1 {
		t.Errorf("additionalContext = %q; want the correction with its restated words, then the earlier directive, and nothing after the boundary", ctx)
	}
	if !strings.Contains(o.SystemMessage, ctx) {
		t.Errorf("systemMessage = %q; want the mirror of %q", o.SystemMessage, ctx)
	}
	if *calls != 1 {
		t.Errorf("Read called %d times, want 1", *calls)
	}
	lines := e.briefingLines(session)
	if len(lines) != 1 || lines[0].Key != "b-cmd-1" || lines[0].Event != "SessionStart" || lines[0].State != "delivered" {
		t.Fatalf("briefing lines = %+v; want one delivered at SessionStart under b-cmd-1", lines)
	}
	if want := baseline.Key(e.cwd); lines[0].Project != want {
		t.Errorf("line project %q, want the cwd's key %q (record scope v5)", lines[0].Project, want)
	}
	if !strings.Contains(lines[0].ModelText, "a quote of 41 bytes, restating a quote of 36 bytes") || len(lines[0].Slots["said1"]) == 0 {
		t.Errorf("line text %q, slots %v; want the record form, the quotes' byte counts and digests", lines[0].ModelText, lines[0].Slots)
	}
	if hits := storeHolding(t, e.root, saidSplit, saidFollowUp, saidSame, saidAfter); len(hits) > 0 {
		t.Errorf("the store holds the person's words: %v", hits)
	}
}

// TestCSL1_OneBriefingPerCompaction: a second compact start at the same
// boundary prints nothing and adds no line, and a start after the next
// compaction briefs again under its own uuid (D-b). Break: drop the dedupe,
// or dedupe by session alone, and this fails.
func TestCSL1_OneBriefingPerCompaction(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-316_harness")
	standInRead(t)
	const session = "sess-csl1-once"
	e.start(session)
	tr := filepath.Join(t.TempDir(), session+".jsonl")
	compactTranscript(t, tr, "b-cmd-1")
	if out := e.compactStart(session, tr); len(out) == 0 {
		t.Fatal("first compact start printed nothing")
	}
	if out := e.compactStart(session, tr); len(out) != 0 {
		t.Errorf("second start at the same boundary printed %s, want nothing", out)
	}
	compactTranscript(t, tr, "b-cmd-1", "b-cmd-2")
	if out := e.compactStart(session, tr); !bytes.Contains(out, []byte("additionalContext")) {
		t.Errorf("start after the next compaction printed %q, want the briefing", out)
	}
	lines := e.briefingLines(session)
	if len(lines) != 2 || lines[0].Key != "b-cmd-1" || lines[1].Key != "b-cmd-2" {
		t.Errorf("briefing lines = %+v; want b-cmd-1 then b-cmd-2", lines)
	}
}

// TestCSL1_AClosedCompactRowPrintsNoBriefing: on a Claude Code below the
// compact row's floor (2.1.279) the briefing is not printed, to the model or
// the person, and its line says unverified_channel; a startup start is the
// brief's, never the briefing's, and Read is not called for it. Break: print
// past the gate, or brief at every source, and this fails.
func TestCSL1_AClosedCompactRowPrintsNoBriefing(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-279_harness")
	calls := standInRead(t)
	const session = "sess-csl1-closed"
	e.start(session)
	if *calls != 0 {
		t.Errorf("a startup start called Read %d times, want 0", *calls)
	}
	tr := compactTranscript(t, filepath.Join(t.TempDir(), session+".jsonl"), "b-cmd-1")
	if out := e.compactStart(session, tr); bytes.Contains(out, []byte(saidSplit)) || bytes.Contains(out, []byte("additionalContext")) {
		t.Errorf("stdout = %s; want no briefing on a closed row", out)
	}
	lines := e.briefingLines(session)
	if len(lines) != 1 || lines[0].State != "suppressed" || lines[0].SuppressedBy == nil || *lines[0].SuppressedBy != "unverified_channel" {
		t.Errorf("briefing lines = %+v; want one suppressed unverified_channel", lines)
	}
}

// TestCSL1_NoBriefingWithoutABoundaryOrAQuote: a compact start whose
// transcript holds no boundary, or is missing, or whose Read returns nothing
// before the boundary, prints nothing and logs no line, and a start from any
// other source, on a transcript with a boundary, never calls Read (resume is
// the first prompt's, D-c). Break: brief on an empty quote list, a missing
// boundary or another source, and this fails.
func TestCSL1_NoBriefingWithoutABoundaryOrAQuote(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-316_harness")
	calls := standInRead(t)
	const session = "sess-csl1-none"
	e.start(session)
	dir := t.TempDir()
	bounded := compactTranscript(t, filepath.Join(dir, "bounded.jsonl"), "b-cmd-1")
	for _, source := range []string{"startup", "resume", "clear", ""} {
		if out := e.startAt(session, bounded, source); bytes.Contains(out, []byte("additionalContext")) || *calls != 0 {
			t.Errorf("a start from %q printed %s and called Read %d times; want no briefing and no Read", source, out, *calls)
		}
	}
	none := filepath.Join(dir, "none.jsonl")
	if err := os.WriteFile(none, []byte(`{"type":"user","uuid":"u1","message":{"role":"user","content":"one"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tr := range []string{none, filepath.Join(dir, "gone.jsonl")} {
		if out := e.compactStart(session, tr); len(out) != 0 {
			t.Errorf("%s: printed %s, want nothing", filepath.Base(tr), out)
		}
	}
	was := readDirectives
	readDirectives = func(string, int64) ([]directive.Directive, error) { return nil, nil }
	defer func() { readDirectives = was }()
	if out := e.compactStart(session, compactTranscript(t, filepath.Join(dir, "s.jsonl"), "b-cmd-1")); len(out) != 0 {
		t.Errorf("no directives: printed %s, want nothing", out)
	}
	if lines := e.briefingLines(session); len(lines) != 0 {
		t.Errorf("briefing lines = %+v, want none", lines)
	}
}

// TestCSL1_TheLeadOpensTheBriefingsMirror: the plugin route's consent line,
// when there is one, opens the briefing's mirror to the person, as it opens
// the brief, and never reaches the model; with none the mirror is the
// briefing's alone. Break: drop the lead, put it after the mirror, or give it
// to the model, and this fails.
func TestCSL1_TheLeadOpensTheBriefingsMirror(t *testing.T) {
	e := newLessonEnv(t)
	t.Setenv("AI_AGENT", "claude-code_2-1-316_harness")
	standInRead(t)
	const session = "sess-csl1-lead"
	e.start(session)
	tr := compactTranscript(t, filepath.Join(t.TempDir(), session+".jsonl"), "b-cmd-1", "b-cmd-2")
	st, err := store.Open(e.root)
	if err != nil {
		t.Fatal(err)
	}
	id := harness.Identity{ID: harness.ClaudeCode, Surface: harness.SurfaceCLI}
	args := []string{"start", install.Marker, e.installID}
	const lead = "rashomon: the plugin's consent line"
	var outs [2]startOutput
	for i, l := range []string{lead, ""} {
		p, err := hookio.IntakeBytes(benchJSON(t, struct {
			benchHead
			Source string `json:"source"`
		}{benchHead{session, tr, e.cwd, "default", "SessionStart"}, "compact"}))
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			compactTranscript(t, tr, "b-cmd-1", "b-cmd-2", "b-cmd-3")
		}
		var stdout bytes.Buffer
		if !printContinuity(args, p, st, l, id, &stdout) || json.Unmarshal(stdout.Bytes(), &outs[i]) != nil {
			t.Fatalf("lead %q: printed %q, want the briefing", l, stdout.String())
		}
	}
	with, without := outs[0], outs[1]
	if with.SystemMessage != lead+"\n"+without.SystemMessage || strings.Contains(with.HookSpecificOutput.AdditionalContext, lead) {
		t.Errorf("with the lead: systemMessage %q, model %q; want the lead, a line feed, then the mirror %q, and the model without it",
			with.SystemMessage, with.HookSpecificOutput.AdditionalContext, without.SystemMessage)
	}
	if without.SystemMessage == "" || strings.Contains(without.SystemMessage, lead) {
		t.Errorf("without the lead: systemMessage %q, want the mirror alone", without.SystemMessage)
	}
}
