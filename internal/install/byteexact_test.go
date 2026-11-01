package install

import (
	"bytes"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// raSeed is, byte for byte, what test/live/ra-1.sh row A8 seeds into the
// scratch config dir before watch: an unrelated tool's settings, with its own
// PreToolUse hook written on one line.
const raSeed = "{\n  \"permissions\": {\"allow\": [\"Bash(ls:*)\"]},\n  \"env\": {\"RA_SEED\": \"1\"},\n" +
	"  \"hooks\": {\"PreToolUse\": [{\"matcher\": \"Bash\", \"hooks\": [{\"type\": \"command\", \"command\": \"true\"}]}]}\n}\n"

// detachByteExactSeeds are files whose hooks the user formatted their own way.
// watch then detach must hand back every one of them as the same bytes.
var detachByteExactSeeds = map[string]string{
	"ra-1 A8 seed (one-line hooks)": raSeed,
	"multi-line hooks, two foreign events": "{\n  \"hooks\": {\n    \"PreToolUse\": [\n      {\"matcher\": \"Bash\", \"hooks\": [{\"type\": \"command\", \"command\": \"a\"}]}\n    ],\n" +
		"    \"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"b\"}]}]\n  }\n}\n",
	"tabs and a compact hooks object":  "{\"hooks\":{\"PreToolUse\":[{\"matcher\":\"Bash\",\"hooks\":[{\"type\":\"command\",\"command\":\"a\"}]},{\"matcher\":\"Edit\",\"hooks\":[]}]}}\n",
	"foreign Stop beside our four":     "{\n  \"hooks\": {\"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"b\"}]}], \"Other\": []}\n}\n",
	"no hooks at all":                  "{\n  \"env\": {\"A\": \"1\"}\n}\n",
	"compact top level, no hooks":      "{\"env\": {\"A\": \"1\"}, \"x\": 2}",
	"hooks with a trailing odd layout": "{\n  \"hooks\" : { \"PreToolUse\" :   [ {\"hooks\": []} ,\n   {\"hooks\": [1]} ]  }\n}\n",
}

func TestDetachByteExact_WatchThenDetachRestoresTheBytes(t *testing.T) {
	for name, seed := range detachByteExactSeeds {
		t.Run(name, func(t *testing.T) {
			doc, err := settings.Parse([]byte(seed))
			if err != nil {
				t.Fatalf("seeding: %v", err)
			}
			if changed, err := Apply(doc, spec(oneID)); err != nil || !changed {
				t.Fatalf("Apply: changed=%v err=%v", changed, err)
			}
			// Through the bytes, as the files do: detach reads what watch wrote.
			installed := doc.Bytes()
			back, err := settings.Parse(installed)
			if err != nil {
				t.Fatalf("what watch wrote does not parse: %v", err)
			}
			if _, _, err := Remove(back, spec(oneID), false); err != nil {
				t.Fatalf("Remove: %v", err)
			}
			if got := back.Bytes(); !bytes.Equal(got, []byte(seed)) {
				t.Errorf("detach did not restore the file byte for byte\nseed:      %q\ninstalled: %q\ngot:       %q", seed, installed, got)
			}
		})
	}
}

// A user edit between watch and detach (a new permission, a hook of their own
// under an event we also use) survives; only our entries go.
func TestDetachByteExact_UserEditBetweenWatchAndDetachSurvives(t *testing.T) {
	doc, err := settings.Parse([]byte(raSeed))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(doc, spec(oneID)); err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(doc.Bytes(), []byte(`"Bash(ls:*)"`), []byte(`"Bash(ls:*)", "Bash(pwd:*)"`), 1)
	if bytes.Equal(edited, doc.Bytes()) {
		t.Fatal("the edit did not apply")
	}
	back, err := settings.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Remove(back, spec(oneID), false); err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace([]byte(raSeed), []byte(`"Bash(ls:*)"`), []byte(`"Bash(ls:*)", "Bash(pwd:*)"`), 1)
	if got := back.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("a user's edit did not survive detach exactly\nwant: %q\ngot:  %q", want, got)
	}
}

// A file written on one line keeps its entry on one line: the layout is the
// user's, and so is the line a diff of the file shows.
func TestDetachByteExact_InlineHooksStayInline(t *testing.T) {
	doc, err := settings.Parse([]byte(raSeed))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(doc, spec(oneID)); err != nil {
		t.Fatal(err)
	}
	const want = `  "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "true"}]}, {"matcher":"*","hooks":[{"type":"command","command":"/usr/local/bin/rashomon hook --install `
	if !strings.Contains(string(doc.Bytes()), want) {
		t.Errorf("the one-line hooks object was reflowed:\n%s", doc.Bytes())
	}
}

// What watch adds follows the layout the file already has between entries:
// the separator of its last two entries, or the line break and indentation of
// its only one.
func TestDetachByteExact_AddedEntryFollowsTheFilesSeparator(t *testing.T) {
	for _, c := range []struct{ name, seed, want string }{
		{"compact pair", detachByteExactSeeds["tabs and a compact hooks object"], `{"matcher":"Edit","hooks":[]},{"matcher":"*"`},
		{"one multi-line entry", detachByteExactSeeds["multi-line hooks, two foreign events"], "\"command\": \"a\"}]},\n      {\n        \"matcher\": \"*\""},
	} {
		doc, err := settings.Parse([]byte(c.seed))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Apply(doc, spec(oneID)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(doc.Bytes()), c.want) {
			t.Errorf("%s: the added entry does not follow the file's layout:\n%s", c.name, doc.Bytes())
		}
	}
}
