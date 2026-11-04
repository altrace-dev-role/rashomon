package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestTB1_AModelLineIsSentOnlyWhenItNamesSomething (card TOKEN-BUDGET1): a
// preflight line goes to the model only when its list names something,
// listed, left out by a check, or as the record form's digests. A line
// whose list holds only a count of items given earlier, withheld from an
// untrusted spec or learned from earlier sessions says nothing new and is
// left out; with every line left out the model's text is "", and the aside
// still says what was not known. A lesson's line holds no list and is
// always sent. Break: send a line that lists nothing, or keep the header
// alone as the model's text.
func TestTB1_AModelLineIsSentOnlyWhenItNamesSomething(t *testing.T) {
	const head = "rashomon record: preflight for this request from the repository; the request text is not kept."
	counts := preflightNone()
	counts.Paths = slot.NewPathSet(nil, 0, 2)
	counts.Rules = slot.NewRuleRefs(nil, 0, 1, 3)
	counts.Gates = slot.NewGateRefs(nil, 0, 2)
	m, err := New(pattern.TaskPreflight, counts)
	if err != nil {
		t.Fatalf("New(counts only): %v", err)
	}
	if m.Text() != "" || m.Aside() != "unknown: no spec found" {
		t.Errorf("counts only = %q, aside %q; want no model text and the unknowns", m.Text(), m.Aside())
	}
	for _, c := range []struct {
		name string
		set  func(*TaskPreflightBinding)
		want string
	}{
		{"a path left out", func(b *TaskPreflightBinding) { b.Paths = slot.NewPathSet(nil, 1, 2) },
			"paths named: 1 left out; 2 given earlier"},
		{"a path as its digest", func(b *TaskPreflightBinding) {
			b.Paths = slot.NewDigestPathSet([]string{strings.Repeat("a", 64)}, 0, 0)
		},
			"paths named: 1"},
		{"a rule", func(b *TaskPreflightBinding) {
			b.Rules = slot.NewRuleRefs([]slot.RuleRef{{ID: "r1", Trust: slot.NewEnum(slot.EnumTrust, "trusted")}}, 0, 0, 0)
		}, "spec rules that apply: 'r1' (trusted)"},
		{"a gate", func(b *TaskPreflightBinding) {
			b.Gates = slot.NewGateRefs([]slot.GateRef{{Words: "go test", Trust: slot.NewEnum(slot.EnumTrust, "builtin")}}, 0, 2)
		}, "gates before done: 'go test' (builtin); 2 learned from earlier sessions"},
	} {
		b := counts
		c.set(&b)
		m, err := New(pattern.TaskPreflight, b)
		if err != nil {
			t.Fatalf("%s: New: %v", c.name, err)
		}
		if want := head + "\n" + c.want; m.Text() != want {
			t.Errorf("%s: model text %q,\nwant %q", c.name, m.Text(), want)
		}
	}
	lesson := TaskPreflightLessonKnownBinding{Paths: counts.Paths, Rules: counts.Rules, Gates: counts.Gates,
		Source: counts.Source, Evidence: counts.Evidence, Lesson: "go-test-cwd", Text: "use the staging profile"}
	m, err = New(pattern.TaskPreflight, lesson)
	if err != nil {
		t.Fatalf("New(lesson): %v", err)
	}
	if want := head + "\nlesson go-test-cwd, approved for this project: 'use the staging profile'"; m.Text() != want || m.Aside() != "" {
		t.Errorf("lesson = %q, aside %q; want %q and no aside", m.Text(), m.Aside(), want)
	}
}

// TestTB1_ANoteWithNoLineAtAllIsRefused: a sparse message whose every line,
// the model's and the person's, lists nothing has nothing to print, and New
// refuses it, so the producer's ladder finds no note. Control: one unknown
// is a line. Break: return a message with no text and no aside.
func TestTB1_ANoteWithNoLineAtAllIsRefused(t *testing.T) {
	b := preflightNone()
	b.Unknown = slot.NewEnumList(slot.EnumPreflightUnknown, nil)
	if _, err := New(pattern.TaskPreflight, b); !errors.Is(err, ErrNothing) {
		t.Errorf("New(nothing at all) = %v, want ErrNothing", err)
	}
	if _, err := New(pattern.TaskPreflight, preflightNone()); err != nil {
		t.Errorf("New(one unknown) = %v, want the aside alone", err)
	}
}

// TestTB1_TheModelTextHoldsNoEvidenceKindOrUnknown: whatever the request
// names, no evidence kind (an internal enum word) and no unknowns line is in
// the model's text; both are the aside's. Break: keep either line in the
// format.
func TestTB1_TheModelTextHoldsNoEvidenceKindOrUnknown(t *testing.T) {
	b := preflightNone()
	b.Paths = slot.NewPathSet([]slot.Path{"a.go"}, 0, 0)
	b.Evidence = slot.NewEnumList(slot.EnumEvidenceKinds, []string{"judgment_disclosed", "mechanism_stated"})
	for _, binding := range []any{
		b,
		TaskPreflightLessonBinding{Paths: b.Paths, Rules: b.Rules, Gates: b.Gates, Source: b.Source, Evidence: b.Evidence,
			Unknown: b.Unknown, Lesson: "go-test-cwd", Text: "use the staging profile"},
	} {
		m, err := New(pattern.TaskPreflight, binding)
		if err != nil {
			t.Fatalf("New(%T): %v", binding, err)
		}
		if strings.Contains(m.Text(), "judgment_disclosed") || strings.Contains(m.Text(), "\nunknown:") ||
			!strings.Contains(m.Aside(), "judgment_disclosed; mechanism_stated") || !strings.Contains(m.Aside(), "unknown: no spec found") {
			t.Errorf("%T: model %q, aside %q; want the evidence and the unknowns in the aside only", binding, m.Text(), m.Aside())
		}
	}
}
