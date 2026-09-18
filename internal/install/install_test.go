package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// Two installs and a hook that is nobody's install: the arrangement every
// removal rule here has to tell apart.
const (
	oneID       = "0123456789abcdef0123456789abcdef"
	otherID     = "ffffffffffffffffffffffffffffffff"
	foreignHook = `{"matcher":"Bash","hooks":[{"type":"command","command":"echo foreign-sentinel","timeout":30}]}`
)

func spec(id string) Spec { return Spec{Executable: "/usr/local/bin/rashomon", InstallID: id} }

// TestOwner covers what an entry is recognised by. Ownership is read out of the
// command line rather than out of a matcher or a position, because the command
// line is the one part of an entry that survives every round trip a settings
// file can take -- including the one Claude Code performs on every "always
// allow".
func TestOwner(t *testing.T) {
	cases := []struct {
		name  string
		event string
		raw   string
		want  string
	}{
		{"ours", EventPreToolUse, string(spec(oneID).Entry(EventPreToolUse)), oneID},
		{"another install", EventPreToolUse, string(spec(otherID).Entry(EventPreToolUse)), otherID},
		{"a foreign group", EventPreToolUse, foreignHook, ""},
		{
			// Still ours, so that intact() gets the chance to refuse over the
			// added hook. An entry read as foreign here would be left behind
			// by detach and silently duplicated by watch.
			"our marker in the second of two hooks",
			EventSessionStart,
			`{"hooks":[{"type":"command","command":"my-linter"},` +
				`{"type":"command","command":"` + spec(oneID).Command(EventSessionStart) + `","timeout":5}]}`,
			oneID,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Owner(json.RawMessage(c.raw), c.event); got != c.want {
				t.Errorf("Owner = %q, want %q", got, c.want)
			}
		})
	}
}

// TestRemoveIfAnyInstallID is the detach --all path. With the store gone there
// is no id left to match against, so the predicate accepts every install -- and
// the entry belonging to no install still has to survive it.
func TestRemoveIfAnyInstallID(t *testing.T) {
	doc := seedDocument(t)

	n, err := RemoveIf(doc, func(string) bool { return true })
	if err != nil {
		t.Fatalf("RemoveIf: %v", err)
	}
	if want := 2 * len(Events); n != want {
		t.Errorf("removed %d entries, want %d", n, want)
	}
	for _, event := range Events {
		remaining := entriesOf(t, doc, event)
		if len(remaining) != 1 {
			t.Errorf("under %s, %d entries remain, want 1 (the foreign one)", event, len(remaining))
			continue
		}
		if id := Owner(remaining[0], event); id != "" {
			t.Errorf("under %s, the surviving entry belongs to install %s", event, id)
		}
	}
	if got := string(doc.Bytes()); !strings.Contains(got, foreignHook) {
		t.Errorf("the foreign entry did not survive byte for byte:\n%s", got)
	}
}

// TestRemoveNamedInstallLeavesTheOther is the same function under detach
// --install <id>: the predicate is the id filter, and the other install's
// entries are as much somebody else's work as the foreign hook is.
func TestRemoveNamedInstallLeavesTheOther(t *testing.T) {
	doc := seedDocument(t)

	n, err := Remove(doc, Spec{InstallID: oneID})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if n != len(Events) {
		t.Errorf("removed %d entries, want %d", n, len(Events))
	}
	for _, event := range Events {
		remaining := entriesOf(t, doc, event)
		if len(remaining) != 2 {
			t.Errorf("under %s, %d entries remain, want 2 (the foreign one and the other install's)", event, len(remaining))
			continue
		}
		if id := Owner(remaining[0], event); id != "" {
			t.Errorf("under %s, the foreign entry was replaced by install %s", event, id)
		}
		if id := Owner(remaining[1], event); id != otherID {
			t.Errorf("under %s, the other install's entry reads as %q", event, id)
		}
	}
}

// TestRemoveIfRefusesAnEntryThatIsNotIntact: a predicate widens which entries
// are considered, never what may be done to one. The refusal names the field,
// and nothing in the document is changed -- not even the intact entry the
// removal had already walked past.
func TestRemoveIfRefusesAnEntryThatIsNotIntact(t *testing.T) {
	narrowed := `{"matcher":"Bash","hooks":[{"type":"command","command":"` +
		spec(otherID).Command(EventPreToolUse) + `","timeout":5}]}`
	doc, err := settings.Parse([]byte(`{"hooks":{"PreToolUse":[` +
		string(spec(oneID).Entry(EventPreToolUse)) + `,` + narrowed + `]}}`))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	before := string(doc.Bytes())

	n, err := RemoveIf(doc, func(string) bool { return true })
	var modified *ErrModified
	if !errors.As(err, &modified) {
		t.Fatalf("RemoveIf removed %d entries and returned %v, want a refusal", n, err)
	}
	if !strings.Contains(modified.Detail, "matcher") {
		t.Errorf("the refusal does not name the field: %q", modified.Detail)
	}
	if got := string(doc.Bytes()); got != before {
		t.Errorf("a refusing RemoveIf still changed the document:\n%s", got)
	}
}

// seedDocument builds a settings document holding, under every event watch
// installs into, a foreign entry and one entry for each of two installs.
func seedDocument(t *testing.T) *settings.Document {
	t.Helper()
	var b strings.Builder
	b.WriteString(`{"hooks":{`)
	for i, event := range Events {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%q:[%s,%s,%s]", event, foreignHook, spec(oneID).Entry(event), spec(otherID).Entry(event))
	}
	b.WriteString("}}")

	doc, err := settings.Parse([]byte(b.String()))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return doc
}

func entriesOf(t *testing.T, doc *settings.Document, event string) []json.RawMessage {
	t.Helper()
	entries, err := doc.HookEntries(event)
	if err != nil {
		t.Fatalf("reading hooks.%s: %v", event, err)
	}
	return entries
}

func TestShellQuote(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a path of safe characters is left bare",
			"/usr/local/bin/rashomon", "/usr/local/bin/rashomon"},
		{"a space is quoted",
			"/Users/sam/Application Support/rashomon", "'/Users/sam/Application Support/rashomon'"},
		{"a single quote is closed, escaped and reopened",
			"/home/o'brien/bin/rashomon", `'/home/o'\''brien/bin/rashomon'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellQuote(tc.in); got != tc.want {
				t.Errorf("shellQuote(%q) is %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
