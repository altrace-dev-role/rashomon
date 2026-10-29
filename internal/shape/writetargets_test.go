package shape

import (
	"encoding/json"
	"slices"
	"testing"
)

// wtRow is one WrittenPaths row: a Bash line, the targets it names in line
// order (nil for none), and whether the list is complete.
type wtRow struct {
	cmd      string
	targets  []string
	complete bool
}

func checkWT(t *testing.T, rows []wtRow) {
	t.Helper()
	for _, r := range rows {
		got, complete := WrittenPaths("Bash", bashInput(t, r.cmd))
		if !slices.Equal(got, r.targets) || (got == nil) != (r.targets == nil) || complete != r.complete {
			t.Errorf("%q: %q complete %t, want %q complete %t", r.cmd, got, complete, r.targets, r.complete)
		}
	}
}

// TestWT1_RedirectionOperand: a redirection to a file names the word after its
// operator, written in any of the forms the shell writes a file by, and a
// descriptor duplication or an input names nothing. A word the shell expands
// names no one file and leaves the list incomplete. The program's own writes
// are a source of their own: cat's are none, echo's and make's are not known
// (ruling f, 1.2w Gate A), so their redirect lines name the file and stay
// incomplete. Break: name the word after `<`, or count a stage with a named
// redirect as complete whatever its program.
func TestWT1_RedirectionOperand(t *testing.T) {
	checkWT(t, []wtRow{
		{"cat a.txt > b.log", []string{"b.log"}, true},
		{"cat a.txt >> b.log", []string{"b.log"}, true},
		{"cat a.txt >| b.log", []string{"b.log"}, true},
		{"cat a.txt &> b.log", []string{"b.log"}, true},
		{"cat a.txt 2> err.log", []string{"err.log"}, true},
		{"cat a.txt >b.log", []string{"b.log"}, true},
		{"cat <> a.log", []string{"a.log"}, true},
		{"cat a.txt >&2", nil, true},
		{"cat < a.txt", nil, true},
		// The same two beside a file redirect, so the walk reaches them.
		{"cat < a.txt > b.log", []string{"b.log"}, true},
		{"cat a.txt > b.log 2>&1", []string{"b.log"}, true},
		{"cat a.txt > /dev/null", nil, true},
		{"cat a.txt > $out", nil, false},
		{"cat a.txt >> $(date).log", nil, false},
		{"echo x > a.log", []string{"a.log"}, false},
		{"make > build.log", []string{"build.log"}, false},
	})
}

// TestWT2_InPlaceEditorOperands: an editor its stage arms names its files, one
// row per inPlaceEditors key: sed's after the script, whether -e, -f or the
// first operand gives it; perl's after -e or the program file; a formatter's
// every operand but its arming word. go fmt and cargo fmt take packages and
// name nothing. Break: report sed's script or perl's program file, or ruff's
// format, as a target.
func TestWT2_InPlaceEditorOperands(t *testing.T) {
	checkWT(t, []wtRow{
		{"sed -i -e s/x/y/ a.go", []string{"a.go"}, true},
		{"sed -i s/x/y/ a.go b.go", []string{"a.go", "b.go"}, true},
		{"sed -i -f fix.sed a.go", []string{"a.go"}, true},
		{"sed -i a.go -e s/x/y/", []string{"a.go"}, true},
		{"perl -i -pe s/x/y/ a.go", []string{"a.go"}, true},
		{"perl -pi -e s/x/y/ a.go b.go", []string{"a.go", "b.go"}, true},
		{"perl -pi fix.pl a.go", []string{"a.go"}, true},
		{"perl -pi -e s/x/y/ a.go -b.go", []string{"a.go", "-b.go"}, true},
		{"gofmt -w a.go b.go", []string{"a.go", "b.go"}, true},
		{"go fmt ./...", nil, false},
		{"cargo fmt", nil, false},
		{"ruff format a.py", []string{"a.py"}, true},
		{"prettier --write a.ts", []string{"a.ts"}, true},
		{"clang-format -i a.cc", []string{"a.cc"}, true},
		{"eslint --fix a.js", []string{"a.js"}, true},
		{"black a.py", []string{"a.py"}, true},
		{"isort a.py", []string{"a.py"}, true},
		{"rustfmt src/main.rs", []string{"src/main.rs"}, true},
	})
}

// TestWT2b_CheckOnlyClears: a check word clears the editor, as inPlaceEdit
// says, and an editor programVerb does not class writes only when armed
// (ruling c'), so its stage may not write: nil and complete, one row per
// checkOnly word. Break: class an unarmed editor's stage by its program.
func TestWT2b_CheckOnlyClears(t *testing.T) {
	checkWT(t, []wtRow{
		{"black --check a.py", nil, true},
		{"isort --check-only a.py", nil, true},
		{"gofmt -w --diff a.go", nil, true},
		{"prettier --write --list-different a.ts", nil, true},
		{"clang-format -i --dry-run a.cc", nil, true},
		{"rustfmt --help", nil, true},
		{"black --version", nil, true},
		{"gofmt -w -h", nil, true},
	})
}

// TestWT2c_BSDSedEmptySuffix: macOS's sed is BSD's, whose -i and -I take the
// next word as the suffix, empty for none: that word is no script and no
// file. Break: read the empty word as sed's script, which leaves the script
// to be named as a file.
func TestWT2c_BSDSedEmptySuffix(t *testing.T) {
	checkWT(t, []wtRow{
		{"sed -i '' s/x/y/ a.go", []string{"a.go"}, true},
		{"sed -I '' s/x/y/ a.go", []string{"a.go"}, true},
		{"sed -i '' -e s/x/y/ a.go b.go", []string{"a.go", "b.go"}, true},
	})
}

// TestWT3_TeeOperands: tee names every operand, wherever it stands in a
// pipeline, and its options are none. A stage before it that may write by its
// program (go) leaves the line incomplete. Break: name -a.
func TestWT3_TeeOperands(t *testing.T) {
	checkWT(t, []wtRow{
		{"cat a | tee a.log", []string{"a.log"}, true},
		{"cat a | tee a.log b.log", []string{"a.log", "b.log"}, true},
		{"cat a | tee -a a.log", []string{"a.log"}, true},
		{"cat a | tee a.log | grep x", []string{"a.log"}, true},
		{"go test ./... | tee test.log", []string{"test.log"}, false},
	})
}

// TestWT4_CopyMoveInstallDestination: cp, mv and install name their
// destination: -t's word, in any of its forms, or the last operand. A
// directory destination names no one file. Break: name every operand, or
// the last one past a -t.
func TestWT4_CopyMoveInstallDestination(t *testing.T) {
	checkWT(t, []wtRow{
		{"cp a.go b.go", []string{"b.go"}, true},
		{"mv a.go b.go", []string{"b.go"}, true},
		{"install -m 755 a b", []string{"b"}, true},
		{"cp -t dir a.go b.go", []string{"dir"}, true},
		{"cp -tdir a.go", []string{"dir"}, true},
		{"cp --target-directory=dir a.go", []string{"dir"}, true},
		{"cp a.go dir/", nil, false},
	})
}

// TestWT5_RemoveMkdirTouchTruncate: rm, mkdir, touch and truncate name every
// operand, and truncate's -s takes its size. Break: drop truncate's -s
// value, so the size reads as a file.
func TestWT5_RemoveMkdirTouchTruncate(t *testing.T) {
	checkWT(t, []wtRow{
		{"rm a.go b.go c.go", []string{"a.go", "b.go", "c.go"}, true},
		{"rm -f a.go", []string{"a.go"}, true},
		{"touch a b", []string{"a", "b"}, true},
		{"mkdir -p a/b", []string{"a/b"}, true},
		{"truncate -s 0 a.log", []string{"a.log"}, true},
		{"truncate -s0 a.log", []string{"a.log"}, true},
		{"rm a.go 2> err.log", []string{"a.go", "err.log"}, true},
		{"rm -rf", nil, false},
	})
}

// TestWT6_ProgramIdentity: the program is the one programToken names, and a
// word after it is never read as another, so a wrapper names nothing and its
// stage, which may write, leaves the line incomplete. A path to a program is
// the program. Break: step over sudo, env or xargs to the program after it.
func TestWT6_ProgramIdentity(t *testing.T) {
	checkWT(t, []wtRow{
		{"git checkout -- a.go", nil, false},
		{"make", nil, false},
		{"npm install", nil, false},
		{`find . -exec rm {} \;`, nil, false},
		{"ls | xargs rm", nil, false},
		{"sudo touch a.go", nil, false},
		{"env X=1 touch a.go", nil, false},
		{"npx prettier --write a.ts", nil, false},
		{"/bin/rm a.go", []string{"a.go"}, true},
		{"X=1 touch a.go", []string{"a.go"}, true},
	})
}

// TestWT7_ExpansionGlobTildeDash: a word that names no one file -- an
// expansion, a glob, a brace, a tilde, `-`, an empty word, a directory --
// is not named and leaves the list incomplete, beside a named one too
// (ruling f); `--` ends the options, so the word after it is a file however
// it starts. A device is no file. Break: name a word holding `$`.
func TestWT7_ExpansionGlobTildeDash(t *testing.T) {
	checkWT(t, []wtRow{
		{"rm $f", nil, false},
		{`rm "$f"`, nil, false},
		{"rm `ls`", nil, false},
		{"rm 'a`b'", nil, false},
		{"rm *.go", nil, false},
		{"rm a?.go", nil, false},
		{"rm [ab].go", nil, false},
		{"touch {a,b}.go", nil, false},
		{"rm ~/a.go", nil, false},
		{"rm -", nil, false},
		{"rm ''", nil, false},
		{"rm -- -f.txt", []string{"-f.txt"}, true},
		{"rm a.go $f", []string{"a.go"}, false},
		{"rm -r .", nil, false},
		{"rm -r ..", nil, false},
		{"rm -r build/", nil, false},
		{"rm -r a/.", nil, false},
		{"gofmt -w ./...", nil, false},
		{"cat a | tee /dev/null", nil, false},
		{"cat a > /dev/stderr", nil, true},
	})
}

// TestWT8_CompleteIffEveryStageNamed: complete says every source on the line
// named a target; each stage is read on its own, at a separator or a
// newline, and the targets are in line order. Break: read the line as one
// stage, so sed's words are touch's operands.
func TestWT8_CompleteIffEveryStageNamed(t *testing.T) {
	checkWT(t, []wtRow{
		{"touch a.go && git add a.go", []string{"a.go"}, false},
		{"touch a.go; sed -i s/x/y/ b.go", []string{"a.go", "b.go"}, true},
		{"touch a.go\nrm b.go", []string{"a.go", "b.go"}, true},
		{"cp a b 2> err.log", []string{"b", "err.log"}, true},
		{"echo $(touch a.go) > b.log", []string{"b.log"}, false},
		{"ls -la", nil, true},
		{"cat a | grep x | head", nil, true},
	})
}

// TestWT9_NonBashAndUnreadable: a tool that is not a shell names nothing and
// is complete; a line the lexer cannot read to its end, one with a control
// byte, and one whose program cannot be named are nil and incomplete.
func TestWT9_NonBashAndUnreadable(t *testing.T) {
	for _, c := range []struct {
		tool  string
		input string
	}{
		{"Edit", `{"file_path":"a.go","old_string":"x","new_string":"y"}`},
		{"Write", `{"file_path":"a.go","content":"x"}`},
		{"BashOutput", `{"bash_id":"b1"}`},
		{"mcp__fs__write", `{"command":"touch a.go"}`},
	} {
		if got, complete := WrittenPaths(c.tool, json.RawMessage(c.input)); got != nil || !complete {
			t.Errorf("%s: %q complete %t, want nil and complete", c.tool, got, complete)
		}
	}
	checkWT(t, []wtRow{
		{"touch 'a.go", nil, false},
		{"touch a.go\r", nil, false},
		{"$(echo rm) a.go", nil, false},
	})
}

// TestWT12_MayWriteNeverReadsAsWritesNothing: no line of the matcher corpus
// or of the write-target corpus that may_write says may write is complete
// with no targets, which is the record's "writes nothing" (verify's
// appendWrites). Break: drop WrittenPaths' mayWrite guard, so a line whose
// curl and output flag stand in different stages reads as writing nothing.
func TestWT12_MayWriteNeverReadsAsWritesNothing(t *testing.T) {
	lines := append(slices.Clone(matcherCorpus), writeTargetCorpus...)
	lines = append(lines, "cat curl | cat -o", "cat gofmt\ncat -w")
	for _, cmd := range lines {
		raw := bashInput(t, cmd)
		got, complete := WrittenPaths("Bash", raw)
		if Derive("Bash", raw, []byte("key")).MayWrite && complete && got == nil {
			t.Errorf("%q: may_write true, and WrittenPaths reads complete with no targets", cmd)
		}
	}
}

// TestWT13_LeadingCdFold: a relative target past the leading `cd DIR &&` run
// is written where the run leads, and recorded there; an absolute one as it
// is. Where the line stands is not on it -- a run LeadingDirectory does not
// read, `~`, a cd later on the line -- a relative target is not named
// (10-27 addition to the 1.2w section). Break: record a relative target as
// written, against the payload's cwd.
func TestWT13_LeadingCdFold(t *testing.T) {
	checkWT(t, []wtRow{
		{"cd sub && go run gen.go > out.go", []string{"sub/out.go"}, false},
		{"cd sub && cat a > out.go", []string{"sub/out.go"}, true},
		{"cd /abs && touch x", []string{"/abs/x"}, true},
		{"cd a && cd b && touch x", []string{"a/b/x"}, true},
		{"cd a && cd /b && touch x", []string{"/b/x"}, true},
		{"cd sub && touch /tmp/x", []string{"/tmp/x"}, true},
		{"cd sub; touch x", nil, false},
		{"cd sub; touch /tmp/x", []string{"/tmp/x"}, true},
		{"cd ~ && touch x", nil, false},
		{"cd $D && touch x", nil, false},
		{"touch a && cd sub && touch b", []string{"a"}, false},
	})
}
