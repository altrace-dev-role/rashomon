package hygiene

import (
	"strings"
	"testing"
)

// TestRewriteCopiesWhatItLeavesAlone: with nothing to replace, rewrite gives
// back the document byte for byte -- escapes, spacing, number spellings and
// member order -- which is what lets a scrubbed fixture's diff show only
// what was removed.
func TestRewriteCopiesWhatItLeavesAlone(t *testing.T) {
	for _, doc := range []string{
		`{}`, `[]`, `{"a":1e3,"b":-0.50,"c":[true,false,null],"d":"caf\u00e9 \/ \"q\" \\ \n"}`,
		"{\n  \"tools\" : [ \"Bash\" , {\"x\":[1,2]} ],\n  \"z\": \"\\ud83d\\ude00\"\n}",
		`[{"skills":[]},{"agents":[[],[[]]]}]`, `{"k":"{\"nested\":[\"x\"]}"}`,
	} {
		got := rewrite(doc, func(_, s string, _ int) string { return s }, func(string, int, int) bool { return false })
		if got != doc {
			t.Errorf("rewrite changed a document it had nothing to replace:\n got %s\nwant %s", got, doc)
		}
	}
}

// TestInventoryListsBecomeTheirLength: each inventory member's array, however
// nested or spaced, is replaced by how many elements it holds at its own
// level, and nothing else is touched.
func TestInventoryListsBecomeTheirLength(t *testing.T) {
	for in, want := range map[string]string{
		`{"tools":[]}`:                           `{"tools":0}`,
		`{"tools":[ ]}`:                          `{"tools":0}`,
		`{"skills":["a"]}`:                       `{"skills":1}`,
		`{"agents":["a,b",["c","d"],{"e":[1]}]}`: `{"agents":3}`,
		`{"plugins" : [{"name":"x","path":"/p"}, {"name":"y"}],"n":1}`: `{"plugins" : 2,"n":1}`,
		`{"tools":"Bash"}`:           `{"tools":"Bash"}`,
		`{"allowed_tools":["Bash"]}`: `{"allowed_tools":["Bash"]}`,
		`["tools",["Bash"]]`:         `["tools",["Bash"]]`,
	} {
		got := rewrite(in, func(_, s string, _ int) string { return s }, func(string, int, int) bool { return true })
		if got != want {
			t.Errorf("rewrite(%s) = %s, want %s", in, got, want)
		}
	}
}

// TestPathShapesStayNarrow: the home, project-directory and temp patterns
// match the paths a machine writes and not the same words elsewhere -- a URL
// path, a directory that merely ends in home, a flag that begins -root.
func TestPathShapesStayNarrow(t *testing.T) {
	for s, want := range map[string]bool{
		"/home/alice/src":                     true,
		"cd /Users/alice && ls":               true,
		"/root/.claude/settings.json":         true,
		`C:\Users\alice\repo`:                 true,
		`C:\\Users\\alice\\repo`:              true,
		"file:///home/alice/x":                true,
		"~/.claude/projects/-home-alice-app/": true,
		"projects/-root/x.jsonl":              true,
		"projects/-Users-alice-app":           true,
		"https://host.example/home/alice":     false,
		"/var/home/alice":                     false,
		"/rootfs/etc":                         false,
		"run --rooted -home-x":                false,
		"~/src/app":                           false,
		"projects/-project/x.jsonl":           false,
	} {
		if got := homeIn(s); got != want {
			t.Errorf("homeIn(%q) = %v, want %v", s, got, want)
		}
	}
	for s, want := range map[string]bool{
		"/tmp/x": true, "/private/tmp/x": true, "/var/folders/ab/cd12/T/x": true, "/var/tmp": true,
		`C:\Users\a\AppData\Local\Temp\x`: true, "/tmpfs/x": false, "https://h/tmp/x": false, "$TMPDIR/x": false,
	} {
		if got := tempRE.MatchString(s); got != want {
			t.Errorf("temp root in %q = %v, want %v", s, got, want)
		}
	}
}

// TestEmailAndUUIDShapes: an address at a reserved domain, git@ of an SSH
// remote and a version pin are not a person's; a UUID is an account's only
// inside a path and only when it is not a placeholder or a slice of a longer
// run.
func TestEmailAndUUIDShapes(t *testing.T) {
	for s, want := range map[string]bool{
		"alice@acme-corp.io": true, "<a.b+c@mail.example.co.uk>": true, "user-1@example.invalid": false,
		"x@example.com": false, "git@github.com:org/repo.git": false, "react@18.3.1": false, "a@localhost": false,
	} {
		if got := len(emailSpans(s)) > 0; got != want {
			t.Errorf("email in %q = %v, want %v", s, got, want)
		}
	}
	const u = "5f0c8a12-3b7e-4c1d-9a2f-6e8b4d0c7a31"
	for s, want := range map[string]bool{
		"~/.claude/projects/-project/" + u + ".jsonl": true, `C:\x\` + u: true, u: false, "session " + u: false,
		"/p/" + placeholderUUID + "4000-8000-000000000001": false, "/p/x" + u: false,
	} {
		if got := uuidInPath(s); got != want {
			t.Errorf("uuidInPath(%q) = %v, want %v", s, got, want)
		}
	}
}

// TestScrubIsStableAndIdempotent: one Scrubber numbers ids in order of first
// appearance, case aside, and output scrubbed again is unchanged.
func TestScrubIsStableAndIdempotent(t *testing.T) {
	z := NewScrubber("/srv/people/alice")
	in := strings.Join([]string{
		`{"a":"5F0C8A12-3B7E-4C1D-9A2F-6E8B4D0C7A31","b":"/srv/people/alice/x","c":"/srv/people/alicex"}`,
		`{"a":"5f0c8a12-3b7e-4c1d-9a2f-6e8b4d0c7a31","m":"bob@corp.io bob@corp.io carol@corp.io"}`,
	}, "\n")
	out, err := z.Scrub("x.ndjson", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		`{"a":"00000000-0000-4000-8000-000000000001","b":"~/x","c":"/srv/people/alicex"}`,
		`{"a":"00000000-0000-4000-8000-000000000001","m":"user-1@example.invalid user-1@example.invalid user-2@example.invalid"}`,
	}, "\n")
	if string(out) != want {
		t.Errorf("Scrub:\n got %s\nwant %s", out, want)
	}
	again, _ := z.Scrub("x.ndjson", out)
	if string(again) != string(out) {
		t.Errorf("a second Scrub changed the output:\n%s", again)
	}
	if _, err := z.Scrub("x.tar.gz", out); err == nil {
		t.Error("Scrub accepted an archive")
	}
}
