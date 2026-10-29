package shape

import (
	"encoding/json"
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// ToolVerb is the verb class a tool's name alone gives, before any command
// line is read: what Derive starts from. A shell tool is execute here
// whatever its command runs, so this is the answer for the tool, never for
// the call.
func ToolVerb(name string) string { return verbForTool(name) }

// readTools are the tools classed read by name alone. It is the one list:
// report's lookups (IsLookup) and the words the report and spend name them
// with are both read from it, through ReadTools.
var readTools = []string{"Read", "Glob", "Grep", "NotebookRead"}

// ReadTools returns the tools classed read by name alone, in a fresh slice a
// caller may keep or change.
func ReadTools() []string { return slices.Clone(readTools) }

func verbForTool(name string) string {
	if strings.HasPrefix(name, "mcp__") {
		return VerbMCP
	}
	if slices.Contains(readTools, name) {
		return VerbRead
	}
	switch name {
	case "Write", "Edit", "MultiEdit", "NotebookEdit":
		return VerbWrite
	case "WebFetch", "WebSearch":
		return VerbNetwork
	case "Bash", "BashOutput", "KillShell":
		return VerbExecute
	case "Task", "Agent":
		return VerbAgent
	default:
		return VerbUnknown
	}
}

// programVerb is a coarse classification and is documented as such. It exists to
// make a declaration readable at a glance, not to be a security boundary: an
// unrecognized program is "execute", which is the truthful answer for anything
// being run.
var programVerb = map[string]string{
	"cat": VerbRead, "less": VerbRead, "more": VerbRead, "head": VerbRead,
	"tail": VerbRead, "ls": VerbRead, "find": VerbRead, "grep": VerbRead,
	"rg": VerbRead, "wc": VerbRead, "stat": VerbRead, "file": VerbRead,
	"tree": VerbRead, "diff": VerbRead, "realpath": VerbRead, "pwd": VerbRead,

	"rm": VerbWrite, "mv": VerbWrite, "cp": VerbWrite, "mkdir": VerbWrite,
	"rmdir": VerbWrite, "touch": VerbWrite, "tee": VerbWrite, "ln": VerbWrite,
	"chmod": VerbWrite, "chown": VerbWrite, "truncate": VerbWrite,

	"curl": VerbNetwork, "wget": VerbNetwork, "ssh": VerbNetwork,
	"scp": VerbNetwork, "rsync": VerbNetwork, "nc": VerbNetwork,
	"dig": VerbNetwork, "host": VerbNetwork, "ping": VerbNetwork,

	"git": VerbVCS, "gh": VerbVCS, "hg": VerbVCS, "svn": VerbVCS,

	"npm": VerbPackage, "npx": VerbPackage, "yarn": VerbPackage,
	"pnpm": VerbPackage, "pip": VerbPackage, "pip3": VerbPackage,
	"go": VerbPackage, "cargo": VerbPackage, "gem": VerbPackage,
	"bundle": VerbPackage, "brew": VerbPackage, "apt": VerbPackage,
	"apt-get": VerbPackage, "uv": VerbPackage, "poetry": VerbPackage,
}

// refusalsOf names the notARun entry a testCommands row is checked against:
// its own program's, or for a wrapper the runner it wraps, since `npx jest
// --listTests` lists as `jest --listTests` does. gradlew and mvnw take
// gradle's and mvn's.
func refusalsOf(c []string) string {
	switch c[0] {
	case "npx", "uv", "poetry", "bundle":
		return c[len(c)-1]
	case "gradlew":
		return "gradle"
	case "mvn", "mvnw":
		if len(c) == 1 {
			// The build row: a flag that skips the tests still builds.
			return "mvn build"
		}
		return "mvn"
	}
	return c[0]
}

// notARun is, per runner by its first word, the arguments with which it does
// something other than run the tests: compile them (`go test -c`, `cargo test
// --no-run`), list them (`-list`, `--collect-only`, `--listTests`), print or
// check what it would do (`make -n`, `make -q`, `--dry-run`), skip them
// (`-DskipTests`, `tox --notest`, `nox --install-only`), set up an
// environment instead (`tox devenv`), watch for changes (`gradle -t`), show
// its version (`-v` where that is what -v means), or end ok whatever the
// tests did (`make -i`, `mvn --fail-never`, `npm --if-present` where there is
// no test script). Each of those ends ok or failed on something no test
// decided, and as a test run it would complete a pattern on that outcome. A
// word ending in * is a prefix: `-list=Foo`, `-elint`, matched whatever
// follows it. A flag is matched in its `flag=value` form too (`-c=true`,
// `--watch=true`), unless the value is false (onList). Maven's skip
// properties are refused by a rule of their own (mavenSkip):
// `-DskipTests=false` runs the tests. One set in MAVEN_OPTS or
// .mvn/maven.config is not on the line and is not seen.
//
// The arguments read are the ones after the row's words. Before them only
// leadingFlags may stand (runnerOn), none of which is on these lists, so
// `make -n test` matches no row at all.
//
// The lists are the spellings named here, not every spelling a runner
// accepts: a combined short flag (`make -nk`), an option set in a config file
// or an environment variable (other than PYTEST_ADDOPTS, below), or a
// spelling not listed still counts as a test run.
//
// tox and nox are here with their targets, not their listings only: `tox -e
// lint` and `nox -s lint` run a lint session, and a target cannot be told to
// be tests from its name without a guess. Plain `tox` and `nox` run the
// project's default sessions and stay test.
//
// Compared and dropped, like the runner's own words: a match refuses the
// class, and the word is not kept. A word is compared whether quoted or not,
// since the shell passes `"--watch"` as --watch; refusing on it is the
// under-claim.
var notARun = map[string][]string{
	"go":      {"-c", "--c", "-n", "--n", "-list*", "--list*"},
	"cargo":   {"--no-run", "--list", "-V"},
	"npm":     {"--if-present"},
	"pytest":  pytestNotARun,
	"python":  pytestNotARun,
	"python3": pytestNotARun,
	"jest":    {"--listTests", "--showConfig", "--clearCache", "--init", "-v"},
	"vitest":  {"watch", "dev", "list", "bench", "init", "-w", "-v"},
	"mocha":   {"-w", "-V", "--dry-run", "--list-reporters", "--list-interfaces"},
	"rspec":   {"--dry-run", "--init", "-v"},
	"phpunit": {"--list-*", "--generate-configuration", "--migrate-configuration", "--check-version"},
	"ctest":   {"-N", "--show-only*"},
	"dotnet":  {"--list-tests", "-t"},
	"mvn":     {"-v", "-fn", "--fail-never"},
	"gradle":  {"--dry-run", "-m", "-v", "-t", "--continuous"},
	"make": {"-n", "--just-print", "--dry-run", "--recon", "-q", "--question", "-t", "--touch", "-v",
		"-i", "--ignore-errors"},
	"tox": {"-e*", "--env*", "-m", "-f", "-l", "-a", "--listenvs*", "--showconfig", "--help-ini", "--notest",
		"--devenv*", "list", "l", "config", "c", "depends", "de", "quickstart", "q", "exec", "e", "devenv", "d"},
	"nox": {"-s*", "--session*", "-e*", "-k*", "--keywords*", "-t*", "--tags*", "-l", "--list*", "--install-only"},
}

var pytestNotARun = []string{
	"--collect-only", "--co", "-V", "--fixtures*", "--markers", "--setup-plan", "--setup-only", "--cache-show*",
}

// notARunAny is the spellings of help, version and watch mode that are
// checked for every runner: -h, -help, --help, --version, --watch and
// --watchAll. Watch mode never ends on a result of its own and records the
// launch's instead. A runner's own spelling of these (`-v`, `-V`, `-w`, `-t`,
// `vitest watch`) is on its notARun entry or is not refused at all; the list
// does not make every runner's help, version or watch mode refused.
var notARunAny = []string{"-h", "-help", "--help", "--version", "--watch", "--watchAll"}

// refusesRun reports an argument in args that is on runner's notARun list or
// on notARunAny, or, for pytest, a word starting PYTEST_ADDOPTS= earlier on
// the line (before), an assignment or an export's argument: its value is more
// arguments, which may be `--co`, and they are not read, so any value
// refuses. One exported by an earlier call is not on the line and is not
// seen.
func refusesRun(runner string, before, args []token) bool {
	// mvn's build row is refused as mvn is, but for the test-skip
	// properties: skipping the tests still builds (refusalsOf).
	list := notARun[strings.TrimSuffix(runner, " build")]
	for _, t := range args {
		if onList(t.text, list) || onList(t.text, notARunAny) || runner == "mvn" && mavenSkip(t.text) {
			return true
		}
	}
	if onList(runner, []string{"pytest", "python", "python3"}) {
		for _, t := range before {
			if strings.HasPrefix(t.text, "PYTEST_ADDOPTS=") {
				return true
			}
		}
	}
	return false
}

// onList reports word equal to an entry of list, or starting with the part
// of one before its trailing *. A word `flag=value` matches the entry flag
// unless value is a false the flag parsers read as false (isFalse):
// `--watchAll=false` runs the tests once, `--watchAll=true` watches, and
// `-c=true` compiles. A prefix entry takes no =false exception: its flag
// either takes a pattern or name (go -list, tox -e, nox -s/-k/-t, ctest
// --show-only, pytest --cache-show) or is a no-value switch whose parser
// rejects any =value (pytest --fixtures, nox --list, tox --listenvs, phpunit
// --list-*), so the line runs no test either way.
//
// A combined short flag is not split (notARun's lists name spellings, not
// clusters): that is safe for notARun, where a missed refusal still counts as
// a test run, and for the in-place editors shortClusters closes the
// letter-clustering case only; a new caller re-justifies it against its
// own safe direction.
func onList(word string, list []string) bool {
	name, value, hasValue := strings.Cut(word, "=")
	for _, w := range list {
		if p, ok := strings.CutSuffix(w, "*"); ok {
			if strings.HasPrefix(word, p) {
				return true
			}
		} else if word == w {
			return true
		} else if hasValue && strings.HasPrefix(w, "-") && name == w && !isFalse(value) {
			return true
		}
	}
	return false
}

// isFalse reports the false spellings of Go's strconv.ParseBool, for Go's and
// yargs's boolean flags; a yargs flag given another non-true value is still
// refused. Maven's properties follow Java instead (mavenSkip).
func isFalse(v string) bool {
	switch v {
	case "false", "False", "FALSE", "f", "F", "0":
		return true
	}
	return false
}

// mavenSkipProperties are the Maven properties mavenSkip reads: the ones
// that skip the tests, -DskipTests, -Dmaven.test.skip and
// -Dmaven.test.skip.exec, and the one that ignores their failures,
// -Dmaven.test.failure.ignore (Surefire's testFailureIgnore). A list, so
// that TestLeadingFlagsAreNotRefusals tries each against leadingFlags.
var mavenSkipProperties = []string{"-DskipTests", "-Dmaven.test.skip", "-Dmaven.test.skip.exec", "-Dmaven.test.failure.ignore"}

// mavenSkip reports one of mavenSkipProperties with no `=value` or with a
// value equal to true ignoring case. Maven reads them with Java's
// Boolean.valueOf, where nothing else is true, so `-DskipTests=false`, `=1`
// and `=no` all run the tests.
func mavenSkip(word string) bool {
	name, value, hasValue := strings.Cut(word, "=")
	return slices.Contains(mavenSkipProperties, name) && (!hasValue || strings.EqualFold(value, "true"))
}

// runsTests reports whether the command whose program is the token at i,
// named prog, is one of testCommands, and the whole line (wholeCommand), so
// that the line's exit status is the runner's.
//
// The program was found under programToken's rules, so it is certain; each
// word after it must be certain in the same way before it is compared: an
// unquoted word of this same command, and not running on into a process
// substitution the shell reads as more of the same word. A word on the next
// line is the next command's, and wholeCommand refuses any line that has one.
// An operator or an expansion keeps its bytes in the token's text ($, `, ${ ),
// so equality to a list word already refuses those. programToken has refused
// a command whose end it could not find, so the words up to that end are ones
// the lexer vouched for -- and a line the lexer could not finish at all
// (whole) is refused outright: `go test\` ends in a backslash the tokenizer
// dropped and the shell keeps.
//
// Quoted is refused although `go "test"` runs the tests: the rule is "a plain
// word equal to the list", and a quote is where plain stops. And a runner on
// the list is still refused when an argument after its words is on notARun.
func runsTests(toks []token, i int, prog string, whole bool) bool {
	if !whole {
		return false
	}
	i, prog, ok := runnerPrefix(toks, i, prog)
	if !ok {
		return false
	}
	return runnerOn(toks, i, prog, len(toks), testCommands) && wholeCommand(toks, i)
}

// runnerPrefix steps past the prefixes that pass a runner's exit status
// through: `timeout DURATION`, then `time` (`time -p`), each at most once
// and in that order. It returns the index and name of the word after them,
// or i and prog unchanged when there is no prefix; ok is false when a prefix
// is not followed by a word this search can vouch for.
//
// timeout is taken only as `timeout DURATION`: an option before the duration
// (`-k 5`, `-s KILL`, `--preserve-status`) changes what status it exits
// with, and is refused rather than read. Its own 124, when it fires, is the
// timeout's status and not the runner's; the report reads that as no result.
// The recorded program stays timeout or time, the first word.
func runnerPrefix(toks []token, i int, prog string) (int, string, bool) {
	// A plain word of this command. The duration is all digits, which
	// plainWord refuses in command position, so it is held only to the rest.
	plain := func(j int) bool {
		if j >= len(toks) {
			return false
		}
		t := toks[j]
		return !t.meta && !t.nlBefore && !t.opaque && t.quotedAt < 0 && !runsOn(toks, j)
	}
	word := func(j int) bool { return plain(j) && plainWord(toks[j].text) }
	if prog == "timeout" {
		if !plain(i+1) || !isDuration(toks[i+1].text) || !word(i+2) {
			return i, prog, false
		}
		i += 2
		prog = path.Base(toks[i].text)
	}
	if prog == "time" {
		next := i + 1
		if next < len(toks) && toks[next].text == "-p" && word(next) {
			next++
		}
		if !word(next) {
			return i, prog, false
		}
		i = next
		prog = path.Base(toks[i].text)
	}
	return i, prog, true
}

// isDuration reports a timeout(1) duration: digits, an optional fraction, and
// an optional unit.
func isDuration(w string) bool {
	if n := len(w); n > 0 && strings.IndexByte("smhd", w[n-1]) >= 0 {
		w = w[:n-1]
	}
	whole, frac, dot := strings.Cut(w, ".")
	digits := func(s string) bool {
		for i := 0; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return false
			}
		}
		return true
	}
	return whole != "" && digits(whole) && digits(frac) && (!dot || frac != "")
}

// wholeCommand reports whether the runner whose program is the token at i is
// the whole line: nothing follows its own words, arguments and redirections
// but the end of the line.
//
// The recorded outcome is the line's exit status, and it is the runner's only
// when the runner ran last and in the foreground. `go test ./... 2>&1 | tail`
// ends with tail's status, `|| true` and `; echo done` with true's and echo's,
// `&` with the fork's, and `| grep FAIL` inverts it: grep exits 1 when the
// tests passed. `&&` is refused as well, although a failure there is the
// runner's: its success is whatever ran after it. So any separator, and any
// next line, leaves the class what it was before -- the under-claim. The end
// is found by commandEnd, the scan pastDirectoryChange uses, so a `;` inside
// `$( )` ends nothing; a newline anywhere after the runner refuses, whether
// commandEnd reached it or stopped at it. The one separator passed is the
// `;` of a `; }` that closes a brace group opened before the runner, and a
// `)` is the line's only where it closes a `(` opened there (closesGroups):
// `{ go test ./...; }` and `(go test ./...)` are whole, and `go test ./... )`
// is a syntax error that runs nothing.
func wholeCommand(toks []token, i int) bool {
	for _, t := range toks[i+1:] {
		if t.nlBefore {
			return false
		}
	}
	return closesGroups(toks, i)
}

// backgrounded reports a shell call Claude Code was asked to run in the
// background: its tool_input's run_in_background is the JSON boolean true.
//
// Such a call's PostToolUse fires when the shell is launched, not when the
// command ends, so its recorded outcome is the launch's: ok, before any test
// has run. The digest covers the command line only, so the launch digests
// equal to the same command run in the foreground, and as a test run it would
// pair with a real run as passing. It keeps the class the program gave it.
// The field is read as a boolean and nothing else, like the command field is
// read for its words and dropped.
//
// A command Claude Code moves to the background itself, when it reaches its
// timeout or on Ctrl+B, asked for nothing: its tool_input has no such field,
// so it is a test run here. Its execution record says backgrounded instead
// (read from the response on the post path; on Ctrl+B only if Claude Code
// marks it with backgroundTaskId or backgroundedByUser, which was not
// measured), and the report reads such a run as outcome unobserved, as it
// does a launch declared here.
func backgrounded(raw json.RawMessage) bool {
	var s [1]jsonscan.Span
	if len(raw) == 0 || jsonscan.Field(raw, jsonscan.ValueSpan(raw), runInBackgroundKey, s[:]) != nil {
		return false
	}
	return s[0].Kind == jsonscan.True
}

// runInBackgroundKey matches run_in_background in any case, as the struct
// field it was once decoded into did; command and the other fields shape
// reads were decoded from a map, and are matched exactly.
var runInBackgroundKey = jsonscan.NewFoldKeySet("run_in_background")

// mayWrite reports a shell line whose program is the token at i and which may
// write files whatever class that program gives it: an output redirection to
// a file (not a descriptor duplication such as 2>&1, and not /dev/null); a
// word that writes by itself -- find's -delete, -exec, -execdir, -ok and
// -okdir, xargs, tee, wget, rsync or scp; curl with an output flag (-o, -O,
// --output, --remote-name, or a run of short flags that reaches o or O, with
// or without the file attached); a command or process substitution, whose
// command is not looked into; or a later pipeline or list stage whose program
// is outside the read class, or cannot be named.
//
// The words are compared wherever they stand, quoted or not, and not parsed
// as the program would parse them: `grep -rn xargs .` sets the bit. That is
// the direction to be wrong in. The bit makes a call count as a possible file
// edit (the report's mayEdit), and an edit counted that did not happen stops
// a test-bending pair, while one missed completes a pair over it.
//
// Stages before i are scanned for writer words and redirects, but not classed
// by program: they are the `cd DIR &&` that pastDirectoryChange stepped over,
// and a directory change is told apart by the declarations' directory
// digests, not as a write.
func mayWrite(toks []token, i int) bool {
	if inPlaceEdit(toks) {
		return true
	}
	curl, curlOut := false, false
	for j, t := range toks {
		if opensSubstitution(toks, j) {
			return true
		}
		if t.meta {
			if outputRedirect(toks, j) {
				return true
			}
			continue
		}
		if t.ticks > 0 || t.opaque && holdsSubstitution(t.text) {
			return true
		}
		if slices.Contains(inlineWriters, path.Base(t.text)) || slices.Contains(findWriteWords, t.text) {
			return true
		}
		if path.Base(t.text) == "curl" {
			curl = true
		}
		curlOut = curlOut || curlOutputFlag(t.text)
	}
	if curl && curlOut {
		return true
	}
	for j := i + 1; j < len(toks); j++ {
		start := j
		switch {
		case stageSeparator(toks, j):
			start = j + 1
		case toks[j].nlBefore:
		default:
			continue
		}
		if start >= len(toks) {
			break
		}
		k, ok := programToken(toks[start:], false)
		if !ok || verbForProgram(path.Base(toks[start+k].text)) != VerbRead {
			return true
		}
	}
	return false
}

// inlineWriters are the programs whose word anywhere in a stage sets may_write
// (mayWrite): they take a file or a command to run as an argument, and the
// words that name which are not read.
var inlineWriters = []string{"xargs", "tee", "wget", "rsync", "scp"}

// findWriteWords are find's actions that delete or run a command, and set
// may_write wherever they stand.
var findWriteWords = []string{"-delete", "-exec", "-execdir", "-ok", "-okdir"}

// outputRedirect reports an output redirection whose operator starts at j and
// whose target is a file: `>`, `>>`, `>|`, `&>`, `<>`, and `>&` onto a word
// that is not a descriptor number or `-`. /dev/null, /dev/stdout,
// /dev/stderr and /dev/tty are not files it writes.
func outputRedirect(toks []token, j int) bool {
	t := toks[j]
	if t.text != ">" && t.text != ">>" {
		return false
	}
	if j > 0 && toks[j-1].meta && toks[j-1].text == "<" && t.glued {
		return true // <>: opened for reading and writing, and created
	}
	end := operatorEnd(toks, j)
	dup := false
	for k := j + 1; k <= end; k++ {
		dup = dup || toks[k].text == "&"
	}
	if end+1 >= len(toks) || toks[end+1].meta {
		// No target: nothing on the line runs. A process substitution
		// writes into a command, which may itself write a file, so it
		// counts as a possible write.
		return end+1 < len(toks)
	}
	target := toks[end+1].text
	if dup && (target == "-" || isFDPrefix(toks[end+1])) {
		return false
	}
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr", "/dev/tty":
		return false
	}
	return true
}

// opensSubstitution reports a token at j that opens a command or process
// substitution: an unquoted `$`, `<` or `>` with a `(` glued after it. A `$`
// with two glued `(` opens arithmetic, `$((n+1))`, which runs nothing, when
// arithmeticAt holds; otherwise it opens a substitution of a subshell. The
// command inside is not classed.
func opensSubstitution(toks []token, j int) bool {
	t := toks[j]
	if !gluedParen(toks, j+1) {
		return false
	}
	if t.meta {
		return t.text == "<" || t.text == ">"
	}
	return strings.HasSuffix(t.text, "$") && !(gluedParen(toks, j+2) && arithmeticAt(parenText(toks[j+3:])))
}

// holdsSubstitution reports text that holds a `$(` opening a command
// substitution: any `$(` but the `$((` of arithmetic. A `$(` nested inside
// arithmetic is followed by a byte other than `(`, so it still counts.
func holdsSubstitution(s string) bool {
	for k := 0; k < len(s); k++ {
		if !strings.HasPrefix(s[k:], "$(") {
			continue
		}
		if !strings.HasPrefix(s[k+2:], "(") || !arithmeticAt(s[k+3:]) {
			return true
		}
	}
	return false
}

// arithmeticAt reports whether s, the text after a `$((`, is arithmetic: the
// `)` that matches the second `(` is followed by a glued `)`. Bash and zsh
// read a `$((` that does not close as `))`, such as `$((rm x) )`, as a
// command substitution of a subshell. comsub.go's arithEnd is the stricter,
// tokenizer-side form of the same rule.
func arithmeticAt(s string) bool {
	depth := 1
	for k := 0; k < len(s); k++ {
		switch s[k] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return strings.HasPrefix(s[k+1:], ")")
			}
		}
	}
	return false
}

// parenText rebuilds the line from toks as far as arithmeticAt reads it: an
// operator as written, any other word as one placeholder byte, since an
// unquoted ( or ) is always an operator token, and a blank where two tokens
// were apart.
func parenText(toks []token) string {
	var b strings.Builder
	for k, t := range toks {
		if k > 0 && !t.glued {
			b.WriteByte(' ')
		}
		if t.meta {
			b.WriteString(t.text)
		} else {
			b.WriteByte('w')
		}
	}
	return b.String()
}

// gluedParen reports a meta `(` at j glued to the token before it.
func gluedParen(toks []token, j int) bool {
	return j < len(toks) && toks[j].meta && toks[j].glued && toks[j].text == "("
}

// stageSeparator reports a token at j that ends one pipeline or list stage:
// `;`, `&&`, `||`, `|`, or a lone `&` -- not the `&` of `>&`, `&>` or `|&`,
// which belongs to a redirection or a pipe.
func stageSeparator(toks []token, j int) bool {
	t := toks[j]
	if !t.meta {
		return false
	}
	switch t.text {
	case ";", "&&", "||", "|":
		return true
	case "&":
		if j > 0 && t.glued && toks[j-1].meta && (toks[j-1].text == ">" || toks[j-1].text == ">>" || toks[j-1].text == "|") {
			return false
		}
		if next := j + 1; next < len(toks) && toks[next].meta && toks[next].glued && strings.HasPrefix(toks[next].text, ">") {
			return false
		}
		return true
	}
	return false
}

// curlOutputFlag reports a curl argument that names an output file: -o, -O,
// their long forms, or a short-option word that reaches o or O. The word is
// walked as curl reads it. A letter, a digit, # or : that takes no argument is
// a flag, and the walk goes on past it; an option that takes an argument ends
// the walk, since the rest of the word is its value (`-XPOST`, `-dfoo=bar`),
// and so does any other character. Whatever follows o is its file, so
// `-o./calc.go`, `-#O` and `-sSLotestdata/x.json` name files.
func curlOutputFlag(w string) bool {
	switch {
	case slices.Contains(curlLongOutputs, w),
		strings.HasPrefix(w, "--output="), strings.HasPrefix(w, "--output-dir="):
		return true
	case len(w) > 1 && w[0] == '-' && w[1] != '-':
		for _, c := range w[1:] {
			switch {
			case c == 'o' || c == 'O':
				return true
			case strings.ContainsRune(curlArgumentOptions, c):
				return false
			case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '#', c == ':':
			default:
				return false
			}
		}
	}
	return false
}

// curlLongOutputs are curl's long options that name where a download is
// written, with no =value; --output= and --output-dir= are read by prefix.
var curlLongOutputs = []string{"--output", "--remote-name", "--remote-name-all", "--output-dir"}

// curlArgumentOptions are curl's short options that take an argument, per
// `curl --help all`.
const curlArgumentOptions = "AbcCdDeEFhHKmPQrtTuUwxXyYz"

func verbForProgram(prog string) string {
	if v, ok := programVerb[prog]; ok {
		return v
	}
	return VerbExecute
}

// inPlaceEditors are programs that rewrite files in place, with the words
// that make them do so: a flag (`sed -i`, `perl -pi`, `gofmt -w`,
// `prettier --write`, `clang-format -i`, `eslint --fix`), a subcommand
// (`go fmt`, `cargo fmt`, `ruff format`), or nothing, for a formatter that
// writes unless told to check (black, isort, rustfmt). A trailing `*` matches
// a prefix, as onList reads it, so `-i.bak` and `-pie` count. BSD sed's -I
// edits in place as -i does (its man page's SYNOPSIS: `[-I extension] [-i
// extension]`); GNU sed has no -I, so the prefix reads nothing else.
var inPlaceEditors = map[string][]string{
	"sed":          {"-i*", "-I*", "--in-place*"},
	"perl":         {"-i*", "-pi*", "-ni*", "-0pi*"},
	"gofmt":        {"-w"},
	"go":           {"fmt"},
	"cargo":        {"fmt"},
	"ruff":         {"format"},
	"prettier":     {"--write", "-w"},
	"clang-format": {"-i"},
	"eslint":       {"--fix"},
	"black":        {},
	"isort":        {},
	"rustfmt":      {},
}

// shortClusters are the in-place editors whose option grammar bundles
// single-letter options into one word, with the letters that edit in place:
// i for both, and BSD sed's I. sed's grammar (GNU sed's Command-Line
// Options, and the BSD sed man page) and perl's (perlrun) both bundle, and
// both let the in-place letter take an attached suffix, so it is found by
// scanning the cluster's letters in order (clusterHas): `sed -ni`, `sed
// -nI.bak`, `sed -li` and `perl -lpi` edit in place. A letter
// in consumes takes the rest of the word as its value (`-escript`,
// `-Mmodule`) and ends the scan, so `sed -ne` and `perl -Mstrict` do not.
// sed's -l is not a consumer: BSD sed, the sed macOS ships, reads it as
// line-buffered output with no argument. GNU sed's -l N does take one, and
// the word where the two collide, `-l5i`, reads as a write: the over-claim.
// perl's -l[octnum] takes digits only and is not a consumer either;
// -x[directory] is, since whatever follows x in the word is its directory.
// gofmt and clang-format are not clustered (Go's flag package and LLVM read
// -w and -i whole) and are not here.
var shortClusters = map[string]shortCluster{
	"sed":  {letters: "iI", consumes: "ef"},
	"perl": {letters: "i", consumes: "eEMmIFx"},
}

type shortCluster struct {
	letters  string
	consumes string
}

// checkOnly are the words under which a formatter reads and reports instead
// of writing, or prints its help or version; one of them anywhere in the
// stage clears the editor. -l is not one: gofmt's -l only lists the files
// that differ and writes with -w beside it (`gofmt -l -w .`), black's and
// isort's is a line length, sed's a line length or line-buffered output, and
// perl's a line-ending switch, so none of them reads instead of writing.
// `gofmt -l .` reads because no writing word armed the stage.
var checkOnly = []string{"--check", "--check-only", "--diff", "--list-different", "--dry-run",
	"--help", "--version", "-h"}

// inPlaceEdit reports a line with a stage on which an in-place editor stands
// with a word that makes it write (inPlaceEditors) and no word that makes it
// only check (checkOnly). Words are compared wherever they stand in the
// stage, as mayWrite compares its own: the direction to be wrong in. A
// clustered editor's word is also scanned as a cluster (shortClusters). A
// stage separator starts the search over, so `sed -n p a | grep -i x` is a
// read.
func inPlaceEdit(toks []token) bool {
	var flags []string
	var cluster shortCluster
	armed, checking, clustered := false, false, false
	for j, t := range toks {
		if stageSeparator(toks, j) {
			if armed && !checking {
				return true
			}
			flags, armed, checking, clustered = nil, false, false, false
			continue
		}
		if t.meta {
			continue
		}
		if f, ok := inPlaceEditors[path.Base(t.text)]; ok {
			flags = f
			armed = len(f) == 0
			cluster, clustered = shortClusters[path.Base(t.text)]
			continue
		}
		if flags == nil {
			continue
		}
		switch {
		case onList(t.text, checkOnly):
			checking = true
		case onList(t.text, flags):
			armed = true
		case clustered && clusterHas(t.text, cluster):
			armed = true
		}
	}
	return armed && !checking
}

// clusterHas reports a word that is a cluster of short options (one dash,
// then a letter or a 0, and no =) in which one of c.letters stands before
// any letter of c.consumes: the walk stops at the first of either, since a
// consuming letter takes the rest of the word as its value. A 0 leads
// perl's -0[digits], the record separator, whose digits the walk passes
// over as it passes -l's, so `perl -0777pi` and `perl -00pi` edit in place.
// No sed option is a digit; `sed -0i` is an error to both seds, and reads
// as a write here: the over-claim.
func clusterHas(tok string, c shortCluster) bool {
	if len(tok) < 2 || tok[0] != '-' || !('a' <= tok[1] && tok[1] <= 'z' || 'A' <= tok[1] && tok[1] <= 'Z' || tok[1] == '0') ||
		strings.Contains(tok, "=") {
		return false
	}
	for i := 1; i < len(tok); i++ {
		switch {
		case strings.IndexByte(c.letters, tok[i]) >= 0:
			return true
		case strings.IndexByte(c.consumes, tok[i]) >= 0:
			return false
		}
	}
	return false
}
