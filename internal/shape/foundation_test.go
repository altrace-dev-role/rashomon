package shape

import (
	"encoding/json"
	"slices"
	"testing"
)

// TestPG1b_FourMoreRunnersAreTestGates: rake, composer, mix and bazel test
// runs are test gates, class test with a runner digest, so the evaluator can
// key a family on them. The shape probe of 2026-10-23 found all four missing.
func TestPG1b_FourMoreRunnersAreTestGates(t *testing.T) {
	for _, cmd := range []string{"rake test", "composer test", "mix test", "bazel test //..."} {
		sh := verbOf(t, cmd)
		if sh.VerbClass != VerbTest || sh.RunnerDigest == nil {
			t.Errorf("%q: class %s, runner digest %v; want test with a digest", cmd, sh.VerbClass, sh.RunnerDigest != nil)
		}
	}
}

// TestRC3b_InPlaceEditorsSetMayWrite: an editor that rewrites files in place
// sets may_write, so a gate that ran before it reads stale after it. A check
// or list run of the same tool does not write and keeps the bit clear. The
// probe of 2026-10-23 found every editor below reading false.
func TestRC3b_InPlaceEditorsSetMayWrite(t *testing.T) {
	for _, cmd := range []string{
		"sed -i 's/a/b/' x.go", "sed -i '' 's/a/b/' x.go", "sed -i.bak 's/a/b/' x.go",
		"perl -pi -e 's/a/b/' x.go", "perl -i -pe 's/a/b/' x.go",
		"gofmt -w .", "go fmt ./...", "cargo fmt", "black .", "ruff format .", "isort .", "rustfmt src/main.rs",
		"npx prettier --write .", "clang-format -i a.cc", "npx eslint --fix .",
		// The in-place letter inside a cluster of short options.
		"sed -ni p a.go", "sed -Ei.bak s/x/y/ a.go", "perl -lpi -e s/x/y/ a.go",
		// BSD sed's -l takes no argument, so the i after it is reached.
		"sed -li s/x/y/ a.go",
		// BSD sed's -I: alone, in a cluster, and with =. clusterHas refuses a
		// word holding =, so only the prefix entry reads the last, and BSD
		// sed edits on it, keeping a.go=.bak as the backup.
		"sed -I '' s/x/y/ a.go", "sed -nI.bak s/x/y/ a.go", "sed -I=.bak s/x/y/ a.go",
		// ACCEPTED GAP: neither sed edits on -l5i (BSD rejects -5, GNU takes
		// 5i as -l's number); the scan does not know -l's number and arms on
		// the i, an over-claim on a spelling not in use.
		"sed -l5i p a.go",
		// -l clears nothing: gofmt's only lists the files, and the -w beside
		// it writes them; the others' -l is a line length, a line-ending
		// switch, or sed's line wrap.
		"gofmt -l -w .", "gofmt -s -w -l .", "perl -l -pi -e s/a/b/ f", "sed -l 5 -i s/a/b/ f",
		"black -l 100 x.py", "isort -l 100 x.py",
	} {
		if sh := verbOf(t, cmd); !sh.MayWrite {
			t.Errorf("%q: may_write false, want true", cmd)
		}
	}
	for _, cmd := range []string{
		"sed 's/a/b/' x.go", "gofmt -l .", "cargo fmt --check", "black --check .", "ruff format --diff .",
		"npx prettier --check .", "sed -n '1,3p' x.go | grep -i foo",
		// A cluster whose option takes the rest of the word before any i, and
		// a formatter asked for its help or version.
		"sed -ne p a.go", "perl -Mstrict -e print a.go", "perl -ne print a.go",
		"black --version", "black --help", "rustfmt -h", "sed -n p a.go | grep -i x",
	} {
		if sh := verbOf(t, cmd); sh.MayWrite {
			t.Errorf("%q: may_write true, want false (a check or a read does not write)", cmd)
		}
	}
}

// runnerKindLines carry a runner digest, each with the kind of the runner it
// digests. The last two have two runners of different kinds, so the kind is
// the digested runner's and not any runner's on the line.
var runnerKindLines = []struct{ cmd, kind string }{
	{"go test ./...", RunnerTest}, {"go test ./... && echo ok", RunnerTest}, {"make test 2>&1 | tail -40", RunnerTest},
	{"go build ./...", RunnerBuild}, {"make release; ls dist", RunnerBuild}, {"cargo build --release | tee log", RunnerBuild},
	{"bazel test //...", RunnerTest}, {"rake test", RunnerTest},
	{"go test ./... && go build ./...", RunnerBuild},      // both reach the line: the last is digested
	{"go build ./... | tail; go test ./...", RunnerBuild}, // the hidden runner is digested over the reaching one
}

// noRunnerDigestLines carry none: no runner, a runner a popd displaced, two
// hidden runners, and a line whose masking is not known.
var noRunnerDigestLines = []string{
	"ls -la", "echo hi | grep h", "cd x && cat y",
	"popd && go test ./...", "make release | tail; make test | tail", "make test | tail; exit ${PIPESTATUS[0]}",
}

func orNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TestPG1c_RunnerKindFollowsRunnerDigest: the record names the list the
// digested runner came from (D9, B27). status_masked is none on a line read
// to its end and verb_class flips under sequencing, so `go test ./... && echo
// ok` is class package, masked none, digest set: without the kind nothing on
// the record says it ran a test runner.
func TestPG1c_RunnerKindFollowsRunnerDigest(t *testing.T) {
	for _, tc := range runnerKindLines {
		t.Run(tc.cmd, func(t *testing.T) {
			sh := verbOf(t, tc.cmd)
			if sh.RunnerDigest == nil || sh.RunnerKind == nil || *sh.RunnerKind != tc.kind {
				t.Fatalf("digest set %t kind %s, want kind %s", sh.RunnerDigest != nil, orNil(sh.RunnerKind), tc.kind)
			}
		})
	}
	for _, cmd := range noRunnerDigestLines {
		t.Run(cmd, func(t *testing.T) {
			sh := verbOf(t, cmd)
			if sh.RunnerDigest != nil || sh.RunnerKind != nil {
				t.Fatalf("no runner digest: digest set %t kind %s", sh.RunnerDigest != nil, orNil(sh.RunnerKind))
			}
		})
	}
}

// TestPG1c_RunnerKindIsSetExactlyWhenRunnerDigestIs: the two are one fact,
// so a reader that finds a digest finds its kind, and a kind never names a
// runner the record does not identify. Every line above, and a call that is
// not a shell.
func TestPG1c_RunnerKindIsSetExactlyWhenRunnerDigestIs(t *testing.T) {
	var shapes []Shape
	for _, tc := range runnerKindLines {
		shapes = append(shapes, verbOf(t, tc.cmd))
	}
	for _, cmd := range noRunnerDigestLines {
		shapes = append(shapes, verbOf(t, cmd))
	}
	shapes = append(shapes, Derive("Read", []byte(`{"file_path":"/x"}`), []byte("key")))
	for i, sh := range shapes {
		if (sh.RunnerKind == nil) != (sh.RunnerDigest == nil) {
			t.Errorf("shape %d: digest set %t, kind %s: one is set without the other", i, sh.RunnerDigest != nil, orNil(sh.RunnerKind))
		}
	}
}

// TestPG1c_RunnerKindsAreTheListsWords: the vocabulary is the two lists'
// names, in this order, for the schema enum test.
func TestPG1c_RunnerKindsAreTheListsWords(t *testing.T) {
	if got, want := RunnerKinds(), []string{"test", "build"}; !slices.Equal(got, want) {
		t.Fatalf("RunnerKinds() = %q, want %q", got, want)
	}
}

// TestS1_EditorWordsAndPerlDigitRun (5.9): each editor's own words decide,
// and perl's -0 record separator, whose digit run may lead a cluster, does
// not hide the in-place letter after it. The first three writes read true
// already when this test was added, since -l left checkOnly; -0777pi and
// -00pi read false (a cluster had to start with a letter).
func TestS1_EditorWordsAndPerlDigitRun(t *testing.T) {
	for _, tc := range []struct {
		cmd   string
		write bool
	}{
		{"sed -l -i.bak s/x/y/ a.go", true}, {"perl -l -pi -e s/x/y/ a.go", true}, {"perl -l -i -pe s/x/y/ a.go", true},
		{"perl -0777pi -e s/x/y/ a.go", true}, {"perl -00pi -e s/x/y/ a.go", true},
		{"gofmt -l .", false}, {"npx prettier -l .", false},
	} {
		if sh := verbOf(t, tc.cmd); sh.MayWrite != tc.write {
			t.Errorf("%q: may_write %t, want %t", tc.cmd, sh.MayWrite, tc.write)
		}
	}
}

// TestS1_WholeCommandCountsGroupsAndComments (5.10): a `)` after the runner
// is the line's only when it closes a `(` opened before the runner, and `; }`
// likewise for `{`; a word starting with `#` ends the command, so a paren
// after it is comment text. A stray paren is a syntax error that runs
// nothing, and the line keeps go's own class, package.
func TestS1_WholeCommandCountsGroupsAndComments(t *testing.T) {
	for _, tc := range []struct{ cmd, class, masked, kind string }{
		{"go test ./... )", VerbPackage, "<nil>", "<nil>"},
		{"(go test ./...", VerbPackage, "<nil>", "<nil>"},
		{"(go test ./...))", VerbPackage, "<nil>", "<nil>"},
		{"{ go test ./...; }", VerbTest, MaskedNone, RunnerTest},
		{"go test ./... # )", VerbTest, MaskedNone, RunnerTest},
		{"go test ./... #)", VerbTest, MaskedNone, RunnerTest},
	} {
		sh := verbOf(t, tc.cmd)
		if got := []string{sh.VerbClass, orNil(sh.StatusMasked), orNil(sh.RunnerKind)}; !slices.Equal(got, []string{tc.class, tc.masked, tc.kind}) {
			t.Errorf("%q: class, masked, kind %q, want %q", tc.cmd, got, []string{tc.class, tc.masked, tc.kind})
		}
	}
}

// TestS1_CommandEndReadersUnmoved: the two other readers of commandEnd,
// directoryChanges and definesFunctions, read these lines as they did before
// wholeCommand counted groups, since the count is wholeCommand's own. A `)`
// after cd is a syntax error the cd step search does not see; a `#` after cd
// makes the rest comment text, and cd is the whole command.
func TestS1_CommandEndReadersUnmoved(t *testing.T) {
	for _, tc := range []struct {
		cmd, class, masked, program string
		steps                       int
	}{
		{"cd /a) && go test ./...", VerbTest, "<nil>", "go", 1},
		{"cd /a # ) && go test ./...", VerbExecute, MaskedNone, "cd", 0},
	} {
		sh := verbOf(t, tc.cmd)
		toks, err := tokenizeProgram(tc.cmd)
		i, ok := programToken(toks, false)
		steps := -1
		if err == nil && ok {
			_, steps, _, _ = directoryChanges(toks, i, false)
		}
		if sh.VerbClass != tc.class || orNil(sh.StatusMasked) != tc.masked || sh.RunnerKind != nil ||
			orNil(sh.Program) != tc.program || steps != tc.steps {
			t.Errorf("%q: class %s masked %s kind %s program %s steps %d, want class %s masked %s kind <nil> program %s steps %d",
				tc.cmd, sh.VerbClass, orNil(sh.StatusMasked), orNil(sh.RunnerKind), orNil(sh.Program), steps,
				tc.class, tc.masked, tc.program, tc.steps)
		}
	}
}

// TestS2_PushdFoldsAsCd (5.12): `pushd DIR &&` is a directory change to every
// reader of a leading step at once: the location reader (leadingCd), the
// program search and the masked parser (directoryChanges, one loop for both).
// The end-to-end row records location /b with the runner's digest and kind, so
// a pushd folded in one reading only, which displaces the runner, fails here.
// popd is never folded, and pushd's stack forms and a `;` step are not read as
// a location (dirs empty, the fold unread). may_write is set where popd is the
// program, which is not classed, and clear where the runner is.
func TestS2_PushdFoldsAsCd(t *testing.T) {
	for _, tc := range []struct {
		cmd, class, program string
		dirs                []string
		unread, digest      bool
	}{
		{"cd /a && pushd /b && go test ./...", VerbTest, "go", []string{"/a", "/b"}, false, true},
		{"pushd /b && go test ./...", VerbTest, "go", []string{"/b"}, false, true},
		{"pushd /b; go test ./...", VerbTest, "go", nil, true, true},
		{"pushd && go test ./...", VerbTest, "go", nil, true, true},
		{"pushd +1 && go test ./...", VerbTest, "go", nil, true, true},
		{"pushd -1 && go test ./...", VerbTest, "go", nil, true, true},
		{"popd && go test ./...", VerbExecute, "popd", nil, false, false},
		{"popd; go test ./...", VerbExecute, "popd", nil, false, false},
		{"popd +1 && go test ./...", VerbExecute, "popd", nil, false, false},
		{"cd /a && popd && go test ./...", VerbExecute, "popd", nil, true, false},
	} {
		raw, _ := json.Marshal(map[string]string{"command": tc.cmd})
		sh, _, dirs, unread := DeriveWithDirs("Bash", raw, []byte("key"))
		kind := "<nil>"
		if tc.digest {
			kind = RunnerTest
		}
		if sh.VerbClass != tc.class || orNil(sh.Program) != tc.program || !slices.Equal(dirs, tc.dirs) ||
			(unread != "") != tc.unread || (sh.RunnerDigest != nil) != tc.digest || orNil(sh.RunnerKind) != kind || sh.MayWrite != (tc.program == "popd") {
			t.Errorf("%q: class %s program %s dirs %q unread %t digest %t kind %s may_write %t; want %s %s %q %t %t %s %t",
				tc.cmd, sh.VerbClass, orNil(sh.Program), dirs, unread != "", sh.RunnerDigest != nil, orNil(sh.RunnerKind), sh.MayWrite,
				tc.class, tc.program, tc.dirs, tc.unread, tc.digest, kind, tc.program == "popd")
		}
	}
}

// TestS3_TK15_CommentEndsAtItsNewline (5.15): the program search reads a
// comment as the shell does, to the end of its line, where a backslash
// continues nothing: `# note \` and then `go test ./...` runs go test, and a
// comment after the runner no longer hides the command on the next line, whose
// status is then the line's. A backslash-newline outside a comment still joins
// the next line to the word before it.
func TestS3_TK15_CommentEndsAtItsNewline(t *testing.T) {
	for _, tc := range []struct {
		cmd, program string
		test         bool
	}{
		{"# note \\\ngo test ./...", "go", true},
		{"echo a \\\ngo test ./...", "echo", false},
		{"go test ./... # c\\\nrm -rf x", "go", false},
	} {
		raw, _ := json.Marshal(map[string]string{"command": tc.cmd})
		s := Derive("Bash", raw, []byte("key"))
		if orNil(s.Program) != tc.program || (s.VerbClass == VerbTest) != tc.test {
			t.Errorf("%q: program %s, class %s; want %s, test %t", tc.cmd, orNil(s.Program), s.VerbClass, tc.program, tc.test)
		}
	}
}

// TestS3_TK15b_BothReadingsFoldAlike (5.15): a line with a comment and no
// here-document is the same tokens to the program search and to the masked
// parser, so both fold the same leading steps, all of them.
func TestS3_TK15b_BothReadingsFoldAlike(t *testing.T) {
	for _, cmd := range append([]string{"# note \\\ncd a && go test ./..."}, foldApart...) {
		lt, lerr := tokenizeList(cmd)
		pt, perr := tokenizeProgram(cmd)
		if lerr != nil || perr != nil || !slices.Equal(lt, pt) {
			t.Errorf("%q: the two readings are apart (%v, %v)", cmd, lerr, perr)
			continue
		}
		if p := leadingFold(pt, false); p == 0 || agreedFold(lt, pt[:p]) != p {
			t.Errorf("%q: the program search folds %d tokens, the masked parser %d", cmd, p, agreedFold(lt, pt[:p]))
		}
	}
}

// TestS3_TK15c_HereDocumentStillRead (5.15's boundary): the program stream
// still does not skip a here-document's body, which only tokenizeList does:
// the comment reading moved, the here-document reading did not.
func TestS3_TK15c_HereDocumentStillRead(t *testing.T) {
	cmd := "cat <<E\n# x\ncd a && go test ./...\nE\nls"
	has := func(toks []token, w string) bool {
		return slices.ContainsFunc(toks, func(t token) bool { return t.text == w })
	}
	lt, _ := tokenizeList(cmd)
	pt, _ := tokenizeProgram(cmd)
	if !has(pt, "go") || has(lt, "go") {
		t.Errorf("%q: the body's go read by the program stream %t, by the list stream %t; want true, false", cmd, has(pt, "go"), has(lt, "go"))
	}
}
