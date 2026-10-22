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
		{"ours", EventPreToolUse, string(mustEntry(t, spec(oneID), EventPreToolUse)), oneID},
		{"another install", EventPreToolUse, string(mustEntry(t, spec(otherID), EventPreToolUse)), otherID},
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
		// The four cases below exercise afterFirstWord's single-quoted
		// branch, the one shellQuote sends every path through that is not
		// made of bare-safe characters: a backslash (so every Windows
		// install), an embedded single quote, or a space. Without these,
		// the branch that every Windows install and any POSIX install
		// under a path with a space or a quote actually runs had no
		// coverage at all -- a break in it would have shown as "every
		// watch adds a duplicate, every detach removes nothing" with no
		// test going red first.
		{
			"quoted executable with an embedded single quote",
			EventPreToolUse,
			string(mustEntry(t, Spec{Executable: "/home/o'brien/bin/rashomon", InstallID: oneID}, EventPreToolUse)),
			oneID,
		},
		{
			"quoted executable with a space",
			EventPreToolUse,
			string(mustEntry(t, Spec{Executable: "/a b/rashomon", InstallID: oneID}, EventPreToolUse)),
			oneID,
		},
		{
			"quoted executable with Windows backslashes",
			EventPreToolUse,
			string(mustEntry(t, Spec{Executable: `C:\x\rashomon.exe`, InstallID: oneID}, EventPreToolUse)),
			oneID,
		},
		{
			// shellQuote never writes this shape -- it always closes the
			// quote it opens -- but afterFirstWord has to resolve any
			// command a settings file might actually hold, including one
			// hand-edited into an invalid state. An unclosed quote has no
			// "end of the first word" to split at, so Owner must read it
			// as nobody's rather than guess.
			"unterminated quote never resolves to an owner",
			EventPreToolUse,
			`{"hooks":[{"type":"command","command":"'/no/closing/quote post --install ` + oneID + `"}]}`,
			"",
		},
		{
			// LUI-2026-10-17: the narrowing to a whole-remainder match (see
			// TestLUI3_) also means a hand-wrapped command -- a env-prefix
			// here -- is no longer recognised as ours, even though the
			// wrapped tail is otherwise byte-identical to an owned entry.
			// Pinned here so the choice stays visible: a future change that
			// restores suffix matching to recover this case would have to
			// change this test, not just add one.
			"a command wrapped in an env prefix is not ours",
			EventPostToolUse,
			`{"hooks":[{"type":"command","command":"env FOO=1 ` + spec(oneID).Command(EventPostToolUse) + `"}]}`,
			"",
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

// TestLUI3_OwnerDoesNotClaimSteerPostAsPost pins LUI-3: identity is read off
// the whole of the command after its executable, not a suffix of it.
//
// "post" is PostToolUse's own subcommand, and it is also the tail of "steer
// post" -- a longer subcommand a future entry under a different role
// installs on the very same event. The old implementation searched for
// " post --install " anywhere in the command, so a "steer post" entry would
// have read, wrongly, as this install's post entry: detach would have
// removed somebody's steer entry under the recorder's name, and a second
// watch would have found "ours" already present and left a stray steer
// entry untouched. The fix is a whole-remainder comparison, so a longer
// subcommand that merely ends the same way is never mistaken for a shorter
// one that is a true suffix of it.
func TestLUI3_OwnerDoesNotClaimSteerPostAsPost(t *testing.T) {
	notOurs := json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command",` +
		`"command":"/usr/local/bin/rashomon steer post --install ` + oneID + `","timeout":5}]}`)
	if got := Owner(notOurs, EventPostToolUse); got != "" {
		t.Errorf("Owner(%s) = %q, want %q: a steer post entry is not the recorder's post entry",
			notOurs, got, "")
	}

	// The positive twin, so the fix is a narrowing and not a break: the
	// recorder's own post entry, same executable and id, is still read.
	ours := json.RawMessage(mustEntry(t, spec(oneID), EventPostToolUse))
	if got := Owner(ours, EventPostToolUse); got != oneID {
		t.Errorf("Owner(%s) = %q, want %q", ours, got, oneID)
	}
}

// TestLUI4_WatchThenDetachLeavesNoResidue pins LUI-4's legacy half: Owner
// recognises a listed legacy subcommand as ours, so an entry a previous
// release wrote under an older subcommand is refreshed to the current one in
// place -- never duplicated -- and a later detach removes it exactly as it
// would the current form. A foreign entry under the same event, and every
// event watch never touches, stay byte for byte as found; that half of the
// promise is TestH06_DetachLeavesTheFileByteForByteAsFound's and is
// unchanged here.
//
// No real subcommand has been renamed yet (legacyForms is empty for every
// production event), so this test supplies its own legacy form rather than
// one shipped: it proves the mechanism a rename rides on, not a rename.
func TestLUI4_WatchThenDetachLeavesNoResidue(t *testing.T) {
	const legacy = "recap-legacy-v1"
	legacyForms[EventStop] = []string{legacy}
	t.Cleanup(func() { delete(legacyForms, EventStop) })

	exe := "/usr/local/bin/rashomon"
	legacyEntry := `{"hooks":[{"type":"command","command":"` +
		exe + " " + legacy + " " + Marker + " " + oneID + `","timeout":` + fmt.Sprint(RecapTimeout) + `}]}`
	doc, err := settings.Parse([]byte(`{"hooks":{"Stop":[` + foreignHook + `,` + legacyEntry + `]}}`))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if _, err := Apply(doc, Spec{Executable: exe, InstallID: oneID}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	stopEntries := entriesOf(t, doc, EventStop)
	if len(stopEntries) != 2 {
		t.Fatalf("Stop carries %d entries after refresh, want 2 (foreign, plus ours refreshed in place)",
			len(stopEntries))
	}
	var refreshed json.RawMessage
	for _, e := range stopEntries {
		if Owner(e, EventStop) == oneID {
			refreshed = e
		}
	}
	if refreshed == nil {
		t.Fatal("no entry under Stop reads as ours after Apply refreshed the legacy form")
	}
	if detail := intact(refreshed, EventStop); detail != "" {
		t.Errorf("the refreshed entry is not intact: %s", detail)
	}
	if strings.Contains(string(refreshed), legacy) {
		t.Errorf("the refreshed entry still carries the legacy subcommand %q: %s", legacy, refreshed)
	}

	removed, modified, err := Remove(doc, Spec{InstallID: oneID}, false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed == 0 {
		t.Error("Remove removed nothing; the refreshed entry should have been recognised and removed")
	}
	if len(modified) != 0 {
		t.Errorf("Remove reported entries it had to leave edited: %v", modified)
	}
	remainingStop := entriesOf(t, doc, EventStop)
	if len(remainingStop) != 1 {
		t.Fatalf("under Stop, %d entries remain, want 1 (the foreign one)", len(remainingStop))
	}
	if id := Owner(remainingStop[0], EventStop); id != "" {
		t.Errorf("an owned entry survived detach under Stop, reading as %q", id)
	}
	if !strings.Contains(string(doc.Bytes()), foreignHook) {
		t.Errorf("the foreign entry did not survive byte for byte:\n%s", doc.Bytes())
	}
}

// TestRemoveIfAnyInstallID is the detach --all path. With the store gone there
// is no id left to match against, so the predicate accepts every install -- and
// the entry belonging to no install still has to survive it.
func TestRemoveIfAnyInstallID(t *testing.T) {
	doc := seedDocument(t)

	n, _, err := RemoveIf(doc, func(string) bool { return true }, false)
	if err != nil {
		t.Fatalf("RemoveIf: %v", err)
	}
	if want := 2 * len(Events()); n != want {
		t.Errorf("removed %d entries, want %d", n, want)
	}
	for _, event := range Events() {
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

	n, _, err := Remove(doc, Spec{InstallID: oneID}, false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if n != len(Events()) {
		t.Errorf("removed %d entries, want %d", n, len(Events()))
	}
	for _, event := range Events() {
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
		string(mustEntry(t, spec(oneID), EventPreToolUse)) + `,` + narrowed + `]}}`))
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	before := string(doc.Bytes())

	n, _, err := RemoveIf(doc, func(string) bool { return true }, false)
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
	for i, event := range Events() {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%q:[%s,%s,%s]", event, foreignHook, mustEntry(t, spec(oneID), event), mustEntry(t, spec(otherID), event))
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
	for _, tc := range []struct {
		name, goos, in, want string
	}{
		// POSIX (darwin / linux)
		{"posix: safe path left bare",
			"linux", "/usr/local/bin/rashomon", "/usr/local/bin/rashomon"},
		{"posix: space is single-quoted",
			"linux", "/Users/sam/Application Support/rashomon", "'/Users/sam/Application Support/rashomon'"},
		{"posix: single quote is closed, escaped and reopened",
			"linux", "/home/o'brien/bin/rashomon", `'/home/o'\''brien/bin/rashomon'`},
		{"posix: darwin safe path left bare",
			"darwin", "/usr/local/bin/rashomon", "/usr/local/bin/rashomon"},

		// Windows (cmd.exe double-quote syntax)
		{"windows: safe path left bare",
			"windows", `C:\Users\sam\go\bin\rashomon.exe`, `C:\Users\sam\go\bin\rashomon.exe`},
		{"windows: space is double-quoted",
			"windows", `C:\Users\sam\My Go Bin\rashomon.exe`, `"C:\Users\sam\My Go Bin\rashomon.exe"`},
		{"windows: double quote inside path is doubled",
			"windows", `C:\weird"path\rashomon.exe`, `"C:\weird""path\rashomon.exe"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellQuoteForOS(tc.in, tc.goos); got != tc.want {
				t.Errorf("shellQuoteForOS(%q, %q) = %s, want %s", tc.in, tc.goos, got, tc.want)
			}
		})
	}
}

// mustEntry renders an entry and fails the test rather than ignoring the error,
// which is what the old panicking signature let callers do implicitly.
func mustEntry(t *testing.T, s Spec, event string) json.RawMessage {
	t.Helper()
	raw, err := s.Entry(event)
	if err != nil {
		t.Fatalf("rendering the %s entry: %v", event, err)
	}
	return raw
}

// TestCoverageIntact_MatcherEquivalenceAndExtraHooksDoNotCostCoverage:
// coverageIntact, unlike intact, asks only whether a group's matcher fires
// on every call this event raises and whether a command hook is in the
// group at all. A different timeout and an extra hook alongside ours change
// how fast or how often something ELSE in the group runs, never whether
// ours does, so neither is asked here; intact's own job -- catching either
// one so Apply and RemoveIf do not silently overwrite or remove a hand
// edit -- is untouched.
func TestCoverageIntact_MatcherEquivalenceAndExtraHooksDoNotCostCoverage(t *testing.T) {
	ptr := func(s string) *string { return &s }
	cases := []struct {
		name  string
		event string
		raw   string
		want  string // "" means coverageIntact accepts the group
	}{
		{"an absent matcher fires on every call", EventPreToolUse,
			`{"hooks":[{"type":"command","command":"x","timeout":5}]}`, ""},
		{"an empty matcher fires on every call", EventPreToolUse,
			`{"matcher":"","hooks":[{"type":"command","command":"x","timeout":5}]}`, ""},
		{"a star matcher fires on every call", EventPreToolUse,
			`{"matcher":"*","hooks":[{"type":"command","command":"x","timeout":5}]}`, ""},
		{"a narrowed matcher does not fire on every call", EventPreToolUse,
			`{"matcher":"Bash","hooks":[{"type":"command","command":"x","timeout":5}]}`, "matcher"},
		{"a matcher on an event with none narrows it", EventSessionStart,
			`{"matcher":"startup","hooks":[{"type":"command","command":"x","timeout":5}]}`, "matcher"},
		{"a star matcher on a session event still fires on every call", EventSessionStart,
			`{"matcher":"*","hooks":[{"type":"command","command":"x","timeout":5}]}`, ""},
		{"a different timeout does not cost coverage", EventPreToolUse,
			`{"matcher":"*","hooks":[{"type":"command","command":"x","timeout":600}]}`, ""},
		{"a second hook in the group does not cost coverage", EventPreToolUse,
			`{"matcher":"*","hooks":[{"type":"command","command":"x","timeout":5},` +
				`{"type":"command","command":"other","timeout":30}]}`, ""},
		{"no command hook at all", EventPreToolUse,
			`{"matcher":"*","hooks":[]}`, "command"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coverageIntact(json.RawMessage(c.raw), c.event)
			if c.want == "" && got != "" {
				t.Errorf("coverageIntact = %q, want accepted", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("coverageIntact = %q, want it to mention %q", got, c.want)
			}
		})
	}

	// matcherFires itself, the equivalence coverageIntact and intact
	// disagree on. intact (Apply, RemoveIf) still requires the literal
	// string "*" and refuses nil or "" as a modification, which is correct
	// for them: it is deciding whether to overwrite, not whether the group
	// still fires.
	for _, tc := range []struct {
		event string
		m     *string
		want  bool
	}{
		{EventPreToolUse, nil, true},
		{EventPreToolUse, ptr(""), true},
		{EventPreToolUse, ptr(Matcher), true},
		{EventPreToolUse, ptr("Bash"), false},
		{EventSessionStart, nil, true},
		{EventSessionStart, ptr(""), true},
		{EventSessionStart, ptr(Matcher), true},
		{EventSessionStart, ptr("startup"), false},
	} {
		if got := matcherFires(tc.event, tc.m); got != tc.want {
			name := "nil"
			if tc.m != nil {
				name = *tc.m
			}
			t.Errorf("matcherFires(%s, %q) = %v, want %v", tc.event, name, got, tc.want)
		}
	}
}

// TestPresent_TimeoutDifferentFromWatchStillReadsPresent: an entry whose
// timeout differs from the one watch installs, under our own command and
// matcher, still reads Present, because that is what coverage asks:
// whether the command fires on every call, not whether every field of the
// entry matches byte for byte. Before Present asked intact instead of
// coverageIntact, a timeout edit -- the most natural edit there is to make
// to one of these entries -- made a verified run read hook_entry absent.
func TestPresent_TimeoutDifferentFromWatchStillReadsPresent(t *testing.T) {
	raw := mustEntry(t, spec(oneID), EventPreToolUse)
	var g map[string]any
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	g["hooks"].([]any)[0].(map[string]any)["timeout"] = 600
	retimed, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}

	doc := &settings.Document{}
	if err := doc.SetHookEntries(EventPreToolUse, []json.RawMessage{retimed}); err != nil {
		t.Fatal(err)
	}
	present, err := Present(doc, oneID, EventPreToolUse)
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Error("Present = false for an entry retimed to 600, want true: coverageIntact does not ask about timeout")
	}
}
