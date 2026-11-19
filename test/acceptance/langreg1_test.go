package acceptance

// LANG-REG1: the hook reaches a file's reader through the language registry
// (lang.Registry, index.Wire), not through the Go resolver by name. The
// registry, the kinds table and the wiring exist before any hook calls
// them, so a Go session's bytes cannot move yet. These two tests are the
// controls the change that rewires the hook must keep: an
// edit of a file no language reads (a kinds-table row) says nothing and
// keeps nothing, and a Go edit writes the same overlay group, the same
// unit-brief miss and the same stdout as the base binary did.
//
// The project is test/fixtures/lang/py-unit: a Go module with one package,
// svc, and a Python unit inv of two modules, both declared in its
// .rashomon/graph.json, synced by pgraph and indexed by `rashomon graph
// index`. Its .git is empty, so the index's commit is C1 on every machine.

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/intent"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lrFixture is the py-unit fixture, relative to the module root.
const lrFixture = "test/fixtures/lang/py-unit"

// lrProject is a copy of the py-unit fixture in a git project of its own,
// synced by pgraph and indexed, and the index's header.
func (e *env) lrProject() (string, graph.Header) {
	e.t.Helper()
	return e.lrIndexed(e.lrFiles())
}

// lrFiles is the py-unit fixture's five files, by slash path.
func (e *env) lrFiles() map[string]string {
	e.t.Helper()
	src := filepath.Join(moduleRoot, filepath.FromSlash(lrFixture))
	files := map[string]string{}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		files[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		e.t.Fatal(err)
	}
	if len(files) != 5 {
		e.t.Fatalf("premise: the fixture holds %d files, want 5: %v", len(files), files)
	}
	return files
}

// lrIndexed is files in a git project of its own, synced by pgraph and
// indexed, and the index's header.
func (e *env) lrIndexed(files map[string]string) (string, graph.Header) {
	e.t.Helper()
	dir := gi1Project(e.t, files)
	h156Sync(e.t, dir)
	if res := e.run("", nil, "graph", "index", dir); res.exitCode != 0 {
		e.t.Fatalf("graph index: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	hdr, err := graph.ReadIndexHeader(filepath.Join(dir, filepath.FromSlash(graph.GraphFile)))
	if err != nil || hdr.Rev.Commit != "C1" || hdr.Repo == "" {
		e.t.Fatalf("premise: the index header is %+v (%v), want commit C1 under a repository id", hdr, err)
	}
	return dir, hdr
}

// lrEdit runs an Edit of dir's file through the hook and the post, with
// dir as the working directory, and returns the post's stdout. The hook
// must print nothing.
func (e *env) lrEdit(dir, id, file, old string) string {
	e.t.Helper()
	pre, post := e.lrCall(dir, id, file, old)
	if res := e.hook(pre, lrEnv...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	res := e.post(post, lrEnv...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// lrEnv is the environment every LANG-REG1 prompt, hook and post runs under.
var lrEnv = []string{stAgent, "CI="}

// lrCall is the PreToolUse and PostToolUse payloads of an Edit of dir's
// file that replaces old with itself, with dir as the working directory.
func (e *env) lrCall(dir, id, file, old string) (pre, post string) {
	e.t.Helper()
	input := map[string]any{"file_path": filepath.Join(dir, filepath.FromSlash(file)), "old_string": old, "new_string": old}
	resp := map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
	return stPayload(e.t, "PreToolUse", id, "Edit", input, map[string]any{"cwd": dir}),
		stPayload(e.t, "PostToolUse", id, "Edit", input, map[string]any{"cwd": dir, "tool_response": resp})
}

// lrPrompt sends prompt as the session's UserPromptSubmit, with dir as the
// working directory, through the entry watch installs for it (recap
// pending), and returns what it printed.
func (e *env) lrPrompt(dir, id, prompt string) string {
	e.t.Helper()
	b, err := json.Marshal(map[string]any{"session_id": testSession, "hook_event_name": "UserPromptSubmit",
		"transcript_path": "/tmp/transcripts/" + testSession + ".jsonl", "cwd": dir, "permission_mode": "default",
		"prompt": prompt, "prompt_id": id})
	if err != nil {
		e.t.Fatal(err)
	}
	res := e.run(string(b), lrEnv, append([]string{"recap", "pending"}, e.installArgs()...)...)
	if res.exitCode != 0 {
		e.t.Fatalf("recap pending %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	return res.stdout
}

// lrSaid runs the Edit lrCall builds through the hook and the post and
// returns what both printed, the hook's first. A line the session discloses
// to the person is read from either, so these tests fix its text and leave
// its channel to the code that prints it; the lines hold no character a
// JSON encoder rewrites, so a line inside a JSON envelope reads the same.
func (e *env) lrSaid(dir, id, file, old string) string {
	e.t.Helper()
	pre, post := e.lrCall(dir, id, file, old)
	h := e.hook(pre, lrEnv...)
	if h.exitCode != 0 {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, h.exitCode, h.stdout, h.stderr)
	}
	p := e.post(post, lrEnv...)
	if p.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stdout %q, stderr %q", id, p.exitCode, p.stdout, p.stderr)
	}
	return h.stdout + p.stdout
}

// lrLog is the store's session graph log for the index's repository,
// false when there is none.
func (e *env) lrLog(hdr graph.Header) (string, bool) {
	e.t.Helper()
	b, err := os.ReadFile(graph.SessionLogPath(e.home, hdr.Repo))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b), true
}

// lrMisses is the session's unitbrief_miss records, line by line.
func (e *env) lrMisses() []string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "runs", testSession, "records.ndjson"))
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for _, l := range strings.SplitAfter(string(b), "\n") {
		if strings.Contains(l, `"type":"`+store.TypeUnitBriefMiss+`"`) {
			out = append(out, l)
		}
	}
	return out
}

// TestLANGREG1_ANonSourceKindIsSilent: in an indexed project whose
// readers are go and py, an edit of a file of a kinds-table row prints
// nothing, keeps no overlay and counts no unit-brief miss: a Markdown file,
// and go.mod in a Go session. Break: a hook
// that discloses every unclaimed kind (the no-reader line with no kinds
// table) prints a line for each.
func TestLANGREG1_ANonSourceKindIsSilent(t *testing.T) {
	e := newSteerEnv(t)
	dir, hdr := e.lrProject()
	ai1Write(t, dir, "NOTES.md", "# notes\n")
	for _, c := range []struct{ id, file, old string }{{"toolu_k1", "NOTES.md", "notes"}, {"toolu_k2", "go.mod", "module"}} {
		if got := e.lrEdit(dir, c.id, c.file, c.old); got != "" {
			t.Errorf("an edit of %s printed %q, want 0 bytes", c.file, got)
		}
		if got, ok := e.lrLog(hdr); ok {
			t.Errorf("an edit of %s kept a session graph log: %q", c.file, got)
		}
		if got := e.lrMisses(); len(got) != 0 {
			t.Errorf("an edit of %s counted a unit-brief miss: %q", c.file, got)
		}
	}
}

// lrMS is a record's stamp, the one field of the Go control the clock sets.
var lrMS = regexp.MustCompile(`"recorded_at_unix_ms":\d{13}`)

// lrGoWant is what main's binary at aa5fd4ae, before the registry, wrote for
// TestLANGREG1_AGoEditIsByteEqual's edit, normalized by lrNormal.
const lrGoWant = `stdout:

log:
{"schema":1,"repo":"<RID>","rev":{"commit":"C1"},"producers":{"session":"session/v1"},"records":0}
{"Node":{"ID":"file:svc/svc.go","Kind":"file","Attrs":{"digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","lang":"go"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"unit:go:svc","Pred":"contains","Dst":"file:svc/svc.go","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"sym:go:svc:func:Name","Kind":"sym","Attrs":{"calls":"ToUpper","doc":"Name is the service name in upper case.","name":"Name","symkind":"func"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"file:svc/svc.go","Pred":"declares","Dst":"sym:go:svc:func:Name","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"ovgroup:sess-1:3:landed","Kind":"overlay_group","Attrs":{"carried":"declares,imports","count":"4","digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","file":"file:svc/svc.go","kind":"landed","seq":"3","session_id":"sess-1","tool_use_id":"toolu_g1"},"Tier":"observed","Conf":1,"Status":"asserted","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
misses:
{"type":"unitbrief_miss","schema_version":5,"critical":false,"seq":4,"recorded_at_unix_ms":<MS>,"session_id":"sess-1","landed_seq":3,"unit":"<unit:go:svc>","reason":"absent"}
`

// lrNormal is the Go control's three outputs as one text, with the
// project's directory, the repository id and the clock stamp replaced by
// names. The miss record's unit digest is replaced only where it is the
// digest of unit:go:svc under this store's key, so a miss for another unit
// stays in the text and fails the comparison.
func (e *env) lrNormal(dir string, hdr graph.Header, stdout, log string, misses []string) string {
	e.t.Helper()
	st, err := store.OpenExisting(e.home)
	if err != nil {
		e.t.Fatal(err)
	}
	unit := intent.PathDigest(st.Key(), string(graph.GlobalID(hdr.Repo, graph.UnitID("go", "svc"))))
	var b strings.Builder
	b.WriteString("stdout:\n" + stdout + "\nlog:\n" + log + "misses:\n")
	for _, l := range misses {
		b.WriteString(lrMS.ReplaceAllString(strings.ReplaceAll(l, unit, "<unit:go:svc>"), `"recorded_at_unix_ms":<MS>`))
	}
	return strings.NewReplacer(dir, "<ROOT>", hdr.Repo, "<RID>").Replace(b.String())
}

// lrGoWantSplice and lrGoWantBrief are what the base binary wrote for the
// two later cases of TestLANGREG1_AGoEditIsByteEqual, normalized by lrNormal.
const (
	lrGoWantSplice = `stdout:

log:
{"schema":1,"repo":"<RID>","rev":{"commit":"C1"},"producers":{"session":"session/v1"},"records":0}
{"Node":{"ID":"file:svc/svc.go","Kind":"file","Attrs":{"digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","lang":"go"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"unit:go:svc","Pred":"contains","Dst":"file:svc/svc.go","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"sym:go:svc:func:Name","Kind":"sym","Attrs":{"calls":"ToUpper","doc":"Name is the service name in upper case.","name":"Name","symkind":"func"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"file:svc/svc.go","Pred":"declares","Dst":"sym:go:svc:func:Name","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"ovgroup:sess-1:1:declared","Kind":"overlay_group","Attrs":{"carried":"declares,imports","count":"4","digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","file":"file:svc/svc.go","kind":"declared","seq":"1","session_id":"sess-1","tool_use_id":"toolu_g1"},"Tier":"observed","Conf":1,"Status":"asserted","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":{"ID":"file:svc/svc.go","Kind":"file","Attrs":{"digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","lang":"go"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"unit:go:svc","Pred":"contains","Dst":"file:svc/svc.go","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"sym:go:svc:func:Name","Kind":"sym","Attrs":{"calls":"ToUpper","doc":"Name is the service name in upper case.","name":"Name","symkind":"func"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"file:svc/svc.go","Pred":"declares","Dst":"sym:go:svc:func:Name","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"ovgroup:sess-1:3:landed","Kind":"overlay_group","Attrs":{"carried":"declares,imports","count":"4","digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","file":"file:svc/svc.go","kind":"landed","seq":"3","session_id":"sess-1","tool_use_id":"toolu_g1"},"Tier":"observed","Conf":1,"Status":"asserted","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
misses:
{"type":"unitbrief_miss","schema_version":5,"critical":false,"seq":4,"recorded_at_unix_ms":<MS>,"session_id":"sess-1","landed_seq":3,"unit":"<unit:go:svc>","reason":"absent"}
`
	lrGoWantBrief = `stdout:
prompt:
{"hookSpecificOutput":{"additionalContext":"rashomon record: the code this request touches, read from the repository graph at rev C1: 2 node(s) matched, 4 reached.\nunit 'svc' (named in the request) is consumed by 'app/app.go', 'app2/app2.go'\n'svc/svc.go'\nconsumer 'app/app.go' declares 'Run'; calls 'Name'","hookEventName":"UserPromptSubmit"},"systemMessage":"` + "\u203b" + ` rashomon told the agent: \"rashomon record: the code this request touches, read from the repository graph at rev C1: 2 node(s) matched, 4 reached.\nunit 'svc' (named in the request) is consumed by 'app/app.go', 'app2/app2.go'\n'svc/svc.go'\nconsumer 'app/app.go' declares 'Run'; calls 'Name'\""}post:
{"hookSpecificOutput":{"additionalContext":"rashomon record: an edit in unit 'svc', noted once a session; the repository graph at rev C1 records for it:\nconsumer 'app2/app2.go' declares 'Tag'; calls 'Other'","hookEventName":"PostToolUse"},"systemMessage":"` + "\u203b" + ` rashomon told the agent: \"rashomon record: an edit in unit 'svc', noted once a session; the repository graph at rev C1 records for it:\nconsumer 'app2/app2.go' declares 'Tag'; calls 'Other'\""}

log:
{"schema":1,"repo":"<RID>","rev":{"commit":"C1"},"producers":{"session":"session/v1"},"records":0}
{"Node":{"ID":"file:svc/svc.go","Kind":"file","Attrs":{"digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","lang":"go"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"unit:go:svc","Pred":"contains","Dst":"file:svc/svc.go","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"sym:go:svc:func:Name","Kind":"sym","Attrs":{"calls":"ToUpper","doc":"Name is the service name in upper case.","name":"Name","symkind":"func"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"file:svc/svc.go","Pred":"declares","Dst":"sym:go:svc:func:Name","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"ovgroup:sess-1:1:declared","Kind":"overlay_group","Attrs":{"carried":"declares,imports","count":"4","digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","file":"file:svc/svc.go","kind":"declared","seq":"1","session_id":"sess-1","tool_use_id":"toolu_g1"},"Tier":"observed","Conf":1,"Status":"asserted","Rev":{"Kind":"overlay","Commit":"C1","Seq":1},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":{"ID":"file:svc/svc.go","Kind":"file","Attrs":{"digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","lang":"go"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"unit:go:svc","Pred":"contains","Dst":"file:svc/svc.go","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"sym:go:svc:func:Name","Kind":"sym","Attrs":{"calls":"ToUpper","doc":"Name is the service name in upper case.","name":"Name","symkind":"func"},"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
{"Node":null,"Edge":{"Src":"file:svc/svc.go","Pred":"declares","Dst":"sym:go:svc:func:Name","Attrs":null,"Tier":"algorithmic","Conf":0.8,"Status":"","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null},"Certain":false}}
{"Node":{"ID":"ovgroup:sess-1:3:landed","Kind":"overlay_group","Attrs":{"carried":"declares,imports","count":"4","digest":"2ef112d79e9334c8a46f5e31359588685b3f74d9fde18d40f8812819d7d1df75","file":"file:svc/svc.go","kind":"landed","seq":"3","session_id":"sess-1","tool_use_id":"toolu_g1"},"Tier":"observed","Conf":1,"Status":"asserted","Rev":{"Kind":"overlay","Commit":"C1","Seq":3},"Prov":{"Extractor":"","Version":"","Evidence":null}},"Edge":null}
misses:
`
)

// lrPyOnly is a configuration that declares the py-unit fixture's Python
// unit and not its Go one.
const lrPyOnly = `{"units": [{"path": "inv", "mod_root": "", "modules": ["inv"], "lang": "py"}]}` + "\n"

// lrConsumer is a Go package of the py-unit project's module that calls
// svc.Name, so the go:svc unit has a consumer. A prompt that names Name
// reaches it (Name, then Run, then app/app.go), so the prompt brief says
// it and the unit brief leaves it out (engine.Deltas).
const lrConsumer = "package app\n\nimport \"example.com/inv/svc\"\n\n// Run is the service's name for inv.\nfunc Run() string { return svc.Name(\"inv\") }\n"

// lrOther is a second file of the go:svc unit, and lrConsumer2 a Go
// package that calls only its Other. A prompt that names svc/svc.go and
// Name does not reach app2, so the unit brief's consumer line for it is
// new, and the edit's post delivers it.
const (
	lrOther     = "package svc\n\n// Other is the service's second name.\nfunc Other() string { return \"other\" }\n"
	lrConsumer2 = "package app2\n\nimport \"example.com/inv/svc\"\n\n// Tag is the service's second name for app2.\nfunc Tag() string { return svc.Other() }\n"
)

// lrBriefTitle opens the note a post prints when it delivers a unit brief.
const lrBriefTitle = "rashomon record: an edit in unit"

// TestLANGREG1_AGoEditIsByteEqual: an edit of svc/svc.go in the
// indexed py-unit project gives the base binary's bytes: what the session
// printed, the session graph log (its header, the overlay's records and its
// groups) and the unit-brief misses the edit counts. The project declares a
// Python unit beside the Go one, so a registry that let the py unit's
// configuration reach a Go file would show here. Four cases:
//   - "refused splice": old_string "package", which svc.go holds twice (its
//     doc comment and its clause), so the PreToolUse writer's splice
//     (internal/hook/overlaywrite.go spliceEdits) refuses it and writes no
//     declared group; the post writes the landed group from the bytes on
//     disk, and the miss is absent, since the session holds no brief.
//   - "declared and landed": old_string "ToUpper", which svc.go holds once,
//     so the declared group is written too.
//   - "brief delivered": a prompt naming svc/svc.go and Name runs first. Its
//     graph brief says app (lrConsumer) and stores a unit brief for go:svc
//     that leaves app out and names app2 (lrConsumer2, which calls
//     svc.Other in svc/other.go); the edit's post delivers that brief. What
//     the session printed is the prompt's output, then the post's.
//   - "no go producer": the project is indexed while its configuration
//     declares the py unit only (lrPyOnly), so the index holds py records
//     and no go ones; then the whole configuration is written back and the
//     refused splice runs. Go is not a language the index must hold records
//     of (index.Wire registers it plain), so the bytes are the first case's.
//
// Breaks: build the Go overlay with no import map, and every landed group's
// carried set loses Imports; say the unindexed-language line for every
// language the producers lack, and the fourth case prints it for go and
// writes no group.
func TestLANGREG1_AGoEditIsByteEqual(t *testing.T) {
	for _, c := range []struct {
		name, old, prompt, want, config string
		extra                           map[string]string
	}{
		{"refused splice", "package", "", lrGoWant, "", nil},
		{"declared and landed", "ToUpper", "", lrGoWantSplice, "", nil},
		{"brief delivered", "ToUpper", "make Name in svc/svc.go trim the spaces around s before it upper-cases it", lrGoWantBrief, "",
			map[string]string{"app/app.go": lrConsumer, "svc/other.go": lrOther, "app2/app2.go": lrConsumer2}},
		{"no go producer", "package", "", lrGoWant, lrPyOnly, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			files := e.lrFiles()
			for k, v := range c.extra {
				files[k] = v
			}
			full := files[index.ConfigFile]
			if c.config != "" {
				files[index.ConfigFile] = c.config
			}
			dir, hdr := e.lrIndexed(files)
			if c.config != "" {
				_, py := hdr.Producers[pylite.Lang]
				if _, gol := hdr.Producers["go"]; gol || !py {
					t.Fatalf("premise: the index's producers are %v, want %s and not go", hdr.Producers, pylite.Lang)
				}
				ai1Write(t, dir, index.ConfigFile, full)
			}
			said := ""
			if c.prompt != "" {
				said = "prompt:\n" + e.lrPrompt(dir, "prompt-1", c.prompt) + "post:\n"
			}
			post := e.lrEdit(dir, "toolu_g1", "svc/svc.go", c.old)
			if c.prompt != "" && !strings.Contains(post, lrBriefTitle) {
				t.Errorf("premise: the post printed %q, want the unit brief's note %q", post, lrBriefTitle)
			}
			log, ok := e.lrLog(hdr)
			if !ok {
				t.Fatal("the Go edit kept no session graph log")
			}
			got := e.lrNormal(dir, hdr, said+post, log, e.lrMisses())
			if got != c.want {
				t.Errorf("the Go edit's bytes differ from the base binary's\ngot:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

// The lines the session discloses: a file kind no reader here claims
// (lrSayTS, and lrSaySH for .sh, the line being keyed by extension), a
// language the index holds no records of (lrSayPy), and a configuration
// that was not read (lrSayUnread, which keeps its <file>, <reason> and
// <ext> to be filled). The readers lrSayTS and lrSaySH name, go and py, are
// this build's registry (index.Wire) in its order, which the line is
// composed from and never holds as a constant.
const (
	lrSayTS     = "rashomon: no reader here for .ts files, so edits to them carry no unit, caller or conflict facts this session (readers here: go, py)."
	lrSaySH     = "rashomon: no reader here for .sh files, so edits to them carry no unit, caller or conflict facts this session (readers here: go, py)."
	lrSayPy     = "rashomon: the index here holds no py records, so edits to .py files carry no unit, caller or conflict facts until rashomon graph index runs."
	lrSayUnread = "rashomon: <file> was not read (<reason>), so edits to <ext> files carry no unit, caller or conflict facts this session."
)

// lrUnread is lrSayUnread for file and ext as a pattern, its one group the
// reason, whose words these tests leave to the code that prints the line.
func lrUnread(file, ext string) *regexp.Regexp {
	q := regexp.QuoteMeta(strings.NewReplacer("<file>", file, "<ext>", ext).Replace(lrSayUnread))
	return regexp.MustCompile(strings.Replace(q, "<reason>", `([^()]+)`, 1))
}

// lrOnce fails unless said holds want exactly once.
func lrOnce(t *testing.T, said, want string) {
	t.Helper()
	if n := strings.Count(said, want); n != 1 {
		t.Errorf("the session printed %q %d times, want once; it printed %q", want, n, said)
	}
}

// lrReason is the reason of the one line re matches in said, failing unless
// there is exactly one and its reason is not blank.
func lrReason(t *testing.T, said string, re *regexp.Regexp) string {
	t.Helper()
	m := re.FindAllStringSubmatch(said, -1)
	if len(m) != 1 || strings.TrimSpace(m[0][1]) == "" {
		t.Errorf("the session printed %d lines matching %s, want one with a reason; it printed %q", len(m), re, said)
		return ""
	}
	return m[0][1]
}

// lrGroup is the attributes of the overlay group of kind for file in the
// session graph log, false when there is none.
func lrGroup(t *testing.T, log string, file graph.ID, kind string) (map[string]string, bool) {
	t.Helper()
	for _, l := range strings.Split(strings.TrimSpace(log), "\n") {
		var r struct {
			Node *struct {
				Kind  string
				Attrs map[string]string
			}
		}
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("a session graph log line is not JSON: %v: %q", err, l)
		}
		if r.Node != nil && r.Node.Kind == "overlay_group" && r.Node.Attrs["file"] == string(file) && r.Node.Attrs["kind"] == kind {
			return r.Node.Attrs, true
		}
	}
	return nil, false
}

// TestLANGREG1_AnUnclaimedKindSaysSoOnce: in the indexed py-unit
// project, an edit of a file that no reader here claims and the kinds table
// does not list says so, once in the session for its extension, naming the
// readers of this build's registry: two .ts files edited three times print
// the .ts line once, and a .sh file prints a line of its own. Base: no line.
// Break: key the line by the file, and the second .ts file prints it again;
// print it once per session, and the .sh file prints none.
func TestLANGREG1_AnUnclaimedKindSaysSoOnce(t *testing.T) {
	e := newSteerEnv(t)
	dir, _ := e.lrProject()
	ai1Write(t, dir, "web/app.ts", "export const a = 1;\n")
	ai1Write(t, dir, "web/util.ts", "export const u = 2;\n")
	ai1Write(t, dir, "run.sh", "echo run\n")
	var said strings.Builder
	for _, c := range []struct{ id, file, old string }{
		{"toolu_t1", "web/app.ts", "export"}, {"toolu_t2", "web/util.ts", "export"},
		{"toolu_t3", "web/app.ts", "const"}, {"toolu_t4", "run.sh", "echo"},
	} {
		said.WriteString(e.lrSaid(dir, c.id, c.file, c.old))
	}
	lrOnce(t, said.String(), lrSayTS)
	lrOnce(t, said.String(), lrSaySH)
}

// TestLANGREG1_TheKindLineIsForThePersonAlone: the unclaimed kind's line is
// the post's whole output, one JSON object whose only key is systemMessage,
// equal to the line; the PreToolUse prints nothing (lrEdit), and no
// hookSpecificOutput carries it to the model. Base: the post prints 0 bytes.
// Break: say the line as additionalContext.
func TestLANGREG1_TheKindLineIsForThePersonAlone(t *testing.T) {
	e := newSteerEnv(t)
	dir, _ := e.lrProject()
	ai1Write(t, dir, "web/app.ts", "export const a = 1;\n")
	post := e.lrEdit(dir, "toolu_c1", "web/app.ts", "export")
	var out map[string]any
	if err := json.Unmarshal([]byte(post), &out); err != nil {
		t.Fatalf("the post printed %q, not one JSON object: %v", post, err)
	}
	if msg, _ := out["systemMessage"].(string); msg != lrSayTS || len(out) != 1 {
		t.Errorf("the post printed %q; want one object whose only key is systemMessage, equal to %q", post, lrSayTS)
	}
}

// TestLANGREG1_APythonEditWritesAnOverlayGroup: an edit of a .py
// file of the indexed py unit writes a landed overlay group for the file in
// the session graph log, carrying pylite's set of predicates, and prints
// nothing. Base: the hook reads a .py file through no resolver, so no group.
// Break: carry the Go resolver's set for every language, and the group's
// set is Go's.
func TestLANGREG1_APythonEditWritesAnOverlayGroup(t *testing.T) {
	e := newSteerEnv(t)
	dir, hdr := e.lrProject()
	if _, ok := hdr.Producers[pylite.Lang]; !ok {
		t.Fatalf("premise: the index's producers are %v, want %s among them", hdr.Producers, pylite.Lang)
	}
	if got := e.lrEdit(dir, "toolu_p1", "inv/core/calc.py", "sum"); got != "" {
		t.Errorf("the py edit printed %q, want 0 bytes", got)
	}
	log, ok := e.lrLog(hdr)
	if !ok {
		t.Fatal("the py edit kept no session graph log")
	}
	g, ok := lrGroup(t, log, graph.FileID("inv/core/calc.py"), "landed")
	if !ok {
		t.Fatalf("the session graph log holds no landed group for inv/core/calc.py:\n%s", log)
	}
	if want := (pylite.Resolver{}).OverlayCarried().String(); g["carried"] != want {
		t.Errorf("the py group carries %q, want pylite's %q", g["carried"], want)
	}
}

// lrGoOnly is a configuration that declares the py-unit fixture's Go unit
// and not its Python one.
const lrGoOnly = `{"units": [{"path": ".", "lang": "go"}]}` + "\n"

// TestLANGREG1_AnUnindexedLanguageSaysSoOnce: a .py edit where the index
// holds no py records says so, once in the session for the language, and
// does nothing more: no line that the configuration was not read, no
// overlay group for the file and no unit-brief miss. The project is indexed
// while its configuration declares the Go unit only; then the py unit is
// declared (two .py files are edited), or the configuration is removed,
// which is zero py units and so this line, not the unreadable-configuration
// one. Base: no line. Breaks: key the line by the file, and the second .py
// file prints it again; say that a missing configuration was not read, and
// the case with none prints that line too; say the line and go on to build
// the overlay, and the log holds the file's group; leave the edit's gate
// open, and the post counts a unit-brief miss for the py unit.
func TestLANGREG1_AnUnindexedLanguageSaysSoOnce(t *testing.T) {
	for _, c := range []struct {
		name  string
		edits [][3]string
	}{
		{"declared after the index", [][3]string{{"toolu_u1", "inv/core/calc.py", "sum"}, {"toolu_u2", "inv/app/main.py", "run"}}},
		{"no configuration", [][3]string{{"toolu_u3", "inv/core/calc.py", "sum"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			files := e.lrFiles()
			full := files[index.ConfigFile]
			files[index.ConfigFile] = lrGoOnly
			dir, hdr := e.lrIndexed(files)
			_, py := hdr.Producers[pylite.Lang]
			if _, gol := hdr.Producers["go"]; py || !gol {
				t.Fatalf("premise: the index's producers are %v, want go and not %s", hdr.Producers, pylite.Lang)
			}
			cfg := filepath.Join(dir, filepath.FromSlash(index.ConfigFile))
			var err error
			if c.name == "no configuration" {
				err = os.Remove(cfg)
			} else {
				err = os.WriteFile(cfg, []byte(full), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			var said strings.Builder
			for _, ed := range c.edits {
				said.WriteString(e.lrSaid(dir, ed[0], ed[1], ed[2]))
			}
			lrOnce(t, said.String(), lrSayPy)
			if m := lrUnread(index.ConfigFile, ".py").FindAllString(said.String(), -1); len(m) != 0 {
				t.Errorf("the session also said the configuration was not read: %q", m)
			}
			if got := e.lrMisses(); len(got) != 0 {
				t.Errorf("an edit of a py file the index holds no records of counted a unit-brief miss: %q", got)
			}
			if log, ok := e.lrLog(hdr); ok {
				for _, ed := range c.edits {
					for _, kind := range []string{"declared", "landed"} {
						if _, has := lrGroup(t, log, graph.FileID(ed[1]), kind); has {
							t.Errorf("the session graph log holds a %s group for %s:\n%s", kind, ed[1], log)
						}
					}
				}
			}
		})
	}
}

// TestLANGREG1_AnUnreadableConfigSaysWhy: when the configuration
// turns unreadable after the index, a .py edit says the file was not read
// and why, once in the session for the file: a configuration over
// index.MaxConfigBytes and one that is not JSON, each in a session of its
// own, print one line each with two different reasons. The FIFO cases, the
// configuration and go.mod, are TestLANGREG1_AFIFOConfigSaysWhy (unix).
// Base: no line. Break: one reason for every refusal, and the two match.
func TestLANGREG1_AnUnreadableConfigSaysWhy(t *testing.T) {
	re := lrUnread(index.ConfigFile, ".py")
	reasons := map[string]string{}
	for _, c := range []struct{ name, body string }{
		{"over the cap", ""},
		{"not JSON", "units: [inv]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, _ := e.lrProject()
			body := c.body
			if body == "" {
				full := e.lrFiles()[index.ConfigFile]
				body = full + strings.Repeat(" ", index.MaxConfigBytes+1-len(full))
			}
			ai1Write(t, dir, index.ConfigFile, body)
			var said strings.Builder
			said.WriteString(e.lrSaid(dir, "toolu_r1", "inv/core/calc.py", "sum"))
			said.WriteString(e.lrSaid(dir, "toolu_r2", "inv/app/main.py", "run"))
			reasons[c.name] = lrReason(t, said.String(), re)
		})
	}
	if a, b := reasons["over the cap"], reasons["not JSON"]; a != "" && a == b {
		t.Errorf("a configuration over the cap and one that is not JSON give the same reason %q", a)
	}
}
