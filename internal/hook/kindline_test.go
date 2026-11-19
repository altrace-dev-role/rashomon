package hook

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// kindLineOf is the unclaimed kind's line for key in this build's registry,
// spelled out, so a change to the words fails here and not only in an
// acceptance run.
func kindLineOf(key string) string {
	return "rashomon: no reader here for " + key + " files, so edits to them carry no unit, caller or conflict facts this session (readers here: go, py)."
}

// kindOtherLine is the line naming no kind, spelled out as kindLineOf is.
const kindOtherLine = "rashomon: no reader here for some other kinds of file, so edits to them carry no unit, caller or conflict facts this session (readers here: go, py)."

// kindPost is kindEdit's steer bytes.
func (f *ovFix) kindPost(session, id, rel string) []byte {
	f.t.Helper()
	_, b := f.kindEdit(session, id, rel)
	return b
}

// kindEdit is an Edit of rel under the fixture's repository in session, as
// the hook runs it: the PreToolUse, then the PostToolUse with the same id,
// its Close and Flush, then the steer step's bytes (cmd/rashomon/post.go),
// returned with the post. A rel holding a control byte is never created;
// the gate reads no file.
func (f *ovFix) kindEdit(session, id, rel string) (*Post, []byte) {
	f.t.Helper()
	p := f.kindFlushed(session, id, rel, "")
	return p, p.Steer(stEnv(0))
}

// kindFlushed is kindEdit's post before the steer step, its payloads
// carrying prompt as their prompt id where prompt is not empty.
func (f *ovFix) kindFlushed(session, id, rel, prompt string) *Post {
	f.t.Helper()
	file := filepath.Join(f.repo, filepath.FromSlash(rel))
	if strings.IndexFunc(rel, unicode.IsControl) < 0 {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	input := map[string]any{"file_path": file, "old_string": "x", "new_string": "y"}
	payload := func(event string) string {
		p := map[string]any{"session_id": session, "tool_use_id": id, "tool_name": "Edit", "hook_event_name": event,
			"cwd": f.repo, "tool_input": input}
		if event == "PostToolUse" {
			p["tool_response"] = map[string]any{"stdout": "ok"}
		}
		if prompt != "" {
			p["prompt_id"] = prompt
		}
		b, err := json.Marshal(p)
		if err != nil {
			f.t.Fatal(err)
		}
		return string(b)
	}
	h := New(f.st, time.Now)
	h.Close(&Signals{}, h.Capture(strings.NewReader(payload("PreToolUse"))))
	p := NewPost(f.st, time.Now)
	p.Close(&Signals{}, p.Capture(strings.NewReader(payload("PostToolUse"))))
	if err := p.Flush(); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// kindSaid is the system message b carries, failing unless b is one JSON
// object whose only key is systemMessage: the line is for the person, and
// nothing of it reaches the model.
func kindSaid(t *testing.T, b []byte) string {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("the post printed %q, not one JSON object: %v", b, err)
	}
	msg, ok := out["systemMessage"].(string)
	if !ok || len(out) != 1 {
		t.Fatalf("the post printed %q; want one object whose only key is systemMessage", b)
	}
	return msg
}

// kindFake is a reader that names its language and claims nothing; only
// Lang is called on it (Registry.Langs).
type kindFake struct {
	lang.Resolver
	l string
}

func (k kindFake) Lang() string { return k.l }

// TestLANGREG1_AnUnclaimedKindIsSaidOncePerKindUpToSixteen: each of sixteen
// kinds no reader claims says its own line once, as a system message alone;
// the seventeenth says the line naming no kind, once; an eighteenth, named or
// not, and a repeat say nothing. Break: take no marker (every edit repeats),
// or count no limit (the seventeenth names itself).
func TestLANGREG1_AnUnclaimedKindIsSaidOncePerKindUpToSixteen(t *testing.T) {
	f := newOvFix(t, "1\n")
	for i := 0; i < 16; i++ {
		key := ".k" + strconv.Itoa(i)
		b := f.kindPost(ovSess, "toolu_k"+strconv.Itoa(i), "web/a"+key)
		if got := kindSaid(t, b); got != kindLineOf(key) {
			t.Fatalf("kind %s said %q; want %q", key, got, kindLineOf(key))
		}
	}
	if got := kindLineOf(".k0"); got != "rashomon: no reader here for .k0 files, so edits to them carry no unit, caller or conflict facts this session (readers here: go, py)." {
		t.Fatalf("premise: the .k0 line reads %q", got)
	}
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_k16", "web/a.k16")); got != kindOtherLine {
		t.Fatalf("the seventeenth kind said %q; want %q", got, kindOtherLine)
	}
	if b := f.kindPost(ovSess, "toolu_k17", "web/a.k17"); b != nil {
		t.Errorf("the eighteenth kind printed %q; want nothing, the line naming no kind was said", b)
	}
	if b := f.kindPost(ovSess, "toolu_kc", "web/a.t\x01s"); b != nil {
		t.Errorf("an unprintable kind past the limit printed %q; want nothing, the line naming no kind was said", b)
	}
	if b := f.kindPost(ovSess, "toolu_k0b", "web/b.k0"); b != nil {
		t.Errorf("a second edit of .k0 printed %q; want nothing", b)
	}
}

// TestLANGREG1_TheLineNamingNoKindIsNotOneOfTheSixteen: the line naming no
// kind keeps a marker the limit does not count, so after an unprintable key
// takes it, sixteen named kinds still say their own lines; a seventeenth
// then says nothing, the line naming no kind being once per session.
// Break: count notice-other-kinds among the kind markers (the sixteenth
// named kind is silenced).
func TestLANGREG1_TheLineNamingNoKindIsNotOneOfTheSixteen(t *testing.T) {
	f := newOvFix(t, "1\n")
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_c", "web/a.t\x01s")); got != kindOtherLine {
		t.Fatalf("an unprintable key said %q; want %q", got, kindOtherLine)
	}
	for i := 0; i < 16; i++ {
		key := ".k" + strconv.Itoa(i)
		if got := kindSaid(t, f.kindPost(ovSess, "toolu_k"+strconv.Itoa(i), "web/a"+key)); got != kindLineOf(key) {
			t.Fatalf("named kind %d of 16 after the line naming no kind said %q; want %q", i+1, got, kindLineOf(key))
		}
	}
	if b := f.kindPost(ovSess, "toolu_k16", "web/a.k16"); b != nil {
		t.Errorf("a seventeenth kind printed %q; want nothing, the line naming no kind was said first", b)
	}
}

// TestLANGREG1_AnUnprintableKindTakesTheLineNamingNoKind: a key the line
// cannot print (a control byte, or more than sixteen bytes) says the line
// naming no kind, once, and none of the key's bytes; a printable kind after
// it still says its own line. Break: print the key raw, or let the line
// naming no kind silence the named kinds after it.
func TestLANGREG1_AnUnprintableKindTakesTheLineNamingNoKind(t *testing.T) {
	f := newOvFix(t, "1\n")
	b := f.kindPost(ovSess, "toolu_c", "web/a.t\x01s")
	if got := kindSaid(t, b); got != kindOtherLine {
		t.Fatalf("a key with a control byte said %q; want %q", got, kindOtherLine)
	}
	if bytes.Contains(b, []byte{1}) || bytes.Contains(b, []byte(`\u0001`)) {
		t.Errorf("the post printed %q, which carries the key's control byte", b)
	}
	if b := f.kindPost(ovSess, "toolu_l", "web/a.abcdefghijklmnop"); b != nil {
		t.Errorf("a 17-byte key printed %q after the line naming no kind was said; want nothing", b)
	}
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_rs", "src/a.rs")); got != kindLineOf(".rs") {
		t.Errorf("a printable kind after the line naming no kind said %q; want %q", got, kindLineOf(".rs"))
	}
}

// TestLANGREG1_TheKindLineFollowsTheOverlayGate: with no session-content
// marker the overlay writer keeps nothing and no line prints; with it, the
// same edit says the line. Break: print the line outside the writer's gate.
func TestLANGREG1_TheKindLineFollowsTheOverlayGate(t *testing.T) {
	off := newOvFix(t, "")
	if b := off.kindPost(ovSess, "toolu_off", "web/a.ts"); b != nil {
		t.Errorf("a session with no content marker printed %q; want nothing", b)
	}
	on := newOvFix(t, "1\n")
	if got := kindSaid(t, on.kindPost(ovSess, "toolu_on", "web/a.ts")); got != kindLineOf(".ts") {
		t.Errorf("a session with the marker said %q; want %q", got, kindLineOf(".ts"))
	}
}

// TestLANGREG1_EachRowOfTheKindsTableIsSilent: the acceptance control
// TestLANGREG1_ANonSourceKindIsSilent over every row of the kinds table,
// read from the table, in this build's registry. An edit of a path of a
// row's kind prints nothing and is no landed edit, so the unit-brief step
// counts no miss for it, and after every row no session log exists. The
// gate asks the registry before the table, so a reader that claims a row's
// kind fails here. Each row has a session of its own, so the sixteen-kind
// limit cannot hide a row that speaks, and there web/a.ts then says its
// line, so the session is one that would speak. Break: say the unclaimed
// line before asking the table, or let a wired reader claim a row's kind.
func TestLANGREG1_EachRowOfTheKindsTableIsSilent(t *testing.T) {
	f := newOvFix(t, "1\n")
	for i, row := range lang.NonSourceKinds() {
		rel := "docs/" + row
		if strings.HasPrefix(row, ".") {
			rel = "docs/a" + row
		}
		s := "sess-row-" + strconv.Itoa(i)
		p, b := f.kindEdit(s, "toolu_row_"+strconv.Itoa(i), rel)
		if b != nil {
			t.Errorf("an edit of %s printed %q; want nothing", rel, b)
		}
		if _, ok := p.LandedGoEdit(); ok {
			t.Errorf("an edit of %s is a landed edit; want none, so no unit-brief miss", rel)
		}
		if got := kindSaid(t, f.kindPost(s, "toolu_ts_"+strconv.Itoa(i), "web/a.ts")); got != kindLineOf(".ts") {
			t.Errorf("session %s then said %q for web/a.ts; want %q", s, got, kindLineOf(".ts"))
		}
	}
	if _, _, exists := f.read(); exists {
		t.Error("an edit of a kinds-table row made a session log; want none")
	}
}

// TestLANGREG1_EachSessionHearsTheKindLine: the markers are the session's,
// so a second session says the kind's line and the line naming no kind
// again. Break: keep the markers store-wide.
func TestLANGREG1_EachSessionHearsTheKindLine(t *testing.T) {
	f := newOvFix(t, "1\n")
	for _, s := range []string{ovSess, "sess-ov-2"} {
		if got := kindSaid(t, f.kindPost(s, "toolu_ts_"+s, "web/a.ts")); got != kindLineOf(".ts") {
			t.Errorf("session %s said %q for .ts; want %q", s, got, kindLineOf(".ts"))
		}
		if got := kindSaid(t, f.kindPost(s, "toolu_ctl_"+s, "web/a.t\x01s")); got != kindOtherLine {
			t.Errorf("session %s said %q for a control-byte key; want %q", s, got, kindOtherLine)
		}
	}
}

// TestLANGREG1_AKindLineThatDoesNotPrintIsGivenBack: when the line does not
// marshal, nothing prints and the kind's marker is given back, so the
// kind's next edit says it. The language name "go\xff" is not UTF-8, which
// MarshalFor refuses. Break: keep the marker on a failed marshal.
func TestLANGREG1_AKindLineThatDoesNotPrintIsGivenBack(t *testing.T) {
	st := mustOpen(t)
	if err := os.MkdirAll(st.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	var reg lang.Registry
	reg.Register(kindFake{l: "go\xff"})
	p := &Post{st: st, sessionID: "s1", event: "PostToolUse", gate: keptGate{rel: "web/a.ts", reg: &reg, unclaimed: true}}
	if b := p.kindLineAlone(steer.Client{}); b != nil {
		t.Fatalf("a line that is not UTF-8 printed %q; want nothing", b)
	}
	if got, want := st.TakeKindNotice("s1", ".ts"), store.KindNoticeName(".ts"); got != want {
		t.Errorf("after the failed print, the .ts take returned %q; want %q, the marker given back", got, want)
	}
}

// TestLANGREG1_TheKindLineNamesTheRegistrysReaders: the readers the line
// names are the registry's, in its order, never a constant.
// Break: name "go, py" as a literal.
func TestLANGREG1_TheKindLineNamesTheRegistrysReaders(t *testing.T) {
	reg := index.Wire()
	reg.Register(kindFake{l: "rb"})
	if got, want := noReaderLine(".x files", reg), "(readers here: go, py, rb)."; !strings.HasSuffix(got, want) {
		t.Errorf("the line reads %q; want it to end %q", got, want)
	}
}

// TestLANGREG1_TheKindLineIsALineOfItsOwn: after a message the line follows
// on a line of its own; alone, it is the whole message; no line leaves the
// message as it was.
func TestLANGREG1_TheKindLineIsALineOfItsOwn(t *testing.T) {
	for _, c := range []struct{ msg, line, want string }{
		{"mirror", "kind", "mirror\nkind"},
		{"", "kind", "kind"},
		{"mirror", "", "mirror"},
		{"", "", ""},
	} {
		if got := withKindLine(c.msg, c.line); got != c.want {
			t.Errorf("withKindLine(%q, %q) = %q; want %q", c.msg, c.line, got, c.want)
		}
	}
}

// TestLANGREG1_APrintableKindIsShortASCII: the key prints only as 1 to 16
// bytes of ASCII letters, digits, dot, underscore, hyphen and plus.
func TestLANGREG1_APrintableKindIsShortASCII(t *testing.T) {
	for key, want := range map[string]bool{
		".ts": true, ".c++": true, "Makefile": true, "CMakeLists.txt": true, ".abcdefghijklmno": true,
		"": false, ".abcdefghijklmnop": false, ".t\x01s": false, ".t s": false, ".t/s": false, ".\xc3\xa9": false,
		`.t"s`: false,
	} {
		if got := printableKind(key); got != want {
			t.Errorf("printableKind(%q) = %v; want %v", key, got, want)
		}
	}
}

// unindexedLineOf is the unindexed language's line for the language l and
// the words what, spelled out as kindLineOf is.
func unindexedLineOf(l, what string) string {
	return "rashomon: the index here holds no " + l + " records, so edits to " + what +
		" files carry no unit, caller or conflict facts until rashomon graph index runs."
}

// TestUnindexedLanguage_IsSaidOncePerSessionAndLanguage: under the fixture's
// index, whose header names no producer, with a configuration that declares
// a py unit, an edit of a .py file says the language's line as a system
// message alone and keeps no overlay group; a second .py file in the session
// says nothing, and a second session says the line again. Break: key the
// marker by the file (the second file repeats), keep it store-wide (the
// second session is silent), or go on to the reader's configuration and
// overlay (a group is kept).
func TestUnindexedLanguage_IsSaidOncePerSessionAndLanguage(t *testing.T) {
	f := newOvFix(t, "1\n")
	cfg := filepath.Join(f.repo, filepath.FromSlash(index.ConfigFile))
	if err := os.WriteFile(cfg, []byte(`{"units": [{"path": "inv", "modules": ["inv"], "lang": "py"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := unindexedLineOf(pylite.Lang, ".py")
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_py1", "inv/a.py")); got != want {
		t.Errorf("the first .py edit said %q; want %q", got, want)
	}
	if b := f.kindPost(ovSess, "toolu_py2", "inv/b.py"); b != nil {
		t.Errorf("a second .py file in the session printed %q; want nothing", b)
	}
	if got := kindSaid(t, f.kindPost("sess-ov-2", "toolu_py3", "inv/a.py")); got != want {
		t.Errorf("a second session said %q; want %q", got, want)
	}
	if groups, _, _ := f.read(); len(groups) != 0 {
		t.Errorf("the unindexed .py edits kept %d overlay groups; want none", len(groups))
	}
}

// TestUnindexedLanguage_AnUnprintableKindNamesTheLanguage: where the edited
// file's kind does not print (printableKind), the line names the language's
// files, and the marker is the language's. Break: print the key raw.
func TestUnindexedLanguage_AnUnprintableKindNamesTheLanguage(t *testing.T) {
	st := mustOpen(t)
	if err := os.MkdirAll(st.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	var reg lang.Registry
	p := &Post{st: st, sessionID: "s1", event: "PostToolUse", gate: keptGate{rel: "a/b.f\x01k", reg: &reg, unindexed: "fake"}}
	line, marker := p.takeKindLine()
	if want := unindexedLineOf("fake", "fake"); line != want {
		t.Errorf("the line reads %q; want %q", line, want)
	}
	if want := store.UnindexedNoticeName("fake"); marker != want {
		t.Errorf("the marker taken is %q; want the language's %q", marker, want)
	}
}

// unreadLineOf is the unread configuration's line for file, the reason's
// words why and the words what, spelled out as kindLineOf is.
func unreadLineOf(file, why, what string) string {
	return "rashomon: " + file + " was not read (" + why + "), so edits to " + what +
		" files carry no unit, caller or conflict facts this session."
}

// TestUnreadConfig_IsSaidOncePerSessionAndFile: under the fixture's index,
// with go.mod a directory, an edit of a .go file says go.mod was not read,
// not being a regular file, as a system message alone, and keeps no
// overlay group; a second .go file in the session says nothing, and a
// second session says the line again. Break: key the marker by the edited
// file (the second file repeats), keep it store-wide (the second session
// is silent), or say the step's error (the line names the path).
func TestUnreadConfig_IsSaidOncePerSessionAndFile(t *testing.T) {
	f := newOvFix(t, "1\n")
	if err := os.Mkdir(filepath.Join(f.repo, gopg.ManifestFile), 0o755); err != nil {
		t.Fatal(err)
	}
	want := unreadLineOf(gopg.ManifestFile, "not a regular file", ".go")
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_gm1", "pkg/a.go")); got != want {
		t.Errorf("the first .go edit said %q; want %q", got, want)
	}
	if b := f.kindPost(ovSess, "toolu_gm2", "pkg/b.go"); b != nil {
		t.Errorf("a second .go file in the session printed %q; want nothing", b)
	}
	if got := kindSaid(t, f.kindPost("sess-ov-2", "toolu_gm3", "pkg/a.go")); got != want {
		t.Errorf("a second session said %q; want %q", got, want)
	}
	if groups, _, _ := f.read(); len(groups) != 0 {
		t.Errorf("the refused .go edits kept %d overlay groups; want none", len(groups))
	}
}

// TestUnreadConfig_EachReasonSaysItsOwnWords: the line for each reason of
// the closed set names the refused file and the reason's own words, five
// different phrases, and a reason past the set reads as a failed read; the
// words are the class alone, never the step's error, whose text names a
// path. The marker is the file's. Where the edited file's kind does not
// print, the line names the files of the reader's language. Break: one
// phrase for two reasons, say the error's text, or print the key raw.
func TestUnreadConfig_EachReasonSaysItsOwnWords(t *testing.T) {
	st := mustOpen(t)
	var reg lang.Registry
	stepErr := errors.New("/r/x.cfg: open /r/x.cfg: permission denied")
	take := func(sess, rel string, r lang.Reason) (string, string) {
		if err := os.MkdirAll(st.RunDir(sess), 0o700); err != nil {
			t.Fatal(err)
		}
		p := &Post{st: st, sessionID: sess, event: "PostToolUse", gate: keptGate{rel: rel, reg: &reg,
			unread: &lang.Refusal{File: "x.cfg", Reason: r, Err: stepErr}, unreadLang: "fake"}}
		return p.takeKindLine()
	}
	phrases := map[string]bool{}
	for i, c := range []struct {
		r   lang.Reason
		why string
	}{
		{lang.ReadFailed, "read failed"}, {lang.NotRegular, "not a regular file"}, {lang.OverCap, "over the size cap"},
		{lang.NotJSON, "not JSON"}, {lang.WrongShape, "JSON of the wrong shape"}, {lang.WrongShape + 1, "read failed"},
	} {
		line, marker := take("s"+strconv.Itoa(i), "a/b.cfgx", c.r)
		if want := unreadLineOf("x.cfg", c.why, ".cfgx"); line != want {
			t.Errorf("reason %d: the line reads %q; want %q", c.r, line, want)
		}
		if marker != store.UnreadNoticeName("x.cfg") {
			t.Errorf("reason %d: the marker taken is %q; want the file's", c.r, marker)
		}
		if strings.Contains(line, "/r/") || strings.Contains(line, "denied") {
			t.Errorf("reason %d: the line %q carries the step's error", c.r, line)
		}
		phrases[reasonWords(c.r)] = true
	}
	if len(phrases) != 5 {
		t.Errorf("the reasons say %d phrases, want 5: %v", len(phrases), phrases)
	}
	if line, _ := take("s-raw", "a/b.f\x01k", lang.NotJSON); line != unreadLineOf("x.cfg", "not JSON", "fake") {
		t.Errorf("an unprintable kind's line reads %q; want the language's files named", line)
	}
}

// goEditOut is a Go edit of the fixture's file as the hook runs it: the
// PreToolUse, the file as the tool left it, the PostToolUse with its Close
// and Flush, then the steer step. It returns the steer step's bytes and the
// session log's bytes, the repository's path in the one and its id in the
// other (the log header's repo) each written as a word of its own: those
// are the only bytes that name the fixture's directory.
func goEditOut(f *ovFix) (steerOut, logged []byte) {
	f.t.Helper()
	in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
	f.pre("toolu_cfg", "Edit", in)
	after := "package pkg\n\nfunc Old() {}\n\nfunc Added() {}\n\nfunc OnDisk() {}\n"
	if err := os.WriteFile(f.file, []byte(after), 0o644); err != nil {
		f.t.Fatal(err)
	}
	p := NewPost(f.st, time.Now)
	p.Close(&Signals{}, p.Capture(strings.NewReader(f.payload("PostToolUse", "toolu_cfg", "Edit", in))))
	if err := p.Flush(); err != nil {
		f.t.Fatal(err)
	}
	steerOut = bytes.ReplaceAll(p.Steer(stEnv(0)), []byte(f.repo), []byte("repo"))
	b, err := os.ReadFile(f.logPath())
	if err != nil {
		f.t.Fatal(err)
	}
	return steerOut, bytes.ReplaceAll(b, []byte(f.rid), []byte("rid"))
}

// TestUnreadConfig_AGoEditIsByteEqualWhilePythonsConfigIsUnread: under the
// fixture's index, a Go edit prints the same bytes and writes the same
// session log, its declared and its landed group, whether the Python
// configuration (index.ConfigFile) is absent or refused for four reasons of
// the closed set: a directory, past the limit, a stray word, units a
// string. Each refusal is real: the wired registry's Claim of a .py file
// under the same root returns it. The configuration is the Python reader's
// alone, and a Claim runs only the claiming reader's step. Break: run every
// reader's step on a Claim (the Go edit is refused for the Python file and
// keeps no landed group), or keep another reader's refusal on the gate
// (the Go edit says the Python file was not read).
func TestUnreadConfig_AGoEditIsByteEqualWhilePythonsConfigIsUnread(t *testing.T) {
	base := newOvFix(t, "1\n")
	if _, ok, err := index.Wire().Claim(base.repo, "inv/a.py"); !ok || err != nil {
		t.Fatalf("premise: with no configuration the .py Claim = %v, %v; want it claimed", ok, err)
	}
	baseSteer, baseLog := goEditOut(base)
	if groups, _, _ := base.read(); len(groups) != 2 {
		t.Fatalf("premise: with no configuration the Go edit kept %d groups; want the declared and the landed", len(groups))
	}
	for _, c := range []struct {
		name  string
		write func(cfg string) error
		want  lang.Reason
	}{
		{"a directory", func(cfg string) error { return os.Mkdir(cfg, 0o755) }, lang.NotRegular},
		{"past the limit", func(cfg string) error {
			return os.WriteFile(cfg, []byte(`{"units":[]}`+strings.Repeat(" ", index.MaxConfigBytes)), 0o644)
		}, lang.OverCap},
		{"a stray word", func(cfg string) error { return os.WriteFile(cfg, []byte("units: [inv]\n"), 0o644) }, lang.NotJSON},
		{"units a string", func(cfg string) error { return os.WriteFile(cfg, []byte(`{"units":"inv"}`), 0o644) }, lang.WrongShape},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newOvFix(t, "1\n")
			if err := c.write(filepath.Join(f.repo, filepath.FromSlash(index.ConfigFile))); err != nil {
				t.Fatal(err)
			}
			_, _, err := index.Wire().Claim(f.repo, "inv/a.py")
			var rf *lang.Refusal
			if !errors.As(err, &rf) || rf.File != index.ConfigFile || rf.Reason != c.want {
				t.Fatalf("premise: the .py Claim's error is %v; want a refusal of %s, reason %d", err, index.ConfigFile, c.want)
			}
			gotSteer, gotLog := goEditOut(f)
			if !bytes.Equal(gotSteer, baseSteer) {
				t.Errorf("the Go edit printed %q; with no configuration it printed %q", gotSteer, baseSteer)
			}
			if !bytes.Equal(gotLog, baseLog) {
				t.Errorf("the Go edit's session log differs from the one with no configuration:\n%s\nwant:\n%s", gotLog, baseLog)
			}
		})
	}
}
