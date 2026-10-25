package shape

// Masked kinds: what shape.status_masked says of a shell line. A closed set,
// like the verb classes, and decided against fixed lists whose words are
// compared and dropped.
const (
	// MaskedNone: the line was read to its end, and every recognised build
	// or test runner on it has its failure reach the line's exit status --
	// or there is none.
	MaskedNone = "none"
	// MaskedTest: a recognised test runner (testCommands) runs on the line,
	// and the line exits 0 when it fails, or with the same status whether it
	// fails or passes.
	MaskedTest = "test"
	// MaskedBuild: the same for a recognised build runner (buildCommands),
	// and no test runner's status is hidden.
	MaskedBuild = "build"
)

// RulesVersion names every table this build decides a recorded shape field
// with (verb_class, status_masked, runner_kind, may_write), and the matchers
// that read them, and the rule that decides a declaration's cwd_digest, and
// moves whenever one of them changes. The tables are the test runners
// (testCommands) and the build runners (buildCommands), with the arguments
// that refuse one (notARun, notARunAny) and the flags that may stand before
// its words (leadingFlags), which set verb_class, status_masked and
// runner_kind; the in-place tables (inPlaceEditors, checkOnly, shortClusters),
// which set may_write; and the pass words (the report's passVocabulary), which
// decide when the masked-runs line fires. cwd_digest's producers are which
// leading cd steps are read (leadingCd), how the hook folds them onto the
// payload's cwd (its startDirectory, `~` expanded with its own home), and the
// domain of a fold that is not read (UnreadCWDDigest). Written on every
// declaration and execution as rules_version, so records written before and
// after a change can be told apart.
//
// Bump it whenever a table, or a matcher or rule that decides a recorded
// field, changes -- version 4 is the first bump for a matcher, runnerOn
// passing over leadingFlags before a runner's words; version 5 passes the
// masked parser over the leading cd steps the program search steps over
// where its reading agrees (agreedFold), reads `~`, and gives an unread fold
// a location of its own --
// and add the new pins to the tests that hold one (rules_version_test.go here,
// in report and in hook): a table's pin fails when the table changes without
// a bump, the matcher pin when a field moves on one of its corpus lines
// without one, and the hook's location pin when a cwd digest moves on one of
// its pairs without one. The exceptions: notARun, notARunAny and leadingFlags
// have no table pin, and are held only as far as the matcher pin's lines read
// them; and mayWrite's inline writer-word checks (xargs, tee, wget, rsync,
// scp, find's -delete and -exec words, curl's output flags with
// curlArgumentOptions) also decide may_write and are not yet under a pin. The
// first change to any of them brings it under one.
const RulesVersion = 5

// MaskedKinds is the vocabulary of status_masked, for the schema enum test.
func MaskedKinds() []string {
	return []string{MaskedNone, MaskedTest, MaskedBuild}
}

// Runner kinds: what shape.runner_kind says of the runner runner_digest
// digests, which is the list it is on. A closed set that shares two words
// with the masked kinds and is a different fact: status_masked is none on a
// line that returns its runner's status, and runner_kind still names the
// runner.
const (
	// RunnerTest: the runner is on testCommands.
	RunnerTest = "test"
	// RunnerBuild: the runner is on buildCommands and not on testCommands.
	RunnerBuild = "build"
)

// RunnerKinds is the vocabulary of runner_kind, for the schema enum test.
func RunnerKinds() []string {
	return []string{RunnerTest, RunnerBuild}
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
	{"dotnet", "build"}, {"cmake", "--build"}, {"ninja"}, {"bazel", "build"},
}

// maxRunnerGroups bounds how many distinctly-worded runners statusMasked
// will re-run the line for. See the check inside statusMasked for why: past
// it, the kind is null, the same answer a line this function gives up
// reading already returns.
const maxRunnerGroups = 64

// statusMasked reports, for a shell line, whether a recognised build or test
// runner on it has an exit status the line does not return, and the keyed
// digest of that runner's own words -- or, when none does, of a runner whose
// status the line does return (runnerDigest) -- with the kind of the runner
// digested (runnerOf).
//
// The question is the one a failing runner asks of the line: when it fails,
// does the line? The line is parsed (lists, && and ||, pipelines, `!`, ( ),
// { }, if, while, until and for) and then run twice in the abstract, once
// with the runner failing and once passing, every other command succeeding
// and every other recognised runner passing. The same runner written twice is
// one runner, failing or passing at both places. Followed: `set -e` and `set
// -o pipefail` and their `+` forms, which errexit ignores (a command before
// && or ||, a condition, a negated pipeline), `exit` with no argument, a
// number, `$?` or a variable assigned one of those on the line, and a
// pipeline's stages as subshells, as bash runs them.
//
// Claude Code runs a line in the user's shell, which on a stock macOS is zsh,
// as `zsh -c "... && eval '<line>' < /dev/null && ..."`, where errexit never
// fires and PIPESTATUS is empty, and the record does not say which shell ran
// it. So a line where a set turns errexit on is run again with errexit never
// firing, and is null where the two runs disagree (runShells); PIPESTATUS is
// not read, and a line whose status depends on it is null.
//
//   - The line exits 0 when the runner fails, or with the same status either
//     way: the runner's status is hidden, and the kind is reported --
//     `make test 2>&1 | tail -40`, `go test ./...; echo $?`, `npm test ||
//     true`, `if make test; then echo ok; fi`, `! make test`, and `make test
//     &`, whose status is no one's.
//   - The line exits non-zero when the runner fails and 0 when it passes:
//     MaskedNone -- `make test && echo ok`, `make test || exit 1`, `set -o
//     pipefail; make test | tail`, `set -euo pipefail; make test | tail`.
//
// Where it is not sure the runner's status reaches the line's -- a word it
// cannot read, a status the abstract run cannot tell (a test of $?, `wait`
// with an argument, PIPESTATUS), a loop that holds a runner, an exit or a
// status read, a runner the abstract run never reaches, a line bash and zsh
// return differently (`set -e; make test; echo done`), `case`, a function,
// `eval`, `trap`, `exec`, or a shell's `-c`, whose line is not looked into --
// it says nothing: null, never a kind and never "none". So does a line that
// cannot be read to its end (tokenizeList, parseLine).
//
// The leading cd steps the program search steps over, fold, its own reading
// of them, are passed over here too, read or not, where this reading's steps
// are the same tokens (agreedFold): the runner after them keeps its digest,
// which is its words, and where it ran is the cwd digest's to say. A cd,
// pushd or popd past them, past MaxLeadingCd steps, or in a line the two
// readings fold apart, displaces the runners after it (displace).
func statusMasked(cmd string, key []byte, fold []token) (kind, runner, runnerKind *string) {
	if controlByte(cmd) {
		return nil, nil, nil
	}
	toks, err := tokenizeList(cmd)
	if err != nil {
		return nil, nil, nil
	}
	p := &lineParser{toks: toks, groups: map[string]int{}, assigned: map[string]bool{}}
	body, ok := p.parseLine()
	if !ok {
		return nil, nil, nil
	}
	if len(p.runners) > maxRunnerGroups {
		// Not sure: null. runShells below re-runs the whole parsed line for
		// every group (twice, or four times where the line turns errexit
		// on), so the cost is runners times line size -- a line of
		// thousands of distinctly-named runners, each its own group because
		// runnerWords keys a group by its exact words, made that the
		// square of the payload: 16,000 `make tN;` groups took 4.5s and
		// 4,000 `make tN|tail;` ones allocated 4.4 GB. A line this shaped is
		// already past what a build or test script looks like.
		return nil, nil, nil
	}
	p.displace(agreedFold(toks, fold))

	var (
		hidden []int
		reach  = -1
	)
	for g := range p.runners {
		f, pass, ran := p.runShells(body, g)
		if f < 0 || pass < 0 || !ran {
			// Not sure: null.
			return nil, nil, nil
		}
		if f == 0 || f == pass {
			hidden = append(hidden, g)
			continue
		}
		if reach < 0 || p.runners[g].last > p.runners[reach].last {
			reach = g
		}
	}
	none := MaskedNone
	switch {
	case len(hidden) > 0:
		k := MaskedBuild
		for _, g := range hidden {
			if p.runners[g].kind == MaskedTest {
				k = MaskedTest
			}
		}
		if len(hidden) > 1 {
			// No one later run follows up two runners.
			return &k, nil, nil
		}
		d, rk := p.runnerOf(hidden[0], key)
		return &k, d, rk
	case reach >= 0:
		d, rk := p.runnerOf(reach, key)
		return &none, d, rk
	}
	return &none, nil, nil
}

// runnerOf is runner group g's digest and kind, the one place either is
// decided: the kind is set exactly when the digest is, and names the list
// of the runner the digest is of (D9), so the two never disagree.
func (p *lineParser) runnerOf(g int, key []byte) (digest, kind *string) {
	d := p.runnerDigest(g, key)
	if d == nil {
		return nil, nil
	}
	k := RunnerBuild
	if p.runners[g].kind == MaskedTest {
		k = RunnerTest
	}
	return d, &k
}

// The parsed line. A list is and-or lists separated by `;`, `&` or a
// newline; an and-or list is pipelines joined by && and ||; a pipeline is
// commands joined by | or |&, maybe negated by `!`.
type (
	list     struct{ items []listItem }
	listItem struct {
		ao andOr
		bg bool // ended by &: run in the background
	}
	andOr struct {
		pipes []pipeline
		ops   []string // ops[k] joins pipes[k] and pipes[k+1]
	}
	pipeline struct {
		neg    bool
		stages []command
	}
	command  interface{}
	subshell struct{ body list }
	group    struct{ body list }
	ifCmd    struct {
		conds, bodies []list
		els           *list
	}
	// loop is a while, until or for loop, which is not run: unsure when it
	// holds a runner, an exit, a status read or a set, and otherwise a
	// command that succeeds.
	loop struct{ unsure bool }
)

// simpleOp is what a simple command does to the abstract run.
type simpleOp int

const (
	opOther      simpleOp = iota // succeeds
	opTrue                       // true, :
	opFalse                      // false
	opExit                       // exit
	opSet                        // set, which may change errexit and pipefail
	opAssign                     // assignments only, or export, local and the like
	opStatusRead                 // a test of a status: its own status is not known
	opRunner                     // a recognised build or test runner
	opCd                         // cd, pushd, popd
)

type simple struct {
	start, end int
	op         simpleOp
	runner     int // the runner group, for opRunner
	args       []value
	opts       []setOpt
	assigns    []assignment
}

type setOpt struct {
	errexit bool // else pipefail
	on      bool
}

type assignment struct {
	name string
	val  value
}

// value is a word whose number the abstract run can tell: a literal, $? or a
// variable. Anything else, PIPESTATUS among it, is unknown.
type value struct {
	kind valueKind
	n    int
	name string
}

type valueKind int

const (
	valUnknown valueKind = iota
	valLiteral
	valStatus
	valVar
)

// runnerGroup is one recognised runner of the line: its words, wherever
// they appear, and its kind.
type runnerGroup struct {
	key       string // the words runnerDigest digests
	kind      string
	starts    []int // the first token of each command that runs it
	last      int
	displaced bool // a cd, pushd or popd past the leading fold ran before it
}

type lineParser struct {
	toks     []token
	pos      int
	simples  []*simple
	runners  []runnerGroup
	groups   map[string]int
	assigned map[string]bool // names an assignment on the line set
	errexit  bool            // a set on the line turns errexit on
}
