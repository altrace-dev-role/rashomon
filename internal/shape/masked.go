package shape

import (
	"path"
	"strings"
)

// Masked kinds: what shape.status_masked says of a shell line. A closed set,
// like the verb classes, and decided against fixed lists whose words are
// compared and dropped.
const (
	// MaskedNone: the line was read to its end, and no recognised build or
	// test runner on it has its exit status hidden.
	MaskedNone = "none"
	// MaskedTest: a recognised test runner (testCommands) runs on the line,
	// and the line's exit status is not that runner's.
	MaskedTest = "test"
	// MaskedBuild: the same for a recognised build runner (buildCommands),
	// and no test runner's status is hidden.
	MaskedBuild = "build"
)

// MaskedKinds is the vocabulary of status_masked, for the schema enum test.
func MaskedKinds() []string {
	return []string{MaskedNone, MaskedTest, MaskedBuild}
}

// buildCommands are the build and check runners status_masked recognises
// besides testCommands, in the same form: the program, as path.Base names it,
// then the plain words that must follow it. make is any target -- `make
// release`, `make dist`, `make check` and `make validate` are a project's own
// gates, and which of them builds and which checks is the Makefile's to say --
// and so are npm, yarn and pnpm's `run`, mvn and gradle. A runner not listed
// is not recognised, and its hidden status is not reported: the under-claim.
// The arguments that make a runner do something other than run (notARun:
// `make -n`, `go build -n`, --help, --version) refuse it as they refuse a
// test run.
var buildCommands = [][]string{
	{"make"}, {"gmake"},
	{"go", "build"}, {"go", "vet"}, {"cargo", "build"}, {"cargo", "check"}, {"cargo", "clippy"},
	{"npm", "run"}, {"yarn", "run"}, {"yarn", "build"}, {"pnpm", "run"}, {"pnpm", "build"},
	{"tsc"}, {"npx", "tsc"}, {"mvn"}, {"mvnw"}, {"gradle"}, {"gradlew"},
	{"dotnet", "build"}, {"cmake", "--build"}, {"ninja"}, {"bazel", "build"}, {"bazel", "test"},
}

// command is one command of a line's list, as listCommands found it.
type command struct {
	// prog is the index of the program's token; the command's own words end
	// before end.
	prog, end int
	// op is what ends the command: "|", "&&", "||", ";" or "&", "\n" for a
	// newline, and "" for the end of the line.
	op string
	// grouped: the command opens a group, ( or {, before its program.
	grouped bool
}

// listCommands walks a line's whole list -- every command, past each
// separator and each newline -- and names each command's program, by the
// rules programToken and commandBound apply to the line's first. False when
// any command's program or end cannot be told: a reading of what comes after
// a runner that is not certain cannot say whether the runner's status is the
// line's. A comment line, which tokenizeList has already dropped, is
// nothing. A program in a group, `(make test) | tail`, is named; the group's
// close is a word of the command it ends.
func listCommands(toks []token) ([]command, bool) {
	var out []command
	for pos := 0; pos < len(toks); {
		k, ok := programToken(toks[pos:], false)
		if !ok {
			return nil, false
		}
		i := pos + k
		next, sep, ok := commandBound(toks, i, false)
		if !ok {
			return nil, false
		}
		c := command{prog: i, end: next}
		for _, t := range toks[pos:i] {
			if t.meta && t.text == "(" || !t.meta && t.text == "{" && t.quotedAt < 0 {
				c.grouped = true
			}
		}
		switch {
		case sep >= 0:
			c.op, c.end = toks[sep].text, sep
			if c.op == "|" && next < len(toks) && toks[next].meta && toks[next].glued && toks[next].text == "&" {
				// |&, which pipes stderr as well: a pipe.
				next++
			}
		case next < len(toks):
			c.op = "\n"
		}
		out = append(out, c)
		pos = next
	}
	return out, true
}

// statusMasked reports, for a shell line, whether a recognised build or test
// runner on it has an exit status the line does not return, and the keyed
// digest of that runner's own words -- or, when none does and the line ends
// with a runner whose status it is, that runner's (runnerDigest).
//
// The line's exit status is the last command's that ran. A runner's status
// is hidden from it, so that the call recorded ok whether the runner passed
// or failed, when:
//
//   - a pipe follows the runner (`make test 2>&1 | tail -40`): the pipeline's
//     status is its last stage's, unless `set -o pipefail` ran before it on
//     the line;
//   - `;` or a newline follows the runner's pipeline (`go test ./...; echo
//     $?`, `make check; git status`), since what comes next runs either way
//     -- unless it is `exit` alone or `exit $?`, which returns the runner's;
//   - `||` follows it (`npm test || true`), since what comes next runs
//     exactly when the runner failed -- unless it is `exit` (alone, `$?` or
//     1 to 255), which exits failed, or `false`, after which the list goes on
//     failed and the next separator is read the same way.
//
// `&&` hides nothing: a failure stops the list there, and the line exits
// with it. What comes after is read the same way, so `make test && x; y`
// hides make's status at the `;`. A runner sent to the background with `&`
// is not read as hidden: its status is no one's. Nor is a runner after `set
// -e` (or -o errexit): with it a failed runner may stop the line, and when
// it does not -- inside an && or || list -- the rule is not followed here.
// The under-claim.
//
// Nothing is reported for a line that cannot be read to its end
// (tokenizeList, listCommands), or where a runner is followed by a group or
// a compound command (undecided): null, where "none" would claim a reading.
// bash -c '...' is one command whose words are not looked into, so a runner
// inside it is not seen, pipefail or not.
func statusMasked(cmd string, key []byte) (kind, runner *string) {
	if controlByte(cmd) {
		return nil, nil
	}
	toks, err := tokenizeList(cmd)
	if err != nil {
		return nil, nil
	}
	cs, ok := listCommands(toks)
	if !ok {
		return nil, nil
	}
	var (
		pipefail, errexit bool
		hidden            string // the kind of the first runner found hidden
		hiddenAt          = -1   // its program, past any prefix
		lastRunner        = -1   // the last command's runner, when it is one
	)
	for n, c := range cs {
		prog := path.Base(toks[c.prog].text)
		if prog == "set" && toks[c.prog].quotedAt < 0 {
			p, e := setOptions(toks[c.prog+1 : c.end])
			pipefail, errexit = pipefail || p, errexit || e
			continue
		}
		i, name, ok := runnerPrefix(toks, c.prog, prog)
		if !ok {
			continue
		}
		var k string
		switch {
		case runnerOn(toks, i, name, c.end, testCommands):
			k = MaskedTest
		case runnerOn(toks, i, name, c.end, buildCommands):
			k = MaskedBuild
		default:
			continue
		}
		if n == len(cs)-1 {
			lastRunner = i
		}
		if errexit {
			continue
		}
		h, known := hides(toks, cs, n, pipefail)
		if !known {
			return nil, nil
		}
		if !h {
			continue
		}
		if hidden == "" || hidden == MaskedBuild && k == MaskedTest {
			hidden, hiddenAt = k, i
		}
	}
	none := MaskedNone
	switch {
	case hidden != "":
		return &hidden, runnerDigest(toks, hiddenAt, cs, key)
	case lastRunner >= 0:
		return &none, runnerDigest(toks, lastRunner, cs, key)
	}
	return &none, nil
}

// hides reports whether the line's exit status is not that of the runner
// that is command n of cs, by statusMasked's rules. known is false where
// those rules are not followed (undecided).
func hides(toks []token, cs []command, n int, pipefail bool) (hidden, known bool) {
	// Within its pipeline.
	for cs[n].op == "|" {
		if !pipefail {
			return true, true
		}
		if n+1 >= len(cs) {
			return false, true
		}
		n++
	}
	for {
		switch cs[n].op {
		case "&&":
			// Skipped when the runner failed: on to whatever ends the
			// pipeline after it.
			if n = pipelineEnd(cs, n+1); n < 0 {
				return false, true
			}
		case "||":
			if n+1 >= len(cs) {
				return false, true
			}
			next := cs[n+1]
			if undecided(toks, next) {
				return false, false
			}
			if exitsFailed(toks, next, true) {
				return false, true
			}
			if !isFalseCommand(toks, next) {
				return true, true
			}
			n++
			if cs[n].op == "|" {
				// false | x: x's status.
				return true, true
			}
		case ";", "\n":
			if n+1 >= len(cs) {
				return false, true
			}
			if undecided(toks, cs[n+1]) {
				return false, false
			}
			return !exitsFailed(toks, cs[n+1], false), true
		default:
			// The end of the line, or `&`.
			return false, true
		}
	}
}

// undecided reports a command after which, or in which, what becomes of the
// status before it is not followed here: a group, ( ... ) or { ... }, whose
// last command may be an exit; a compound command or a test, which may read
// $? -- `make test; if [ $? -ne 0 ]; then exit 1; fi` exits failed. The
// line is then not read: null, not "none".
func undecided(toks []token, c command) bool {
	return c.grouped || toks[c.prog].quotedAt < 0 && compound[toks[c.prog].text]
}

// compound is the reserved words that begin a compound command, and the test
// commands.
var compound = map[string]bool{
	"if": true, "while": true, "until": true, "case": true, "for": true, "select": true,
	"function": true, "!": true, "[": true, "[[": true, "test": true, "{": true,
}

// pipelineEnd returns the index of the last command of the pipeline that
// begins at command n, or -1 when there is none.
func pipelineEnd(cs []command, n int) int {
	if n >= len(cs) {
		return -1
	}
	for n < len(cs)-1 && cs[n].op == "|" {
		n++
	}
	return n
}

// exitsFailed reports a command that is `exit` returning the status before
// it: `exit` alone or `exit $?` -- and, after `||` (orElse), where that
// status is a failure, `exit N` for N from 1 to 255 too. The command must end
// its pipeline: `exit 1 | cat` exits a subshell.
func exitsFailed(toks []token, c command, orElse bool) bool {
	if c.op == "|" || toks[c.prog].text != "exit" || toks[c.prog].quotedAt >= 0 {
		return false
	}
	args := toks[c.prog+1 : c.end]
	switch {
	case len(args) == 0:
		return true
	case len(args) > 1 || args[0].meta:
		return false
	case args[0].text == "$?":
		return true
	}
	if !orElse {
		return false
	}
	n := 0
	for _, r := range args[0].text {
		if r < '0' || r > '9' || n > 255 {
			return false
		}
		n = n*10 + int(r-'0')
	}
	return n >= 1 && n <= 255
}

// isFalseCommand reports the command `false`, with no arguments.
func isFalseCommand(toks []token, c command) bool {
	return toks[c.prog].text == "false" && toks[c.prog].quotedAt < 0 && c.end == c.prog+1
}

// setOptions reads a `set` command's arguments for the two options that
// change whether a status is hidden: pipefail (`-o pipefail`) and errexit
// (`-e`, in any cluster of short options such as -euo, or `-o errexit`). A
// cluster holding o takes the next word as the option's name.
func setOptions(args []token) (pipefail, errexit bool) {
	for j := 0; j < len(args); j++ {
		w := args[j].text
		if args[j].meta || len(w) < 2 || w[0] != '-' || w[1] == '-' {
			continue
		}
		if strings.ContainsRune(w[1:], 'e') {
			errexit = true
		}
		if strings.ContainsRune(w[1:], 'o') && j+1 < len(args) {
			switch args[j+1].text {
			case "pipefail":
				pipefail = true
			case "errexit":
				errexit = true
			}
			j++
		}
	}
	return pipefail, errexit
}

// runnerDigest is the keyed digest of the words of the runner whose program
// is the token at i, up to the end of its command: what it was asked to run,
// with its redirections left out, so that `make test 2>&1 | tail` and a
// later plain `make test` are the same runner. Nil when a word is one this
// tokenizer cannot vouch for (opaque). Like the shape digest, compared and
// never printed; the domain is a name no tool has.
func runnerDigest(toks []token, i int, cs []command, key []byte) *string {
	end := len(toks)
	for _, c := range cs {
		if c.prog <= i && i < c.end {
			end = c.end
		}
	}
	var words []string
	for j := i; j < end; j++ {
		t := toks[j]
		switch {
		case t.opaque:
			return nil
		case !t.meta && j+1 < end && toks[j+1].meta && toks[j+1].glued && isRedirect(toks[j+1].text) && isFDPrefix(t):
			// 2 in 2>&1.
			continue
		case t.meta && t.text == "&":
			// &> and &>>: commandBound ended the command at any other &.
			j++
			fallthrough
		case t.meta && isRedirect(toks[j].text):
			// The operator, its further pieces, and its target.
			j = operatorEnd(toks, j) + 1
			continue
		case t.meta:
			// A group's close.
			continue
		}
		words = append(words, t.text)
	}
	d := digest(key, "\x00runner", []byte(strings.Join(words, "\x00")))
	return &d
}
