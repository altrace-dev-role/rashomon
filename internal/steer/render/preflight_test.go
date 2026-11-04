package render

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

func preflightNone() TaskPreflightBinding {
	return TaskPreflightBinding{
		Paths:    slot.NewPathSet(nil, 0, 0),
		Rules:    slot.NewRuleRefs(nil, 0, 0, 0),
		Gates:    slot.NewGateRefs(nil, 0, -1),
		Source:   slot.NewEnum(slot.EnumEvidenceSource, "inferred from the request"),
		Evidence: slot.NewEnumList(slot.EnumEvidenceKinds, nil),
		Unknown:  slot.NewEnumList(slot.EnumPreflightUnknown, []string{"no spec found"}),
	}
}

// TestPF1r_PreflightRowsRenderUnderTheBriefingCheck: task-preflight's two
// rows, the later prompts' and the first prompt's, render line by line and
// pass ValidateBriefing, the multi-line check (R3). The model's text is the
// header and the lines that list something; the aside, the person's, holds
// the first prompt's project line, the evidence and the unknowns, each left
// out when it lists nothing (TOKEN-BUDGET1). With nothing listed for the
// model, its text is "" and the aside alone prints.
func TestPF1r_PreflightRowsRenderUnderTheBriefingCheck(t *testing.T) {
	const head = "rashomon record: preflight for this request from the repository; the request text is not kept."
	m, err := New(pattern.TaskPreflight, preflightNone())
	if err != nil {
		t.Fatalf("New(later row, nothing listed): %v", err)
	}
	if m.Text() != "" || m.Aside() != "unknown: no spec found" {
		t.Errorf("later row with nothing listed = %q, aside %q; want no model text and the unknowns", m.Text(), m.Aside())
	}
	if m.Template() != "task-preflight/v2" {
		t.Errorf("Template() = %q, want task-preflight/v2", m.Template())
	}

	b := preflightNone()
	b.Paths = slot.NewPathSet([]slot.Path{"a.go"}, 0, 0)
	b.Evidence = slot.NewEnumList(slot.EnumEvidenceKinds, []string{"judgment_disclosed"})
	m, err = New(pattern.TaskPreflight, b)
	if err != nil {
		t.Fatalf("New(later row): %v", err)
	}
	if want := head + "\npaths named: 'a.go'"; m.Text() != want {
		t.Errorf("later row = %q,\nwant %q", m.Text(), want)
	}
	if want := "evidence for a done claim (inferred from the request): judgment_disclosed\nunknown: no spec found"; m.Aside() != want {
		t.Errorf("later row's aside = %q,\nwant %q", m.Aside(), want)
	}

	first := TaskPreflightFirstBinding{
		Instructions: slot.NewPathSet([]slot.Path{"CLAUDE.md"}, 0, 0),
		Manifests:    slot.NewPathSet([]slot.Path{"go.mod"}, 0, 0),
		GoDirs:       3,
		Paths:        b.Paths, Rules: b.Rules, Gates: b.Gates, Source: b.Source, Evidence: b.Evidence, Unknown: b.Unknown,
	}
	m, err = New(pattern.TaskPreflight, first)
	if err != nil {
		t.Fatalf("New(first row): %v", err)
	}
	lines := strings.Split(m.Aside(), "\n")
	if m.Text() != head+"\npaths named: 'a.go'" || len(lines) != 3 ||
		lines[0] != "project files: 'CLAUDE.md'; manifests: 'go.mod'; directories with Go files: 3" {
		t.Errorf("first row = %q, aside %q; want the later row's text and three aside lines, the first the project line", m.Text(), m.Aside())
	}
	if m.Template() != "task-preflight-first/v2" {
		t.Errorf("Template() = %q, want task-preflight-first/v2", m.Template())
	}
}

// TestPF1r_PreflightRowsAreStatementsOfFact: every row's format and aside
// pass the template check STR-14 runs over the table, and a filled row whose
// model text and aside are over 1,600 bytes together is refused, never cut,
// so the caller's ladder sees the error. The rows are
// the later prompts' and the first prompt's, two that carry a lesson as
// their last line (LEARN-RIDE1): the later note with its unknowns line and
// without it, and four with the symbols line (PW1b-ON-230 step 3): later,
// later without unknowns, later with a lesson, and the first prompt's. The
// first prompt's six lines are the cap, so it carries no lesson.
func TestPF1r_PreflightRowsAreStatementsOfFact(t *testing.T) {
	n := 0
	for k, tmpl := range templates() {
		if k.pattern != pattern.TaskPreflight {
			continue
		}
		n++
		if err := CheckTemplate(tmpl.format); err != nil {
			t.Errorf("%s: %v", tmpl.id, err)
		}
		// The graph row's lines are all the model's, so it has no aside
		// (GRAPH-SLOT1); every preflight note has one.
		if err := CheckTemplate(tmpl.aside); tmpl.aside == "" && tmpl.id != "task-preflight-graph/v1" || err != nil {
			t.Errorf("%s: aside %q: %v", tmpl.id, tmpl.aside, err)
		}
	}
	if n != 9 {
		t.Errorf("task-preflight has %d rows, want 9", n)
	}
	long := make([]slot.Path, 3)
	for i := range long {
		long[i] = slot.Path(strings.Repeat("d", 127) + "/" + strings.Repeat(string(rune('a'+i)), 124) + ".go")
	}
	b := preflightNone()
	b.Paths = slot.NewPathSet(long, 0, 0)
	b.Unknown = slot.NewEnumList(slot.EnumPreflightUnknown, []string{"no spec found", "no gate history for this project",
		"symbols not matched, no platform graph log", "request over 16 KiB, the rest not read"})
	b.Rules = slot.NewRuleRefs([]slot.RuleRef{
		{ID: "r1", Trust: slot.NewEnum(slot.EnumTrust, "trusted"), Why: slot.Approved(strings.Repeat("word ", 31) + "words")},
		{ID: "r2", Trust: slot.NewEnum(slot.EnumTrust, "trusted"), Why: slot.Approved(strings.Repeat("text ", 31) + "texts")},
		{ID: "r3", Trust: slot.NewEnum(slot.EnumTrust, "trusted"), Why: slot.Approved(strings.Repeat("more ", 31) + "mores")},
	}, 0, 0, 0)
	b.Evidence = slot.NewEnumList(slot.EnumEvidenceKinds, []string{"real_data_run", "hostile_input_run", "cross_check",
		"judgment_disclosed", "mechanism_stated"})
	small := b
	small.Paths = slot.NewPathSet(long[:1], 0, 0)
	if _, err := New(pattern.TaskPreflight, small); err != nil {
		t.Fatalf("premise: the same note with one path renders: %v", err)
	}
	// The model's lines alone are under the cap; with the aside they are
	// over it, and the cap counts both, as it did the whole note.
	known := b
	known.Evidence, known.Unknown = slot.NewEnumList(slot.EnumEvidenceKinds, nil), slot.NewEnumList(slot.EnumPreflightUnknown, nil)
	if _, err := New(pattern.TaskPreflight, known); err != nil {
		t.Fatalf("premise: the model's lines alone render: %v", err)
	}
	if m, err := New(pattern.TaskPreflight, b); err == nil {
		t.Errorf("a %d-byte preflight with a %d-byte aside rendered, want the 1,600-byte refusal", len(m.Text()), len(m.Aside()))
	}
}
