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
// rows, the later prompts' (header and four lines) and the first prompt's
// (header, the project line and the same four), render line by line and pass
// ValidateBriefing, the multi-line check (R3), with every slot's "none".
func TestPF1r_PreflightRowsRenderUnderTheBriefingCheck(t *testing.T) {
	m, err := New(pattern.TaskPreflight, preflightNone())
	if err != nil {
		t.Fatalf("New(later row): %v", err)
	}
	want := "rashomon record: preflight for this request from the repository; the request text is not kept.\n" +
		"paths named: none\n" +
		"spec rules that apply: none\n" +
		"gates before done: none; evidence for a done claim (inferred from the request): none\n" +
		"unknown: no spec found"
	if m.Text() != want {
		t.Errorf("later row = %q,\nwant %q", m.Text(), want)
	}
	if m.Template() != "task-preflight/v1" {
		t.Errorf("Template() = %q, want task-preflight/v1", m.Template())
	}

	b := preflightNone()
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
	lines := strings.Split(m.Text(), "\n")
	if len(lines) != 6 || lines[1] != "project files: 'CLAUDE.md'; manifests: 'go.mod'; directories with Go files: 3" {
		t.Errorf("first row = %q, want six lines, the second the project line", m.Text())
	}
	if m.Template() != "task-preflight-first/v1" {
		t.Errorf("Template() = %q, want task-preflight-first/v1", m.Template())
	}
}

// TestPF1r_PreflightRowsAreStatementsOfFact: both rows pass the template
// check STR-14 runs over the table, and a filled row over 1,600 bytes is
// refused, never cut, so the caller's ladder sees the error.
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
	}
	if n != 2 {
		t.Errorf("task-preflight has %d rows, want 2", n)
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
	small := b
	small.Paths = slot.NewPathSet(long[:1], 0, 0)
	if _, err := New(pattern.TaskPreflight, small); err != nil {
		t.Fatalf("premise: the same note with one path renders: %v", err)
	}
	if m, err := New(pattern.TaskPreflight, b); err == nil {
		t.Errorf("a %d-byte preflight rendered, want the 1,600-byte refusal", len(m.Text()))
	}
}
