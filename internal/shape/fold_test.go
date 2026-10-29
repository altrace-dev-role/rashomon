package shape

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// TestFoldKeepsRunnerIdentity: the leading cd steps the program search folds
// are the ones the masked parser passes over, read or not, where its reading
// folds the same tokens and none where it does not (agreedFold), so the runner
// after them keeps its kind and its digest, which is the runner's words
// alone: the same digest as the runner typed without the fold. Where the
// runner ran is the cwd digest's to say, not the runner digest's. The class
// and status_masked are unchanged by the fold. Break: exempt only the steps
// leadingCd reads, and every row here but the unchanged ones loses its kind
// and digest; exempt a cd the program search does not fold, and an unchanged
// row gains one.
func TestFoldKeepsRunnerIdentity(t *testing.T) {
	key := []byte("key")
	derive := func(cmd string) Shape {
		t.Helper()
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		return Derive("Bash", raw, key)
	}
	bare := map[string]*string{}
	for _, r := range []string{"go test ./...", "make -j4 test"} {
		if bare[r] = derive(r).RunnerDigest; bare[r] == nil {
			t.Fatalf("%q: no runner digest", r)
		}
	}
	for _, tc := range []struct {
		cmd, class, masked, kind string // masked "" for null, kind "" for null
		runner                   string // the bare runner whose digest it carries, "" for none
	}{
		// A fold leadingCd does not read: a variable, `-`, a quoted word, an
		// option, a prefix, an opening ( or {, and `;`.
		{"cd $REPO && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd /tmp; go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd - && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{`cd "/tmp" && go test ./...`, VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd -P /tmp && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"X=1 cd /tmp && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd /a && cd $B && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"(cd /tmp && go test ./...)", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"{ cd /tmp && go test ./...; }", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"(cd /tmp && go test ./...) | tail", VerbPackage, MaskedTest, RunnerTest, "go test ./..."},
		{`cd "$REPO" && go test ./...`, VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~user/x && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~+/x && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~-/x && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd sub && cd - && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd $(git rev-parse --show-toplevel) && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd /a && (cd b && go test ./...)", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd /a && { cd b && go test ./...; }", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"(cd /x); go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{strings.Repeat("cd $D && ", MaxLeadingCd) + "go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		// A fold leadingCd reads, into the home.
		{"cd ~/x && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~ && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~/x && go test ./... 2>&1 | tail -20", VerbPackage, MaskedTest, RunnerTest, "go test ./..."},
		{"cd ~/x && echo hi && go test ./...", VerbExecute, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~/x && go build ./... && go test ./...", VerbPackage, MaskedNone, RunnerTest, "go test ./..."},
		{"cd ~/x && make -j4 test", VerbTest, MaskedNone, RunnerTest, "make -j4 test"},
		{"cd /tmp && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{strings.Repeat("cd a && ", MaxLeadingCd) + "go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		// pushd folds as cd does, in both readings (version 8): `&&` and
		// `;` alike, its stack forms too, which the location reader refuses.
		{"pushd /b && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd /a && pushd /b && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"pushd /b; go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"pushd +1 && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		// A comment ends at its newline in both readings (version 9), as it
		// does to the shell: a backslash at its end continues nothing, so the
		// cd steps after it fold and the runner is go test.
		{"cd a && # c\\\ncd b && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"# c\\\ncd a && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},
		{"cd a && #\\\n cd b && go test ./...", VerbTest, MaskedNone, RunnerTest, "go test ./..."},

		// Unchanged. A cd the program search does not fold still displaces
		// the runner after it: popd, one whose rest is a comment,
		// `||`, a cd mid-line, and the second run of two. A quoted cd is no
		// cd to either reader and leaves the runner its digest. Past
		// MaxLeadingCd steps none is folded (RES-10). A line the masked
		// parser reads as null (a cd behind a redirection) stays null.
		{"cd /a && popd && go test ./...", VerbExecute, MaskedNone, "", ""},
		{"popd && make test | tail", VerbExecute, MaskedTest, "", ""},
		{"cd /a && popd && ls", VerbExecute, MaskedNone, "", ""},
		{">/dev/null cd /tmp && go test ./...", VerbTest, "", "", ""},
		{"cd /tmp # x && go test ./...", VerbExecute, MaskedNone, "", ""},
		{`"cd" /tmp && go test ./...`, VerbExecute, MaskedNone, RunnerTest, "go test ./..."},
		{"cd /tmp || exit 1; go test ./...", VerbExecute, MaskedNone, "", ""},
		{"go build ./... && cd sub && go test ./...", VerbPackage, MaskedNone, "", ""},
		{"cd /tmp && go test ./... ; cd /a && go test ./...", VerbPackage, MaskedNone, "", ""},
		{strings.Repeat("cd a && ", MaxLeadingCd+1) + "go test ./...", VerbTest, MaskedNone, "", ""},
		{strings.Repeat("cd $D && ", MaxLeadingCd+1) + "go test ./...", VerbTest, MaskedNone, "", ""},
		{strings.Repeat("cd ~/a && ", MaxLeadingCd+1) + "go test ./...", VerbTest, MaskedNone, "", ""},
	} {
		s := derive(tc.cmd)
		masked, kind := "", ""
		if s.StatusMasked != nil {
			masked = *s.StatusMasked
		}
		if s.RunnerKind != nil {
			kind = *s.RunnerKind
		}
		cmd := short(tc.cmd)
		if s.VerbClass != tc.class || masked != tc.masked || kind != tc.kind {
			t.Errorf("%q: class %q, masked %q, kind %q; want %q, %q, %q", cmd, s.VerbClass, masked, kind, tc.class, tc.masked, tc.kind)
		}
		switch {
		case tc.runner == "" && s.RunnerDigest != nil:
			t.Errorf("%q: runner digest set; want none", cmd)
		case tc.runner != "" && (s.RunnerDigest == nil || *s.RunnerDigest != *bare[tc.runner]):
			t.Errorf("%q: runner digest set %v; want %q's", cmd, s.RunnerDigest != nil, tc.runner)
		}
	}
}

// TestAfterVerbDirectoryKeepsItsIdentity: a directory option after the row's
// words is part of the runner's words, so its runner digest is apart from the
// bare row's and from the same row behind `cd sub &&`. Break: leave the
// option out of runnerWords, and `make test -C sub` follows up `make test`
// run in the start directory.
func TestAfterVerbDirectoryKeepsItsIdentity(t *testing.T) {
	for _, tc := range [][3]string{
		{"make test -C sub", "make test", "cd sub && make test"},
		{"go test -C sub ./...", "go test ./...", "cd sub && go test ./..."},
		{"npm test --prefix web", "npm test", "cd web && npm test"},
		{"cargo test --manifest-path sub/Cargo.toml", "cargo test", "cd sub && cargo test"},
	} {
		var d [3]*string
		for i, cmd := range tc {
			if _, d[i] = maskedOf(t, cmd); d[i] == nil {
				t.Fatalf("%q: no runner digest", cmd)
			}
		}
		if *d[0] == *d[1] || *d[0] == *d[2] || *d[1] != *d[2] {
			t.Errorf("%q equals %q: %v, equals %q: %v, and those two equal: %v; want false, false, true",
				tc[0], tc[1], *d[0] == *d[1], tc[2], *d[0] == *d[2], *d[1] == *d[2])
		}
	}
}

// TestLeadingFold: the fold is the index past the separator of the last
// leading cd step the program search passes, over either reading of the line
// (tokenizeList for the masked parser, tokenizeProgram for the program
// search), and 0 where there is none or past MaxLeadingCd steps (RES-10). A
// backslash-newline inside a step is removed before either reading splits
// words, and a # inside a word starts no comment. Break: count a step the
// program search does not pass, or drop the bound, and a row here moves.
func TestLeadingFold(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", maxTokens+10_000))
	for _, tc := range []struct {
		cmd           string
		list, program int // -1 where that reading fails
	}{
		{"cd /t\\\nmp && go test ./...", 3, 3},
		{"cd sub\\\n && go test ./...", 3, 3},
		{"cd \\\n/tmp && go test ./...", 3, 3},
		{"cd a#b && go test ./...", 3, 3},
		{"cd /tmp #x && go test ./...", 0, 0},
		{"cd a #b\ngo test ./...", 0, 0},
		{"cd a && cd b; go test", 6, 6},
		{"(cd /tmp && go test ./...)", 4, 4},
		{"X=1 cd /tmp && go test", 4, 4},
		{"go test", 0, 0},
		{"cd /tmp", 0, 0},
		{strings.Repeat("cd a && ", MaxLeadingCd) + "make test", 3 * MaxLeadingCd, 3 * MaxLeadingCd},
		{strings.Repeat("cd a && ", MaxLeadingCd+1) + "make test", 0, 0},
		// The program search reads to the token bound, uncertain past it; the
		// masked parser skips the here-document's body.
		{"cd sub && python3 - <<'EOF'\n" + long + "\nEOF\nmake test", 3, 3},
	} {
		lt, lerr := tokenizeList(tc.cmd)
		pt, perr := tokenizeProgram(tc.cmd)
		list, program := -1, -1
		if lerr == nil {
			list = leadingFold(lt, false)
		}
		if perr == nil || perr == errTokenBound {
			program = leadingFold(pt, perr != nil)
		}
		if list != tc.list || program != tc.program {
			t.Errorf("%q: leadingFold over the list %d (%v), over the program search %d (%v); want %d, %d",
				short(tc.cmd), list, lerr, program, perr, tc.list, tc.program)
		}
	}
}

// foldApart are lines the two readings folded apart before version 9: a
// comment ending in a backslash, before the leading steps or among them, which
// tokenizeProgram joined the next line to. Both readings now end a comment at
// its newline (lex), and the lines stay as seeds; the one divergence left is a
// here-document, whose body only tokenizeList skips.
var foldApart = []string{
	"cd a && # c\\\ncd b && go test ./...",
	"# c\\\ncd a && go test ./...",
	"cd a && #\\\n cd b && go test ./...",
}

// FuzzLeadingFoldAgrees: the masked parser and the program search read the
// line through two tokenizers, and each calls leadingFold over its own; where
// both read the line without error, the fold the masked parser passes over
// (agreedFold) is the program search's, the same tokens, or none, and never
// longer, so the runner the masked parser passes the fold for is the one the
// program search names. The folds are compared as the tokens they cover, not
// as indices: the two readings tokenize a here-document and a comment apart
// (foldApart). Break: keep the masked parser's own fold where the two
// differ, and a foldApart line folds a cd the program search does not.
func FuzzLeadingFoldAgrees(f *testing.F) {
	for _, s := range shapeSeeds() {
		f.Add(s)
	}
	f.Add("cd /t\\\nmp && go test ./...")
	f.Add("cd a#b && cd c && go test")
	f.Add("X=1 >/dev/null cd $D; (cd b && { cd c && make test; })")
	for _, s := range foldApart {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, cmd string) { checkLeadingFoldAgrees(t, cmd) })
}

func checkLeadingFoldAgrees(t *testing.T, cmd string) {
	t.Helper()
	lt, lerr := tokenizeList(cmd)
	pt, perr := tokenizeProgram(cmd)
	if lerr != nil || perr != nil {
		return
	}
	p := leadingFold(pt, false)
	if l := agreedFold(lt, pt[:p]); l != 0 && !slices.Equal(lt[:l], pt[:p]) {
		t.Fatalf("%q: the masked parser folds %d tokens and the program search %d, apart", cmd, l, p)
	}
}

// TestRubricTEST1_LeadingFoldSeeds runs FuzzLeadingFoldAgrees's property
// over its seeds as a plain test, which is what a mutant names and what runs
// without -fuzz.
func TestRubricTEST1_LeadingFoldSeeds(t *testing.T) {
	for _, s := range append(append(shapeSeeds(), "cd /t\\\nmp && go test ./...", "cd a#b && cd c && go test",
		"X=1 >/dev/null cd $D; (cd b && { cd c && make test; })"), foldApart...) {
		checkLeadingFoldAgrees(t, s)
	}
}

// TestDeriveWithDirsUnread: DeriveWithDirs's fourth result is the
// tokenizer's view of the leading cd steps the program search passes (each
// token's text, a NUL, q for quoted or p, a NUL), for a line whose leading run
// leadingCd does not read; "" where the run is read, past MaxLeadingCd steps
// or not, and where there is no fold. Spacing is not in it and quoting is.
// It is counted as the program search counts, without MaxLeadingCd's bound,
// so a fold of more unread steps than that is still unread. Break: take the
// view through leadingFold, and the last row reads ""; drop the quoting byte,
// and the quoted row reads as the plain one.
func TestDeriveWithDirsUnread(t *testing.T) {
	view := func(words ...string) string {
		var b strings.Builder
		for _, w := range words {
			q, ok := strings.CutPrefix(w, "q:")
			if ok {
				b.WriteString(q + "\x00q\x00")
			} else {
				b.WriteString(w + "\x00p\x00")
			}
		}
		return b.String()
	}
	for _, tc := range []struct{ cmd, want string }{
		{"cd $REPO && go test ./...", view("cd", "$REPO", "&&")},
		{"cd  $REPO  &&  go test ./...", view("cd", "$REPO", "&&")},
		{`cd "$REPO" && go test ./...`, view("cd", "q:$REPO", "&&")},
		{"cd ${REPO} && go test ./...", view("cd", "${REPO}", "&&")},
		{"cd /tmp; go test ./...", view("cd", "/tmp", ";")},
		{"X=1 cd /tmp && go test", view("X=1", "cd", "/tmp", "&&")},
		{"(cd /tmp && go test ./...)", view("(", "cd", "/tmp", "&&")},
		{"cd /x && cd - && go test ./...", view("cd", "/x", "&&", "cd", "-", "&&")},
		{"pushd /b; go test ./...", view("pushd", "/b", ";")},
		{"cd /a && pushd +1 && go test ./...", view("cd", "/a", "&&", "pushd", "+1", "&&")},
		{"cd && go test ./...", view("cd", "&&")},
		{"cd ~user/x && go test ./...", view("cd", "~user/x", "&&")},

		{"cd /tmp && go test ./...", ""},
		{"cd ~/x && go test ./...", ""},
		{"cd /a && ls", ""},
		{"go test ./...", ""},
		{"pushd /b && go test ./...", ""},
		{"cd /a && pushd /b && go test ./...", ""},
		{"cd /repo\r && ls", ""},
		{"cd $D\r && ls", ""},
		{strings.Repeat("cd a && ", MaxLeadingCd+1) + "go test ./...", ""},
		{strings.Repeat("cd $D && ", MaxLeadingCd+1) + "go test ./...", strings.Repeat(view("cd", "$D", "&&"), MaxLeadingCd+1)},
	} {
		_, _, dirs, unread := DeriveWithDirs("Bash", bashInput(t, tc.cmd), []byte("key"))
		if unread != tc.want || unread != "" && len(dirs) != 0 {
			t.Errorf("%q: unread %q with %d directories; want %q", short(tc.cmd), short(unread), len(dirs), short(tc.want))
		}
	}
}

// short is a line cut to fit a test message.
func short(s string) string {
	if len(s) > 60 {
		return s[:40] + "..." + s[len(s)-15:]
	}
	return s
}
